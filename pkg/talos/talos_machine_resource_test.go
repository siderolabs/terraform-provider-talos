// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	frameworkpath "github.com/hashicorp/terraform-plugin-framework/path"
	frameworkresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// TestAccTalosMachine_bootstrap applies machine configuration via talos_machine,
// bootstraps etcd, waits for cluster health, and confirms idempotency.
func TestAccTalosMachine_bootstrap(t *testing.T) {
	const (
		baseImage = "ghcr.io/siderolabs/installer"
	)

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
				Config: testAccTalosMachineConfig(rName, baseImage, gendata.VersionTag, gendata.VersionTag),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine.this", "id"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "node"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "image"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// second apply must produce an empty plan
			{
				Config:   testAccTalosMachineConfig(rName, baseImage, gendata.VersionTag, gendata.VersionTag),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachine_drainWorkerUpgrade verifies that drain_on_upgrade = true works
// on worker nodes when kubeconfig_wo is provided. Workers do not serve the Talos
// kubeconfig API, so the provider must use the supplied kubeconfig to cordon and drain.
// Uses Talos v1.13.x so the LifecycleService path (pull → install → drain → reboot →
// uncordon) is exercised; the legacy path (< v1.13) silently skips drain.
func TestAccTalosMachine_drainWorkerUpgrade(t *testing.T) {
	const (
		baseVersion    = "v1.13.0-rc.0"
		upgradeVersion = "v1.13.0"
	)

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
			// Step 1: CP + worker, cluster bootstrapped and healthy.
			{
				Config: testAccTalosMachineWorkerDrainConfig(rName, baseVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine.cp", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("talos_machine.worker", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 2: upgrade the worker with drain_on_upgrade = true.
			// The worker must be drained via the Kubernetes API before rebooting.
			{
				Config: testAccTalosMachineWorkerDrainConfig(rName, baseVersion, upgradeVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.worker", "image",
						fmt.Sprintf("ghcr.io/siderolabs/installer:%s", upgradeVersion)),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
		},
	})
}

// testAccTalosMachineWorkerDrainConfig creates a CP + worker cluster where the
// worker has drain_on_upgrade = true. cpImageTag is the Talos version for the CP;
// workerImageTag is the target image for the worker (bumped in step 2 to trigger upgrade).
func testAccTalosMachineWorkerDrainConfig(rName, cpImageTag, workerImageTag string) string {
	cpuMode := cpuModeDefault
	if os.Getenv("CI") != "" {
		cpuMode = cpuModeCI
	}

	// Both CP and worker boot from the same base ISO. The worker upgrade target
	// is controlled via talos_machine.image, not the ISO URL.
	isoURL := fmt.Sprintf(
		"https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso",
		cpImageTag,
	)

	return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "cp" {
  cluster_name       = "test"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "controlplane"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  talos_version      = %[4]q
  kubernetes_version = "v1.35.3"
  docs               = false
  examples           = false
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/vda"
          image = "ghcr.io/siderolabs/installer:%[4]s"
        }
      }
    })
  ]
}

data "talos_machine_configuration" "worker" {
  cluster_name       = "test"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "worker"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  talos_version      = %[4]q
  kubernetes_version = "v1.35.3"
  docs               = false
  examples           = false
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/vda"
          image = "ghcr.io/siderolabs/installer:%[4]s"
        }
      }
    })
  ]
}

resource "libvirt_volume" "cp" {
  name = "%[1]s-cp"
  size = 6442450944
}

