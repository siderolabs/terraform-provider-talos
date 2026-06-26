// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"

	"go.yaml.in/yaml/v4"
)

// k8sImagePaths enumerates the v1alpha1 YAML paths that upgrade-k8s patches
// when upgrading Kubernetes. talos_machine excludes these from hash computation
// and never re-applies them: they are owned by talos_cluster (or talosctl
// upgrade-k8s), which sequences updates per-node with health gating.
// talos_machine writing them in parallel would bypass that safety.
//
// Source: github.com/siderolabs/talos@v1.14.0-alpha.0 pkg/cluster/kubernetes/upgrade.go
// If upgrade-k8s ever manages additional image fields in v1alpha1, add them here.
var k8sImagePaths = [][]string{
	{"machine", "kubelet", "image"},
	{"cluster", "apiServer", "image"},
	{"cluster", "controllerManager", "image"},
	{"cluster", "scheduler", "image"},
	{"cluster", "proxy", "image"},
}

// k8sDocImageKinds lists multi-doc document kinds (Talos 1.14+) whose "image"
// field is managed by upgrade-k8s. Only the image field is stripped; all other
// fields (extraArgs, env, resources, enabled, config, …) remain in the hash so
// talos_machine can detect user-driven drift in those documents.
//
// Source: github.com/siderolabs/talos@v1.14.0-alpha.2 pkg/machinery/config/generate/stdpatches/stdpatches.go
// If upgrade-k8s ever manages additional document kinds, add them here.
var k8sDocImageKinds = map[string]bool{
	"KubeAPIServerConfig":         true,
	"KubeControllerManagerConfig": true,
	"KubeSchedulerConfig":         true,
	"KubeProxyConfig":             true,
}

// K8sManagedConfigHash returns a SHA256 hex digest of cfgBytes with the
// version tags stripped from upgrade-k8s-managed image fields. The digest is
// stable across kubernetes_version bumps: only the tag (e.g. ":v1.35.4") is
// removed, so the registry and image name remain in the hash. A registry
// change is still detected as drift; a tag-only bump from upgrade-k8s is not.
//
// Multi-document YAML (Talos 1.14+) is handled: each document is processed
// individually, with tag stripping applied per document kind.
//
// stripped reports whether the K8s image tags were successfully stripped
// before hashing. When false, the hash covers the raw bytes (YAML parse or
// marshal failed); callers may log this as a warning. In practice this cannot
// happen for configs produced by talos_machine_configuration.
func K8sManagedConfigHash(cfgBytes []byte) (hash string, stripped bool) {
	normalized, ok := stripK8sImages(cfgBytes)
	sum := sha256.Sum256(normalized)

	return hex.EncodeToString(sum[:]), ok
}

func stripK8sImages(cfgBytes []byte) ([]byte, bool) {
	dec := yaml.NewDecoder(bytes.NewReader(cfgBytes))

	var docs [][]byte

	for {
		var m map[string]any

		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			return cfgBytes, false
		}

		var kind string
		if s, ok := m["kind"].(string); ok {
			kind = s
		}

		switch {
		case kind == "":
			// v1alpha1 document: strip the tag from all 5 image paths. On Talos
			// 1.14+ nodes the controllerManager and scheduler paths may already be
			// absent (moved to separate documents); stripTagAtYAMLPath is a no-op
			// for missing paths.
			for _, p := range k8sImagePaths {
				stripTagAtYAMLPath(m, p...)
			}
		case k8sDocImageKinds[kind]:
			// Multi-doc document kind (Talos 1.14+) with an upgrade-k8s-managed
			// image field. Strip only the tag; keep the registry and image name so
			// a registry change is still detected as drift.
			if s, ok := m["image"].(string); ok {
				m["image"] = stripImageTag(s)
			}
		}

		out, err := yaml.Marshal(m)
		if err != nil {
			return cfgBytes, false
		}

		docs = append(docs, out)
	}

	if len(docs) == 0 {
		return cfgBytes, false
	}

	return bytes.Join(docs, []byte("---\n")), true
}

// NormalizedConfigHash returns a SHA256 hex digest of cfgBytes after YAML
// normalization (parse + remarshal for consistent key ordering) without
// stripping upgrade-k8s-managed image tags. All config changes — including
// kubernetes_version bumps — produce a new hash.
//
// Use this when ignore_kubernetes_upgrade_drift is false (the default).
func NormalizedConfigHash(cfgBytes []byte) (hash string, ok bool) {
	dec := yaml.NewDecoder(bytes.NewReader(cfgBytes))

	var docs [][]byte

	for {
		var m map[string]any

		err := dec.Decode(&m)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			sum := sha256.Sum256(cfgBytes)

			return hex.EncodeToString(sum[:]), false
		}

		out, err := yaml.Marshal(m)
		if err != nil {
			sum := sha256.Sum256(cfgBytes)

			return hex.EncodeToString(sum[:]), false
		}

		docs = append(docs, out)
	}

	if len(docs) == 0 {
		sum := sha256.Sum256(cfgBytes)

		return hex.EncodeToString(sum[:]), false
	}

	normalized := bytes.Join(docs, []byte("---\n"))
	sum := sha256.Sum256(normalized)

	return hex.EncodeToString(sum[:]), true
}

// stripTagAtYAMLPath walks the nested map and strips the image tag from the
// string value at the given key path. It is a no-op when the path is absent.
func stripTagAtYAMLPath(m map[string]any, keys ...string) {
	if len(keys) == 0 || m == nil {
		return
	}

	if len(keys) == 1 {
		if s, ok := m[keys[0]].(string); ok {
			m[keys[0]] = stripImageTag(s)
		}

		return
	}

	sub, ok := m[keys[0]].(map[string]any)
	if ok {
		stripTagAtYAMLPath(sub, keys[1:]...)
	}
}

// stripImageTag removes the version tag from an OCI image reference, leaving
// the registry and image name. For example:
//
//	"ghcr.io/siderolabs/kubelet:v1.35.4"      → "ghcr.io/siderolabs/kubelet"
//	"my-reg.example.com:5000/kubelet:v1.35.4"  → "my-reg.example.com:5000/kubelet"
//	"ghcr.io/siderolabs/kubelet"               → "ghcr.io/siderolabs/kubelet"
func stripImageTag(image string) string {
	i := strings.LastIndex(image, ":")
	if i < 0 {
		return image
	}

	// If there is a '/' after the last ':', the colon is a registry port
	// separator, not a tag separator — leave the string unchanged.
	if strings.Contains(image[i:], "/") {
		return image
	}

	return image[:i]
}
