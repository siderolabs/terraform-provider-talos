// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccTalosMachineConfigurationApplyResource(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineConfigurationApplyResourceConfig("talos", rName),
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
				Config:   testAccTalosMachineConfigurationApplyResourceConfig("talos", rName),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachineConfigurationApplyResourceAutoStaged tests the "staged_if_needing_reboot" apply mode.
//
// Note on local vs CI environment:
// During local development, the node IP was sometimes unknown during the plan phase,
// preventing the dry-run from being performed. However, in CI, the libvirt setup
// allows the node IP to be known immediately, enabling the dry-run to execute.
// Since the configuration requires a reboot, the dry-run correctly resolves to
// "staged" mode to prevent uncontrolled reboots.
func TestAccTalosMachineConfigurationApplyResourceAutoStaged(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineConfigurationApplyResourceConfigWithAutoStaged("talos", rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "id", "machine_configuration_apply"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "apply_mode", "staged_if_needing_reboot"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "resolved_apply_mode", "staged"),
				),
			},
		},
	})
}

// logApplyModeState returns a TestCheckFunc that logs the apply_mode and resolved_apply_mode
// attributes of the staged_if_needing_reboot resource for debugging upgrade tests.
func logApplyModeState(t *testing.T, stepName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources["talos_machine_configuration_apply.staged_if_needing_reboot"]
		if !ok {
			t.Logf("[%s] Resource not found in state", stepName)

			return nil
		}

		t.Logf("[%s] apply_mode = %q", stepName, rs.Primary.Attributes["apply_mode"])

		resolvedApplyMode, exists := rs.Primary.Attributes["resolved_apply_mode"]

		switch {
		case !exists:
			t.Logf("[%s] resolved_apply_mode = <DOES NOT EXIST>", stepName)
		case resolvedApplyMode == "":
			t.Logf("[%s] resolved_apply_mode = <EMPTY STRING>", stepName)
		default:
			t.Logf("[%s] resolved_apply_mode = %q", stepName, resolvedApplyMode)
		}

		return nil
	}
}

// TestAccTalosMachineConfigurationApplyResourceUpgradeWithResolvedApplyModeBug tests the bug in v0.10.1.
//
// Bug scenario: v0.10.0 → v0.10.1
//   - v0.10.0: staged_if_needing_reboot and resolved_apply_mode don't exist.
//   - v0.10.1: add staged_if_needing_reboot, resolved_apply_mode appears but is EMPTY (this is the bug).
func TestAccTalosMachineConfigurationApplyResourceUpgradeWithResolvedApplyModeBug(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			// Step 1: v0.10.0 - staged_if_needing_reboot doesn't exist, use default apply_mode
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.10.0",
						Source:            "siderolabs/talos",
					},
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				Config: testAccTalosMachineConfigurationApplyResourceConfigAutoStagedUpgrade(rName, "auto"),
				Check: resource.ComposeAggregateTestCheckFunc(
					logApplyModeState(t, "v0.10.0 - baseline"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "apply_mode", "auto"),
				),
			},
			// Step 2: v0.10.1 - switch to staged_if_needing_reboot, resolved_apply_mode is EMPTY (bug)
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.10.1",
						Source:            "siderolabs/talos",
					},
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				Config: testAccTalosMachineConfigurationApplyResourceConfigAutoStagedUpgrade(rName, "staged_if_needing_reboot"),
				Check: resource.ComposeAggregateTestCheckFunc(
					logApplyModeState(t, "v0.10.1 - BUG: resolved_apply_mode is empty"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "apply_mode", "staged_if_needing_reboot"),
					// Bug: resolved_apply_mode is empty here because config didn't change
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "resolved_apply_mode", ""),
				),
			},
		},
	})
}

