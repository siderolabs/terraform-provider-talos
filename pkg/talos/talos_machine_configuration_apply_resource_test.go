// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
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
//   - Current version: resolved_apply_mode is correctly computed (not empty).
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
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.staged_if_needing_reboot", "resolved_apply_mode"),
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
		Provider:               providerName,
		ResourceName:           rName,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
		WithClusterHealth:      true,
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
  depends_on = [data.talos_cluster_health.this]
}
`
}

// TestAccTalosMachineConfigurationApplyWithEphemeralClientConfigWO tests write-only attributes
// with ephemeral resources.
//
// This test uses ephemeral talos_machine_secrets and talos_machine_configuration WITHOUT
// persistence (not recommended for production - see docs/guides/using_ephemeral_resources.md).
// Secrets regenerate on each Open, so the rendered machine configuration — and therefore
// machine_configuration_hash — differs between plans. ExpectNonEmptyPlan is true to reflect
// this documented anti-pattern; production usage should persist secrets in a secret manager,
// which keeps the hash stable across runs.
//
// The test validates:
// - Write-only attributes work correctly with ephemeral inputs
// - Resource creation succeeds with ephemeral values
// - Write-only attributes are not stored in state
// - machine_configuration_hash IS populated in state (hash fingerprint, not a secret)
// - Hash drift surfaces when non-persisted ephemeral secrets regenerate (correct behavior
//   that was previously hidden by the write-only invisibility to state)
func TestAccTalosMachineConfigurationApplyWithEphemeralClientConfigWO(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineConfigurationApplyWithEphemeralClientConfigWOConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "id", "machine_configuration_apply"),
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.this", "apply_mode", "auto"),
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "node"),
					// machine_configuration should NOT be in state when using write-only inputs
					resource.TestCheckNoResourceAttr("talos_machine_configuration_apply.this", "machine_configuration"),
					// machine_configuration_hash IS in state — it's a SHA256 fingerprint, not a secret
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration_hash"),
					// client_configuration_wo should not be in state (write-only)
					resource.TestCheckNoResourceAttr("talos_machine_configuration_apply.this", "client_configuration_wo"),
					// machine_configuration_input_wo should not be in state (write-only)
					resource.TestCheckNoResourceAttr("talos_machine_configuration_apply.this", "machine_configuration_input_wo"),
					// client_configuration should not be set (using WO variant)
					resource.TestCheckNoResourceAttr("talos_machine_configuration_apply.this", "client_configuration"),
					// machine_configuration_input should not be set (using WO variant)
					resource.TestCheckNoResourceAttr("talos_machine_configuration_apply.this", "machine_configuration_input"),
				),
				// Drift on non-persisted ephemeral secrets: each Open regenerates secrets,
				// which changes the rendered machine configuration, which changes the hash.
				// This is the correct behavior for this anti-pattern; persist secrets in
				// production and the hash stays stable.
				ExpectNonEmptyPlan: true,
			},
		},
	})
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

func testAccTalosMachineConfigurationApplyWithEphemeralClientConfigWOConfig(rName string) string {
	cpuMode := "host-passthrough"
	if os.Getenv("CI") != "" {
		cpuMode = "host-model"
	}

	isoURL := fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso", gendata.VersionTag)

	return fmt.Sprintf(`
# Generate ephemeral machine secrets (NOT persisted - causes expected drift)
# In production, these should be persisted in a secret manager as documented
ephemeral "talos_machine_secrets" "this" {}

# Generate ephemeral machine configuration
ephemeral "talos_machine_configuration" "this" {
  cluster_name       = "test-cluster"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "controlplane"
  machine_secrets    = ephemeral.talos_machine_secrets.this.machine_secrets
  talos_version      = "%[3]s"
  kubernetes_version = "1.32.2"

  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk = "/dev/vda"
        }
      }
    })
  ]
}

# Create libvirt VM
resource "libvirt_volume" "cp" {
  name = "%[1]s"
  size = 6442450944
}

