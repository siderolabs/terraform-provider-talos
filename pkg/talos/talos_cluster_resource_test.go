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
	"github.com/siderolabs/talos/pkg/images"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
)

// TestAccTalosCluster_bootstrapAndUpgrade bootstraps etcd via talos_cluster, verifies
// idempotency, then upgrades Kubernetes from v1.35.4 to v1.36.0 on the same cluster
// and verifies idempotency again.
func TestAccTalosCluster_bootstrapAndUpgrade(t *testing.T) {
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
			// Step 1: bootstrap cluster at base k8s version, check health
			{
				Config: testAccTalosClusterConfig(rName, gendata.VersionTag, baseK8sVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_cluster.this", "id"),
					resource.TestCheckResourceAttr("talos_cluster.this", "kubernetes_version", baseK8sVersion),
				),
			},
			// Step 2: idempotency at base version
			{
				Config:   testAccTalosClusterConfig(rName, gendata.VersionTag, baseK8sVersion),
				PlanOnly: true,
			},
			// Step 3: upgrade Kubernetes to v1.36.0 on the same cluster
			{
				Config: testAccTalosClusterConfig(rName, gendata.VersionTag, upgradeK8sVersion),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_cluster.this", "kubernetes_version", upgradeK8sVersion),
				),
			},
			// Step 4: idempotency after upgrade
			{
				Config:   testAccTalosClusterConfig(rName, gendata.VersionTag, upgradeK8sVersion),
				PlanOnly: true,
			},
		},
	})
}

// TestAccTalosCluster_bootstrapHA exercises bootstrapping a three-control-plane HA
// cluster via talos_cluster. This is the red-phase test for the bug reported in
// https://github.com/siderolabs/terraform-provider-talos/issues/339: the resource
// times out because talosClusterWaitForK8s only supplies the bootstrap node IP to
// the etcd membership check, which fails when etcd has three members.
func TestAccTalosCluster_bootstrapHA(t *testing.T) {
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
				Config: testAccTalosClusterHAConfig(rName, gendata.VersionTag, "v1.35.4"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_cluster.this", "id"),
				),
			},
			// second apply must produce an empty plan
			{
				Config:   testAccTalosClusterHAConfig(rName, gendata.VersionTag, "v1.35.4"),
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

	installerBase := images.InstallerImageRepository("metal")

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
          image = "%[6]s:%[4]s"
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

data "talos_cluster_health" "this" {
  depends_on           = [talos_cluster.this]
  client_configuration = talos_machine_secrets.this.client_configuration
  endpoints            = libvirt_domain.cp.network_interface[0].addresses
  control_plane_nodes  = libvirt_domain.cp.network_interface[0].addresses

  timeouts = {
    read = "20m"
  }
}
`, rName, cpuMode, isoURL, talosVersion, k8sVersion, installerBase)
}

func testAccTalosClusterHAConfig(rName, talosVersion, k8sVersion string) string {
	cpuMode := cpuModeHostPassthrough
	if os.Getenv("CI") != "" {
		cpuMode = cpuModeHostModel
	}

	isoURL := fmt.Sprintf(
		"https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso",
		talosVersion,
	)

	installerBase := images.InstallerImageRepository("metal")

	return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "cp" {
  cluster_name       = "test"
  cluster_endpoint   = "https://${libvirt_domain.cp_01.network_interface[0].addresses[0]}:6443"
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
          image = "%[6]s:%[4]s"
        }
      }
    })
  ]
}

resource "libvirt_volume" "cp_01" {
  name = "%[1]s-01"
  size = 6442450944
}

resource "libvirt_volume" "cp_02" {
  name = "%[1]s-02"
  size = 6442450944
}

resource "libvirt_volume" "cp_03" {
  name = "%[1]s-03"
  size = 6442450944
}

resource "libvirt_domain" "cp_01" {
  name     = "%[1]s-01"
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s-01_VARS.fd"
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
    volume_id = libvirt_volume.cp_01.id
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

resource "libvirt_domain" "cp_02" {
  name     = "%[1]s-02"
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s-02_VARS.fd"
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
    volume_id = libvirt_volume.cp_02.id
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

resource "libvirt_domain" "cp_03" {
  name     = "%[1]s-03"
  firmware = "/usr/share/OVMF/OVMF_CODE_4M.fd"

  nvram {
    file     = "/var/lib/libvirt/qemu/nvram/%[1]s-03_VARS.fd"
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
    volume_id = libvirt_volume.cp_03.id
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

resource "talos_machine" "cp_01" {
  node                  = libvirt_domain.cp_01.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.cp_01.network_interface[0].addresses[0]
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.cp.machine_configuration
  image                 = "%[6]s:%[4]s"
  drain_on_upgrade      = false

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}

resource "talos_machine" "cp_02" {
  node                  = libvirt_domain.cp_02.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.cp_02.network_interface[0].addresses[0]
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.cp.machine_configuration
  image                 = "%[6]s:%[4]s"
  drain_on_upgrade      = false

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}

resource "talos_machine" "cp_03" {
  node                  = libvirt_domain.cp_03.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.cp_03.network_interface[0].addresses[0]
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.cp.machine_configuration
  image                 = "%[6]s:%[4]s"
  drain_on_upgrade      = false

  timeouts = {
    create = "20m"
    update = "60m"
    delete = "5m"
  }
}

resource "talos_cluster" "this" {
  node                 = talos_machine.cp_01.node
  control_plane_nodes  = [talos_machine.cp_01.node, talos_machine.cp_02.node, talos_machine.cp_03.node]
  client_configuration = talos_machine_secrets.this.client_configuration
  kubernetes_version   = %[5]q

  timeouts = {
    create = "20m"
    update = "30m"
  }
}
`, rName, cpuMode, isoURL, talosVersion, k8sVersion, installerBase)
}
