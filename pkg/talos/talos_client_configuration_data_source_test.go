// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"strings"
	"testing"
	"text/template"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v4"
)

func TestAccTalosClientConfigurationDataSource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test data source with default values
			{
				Config: testAccTalosClientConfigurationDataSourceConfig(testCluster, nil, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosClientConf, "id", testCluster),
					resource.TestCheckResourceAttr(dataTalosClientConf, fieldClusterName, testCluster),
					resource.TestCheckResourceAttrSet(dataTalosClientConf, "client_configuration.%"),
					resource.TestCheckResourceAttr(dataTalosClientConf, "endpoints.#", "0"),
					resource.TestCheckResourceAttr(dataTalosClientConf, "nodes.#", "0"),
					resource.TestCheckResourceAttrSet(dataTalosClientConf, fieldTalosConfig),
					resource.TestCheckResourceAttrWith(dataTalosClientConf, fieldTalosConfig, func(value string) error {
						return validateTalosClientConfigContext(t, value, testCluster, nil, nil)
					}),
				),
			},
			// test data source with overrides
			{
				Config: testAccTalosClientConfigurationDataSourceConfig(testCluster1, []string{testIP1, testIP2}, []string{testIP3}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosClientConf, "id", testCluster1),
					resource.TestCheckResourceAttr(dataTalosClientConf, fieldClusterName, testCluster1),
					resource.TestCheckResourceAttrSet(dataTalosClientConf, "client_configuration.%"),
					resource.TestCheckResourceAttr(dataTalosClientConf, "endpoints.0", testIP1),
					resource.TestCheckResourceAttr(dataTalosClientConf, "endpoints.1", testIP2),
					resource.TestCheckResourceAttr(dataTalosClientConf, "nodes.0", testIP3),
					resource.TestCheckResourceAttrSet(dataTalosClientConf, fieldTalosConfig),
					resource.TestCheckResourceAttrWith(dataTalosClientConf, fieldTalosConfig, func(value string) error {
						return validateTalosClientConfigContext(t, value, testCluster1, []string{testIP1, testIP2}, []string{testIP3})
					}),
				),
			},
		},
	})
}

func testAccTalosClientConfigurationDataSourceConfig(clusterName string, endpoints, nodes []string) string {
	configTemplate := `
resource "talos_machine_secrets" "this" {}

data "talos_client_configuration" "this" {
	cluster_name         = "{{ .ClusterName }}"
  client_configuration = talos_machine_secrets.this.client_configuration
  {{if .Endpoints }}endpoints            = [{{- range .Endpoints }}
    "{{  . }}",
  {{- end }}
  ]{{end }}
  {{if .Nodes }}nodes                = [{{- range .Nodes }}
    "{{  . }}",
  {{- end }}
  ]{{end }}
}
`

	var config strings.Builder

	template.Must(template.New(tfConfigTemplateName).Parse(configTemplate)).Execute(&config, struct { //nolint:errcheck
		ClusterName string
		Endpoints   []string
		Nodes       []string
	}{
		ClusterName: clusterName,
		Endpoints:   endpoints,
		Nodes:       nodes,
	})

	return config.String()
}

func validateTalosClientConfigContext(t *testing.T, tc, contextName string, endpoints, nodes []string) error {
	var talosConfig config.Config

	if err := yaml.Unmarshal([]byte(tc), &talosConfig); err != nil {
		return err
	}

	assert.Equal(t, contextName, talosConfig.Context)

	if endpoints != nil {
		assert.Equal(t, endpoints, talosConfig.Contexts[contextName].Endpoints)
	}

	if nodes != nil {
		assert.Equal(t, nodes, talosConfig.Contexts[contextName].Nodes)
	}

	return nil
}
