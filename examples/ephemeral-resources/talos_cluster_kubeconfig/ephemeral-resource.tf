ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_cluster_kubeconfig" "this" {
  cluster_name    = "example-cluster"
  machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
  endpoint        = "https://10.5.0.2:6443"
}

# Recommended pattern for stable kubeconfig when storing in a secret manager:
# Persist not_before in terraform_data so the admin cert timestamps are fixed
# across plan invocations and kubeconfig_raw is byte-identical on every open.
resource "terraform_data" "kubeconfig_nbf" {
  input = plantimestamp()
  lifecycle {
    ignore_changes = [input]
  }
}

ephemeral "talos_cluster_kubeconfig" "stable" {
  cluster_name    = "example-cluster"
  machine_secrets = ephemeral.talos_machine_secrets.this.machine_secrets
  endpoint        = "https://10.5.0.2:6443"
  not_before      = terraform_data.kubeconfig_nbf.output
  crt_ttl         = "87600h"
}
