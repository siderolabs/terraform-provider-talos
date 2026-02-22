// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccTalosClientConfigurationEphemeralResource tests that the ephemeral
// resource generates client configuration and can be chained from machine_secrets.
func TestAccTalosClientConfigurationEphemeralResource(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: `
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_client_configuration" "this" {
	cluster_name         = "test-cluster"
	client_configuration = ephemeral.talos_machine_secrets.this.client_configuration
	endpoints            = ["10.0.0.1"]
	nodes                = ["10.0.0.2"]
}

provider "echo" {
	data = {
		cluster_name = ephemeral.talos_client_configuration.this.cluster_name
		talos_config = ephemeral.talos_client_configuration.this.talos_config
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("talos_config"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("cluster_name"), knownvalue.StringExact("test-cluster")),
				},
			},
		},
	})
}

// TestAccTalosClientConfigurationEphemeralResourceChained tests chaining
// ephemeral resources together: machine_secrets -> client_configuration.
func TestAccTalosClientConfigurationEphemeralResourceChained(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: `
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_client_configuration" "this" {
	cluster_name         = "chained-cluster"
	client_configuration = ephemeral.talos_machine_secrets.this.client_configuration
}

provider "echo" {
	data = {
		has_talos_config = ephemeral.talos_client_configuration.this.talos_config != ""
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("has_talos_config"), knownvalue.Bool(true)),
				},
			},
		},
	})
}
