// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/echoprovider"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// testAccProtoV6ProviderFactoriesWithEcho includes both the talos provider and echo provider.
var testAccProtoV6ProviderFactoriesWithEcho = map[string]func() (tfprotov6.ProviderServer, error){
	providerName: providerserver.NewProtocol6WithError(talos.New()),
	resEcho:      echoprovider.NewProviderServer(),
}

// TestAccTalosMachineSecretsEphemeralResource tests that:
// 1. Ephemeral resource generates secrets
// 2. Secrets can be passed to other resources via echo provider
//
// Uses the Echo Provider to test values set in ephemeral resources
// see documentation here for more details:
// https://developer.hashicorp.com/terraform/plugin/testing/acceptance-tests/ephemeral-resources#using-echo-provider-in-acceptance-tests
func TestAccTalosMachineSecretsEphemeralResource(t *testing.T) {
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

provider "echo" {
	data = ephemeral.talos_machine_secrets.this
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					// Verify that the ephemeral resource provides client_configuration
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey(fieldClientConfiguration).AtMapKey(fieldCACertificate), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey(fieldClientConfiguration).AtMapKey(fieldClientCertificate), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey(fieldClientConfiguration).AtMapKey(fieldClientKey), knownvalue.NotNull()),

					// Verify that machine_secrets are populated
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey(fieldMachineSecrets).AtMapKey("cluster").AtMapKey("id"), knownvalue.NotNull()),
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey(fieldMachineSecrets).AtMapKey("cluster").AtMapKey("secret"), knownvalue.NotNull()),

					// Verify bootstrap token exists
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey(fieldMachineSecrets).AtMapKey("secrets").AtMapKey("bootstrap_token"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosMachineSecretsEphemeralResourceNotInState verifies that
// ephemeral resources are not persisted to state by the framework.
func TestAccTalosMachineSecretsEphemeralResourceNotInState(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: `ephemeral "talos_machine_secrets" "this" {}`,
			},
		},
	})
}

// TestAccTalosMachineSecretsEphemeralResourceWithDefault tests generation
// with default talos_version.
func TestAccTalosMachineSecretsEphemeralResourceWithDefault(t *testing.T) {
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

provider "echo" {
	data = {
		cluster_id = ephemeral.talos_machine_secrets.this.machine_secrets.cluster.id
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue(resEchoTest, tfjsonpath.New(echoDataKey).AtMapKey("cluster_id"), knownvalue.NotNull()),
				},
			},
		},
	})
}
