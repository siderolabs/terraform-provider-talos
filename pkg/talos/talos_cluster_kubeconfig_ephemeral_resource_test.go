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

// TestAccTalosClusterKubeconfigEphemeralResourceBasic tests that the ephemeral resource
// generates kubeconfig_raw and that kubernetes_client_configuration.host matches the endpoint.
func TestAccTalosClusterKubeconfigEphemeralResourceBasic(t *testing.T) {
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

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
}

provider "echo" {
	data = {
		kubeconfig_raw = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
		host           = ephemeral.talos_cluster_kubeconfig.this.kubernetes_client_configuration.host
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("kubeconfig_raw"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("host"), knownvalue.StringExact("https://10.0.0.1:6443")),
				},
			},
		},
	})
}

// TestAccTalosClusterKubeconfigEphemeralResourceKubernetesClientConfigurationFields tests
// that all kubernetes_client_configuration fields are populated.
func TestAccTalosClusterKubeconfigEphemeralResourceKubernetesClientConfigurationFields(t *testing.T) {
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

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
}

provider "echo" {
	data = {
		host               = ephemeral.talos_cluster_kubeconfig.this.kubernetes_client_configuration.host
		ca_certificate     = ephemeral.talos_cluster_kubeconfig.this.kubernetes_client_configuration.ca_certificate
		client_certificate = ephemeral.talos_cluster_kubeconfig.this.kubernetes_client_configuration.client_certificate
		client_key         = ephemeral.talos_cluster_kubeconfig.this.kubernetes_client_configuration.client_key
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("host"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("ca_certificate"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("client_certificate"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("client_key"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosClusterKubeconfigEphemeralResourceDeterminism tests that two opens with
// identical inputs (CA-pinned, no not_before) produce byte-identical kubeconfig_raw.
func TestAccTalosClusterKubeconfigEphemeralResourceDeterminism(t *testing.T) {
	t.Parallel()

	cfg := `
resource "talos_machine_secrets" "this" {}

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
}

provider "echo" {
	data = {
		kubeconfig_raw = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
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
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("kubeconfig_raw"), knownvalue.NotNull()),
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

// TestAccTalosClusterKubeconfigEphemeralResourceDeterminismWithNotBefore tests that two
// opens with a terraform_data-sourced not_before produce byte-identical kubeconfig_raw.
func TestAccTalosClusterKubeconfigEphemeralResourceDeterminismWithNotBefore(t *testing.T) {
	t.Parallel()

	cfg := `
resource "talos_machine_secrets" "this" {}

resource "terraform_data" "kubeconfig_nbf" {
	input = "2024-01-01T00:00:00Z"
}

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
	not_before      = terraform_data.kubeconfig_nbf.output
	crt_ttl         = "87600h"
}

provider "echo" {
	data = {
		kubeconfig_raw = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
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
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("kubeconfig_raw"), knownvalue.NotNull()),
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

// TestAccTalosClusterKubeconfigEphemeralResourceCustomTTL tests that not_before + crt_ttl
// are accepted and the resource opens without error.
func TestAccTalosClusterKubeconfigEphemeralResourceCustomTTL(t *testing.T) {
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

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
	not_before      = "2024-01-01T00:00:00Z"
	crt_ttl         = "8760h"
}

provider "echo" {
	data = {
		kubeconfig_raw = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("kubeconfig_raw"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosClusterKubeconfigEphemeralResourceInvalidTTL tests that an invalid crt_ttl
// is rejected with an error.
func TestAccTalosClusterKubeconfigEphemeralResourceInvalidTTL(t *testing.T) {
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

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
	not_before      = "2024-01-01T00:00:00Z"
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

// TestAccTalosClusterKubeconfigEphemeralResourceInvalidNotBefore tests that an invalid
// not_before is rejected with an error.
func TestAccTalosClusterKubeconfigEphemeralResourceInvalidNotBefore(t *testing.T) {
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

ephemeral "talos_cluster_kubeconfig" "this" {
	cluster_name    = "test-cluster"
	machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
	endpoint        = "https://10.0.0.1:6443"
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
