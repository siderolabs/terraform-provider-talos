terraform {
  required_providers {
    talos = {
      source = "siderolabs/talos"
      version = "0.1.2"
    }
  }
}

# You would need to provide a talos_client_configuration data source
# or hardcode the values needed for the ephemeral resource.

resource "talos_cluster_kubeconfig_ephemeral" "this" {
  # ... your required inputs like node and client_configuration ...
}

# Use the ephemeral kubeconfig to configure the kubernetes provider
provider "kubernetes" {
  host                   = talos_cluster_kubeconfig_ephemeral.this.kubernetes_client_configuration.host
  cluster_ca_certificate = base64decode(talos_cluster_kubeconfig_ephemeral.this.kubernetes_client_configuration.ca_certificate)
  client_certificate     = base64decode(talos_cluster_kubeconfig_ephemeral.this.kubernetes_client_configuration.client_certificate)
  client_key             = base64decode(talos_cluster_kubeconfig_ephemeral.this.kubernetes_client_configuration.client_key)
}

# Try to create a real Kubernetes resource to prove the connection works
resource "kubernetes_namespace" "test" {
  metadata {
    name = "test-ephemeral-kubeconfig"
  }
}