resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name       = "example-cluster"
  machine_type       = "controlplane"
  cluster_endpoint   = "https://10.5.0.2:6443"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  kubernetes_version = "v1.32.0"
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = "10.5.0.2"
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk = "/dev/sda"
        }
      }
    })
  ]
}

resource "talos_cluster" "this" {
  depends_on           = [talos_machine_configuration_apply.this]
  node                 = "10.5.0.2"
  client_configuration = talos_machine_secrets.this.client_configuration
  kubernetes_version   = "v1.32.0"
}

data "talos_cluster_kubeconfig" "this" {
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = "10.5.0.2"
  depends_on           = [talos_cluster.this]
}
