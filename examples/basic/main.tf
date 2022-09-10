terraform {
  required_version = ">= 0.12"
  required_providers {
    talos = {
      source = "siderolabs/talos"
    }
  }
}

provider "talos" {}

resource "talos_machine_secrets" "machine_secrets" {}

resource "talos_machine_configuration_controlplane" "machineconfig_cp" {
  cluster_name     = var.cluster_name
  cluster_endpoint = var.cluster_endpoint
  machine_secrets  = talos_machine_secrets.machine_secrets.machine_secrets
  config_patches = [
    templatefile("${path.module}/templates/patch.yaml.tmpl", {
      hostname = "talos-cp-0"
    }),
    file("${path.module}/files/patch.json"),
  ]
}

resource "talos_machine_configuration_worker" "machineconfig_worker" {
  cluster_name     = var.cluster_name
  cluster_endpoint = var.cluster_endpoint
  machine_secrets  = talos_machine_secrets.machine_secrets.machine_secrets
}

resource "talos_client_configuration" "talosconfig" {
  cluster_name    = var.cluster_name
  machine_secrets = talos_machine_secrets.machine_secrets.machine_secrets
  endpoints       = ["10.5.0.2"]
}

// use data source if you want a get a new talos client config each time TF is run
data "talos_client_configuration" "talosconfig" {
  cluster_name    = var.cluster_name
  machine_secrets = talos_machine_secrets.machine_secrets.machine_secrets
  endpoints       = ["10.5.0.2"]
}

resource "talos_machine_configuration_apply" "cp_config_apply" {
  talos_config          = talos_client_configuration.talosconfig.talos_config
  machine_configuration = talos_machine_configuration_controlplane.machineconfig_cp.machine_config
  nodes                 = ["10.5.0.2"]
}

resource "talos_machine_configuration_apply" "worker_config_apply" {
  talos_config          = talos_client_configuration.talosconfig.talos_config
  machine_configuration = talos_machine_configuration_worker.machineconfig_worker.machine_config
  config_patches = [
    file("${path.module}/files/worker.yaml"),
  ]
  nodes = ["10.5.0.3", "10.5.0.4"]
}

resource "talos_machine_bootstrap" "bootstrap" {
  talos_config = talos_client_configuration.talosconfig.talos_config
  node         = "10.5.0.2"
}

resource "talos_cluster_kubeconfig" "kubeconfig" {
  talos_config = talos_client_configuration.talosconfig.talos_config
  nodes        = ["10.5.0.2"]
}

// use data source if you want a get a new kubeconfig each time TF is run
data "talos_cluster_kubeconfig" "kubeconfig" {
  talos_config = talos_client_configuration.talosconfig.talos_config
  nodes        = ["10.5.0.2"]
}
