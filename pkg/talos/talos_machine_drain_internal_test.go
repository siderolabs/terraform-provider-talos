// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	ktesting "k8s.io/client-go/testing"
)

const drainTestNode = "node1"

// drainTestOptions are the options Create uses: it can run before the cluster is
// healthy, so a missing node is skipped and a not-Ready one is not waited for. Update
// leaves OnCreate off.
func drainTestOptions() drainAroundOptions {
	return drainAroundOptions{
		DrainTimeout: time.Second,
		ReadyTimeout: 300 * time.Millisecond,
		OnCreate:     true,
	}
}

func drainTestNodeObject(ready corev1.ConditionStatus) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: drainTestNode},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: ready}},
		},
	}
}

func nodeUnschedulable(ctx context.Context, t *testing.T, cs *fake.Clientset) bool {
	t.Helper()

	node, err := cs.CoreV1().Nodes().Get(ctx, drainTestNode, metav1.GetOptions{})
	if err != nil {
		t.Fatalf("getting node: %v", err)
	}

	return node.Spec.Unschedulable
}

// TestRunDrained_NotReadyNodeIsNotWaitedFor covers the Create-time upgrade of a node on a
// cluster whose Nodes cannot become Ready yet (no CNI installed).
// talos_machine's Create upgraded the node, which drained fine (only DaemonSet pods were
// there), rebooted fine, and then waited five minutes for the node to report Ready before
// uncordoning. It never does without a CNI, so the apply failed with
//
//	uncordoning node: waiting for node ready: context deadline exceeded
//
// and left the node cordoned. A node that was not Ready before the upgrade is still
// drained, but it must not be waited for: it cannot be expected to come back Ready.
func TestRunDrained_NotReadyNodeIsNotWaitedFor(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(
		drainTestNodeObject(corev1.ConditionFalse),
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "ds", Namespace: "kube-system"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ds-pod", Namespace: "kube-system",
				OwnerReferences: []metav1.OwnerReference{{Kind: "DaemonSet", Name: "ds", APIVersion: "apps/v1", Controller: new(true)}},
			},
			Spec: corev1.PodSpec{NodeName: drainTestNode},
		},
	)

	var cordonedDuringAction bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(actionCtx context.Context) error {
		cordonedDuringAction = nodeUnschedulable(actionCtx, t, cs)

		return nil
	})
	if err != nil {
		t.Fatalf("expected the upgrade to go ahead on a not-Ready node, got: %v", err)
	}

	if !cordonedDuringAction {
		t.Error("a node that exists must be drained, even when it is not Ready")
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a not-Ready node must be uncordoned without waiting for it to become Ready")
	}
}

// TestRunDrained_NotReadyNodeWithWorkloadsIsNotRebootedUndrained makes sure a node that is
// briefly not Ready but still runs pods (a kubelet blip on a live cluster) is not rebooted
// with them still on it: the drain runs, and if it cannot finish the upgrade does not go on.
func TestRunDrained_NotReadyNodeWithWorkloadsIsNotRebootedUndrained(t *testing.T) {
	t.Parallel()

	cs := blockedEvictionClientset(drainTestNodeObject(corev1.ConditionFalse))

	opts := drainTestOptions()
	opts.DrainTimeout = 200 * time.Millisecond

	var ran bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error {
		ran = true

		return nil
	})
	if err == nil {
		t.Fatal("expected the blocked drain to fail")
	}

	if ran {
		t.Error("a not-Ready node that still runs pods must not be rebooted undrained")
	}
}

// TestRunDrained_MissingNodeIsSkipped covers a node that has not registered with the
// Kubernetes API yet, which is the normal state of a machine on its first Create.
func TestRunDrained_MissingNodeIsSkipped(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()

	var ran bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(context.Context) error {
		ran = true

		return nil
	})
	if err != nil {
		t.Fatalf("expected the upgrade to go ahead when the node is not registered, got: %v", err)
	}

	if !ran {
		t.Error("action did not run")
	}
}

