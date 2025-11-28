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
				Config: testAccTalosClientConfigurationDataSourceConfig("test-cluster", nil, nil),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "id", "test-cluster"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "cluster_name", "test-cluster"),
					resource.TestCheckResourceAttrSet("data.talos_client_configuration.this", "client_configuration.%"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "endpoints.#", "0"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "nodes.#", "0"),
					resource.TestCheckResourceAttrSet("data.talos_client_configuration.this", "talos_config"),
					resource.TestCheckResourceAttrWith("data.talos_client_configuration.this", "talos_config", func(value string) error {
						return validateTalosClientConfigContext(t, value, "test-cluster", nil, nil)
					}),
				),
			},
			// test data source with overrides
			{
				Config: testAccTalosClientConfigurationDataSourceConfig("test-cluster-1", []string{"10.5.0.2", "10.5.0.3"}, []string{"10.5.0.4"}),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "id", "test-cluster-1"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "cluster_name", "test-cluster-1"),
					resource.TestCheckResourceAttrSet("data.talos_client_configuration.this", "client_configuration.%"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "endpoints.0", "10.5.0.2"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "endpoints.1", "10.5.0.3"),
					resource.TestCheckResourceAttr("data.talos_client_configuration.this", "nodes.0", "10.5.0.4"),
					resource.TestCheckResourceAttrSet("data.talos_client_configuration.this", "talos_config"),
					resource.TestCheckResourceAttrWith("data.talos_client_configuration.this", "talos_config", func(value string) error {
						return validateTalosClientConfigContext(t, value, "test-cluster-1", []string{"10.5.0.2", "10.5.0.3"}, []string{"10.5.0.4"})
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

	template.Must(template.New("tf_config").Parse(configTemplate)).Execute(&config, struct { //nolint:errcheck
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
