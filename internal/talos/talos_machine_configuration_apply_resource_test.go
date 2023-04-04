// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosMachineConfigurationApplyResource(t *testing.T) {
	testDir, err := os.MkdirTemp("", "talos-machine-configuration-apply-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	if err := os.Chmod(testDir, 0o755); err != nil {
		t.Fatal(err)
	}

	isoPath := filepath.Join(testDir, "talos.iso")

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	cpuMode := "host-passthrough"
	if os.Getenv("CI") != "" {
		cpuMode = "host-model"
	}

	resource.ParallelTest(t, resource.TestCase{
		WorkingDir: testDir,
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source: "dmacvicar/libvirt",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				PreConfig: func() {
					if err := downloadTalosISO(testDir, isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccTalosMachineConfigurationApplyResourceConfig("talos", rName, cpuMode, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "id", "machine_configuration_apply"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "apply_mode", "auto"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "node"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "endpoint"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration_input"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "config_patches.#", "1"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "config_patches.0", "\"machine\":\n  \"install\":\n    \"disk\": \"/dev/vda\"\n"),
				),
			},
			// ensure there is no diff
			{
				PreConfig: func() {
					if err := downloadTalosISO(testDir, isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config:   testAccTalosMachineConfigurationApplyResourceConfig("talos", rName, cpuMode, isoPath),
				PlanOnly: true,
			},
		},
	})
}

func TestAccTalosMachineConfigurationApplyResourceUpgrade(t *testing.T) {
	testDir, err := os.MkdirTemp("", "talos-machine-configuration-apply-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(testDir)

	if err := os.Chmod(testDir, 0o755); err != nil {
		t.Fatal(err)
	}

	isoPath := filepath.Join(testDir, "talos.iso")

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	cpuMode := "host-passthrough"
	if os.Getenv("CI") != "" {
		cpuMode = "host-model"
	}

	resource.ParallelTest(t, resource.TestCase{
		WorkingDir: testDir,
		Steps: []resource.TestStep{
			// create TF config with v0.1.2 of the talos provider
			{
				PreConfig: func() {
					if err := downloadTalosISO(testDir, isoPath); err != nil {
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
				Config: testAccTalosMachineConfigurationApplyResourceConfigV0("talosv1", rName, cpuMode, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					testAccCheckResourceDisappears([]string{
						"talos_client_configuration.this",
						"talos_machine_configuration_controlplane.this",
					}),
				),
			},
			// now test state migration with the latest version of the provider
			{
				PreConfig: func() {
					if err := downloadTalosISO(testDir, isoPath); err != nil {
						t.Fatal(err)
					}
				},
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source: "dmacvicar/libvirt",
					},
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineConfigurationApplyResourceConfigV1("talos", rName, cpuMode, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "id", "machine_configuration_apply"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "apply_mode", "auto"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "node"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "endpoint"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration_input"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "config_patches.#", "1"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "config_patches.0", "\"machine\":\n  \"install\":\n    \"disk\": \"/dev/vda\"\n"),
				),
			},
			// ensure there is no diff
			{
				PreConfig: func() {
					if err := downloadTalosISO(testDir, isoPath); err != nil {
						t.Fatal(err)
					}
				},
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source: "dmacvicar/libvirt",
					},
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineConfigurationApplyResourceConfigV1("talos", rName, cpuMode, isoPath),
				PlanOnly:                 true,
			},
		},
	})
}

func testAccTalosMachineConfigurationApplyResourceConfig(providerName, rName, cpuMode, isoPath string) string {
	return testAccDynamicConfig(providerName, rName, cpuMode, isoPath, false, false)
}

func testAccTalosMachineConfigurationApplyResourceConfigV0(providerName, rName, cpuMode, isoPath string) string {
	return testAccDynamicConfig(providerName, rName, cpuMode, isoPath, false, false)
}

func testAccTalosMachineConfigurationApplyResourceConfigV1(providerName, rName, cpuMode, isoPath string) string {
	return testAccDynamicConfig(providerName, rName, cpuMode, isoPath, false, false)
}
