output "machineconfig_controlplane" {
  value = talos_machine_configuration_controlplane.machineconfig_cp.machine_config
}

output "machineconfig_worker" {
  value = talos_machine_configuration_worker.machineconfig_worker.machine_config
}

output "talosconfig" {
  value = talos_client_configuration.talosconfig.talos_config
}
