// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/cosi-project/runtime/pkg/state"
	"github.com/cosi-project/runtime/pkg/state/impl/inmem"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	k8sres "github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	serviceres "github.com/siderolabs/talos/pkg/machinery/resources/v1alpha1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

const cpTestNode = "cp-1"

var cpTestTiming = talos.ControlPlaneWaitTiming{
	SettleWindow: 50 * time.Millisecond,
	SettleMax:    time.Second,
	PollInterval: 10 * time.Millisecond,
	Timeout:      5 * time.Second,
}

func cpTestAnnotations(configFileVersion string) map[string]string {
	return map[string]string{
		constants.AnnotationStaticPodSecretsVersion:    "1",
		constants.AnnotationStaticPodConfigVersion:     "2",
		constants.AnnotationStaticPodConfigFileVersion: configFileVersion,
	}
}

func cpTestPodStatus(ready bool) corev1.PodStatus {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}

	return corev1.PodStatus{Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}}}
}

func cpTestNodename() *k8sres.Nodename {
	res := k8sres.NewNodename(k8sres.NamespaceName, k8sres.NodenameID)
	res.TypedSpec().Nodename = cpTestNode

	return res
}

func cpTestStaticPod(t *testing.T, configFileVersion string) *k8sres.StaticPod {
	t.Helper()

	pod, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{
		Name:        k8sres.APIServerID,
		Namespace:   metav1.NamespaceSystem,
		Annotations: cpTestAnnotations(configFileVersion),
	}})
	require.NoError(t, err)

	res := k8sres.NewStaticPod(k8sres.NamespaceName, k8sres.APIServerID)
	res.TypedSpec().Pod = pod

	return res
}

func cpTestStaticPodStatus(t *testing.T, ready bool) *k8sres.StaticPodStatus {
	t.Helper()

	status := cpTestPodStatus(ready)

	podStatus, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&status)
	require.NoError(t, err)

	res := k8sres.NewStaticPodStatus(k8sres.NamespaceName, metav1.NamespaceSystem+"/"+k8sres.APIServerID+"-"+cpTestNode)
	res.TypedSpec().PodStatus = podStatus

	return res
}

func cpTestEtcd(healthy bool) *serviceres.Service {
	res := serviceres.NewService("etcd")
	res.TypedSpec().Running = true
	res.TypedSpec().Healthy = healthy

	return res
}

func cpTestMirrorPod(configFileVersion string, ready bool) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        k8sres.APIServerID + "-" + cpTestNode,
			Namespace:   metav1.NamespaceSystem,
			Annotations: cpTestAnnotations(configFileVersion),
		},
		Status: cpTestPodStatus(ready),
	}
}

// cpTestClient builds a Talos client over an in-memory COSI state holding res.
func cpTestClient(t *testing.T, res ...resource.Resource) *client.Client {
	t.Helper()

	st := state.WrapCore(inmem.NewState(k8sres.NamespaceName))

	for _, r := range res {
		require.NoError(t, st.Create(t.Context(), r))
	}

	return &client.Client{COSI: st}
}

func cpTestOp(c *client.Client) func(context.Context, string, string, *clientconfig.Config, func(context.Context, *client.Client) error) error {
	return func(ctx context.Context, _, _ string, _ *clientconfig.Config, fn func(context.Context, *client.Client) error) error {
		return fn(ctx, c)
	}
}

// cpTestRender simulates Talos re-rendering kube-apiserver with a new config file version.
func cpTestRender(ctx context.Context, t *testing.T, c *client.Client, configFileVersion string) {
	if _, err := safe.StateUpdateWithConflicts(ctx, c.COSI, k8sres.NewStaticPod(k8sres.NamespaceName, k8sres.APIServerID).Metadata(), func(r *k8sres.StaticPod) error {
		r.TypedSpec().Pod = cpTestStaticPod(t, configFileVersion).TypedSpec().Pod

		return nil
	}); err != nil && ctx.Err() == nil {
		t.Error(err)
	}
}

