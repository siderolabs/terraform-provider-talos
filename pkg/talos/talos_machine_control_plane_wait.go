// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"time"

	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	cosistate "github.com/cosi-project/runtime/pkg/state"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/nodedrain"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	k8sres "github.com/siderolabs/talos/pkg/machinery/resources/k8s"
	serviceres "github.com/siderolabs/talos/pkg/machinery/resources/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/util/podutils"
)

// controlPlaneStaticPodIDs are the Talos StaticPod resources whose restart
// wait_for_control_plane waits for.
var controlPlaneStaticPodIDs = []cosiresource.ID{k8sres.APIServerID, k8sres.ControllerManagerID, k8sres.SchedulerID}

// staticPodVersionAnnotations are bumped by Talos whenever an input of a
// control plane static pod changes: secrets, the component config, or rendered
// files such as the audit policy.
var staticPodVersionAnnotations = []string{
	constants.AnnotationStaticPodSecretsVersion,
	constants.AnnotationStaticPodConfigVersion,
	constants.AnnotationStaticPodConfigFileVersion,
}

// controlPlaneWaitTiming bounds the wait after a config apply. The rendered
// static pods must stay unchanged for SettleWindow (at most SettleMax) before the
// first check; Timeout covers the whole wait.
type controlPlaneWaitTiming struct {
	SettleWindow time.Duration
	SettleMax    time.Duration
	PollInterval time.Duration
	Timeout      time.Duration
}

var defaultControlPlaneWaitTiming = controlPlaneWaitTiming{
	SettleWindow: 5 * time.Second,
	SettleMax:    time.Minute,
	PollInterval: 2 * time.Second,
	Timeout:      10 * time.Minute,
}

// talosMachineControlPlaneNodeName returns the Kubernetes node name when the
// node runs a kube-apiserver that is Ready, and "" otherwise. Workers, nodes in
// maintenance mode or before etcd bootstrap and an apiserver that is already
// down all return "", so the caller skips the wait instead of deadlocking on an
// API server that cannot come up. An unreachable node is retried until ctx
// expires; any other failure is returned rather than disabling the wait.
func talosMachineControlPlaneNodeName(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, op clientOpFunc) (string, error) {
	var nodeName string

	err := talosMachineRetryNodeOp(ctx, endpoint, node, talosConfig, op, func(nodeCtx context.Context, c *client.Client) error {
		var err error

		nodeName, err = talosMachineReadyAPIServerNode(nodeCtx, c)

		return err
	})

	return nodeName, err
}

func talosMachineReadyAPIServerNode(ctx context.Context, c *client.Client) (string, error) {
	name, err := nodedrain.GetKubernetesNodeName(ctx, c)
	if cosistate.IsNotFoundError(err) {
		tflog.Info(ctx, "node has no Kubernetes node name; not waiting for control plane static pods")

		return "", nil
	}

	if err != nil {
		return "", err
	}

	res, err := safe.StateGetByID[*k8sres.StaticPodStatus](ctx, c.COSI, metav1.NamespaceSystem+"/"+k8sres.APIServerID+"-"+name)
	if cosistate.IsNotFoundError(err) {
		tflog.Info(ctx, "node runs no kube-apiserver; not waiting for control plane static pods")

		return "", nil
	}

	if err != nil {
		return "", fmt.Errorf("reading kube-apiserver status: %w", err)
	}

	var status corev1.PodStatus

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.TypedSpec().PodStatus, &status); err != nil {
		return "", fmt.Errorf("decoding kube-apiserver status: %w", err)
	}

	if !podutils.IsPodReady(&corev1.Pod{Status: status}) {
		tflog.Info(ctx, "kube-apiserver is not ready; not waiting for control plane static pods")

		return "", nil
	}

	return name, nil
}

// talosMachineWaitForControlPlane waits until kubelet runs the control plane
// static pods Talos rendered from the applied configuration and they are Ready.
// The target is re-read on every check, so a render that lands after the settle
// window is still waited for. Unchanged pods already match.
func talosMachineWaitForControlPlane(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, op clientOpFunc,
	cs kubernetes.Interface, nodeName string, timing controlPlaneWaitTiming,
) error {
	ctx, cancel := context.WithTimeout(ctx, timing.Timeout)
	defer cancel()

	return op(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		if err := talosMachineStaticPodsSettle(nodeCtx, c, timing.SettleWindow, timing.SettleMax); err != nil {
			return err
		}

		ticker := time.NewTicker(timing.PollInterval)
		defer ticker.Stop()

		for {
			err := talosMachineControlPlaneCurrent(nodeCtx, c, cs, nodeName)
			if err == nil {
				return nil
			}

			// The kubeconfig will not gain permissions while we wait.
			if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
				return err
			}

			select {
			case <-nodeCtx.Done():
				return fmt.Errorf("timed out waiting for control plane static pods to run the applied configuration: %w", err)
			case <-ticker.C:
			}
		}
	})
}

