output "machineconfig_controlplane" {
  value     = talos_machine_configuration_controlplane.machineconfig_cp.machine_config
  sensitive = true
}

output "machineconfig_worker" {
  value     = talos_machine_configuration_worker.machineconfig_worker.machine_config
  sensitive = true
}

output "talosconfig" {
  value     = talos_client_configuration.talosconfig.talos_config
  sensitive = true
}

output "talosconfig_dynamic" {
  value     = data.talos_client_configuration.talosconfig.talos_config
  sensitive = true
}

output "kubeconfig" {
  value     = talos_cluster_kubeconfig.kubeconfig.kube_config
  sensitive = true
}

output "kubeconfig_dynamic" {
  value     = data.talos_cluster_kubeconfig.kubeconfig.kube_config
  sensitive = true
}
