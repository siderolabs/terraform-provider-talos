resource "talos_machine_secrets" "machine_secrets" {}

resource "talos_client_configuration" "talosconfig" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.machine_secrets.machine_secrets
  endpoints       = ["10.5.0.2"]
}

resource "talos_machine_bootstrap" "bootstrap" {
  talos_config = talos_client_configuration.talosconfig.talos_config
  endpoint     = "10.5.0.2"
  node         = "10.5.0.2"
}
