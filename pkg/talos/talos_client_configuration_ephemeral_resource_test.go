// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
)

// TestAccTalosClientConfigurationEphemeralResourceFromMachineSecrets tests that the ephemeral
// resource generates talos_config from machine_secrets.
func TestAccTalosClientConfigurationEphemeralResourceFromMachineSecrets(t *testing.T) {
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
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	endpoints       = ["10.0.0.1"]
	nodes           = ["10.0.0.2"]
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

// TestAccTalosClientConfigurationEphemeralResourceCustomTTL tests that not_before + crt_ttl
// are accepted and the resource opens without error.
func TestAccTalosClientConfigurationEphemeralResourceCustomTTL(t *testing.T) {
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
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	not_before      = "2024-01-01T00:00:00Z"
	crt_ttl         = "8760h"
}

provider "echo" {
	data = {
		talos_config = ephemeral.talos_client_configuration.this.talos_config
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("talos_config"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosClientConfigurationEphemeralResourceCrtTTLInvalid tests that an invalid
// crt_ttl value is rejected with an error.
func TestAccTalosClientConfigurationEphemeralResourceCrtTTLInvalid(t *testing.T) {
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
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	crt_ttl         = "not-a-duration"
}

provider "echo" {
	data = {}
}

resource "echo" "test" {}
`,
				ExpectError: regexp.MustCompile(`invalid crt_ttl`),
			},
		},
	})
}

// TestAccTalosClientConfigurationEphemeralResourceDeterminism tests that two opens with
// identical inputs (CA-pinned, no not_before) produce byte-identical talos_config.
func TestAccTalosClientConfigurationEphemeralResourceDeterminism(t *testing.T) {
	t.Parallel()

	cfg := `
resource "talos_machine_secrets" "this" {}

ephemeral "talos_client_configuration" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = talos_machine_secrets.this.machine_secrets
	endpoints       = ["10.0.0.1"]
}

provider "echo" {
	data = {
		talos_config = ephemeral.talos_client_configuration.this.talos_config
	}
}

resource "echo" "test" {}
`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("talos_config"), knownvalue.NotNull()),
				},
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccTalosClientConfigurationEphemeralResourceDeterminismWithNotBefore tests that two
// opens with a terraform_data-sourced not_before produce byte-identical talos_config.
func TestAccTalosClientConfigurationEphemeralResourceDeterminismWithNotBefore(t *testing.T) {
	t.Parallel()

	cfg := `
resource "talos_machine_secrets" "this" {}

resource "terraform_data" "client_config_nbf" {
	input = "2024-01-01T00:00:00Z"
}

ephemeral "talos_client_configuration" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = talos_machine_secrets.this.machine_secrets
	not_before      = terraform_data.client_config_nbf.output
	crt_ttl         = "87600h"
}

provider "echo" {
	data = {
		talos_config = ephemeral.talos_client_configuration.this.talos_config
	}
}

resource "echo" "test" {}
`

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: cfg,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("talos_config"), knownvalue.NotNull()),
				},
			},
			{
				Config: cfg,
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
			},
		},
	})
}

// TestAccTalosClientConfigurationEphemeralResourceInvalidNotBefore tests that an invalid
// not_before is rejected with an error.
func TestAccTalosClientConfigurationEphemeralResourceInvalidNotBefore(t *testing.T) {
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
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	not_before      = "not-a-timestamp"
}

provider "echo" {
	data = {}
}

resource "echo" "test" {}
`,
				ExpectError: regexp.MustCompile(`invalid not_before`),
			},
		},
	})
}

// TestAccTalosClientConfigurationEphemeralResourceClientConfigurationOutput tests that
// client_configuration is populated with ca_certificate, client_certificate, and client_key.
func TestAccTalosClientConfigurationEphemeralResourceClientConfigurationOutput(t *testing.T) {
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
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
}

provider "echo" {
	data = {
		ca_certificate     = ephemeral.talos_client_configuration.this.client_configuration.ca_certificate
		client_certificate = ephemeral.talos_client_configuration.this.client_configuration.client_certificate
		client_key         = ephemeral.talos_client_configuration.this.client_configuration.client_key
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("ca_certificate"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("client_certificate"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("client_key"), knownvalue.NotNull()),
				},
			},
		},
	})
}
