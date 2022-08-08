terraform {
  required_version = ">= 0.12"
}

provider "talos" {}

resource "talos_machine_secrets" "machine_secrets" {}

resource "talos_machine_configuration_controlplane" "machineconfig_cp" {
  cluster_name     = var.cluster_name
  cluster_endpoint = var.cluster_endpoint
  machine_secrets  = talos_machine_secrets.machine_secrets.machine_secrets
  config_patch     = <<EOT
cluster:
  allowSchedulingOnControlPlanes: true
EOT
}

resource "talos_machine_configuration_worker" "machineconfig_worker" {
  cluster_name     = var.cluster_name
  cluster_endpoint = var.cluster_endpoint
  machine_secrets  = talos_machine_secrets.machine_secrets.machine_secrets
}

data "talos_client_configuration" "talosconfig" {
  cluster_name    = var.cluster_name
  machine_secrets = talos_machine_secrets.machine_secrets.machine_secrets
  endpoints       = ["10.5.0.2"]
}
