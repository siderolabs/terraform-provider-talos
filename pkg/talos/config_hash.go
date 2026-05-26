// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"crypto/sha256"
	"encoding/hex"

	"go.yaml.in/yaml/v4"
)

// k8sImagePaths enumerates the YAML paths that upgrade-k8s patches when
// upgrading Kubernetes (see Talos pkg/cluster/kubernetes). talos_machine
// excludes these from hash computation and never re-applies them: they are
// owned by talos_cluster (or talosctl upgrade-k8s), which sequences updates
// per-node with health gating. talos_machine writing them in parallel would
// bypass that safety.
var k8sImagePaths = [][]string{
	{"machine", "kubelet", "image"},
	{"cluster", "apiServer", "image"},
	{"cluster", "controllerManager", "image"},
	{"cluster", "scheduler", "image"},
	{"cluster", "proxy", "image"},
}

// K8sManagedConfigHash returns a SHA256 hex digest of cfgBytes with the
// upgrade-k8s-managed image fields stripped. The digest is stable across
// kubernetes_version bumps and across whatever value upgrade-k8s last wrote
// for those fields, so talos_machine will not see them as drift.
func K8sManagedConfigHash(cfgBytes []byte) string {
	sum := sha256.Sum256(stripK8sImages(cfgBytes))

	return hex.EncodeToString(sum[:])
}

func stripK8sImages(cfgBytes []byte) []byte {
	var m map[string]any
	if err := yaml.Unmarshal(cfgBytes, &m); err != nil {
		return cfgBytes
	}

	for _, p := range k8sImagePaths {
		deleteYAMLPath(m, p...)
	}

	out, err := yaml.Marshal(m)
	if err != nil {
		return cfgBytes
	}

	return out
}

func deleteYAMLPath(m map[string]any, keys ...string) {
	if len(keys) == 0 || m == nil {
		return
	}

	if len(keys) == 1 {
		delete(m, keys[0])

		return
	}

	sub, ok := m[keys[0]].(map[string]any)
	if ok {
		deleteYAMLPath(sub, keys[1:]...)
	}
}
