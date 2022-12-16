// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestAccTalosClientConfiguration(t *testing.T) {
	rString := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
	name := fmt.Sprintf("data.talos_client_configuration.%s", rString)

	resource.ParallelTest(t, resource.TestCase{
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClientConfigurationConfig(rString, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "cluster_name", rString),
					resource.TestCheckResourceAttrSet(name, "machine_secrets"),
					resource.TestCheckNoResourceAttr(name, "endpoints"),
					resource.TestCheckNoResourceAttr(name, "nodes"),
					resource.TestCheckResourceAttrWith(name, "talos_config", func(value string) error {
						return validateTalosClientConfigContext(t, value, rString, nil, nil)
					}),
				),
			},
			{
				Config: testAccTalosClientConfigurationConfigWithNodesEndpoints(rString, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "cluster_name", rString),
					resource.TestCheckResourceAttrSet(name, "machine_secrets"),
					resource.TestCheckResourceAttr(name, "endpoints.0", "bar"),
					resource.TestCheckResourceAttr(name, "nodes.0", "foo"),
					resource.TestCheckResourceAttrWith(name, "talos_config", func(value string) error {
						return validateTalosClientConfigContext(t, value, rString, []string{"bar"}, []string{"foo"})
					}),
				),
			},
		},
	})
}

func testAccTalosClientConfigurationConfig(rName, talosVersion string) string {
	return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {
}

data "talos_client_configuration" "%s" {
	cluster_name = "%s"
	machine_secrets = talos_machine_secrets.%s.machine_secrets
}
`, rName, rName, rName, rName)
}

func testAccTalosClientConfigurationConfigWithNodesEndpoints(rName, talosVersion string) string {
	return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {
}

data "talos_client_configuration" "%s" {
	cluster_name = "%s"
	machine_secrets = talos_machine_secrets.%s.machine_secrets
	endpoints = ["bar"]
	nodes = ["foo"]
}
`, rName, rName, rName, rName)
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
