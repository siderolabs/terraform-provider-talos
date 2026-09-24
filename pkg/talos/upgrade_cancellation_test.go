// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the real upgrade path and its process-wide lock

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	cosiapi "github.com/cosi-project/runtime/api/v1alpha1"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	cosiserver "github.com/cosi-project/runtime/pkg/state/protobuf/server"
	"github.com/hashicorp/terraform-plugin-framework/types"
	commonapi "github.com/siderolabs/talos/pkg/machinery/api/common"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	storageapi "github.com/siderolabs/talos/pkg/machinery/api/storage"
	k8sres "github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/protobuf/types/known/emptypb"
)

type cancellationMachineServer struct {
	machineapi.UnimplementedMachineServiceServer
	rebooted chan struct{}
	reboots  atomic.Int32
}

func (*cancellationMachineServer) Version(context.Context, *emptypb.Empty) (*machineapi.VersionResponse, error) {
	return &machineapi.VersionResponse{Messages: []*machineapi.Version{{Version: &machineapi.VersionInfo{Tag: "v1.14.0"}}}}, nil
}

func (*cancellationMachineServer) Read(_ *machineapi.ReadRequest, stream grpc.ServerStreamingServer[commonapi.Data]) error {
	return stream.Send(&commonapi.Data{Bytes: []byte("original-boot-id")})
}

func (*cancellationMachineServer) Events(_ *machineapi.EventsRequest, stream grpc.ServerStreamingServer[machineapi.Event]) error {
	if err := stream.SendHeader(nil); err != nil {
		return err
	}

	<-stream.Context().Done()

	return stream.Context().Err()
}

func (s *cancellationMachineServer) Reboot(context.Context, *machineapi.RebootRequest) (*machineapi.RebootResponse, error) {
	if s.reboots.Add(1) == 1 {
		close(s.rebooted)
	}

	return &machineapi.RebootResponse{Messages: []*machineapi.Reboot{{ActorId: "upgrade-test"}}}, nil
}

type cancellationImageServer struct {
	machineapi.UnimplementedImageServiceServer
}

func (*cancellationImageServer) Pull(*machineapi.ImageServicePullRequest, grpc.ServerStreamingServer[machineapi.ImageServicePullResponse]) error {
	return nil
}

type cancellationLifecycleServer struct {
	machineapi.UnimplementedLifecycleServiceServer
	prepared chan struct{}
}

func (s *cancellationLifecycleServer) Upgrade(*machineapi.LifecycleServiceUpgradeRequest, grpc.ServerStreamingServer[machineapi.LifecycleServiceUpgradeResponse]) error {
	s.prepared <- struct{}{}

	return nil
}

type cancellationStorageServer struct {
	storageapi.UnimplementedStorageServiceServer
}

func (*cancellationStorageServer) Disks(context.Context, *emptypb.Empty) (*storageapi.DisksResponse, error) {
	return &storageapi.DisksResponse{}, nil
}

// Use local APIs rather than replacing production functions: preparation, drain,
// the reboot tracker and the deferred lock release all run through their real code.
func cancellationEndpoint(t *testing.T, machine *cancellationMachineServer) (string, <-chan struct{}) {
	t.Helper()

	certServer := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(certServer.Close)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{
		Certificates: certServer.TLS.Certificates,
		MinVersion:   tls.VersionTLS12,
	})))
	t.Cleanup(server.Stop)
	machineapi.RegisterMachineServiceServer(server, machine)
	machineapi.RegisterImageServiceServer(server, &cancellationImageServer{})

	prepared := make(chan struct{}, 3)
	machineapi.RegisterLifecycleServiceServer(server, &cancellationLifecycleServer{prepared: prepared})
	storageapi.RegisterStorageServiceServer(server, &cancellationStorageServer{})

	state := inmem.NewState(k8sres.NamespaceName)
	nodename := k8sres.NewNodename(k8sres.NamespaceName, k8sres.NodenameID)
	nodename.TypedSpec().Nodename = "test-node"
	require.NoError(t, state.Create(t.Context(), nodename))
	cosiapi.RegisterStateServer(server, cosiserver.NewState(state))

	listener, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	go func() { assert.NoError(t, server.Serve(listener)) }()

	return listener.Addr().String(), prepared
}

func TestUpgradeCancellationStopsSubsequentUpgrades(t *testing.T) {
	// Do not run in parallel: the real upgrade function uses the process-wide lock.
	for _, stage := range []string{"drain", "reboot"} {
		t.Run(stage, func(t *testing.T) {
			previous := machineUpgradeLock
			machineUpgradeLock = newUpgradeLock()

			t.Cleanup(func() { machineUpgradeLock = previous })

			ctx, timeout := context.WithTimeout(t.Context(), 10*time.Second)
			defer timeout()

			activeCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			machine := &cancellationMachineServer{rebooted: make(chan struct{})}
			endpoint, prepared := cancellationEndpoint(t, machine)
			draining := make(chan struct{})

			var cordons atomic.Int32

			kubeAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")

				switch r.URL.Path {
				case "/api/v1/nodes/test-node":
					if r.Method == http.MethodPatch {
						cordons.Add(1)
					}

					assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{
						"apiVersion": "v1", "kind": "Node",
						"metadata": map[string]any{"name": "test-node"},
						"spec":     map[string]any{"unschedulable": cordons.Load() > 0},
					}))
				case "/api/v1/pods":
					close(draining)
					<-r.Context().Done()
				default:
					t.Errorf("unexpected Kubernetes request: %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(kubeAPI.Close)
			model := &talosMachineResourceModel{
				SerializeUpgrades: types.BoolValue(true),
				DrainOnUpgrade:    types.BoolValue(stage == "drain"),
				Image:             types.StringValue("example.invalid/installer:v1.14.1"),
				RebootMode:        types.StringValue("DEFAULT"),
				Kubeconfig: types.StringValue(fmt.Sprintf(`{
"apiVersion":"v1", "kind":"Config",
"clusters":[{"name":"test","cluster":{"server":%q}}],
"contexts":[{"name":"test","context":{"cluster":"test"}}],
"current-context":"test"
}`, kubeAPI.URL)),
			}

			result := make(chan error, 1)
			go func() { result <- talosMachineUpgrade(activeCtx, endpoint, "test-node", nil, model, true) }()

			started := machine.rebooted
			if stage == "drain" {
				started = draining
			}

			select {
			case <-started:
			case err := <-result:
				t.Fatalf("upgrade returned before reaching %s: %v", stage, err)
			case <-ctx.Done():
				t.Fatal("upgrade did not reach cancellation point")
			}

			<-prepared // The active upgrade has finished preparation.

			queued := make(chan error, 1)

			go func() { queued <- talosMachineUpgrade(ctx, endpoint, "queued-node", nil, model, true) }()

			select {
			case <-prepared:
			case <-ctx.Done():
				t.Fatal("concurrent upgrade did not prepare its image")
			}

			cancel()

			err := <-result
			require.Error(t, err, "a canceled active upgrade must not report success")
			require.ErrorContains(t, <-queued, "serialized upgrades stopped after an earlier failure")

			// Use an independent, live context. Otherwise cancellation alone could
			// hide a missing failure latch and make the following upgrade look safe.
			err = talosMachineUpgrade(ctx, endpoint, "next-node", nil, model, true)
			require.ErrorContains(t, err, "serialized upgrades stopped after an earlier failure")

			if stage == "drain" {
				require.EqualValues(t, 1, cordons.Load())
				require.Zero(t, machine.reboots.Load())
			} else {
				require.EqualValues(t, 1, machine.reboots.Load())
			}
		})
	}
}