// talosMachineStaticPodsSettle waits until the StaticPod resources have not
// changed for window, at most maxWait. Talos renders them asynchronously after
// ApplyConfiguration returns.
func talosMachineStaticPodsSettle(ctx context.Context, c *client.Client, window, maxWait time.Duration) error {
	watchCtx, cancel := context.WithTimeout(ctx, maxWait)
	defer cancel()

	watchCh := make(chan cosistate.Event)

	if err := c.COSI.WatchKind(watchCtx, cosiresource.NewMetadata(k8sres.NamespaceName, k8sres.StaticPodType, "", cosiresource.VersionUndefined), watchCh); err != nil {
		return fmt.Errorf("watching static pods: %w", err)
	}

	quiet := time.NewTimer(window)
	defer quiet.Stop()

	for {
		select {
		case ev := <-watchCh:
			if ev.Type == cosistate.Errored {
				return fmt.Errorf("watching static pods: %w", ev.Error)
			}

			quiet.Reset(window)
		case <-quiet.C:
			return nil
		case <-watchCtx.Done():
			// Reaching maxWait is not an error: the checks re-read the rendered pods.
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("waiting for control plane static pods to settle: %w", err)
			}

			return nil
		}
	}
}

// talosMachineControlPlaneCurrent checks that every control plane mirror pod
// carries the version annotations of the currently rendered static pod and is Ready.
func talosMachineControlPlaneCurrent(ctx context.Context, c *client.Client, cs kubernetes.Interface, nodeName string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Talos only renders the static pods while etcd is healthy. Until then the
	// StaticPod resources can still hold the previous configuration.
	etcd, err := safe.StateGet[*serviceres.Service](ctx, c.COSI, serviceres.NewService("etcd").Metadata())
	if err != nil {
		return fmt.Errorf("reading etcd service status: %w", err)
	}

	if !etcd.TypedSpec().Healthy {
		return errors.New("waiting for etcd to be healthy")
	}

	pods, err := talosMachineControlPlaneStaticPods(ctx, c)
	if err != nil {
		return err
	}

	for i := range pods {
		if err := staticPodMirrorCurrent(ctx, cs, &pods[i], nodeName); err != nil {
			return err
		}
	}

	return nil
}

func talosMachineControlPlaneStaticPods(ctx context.Context, c *client.Client) ([]metav1.ObjectMeta, error) {
	var pods []metav1.ObjectMeta

	for _, id := range controlPlaneStaticPodIDs {
		res, err := safe.StateGetByID[*k8sres.StaticPod](ctx, c.COSI, id)
		if cosistate.IsNotFoundError(err) {
			// Disabled controller-manager or scheduler: no pod to wait for.
			continue
		}

		if err != nil {
			return nil, fmt.Errorf("reading static pod %s: %w", id, err)
		}

		var pod corev1.Pod

		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(res.TypedSpec().Pod, &pod); err != nil {
			return nil, fmt.Errorf("decoding static pod %s: %w", id, err)
		}

		pods = append(pods, pod.ObjectMeta)
	}

	return pods, nil
}

// staticPodMirrorCurrent checks that the API server's mirror pod carries the
// rendered pod's version annotations and is Ready.
func staticPodMirrorCurrent(ctx context.Context, cs kubernetes.Interface, pod *metav1.ObjectMeta, nodeName string) error {
	mirrorName := pod.Name + "-" + nodeName

	mirror, err := cs.CoreV1().Pods(pod.Namespace).Get(ctx, mirrorName, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("getting mirror pod %s: %w", mirrorName, err)
	}

	for _, k := range staticPodVersionAnnotations {
		want, ok := pod.Annotations[k]
		if !ok {
			continue
		}

		if got := mirror.Annotations[k]; got != want {
			return fmt.Errorf("%s: %s is %q, want %q", mirrorName, k, got, want)
		}
	}

	if !podutils.IsPodReady(mirror) {
		return fmt.Errorf("%s is not ready", mirrorName)
	}

	return nil
}
