resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name     = "example-cluster"
  machine_type     = "controlplane"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/sda"
          image = "ghcr.io/siderolabs/installer:v1.9.0"
        }
      }
    })
  ]
}

resource "talos_machine" "this" {
  node                  = "10.5.0.2"
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.this.machine_configuration
  image                 = "ghcr.io/siderolabs/installer:v1.9.0"
}

resource "talos_machine_bootstrap" "this" {
  depends_on           = [talos_machine.this]
  node                 = "10.5.0.2"
  client_configuration = talos_machine_secrets.this.client_configuration
}
