terraform {
  required_version = ">= 1.11"
  required_providers {
    talos = {
      source  = "siderolabs/talos"
      version = "~> 0.11"
    }
    vault = {
      source  = "hashicorp/vault"
      version = "~> 5.0"
    }
  }
}

# This example demonstrates the correct pattern for using ephemeral resources
# with Talos. Machine secrets are persisted to Vault to ensure deterministic
# infrastructure state across Terraform runs.

# STEP 1: Generate and store machine secrets in Vault
# The ephemeral resource generates secrets, which are immediately stored in Vault
# using write-only attributes. After the initial run, this ephemeral resource won't
# be evaluated again because data_json_wo_version is hardcoded.
ephemeral "talos_machine_secrets" "this" {}

resource "vault_kv_secret_v2" "talos_secrets" {
  mount = "secret"
  name  = "talos-example-cluster"

  # Use write-only attributes to prevent secrets from being stored in Terraform state
  data_json_wo = jsonencode({
    machine_secrets      = ephemeral.talos_machine_secrets.this.machine_secrets
    client_configuration = ephemeral.talos_machine_secrets.this.client_configuration
  })
  # Hardcoded version prevents unnecessary refreshes after initial creation
  data_json_wo_version = 1
}

# STEP 2: Retrieve secrets ephemerally from Vault for cluster operations
# This ephemeral resource reads from Vault on every run, but values are never
# stored in Terraform state. Referencing the resource attributes creates an
# implicit dependency so Terraform creates the secret before reading it.
ephemeral "vault_kv_secret_v2" "talos_secrets" {
  mount = vault_kv_secret_v2.talos_secrets.mount
  name  = vault_kv_secret_v2.talos_secrets.name
}

locals {
  # Decode secrets from Vault
  talos_data = jsondecode(ephemeral.vault_kv_secret_v2.talos_secrets.data_json)
}

# Generate controlplane machine configuration using ephemeral secrets from Vault
ephemeral "talos_machine_configuration" "controlplane" {
  cluster_name     = "example-cluster"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_type     = "controlplane"
  machine_secrets  = local.talos_data.machine_secrets

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

# Apply configuration to a machine
resource "talos_machine_configuration_apply" "controlplane" {
  client_configuration_wo        = local.talos_data.client_configuration
  machine_configuration_input_wo = ephemeral.talos_machine_configuration.controlplane.machine_configuration
  node                           = "10.5.0.2"
}

# Bootstrap the cluster
resource "talos_machine_bootstrap" "this" {
  client_configuration_wo = local.talos_data.client_configuration
  node                    = "10.5.0.2"

  depends_on = [
    talos_machine_configuration_apply.controlplane
  ]
}

# Wait for cluster to be healthy (ephemeral - doesn't leak secrets to state)
ephemeral "talos_cluster_health" "this" {
  client_configuration   = local.talos_data.client_configuration
  endpoints              = ["10.5.0.2"]
  control_plane_nodes    = ["10.5.0.2"]
  worker_nodes           = []
  skip_kubernetes_checks = false

  depends_on = [
    talos_machine_bootstrap.this
  ]
}

# Retrieve cluster kubeconfig ephemerally
ephemeral "talos_cluster_kubeconfig" "this" {
  client_configuration = local.talos_data.client_configuration
  node                 = "10.5.0.2"

  depends_on = [
    ephemeral.talos_cluster_health.this
  ]
}

# Output the kubeconfig (marked as ephemeral)
output "kubeconfig" {
  value     = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
  sensitive = true
  ephemeral = true
}