// TestRunDrained_ReadyNodeIsDrainedAroundTheAction guards the behavior that must not
// change: a Ready node is cordoned while the action runs and schedulable again after.
func TestRunDrained_ReadyNodeIsDrainedAroundTheAction(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	var cordonedDuringAction bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(actionCtx context.Context) error {
		cordonedDuringAction = nodeUnschedulable(actionCtx, t, cs)

		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cordonedDuringAction {
		t.Error("a Ready node must be cordoned while the action runs")
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a Ready node must be uncordoned after the action")
	}
}

// TestRunDrained_FailedActionStillUncordons guards that a drained node is not left
// cordoned when the action (the reboot) fails.
func TestRunDrained_FailedActionStillUncordons(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))
	actionErr := errors.New("reboot failed")

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(context.Context) error {
		return actionErr
	})
	if !errors.Is(err, actionErr) {
		t.Fatalf("expected the action error, got: %v", err)
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a node must be uncordoned after a failed action")
	}
}

// TestRunDrained_NodeLookupErrorIsNotSkipped makes sure the fix does not turn an API
// error into "nothing to drain": if the node's state cannot be read, the upgrade must
// not go on to reboot a node that may be Ready and carrying workloads. That includes an
// unreachable API server, so a rolling kube-apiserver restart or a wrong kubeconfig
// endpoint fails the apply instead of rebooting the node undrained.
func TestRunDrained_NodeLookupErrorIsNotSkipped(t *testing.T) {
	t.Parallel()

	for name, lookupErr := range map[string]error{
		"forbidden":          apierrors.NewForbidden(corev1.Resource("nodes"), drainTestNode, errors.New("denied")),
		"connection refused": &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED},
		"service unavail":    apierrors.NewServiceUnavailable("no backends"),
		"untrusted cert":     x509.UnknownAuthorityError{},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))
			cs.PrependReactor("get", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, lookupErr
			})

			var ran bool

			err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(context.Context) error {
				ran = true

				return nil
			})
			if err == nil {
				t.Fatal("expected an error when the node state cannot be read")
			}

			if ran {
				t.Error("action must not run when the node state is unknown")
			}
		})
	}
}

// TestRunDrained_UpdateKeepsDrainingNotReadyNodes guards Update, which is not covered by
// the Create-only skip: a transiently not-Ready node still runs pods, so it is still
// cordoned and drained, and the uncordon still waits for it to come back Ready.
func TestRunDrained_UpdateKeepsDrainingNotReadyNodes(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionFalse))

	opts := drainTestOptions()
	opts.OnCreate = false

	var cordonedDuringAction bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(actionCtx context.Context) error {
		cordonedDuringAction = nodeUnschedulable(actionCtx, t, cs)

		return nil
	})

	if !cordonedDuringAction {
		t.Error("on Update a not-Ready node must still be cordoned and drained")
	}

	if err == nil || !strings.Contains(err.Error(), "waiting for node ready") {
		t.Errorf("on Update the uncordon must still wait for the node to be Ready, got: %v", err)
	}

	// After a real upgrade, a node that did not come back Ready is exactly when the cordon is
	// a safety net: the not-ready taint does not cover a node that flaps Ready on a bad
	// upgrade. It stays cordoned, and the error says how to undo that once it is healthy.
	if !nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a node that did not come back Ready after the upgrade must stay cordoned")
	}

	if !strings.Contains(err.Error(), "kubectl uncordon "+drainTestNode) {
		t.Errorf("the error should say how to uncordon the node, got: %v", err)
	}
}

// TestRunDrained_UpdateFailsOnMissingNode guards Update: a node name that does not match
// a registered Node (for example a hostname vs FQDN mismatch) is an error, not a silent
// skip of the drain.
func TestRunDrained_UpdateFailsOnMissingNode(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()

	opts := drainTestOptions()
	opts.OnCreate = false

	var ran bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error {
		ran = true

		return nil
	})
	if err == nil {
		t.Fatal("expected an error for a node that does not exist")
	}

	if ran {
		t.Error("action must not run when the drain failed")
	}

	if strings.Contains(err.Error(), "kubectl uncordon") {
		t.Errorf("a node that does not exist was never cordoned, so the error must not tell the user to uncordon it: %v", err)
	}
}

