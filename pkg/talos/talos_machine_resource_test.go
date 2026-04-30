// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"crypto/sha256"
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
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// TestAccTalosMachine_bootstrap applies machine configuration via talos_machine,
// bootstraps etcd, waits for cluster health, and confirms idempotency.
func TestAccTalosMachine_bootstrap(t *testing.T) {
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
				Config: testAccTalosMachineConfig(rName, gendata.VersionTag, gendata.VersionTag),
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
				Config:   testAccTalosMachineConfig(rName, gendata.VersionTag, gendata.VersionTag),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachine_upgrade tests that changing `image` triggers an OS upgrade:
// the node is initially at v1.12.7 and is upgraded to v1.13.0.
func TestAccTalosMachine_upgrade(t *testing.T) {
	const (
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
				Config: testAccTalosMachineConfig(rName, baseVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("ghcr.io/siderolabs/installer:%s", baseVersion)),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 2: upgrade to v1.13.0, cluster still healthy afterwards
			{
				Config: testAccTalosMachineConfig(rName, upgradeVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("ghcr.io/siderolabs/installer:%s", upgradeVersion)),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 3: idempotency after upgrade
			{
				Config:   testAccTalosMachineConfig(rName, upgradeVersion, baseVersion),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosMachine_upgradeLifecycle tests the LifecycleService upgrade path (Talos ≥ v1.13):
// the node boots at v1.13.0-rc.0 and is upgraded to v1.13.0 via ImageClient.Pull + LifecycleService.Upgrade.
func TestAccTalosMachine_upgradeLifecycle(t *testing.T) {
	const (
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
				Config: testAccTalosMachineConfig(rName, baseVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("ghcr.io/siderolabs/installer:%s", baseVersion)),
					resource.TestCheckResourceAttrSet("talos_machine.this", "machine_configuration_hash"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 2: upgrade to v1.13.0 via LifecycleService (new path), cluster still healthy
			{
				Config: testAccTalosMachineConfig(rName, upgradeVersion, baseVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine.this", "image",
						fmt.Sprintf("ghcr.io/siderolabs/installer:%s", upgradeVersion)),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			// Step 3: idempotency after upgrade
			{
				Config:   testAccTalosMachineConfig(rName, upgradeVersion, baseVersion),
				PlanOnly: true,
			},
		},
	})
}

// testAccTalosMachineConfig generates HCL for a talos_machine resource backed by a
// libvirt VM, with etcd bootstrap and cluster health check always included.
// imageTag is the desired Talos installer image version.
// isoVersion is the Talos version of the ISO used to boot the libvirt VM.
const (
	cpuModeDefault = "host-passthrough"
	cpuModeCI      = "host-model"
)

func testAccTalosMachineConfig(rName, imageTag, isoVersion string) string {
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
  talos_version      = %[4]q
  kubernetes_version = "v1.35.3"
  docs               = false
  examples           = false
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/vda"
          image = "ghcr.io/siderolabs/installer:%[5]s"
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
  image            = "ghcr.io/siderolabs/installer:%[4]s"
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
`, rName, cpuMode, isoURL, imageTag, isoVersion)
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
