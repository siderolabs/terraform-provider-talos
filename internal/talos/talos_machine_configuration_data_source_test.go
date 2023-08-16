// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"github.com/stretchr/testify/assert"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"

	"github.com/siderolabs/terraform-provider-talos/internal/talos"
)

func TestAccTalosMachineConfigurationDataSource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test data source with default values
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster", "controlplane", "https://cluster.local:6443", "", false, false, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "example-cluster"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "machine_type", "controlplane"),
					resource.TestCheckNoResourceAttr("data.talos_machine_configuration.this", "config_patches"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "kubernetes_version", constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "talos_version", semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "docs", "true"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "examples", "true"),
					resource.TestCheckResourceAttrWith("data.talos_machine_configuration.this", "machine_configuration", func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							"example-cluster",
							"https://cluster.local:6443",
							"/dev/sda",
							constants.DefaultKubernetesVersion,
							"controlplane",
							value,
							true,
							true,
							func(t *testing.T, config v1alpha1.Config) error {
								assert.Empty(t, config.Cluster().AESCBCEncryptionSecret())
								assert.NotEmpty(t, config.Cluster().SecretboxEncryptionSecret())

								return nil
							},
						)
					}),
				),
			},
			// test data source with custom values
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-1", "controlplane", "https://cluster-1.local:6443", "v1.26.3", true, false, false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "example-cluster-1"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster-1"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster-1.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "machine_type", "controlplane"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "config_patches.#", "4"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "config_patches.0", "\"machine\":\n  \"install\":\n    \"disk\": \"/dev/sdd\"\n"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "kubernetes_version", "v1.26.3"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "talos_version", semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "docs", "false"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "examples", "false"),
					resource.TestCheckResourceAttrWith("data.talos_machine_configuration.this", "machine_configuration", func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							"example-cluster-1",
							"https://cluster-1.local:6443",
							"/dev/sdd",
							"1.26.3",
							"controlplane",
							value,
							false,
							false,
							func(t *testing.T, config v1alpha1.Config) error {
								assert.Equal(t, map[string]string{"foo": "bar"}, config.Machine().Sysfs())
								assert.Equal(t, map[string]string{"foo": "bar"}, config.Cluster().APIServer().ExtraArgs())
								assert.Equal(t, "cp-test", config.Machine().Network().Hostname())
								assert.Equal(t, true, config.Cluster().ScheduleOnControlPlanes())
								assert.Empty(t, config.Cluster().AESCBCEncryptionSecret())
								assert.NotEmpty(t, config.Cluster().SecretboxEncryptionSecret())

								return nil
							},
						)
					}),
				),
			},
			// test data source for a worker node
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-2", "worker", "https://cluster-2.local:6443", "", false, false, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "example-cluster-2"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster-2"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster-2.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "machine_type", "worker"),
					resource.TestCheckNoResourceAttr("data.talos_machine_configuration.this", "config_patches"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "kubernetes_version", constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "talos_version", semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "docs", "true"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "examples", "false"),
					resource.TestCheckResourceAttrWith("data.talos_machine_configuration.this", "machine_configuration", func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							"example-cluster-2",
							"https://cluster-2.local:6443",
							"/dev/sda",
							constants.DefaultKubernetesVersion,
							"worker",
							value,
							true,
							false,
							nil,
						)
					}),
				),
			},
			// test data source for talos v1.2 that has aescbc encryption
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("v1.2", "example-cluster-3", "controlplane", "https://cluster-3.local:6443", "", false, false, false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "example-cluster-3"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster-3"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster-3.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "machine_type", "controlplane"),
					resource.TestCheckNoResourceAttr("data.talos_machine_configuration.this", "config_patches"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "kubernetes_version", constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "talos_version", "v1.2"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "docs", "false"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "examples", "true"),
					resource.TestCheckResourceAttrWith("data.talos_machine_configuration.this", "machine_configuration", func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							"example-cluster-3",
							"https://cluster-3.local:6443",
							"/dev/sda",
							constants.DefaultKubernetesVersion,
							"controlplane",
							value,
							true,
							false,
							func(t *testing.T, config v1alpha1.Config) error {
								assert.NotEmpty(t, config.Cluster().AESCBCEncryptionSecret())
								assert.Empty(t, config.Cluster().SecretboxEncryptionSecret())

								return nil
							},
						)
					}),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test validating cluster endpoint
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-4", "controlplane", "cluster.local", "", false, false, true, true),
				ExpectError: regexp.MustCompile("no scheme and port specified for the cluster endpoint URL\ntry: \"https://cluster.local:6443\""),
			},
			// test validating talos machine config features version
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("nil", "example-cluster-5", "controlplane", "https://cluster.local", "", false, false, true, true),
				ExpectError: regexp.MustCompile("error parsing version \"nil\""),
			},
			// test validating machine type
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-6", "control", "https://cluster.local", "", false, false, true, true),
				ExpectError: regexp.MustCompile("Attribute machine_type value must be one of:"),
			},
			// test validating kubernetes compatibility with the default talos version
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-7", "controlplane", "https://cluster.local", "v1.25.0", false, false, true, true),
				ExpectError: regexp.MustCompile("version of Kubernetes 1.25.0 is too old to be used with Talos 1.5.0"),
			},
			// test validating kubernetes compatibility with a specific talos version
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("v1.3", "example-cluster-8", "controlplane", "https://cluster.local", "v1.23.0", false, false, true, true),
				ExpectError: regexp.MustCompile("version of Kubernetes 1.23.0 is too old to be used with Talos 1.3.0"),
			},
			// test validating config patches at plan time
			{
				PlanOnly:    true,
				Config:      testAccTalosMachineConfigurationDataSourceConfig("v1.3", "example-cluster-8", "controlplane", "https://cluster.local", "v1.23.0", true, true, true, true),
				ExpectError: regexp.MustCompile("unknown keys found during decoding:"),
			},
		},
	})
}

