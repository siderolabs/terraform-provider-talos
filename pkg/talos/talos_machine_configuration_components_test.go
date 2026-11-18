// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/talos/pkg/machinery/config"
	"github.com/siderolabs/talos/pkg/machinery/config/configloader"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

const containerConfigPatch = `apiVersion: v1alpha1
kind: ContainerConfig
name: hello
image: docker.io/library/busybox:1.37.0
entrypoint: [sh, -c, "sleep infinity"]
`

type testComponents struct {
	disableKubernetes bool
	disableEtcd       bool
}

func generateTestMachineConfig(t *testing.T, machineType machine.Type, talosVersion string, components testComponents, patches ...string) (string, *secrets.Bundle, error) {
	t.Helper()

	// talos_machine_secrets always generates a full bundle (including Kubernetes and etcd PKI),
	// so do the same here regardless of the components.
	contract, err := config.ParseContractFromVersion(talosVersion)
	require.NoError(t, err)

	secretsBundle, err := secrets.NewBundle(secrets.NewFixedClock(time.Now()), contract)
	require.NoError(t, err)

	mc, err := talos.GenerateMachineConfiguration(secretsBundle, machineType, talosVersion, components.disableKubernetes, components.disableEtcd, patches...)

	return mc, secretsBundle, err
}

func kubernetesKinds(kinds []string) []string {
	var out []string

	for _, kind := range kinds {
		if strings.HasPrefix(kind, "Kube") {
			out = append(out, kind)
		}
	}

	return out
}

// pemBodyLine returns the first base64 line of a PEM block. Depending on the field, the generated
// configuration holds certificates base64-encoded (etcd CA) or as PEM (Kubernetes CA, multidoc).
func pemBodyLine(t *testing.T, pem []byte) string {
	t.Helper()

	lines := strings.Split(string(pem), "\n")
	require.GreaterOrEqual(t, len(lines), 2, "not a PEM block")

	return lines[1]
}

func TestGenerateMachineConfigComponents(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		components     testComponents
		wantKubernetes bool
		wantEtcd       bool
	}{
		"default":        {wantKubernetes: true, wantEtcd: true},
		"etcd only":      {components: testComponents{disableKubernetes: true}, wantEtcd: true},
		"no k8s or etcd": {components: testComponents{disableKubernetes: true, disableEtcd: true}},
	}

	for name, test := range tests {
		for _, machineType := range []machine.Type{machine.TypeControlPlane, machine.TypeWorker} {
			t.Run(name+"/"+machineType.String(), func(t *testing.T) {
				t.Parallel()

				mc, secretsBundle, err := generateTestMachineConfig(t, machineType, "v1.14", test.components, containerConfigPatch)
				require.NoError(t, err)

				assertGeneratedComponents(t, machineType, test.wantKubernetes, test.wantEtcd, mc, secretsBundle)
			})
		}
	}
}

// assertGeneratedComponents checks which components a generated configuration contains.
func assertGeneratedComponents(t *testing.T, machineType machine.Type, wantKubernetes, wantEtcd bool, mc string, secretsBundle *secrets.Bundle) {
	t.Helper()

	provider, err := configloader.NewFromBytes([]byte(mc))
	require.NoError(t, err)

	kinds := make([]string, 0, len(provider.Documents()))
	for _, doc := range provider.Documents() {
		kinds = append(kinds, doc.Kind())
	}

	assert.Contains(t, kinds, "ContainerConfig")

	raw := provider.RawV1Alpha1()
	require.NotNil(t, raw)
	assert.Equal(t, machineType, raw.Machine().Type())

	if wantKubernetes {
		assert.Contains(t, kinds, "KubeletConfig")
	} else {
		assert.Empty(t, kubernetesKinds(kinds), "no Kubernetes documents expected")
	}

	assert.Equal(t, wantKubernetes, provider.K8sClusterConfig() != nil, "Kubernetes cluster config")
	assert.Equal(t, wantKubernetes, strings.Contains(mc, pemBodyLine(t, secretsBundle.Certs.K8s.Crt)), "Kubernetes CA")

	// The Talos worker generator sets the bootstrap token unconditionally
	// (pkg/machinery/config/generate/worker.go), same as `talosctl gen config --skip-k8s-etcd`.
	assert.Equal(t, wantKubernetes || machineType == machine.TypeWorker, raw.ClusterConfig.BootstrapToken != "", "cluster.token") //nolint:staticcheck // legacy configuration

	// workers never run etcd
	wantEtcd = wantEtcd && machineType == machine.TypeControlPlane

	assert.Equal(t, wantEtcd, raw.ClusterConfig.EtcdConfig != nil, "cluster.etcd")
	assert.Equal(t, wantEtcd, strings.Contains(mc, base64.StdEncoding.EncodeToString(secretsBundle.Certs.Etcd.Crt)), "etcd CA")
}

