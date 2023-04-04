resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name     = "example-cluster"
  type             = "controlplane"
  cluster_endpoint = "https://cluster.local:6443"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
}

data "talos_client_configuration" "this" {
  cluster_name         = "example-cluster"
  client_configuration = talos_machine_secrets.this.client_configuration
  nodes                = ["10.5.0.2"]
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.this.machine_configuration
  node                  = "10.5.0.2"
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk = "/dev/sdd"
        }
      }
    })
  ]
}

resource "talos_machine_bootstrap" "this" {
  depends_on = [
    talos_machine_configuration_apply.this
  ]
  node                 = "10.5.0.2"
  client_configuration = talos_machine_secrets.this.client_configuration
}


data "talos_cluster_kubeconfig" "this" {
  depends_on = [
    talos_machine_bootstrap.this
  ]
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = "10.5.0.2"
}
