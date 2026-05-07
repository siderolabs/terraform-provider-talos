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

// TestAccTalosCluster_bootstrap bootstraps etcd via talos_cluster, checks health, and
// verifies idempotency.
func TestAccTalosCluster_bootstrap(t *testing.T) {
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
				Config: testAccTalosClusterConfig(rName, gendata.VersionTag, "v1.35.4"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_cluster.this", "id"),
				),
			},
			// second apply must produce an empty plan
			{
				Config:   testAccTalosClusterConfig(rName, gendata.VersionTag, "v1.35.4"),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosCluster_upgrade tests that changing kubernetes_version triggers a
// rolling Kubernetes upgrade from v1.35.4 to v1.36.0.
func TestAccTalosCluster_upgrade(t *testing.T) {
	const (
		baseK8sVersion    = "v1.35.4"
		upgradeK8sVersion = "v1.36.0"
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
			// Step 1: cluster bootstrapped at base k8s version
			{
				Config: testAccTalosClusterConfig(rName, gendata.VersionTag, baseK8sVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_cluster.this", "kubernetes_version", baseK8sVersion),
				),
			},
			// Step 2: upgrade to v1.36.0
			{
				Config: testAccTalosClusterConfig(rName, gendata.VersionTag, upgradeK8sVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_cluster.this", "kubernetes_version", upgradeK8sVersion),
				),
			},
			// Step 3: idempotency after upgrade
			{
				Config:   testAccTalosClusterConfig(rName, gendata.VersionTag, upgradeK8sVersion),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosClusterConfig(rName, talosVersion, k8sVersion string) string {
	cpuMode := cpuModeHostPassthrough
	if os.Getenv("CI") != "" {
		cpuMode = cpuModeHostModel
	}

	isoURL := fmt.Sprintf(
		"https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso",
		talosVersion,
	)

	return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name       = "test"
  cluster_endpoint   = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type       = "controlplane"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  talos_version      = %[4]q
  kubernetes_version = %[5]q
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

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = libvirt_domain.cp.network_interface[0].addresses[0]

  timeouts = {
    create = "15m"
  }
}

resource "talos_cluster" "this" {
  depends_on           = [talos_machine_configuration_apply.this]
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration = talos_machine_secrets.this.client_configuration
  kubernetes_version   = %[5]q

  timeouts = {
    create = "20m"
    update = "30m"
  }
}
`, rName, cpuMode, isoURL, talosVersion, k8sVersion)
}
