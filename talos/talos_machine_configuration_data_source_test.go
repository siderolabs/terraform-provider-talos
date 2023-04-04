// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"fmt"
	"net/url"
	"strings"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/siderolabs/go-pointer"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"github.com/stretchr/testify/assert"
	"golang.org/x/mod/semver"
	"gopkg.in/yaml.v3"
)

func TestAccTalosMachineConfigurationDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster", "controlplane", "https://cluster.local:6443", "", false, true, true),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "machine_configuration"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "type", "controlplane"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "config_patches.#", "1"),
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
							nil,
						)
					}),
				),
			},
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-1", "controlplane", "https://cluster-1.local:6443", "v1.26.3", true, false, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "machine_configuration"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster-1"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster-1.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "type", "controlplane"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "config_patches.0.#", "4"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "kubernetes_version", "1.26.3"),
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
								assert.Equal(t, "cp-test", config.MachineConfig.Network().Hostname())
								assert.Equal(t, pointer.To(true), config.ClusterConfig.AllowSchedulingOnControlPlanes)

								return nil
							},
						)
					}),
				),
			},
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("v1.2", "example-cluster-2", "worker", "https://cluster-2.local:6443", "", false, true, false),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "id", "machine_configuration"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_name", "example-cluster-2"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "cluster_endpoint", "https://cluster-2.local:6443"),
					resource.TestCheckResourceAttrSet("data.talos_machine_configuration.this", "machine_secrets.%"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "type", "worker"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "config_patches.#", "1"),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "kubernetes_version", constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr("data.talos_machine_configuration.this", "talos_version", "v1.2"),
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
		},
	})
}

func testAccTalosMachineConfigurationDataSourceConfig(talosConfigVersion, clusterName, machineType, clusterEndpoint, kubernetesVersion string, configPatches, docsEnabled, examplesEnabled bool) string {
	type templateConfigModel struct {
		TalosVersion      string
		ClusterName       string
		ClusterEndpoint   string
		MachineType       string
		ConfigPatches     bool
		DocsEnabled       bool
		ExamplesEnabled   bool
		KubernetesVersion string
	}

	templateConfig := templateConfigModel{
		TalosVersion:      talosConfigVersion,
		ClusterName:       clusterName,
		ClusterEndpoint:   clusterEndpoint,
		MachineType:       machineType,
		ConfigPatches:     configPatches,
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
  type                       = "{{ .MachineType }}"
  machine_secrets            = talos_machine_secrets.this.machine_secrets
  {{ if .TalosVersion  }}talos_version    = "{{ .TalosVersion }}"{{ end }}
  {{ if .ConfigPatches  }}config_patches             = [
	[
      {
        machine = {
          install = {
        	  disk = "/dev/sdd"
      	}
        }
      },
      yamldecode(file("${path.module}/testdata/patch-strategic.yaml")),
	  {
        cluster = {
          allowSchedulingOnControlPlanes = true
		}
	  },
      {
        machine = {
          network = {
          hostname = "cp-test"
          }
        }
      }
    ]
  ]{{ end }}
  docs                       = {{ .DocsEnabled }}
  examples                   = {{ .ExamplesEnabled }}
  {{ if .KubernetesVersion  }}kubernetes_version         = "{{ .KubernetesVersion }}"{{ end }}
}
`

	var config strings.Builder
	template.Must(template.New("tf_config").Parse(configTemplate)).Execute(&config, templateConfig)

	return config.String()
}

func validateGeneratedTalosMachineConfig(t *testing.T, clusterName, endpoint, installDisk, k8sVersion, machineType, mc string, docs, examples bool, extraChecks func(t *testing.T, config v1alpha1.Config) error) error {
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
	assert.Equal(t, generateInstallerImage(), machineConfig.Machine().Install().Image())
	assert.Equal(t, fmt.Sprintf("ghcr.io/siderolabs/kubelet:v%s", k8sVersion), machineConfig.Machine().Kubelet().Image())
	assert.Equal(t, true, machineConfig.Persist())
	assert.Equal(t, "v1alpha1", machineConfig.Version())
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