// blockedEvictionClientset returns a clientset whose node has a pod that can never be
// evicted (a PodDisruptionBudget that allows no disruptions), so a drain of it fails.
func blockedEvictionClientset(node *corev1.Node) *fake.Clientset {
	cs := fake.NewSimpleClientset(
		node,
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: drainTestNode},
		},
	)

	// Advertise eviction support so the drain helper evicts instead of deleting, which
	// would ignore PodDisruptionBudgets and succeed.
	cs.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods/eviction", Kind: "Eviction", Group: "policy", Version: "v1"},
			},
		},
	}

	cs.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}

		return true, nil, apierrors.NewTooManyRequests("blocked by PodDisruptionBudget", 1)
	})

	return cs
}

// TestRunDrained_NotReadyNodeUncordonSurvivesAPIRestart covers a node that was not Ready
// before the upgrade, so its uncordon does not wait for Ready. The reboot returns before
// the node's own kube-apiserver is back, so the first uncordon can find the API server
// unreachable. A short outage is waited out instead of failing the apply and leaving the
// node cordoned.
func TestRunDrained_NotReadyNodeUncordonSurvivesAPIRestart(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionFalse))

	var patches int

	cs.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		patches++

		// The first patch is the cordon; the next two, made by the uncordon, hit the
		// API server while it is still restarting.
		if patches == 2 || patches == 3 {
			return true, nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}

		return false, nil, nil
	})

	opts := drainTestOptions()
	opts.RetryInterval = 10 * time.Millisecond

	if err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("expected the uncordon to wait out a short API server outage, got: %v", err)
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("the node must be uncordoned once the API server is back")
	}
}

// TestRunDrained_ReadyNodeOnCreateIsStillWaitedFor guards the other half of the Create
// rule: only a node that was not Ready beforehand skips the wait. A node that was Ready
// and does not come back Ready after the reboot is an error, as on Update.
func TestRunDrained_ReadyNodeOnCreateIsStillWaitedFor(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(actionCtx context.Context) error {
		// The reboot leaves the node not Ready.
		node, getErr := cs.CoreV1().Nodes().Get(actionCtx, drainTestNode, metav1.GetOptions{})
		if getErr != nil {
			return getErr
		}

		node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}
		_, updateErr := cs.CoreV1().Nodes().Update(actionCtx, node, metav1.UpdateOptions{})

		return updateErr
	})
	if err == nil || !strings.Contains(err.Error(), "waiting for node ready") {
		t.Errorf("a node that was Ready must be waited for after the upgrade on Create too, got: %v", err)
	}
}

// TestRunDrained_NotFoundFromSomethingElseIsNotAMissingNode makes sure only the API server
// saying that this node does not exist counts as "not registered yet". A 404 from anything
// else in the path, such as an ingress or load balancer without a route, says nothing about
// the node, so it must not turn into a reboot without a drain on a cluster that is running.
func TestRunDrained_NotFoundFromSomethingElseIsNotAMissingNode(t *testing.T) {
	t.Parallel()

	for name, lookupErr := range map[string]error{
		"404 without details":         &apierrors.StatusError{ErrStatus: metav1.Status{Code: http.StatusNotFound, Reason: metav1.StatusReasonNotFound}},
		"404 about another resource":  apierrors.NewNotFound(corev1.Resource("pods"), drainTestNode),
		"404 about another node name": apierrors.NewNotFound(corev1.Resource("nodes"), "other"),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))
			cs.PrependReactor("get", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
				return true, nil, lookupErr
			})

			var ran bool

			err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(context.Context) error {
				ran = true

				return nil
			})
			if err == nil {
				t.Fatal("expected an error when the 404 does not come from the API server about this node")
			}

			if ran {
				t.Error("the node must not be rebooted undrained because of a 404 from something else")
			}
		})
	}
}

