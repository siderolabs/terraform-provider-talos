// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"os"
	"path/filepath"
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
				Config:             testAccTalosMachineBootstrapResourceConfigImport("10.5.0.2"),
				ResourceName:       "talos_machine_bootstrap.this",
				ImportStateId:      "this",
				ImportState:        true,
				ImportStatePersist: true,
			},
			// verify state is correct after import
			{
				Config: testAccTalosMachineBootstrapResourceConfigImport("10.5.0.2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_bootstrap.this", "id", "machine_bootstrap"),
					resource.TestCheckResourceAttr("talos_machine_bootstrap.this", "node", "10.5.0.2"),
					resource.TestCheckResourceAttr("talos_machine_bootstrap.this", "endpoint", "10.5.0.2"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "client_configuration.client_key"),
				),
			},
		},
	})
}

func TestAccTalosMachineBootstrapResourceUpgrade(t *testing.T) {
	// ref: https://github.com/hashicorp/terraform-plugin-testing/pull/118
	t.Skip("skipping until TF test framework has a way to remove state resource")

	testDir, err := os.MkdirTemp("", "talos-machine-bootstrap-resource-upgrade")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testDir) //nolint:errcheck

	if err := os.Chmod(testDir, 0o755); err != nil {
		t.Fatal(err)
	}

	isoPath := filepath.Join(testDir, "talos.iso")

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		WorkingDir: testDir,
		Steps: []resource.TestStep{
			// create TF config with v0.1.2 of the talos provider
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.1.2",
						Source:            "siderolabs/talos",
					},
					"libvirt": {
						Source: "dmacvicar/libvirt",
					},
				},
				Config: testAccTalosMachineBootstrapResourceConfigV0("talosv1", rName, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckResourceDisappears([]string{
						"talos_client_configuration.this",
						"talos_machine_configuration_controlplane.this",
						"talos_machine_configuration_apply.this",
					}),
				),
			},
			// now test state migration with the latest version of the provider
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source: "dmacvicar/libvirt",
					},
				},
				Config: testAccTalosMachineBootstrapResourceConfigV1("talos", rName, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_bootstrap.this", "id", "machine_bootstrap"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "node"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "endpoint"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_bootstrap.this", "client_configuration.client_key"),
				),
			},
			// ensure there is no diff
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source: "dmacvicar/libvirt",
					},
				},
				Config:   testAccTalosMachineBootstrapResourceConfigV1("talos", rName, isoPath),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosMachineBootstrapResourceConfigV0(providerName, rName, isoPath string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		IsoPath:         isoPath,
		WithApplyConfig: true,
		WithBootstrap:   true,
	}

	return config.render()
}

func testAccTalosMachineBootstrapResourceConfigV1(providerName, rName, isoPath string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		IsoPath:         isoPath,
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
