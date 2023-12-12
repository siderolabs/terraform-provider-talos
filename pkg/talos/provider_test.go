// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"io"
	"net/http"
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

func downloadTalosISO(isoPath string) error {
	isoURL := fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/metal-amd64.iso", gendata.VersionTag)

	if _, err := os.Stat(isoPath); err == nil {
		return nil
	}

	out, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	defer out.Close() //nolint:errcheck

	resp, err := http.Get(isoURL) //nolint:noctx
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	if _, err = io.Copy(out, resp.Body); err != nil {
		return err
	}

	return nil
}

type dynamicConfig struct {
	Provider               string
	ResourceName           string
	IsoPath                string
	CPUMode                string
	DiskSizeFilter         string
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

	configTemplate := `
resource "talos_machine_secrets" "this" {}

resource "libvirt_volume" "cp" {
  name = "{{ .ResourceName }}"
  size = 6442450944
}

{{ if .DiskSizeFilter }}
resource "libvirt_volume" "extra_disk" {
  name = "{{ .ResourceName }}-extra-disk"
  size = 2000000000
}
{{ end }}

resource "libvirt_domain" "cp" {
  name     = "{{ .ResourceName }}"
  firmware = "/usr/share/OVMF/OVMF_CODE.fd"
  lifecycle {
    ignore_changes = [
      cpu,
      nvram,
    ]
  }
  cpu {
    mode = "{{ .CPUMode }}"
  }
  console {
    type        = "pty"
    target_port = "0"
  }
  disk {
    file = "{{ .IsoPath }}"
  }
  disk {
    volume_id = libvirt_volume.cp.id
  }
{{ if .DiskSizeFilter }}
  disk {
    volume_id = libvirt_volume.extra_disk.id
  }
{{ end }}
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
{{ if .DiskSizeFilter }}
  filters = {
    size = "{{ .DiskSizeFilter }}"
  }
{{ end }}
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
          disk = data.talos_machine_disks.this.disks[0].name
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
data "talos_cluster_kubeconfig" "this" {
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
    data.talos_cluster_kubeconfig.this
  ]

  timeouts = {
    read = "20m"
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
