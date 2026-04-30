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
	"github.com/siderolabs/talos/pkg/machinery/gendata"
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
