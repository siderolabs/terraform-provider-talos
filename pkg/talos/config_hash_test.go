// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// TestK8sManagedConfigHash_IgnoresK8sImageBumps verifies that bumping any of
// the five Kubernetes component image tags does not change the hash. These
// fields are managed by upgrade-k8s (driven by talos_cluster), so they must
// not appear in talos_machine's drift detection or trigger a re-apply.
func TestK8sManagedConfigHash_IgnoresK8sImageBumps(t *testing.T) {
	t.Parallel()

	v135 := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
  controllerManager:
    image: registry.k8s.io/kube-controller-manager:v1.35.4
  scheduler:
    image: registry.k8s.io/kube-scheduler:v1.35.4
  proxy:
    image: registry.k8s.io/kube-proxy:v1.35.4
`)

	v136 := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.36.0
cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.36.0
  controllerManager:
    image: registry.k8s.io/kube-controller-manager:v1.36.0
  scheduler:
    image: registry.k8s.io/kube-scheduler:v1.36.0
  proxy:
    image: registry.k8s.io/kube-proxy:v1.36.0
`)

	if got, want := talos.K8sManagedConfigHash(v135), talos.K8sManagedConfigHash(v136); got != want {
		t.Fatalf("hash changed when only K8s image tags were bumped\n  v1.35.4: %s\n  v1.36.0: %s", got, want)
	}
}

// TestK8sManagedConfigHash_IgnoresCustomRegistryImages verifies that the
// normalization removes the image field regardless of registry. upgrade-k8s
// patches whatever value is on the node; private-registry mirrors still need
// to be set via talos_cluster (or talosctl upgrade-k8s) — not via talos_machine.
func TestK8sManagedConfigHash_IgnoresCustomRegistryImages(t *testing.T) {
	t.Parallel()

	stock := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
  controllerManager:
    image: registry.k8s.io/kube-controller-manager:v1.35.4
`)

	mirror := []byte(`cluster:
  apiServer:
    image: mirror.corp.example/kube-apiserver:v1.35.4
  controllerManager:
    image: mirror.corp.example/kube-controller-manager:v1.35.4
`)

	if got, want := talos.K8sManagedConfigHash(stock), talos.K8sManagedConfigHash(mirror); got != want {
		t.Fatalf("hash differed for stock vs custom-registry K8s images\n  stock:  %s\n  mirror: %s", got, want)
	}
}

// TestK8sManagedConfigHash_StructuralChangeChangesHash verifies that non-image
// changes (e.g. adding a kernel module) DO change the hash, so structural drift
// is still detected and re-applied normally.
func TestK8sManagedConfigHash_StructuralChangeChangesHash(t *testing.T) {
	t.Parallel()

	before := []byte(`machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
`)

	after := []byte(`machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
  kernel:
    modules:
      - name: br_netfilter
`)

	if talos.K8sManagedConfigHash(before) == talos.K8sManagedConfigHash(after) {
		t.Fatal("hash unchanged for a real structural change — structural drift would go undetected")
	}
}

// TestK8sManagedConfigHash_NonImageClusterChangeChangesHash verifies that
// changes to other cluster.apiServer fields (not just image) still trigger
// drift detection. We only ignore the image fields, nothing else.
func TestK8sManagedConfigHash_NonImageClusterChangeChangesHash(t *testing.T) {
	t.Parallel()

	before := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
    extraArgs:
      feature-gates: ""
`)

	after := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
    extraArgs:
      feature-gates: "InTreePluginAWSUnregister=true"
`)

	if talos.K8sManagedConfigHash(before) == talos.K8sManagedConfigHash(after) {
		t.Fatal("hash unchanged for an apiServer.extraArgs change — non-image cluster drift would go undetected")
	}
}