resource "libvirt_domain" "cp" {
  name     = "%[1]s"
  firmware = "/usr/share/OVMF/OVMF_CODE.fd"
  nvram {
    file = "/var/lib/libvirt/qemu/nvram/%[1]s_VARS.fd"
    template = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  }

  lifecycle {
    ignore_changes = [
      cpu,
      nvram,
      disk["url"],
	  firmware,
    ]
  }

  cpu {
    mode = "%[2]s"
  }

  console {
    type        = "pty"
    target_port = "0"
  }

  graphics {
    type        = "vnc"
    listen_type = "address"
  }

  disk {
    url = "%[4]s"
  }

  disk {
    volume_id = libvirt_volume.cp.id
  }

  boot_device {
    dev = ["cdrom"]
  }

  network_interface {
    network_name   = "default"
    wait_for_lease = true
  }

  vcpu   = "2"
  memory = "4096"
}

# Apply configuration using write-only ephemeral attributes
# This tests the actual use case: ephemeral inputs -> write-only attributes -> no secrets in state
resource "talos_machine_configuration_apply" "this" {
  client_configuration_wo        = ephemeral.talos_machine_secrets.this.client_configuration
  machine_configuration_input_wo = ephemeral.talos_machine_configuration.this.machine_configuration
  node                           = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint                       = libvirt_domain.cp.network_interface[0].addresses[0]
}
`, rName, cpuMode, gendata.VersionTag, isoURL)
}

// TestAccTalosMachineConfigurationApplyDetectsEphemeralInputChange verifies that when
// the ephemeral talos_machine_configuration's rendered output changes between plans —
// which is how a talos_version bump or any patch edit propagates in real workflows —
// the apply resource surfaces the change as a plan diff.
//
// Before the fix, machine_configuration_input_wo is write-only (not persisted) and
// setPlanMachineConfiguration explicitly nulls the computed machine_configuration when
// WO inputs are used, so changes are invisible to state and the plan is empty. The
// fix is to persist a content fingerprint (machine_configuration_hash) that differs
// when the rendered config differs, regardless of whether the source was write-only.
//
// A persistent talos_machine_secrets resource is used so the generated config is
// deterministic between plans — the only delta is the disk path.
func TestAccTalosMachineConfigurationApplyDetectsEphemeralInputChange(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineConfigurationApplyDetectsEphemeralInputChangeConfig(rName, "/dev/vda"),
			},
			{
				Config:             testAccTalosMachineConfigurationApplyDetectsEphemeralInputChangeConfig(rName, "/dev/vdb"),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccTalosMachineConfigurationApplyDetectsEphemeralInputChangeConfig(rName, disk string) string {
	cpuMode := "host-passthrough"
	if os.Getenv("CI") != "" {
		cpuMode = "host-model"
	}

	isoURL := fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso", gendata.VersionTag)

	return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {}

ephemeral "talos_machine_configuration" "this" {
  cluster_name       = "test-cluster"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "controlplane"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  talos_version      = "%[3]s"
  kubernetes_version = "1.32.2"

  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk = "%[5]s"
        }
      }
    })
  ]
}

resource "libvirt_volume" "cp" {
  name = "%[1]s"
  size = 6442450944
}

resource "libvirt_domain" "cp" {
  name     = "%[1]s"
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"
  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s_VARS.fd"
    template = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  }

  lifecycle {
    ignore_changes = [
      cpu,
      nvram,
      disk["url"],
    ]
  }

  cpu {
    mode = "%[2]s"
  }

  console {
    type        = "pty"
    target_port = "0"
  }

  graphics {
    type        = "vnc"
    listen_type = "address"
  }

  disk {
    url = "%[4]s"
  }

  disk {
    volume_id = libvirt_volume.cp.id
  }

  boot_device {
    dev = ["cdrom"]
  }

  network_interface {
    network_name   = "default"
    wait_for_lease = true
  }

  vcpu   = "2"
  memory = "4096"
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration_wo        = talos_machine_secrets.this.client_configuration
  machine_configuration_input_wo = ephemeral.talos_machine_configuration.this.machine_configuration
  node                           = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint                       = libvirt_domain.cp.network_interface[0].addresses[0]
}
`, rName, cpuMode, gendata.VersionTag, isoURL, disk)
}