func TestGenerateMachineConfigComponentsInvalid(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		talosVersion string
		wantErr      string
		components   testComponents
	}{
		"kubernetes without etcd": {
			talosVersion: "v1.14",
			components:   testComponents{disableEtcd: true},
			wantErr:      "kubernetes requires etcd",
		},
		// contracts that still generate Kubernetes configuration inline in the v1alpha1 document
		// are rejected instead of half-stripped
		"no k8s or etcd, v1.13": {
			talosVersion: "v1.13",
			components:   testComponents{disableKubernetes: true, disableEtcd: true},
			wantErr:      "disabling kubernetes requires talos_version v1.14 or later",
		},
		"etcd only, v1.13": {
			talosVersion: "v1.13",
			components:   testComponents{disableKubernetes: true},
			wantErr:      "disabling kubernetes requires talos_version v1.14 or later",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, _, err := generateTestMachineConfig(t, machine.TypeControlPlane, test.talosVersion, test.components)
			require.ErrorContains(t, err, test.wantErr)
		})
	}

	t.Run("default, v1.13", func(t *testing.T) {
		t.Parallel()

		_, _, err := generateTestMachineConfig(t, machine.TypeControlPlane, "v1.13", testComponents{})
		require.NoError(t, err)
	})
}

// TestValidateMachineConfigurationConfigComponents covers the plan-time check shared by the
// data source and the ephemeral resource.
func TestValidateMachineConfigurationConfigComponents(t *testing.T) {
	t.Parallel()

	components := func(kubernetes, etcd types.Bool) *talos.MachineConfigurationComponents {
		return &talos.MachineConfigurationComponents{Kubernetes: kubernetes, Etcd: etcd}
	}

	var (
		on      = types.BoolValue(true)
		off     = types.BoolValue(false)
		null    = types.BoolNull()
		unknown = types.BoolUnknown()
	)

	tests := map[string]struct {
		talosVersion types.String
		components   *talos.MachineConfigurationComponents
		wantErr      string
	}{
		"unset":                                  {talosVersion: types.StringValue("v1.13")},
		"empty object":                           {talosVersion: types.StringValue("v1.13"), components: components(null, null)},
		"all enabled":                            {talosVersion: types.StringValue("v1.13"), components: components(on, on)},
		"no k8s or etcd, v1.14":                  {talosVersion: types.StringValue("v1.14"), components: components(off, off)},
		"no k8s or etcd, v1.15.0":                {talosVersion: types.StringValue("v1.15.0"), components: components(off, off)},
		"no k8s or etcd, null version (bundled)": {talosVersion: types.StringNull(), components: components(off, off)},
		"etcd only, v1.14":                       {talosVersion: types.StringValue("v1.14"), components: components(off, on)},
		"etcd only (null etcd), v1.14":           {talosVersion: types.StringValue("v1.14"), components: components(off, null)},
		"no k8s or etcd, unknown version":        {talosVersion: types.StringUnknown(), components: components(off, off)},
		"kubernetes without etcd, unknown version": {
			talosVersion: types.StringUnknown(), components: components(on, off), wantErr: "kubernetes requires etcd",
		},
		"no k8s or etcd, unparsable version": {talosVersion: types.StringValue("nil"), components: components(off, off)},
		"unknown kubernetes, etcd off":       {talosVersion: types.StringValue("v1.14"), components: components(unknown, off)},
		"kubernetes on, unknown etcd":        {talosVersion: types.StringValue("v1.14"), components: components(on, unknown)},
		"kubernetes off, unknown etcd, v1.13": {
			talosVersion: types.StringValue("v1.13"), components: components(off, unknown), wantErr: "requires talos_version v1.14",
		},
		"no k8s or etcd, v1.13": {
			talosVersion: types.StringValue("v1.13"), components: components(off, off), wantErr: "requires talos_version v1.14",
		},
		"etcd only, v1.13": {
			talosVersion: types.StringValue("v1.13"), components: components(off, on), wantErr: "requires talos_version v1.14",
		},
		"kubernetes without etcd": {
			talosVersion: types.StringValue("v1.14"), components: components(on, off), wantErr: "kubernetes requires etcd",
		},
		"kubernetes (null) without etcd": {
			talosVersion: types.StringValue("v1.14"), components: components(null, off), wantErr: "kubernetes requires etcd",
		},
		"kubernetes without etcd, null version": {
			talosVersion: types.StringNull(), components: components(on, off), wantErr: "kubernetes requires etcd",
		},
		"kubernetes without etcd, v1.13": {
			talosVersion: types.StringValue("v1.13"), components: components(on, off), wantErr: "kubernetes requires etcd",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var diags diag.Diagnostics

			talos.ValidateMachineConfigurationConfig(
				t.Context(),
				types.StringValue("https://127.0.0.1:6443"),
				types.ListNull(types.StringType),
				test.talosVersion,
				test.components,
				&diags,
			)

			if test.wantErr == "" {
				assert.False(t, diags.HasError(), "unexpected diagnostics: %v", diags)

				return
			}

			require.Len(t, diags.Errors(), 1)
			assert.Contains(t, diags.Errors()[0].Detail(), test.wantErr)

			withPath, ok := diags.Errors()[0].(diag.DiagnosticWithPath)
			require.True(t, ok, "diagnostic without attribute path")
			assert.Equal(t, path.Root("components"), withPath.Path())
		})
	}
}
