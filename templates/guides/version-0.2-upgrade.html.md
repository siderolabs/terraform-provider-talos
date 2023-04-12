---
page_title: "Terraform Talos Provider Version 0.2 Upgrade Guide"
description: |-
  Terraform Talos Provider Version 0.2 Upgrade Guide
---

# Terraform Talos Provider Version 0.2 Upgrade Guide <!-- omit in toc -->

Version 0.2 of the Talos Terraform provider is a major release and include some breaking chages. This guide will walk you through the changes and how to upgrade your Terraform configuration.

~> **NOTE:** Version 0.2 of the Talos Terraform provider drops support for the following resources:

> * `talos_client_configuration`
> * `talos_cluster_kubeconfig`
> * `talos_machine_configuration_controlplane`
> * `talos_machine_configuration_worker`

The following table lists the resources that have been removed and the new resources that replace them.

| Removed Resource                           | Type     | New Resource                  | Type          |
| ------------------------------------------ | -------- | ----------------------------- | ------------- |
| `talos_client_configuration`               | Resource | `talos_client_configuration`  | Data Source   |
| `talos_cluster_kubeconfig`                 | Resource | `talos_cluster_kubeconfig`    | Data Source   |
| `talos_machine_configuration_controlplane` | Resource | `talos_machine_configuration` | Data Resource |
| `talos_machine_configuration_worker`       | Resource | `talos_machine_configuration` | Data Resource |

## Upgrade topics: <!-- omit in toc -->

- [Upgrading `talos_client_configuration` resource](#upgrading-talos_client_configuration-resource)
- [Upgrading `talos_cluster_kubeconfig` resource](#upgrading-talos_cluster_kubeconfig-resource)
- [Upgrading `talos_machine_configuration_controlplane` resource](#upgrading-talos_machine_configuration_controlplane-resource)
- [Upgrading `talos_machine_configuration_worker` resource](#upgrading-talos_machine_configuration_worker-resource)

### Upgrading `talos_client_configuration` resource

The `talos_client_configuration` resource has been removed. The `talos_client_configuration` data source should be used instead.

For example if the following resource was used:

```hcl
resource "talos_machine_secrets" "this" {}

resource "talos_client_configuration" "talosconfig" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.this.machine_secrets
}
```

`talos_client_configuration` resource should be first removed from the state:

```bash
terraform state rm talos_client_configuration.talosconfig
```

and the code should be updated to:

```hcl
resource "talos_machine_secrets" "machine_secrets" {}

data "talos_client_configuration" "this" {
  cluster_name         = "example-cluster"
  client_configuration = talos_machine_secrets.this.client_configuration
}
```

### Upgrading `talos_cluster_kubeconfig` resource

The `talos_cluster_kubeconfig` resource has been removed. The `talos_cluster_kubeconfig` data source should be used instead.

For example if the following resource was used:

```hcl
resource "talos_machine_secrets" "this" {}

resource "talos_client_configuration" "this" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.this.machine_secrets
}

resource "talos_cluster_kubeconfig" "kubeconfig" {
  talos_config = talos_client_configuration.this.talos_config
  endpoint     = "10.5.0.2"
  node         = "10.5.0.2"
}
```

`talos_cluster_kubeconfig` resource should be first removed from the state:

```bash
terraform state rm talos_cluster_kubeconfig.kubeconfig
```

and the code should be updated to:

```hcl
resource "talos_machine_secrets" "machine_secrets" {}

data "talos_cluster_kubeconfig" "this" {
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = "10.5.0.2"
}
```

### Upgrading `talos_machine_configuration_controlplane` resource

The `talos_machine_configuration_controlplane` resource has been removed. The `talos_machine_configuration` data source should be used instead.

For example if the following resource was used:

```hcl
resource "talos_machine_secrets" "this" {}

resource "talos_client_configuration" "this" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_controlplane" "this" {
  cluster_name     = "example-cluster"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_apply" "this" {
  talos_config          = talos_client_configuration.this.talos_config
  machine_configuration = talos_machine_configuration_controlplane.this.machine_config
  node                  = "10.5.0.2"
  endpoint              = "10.5.0.2"
}
```

`talos_machine_configuration_controlplane` resource should be first removed from the state:

```bash
terraform state rm talos_machine_configuration_controlplane.cp
```

and the code should be updated to:

```hcl
resource "talos_machine_secrets" "machine_secrets" {}

data "talos_machine_configuration" "this" {
  cluster_name         = "example-cluster"
  cluster_endpoint     = "https://10.5.0.2:6443"
  machine_type         = "controlplane"
  talos_version        = talos_machine_secrets.this.talos_version
  machine_secrets      = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = "10.5.0.2"
}
```

### Upgrading `talos_machine_configuration_worker` resource

The `talos_machine_configuration_worker` resource has been removed. The `talos_machine_configuration` data source should be used instead.

For example if the following resource was used:

```hcl
resource "talos_machine_secrets" "this" {}

resource "talos_client_configuration" "this" {
  cluster_name    = "example-cluster"
  machine_secrets = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_worker" "this" {
  cluster_name     = "example-cluster"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_apply" "this" {
  talos_config          = talos_client_configuration.this.talos_config
  machine_configuration = talos_machine_configuration_worker.this.machine_config
  node                  = "10.5.0.3"
  endpoint              = "10.5.0.3"
}
```

`talos_machine_configuration_worker` resource should be first removed from the state:

```bash
terraform state rm talos_machine_configuration_worker.worker
```

and the code should be updated to:

```hcl
resource "talos_machine_secrets" "machine_secrets" {}

data "talos_machine_configuration" "this" {
  cluster_name         = "example-cluster"
  cluster_endpoint     = "https://10.5.0.2:6443"
  machine_type         = "worker"
  talos_version        = talos_machine_secrets.this.talos_version
  machine_secrets      = talos_machine_secrets.this.machine_secrets
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = "10.5.0.3"
}
```