resource "libvirt_domain" "cp" {
  name     = "%[1]s-cp"
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s-cp_VARS.fd"
    template = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  }

  lifecycle {
    ignore_changes = [cpu, nvram, disk["url"]]
  }

  cpu {
    mode = %[2]q
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
    url = %[3]q
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

resource "libvirt_volume" "worker" {
  name = "%[1]s-worker"
  size = 6442450944
}

resource "libvirt_domain" "worker" {
  name     = "%[1]s-worker"
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s-worker_VARS.fd"
    template = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  }

  lifecycle {
    ignore_changes = [cpu, nvram, disk["url"]]
  }

  cpu { mode = %[2]q }

  console {
    type        = "pty"
    target_port = "0"
  }

  graphics {
    type        = "vnc"
    listen_type = "address"
  }

  disk {
    url = %[3]q
  }

  disk {
    volume_id = libvirt_volume.worker.id
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

resource "talos_machine" "cp" {
  node                  = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.cp.machine_configuration
  image                 = "ghcr.io/siderolabs/installer:%[4]s"
  drain_on_upgrade      = false

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}

resource "talos_machine_bootstrap" "this" {
  depends_on           = [talos_machine.cp]
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration = talos_machine_secrets.this.client_configuration
}

ephemeral "talos_cluster_kubeconfig" "this" {
  machine_secrets = talos_machine_secrets.this.machine_secrets
  cluster_name    = "test"
  endpoint        = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
}

resource "talos_machine" "worker" {
  depends_on            = [talos_machine_bootstrap.this]
  node                  = libvirt_domain.worker.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.worker.network_interface[0].addresses[0]
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.worker.machine_configuration
  image                 = "ghcr.io/siderolabs/installer:%[5]s"
  drain_on_upgrade      = true
  kubeconfig_wo         = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}

data "talos_cluster_health" "this" {
  depends_on = [talos_machine.worker]

  client_configuration = talos_machine_secrets.this.client_configuration
  endpoints            = libvirt_domain.cp.network_interface[0].addresses
  control_plane_nodes  = libvirt_domain.cp.network_interface[0].addresses
  worker_nodes         = libvirt_domain.worker.network_interface[0].addresses

  timeouts = { read = "25m" }
}
`, rName, cpuMode, isoURL, cpImageTag, workerImageTag)
}

// TestAccTalosMachine_upgrade tests that changing `image` triggers an OS upgrade:
// the node is initially at v1.12.7 and is upgraded to v1.13.0.
//
//nolint:dupl
func TestAccTalosMachine_upgrade(t *testing.T) {
	const (
		baseImage      = "ghcr.io/siderolabs/installer"
		baseVersion    = "v1.12.7"
		upgradeVersion = "v1.13.0"
	)

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
			// Step 1: node at base version, cluster bootstrapped and healthy
			{
				Config: testAccTalosMachineConfig(rName, baseImage, baseVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("%s:%s", baseImage, baseVersion)),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 2: upgrade to v1.13.0, cluster still healthy afterwards
			{
				Config: testAccTalosMachineConfig(rName, baseImage, upgradeVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("%s:%s", baseImage, upgradeVersion)),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 3: idempotency after upgrade
			{
				Config:   testAccTalosMachineConfig(rName, baseImage, upgradeVersion, baseVersion),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachine_upgrade tests that changing image schematic triggers an OS upgrade.
// It will use the same version v1.13.0 before and after upgrade.
//
//nolint:dupl
func TestAccTalosMachine_upgradeSchematic(t *testing.T) {
	const (
		talosVersion = "v1.13.0"
		baseImage    = "ghcr.io/siderolabs/installer"
		upgradeImage = "factory.talos.dev/metal-installer/c9078f9419961640c712a8bf2bb9174933dfcf1da383fd8ea2b7dc21493f8bac"
	)

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
			// Step 1: node at base version, cluster bootstrapped and healthy
			{
				Config: testAccTalosMachineConfig(rName, baseImage, talosVersion, talosVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("%s:%s", baseImage, talosVersion)),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 2: change the image schematic, cluster still healthy afterwards
			{
				Config: testAccTalosMachineConfig(rName, upgradeImage, talosVersion, talosVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("%s:%s", upgradeImage, talosVersion)),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 3: idempotency after upgrade
			{
				Config:   testAccTalosMachineConfig(rName, upgradeImage, talosVersion, talosVersion),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachine_upgradeLifecycle tests the LifecycleService upgrade path (Talos ≥ v1.13):
// the node boots at v1.13.0-rc.0 and is upgraded to v1.13.0 via ImageClient.Pull + LifecycleService.Upgrade.
//
//nolint:dupl
func TestAccTalosMachine_upgradeLifecycle(t *testing.T) {
	const (
		baseImage      = "ghcr.io/siderolabs/installer"
		baseVersion    = "v1.13.0-rc.0"
		upgradeVersion = "v1.13.0"
	)

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
			// Step 1: node at v1.13.0-rc.0, cluster bootstrapped and healthy
			{
				Config: testAccTalosMachineConfig(rName, baseImage, baseVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("%s:%s", baseImage, baseVersion)),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 2: upgrade to v1.13.0 via LifecycleService (new path), cluster still healthy
			{
				Config: testAccTalosMachineConfig(rName, baseImage, upgradeVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("%s:%s", baseImage, upgradeVersion)),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 3: idempotency after upgrade
			{
				Config:   testAccTalosMachineConfig(rName, baseImage, upgradeVersion, baseVersion),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachine_bootstrapWithWriteOnlyClientConfig verifies that talos_machine
// creates successfully when client_configuration_wo is used instead of client_configuration,
// and that client_configuration is absent from state after apply.
// Without the fix in Create(), OpenTofu would reject the apply with
// "provider produced inconsistent result after apply" because the plan said
// client_configuration = null but the provider returned a non-null value.
func TestAccTalosMachine_bootstrapWithWriteOnlyClientConfig(t *testing.T) {
	const (
		baseImage = "ghcr.io/siderolabs/installer"
	)

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
				Config: testAccTalosMachineConfigWithWriteOnlyAttrs(rName, baseImage, gendata.VersionTag, gendata.VersionTag),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine.this", "id"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "node"),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					// write-only variants used — client_configuration must be absent from state
					resource.TestCheckNoResourceAttr("talos_machine.this", "client_configuration.ca_certificate"),
					resource.TestCheckNoResourceAttr("talos_machine.this", "client_configuration.client_certificate"),
					resource.TestCheckNoResourceAttr("talos_machine.this", "client_configuration.client_key"),
					// machine_configuration_wo used — machine_configuration must be absent from state
					resource.TestCheckNoResourceAttr("talos_machine.this", "machine_configuration"),
				),
				// Ephemeral secrets regenerate on each Open, so the machine_configuration_hash
				// will differ on the next plan — expected drift for this anti-pattern.
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// TestUpdate_ClientConfigWO_RemainsNullInState is a red-phase test for issue #355.
// It proves that Update() unconditionally sets plan.ClientConfiguration (the non-write-only
// variant) even when client_configuration_wo was used, causing Terraform to reject the
// apply with "inconsistent values for sensitive attribute".
//
// The test bypasses network calls by setting image and machine_configuration_hash to the
// same values in plan and state — both imageChanged and configChanged are false, so the
// provider writes state directly without any RPC. After the fix the test passes; without
// the fix it fails because client_configuration is non-null in the returned state.
func TestUpdate_ClientConfigWO_RemainsNullInState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := talos.NewTalosMachineResource()

	var schemaResp frameworkresource.SchemaResponse

	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

	sch := schemaResp.Schema

	schTFType, ok := sch.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema TerraformType is not tftypes.Object")
	}

	nullVals := func() map[string]tftypes.Value {
		m := make(map[string]tftypes.Value, len(schTFType.AttributeTypes))
		for name, typ := range schTFType.AttributeTypes {
			m[name] = tftypes.NewValue(typ, nil)
		}

		return m
	}

	// talosClientTFConfigToTalosClientConfig only base64-decodes and stores bytes;
	// any valid base64 string is accepted without certificate validation.
	fakeB64 := base64.StdEncoding.EncodeToString([]byte("fake-cred"))

	ccWOTFType, ok := schTFType.AttributeTypes["client_configuration_wo"].(tftypes.Object)
	if !ok {
		t.Fatal("client_configuration_wo is not tftypes.Object in schema")
	}

	ccWOVal := tftypes.NewValue(ccWOTFType, map[string]tftypes.Value{
		"ca_certificate":     tftypes.NewValue(tftypes.String, fakeB64),
		"client_certificate": tftypes.NewValue(tftypes.String, fakeB64),
		"client_key":         tftypes.NewValue(tftypes.String, fakeB64),
	})

	const image = "ghcr.io/siderolabs/installer:v1.12.0"

	sum := sha256.Sum256([]byte("machine: {}"))
	hash := hex.EncodeToString(sum[:])

	// State from a previous Create that used client_configuration_wo:
	// client_configuration is null, image and hash are known.
	sv := nullVals()
	sv["id"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	sv["node"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	sv["endpoint"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	sv["image"] = tftypes.NewValue(tftypes.String, image)
	sv["machine_configuration_hash"] = tftypes.NewValue(tftypes.String, hash)
	stateRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, sv)

	// Plan: identical image and hash so imageChanged=false and configChanged=false,
	// meaning Update() makes no network calls and proceeds directly to state.Set.
	// Write-only attrs are null in the plan — Terraform passes them via Config only.
	pv := nullVals()
	pv["id"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	pv["node"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	pv["endpoint"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	pv["image"] = tftypes.NewValue(tftypes.String, image)
	pv["machine_configuration_hash"] = tftypes.NewValue(tftypes.String, hash)
	planRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, pv)

	// Config carries the write-only credentials that the plan does not expose.
	cv := nullVals()
	cv["id"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	cv["node"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	cv["endpoint"] = tftypes.NewValue(tftypes.String, "10.0.0.1")
	cv["image"] = tftypes.NewValue(tftypes.String, image)
	cv["machine_configuration_hash"] = tftypes.NewValue(tftypes.String, hash)
	cv["client_configuration_wo"] = ccWOVal
	configRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, cv)

	planObj := tfsdk.Plan{Schema: sch, Raw: planRaw}
	stateObj := tfsdk.State{Schema: sch, Raw: stateRaw}
	configObj := tfsdk.Config{Schema: sch, Raw: configRaw}

	req := frameworkresource.UpdateRequest{
		Plan:   planObj,
		State:  stateObj,
		Config: configObj,
	}
	resp := frameworkresource.UpdateResponse{State: tfsdk.State{Schema: sch, Raw: stateRaw}}

	r.Update(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("Update() returned unexpected diagnostics: %v", resp.Diagnostics)
	}

	var cc basetypes.ObjectValue
	if diags := resp.State.GetAttribute(ctx, frameworkpath.Root("client_configuration"), &cc); diags.HasError() {
		t.Fatalf("GetAttribute(client_configuration): %v", diags)
	}

	if !cc.IsNull() {
		t.Error("client_configuration must remain null in state when client_configuration_wo is used; Update leaked credentials into state (issue #355)")
	}
}

// testAccTalosMachineConfig generates HCL for a talos_machine resource backed by a
// libvirt VM, with etcd bootstrap and cluster health check always included.
// imageTag is the desired Talos installer image version.
// isoVersion is the Talos version of the ISO used to boot the libvirt VM.
const (
	cpuModeDefault = "host-passthrough"
	cpuModeCI      = "host-model"
)

func testAccTalosMachineConfig(rName, imageUrl, imageTag, isoVersion string) string {
	cpuMode := cpuModeDefault
	if os.Getenv("CI") != "" {
		cpuMode = cpuModeCI
	}

	isoURL := fmt.Sprintf(
		"https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso",
		isoVersion,
	)

	return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name       = "test"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "controlplane"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  talos_version      = %[5]q
  kubernetes_version = "v1.35.3"
  docs               = false
  examples           = false
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/vda"
          image = "%[4]s:%[6]s"
        }
      }
    })
  ]
}

resource "libvirt_volume" "cp" {
  name = %[1]q
  size = 6442450944
}

resource "libvirt_domain" "cp" {
  name     = %[1]q
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s_VARS.fd"
    template = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  }

  lifecycle {
    ignore_changes = [cpu, nvram, disk["url"]]
  }

  cpu {
    mode = %[2]q
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
    url = %[3]q
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

resource "talos_machine" "this" {
  node                  = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.this.machine_configuration
  image            = "%[4]s:%[5]s"
  drain_on_upgrade = false

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}

resource "talos_machine_bootstrap" "this" {
  depends_on           = [talos_machine.this]
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration = talos_machine_secrets.this.client_configuration
}

resource "talos_cluster_kubeconfig" "this" {
  depends_on           = [talos_machine_bootstrap.this]
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
}

data "talos_cluster_health" "this" {
  depends_on = [talos_cluster_kubeconfig.this]

  client_configuration = talos_machine_secrets.this.client_configuration
  endpoints            = libvirt_domain.cp.network_interface[0].addresses
  control_plane_nodes  = libvirt_domain.cp.network_interface[0].addresses

  timeouts = {
    read = "25m"
  }
}
`, rName, cpuMode, isoURL, imageUrl, imageTag, isoVersion)
}