// TestUncordonMayPass pins which uncordon errors are worth retrying: only the ones that
// may pass. A permanent refusal or a bad request must fail at once, not after minutes.
func TestUncordonMayPass(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct {
		err  error
		want bool
	}{
		"connection refused": {&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, true},
		"connection reset":   {&net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}, true},
		"server timeout":     {apierrors.NewServerTimeout(corev1.Resource("nodes"), "patch", 1), true},
		"503":                {apierrors.NewServiceUnavailable("no backends"), true},
		"too many requests":  {apierrors.NewTooManyRequests("slow down", 1), true},
		"forbidden":          {apierrors.NewForbidden(corev1.Resource("nodes"), "n", errors.New("denied")), false},
		"invalid":            {apierrors.NewBadRequest("nope"), false},
		"not found":          {apierrors.NewNotFound(corev1.Resource("nodes"), "n"), false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := uncordonMayPass(tc.err); got != tc.want {
				t.Errorf("uncordonMayPass(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

// TestRunDrained_FailedDrainUncordonsTheNode covers
// https://github.com/siderolabs/terraform-provider-talos/issues/422: a drain that fails after
// the node was cordoned (here: eviction blocked by a PodDisruptionBudget until the drain
// timeout) must not leave the node unschedulable, and the upgrade must not go on.
func TestRunDrained_FailedDrainUncordonsTheNode(t *testing.T) {
	t.Parallel()

	cs := blockedEvictionClientset(drainTestNodeObject(corev1.ConditionTrue))

	opts := drainTestOptions()
	opts.DrainTimeout = 200 * time.Millisecond

	var ran bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error {
		ran = true

		return nil
	})
	if err == nil {
		t.Fatal("expected the blocked drain to fail")
	}

	if ran {
		t.Error("the action must not run when the drain failed")
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a node whose drain failed must not be left cordoned")
	}
}

// TestRunDrained_ExpiredContextStillUncordons covers the likeliest way a drain fails: the
// operation's own timeout (or Ctrl-C) fires while pods are still being evicted. The
// cleanup must not run on that expired context, or it fails the same way and leaves the
// node cordoned.
func TestRunDrained_ExpiredContextStillUncordons(t *testing.T) {
	t.Parallel()

	cs := blockedEvictionClientset(drainTestNodeObject(corev1.ConditionTrue))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	opts := drainTestOptions()
	opts.DrainTimeout = time.Minute // far longer than ctx, so ctx is what stops the drain

	if err := talosMachineRunDrained(ctx, cs, drainTestNode, opts, func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected the drain to fail when the context expires")
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a node must be uncordoned even when the operation's context has expired")
	}
}

// TestRunDrained_UncordonFailureSaysHowToRecover makes the failure actionable: if the
// node cannot be uncordoned, the error tells the user it is still cordoned.
func TestRunDrained_UncordonFailureSaysHowToRecover(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	var patches int

	cs.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		patches++

		// The first patch is the cordon; every later one is the uncordon.
		if patches > 1 {
			return true, nil, errors.New("api server went away")
		}

		return false, nil, nil
	})

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, drainTestOptions(), func(context.Context) error { return nil })
	if err == nil {
		t.Fatal("expected an error when the node cannot be uncordoned")
	}

	if !strings.Contains(err.Error(), "kubectl uncordon "+drainTestNode) {
		t.Errorf("error should tell the user how to uncordon the node, got: %v", err)
	}
}

// notReadyAfterActionCancel returns an action that ends the operation (its timeout or
// Ctrl-C) and leaves the node not Ready, as a reboot that never finished would.
func notReadyAfterActionCancel(cs *fake.Clientset, cancel context.CancelFunc) func(context.Context) error {
	return func(actionCtx context.Context) error {
		node, err := cs.CoreV1().Nodes().Get(actionCtx, drainTestNode, metav1.GetOptions{})
		if err != nil {
			return err
		}

		node.Status.Conditions = []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionFalse}}

		if _, err := cs.CoreV1().Nodes().Update(actionCtx, node, metav1.UpdateOptions{}); err != nil {
			return err
		}

		cancel()

		return context.Canceled
	}
}

// TestRunDrained_EndedOperationWithNotReadyNodeLeavesItCordoned covers the operation ending
// while the reboot is still in progress and the node not Ready. Its health is unknown, which
// is when the cordon is a safety net, so it is not uncordoned: the error says how to do it
// once the node is healthy.
func TestRunDrained_EndedOperationWithNotReadyNodeLeavesItCordoned(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := talosMachineRunDrained(ctx, cs, drainTestNode, drainTestOptions(), notReadyAfterActionCancel(cs, cancel))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected the action error, got: %v", err)
	}

	if !nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a node that is not Ready when the operation ends must stay cordoned")
	}

	if !strings.Contains(err.Error(), "kubectl uncordon "+drainTestNode) {
		t.Errorf("the error should say how to uncordon the node, got: %v", err)
	}
}

// ctxAwareClientset is a clientset that, like a real one, fails a node read whose context has
// ended. The fake one ignores contexts, which would let a canceled operation read Ready.
type ctxAwareClientset struct {
	*fake.Clientset
}