// TestAccTalosMachineConfigurationApplyResourceUpgradeWithResolvedApplyModeFix tests the fix for empty resolved_apply_mode.
//
// Fix scenario: v0.10.0 → current version
//   - v0.10.0: staged_if_needing_reboot and resolved_apply_mode don't exist.
//   - Current version: resolved_apply_mode is correctly computed as "auto".
func TestAccTalosMachineConfigurationApplyResourceUpgradeWithResolvedApplyModeFix(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			// Step 1: v0.10.0 - staged_if_needing_reboot doesn't exist, use default apply_mode
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.10.0",
						Source:            "siderolabs/talos",
					},
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				Config: testAccTalosMachineConfigurationApplyResourceConfigAutoStagedUpgrade(rName, "auto"),
				Check: resource.ComposeAggregateTestCheckFunc(
					logApplyModeState(t, "v0.10.0 - baseline"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "apply_mode", "auto"),
				),
			},
			// Step 2: Current version - switch to staged_if_needing_reboot, resolved_apply_mode is correctly computed
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineConfigurationApplyResourceConfigAutoStagedUpgrade(rName, "staged_if_needing_reboot"),
				Check: resource.ComposeAggregateTestCheckFunc(
					logApplyModeState(t, "current version - FIX: resolved_apply_mode is computed"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "apply_mode", "staged_if_needing_reboot"),
					// Fix: resolved_apply_mode should now be computed (not empty)
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "resolved_apply_mode", "auto"),
				),
			},
		},
	})
}

func TestAccTalosMachineConfigurationApplyResourceUpgrade(t *testing.T) {
	// ref: https://github.com/hashicorp/terraform-plugin-testing/pull/118
	t.Skip("skipping until TF test framework has a way to remove state resource")

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			// create TF config with v0.1.2 of the talos provider
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.1.2",
						Source:            "siderolabs/talos",
					},
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				Config: testAccTalosMachineConfigurationApplyResourceConfigV0("talosv1", rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckNoResourceAttr("talos_client_configuration", "this"),
					resource.TestCheckNoResourceAttr("talos_machine_configuration_controlplane", "this"),
				),
			},
			// now test state migration with the latest version of the provider
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineConfigurationApplyResourceConfigV1("talos", rName),
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
				ExternalProviders: map[string]resource.ExternalProvider{
					"libvirt": {
						Source:            "dmacvicar/libvirt",
						VersionConstraint: "= 0.8.3",
					},
				},
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineConfigurationApplyResourceConfigV1("talos", rName),
				PlanOnly:                 true,
			},
		},
	})
}

func testAccTalosMachineConfigurationApplyResourceConfig(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: true,
		WithBootstrap:   false,
	}

	return config.render()
}

func testAccTalosMachineConfigurationApplyResourceConfigV0(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: true,
		WithBootstrap:   false,
	}

	return config.render()
}

func testAccTalosMachineConfigurationApplyResourceConfigV1(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: true,
		WithBootstrap:   false,
	}

	return config.render()
}

func testAccTalosMachineConfigurationApplyResourceConfigWithAutoStaged(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: true,
		WithBootstrap:   false,
	}

	baseConfig := config.render()

	return baseConfig + `
resource "talos_machine_configuration_apply" "staged_if_needing_reboot" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = libvirt_domain.cp.network_interface[0].addresses[0]
  apply_mode                  = "staged_if_needing_reboot"
  config_patches = [
    yamlencode({
      machine = {
        files = [
          {
            path        = "/var/etc/example-config.yaml"
            permissions = 420  # 0644 in octal
            op          = "create"
            content     = "example: staged_if_needing_reboot test"
          }
        ]
      }
    }),
  ]
}
`
}

func testAccTalosMachineConfigurationApplyResourceConfigAutoStagedUpgrade(rName, applyMode string) string {
	config := dynamicConfig{
		Provider:        "talos",
		ResourceName:    rName,
		WithApplyConfig: false,
		WithBootstrap:   false,
	}

	baseConfig := config.render()

	return baseConfig + `
resource "talos_machine_configuration_apply" "staged_if_needing_reboot" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = libvirt_domain.cp.network_interface[0].addresses[0]
  apply_mode                  = "` + applyMode + `"
}
`
}