// testAccTalosMachineConfigWithWriteOnlyAttrs uses ephemeral talos_machine_secrets and
// talos_machine_configuration so that no credentials touch state, exercising the
// client_configuration_wo / machine_configuration_wo code path end-to-end.
func testAccTalosMachineConfigWithWriteOnlyAttrs(rName, imageUrl, imageTag, isoVersion string) string {
	cpuMode := cpuModeDefault
	if os.Getenv("CI") != "" {
		cpuMode = cpuModeCI
	}

	isoURL := fmt.Sprintf(
		"https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso",
		isoVersion,
	)

	return fmt.Sprintf(`
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_machine_configuration" "this" {
  cluster_name       = "test"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "controlplane"
  machine_secrets    = ephemeral.talos_machine_secrets.this.machine_secrets
  talos_version      = %[5]q
  kubernetes_version = "v1.35.3"
  docs               = false
  examples           = false
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/vda"
          image = "%[4]s:%[6]s"
        }
      }
    })
  ]
}

resource "libvirt_volume" "cp" {
  name = %[1]q
  size = 6442450944
}

resource "libvirt_domain" "cp" {
  name     = %[1]q
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s_VARS.fd"
    template = "/usr/share/OVMF/OVMF_VARS_4M.fd"
  }

  lifecycle {
    ignore_changes = [cpu, nvram, disk["url"]]
  }

  cpu {
    mode = %[2]q
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
    url = %[3]q
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

resource "talos_machine" "this" {
  node                     = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint                 = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration_wo  = ephemeral.talos_machine_secrets.this.client_configuration
  machine_configuration_wo = ephemeral.talos_machine_configuration.this.machine_configuration
  image            = "%[4]s:%[5]s"
  drain_on_upgrade = false

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}
`, rName, cpuMode, isoURL, imageUrl, imageTag, isoVersion)
}

