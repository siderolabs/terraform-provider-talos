---
page_title: "Using Ephemeral Resources - talos Provider"
subcategory: ""
description: |-
  Learn how to use ephemeral resources in the Talos provider to prevent secrets from being stored in Terraform state
---

# Using Ephemeral Resources in the Talos Provider

Ephemeral resources are Terraform resources that are essentially temporary. They allow you to access and use data in your configurations without that data being stored in Terraform state. This is particularly important for sensitive data like machine secrets, certificates, and kubeconfig files.

Ephemeral resources are available in Terraform v1.10 and later. For more information, see the [official HashiCorp documentation for Ephemeral Resources](https://developer.hashicorp.com/terraform/language/resources/ephemeral).

## Available Ephemeral Resources

The Talos provider includes five ephemeral resources:

- [`talos_machine_secrets`](https://registry.terraform.io/providers/siderolabs/talos/latest/docs/ephemeral-resources/machine_secrets) - Generate machine secrets without storing them in state
- [`talos_machine_configuration`](https://registry.terraform.io/providers/siderolabs/talos/latest/docs/ephemeral-resources/machine_configuration) - Generate machine configuration without storing secrets in state
- [`talos_client_configuration`](https://registry.terraform.io/providers/siderolabs/talos/latest/docs/ephemeral-resources/client_configuration) - Generate client configuration (talosconfig) without storing credentials in state
- [`talos_cluster_kubeconfig`](https://registry.terraform.io/providers/siderolabs/talos/latest/docs/ephemeral-resources/cluster_kubeconfig) - Retrieve kubeconfig without storing credentials in state
- [`talos_cluster_health`](https://registry.terraform.io/providers/siderolabs/talos/latest/docs/ephemeral-resources/cluster_health) - Check cluster health without storing credentials in state

These complement the existing data sources and resources, allowing you to avoid storing credentials and secret values in your Terraform state.

## Why Use Ephemeral Resources?

**Security Benefits:**

- Secrets never written to Terraform state files
- Reduces risk of credential exposure through state files
- Complies with security policies requiring secret-free state

**When to Use:**

- Generating Talos machine secrets
- Creating machine configurations with sensitive data
- Retrieving kubeconfig files
- Any workflow where secrets shouldn't persist in state

## Critical: Machine Secrets Persistence

**IMPORTANT**: Do not use `ephemeral "talos_machine_secrets"` without also storing them in a secret manager. Generating ephemeral machine secrets without persistence would create **new secrets on every Terraform run**, causing:

- Unpredictable changes to dependent resources
- Need to reconfigure all cluster nodes
- Loss of access to the cluster with previous credentials
- Non-deterministic infrastructure state

### Correct Pattern: Secret Manager Integration

Machine secrets should be:

1. Generated once and stored in a secret manager (Vault, AWS Secrets Manager, etc.)
2. Retrieved ephemerally from the secret manager when needed
3. Used to generate machine configurations deterministically

This ensures:

- Secrets remain stable across Terraform runs
- No secrets stored in Terraform state
- Deterministic, reproducible infrastructure
- Compliance with security policies

## Using Ephemeral Resources with Write-Only Attributes

Ephemeral resources are a source of ephemeral data, and they can be referenced in your configuration just like the attributes of resources and data sources. However, a field that references an ephemeral resource must be capable of handling ephemeral data.

The Talos provider includes write-only attributes that accept ephemeral values:

- `machine_configuration_input_wo` - Write-only alternative to `machine_configuration_input` on `talos_machine_configuration_apply` resource (requires Terraform 1.11+)

## Example: Using Vault for Secret Persistence

This example demonstrates the correct pattern for managing Talos machine secrets with Vault. Both secret generation and retrieval can coexist in the same configuration:

```terraform
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

# Step 1: Generate and store secrets in Vault
# The ephemeral resource generates secrets only when needed (first run)
# After initial creation, this won't be evaluated because data_json_wo_version is hardcoded
ephemeral "talos_machine_secrets" "this" {}

resource "vault_kv_secret_v2" "talos_secrets" {
  mount = "secret"
  name  = "talos-cluster-${var.cluster_name}"

  # Write-only attributes prevent secrets from being stored in Terraform state
  data_json_wo = jsonencode({
    machine_secrets      = ephemeral.talos_machine_secrets.this.machine_secrets
    client_configuration = ephemeral.talos_machine_secrets.this.client_configuration
  })
  # Hardcoded version prevents unnecessary refreshes after initial creation
  data_json_wo_version = 1
}

# Step 2: Retrieve secrets ephemerally from Vault
# This runs on every terraform operation but values are never stored in state
# Referencing the resource attributes creates implicit dependency on the secret
ephemeral "vault_kv_secret_v2" "talos_secrets" {
  mount = vault_kv_secret_v2.talos_secrets.mount
  name  = vault_kv_secret_v2.talos_secrets.name
}

locals {
  # Decode the secret data
  talos_data = jsondecode(ephemeral.vault_kv_secret_v2.talos_secrets.data_json)
}

# Step 3: Generate machine configuration using retrieved secrets
ephemeral "talos_machine_configuration" "controlplane" {
  cluster_name     = var.cluster_name
  cluster_endpoint = var.cluster_endpoint
  machine_type     = "controlplane"
  machine_secrets  = local.talos_data.machine_secrets
}

# Step 4: Apply configuration using write-only input
resource "talos_machine_configuration_apply" "controlplane" {
  client_configuration_wo        = local.talos_data.client_configuration
  machine_configuration_input_wo = ephemeral.talos_machine_configuration.controlplane.machine_configuration
  node                           = var.controlplane_node

  # Note: machine_configuration computed attribute will be null in state
  # This is expected behavior for secret-free operation
}
```

**Secret-Free Operation:**

When using write-only attributes (`_wo` variants), the provider ensures zero secrets leak into state:

- **Write-only inputs** (`client_configuration_wo`, `machine_configuration_input_wo`): Never stored in state
- **Computed outputs** (`machine_configuration`): Automatically set to null in state when using write-only inputs

The provider computes the machine configuration internally during apply operations without persisting it. This provides complete secret-free operation while maintaining full functionality.

**How This Works:**

1. **First Run**:
   - Secrets are generated ephemerally
   - Stored in Vault with version 1
   - Retrieved from Vault for immediate use
   - All in a single `terraform apply` (Terraform handles the ordering automatically)

2. **Subsequent Runs**:
   - The `vault_kv_secret_v2` resource doesn't need updates (version is hardcoded)
   - The `ephemeral "talos_machine_secrets"` isn't evaluated (no dependent resources need it)
   - Secrets are retrieved ephemerally from Vault for use in configuration

**Key Benefits:**

- Works in a single run on first apply
- Both blocks coexist permanently in your configuration
- Terraform handles all dependencies automatically
- No secrets stored in Terraform state

### Alternative Pattern: External Secret Generation

If you prefer to manage secret generation outside Terraform:

```bash
# Generate secrets manually using talosctl
talosctl gen secrets -o secrets.yaml

# Store in Vault using vault CLI
vault kv put secret/talos-cluster-prod \
  machine_secrets="$(yq -o=json '.machine_secrets' secrets.yaml)" \
  client_configuration="$(yq -o=json '.client_configuration' secrets.yaml)"
```

Then your Terraform configuration only needs the retrieval part:

```terraform
ephemeral "vault_kv_secret_v2" "talos_secrets" {
  mount = "secret"
  name  = "talos-cluster-prod"
}

locals {
  talos_data = jsondecode(ephemeral.vault_kv_secret_v2.talos_secrets.data_json)
}

ephemeral "talos_machine_configuration" "controlplane" {
  cluster_name     = "prod-cluster"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_type     = "controlplane"
  machine_secrets  = local.talos_data.machine_secrets
}
```

## Example: Generating Kubeconfig Ephemerally from Machine Secrets

This example shows how to generate a kubeconfig ephemerally from machine secrets stored in Vault.
The kubeconfig is generated locally from the Kubernetes CA key — no live cluster required.

### Simple usage (CA-pinned timestamps)

When `not_before` is omitted, the admin certificate validity window is taken from the K8s CA's
own timestamps (set once when the cluster was created). The output is byte-identical on every
plan as long as `machine_secrets` does not change.

```terraform
# Retrieve stored secrets from Vault
ephemeral "vault_kv_secret_v2" "talos_secrets" {
  mount = "secret"
  name  = "talos-cluster-prod"
}

locals {
  talos_data = jsondecode(ephemeral.vault_kv_secret_v2.talos_secrets.data_json)
}

# Generate kubeconfig without storing in state
ephemeral "talos_cluster_kubeconfig" "this" {
  cluster_name    = "prod-cluster"
  machine_secrets = local.talos_data.machine_secrets
  endpoint        = "https://10.5.0.2:6443"
}

# Output the kubeconfig (marked as ephemeral)
output "kubeconfig" {
  value     = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
  sensitive = true
  ephemeral = true
}
```

### Recommended pattern for Vault-backed workflows (explicit `not_before`)

When storing `kubeconfig_raw` in a Vault KV secret (or any resource that detects byte changes),
use a `terraform_data` resource to persist a stable `not_before` timestamp in Terraform state.
This pins the admin certificate validity window so `kubeconfig_raw` is byte-identical across
all plan invocations — no `ignore_changes` or `data_json_wo_version` bumps required until you
explicitly rotate the certificate.

```terraform
# Persist the admin cert NotBefore timestamp in regular Terraform state.
# Use ignore_changes so it is set once and never updated automatically.
# To rotate the cert: taint this resource and re-apply.
resource "terraform_data" "kubeconfig_nbf" {
  input = plantimestamp()
  lifecycle {
    ignore_changes = [input]
  }
}

# Generate kubeconfig with pinned timestamps
ephemeral "talos_cluster_kubeconfig" "this" {
  cluster_name    = "prod-cluster"
  machine_secrets = local.talos_data.machine_secrets
  endpoint        = "https://10.5.0.2:6443"
  not_before      = terraform_data.kubeconfig_nbf.output
  crt_ttl         = "87600h"
}

# Store kubeconfig in Vault — kubeconfig_raw is stable so this resource
# only updates when machine_secrets or not_before change.
resource "vault_kv_secret_v2" "kubeconfig" {
  mount = "secret"
  name  = "kubeconfig-prod-cluster"

  data_json_wo         = jsonencode({ kubeconfig = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw })
  data_json_wo_version = 1
}
```

**Certificate rotation**: taint `terraform_data.kubeconfig_nbf` to force a new `not_before`,
which produces a new cert and triggers a `data_json_wo_version` bump on the Vault secret.

**Note**: The kubeconfig is generated locally from the machine secrets and does not require
a running cluster.

## Alternative Secret Managers

While the examples above use HashiCorp Vault, you can use any secret manager that supports:

- Storing secrets via Terraform resources
- Retrieving secrets via ephemeral resources

### AWS Secrets Manager Example

```terraform
# Store secrets in AWS Secrets Manager
resource "aws_secretsmanager_secret" "talos_secrets" {
  name = "talos-cluster-${var.cluster_name}"
}

resource "aws_secretsmanager_secret_version" "talos_secrets" {
  secret_id = aws_secretsmanager_secret.talos_secrets.id
  secret_string = jsonencode({
    machine_secrets      = talos_machine_secrets.this.machine_secrets
    client_configuration = talos_machine_secrets.this.client_configuration
  })
}

# Note: AWS provider doesn't yet have ephemeral resources for Secrets Manager
# You would use a data source, which still stores in state
# Watch for AWS provider updates adding ephemeral support
```

### Azure Key Vault Example

```terraform
# Store secrets in Azure Key Vault
resource "azurerm_key_vault_secret" "talos_secrets" {
  name         = "talos-cluster-${var.cluster_name}"
  value        = jsonencode({
    machine_secrets      = talos_machine_secrets.this.machine_secrets
    client_configuration = talos_machine_secrets.this.client_configuration
  })
  key_vault_id = azurerm_key_vault.main.id
}

# Note: Azure provider doesn't yet have ephemeral resources for Key Vault
# You would use a data source for now
```

## Important Considerations

### Terraform Version Requirements

- **Terraform 1.10+**: Supports ephemeral resources only (no write-only attributes)
- **Terraform 1.11+**: Supports both ephemeral resources and write-only attributes
- **OpenTofu 1.11+**: Supports both ephemeral resources and write-only attributes
  - Note: OpenTofu 1.10 does NOT support ephemeral resources (they were introduced in 1.11)

**For the examples in this guide**: Terraform 1.11+ or OpenTofu 1.11+ required (uses write-only attributes)

### Compatibility with Existing Resources

Ephemeral resources complement existing data sources and resources:

- **Data sources** (e.g., `data.talos_machine_configuration`) - Still work, but store output in state
- **Ephemeral resources** (e.g., `ephemeral.talos_machine_configuration`) - Same functionality, no state storage

You can migrate existing configurations to ephemeral resources by:

1. Changing `data "talos_machine_configuration"` to `ephemeral "talos_machine_configuration"`
2. Updating references from `data.talos_machine_configuration.this` to `ephemeral.talos_machine_configuration.this`
3. Using write-only attributes (e.g., `machine_configuration_input_wo`) where applicable

## Migration Guide

### From Data Source to Ephemeral Resource with Vault

**Before (using data source):**

```terraform
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name     = "my-cluster"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_type     = "controlplane"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = "10.5.0.2"
}
```

**After (using ephemeral resources with Vault):**

Step 1 - Store existing secrets in Vault (one-time migration):

```terraform
# Assuming you have existing talos_machine_secrets resource
resource "vault_kv_secret_v2" "talos_secrets" {
  mount = "secret"
  name  = "talos-cluster-my-cluster"

  data_json_wo = jsonencode({
    machine_secrets      = talos_machine_secrets.this.machine_secrets
    client_configuration = talos_machine_secrets.this.client_configuration
  })
}
```

Step 2 - Use ephemeral resources to retrieve from Vault:

```terraform
# Retrieve secrets from Vault ephemerally
# Reference the resource to create implicit dependency
ephemeral "vault_kv_secret_v2" "talos_secrets" {
  mount = vault_kv_secret_v2.talos_secrets.mount
  name  = vault_kv_secret_v2.talos_secrets.name
}

locals {
  talos_data = jsondecode(ephemeral.vault_kv_secret_v2.talos_secrets.data_json)
}

# Generate configuration ephemerally
ephemeral "talos_machine_configuration" "this" {
  cluster_name     = "my-cluster"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_type     = "controlplane"
  machine_secrets  = local.talos_data.machine_secrets
}

# Apply configuration using write-only attribute
resource "talos_machine_configuration_apply" "this" {
  client_configuration_wo        = local.talos_data.client_configuration
  machine_configuration_input_wo = ephemeral.talos_machine_configuration.this.machine_configuration
  node                           = "10.5.0.2"
}
```

Step 3 - After verifying the migration works, remove the `talos_machine_secrets` resource from your state:

```bash
terraform state rm talos_machine_secrets.this
```

**Benefits:**

- Machine secrets stored securely in Vault, not in Terraform state
- Secrets remain stable across Terraform runs
- Machine configuration never stored in state
- Deterministic infrastructure state
- Improved security and compliance
