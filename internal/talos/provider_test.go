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
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"github.com/siderolabs/terraform-provider-talos/internal/talos"
)

var (
	// testAccProtoV6ProviderFactories are used to instantiate a provider during
	// acceptance testing. The factory function will be invoked for every Terraform
	// CLI command executed to create a provider server to which the CLI can
	// reattach.
	testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
		"talos": providerserver.NewProtocol6WithError(talos.New()),
	}
)

func downloadTalosISO(dir, isoPath string) error {
	isoURL := fmt.Sprintf("https://github.com/siderolabs/talos/releases/download/%s/talos-amd64.iso", gendata.VersionTag)

	if _, err := os.Stat(isoPath); err == nil {
		return nil
	}

	out, err := os.Create(isoPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(isoURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	if _, err = io.Copy(out, resp.Body); err != nil {
		return err
	}

	return nil
}

func testAccCheckResourceDisappears(resourceNames []string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		return s.Remove(resourceNames...)
	}
}

func testAccDynamicConfig(provider, resourceName, cpuMode, isoPath string, withBootstrap, withRetrieveKubeConfig bool) string {
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
    ]
  }
  cpu {
    mode = "{{ .CpuMode }}"
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

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = libvirt_domain.cp.network_interface[0].addresses[0]
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
  wait                 = true
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = libvirt_domain.cp.network_interface[0].addresses[0]
}
{{ end }}
{{ end }}
`

	var config strings.Builder
	template.Must(template.New("tf_config").Parse(configTemplate)).Execute(&config, struct {
		Provider               string
		ResourceName           string
		CpuMode                string
		IsoPath                string
		WithBootstrap          bool
		WithRetrieveKubeConfig bool
	}{
		Provider:               provider,
		ResourceName:           resourceName,
		CpuMode:                cpuMode,
		IsoPath:                isoPath,
		WithBootstrap:          withBootstrap,
		WithRetrieveKubeConfig: withRetrieveKubeConfig,
	})

	return config.String()
}