// TestValidateConfig_DrainKubeconfigRequirement verifies that ValidateConfig rejects
// configs where drain_on_upgrade is enabled (explicitly or by default), image is managed,
// and no kubeconfig is supplied. Drain only runs during OS upgrades (when image is set),
// so configs without image must not require kubeconfig.
//
// Regression: drain_on_upgrade defaults to true, so omitting it (null in config, before
// defaults are applied) must also trigger the error — not silently pass.
func TestValidateConfig_DrainKubeconfigRequirement(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := talos.NewTalosMachineResource()

	var schemaResp frameworkresource.SchemaResponse

	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

	sch := schemaResp.Schema

	schTFType, ok := sch.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema TerraformType is not tftypes.Object")
	}

	clientConfigAttrType, ok := schTFType.AttributeTypes["client_configuration"].(tftypes.Object)
	if !ok {
		t.Fatal("client_configuration attribute type is not tftypes.Object")
	}

	clientConfigVal := tftypes.NewValue(clientConfigAttrType, map[string]tftypes.Value{
		"ca_certificate":     tftypes.NewValue(tftypes.String, "ca"),
		"client_certificate": tftypes.NewValue(tftypes.String, "cert"),
		"client_key":         tftypes.NewValue(tftypes.String, "key"),
	})

	nullVals := func() map[string]tftypes.Value {
		m := make(map[string]tftypes.Value, len(schTFType.AttributeTypes))
		for name, typ := range schTFType.AttributeTypes {
			m[name] = tftypes.NewValue(typ, nil)
		}

		return m
	}

	boolVal := func(b bool) tftypes.Value { return tftypes.NewValue(tftypes.Bool, b) }
	boolValPtr := func(b bool) *tftypes.Value {
		v := boolVal(b)

		return &v
	}

	strPtr := func(s string) *string { return &s }

	const imageRef = "ghcr.io/siderolabs/installer:v1.13.0"

	tests := []struct {
		drainOnUpgrade      *tftypes.Value // nil = omitted (null) in config
		kubeconfig          *string        // nil = null; &"" = empty string; &"x" = value
		kubeconfigWO        *string        // nil = null; &"" = empty string; &"x" = value
		name                string
		imageSet            bool // true = set image to imageRef
		kubeconfigWOUnknown bool // true = kubeconfig_wo is an unresolved reference (unknown)
		wantError           bool
	}{
		{
			name:           "image set, drain=true explicit, no kubeconfig",
			imageSet:       true,
			drainOnUpgrade: boolValPtr(true),
			wantError:      true,
		},
		{
			name:           "image set, drain omitted (defaults to true), no kubeconfig",
			imageSet:       true,
			drainOnUpgrade: nil,
			wantError:      true,
		},
		{
			name:           "image set, drain=true, kubeconfig empty string",
			imageSet:       true,
			drainOnUpgrade: boolValPtr(true),
			kubeconfig:     strPtr(""),
			wantError:      true,
		},
		{
			name:           "image set, drain=true, kubeconfig whitespace",
			imageSet:       true,
			drainOnUpgrade: boolValPtr(true),
			kubeconfig:     strPtr("   "),
			wantError:      true,
		},
		{
			name:           "image set, drain=false, no kubeconfig",
			imageSet:       true,
			drainOnUpgrade: boolValPtr(false),
			wantError:      false,
		},
		{
			name:           "image set, drain=true, kubeconfig set",
			imageSet:       true,
			drainOnUpgrade: boolValPtr(true),
			kubeconfig:     strPtr("kubeconfig-yaml-content"),
			wantError:      false,
		},
		{
			name:           "image set, drain=true, kubeconfig_wo set",
			imageSet:       true,
			drainOnUpgrade: boolValPtr(true),
			kubeconfigWO:   strPtr("kubeconfig-yaml-content"),
			wantError:      false,
		},
		{
			name:           "no image (config-only), drain=true, no kubeconfig",
			imageSet:       false,
			drainOnUpgrade: boolValPtr(true),
			wantError:      false,
		},
		{
			name:                "image set, drain=true, kubeconfig_wo unknown (ephemeral ref not yet resolved)",
			imageSet:            true,
			drainOnUpgrade:      boolValPtr(true),
			kubeconfigWOUnknown: true,
			wantError:           false,
		},
	}

	rvc, ok := r.(frameworkresource.ResourceWithValidateConfig)
	if !ok {
		t.Fatal("resource does not implement ResourceWithValidateConfig")
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			vals := nullVals()
			vals["client_configuration"] = clientConfigVal
			vals["machine_configuration"] = tftypes.NewValue(tftypes.String, "config: {}")

			if tc.imageSet {
				vals["image"] = tftypes.NewValue(tftypes.String, imageRef)
			}

			if tc.drainOnUpgrade == nil {
				vals["drain_on_upgrade"] = tftypes.NewValue(tftypes.Bool, nil)
			} else {
				vals["drain_on_upgrade"] = *tc.drainOnUpgrade
			}

			if tc.kubeconfig != nil {
				vals["kubeconfig"] = tftypes.NewValue(tftypes.String, *tc.kubeconfig)
			}

			if tc.kubeconfigWOUnknown {
				vals["kubeconfig_wo"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
			} else if tc.kubeconfigWO != nil {
				vals["kubeconfig_wo"] = tftypes.NewValue(tftypes.String, *tc.kubeconfigWO)
			}

			raw := tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, vals)
			req := frameworkresource.ValidateConfigRequest{
				Config: tfsdk.Config{Schema: sch, Raw: raw},
			}
			resp := frameworkresource.ValidateConfigResponse{}

			rvc.ValidateConfig(ctx, req, &resp)

			if tc.wantError && !resp.Diagnostics.HasError() {
				t.Error("expected validation error, got none")
			}

			if !tc.wantError && resp.Diagnostics.HasError() {
				t.Errorf("unexpected validation error: %v", resp.Diagnostics)
			}
		})
	}
}

