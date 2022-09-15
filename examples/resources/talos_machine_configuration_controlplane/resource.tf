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
