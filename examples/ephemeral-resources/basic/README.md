# Basic Ephemeral Resources Example

This example demonstrates the fundamental pattern for using ephemeral resources with the Talos provider and HashiCorp Vault.

## What This Example Shows

- How to store machine secrets in Vault (one-time setup)
- How to retrieve machine secrets ephemerally from Vault
- How to generate machine configurations without storing them in state
- How to use write-only attributes to prevent secrets in state
- How to retrieve kubeconfig ephemerally

## Prerequisites

- **Terraform 1.11+** or **OpenTofu 1.11+** (required for ephemeral resources and write-only attributes)
- Talos provider 0.11.0 or later
- HashiCorp Vault provider 5.0+
- Vault instance accessible from Terraform
- Talos node at 10.5.0.2 (or modify variables)

## Setup Instructions

### Step 1: Configure Vault Provider

Ensure your Vault provider is configured with appropriate credentials:

```bash
export VAULT_ADDR="https://vault.example.com:8200"
export VAULT_TOKEN="your-vault-token"
```

Or configure in your Terraform:

```terraform
provider "vault" {
  address = "https://vault.example.com:8200"
}
```

### Step 2: Apply the Configuration

Simply run:

```bash
terraform init
terraform apply
```

**That's it!** On the first run, Terraform will:
1. Generate machine secrets ephemerally
2. Store them in Vault at `secret/data/talos-example-cluster`
3. Retrieve them from Vault for immediate use
4. Configure and bootstrap your Talos cluster

All in a single apply, thanks to Terraform's automatic dependency resolution.

### Step 3: Verify Deterministic Behavior

Run `terraform plan` again:

```bash
terraform plan
```

You should see no changes. The secrets are now stable in Vault and retrieved ephemerally on each run.

## Understanding the Pattern

### Why Vault?

Without Vault:
- `ephemeral "talos_machine_secrets"` would generate NEW secrets every run
- Cluster access would be lost
- All dependent resources would show changes

With Vault:
- Secrets stored once on first run, retrieved ephemerally thereafter
- Stable, deterministic infrastructure
- No secrets in Terraform state
- Works seamlessly in a single apply

### What's Ephemeral?

- **Machine secrets generation**: Only evaluated on first run (stored in Vault with hardcoded version)
- **Machine secrets retrieval**: Retrieved from Vault on every run (never stored in state)
- **Client configuration**: Retrieved from Vault on every run (never stored in state)
- **Machine configuration**: Generated fresh each time (but deterministically from stable secrets)
- **Kubeconfig**: Retrieved fresh each time from the cluster

### The Magic of Hardcoded Version

The `data_json_wo_version = 1` in the Vault resource is key:
- On first run: Secret doesn't exist, so Terraform generates it and stores version 1
- On subsequent runs: Secret version 1 exists and matches, so no update needed
- Since no update is needed, the `ephemeral "talos_machine_secrets"` isn't evaluated
- This prevents regenerating secrets while keeping all code in place

## Outputs

The example includes an ephemeral output for the kubeconfig:

```terraform
output "kubeconfig" {
  value     = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
  sensitive = true
  ephemeral = true
}
```

Access it with:

```bash
terraform output -raw kubeconfig > kubeconfig.yaml
export KUBECONFIG=kubeconfig.yaml
kubectl get nodes
```

## Troubleshooting

### "No secret found at path"

Ensure you've completed Step 2 to store secrets in Vault first.

### "Resources keep changing on every plan"

Check if the `data_json_wo_version` in the `vault_kv_secret_v2` resource is properly hardcoded. It should be set to a fixed value (e.g., `1`) to prevent regeneration.

### "Cannot connect to cluster"

Verify the secrets in Vault match what was originally used to configure the cluster. If you regenerated secrets, you'll need to reconfigure all cluster nodes.
