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

	got, gotStripped := talos.K8sManagedConfigHash(v135)
	want, wantStripped := talos.K8sManagedConfigHash(v136)

	if !gotStripped || !wantStripped {
		t.Fatal("K8sManagedConfigHash: stripping failed on valid YAML")
	}

	if got != want {
		t.Fatalf("hash changed when only K8s image tags were bumped\n  v1.35.4: %s\n  v1.36.0: %s", got, want)
	}
}

// TestK8sManagedConfigHash_IgnoresTagOnlyBump verifies that bumping only the
// image tag (same registry, same image name) does not change the hash. This is
// the upgrade-k8s case: it writes a new version tag but does not change the
// registry that the user configured.
func TestK8sManagedConfigHash_IgnoresTagOnlyBump(t *testing.T) {
	t.Parallel()

	v135 := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
  controllerManager:
    image: registry.k8s.io/kube-controller-manager:v1.35.4
`)

	v136 := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.36.0
  controllerManager:
    image: registry.k8s.io/kube-controller-manager:v1.36.0
`)

	got, gotStripped := talos.K8sManagedConfigHash(v135)
	want, wantStripped := talos.K8sManagedConfigHash(v136)

	if !gotStripped || !wantStripped {
		t.Fatal("K8sManagedConfigHash: stripping failed on valid YAML")
	}

	if got != want {
		t.Fatalf("hash changed for tag-only bump on same registry\n  v1.35.4: %s\n  v1.36.0: %s", got, want)
	}
}

// TestK8sManagedConfigHash_DetectsRegistryChange verifies that switching from
// one registry to another (e.g. stock to a private mirror) produces a different
// hash. Only the tag is stripped; the registry and image name remain in the
// hash so user-driven registry changes are still detected as drift.
func TestK8sManagedConfigHash_DetectsRegistryChange(t *testing.T) {
	t.Parallel()

	stock := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
`)

	mirror := []byte(`cluster:
  apiServer:
    image: mirror.corp.example/kube-apiserver:v1.35.4
`)

	hashStock, stockStripped := talos.K8sManagedConfigHash(stock)
	hashMirror, mirrorStripped := talos.K8sManagedConfigHash(mirror)

	if !stockStripped || !mirrorStripped {
		t.Fatal("K8sManagedConfigHash: stripping failed on valid YAML")
	}

	if hashStock == hashMirror {
		t.Fatalf("hash identical for different registries — registry change would go undetected\n  stock:  %s\n  mirror: %s", hashStock, hashMirror)
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

	hashBefore, _ := talos.K8sManagedConfigHash(before)
	hashAfter, _ := talos.K8sManagedConfigHash(after)

	if hashBefore == hashAfter {
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

	hashBefore, _ := talos.K8sManagedConfigHash(before)
	hashAfter, _ := talos.K8sManagedConfigHash(after)

	if hashBefore == hashAfter {
		t.Fatal("hash unchanged for an apiServer.extraArgs change — non-image cluster drift would go undetected")
	}
}

// TestK8sManagedConfigHash_MultidocIgnoresK8sImageBumps verifies that bumping
// the image field in KubeControllerManagerConfig and KubeSchedulerConfig
// documents (Talos 1.14+ multi-doc format) does not change the hash. Those
// image fields are patched by upgrade-k8s and must not trigger a re-apply.
func TestK8sManagedConfigHash_MultidocIgnoresK8sImageBumps(t *testing.T) {
	t.Parallel()

	v135 := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
  proxy:
    image: registry.k8s.io/kube-proxy:v1.35.4
---
kind: KubeControllerManagerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-controller-manager:v1.35.4
---
kind: KubeSchedulerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-scheduler:v1.35.4
`)

	v136 := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.36.0
cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.36.0
  proxy:
    image: registry.k8s.io/kube-proxy:v1.36.0
---
kind: KubeControllerManagerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-controller-manager:v1.36.0
---
kind: KubeSchedulerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-scheduler:v1.36.0
`)

	got, _ := talos.K8sManagedConfigHash(v135)
	want, _ := talos.K8sManagedConfigHash(v136)

	if got != want {
		t.Fatalf("hash changed when only K8s image tags were bumped in multi-doc config\n  v1.35.4: %s\n  v1.36.0: %s", got, want)
	}
}

// TestK8sManagedConfigHash_KubeletConfigDocIgnoresImageBumps verifies that a
// kubelet image bump in a KubeletConfig document does not change the hash.
//
// Talos 1.14.0-beta.0 moved the kubelet image out of .machine.kubelet.image
// into its own KubeletConfig document, which is what upgrade-k8s now patches
// (see stdpatches.WithKubeletImage). Missing this kind here would make every
// kubernetes_version bump look like drift and race upgrade-k8s.
func TestK8sManagedConfigHash_KubeletConfigDocIgnoresImageBumps(t *testing.T) {
	t.Parallel()

	v135 := []byte(`version: v1alpha1
