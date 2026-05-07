// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosMachineBootstrapResource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // import can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// import the resource
				Config:             testAccTalosMachineBootstrapResourceConfigImport(testIP1),
				ResourceName:       resTalosMachineBootstrap,
				ImportStateId:      resourceThis,
				ImportState:        true,
				ImportStatePersist: true,
			},
			// verify state is correct after import
			{
				Config: testAccTalosMachineBootstrapResourceConfigImport(testIP1),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resTalosMachineBootstrap, "id", "machine_bootstrap"),
					resource.TestCheckResourceAttr(resTalosMachineBootstrap, fieldNode, testIP1),
					resource.TestCheckResourceAttr(resTalosMachineBootstrap, fieldEndpoint, testIP1),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, attrClientConfigCACert),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, attrClientConfigClientCert),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, attrClientConfigClientKey),
				),
			},
		},
	})
}

func TestAccTalosMachineBootstrapResourceUpgrade(t *testing.T) {
	// ref: https://github.com/hashicorp/terraform-plugin-testing/pull/118
	t.Skip("skipping until TF test framework has a way to remove state resource")

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			// create TF config with v0.1.2 of the talos provider
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					providerName: {
						VersionConstraint: testVersionConstraint,
						Source:            testSiderolabsTalos,
					},
					libvirtProvider: {
						Source:            libvirtProviderSource,
						VersionConstraint: libvirtProviderVersionConstraint,
					},
				},
				Config: testAccTalosMachineBootstrapResourceConfigV0(tfTalosV1Provider, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr(fieldTalosClientConf, resourceThis),
					resource.TestCheckNoResourceAttr(tfMachineConfigCtrl, resourceThis),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply", "id", resourceThis),
				),
			},
			// now test state migration with the latest version of the provider
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				ExternalProviders: map[string]resource.ExternalProvider{
					libvirtProvider: {
						Source:            libvirtProviderSource,
						VersionConstraint: libvirtProviderVersionConstraint,
					},
				},
				Config: testAccTalosMachineBootstrapResourceConfigV1(providerName, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resTalosMachineBootstrap, "id", "machine_bootstrap"),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, fieldNode),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, fieldEndpoint),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, attrClientConfigCACert),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, attrClientConfigClientCert),
					resource.TestCheckResourceAttrSet(resTalosMachineBootstrap, attrClientConfigClientKey),
				),
			},
			// ensure there is no diff
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				ExternalProviders: map[string]resource.ExternalProvider{
					libvirtProvider: {
						Source:            libvirtProviderSource,
						VersionConstraint: libvirtProviderVersionConstraint,
					},
				},
				Config:   testAccTalosMachineBootstrapResourceConfigV1(providerName, rName),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosMachineBootstrapResourceConfigV0(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: true,
		WithBootstrap:   true,
	}

	return config.render()
}

func testAccTalosMachineBootstrapResourceConfigV1(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: true,
		WithBootstrap:   true,
	}

	return config.render()
}

func testAccTalosMachineBootstrapResourceConfigImport(node string) string {
	return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {}

resource "talos_machine_bootstrap" "this" {
  node                 = "%s"
  client_configuration = talos_machine_secrets.this.client_configuration
}
`, node)
}
