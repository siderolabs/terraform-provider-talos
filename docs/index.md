---
page_title: "Provider: Talos"
description: |-
  The Talos provider is used manage a Talos cluster config generation and initial setup.
---

# Talos Provider

Talos provider allows to generate configs for a Talos cluster and apply them to the nodes. bootstrap nodes and retrieve `kubeconfig` and `talosconfig`.

A complete example usage is available at [basic example](https://github.com/siderolabs/terraform-provider-talos/tree/release-0.1/examples/basic)

The same example is also rendered below:

`variables.tf`

```terraform
variable "cluster_name" {
  description = "A name to provide for the Talos cluster"
  type        = string
}

variable "cluster_endpoint" {
  description = "The endpoint for the Talos cluster"
  type        = string
}

variable "node_data" {
  description = "A map of node data"
  type = object({
    controlplanes = map(object({
      install_disk = string
      hostname     = optional(string)
    }))
    workers = map(object({
      install_disk = string
      hostname     = optional(string)
    }))
  })
  default = {
    controlplanes = {
      "10.5.0.2" = {
        install_disk = "/dev/sda"
      },
      "10.5.0.3" = {
        install_disk = "/dev/sda"
      },
      "10.5.0.4" = {
        install_disk = "/dev/sda"
      }
    }
    workers = {
      "10.5.0.5" = {
        install_disk = "/dev/nvme0n1"
        hostname     = "worker-1"
      },
      "10.5.0.6" = {
        install_disk = "/dev/nvme0n1"
        hostname     = "worker-2"
      }
    }
  }
}
```

`main.tf`

```terraform
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
```

`outputs.tf`

```terraform
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

output "kubeconfig" {
  value     = talos_cluster_kubeconfig.kubeconfig.kube_config
  sensitive = true
}
```

`files/patch.json`

```json
[
    {
        "op": "add",
        "path": "/cluster/allowSchedulingOnControlPlanes",
        "value": true
    }
]
```

`templates/patch.yaml.tmpl`

```yaml
machine:
  install:
    disk: ${install_disk}
  network:
    hostname: ${hostname}
```