func (c ctxAwareClientset) CoreV1() corev1client.CoreV1Interface {
	return ctxAwareCoreV1{c.Clientset.CoreV1()}
}

type ctxAwareCoreV1 struct {
	corev1client.CoreV1Interface
}

func (c ctxAwareCoreV1) Nodes() corev1client.NodeInterface {
	return ctxAwareNodes{c.CoreV1Interface.Nodes()}
}

type ctxAwareNodes struct {
	corev1client.NodeInterface
}

func (n ctxAwareNodes) Get(ctx context.Context, name string, opts metav1.GetOptions) (*corev1.Node, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	return n.NodeInterface.Get(ctx, name, opts)
}

// TestRunDrained_EndedOperationBeforeReadyIsConfirmedLeavesNodeCordoned covers the operation
// ending while the node is rebooting. A node that reads Ready then proves nothing: its Ready
// condition stays True for a while after the kubelet stops reporting, so it may only be
// the old one. The node stays cordoned and the error says how to undo it.
func TestRunDrained_EndedOperationBeforeReadyIsConfirmedLeavesNodeCordoned(t *testing.T) {
	t.Parallel()

	fakeCS := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))
	cs := ctxAwareClientset{fakeCS}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := talosMachineRunDrained(ctx, cs, drainTestNode, drainTestOptions(), func(context.Context) error {
		cancel()

		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "kubectl uncordon "+drainTestNode) {
		t.Fatalf("expected an error that says how to uncordon the node, got: %v", err)
	}

	if !nodeUnschedulable(context.Background(), t, fakeCS) {
		t.Error("a node must stay cordoned when the operation ended before Ready was confirmed")
	}
}

// TestRunDrained_ReadyWaitHonorsCancellation makes sure the wait for Ready after the
// upgrade stops when the operation is canceled, instead of running on for the whole
// ReadyTimeout on a detached context.
func TestRunDrained_ReadyWaitHonorsCancellation(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	opts := drainTestOptions()
	opts.ReadyTimeout = 10 * time.Second

	start := time.Now()

	action := notReadyAfterActionCancel(cs, func() {
		// The operation is canceled shortly after the reboot, while the wait is running.
		time.AfterFunc(100*time.Millisecond, cancel)
	})

	err := talosMachineRunDrained(ctx, cs, drainTestNode, opts, func(actionCtx context.Context) error {
		// Leaves the node not Ready and schedules the cancel; it returns the cancellation
		// error only to mimic an interrupted reboot, which is not what is under test.
		if actionErr := action(actionCtx); actionErr != nil && !errors.Is(actionErr, context.Canceled) {
			return actionErr
		}

		return nil
	})
	if err == nil {
		t.Error("expected an error: the node never came back Ready")
	}

	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the wait for Ready ran on for %s after the operation was canceled", elapsed)
	}
}

// TestRunDrained_FailedDrainRestoreIsBoundedAgainstADeadAPIServer makes sure the restore after a
// failed drain gives up after FailedDrainRestoreTimeout: if the API server is down the drain
// failed because of that, and the error must not be delayed for long.
func TestRunDrained_FailedDrainRestoreIsBoundedAgainstADeadAPIServer(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))
	cs.PrependReactor("*", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	})

	opts := drainTestOptions()
	opts.OnCreate = false
	opts.RetryInterval = 10 * time.Millisecond
	opts.FailedDrainRestoreTimeout = 300 * time.Millisecond

	start := time.Now()

	if err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected an error when the API server is unreachable")
	}

	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("the failed drain took %s: the uncordon must not be retried", elapsed)
	}
}

// TestRunDrained_JoinedNodeMissingFromKubernetesFails covers a node that Talos knows has
// joined the cluster, but whose name is not found in Kubernetes, for example because it
// registered under a different name than Talos reports. It is not a node that has yet to
// join, so it must not be rebooted without a drain: Update fails in the same case.
func TestRunDrained_JoinedNodeMissingFromKubernetesFails(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()

	opts := drainTestOptions()
	opts.NodeMustExist = true
	opts.NodeAppearTimeout = 200 * time.Millisecond
	opts.RetryInterval = 20 * time.Millisecond

	var ran bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error {
		ran = true

		return nil
	})
	if err == nil {
		t.Fatal("expected an error for a joined node that Kubernetes does not know")
	}

	if ran {
		t.Error("a node that has joined the cluster must not be rebooted undrained because its Kubernetes node was not found")
	}
}