func TestTalosMachineControlPlaneNodeName(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		res  func(t *testing.T) []resource.Resource
		name string
		want string
	}{
		{
			name: "worker or maintenance mode: no nodename",
			res:  func(*testing.T) []resource.Resource { return nil },
		},
		{
			name: "before bootstrap: no kube-apiserver status",
			res:  func(*testing.T) []resource.Resource { return []resource.Resource{cpTestNodename()} },
		},
		{
			name: "kube-apiserver not ready",
			res: func(t *testing.T) []resource.Resource {
				return []resource.Resource{cpTestNodename(), cpTestStaticPodStatus(t, false)}
			},
		},
		{
			name: "kube-apiserver ready",
			res: func(t *testing.T) []resource.Resource {
				return []resource.Resource{cpTestNodename(), cpTestStaticPodStatus(t, true)}
			},
			want: cpTestNode,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := cpTestOp(cpTestClient(t, tc.res(t)...))

			got, err := talos.TalosMachineControlPlaneNodeName(t.Context(), "endpoint", "node", nil, op)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// A probe that cannot read the node must fail the apply, not silently skip the wait.
func TestTalosMachineControlPlaneNodeNameError(t *testing.T) {
	t.Parallel()

	op := func(context.Context, string, string, *clientconfig.Config, func(context.Context, *client.Client) error) error {
		return status.Error(codes.PermissionDenied, "not authorized")
	}

	_, err := talos.TalosMachineControlPlaneNodeName(t.Context(), "endpoint", "node", nil, op)
	require.ErrorContains(t, err, "not authorized")
}

func TestTalosMachineStaticPodsSettleWaitsForQuiet(t *testing.T) {
	t.Parallel()

	c := cpTestClient(t, cpTestStaticPod(t, "4"))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cpTestRender(t.Context(), t, c, "5")
	}()

	start := time.Now()

	require.NoError(t, talos.TalosMachineStaticPodsSettle(t.Context(), c, 500*time.Millisecond, 5*time.Second))
	require.GreaterOrEqual(t, time.Since(start), 550*time.Millisecond, "a render must restart the settle window")
}

func TestTalosMachineStaticPodsSettleRespectsCap(t *testing.T) {
	t.Parallel()

	c := cpTestClient(t, cpTestStaticPod(t, "1"))

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	go func() {
		for i := 0; ctx.Err() == nil; i++ {
			cpTestRender(ctx, t, c, strconv.Itoa(i))
			time.Sleep(10 * time.Millisecond)
		}
	}()

	start := time.Now()

	require.NoError(t, talos.TalosMachineStaticPodsSettle(t.Context(), c, 200*time.Millisecond, 500*time.Millisecond))
	require.GreaterOrEqual(t, time.Since(start), 500*time.Millisecond, "renders must keep resetting the settle window")
	require.Less(t, time.Since(start), 2*time.Second)
}

// The incident case: an audit policy change bumps only config-file-version.
// The wait must hold until the mirror pod carries it and is Ready.
func TestTalosMachineWaitForControlPlaneConfigFileChange(t *testing.T) {
	t.Parallel()

	op := cpTestOp(cpTestClient(t, cpTestStaticPod(t, "5"), cpTestEtcd(true)))

	var gets atomic.Int32

	cs := fake.NewClientset()
	cs.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		switch gets.Add(1) {
		case 1:
			return true, nil, errors.New("EOF") // API outage through the LB
		case 2:
			return true, cpTestMirrorPod("4", true), nil // old pod
		case 3:
			return true, cpTestMirrorPod("5", false), nil // new pod starting
		default:
			return true, cpTestMirrorPod("5", true), nil
		}
	})

	require.NoError(t, talos.TalosMachineWaitForControlPlane(t.Context(), "endpoint", "node", nil, op, cs, cpTestNode, cpTestTiming))
	require.Equal(t, int32(4), gets.Load())
}

func TestTalosMachineWaitForControlPlaneUnchanged(t *testing.T) {
	t.Parallel()

	op := cpTestOp(cpTestClient(t, cpTestStaticPod(t, "4"), cpTestEtcd(true)))
	cs := fake.NewClientset(cpTestMirrorPod("4", true))
	start := time.Now()

	require.NoError(t, talos.TalosMachineWaitForControlPlane(t.Context(), "endpoint", "node", nil, op, cs, cpTestNode, cpTestTiming))
	require.Less(t, time.Since(start), time.Second)
}

func TestTalosMachineWaitForControlPlaneTimeout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		mirror        *corev1.Pod
		name          string
		wantErrSubstr string
		etcdHealthy   bool
	}{
		{
			name:          "mirror pod keeps old config",
			mirror:        cpTestMirrorPod("4", true),
			etcdHealthy:   true,
			wantErrSubstr: `config-file-version is "4", want "5"`,
		},
		{
			// The render may still be pending, so a matching mirror pod proves nothing.
			name:          "etcd not healthy",
			mirror:        cpTestMirrorPod("5", true),
			wantErrSubstr: "etcd to be healthy",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			op := cpTestOp(cpTestClient(t, cpTestStaticPod(t, "5"), cpTestEtcd(tc.etcdHealthy)))
			cs := fake.NewClientset(tc.mirror)

			timing := cpTestTiming
			timing.Timeout = 300 * time.Millisecond

			err := talos.TalosMachineWaitForControlPlane(t.Context(), "endpoint", "node", nil, op, cs, cpTestNode, timing)
			require.ErrorContains(t, err, "timed out waiting for control plane static pods")
			require.ErrorContains(t, err, tc.wantErrSubstr)
		})
	}
}

func TestTalosMachineWaitForControlPlaneForbidden(t *testing.T) {
	t.Parallel()

	op := cpTestOp(cpTestClient(t, cpTestStaticPod(t, "5"), cpTestEtcd(true)))

	cs := fake.NewClientset()
	cs.PrependReactor("get", "pods", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewForbidden(corev1.Resource("pods"), "", errors.New("denied"))
	})

	start := time.Now()

	err := talos.TalosMachineWaitForControlPlane(t.Context(), "endpoint", "node", nil, op, cs, cpTestNode, cpTestTiming)
	require.True(t, apierrors.IsForbidden(err), "got %v", err)
	require.Less(t, time.Since(start), time.Second, "a permission error must not wait for the timeout")
}