machine:
  network:
    hostname: cp-1
---
kind: KubeletConfig
apiVersion: v1alpha1
image: ghcr.io/siderolabs/kubelet:v1.35.4
`)

	v136 := []byte(`version: v1alpha1
machine:
  network:
    hostname: cp-1
---
kind: KubeletConfig
apiVersion: v1alpha1
image: ghcr.io/siderolabs/kubelet:v1.36.0
`)

	got, _ := talos.K8sManagedConfigHash(v135)
	want, _ := talos.K8sManagedConfigHash(v136)

	if got != want {
		t.Fatalf("hash changed when only the KubeletConfig image tag was bumped\n  v1.35.4: %s\n  v1.36.0: %s", got, want)
	}
}

// TestK8sManagedConfigHash_MultidocStructuralChangeChangesHash verifies that
// non-image changes in KubeControllerManagerConfig or KubeSchedulerConfig
// documents DO change the hash. Those documents have user-exposed fields
// (extraArgs, env, resources, enabled, config) that talos_machine must track.
func TestK8sManagedConfigHash_MultidocStructuralChangeChangesHash(t *testing.T) {
	t.Parallel()

	before := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
---
kind: KubeControllerManagerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-controller-manager:v1.35.4
`)

	after := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
---
kind: KubeControllerManagerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-controller-manager:v1.35.4
extraArgs:
  feature-gates: AllBeta=true
`)

	hashBefore, _ := talos.K8sManagedConfigHash(before)
	hashAfter, _ := talos.K8sManagedConfigHash(after)

	if hashBefore == hashAfter {
		t.Fatal("hash unchanged for an extraArgs change in KubeControllerManagerConfig — structural drift in multi-doc configs would go undetected")
	}
}

// TestK8sManagedConfigHash_Talos114FullMultidoc verifies the full Talos 1.14+
// multidoc layout where ALL four kubernetes component images (apiserver,
// controller-manager, scheduler, proxy) are in separate documents and kubelet
// stays inline in v1alpha1. Bumping all five image tags must not change the hash.
func TestK8sManagedConfigHash_Talos114FullMultidoc(t *testing.T) {
	t.Parallel()

	v135 := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.35.4
---
kind: KubeAPIServerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-apiserver:v1.35.4
---
kind: KubeControllerManagerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-controller-manager:v1.35.4
---
kind: KubeSchedulerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-scheduler:v1.35.4
---
kind: KubeProxyConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-proxy:v1.35.4
`)

	v136 := []byte(`version: v1alpha1
machine:
  kubelet:
    image: ghcr.io/siderolabs/kubelet:v1.36.0
---
kind: KubeAPIServerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-apiserver:v1.36.0
---
kind: KubeControllerManagerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-controller-manager:v1.36.0
---
kind: KubeSchedulerConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-scheduler:v1.36.0
---
kind: KubeProxyConfig
apiVersion: v1alpha1
image: registry.k8s.io/kube-proxy:v1.36.0
`)

	got, gotStripped := talos.K8sManagedConfigHash(v135)
	want, wantStripped := talos.K8sManagedConfigHash(v136)

	if !gotStripped || !wantStripped {
		t.Fatal("K8sManagedConfigHash: stripping failed on valid YAML")
	}

	if got != want {
		t.Fatalf("hash changed when only K8s image tags were bumped in Talos 1.14+ full multidoc config\n  v1.35.4: %s\n  v1.36.0: %s", got, want)
	}
}

// TestNormalizedConfigHash_K8sImageBumpChangesHash verifies that NormalizedConfigHash
// (used when ignore_kubernetes_upgrade_drift is false) produces a different hash
// when k8s image tags change. This is the default behavior: kubernetes_version bumps
// ARE detected as drift and trigger a re-apply.
func TestNormalizedConfigHash_K8sImageBumpChangesHash(t *testing.T) {
	t.Parallel()

	v135 := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.35.4
`)
	v136 := []byte(`cluster:
  apiServer:
    image: registry.k8s.io/kube-apiserver:v1.36.0
`)

	hash135, ok135 := talos.NormalizedConfigHash(v135)
	hash136, ok136 := talos.NormalizedConfigHash(v136)

	if !ok135 || !ok136 {
		t.Fatal("NormalizedConfigHash: failed on valid YAML")
	}

	if hash135 == hash136 {
		t.Fatal("NormalizedConfigHash: hash unchanged for k8s image tag bump — drift would go undetected without experimental flag")
	}
}
