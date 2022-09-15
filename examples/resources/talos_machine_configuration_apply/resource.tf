resource "talos_machine_secrets" "machine_secrets" {}

resource "talos_client_configuration" "talosconfig" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.machine_secrets.machine_secrets
  endpoints       = ["10.5.0.2"]
}

resource "talos_machine_configuration_controlplane" "machineconfig_cp" {
  cluster_name     = talos_client_configuration.talosconfig.cluster_name
  cluster_endpoint = "https://cluster.local:6443"
  machine_secrets  = talos_machine_secrets.machine_secrets.machine_secrets
}

resource "talos_machine_configuration_apply" "config_apply" {
  talos_config          = talos_client_configuration.talosconfig.talos_config
  machine_configuration = talos_machine_configuration_controlplane.machineconfig_cp.machine_config
  config_patches = [
    file("${path.module}/files/worker.yaml"),
  ]
  endpoint = "10.5.0.2"
  node     = "10.5.0.2"
}
