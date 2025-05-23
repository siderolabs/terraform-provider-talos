// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/siderolabs/talos/pkg/machinery/gendata"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// testAccProtoV6ProviderFactories are used to instantiate a provider during
// acceptance testing. The factory function will be invoked for every Terraform
// CLI command executed to create a provider server to which the CLI can
// reattach.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"talos": providerserver.NewProtocol6WithError(talos.New()),
}

type dynamicConfig struct {
	Provider               string
	ResourceName           string
	IsoURL                 string
	CPUMode                string
	WithApplyConfig        bool
	WithBootstrap          bool
	WithRetrieveKubeConfig bool
	WithClusterHealth      bool
}

func (c *dynamicConfig) render() string {
	cpuMode := "host-passthrough"
	if os.Getenv("CI") != "" {
		cpuMode = "host-model"
	}

	c.CPUMode = cpuMode

	c.IsoURL = fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso", gendata.VersionTag)

	configTemplate := `
resource "talos_machine_secrets" "this" {}

resource "libvirt_volume" "cp" {
  name = "{{ .ResourceName }}"
  size = 6442450944
}

resource "libvirt_domain" "cp" {
  name     = "{{ .ResourceName }}"
  firmware = "/usr/share/OVMF/OVMF_CODE.fd"
  lifecycle {
    ignore_changes = [
      cpu,
      nvram,
      disk["url"],
    ]
  }
  cpu {
    mode = "{{ .CPUMode }}"
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
    url = "{{ .IsoURL }}"
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

{{ if eq .Provider "talosv1"}}
resource "talos_client_configuration" "this" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_controlplane" "this" {
  cluster_name     = "example-cluster"
  cluster_endpoint = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
}

{{ if .WithApplyConfig }}
resource "talos_machine_configuration_apply" "this" {
  talos_config          = talos_client_configuration.this.talos_config
  machine_configuration = talos_machine_configuration_controlplane.this.machine_config
  node                  = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint              = libvirt_domain.cp.network_interface[0].addresses[0]
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk = "/dev/vda"
        }
      }
    }),
  ]
}
{{ end }}

{{ if .WithBootstrap }}
resource "talos_machine_bootstrap" "this" {
  depends_on = [
    talos_machine_configuration_apply.this
  ]
  node         = libvirt_domain.cp.network_interface[0].addresses[0]
  endpoint     = libvirt_domain.cp.network_interface[0].addresses[0]
  talos_config = talos_client_configuration.this.talos_config
}
{{ end }}
{{ else }}
data "talos_machine_configuration" "this" {
  cluster_name     = "example-cluster"
  cluster_endpoint = "https://${libvirt_domain.cp.network_interface[0].addresses[0]}:6443"
  machine_type     = "controlplane"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
  docs             = false
  examples         = false
}

data "talos_machine_disks" "this" {
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
  selector             = "disk.size > 6u * GB"
}

{{ if .WithApplyConfig }}
resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
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
{{ end }}

{{ if .WithBootstrap }}
resource "talos_machine_bootstrap" "this" {
  depends_on = [
    talos_machine_configuration_apply.this
  ]
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
  client_configuration = talos_machine_secrets.this.client_configuration
}
{{ end }}

{{ if .WithRetrieveKubeConfig }}
resource "talos_cluster_kubeconfig" "this" {
  depends_on = [
    talos_machine_bootstrap.this
  ]
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
}
{{ end }}

{{ if .WithClusterHealth }}
data "talos_cluster_health" "this" {
  depends_on = [
    talos_cluster_kubeconfig.this
  ]

  timeouts = {
    read = "25m"
  }

  client_configuration = talos_machine_secrets.this.client_configuration
  endpoints            = libvirt_domain.cp.network_interface[0].addresses
  control_plane_nodes  = libvirt_domain.cp.network_interface[0].addresses
}
{{ end }}
{{ end }}
`

	var config strings.Builder

	template.Must(template.New("tf_config").Parse(configTemplate)).Execute(&config, c) //nolint:errcheck

	return config.String()
}
