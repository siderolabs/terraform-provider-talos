terraform {
  required_version = ">= 0.12"
  experiments      = [module_variable_optional_attrs]
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
}

resource "talos_machine_configuration_worker" "machineconfig_worker" {
  cluster_name     = var.cluster_name
  cluster_endpoint = var.cluster_endpoint
  machine_secrets  = talos_machine_secrets.machine_secrets.machine_secrets
}

resource "talos_client_configuration" "talosconfig" {
  cluster_name    = var.cluster_name
  machine_secrets = talos_machine_secrets.machine_secrets.machine_secrets
  endpoints       = [for k, v in var.node_data.controlplanes : k]
}

resource "talos_machine_configuration_apply" "cp_config_apply" {
  talos_config          = talos_client_configuration.talosconfig.talos_config
  machine_configuration = talos_machine_configuration_controlplane.machineconfig_cp.machine_config
  for_each              = var.node_data.controlplanes
  endpoint              = each.key
  node                  = each.key
  config_patches = [
    templatefile("${path.module}/templates/patch.yaml.tmpl", {
      hostname     = each.value.hostname == null ? format("%s-cp-%s", var.cluster_name, index(keys(var.node_data.workers), each.key)) : each.value.hostname
      install_disk = each.value.install_disk
    }),
    file("${path.module}/files/patch.json"),
  ]
}

resource "talos_machine_configuration_apply" "worker_config_apply" {
  talos_config          = talos_client_configuration.talosconfig.talos_config
  machine_configuration = talos_machine_configuration_worker.machineconfig_worker.machine_config
  for_each              = var.node_data.workers
  endpoint              = each.key
  node                  = each.key
  config_patches = [
    templatefile("${path.module}/templates/patch.yaml.tmpl", {
      hostname     = each.value.hostname == null ? format("%s-cp-%s", var.cluster_name, index(keys(var.node_data.workers), each.key)) : each.value.hostname
      install_disk = each.value.install_disk
    })
  ]
}

resource "talos_machine_bootstrap" "bootstrap" {
  talos_config = talos_client_configuration.talosconfig.talos_config
  endpoint     = [for k, v in var.node_data.controlplanes : k][0]
  node         = [for k, v in var.node_data.controlplanes : k][0]
}

resource "talos_cluster_kubeconfig" "kubeconfig" {
  talos_config = talos_client_configuration.talosconfig.talos_config
  endpoint     = [for k, v in var.node_data.controlplanes : k][0]
  node         = [for k, v in var.node_data.controlplanes : k][0]
}
