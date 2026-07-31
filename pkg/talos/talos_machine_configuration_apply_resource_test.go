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
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
)

// TestAccTalosMachineConfigurationApplyResource applies machine configuration, checks all
// attributes (including the disk data source that is always part of the config), verifies
// idempotency, then exercises the regression for issue #352 (unknown machine_configuration_hash
// in plan when machine_configuration_input is unknown) on the same cluster.
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
			// Step 1: initial apply — check config apply attributes and disk data source.
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
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration_hash"),
					// disk data source assertions (always rendered by dynamicConfig)
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "id", "machine_disks"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "node"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "endpoint"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "selector", "disk.size > 6u * GB"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "disks.#", "1"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "disks.0.dev_path", "/dev/vda"),
				),
			},
			// Step 2: ensure there is no diff.
			{
				Config:   testAccTalosMachineConfigurationApplyResourceConfig("talos", rName),
				PlanOnly: true,
			},
			// Step 3: regression for issue #352 — machine_configuration_input is unknown during
			// planning (terraform_data.trigger is new, so its output is unknown until applied).
			// Before the fix: machine_configuration_hash stays at H1 (UseStateForUnknown) but
			// is recomputed to H2 during apply — OpenTofu rejects with "inconsistent final plan".
			// After the fix: machine_configuration_hash is marked unknown in the plan and
			// resolves to H2 after apply.
			{
				Config: testAccTalosMachineConfigurationApplyResourceConfigWithUnknownInput("talos", rName),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectUnknownValue(
							"talos_machine_configuration_apply.this",
							tfjsonpath.New("machine_configuration_hash"),
						),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine_configuration_apply.this", "machine_configuration_hash"),
				),
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
//
// Note on Talos 1.14+: the ApplyConfiguration RPC no longer returns the reboot
// requirement in the response Mode field (CanApplyImmediate was removed from the
// AUTO mode handler). On 1.14+ nodes, resolved_apply_mode falls back to "auto".
// This is a known server-side limitation tracked upstream.
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
					// Talos ≤1.13: server detects reboot requirement → "staged".
					// Talos 1.14+: server no longer reports reboot requirement → "auto".
					resource.TestCheckResourceAttrWith("talos_machine_configuration_apply.staged_if_needing_reboot", "resolved_apply_mode", func(value string) error {
						if value != "staged" && value != "auto" {
							return fmt.Errorf("expected resolved_apply_mode to be 'staged' or 'auto', got %q", value)
						}

						return nil
					}),
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
//
// TestAccTalosMachineConfigurationApplyResourceUpgradeWithResolvedApplyMode exercises
// the full upgrade path for resolved_apply_mode on a single cluster:
//
//   - v0.10.0: staged_if_needing_reboot and resolved_apply_mode don't exist (baseline).
//   - v0.10.1: resolved_apply_mode is introduced but EMPTY when config didn't change (bug).
//   - current: resolved_apply_mode is correctly computed after upgrade (fix).
func TestAccTalosMachineConfigurationApplyResourceUpgradeWithResolvedApplyMode(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		Steps: []resource.TestStep{
			// Step 1: v0.10.0 baseline — staged_if_needing_reboot doesn't exist, use default apply_mode.
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
			// Step 2: v0.10.1 — resolved_apply_mode introduced but EMPTY (bug: config didn't change).
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
					// Bug: resolved_apply_mode is empty here because config didn't change.
					resource.TestCheckResourceAttr("talos_machine_configuration_apply.staged_if_needing_reboot", "resolved_apply_mode", ""),
				),
			},
			// Step 3: current version — resolved_apply_mode is correctly computed (fix).
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
					// Fix: resolved_apply_mode is now correctly computed (not empty).
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