// TestImageHasUseStateForUnknown verifies that `image` carries UseStateForUnknown.
// The Terraform framework resolves Unknown → prior-state value before Create or
// Update is called, so the bare IsNull() guard in both methods is correct and a
// defensive IsUnknown() check is unnecessary.
func TestImageHasUseStateForUnknown(t *testing.T) {
	t.Parallel()

	r := talos.NewTalosMachineResource()

	var resp frameworkresource.SchemaResponse

	r.Schema(context.Background(), frameworkresource.SchemaRequest{}, &resp)

	imageAttr, ok := resp.Schema.Attributes["image"].(schema.StringAttribute)
	if !ok {
		t.Fatal("image attribute not found or wrong type")
	}

	// A non-empty PlanModifiers slice means UseStateForUnknown (or similar) is
	// registered; the framework guarantees the value is Known before CRUD methods run.
	if len(imageAttr.PlanModifiers) == 0 {
		t.Fatal("image has no plan modifier — Unknown can reach Create/Update")
	}
}

// TestModifyPlan_UnchangedConfig_HashIsKnown calls the real ModifyPlan and
// verifies that when the planned config bytes produce the same hash as the one
// already stored in state, the planned machine_configuration_hash is set to the
// known state value — so Update() correctly skips re-applying unchanged config.
func TestModifyPlan_UnchangedConfig_HashIsKnown(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := talos.NewTalosMachineResource()

	var schemaResp frameworkresource.SchemaResponse

	r.Schema(ctx, frameworkresource.SchemaRequest{}, &schemaResp)

	sch := schemaResp.Schema

	cfgContent := "machine: {}"
	cfgBytes := []byte(cfgContent)
	sum := sha256.Sum256(cfgBytes)
	expectedHash := hex.EncodeToString(sum[:])

	// Build tftypes.Value with null for every attribute, then override the ones
	// ModifyPlan actually reads.
	schTFType, ok := sch.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema TerraformType is not tftypes.Object")
	}

	nullVals := func() map[string]tftypes.Value {
		m := make(map[string]tftypes.Value, len(schTFType.AttributeTypes))
		for name, typ := range schTFType.AttributeTypes {
			m[name] = tftypes.NewValue(typ, nil)
		}

		return m
	}

	// State: config is set and hash is already known.
	sv := nullVals()
	sv["machine_configuration"] = tftypes.NewValue(tftypes.String, cfgContent)
	sv["machine_configuration_hash"] = tftypes.NewValue(tftypes.String, expectedHash)
	stateRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, sv)

	// Plan: same config, hash is Unknown (not yet resolved).
	pv := nullVals()
	pv["machine_configuration"] = tftypes.NewValue(tftypes.String, cfgContent)
	pv["machine_configuration_hash"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	planRaw := tftypes.NewValue(tftypes.Object{AttributeTypes: schTFType.AttributeTypes}, pv)

	planObj := tfsdk.Plan{Schema: sch, Raw: planRaw}
	req := frameworkresource.ModifyPlanRequest{
		Config: tfsdk.Config{Schema: sch, Raw: planRaw},
		Plan:   planObj,
		State:  tfsdk.State{Schema: sch, Raw: stateRaw},
	}
	resp := frameworkresource.ModifyPlanResponse{Plan: planObj}

	rmp, ok := r.(frameworkresource.ResourceWithModifyPlan)
	if !ok {
		t.Fatal("resource does not implement ResourceWithModifyPlan")
	}

	rmp.ModifyPlan(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("ModifyPlan returned errors: %v", resp.Diagnostics)
	}

	var plannedHash types.String

	if diags := resp.Plan.GetAttribute(ctx, frameworkpath.Root("machine_configuration_hash"), &plannedHash); diags.HasError() {
		t.Fatalf("GetAttribute returned errors: %v", diags)
	}

	if plannedHash.IsUnknown() {
		t.Fatal("planned machine_configuration_hash is Unknown when config is unchanged")
	}

	if plannedHash.ValueString() != expectedHash {
		t.Fatalf("planned hash %q != expected %q", plannedHash.ValueString(), expectedHash)
	}
}

