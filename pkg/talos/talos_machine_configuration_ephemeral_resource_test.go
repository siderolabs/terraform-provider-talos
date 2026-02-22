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

// TestAccTalosMachineConfigurationEphemeralResource tests that the ephemeral
// resource generates machine configuration from machine secrets.
func TestAccTalosMachineConfigurationEphemeralResource(t *testing.T) {
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

ephemeral "talos_machine_configuration" "this" {
	cluster_name     = "test-cluster"
	cluster_endpoint = "https://10.0.0.1:6443"
	machine_type     = "controlplane"
	machine_secrets  = ephemeral.talos_machine_secrets.this.machine_secrets
}

provider "echo" {
	data = {
		machine_configuration = ephemeral.talos_machine_configuration.this.machine_configuration
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("machine_configuration"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosMachineConfigurationEphemeralResourceWorker tests generating
// a worker machine configuration.
func TestAccTalosMachineConfigurationEphemeralResourceWorker(t *testing.T) {
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

ephemeral "talos_machine_configuration" "this" {
	cluster_name     = "test-cluster"
	cluster_endpoint = "https://10.0.0.1:6443"
	machine_type     = "worker"
	machine_secrets  = ephemeral.talos_machine_secrets.this.machine_secrets
}

provider "echo" {
	data = {
		machine_configuration = ephemeral.talos_machine_configuration.this.machine_configuration
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("machine_configuration"), knownvalue.NotNull()),
				},
			},
		},
	})
}