// testAccTalosMachineConfigurationApplyResourceConfigWithUnknownInput returns a config
// where machine_configuration_input is unknown during planning. terraform_data.trigger
// is a new resource (not in state), so its output is unknown until applied. Its input
// holds a v2 machine configuration (different cluster_name) so the resolved hash H2
// differs from the H1 stored in state, ensuring the inconsistency would be observable.
func testAccTalosMachineConfigurationApplyResourceConfigWithUnknownInput(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: false,
		WithBootstrap:   false,
	}

	return config.render() + `
data "talos_machine_configuration" "v2" {
  cluster_name     = "example-cluster-v2"
  cluster_endpoint = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type     = "controlplane"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
  docs             = false
  examples         = false
}

resource "terraform_data" "trigger" {
  input = data.talos_machine_configuration.v2.machine_configuration
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = terraform_data.trigger.output
  node                        = libvirt_domain.cp.network_interface[0].addresses[0]
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk = data.talos_machine_disks.this.disks[0].dev_path
        }
      }
    }),
  ]
}
`
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
//   - Write-only attributes work correctly with ephemeral inputs
//   - Resource creation succeeds with ephemeral values
//   - Write-only attributes are not stored in state
//   - machine_configuration_hash IS populated in state (hash fingerprint, not a secret)
//   - Hash drift surfaces when non-persisted ephemeral secrets regenerate (correct behavior
//     that was previously hidden by the write-only invisibility to state)
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
	cpuMode := cpuModeHostPassthrough
	if os.Getenv("CI") != "" {
		cpuMode = cpuModeHostModel
	}

	isoURL := talosISOURL(gendata.VersionTag)

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

// TestAccTalosMachineConfigurationApplyConfigPatchesUnknownList is a regression test for
// a model-type bug: config_patches is declared as []types.String in the resource model,
// which cannot hold a whole-list-unknown value. This surfaces when config_patches is
// derived from a for-expression over an unknown value (e.g. a data source response body),
// producing:
//
//	Received unknown value, however the target type cannot handle unknown values.
//	Path: config_patches
//	Target Type: []basetypes.StringValue
//	Suggested Type: basetypes.ListValue
//
// The test uses terraform_data (a built-in resource) whose output attribute is unknown
// until apply; feeding it through jsondecode + for produces a whole-list-unknown
// config_patches during plan, which reproduces the failure without requiring libvirt.
func TestAccTalosMachineConfigurationApplyConfigPatchesUnknownList(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             testAccTalosMachineConfigurationApplyConfigPatchesUnknownListConfig(),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccTalosMachineConfigurationApplyConfigPatchesUnknownListConfig() string {
	return `
resource "terraform_data" "source" {
  input = "[\"a\",\"b\"]"
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration = {
    ca_certificate     = "fake-ca"
    client_certificate = "fake-cert"
    client_key         = "fake-key"
  }
  machine_configuration_input = "version: v1alpha1\nmachine:\n  type: controlplane\n"
  node                        = "127.0.0.1"
  endpoint                    = "127.0.0.1"
  config_patches = [
    for item in jsondecode(terraform_data.source.output) : yamlencode({
      machine = { install = { disk = "/dev/${item}" } }
    })
  ]
}
`
}

// TestAccTalosMachineConfigurationApplyOnDestroyUnknownBool is a regression test for
// a model-type bug: on_destroy fields (graceful, reboot, reset) were declared as plain
// bool in the resource model, which cannot hold unknown values. This surfaces when any
// field is derived from an expression over an unknown value (e.g. a ternary on a
// terraform_data output), producing:
//
//	Received unknown value, however the target type cannot handle unknown values.
//	Path: on_destroy.graceful
//	Target Type: bool
//	Suggested Type: basetypes.BoolValue
//
// The test feeds terraform_data.flag.output (unknown at plan time) through a
// conditional into on_destroy.graceful, reproducing the failure without a real node.
func TestAccTalosMachineConfigurationApplyOnDestroyUnknownBool(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:             testAccTalosMachineConfigurationApplyOnDestroyUnknownBoolConfig(),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccTalosMachineConfigurationApplyOnDestroyUnknownBoolConfig() string {
	return `
resource "terraform_data" "flag" {
  input = "false"
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration = {
    ca_certificate     = "fake-ca"
    client_certificate = "fake-cert"
    client_key         = "fake-key"
  }
  machine_configuration_input = "version: v1alpha1\nmachine:\n  type: controlplane\n"
  node                        = "127.0.0.1"
  endpoint                    = "127.0.0.1"
  on_destroy = {
    reset    = true
    graceful = terraform_data.flag.output == "true" ? true : false
    reboot   = false
  }
}
`
}
