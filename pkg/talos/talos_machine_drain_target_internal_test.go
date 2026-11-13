// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"io"
	"testing"

	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	k8sres "github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// nodenameClient returns a client backed by an in-memory COSI state that holds a node name.
// An empty nodename leaves the resource out. skipRegistration sets skipNodeRegistration.
func nodenameClient(t *testing.T, nodename string, skipRegistration bool) *client.Client {
	t.Helper()

	st := state.WrapCore(inmem.NewState(k8sres.NamespaceName))

	if nodename != "" {
		nn := k8sres.NewNodename(k8sres.NamespaceName, k8sres.NodenameID)
		nn.TypedSpec().Nodename = nodename
		nn.TypedSpec().SkipNodeRegistration = skipRegistration
		require.NoError(t, st.Create(t.Context(), nn))
	}

	return &client.Client{COSI: st}
}

// TestDrainNodeName covers the Talos-side lookup of the Kubernetes node name. A node whose
// machine config sets skipNodeRegistration never gets a Kubernetes node, and Talos itself
// skips the drain for it, so there is nothing to drain. Every other failure to resolve the
// name is an error, and an empty name is never used as a signal to skip.
func TestDrainNodeName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		client     *client.Client
		name       string
		want       string
		wantNoNode bool
		wantErr    bool
	}{
		{nodenameClient(t, "w1", false), "name present", "w1", false, false},
		{nodenameClient(t, "w1", true), "skipNodeRegistration", "", true, false},
		{nodenameClient(t, "", false), "missing", "", false, true},
		{nodenameClient(t, " ", false), "blank name", "", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, noNode, err := talosMachineDrainNodeName(t.Context(), tc.client)
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantNoNode, noNode)
		})
	}
}

// listClient is a machine API client whose List answers from a fixed set of entries.
type listClient struct {
	machineapi.MachineServiceClient
	listErr   error
	streamErr error
	files     []*machineapi.FileInfo
}

func (c listClient) List(_ context.Context, _ *machineapi.ListRequest, _ ...grpc.CallOption) (grpc.ServerStreamingClient[machineapi.FileInfo], error) {
	if c.listErr != nil {
		return nil, c.listErr
	}

	return &fileStream{files: c.files, err: c.streamErr}, nil
}

type fileStream struct {
	grpc.ClientStream
	err   error
	files []*machineapi.FileInfo
}

func (s *fileStream) Recv() (*machineapi.FileInfo, error) {
	if len(s.files) > 0 {
		f := s.files[0]
		s.files = s.files[1:]

		return f, nil
	}

	if s.err != nil {
		return nil, s.err
	}

	return nil, io.EOF
}

func fileEntry(rel string) *machineapi.FileInfo {
	return &machineapi.FileInfo{Name: "/var/lib/kubelet/pki/" + rel, RelativeName: rel}
}

// TestKubeletJoined pins how a node is told apart from one that never joined the cluster. It
// is the check Talos itself makes before it drains: the kubelet's client certificate only
// exists once the node has joined. Anything that cannot be told is an error, never "not
// joined", so a node that may be carrying workloads is not rebooted undrained because of it.
func TestKubeletJoined(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		client     listClient
		wantJoined bool
		wantErr    bool
	}{
		{
			name:       "client certificate present",
			client:     listClient{files: []*machineapi.FileInfo{fileEntry("."), fileEntry("kubelet-client-2026-11-16-10-00-00.pem"), fileEntry("kubelet-client-current.pem")}},
			wantJoined: true,
		},
		{
			name:   "pki directory without the client certificate",
			client: listClient{files: []*machineapi.FileInfo{fileEntry("."), fileEntry("kubelet.crt"), fileEntry("kubelet.key")}},
		},
		{
			name: "pki directory does not exist",
			client: listClient{files: []*machineapi.FileInfo{
				{Name: "/var/lib/kubelet/pki", Error: "lstat /var/lib/kubelet/pki: no such file or directory"},
			}},
		},
		{
			// What a real node answers: the whole call fails, nothing is streamed.
			name:   "pki directory does not exist, reported by the call",
			client: listClient{listErr: status.Error(codes.Unknown, "lstat /var/lib/kubelet/pki: no such file or directory")},
		},
		{
			name:   "pki directory does not exist, reported by the stream",
			client: listClient{streamErr: status.Error(codes.Unknown, "lstat /var/lib/kubelet/pki: no such file or directory")},
		},
		{
			name:    "another path is missing",
			client:  listClient{listErr: status.Error(codes.Unknown, "lstat /var/lib/etcd: no such file or directory")},
			wantErr: true,
		},
		{
			name:    "pki directory cannot be read",
			client:  listClient{files: []*machineapi.FileInfo{{Name: "/var/lib/kubelet/pki", Error: "open /var/lib/kubelet/pki: permission denied"}}},
			wantErr: true,
		},
		{
			name:    "list is not allowed for this role",
			client:  listClient{listErr: status.Error(codes.PermissionDenied, "not authorized")},
			wantErr: true,
		},
		{
			name:    "stream fails half way",
			client:  listClient{files: []*machineapi.FileInfo{fileEntry(".")}, streamErr: status.Error(codes.Unavailable, "gone")},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			joined, err := talosMachineKubeletJoined(t.Context(), &client.Client{MachineClient: tc.client})
			if tc.wantErr {
				require.Error(t, err)

				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantJoined, joined)
		})
	}
}
