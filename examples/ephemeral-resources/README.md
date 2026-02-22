# Ephemeral Resources Examples

This directory contains examples demonstrating how to use ephemeral resources in the Talos provider to prevent secrets from being stored in Terraform state.

## Prerequisites

- **Terraform 1.11+** or **OpenTofu 1.11+** (required for ephemeral resources and write-only attributes)
  - Note: Terraform 1.10 supports ephemeral resources but not write-only attributes
  - Note: OpenTofu 1.11 introduced both features together (1.10 does not support ephemeral resources)
- Talos provider 0.11.0 or later
- HashiCorp Vault (or another secret manager)
- Access to Talos nodes

## Important: Secret Manager Requirement

**These examples require a secret manager** (like HashiCorp Vault) to store machine secrets. Do not generate `ephemeral "talos_machine_secrets"` directly without persisting them, as this will create new secrets on every Terraform run, causing unpredictable infrastructure changes.

### Why Secret Manager Integration is Required

Machine secrets must remain stable throughout the cluster lifecycle. Without a secret manager:
- New secrets generated on every `terraform plan` or `apply`
- Dependent resources constantly show changes
- Loss of cluster access when secrets regenerate
- Non-deterministic infrastructure state

### Setup Process

1. **First time setup**: Generate and store machine secrets in Vault
2. **Regular usage**: Retrieve secrets ephemerally from Vault
3. **Always**: Use ephemeral machine configurations to avoid storing configs in state

See the [Using Ephemeral Resources Guide](../../docs/guides/using_ephemeral_resources.md) for detailed setup instructions.

## Examples

### Basic Example

The [basic](./basic/) example demonstrates:
- Storing machine secrets in Vault (initial setup)
- Retrieving machine secrets ephemerally from Vault
- Creating machine configuration without storing secrets
- Applying configuration using write-only attributes
- Retrieving kubeconfig ephemerally

## Key Benefits

1. **Security**: Secrets never written to Terraform state
2. **Stability**: Machine secrets remain constant across runs
3. **Compliance**: Meets security policies requiring secret-free state
4. **Deterministic**: Infrastructure state is reproducible and predictable

## Learn More

- [Using Ephemeral Resources Guide](../../docs/guides/using_ephemeral_resources.md)
- [Ephemeral Resources Documentation](../../docs/ephemeral-resources/)
- [Terraform Ephemeral Resources](https://developer.hashicorp.com/terraform/language/resources/ephemeral)
- [Vault Ephemeral Resources](https://registry.terraform.io/providers/hashicorp/vault/latest/docs/ephemeral-resources/kv_secret_v2)