func testAccTalosMachineConfigurationDataSourceConfig(
	talosConfigVersion,
	clusterName,
	machineType,
	clusterEndpoint,
	kubernetesVersion string,
	configPatches,
	invalidPatch,
	docsEnabled,
	examplesEnabled bool,
) string {
	type templateConfigModel struct {
		TalosVersion      string
		ClusterName       string
		ClusterEndpoint   string
		MachineType       string
		KubernetesVersion string
		ConfigPatches     bool
		InvalidPatch      bool
		DocsEnabled       bool
		ExamplesEnabled   bool
	}

	templateConfig := templateConfigModel{
		TalosVersion:      talosConfigVersion,
		ClusterName:       clusterName,
		ClusterEndpoint:   clusterEndpoint,
		MachineType:       machineType,
		ConfigPatches:     configPatches,
		InvalidPatch:      invalidPatch,
		DocsEnabled:       docsEnabled,
		ExamplesEnabled:   examplesEnabled,
		KubernetesVersion: kubernetesVersion,
	}

	configTemplate := `
resource "talos_machine_secrets" "this" {
  {{ if .TalosVersion  }}talos_version = "{{ .TalosVersion }}"{{ end }}
}

data "talos_machine_configuration" "this" {
  cluster_name               = "{{ .ClusterName }}"
  cluster_endpoint           = "{{ .ClusterEndpoint }}"
  machine_type               = "{{ .MachineType }}"
  machine_secrets            = talos_machine_secrets.this.machine_secrets
  {{ if .TalosVersion  }}talos_version    = "{{ .TalosVersion }}"{{ end }}
  {{ if .ConfigPatches  }}config_patches             = [
    yamlencode({
      machine = {
        install = {
      	  disk = "/dev/sdd"
    	}
      }
    }),
    file("${path.module}/testdata/patch-strategic.yaml"),
    file("${path.module}/testdata/patch-json6502.json"),
	{{ if .InvalidPatch  }}file("${path.module}/testdata/patch-invalid.yaml"),{{ end }}
    yamlencode({
      machine = {
        network = {
        hostname = "cp-test"
        }
      }
    })
  ]{{ end }}
  docs                       = {{ .DocsEnabled }}
  examples                   = {{ .ExamplesEnabled }}
  {{ if .KubernetesVersion  }}kubernetes_version         = "{{ .KubernetesVersion }}"{{ end }}
}
`

	var config strings.Builder

	template.Must(template.New("tf_config").Parse(configTemplate)).Execute(&config, templateConfig) //nolint:errcheck

	return config.String()
}

func validateGeneratedTalosMachineConfig(
	t *testing.T,
	clusterName,
	endpoint,
	installDisk,
	k8sVersion,
	machineType,
	mc string,
	docs,
	examples bool,
	extraChecks func(t *testing.T, config v1alpha1.Config) error,
) error {
	var machineConfig v1alpha1.Config

	if err := yaml.Unmarshal([]byte(mc), &machineConfig); err != nil {
		return err
	}

	installDiskConfig, err := machineConfig.Machine().Install().Disk()
	if err != nil {
		return err
	}

	ep, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	switch machineType {
	case "controlplane":
		assert.Equal(t, machine.TypeControlPlane, machineConfig.Machine().Type())
		assert.Equal(t, clusterName, machineConfig.Cluster().Name())
	case "worker":
		assert.Equal(t, machine.TypeWorker, machineConfig.Machine().Type())
	}

	assert.Equal(t, ep, machineConfig.Cluster().Endpoint())
	assert.Equal(t, constants.DefaultDNSDomain, machineConfig.Cluster().Network().DNSDomain())
	assert.Equal(t, installDisk, installDiskConfig)
	assert.Equal(t, talos.GenerateInstallerImage(), machineConfig.Machine().Install().Image())
	assert.Equal(t, fmt.Sprintf("ghcr.io/siderolabs/kubelet:v%s", k8sVersion), machineConfig.Machine().Kubelet().Image())
	assert.Equal(t, true, machineConfig.Persist())
	assert.Equal(t, "v1alpha1", machineConfig.ConfigVersion)
	assert.Equal(t, true, machineConfig.Cluster().Discovery().Enabled())

	if docs {
		assert.Equal(t, "Indicates the schema used to decode the contents.", machineConfig.Doc().Field(0).Description)
	} else {
		assert.NotContains(t, mc, "Indicates the schema used to decode the contents.")
	}

	if examples {
		// verifying there's examples
		assert.Contains(t, mc, (`
        # diskSelector:
        #     size: 4GB # Disk size.
`))
	} else {
		// verifying there's no examples
		assert.NotContains(t, mc, (`
	        # diskSelector:
	        #     size: 4GB # Disk size.
`))
	}

	if extraChecks != nil {
		return extraChecks(t, machineConfig)
	}

	return nil
}
