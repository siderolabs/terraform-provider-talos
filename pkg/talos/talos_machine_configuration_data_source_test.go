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
	"go.yaml.in/yaml/v4"
	"golang.org/x/mod/semver"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

func TestAccTalosMachineConfigurationDataSource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test data source with default values
			{
				Config: testAccTalosMachineConfigurationDataSourceConfig("", exampleCluster, fieldControlplane, testClusterLocalFull, "", false, false, true, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosMachineConf, "id", exampleCluster),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterName, exampleCluster),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterEndpoint, testClusterLocalFull),
					resource.TestCheckResourceAttrSet(dataTalosMachineConf, attrMachineSecretsPercent),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldMachineType, fieldControlplane),
					resource.TestCheckNoResourceAttr(dataTalosMachineConf, fieldConfigPatches),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldKubernetesVersion, constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldTalosVersion, semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldDocs, boolTrue),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldExamples, boolTrue),
					resource.TestCheckResourceAttrWith(dataTalosMachineConf, fieldMachineConfiguration, func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							exampleCluster,
							testClusterLocalFull,
							testDevSDA,
							constants.DefaultKubernetesVersion,
							fieldControlplane,
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
				Config: testAccTalosMachineConfigurationDataSourceConfig("", exampleCluster1, fieldControlplane, testClusterEndpoint1, "v1.28.0", true, false, false, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosMachineConf, "id", exampleCluster1),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterName, exampleCluster1),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterEndpoint, testClusterEndpoint1),
					resource.TestCheckResourceAttrSet(dataTalosMachineConf, attrMachineSecretsPercent),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldMachineType, fieldControlplane),
					resource.TestCheckResourceAttr(dataTalosMachineConf, attrConfigPatchesCount, "3"),
					resource.TestCheckResourceAttr(dataTalosMachineConf, attrConfigPatchesFirst, "\"machine\":\n  \"install\":\n    \"disk\": \"/dev/sdd\"\n"),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldKubernetesVersion, "v1.28.0"),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldTalosVersion, semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldDocs, boolFalse),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldExamples, boolFalse),
					resource.TestCheckResourceAttrWith(dataTalosMachineConf, fieldMachineConfiguration, func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							exampleCluster1,
							testClusterEndpoint1,
							"/dev/sdd",
							"1.28.0",
							fieldControlplane,
							value,
							false,
							false,
							func(t *testing.T, config v1alpha1.Config) error {
								assert.Equal(t, map[string]string{"foo": "bar"}, config.Machine().Sysfs())
								assert.Equal(t, map[string][]string{"foo": {"bar"}}, config.Cluster().APIServer().ExtraArgs())
								assert.Equal(t, "cp-test", config.Hostname())
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
				Config: testAccTalosMachineConfigurationDataSourceConfig("", exampleCluster2, fieldWorker, testClusterEndpoint2, "", false, false, true, false),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosMachineConf, "id", exampleCluster2),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterName, exampleCluster2),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterEndpoint, testClusterEndpoint2),
					resource.TestCheckResourceAttrSet(dataTalosMachineConf, attrMachineSecretsPercent),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldMachineType, fieldWorker),
					resource.TestCheckNoResourceAttr(dataTalosMachineConf, fieldConfigPatches),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldKubernetesVersion, constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldTalosVersion, semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldDocs, boolTrue),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldExamples, boolFalse),
					resource.TestCheckResourceAttrWith(dataTalosMachineConf, fieldMachineConfiguration, func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							exampleCluster2,
							testClusterEndpoint2,
							testDevSDA,
							constants.DefaultKubernetesVersion,
							fieldWorker,
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
				Config: testAccTalosMachineConfigurationDataSourceConfig(testV1p2, exampleCluster3, fieldControlplane, testClusterEndpoint3, "", false, false, false, true),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosMachineConf, "id", exampleCluster3),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterName, exampleCluster3),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldClusterEndpoint, testClusterEndpoint3),
					resource.TestCheckResourceAttrSet(dataTalosMachineConf, attrMachineSecretsPercent),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldMachineType, fieldControlplane),
					resource.TestCheckNoResourceAttr(dataTalosMachineConf, fieldConfigPatches),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldKubernetesVersion, constants.DefaultKubernetesVersion),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldTalosVersion, testV1p2),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldDocs, boolFalse),
					resource.TestCheckResourceAttr(dataTalosMachineConf, fieldExamples, boolTrue),
					resource.TestCheckResourceAttrWith(dataTalosMachineConf, fieldMachineConfiguration, func(value string) error {
						return validateGeneratedTalosMachineConfig(
							t,
							exampleCluster3,
							testClusterEndpoint3,
							testDevSDA,
							constants.DefaultKubernetesVersion,
							fieldControlplane,
							value,
							false,
							true,
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
				Config:      testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-4", fieldControlplane, "cluster.local", "", false, false, true, true),
				ExpectError: regexp.MustCompile("no scheme and port specified for the cluster endpoint URL\ntry: \"https://cluster.local:6443\""),
			},
			// test validating talos machine config features version
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("nil", "example-cluster-5", fieldControlplane, testClusterLocal, "", false, false, true, true),
				ExpectError: regexp.MustCompile("error parsing version \"vnil\""),
			},
			// test validating machine type
			{
				Config:      testAccTalosMachineConfigurationDataSourceConfig("", "example-cluster-6", "control", testClusterLocal, "", false, false, true, true),
				ExpectError: regexp.MustCompile("Attribute machine_type value must be one of:"),
			},
			// test validating config patches at plan time
			{
				PlanOnly:    true,
				Config:      testAccTalosMachineConfigurationDataSourceConfig(testV1p3, "example-cluster-8", fieldControlplane, testClusterLocal, "v1.23.0", true, true, true, true),
				ExpectError: regexp.MustCompile(`error decoding document /v1alpha1/ \(line 1\): unknown keys found during`),
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
variable "talos_version" {
  type = string
  default = "{{ .TalosVersion }}"
}

resource "talos_machine_secrets" "this" {
  {{ if .TalosVersion  }}talos_version = var.talos_version{{ end }}
}

data "talos_machine_configuration" "this" {
  cluster_name               = "{{ .ClusterName }}"
  cluster_endpoint           = "{{ .ClusterEndpoint }}"
  machine_type               = "{{ .MachineType }}"
  machine_secrets            = talos_machine_secrets.this.machine_secrets
  {{ if .TalosVersion  }}talos_version    = var.talos_version{{ end }}
  {{ if .ConfigPatches  }}config_patches             = [
    yamlencode({
      machine = {
        install = {
      	  disk = "/dev/sdd"
    	}
      }
    }),
    file("${path.module}/testdata/patch-strategic.yaml"),
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

	template.Must(template.New(tfConfigTemplateName).Parse(configTemplate)).Execute(&config, templateConfig) //nolint:errcheck

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

	installDiskConfig := machineConfig.Machine().Install().Disk()

	ep, err := url.Parse(endpoint)
	if err != nil {
		return err
	}

	switch machineType {
	case fieldControlplane:
		assert.Equal(t, machine.TypeControlPlane, machineConfig.Machine().Type())
		assert.Equal(t, clusterName, machineConfig.Cluster().Name())
	case fieldWorker:
		assert.Equal(t, machine.TypeWorker, machineConfig.Machine().Type())
	}

	assert.Equal(t, ep, machineConfig.Cluster().Endpoint())
	assert.Equal(t, constants.DefaultDNSDomain, machineConfig.Cluster().Network().DNSDomain())
	assert.Equal(t, installDisk, installDiskConfig)
	assert.Equal(t, talos.GenerateInstallerImage(), machineConfig.Machine().Install().Image())
	assert.Equal(t, fmt.Sprintf("ghcr.io/siderolabs/kubelet:v%s", k8sVersion), machineConfig.Machine().Kubelet().Image())
	assert.Equal(t, "v1alpha1", machineConfig.ConfigVersion)
	assert.True(t, machineConfig.Cluster().Discovery().Enabled())

	if docs {
		assert.Equal(t, "Indicates the schema used to decode the contents.", machineConfig.Doc().Field(0).Description)
	} else {
		assert.NotContains(t, mc, "Indicates the schema used to decode the contents.")
	}

	if examples {
		// verifying there's examples
		assert.Contains(t, mc, (`
    #   # Uncomment this to enable SANs.
    #   - 10.0.0.10
    #   - 172.16.0.10
    #   - 192.168.0.10
`))
	} else {
		// verifying there's no examples
		assert.NotContains(t, mc, (`
    #   # Uncomment this to enable SANs.
    #   - 10.0.0.10
    #   - 172.16.0.10
    #   - 192.168.0.10
`))
	}

	if extraChecks != nil {
		return extraChecks(t, machineConfig)
	}

	return nil
}