// TestNodeRequiresReplace verifies that `node` carries RequiresReplace, so changing
// it triggers destroy-and-recreate rather than an in-place update that would leave
// the resource ID pointing at the old node.
func TestNodeRequiresReplace(t *testing.T) {
	t.Parallel()

	r := talos.NewTalosMachineResource()

	var resp frameworkresource.SchemaResponse

	r.Schema(context.Background(), frameworkresource.SchemaRequest{}, &resp)

	nodeAttr, ok := resp.Schema.Attributes["node"].(schema.StringAttribute)
	if !ok {
		t.Fatal("node attribute not found or wrong type")
	}

	if len(nodeAttr.PlanModifiers) == 0 {
		t.Fatal("node has no plan modifier: a node change triggers Update() instead of replace, leaving id stale")
	}
}

// TestDefaultTimeoutsSufficientForWorstCase verifies that the default Create and
// Update timeouts cover the worst-case internal retry budgets:
//
//	Create: must exceed 20 m (10 m apply + 10 m wait-for-node)
//	Update: must exceed 80 m (10+10+60 m with legacy upgrade)
func TestDefaultTimeoutsSufficientForWorstCase(t *testing.T) {
	t.Parallel()

	const (
		applyRetryBudget    = 10 * time.Minute
		waitNodeBudget      = 10 * time.Minute
		legacyUpgradeBudget = 60 * time.Minute
	)

	// Default timeouts — read from the exported constants in talos_machine_resource.go
	// so this test breaks if someone lowers them below the worst-case threshold.
	defaultCreate := talos.DefaultCreateTimeout
	defaultUpdate := talos.DefaultUpdateTimeout

	worstCaseCreate := applyRetryBudget + waitNodeBudget                       // 20 m
	worstCaseUpdate := applyRetryBudget + waitNodeBudget + legacyUpgradeBudget // 80 m

	if defaultCreate < worstCaseCreate {
		t.Errorf("Create default %v < worst-case %v: operations will time out", defaultCreate, worstCaseCreate)
	}

	if defaultUpdate < worstCaseUpdate {
		t.Errorf("Update default %v < worst-case %v: legacy upgrade will time out", defaultUpdate, worstCaseUpdate)
	}
}