// TestRunDrained_JoinedNodeThatRegistersShortlyAfterIsDrained covers the moment between the
// kubelet getting its client certificate and its Node showing up in Kubernetes: a node that
// has joined but is not found yet is waited for briefly instead of failing the apply.
func TestRunDrained_JoinedNodeThatRegistersShortlyAfterIsDrained(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset()

	time.AfterFunc(100*time.Millisecond, func() {
		_, _ = cs.CoreV1().Nodes().Create(context.Background(), drainTestNodeObject(corev1.ConditionTrue), metav1.CreateOptions{}) //nolint:errcheck
	})

	opts := drainTestOptions()
	opts.NodeMustExist = true
	opts.NodeAppearTimeout = 5 * time.Second
	opts.RetryInterval = 20 * time.Millisecond

	var cordonedDuringAction bool

	err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(actionCtx context.Context) error {
		cordonedDuringAction = nodeUnschedulable(actionCtx, t, cs)

		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !cordonedDuringAction {
		t.Error("the node must be drained once it shows up")
	}
}

// TestRunDrained_DeadlineDuringFinalUncordonStillUncordons covers the operation ending while
// the final uncordon runs, after the node was confirmed Ready. A healthy, upgraded node must
// not be left cordoned only because the deadline landed on that last call.
func TestRunDrained_DeadlineDuringFinalUncordonStillUncordons(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var patches int

	cs.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		patches++

		// The first patch is the cordon. The first uncordon patch is where the operation ends.
		if patches == 2 {
			cancel()

			return true, nil, apierrors.NewServerTimeout(corev1.Resource("nodes"), "patch", 1)
		}

		return false, nil, nil
	})

	opts := drainTestOptions()
	opts.RetryInterval = 10 * time.Millisecond

	if err := talosMachineRunDrained(ctx, cs, drainTestNode, opts, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a node confirmed Ready must be uncordoned even when the operation ends during the uncordon")
	}
}

// TestRunDrained_FailedDrainUncordonSurvivesABlip covers the failure #422 was reported with:
// the drain dies on a momentary API error, and the uncordon that follows hits the same blip.
// It is retried briefly instead of leaving the node cordoned.
func TestRunDrained_FailedDrainUncordonSurvivesABlip(t *testing.T) {
	t.Parallel()

	cs := blockedEvictionClientset(drainTestNodeObject(corev1.ConditionTrue))

	var patches int

	cs.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		patches++

		// The first patch is the cordon; the first uncordon attempt then meets the blip.
		if patches == 2 {
			return true, nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}

		return false, nil, nil
	})

	opts := drainTestOptions()
	opts.DrainTimeout = 200 * time.Millisecond
	opts.RetryInterval = 10 * time.Millisecond

	if err := talosMachineRunDrained(context.Background(), cs, drainTestNode, opts, func(context.Context) error { return nil }); err == nil {
		t.Fatal("expected the blocked drain to fail")
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("a momentary API error must not leave the node cordoned after a failed drain")
	}
}

// TestRunDrained_LastUncordonAttemptRetries covers the operation ending while the uncordon
// rides out a blip, and the one more attempt made on a context that survives it meeting
// another blip: it is retried too.
func TestRunDrained_LastUncordonAttemptRetries(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(drainTestNodeObject(corev1.ConditionTrue))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var patches int

	cs.PrependReactor("patch", "nodes", func(ktesting.Action) (bool, runtime.Object, error) {
		patches++

		switch patches {
		case 2: // the first uncordon: the operation ends here
			cancel()

			return true, nil, apierrors.NewServerTimeout(corev1.Resource("nodes"), "patch", 1)
		case 3: // the last attempt meets another blip
			return true, nil, &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
		}

		return false, nil, nil
	})

	opts := drainTestOptions()
	opts.RetryInterval = 10 * time.Millisecond

	if err := talosMachineRunDrained(ctx, cs, drainTestNode, opts, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if nodeUnschedulable(context.Background(), t, cs) {
		t.Error("the last uncordon attempt must be retried")
	}
}
