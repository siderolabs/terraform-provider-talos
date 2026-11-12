---
page_title: "talos_machine Resource - talos"
subcategory: ""
description: |-
  Manages a Talos node: applies machine configuration and keeps the Talos OS version in sync.
---

# talos_machine (Resource)

Manages a Talos node: applies machine configuration and keeps the Talos OS version in sync.

On every `terraform refresh`, the provider reads the running Talos version and the active machine configuration hash from the node. If either differs from what Terraform last wrote, the next `terraform apply` will reconcile the drift — re-applying configuration or upgrading the OS as needed.

## Example Usage

```terraform
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name     = "example-cluster"
  machine_type     = "controlplane"
  cluster_endpoint = "https://10.5.0.2:6443"
  machine_secrets  = talos_machine_secrets.this.machine_secrets
  config_patches = [
    yamlencode({
      machine = {
        install = {
          disk  = "/dev/sda"
          image = "ghcr.io/siderolabs/installer:v1.9.0"
        }
      }
    })
  ]
}

resource "talos_machine" "this" {
  node                  = "10.5.0.2"
  client_configuration  = talos_machine_secrets.this.client_configuration
  machine_configuration = data.talos_machine_configuration.this.machine_configuration
  image                 = "ghcr.io/siderolabs/installer:v1.9.0"
}

resource "talos_machine_bootstrap" "this" {
  depends_on           = [talos_machine.this]
  node                 = "10.5.0.2"
  client_configuration = talos_machine_secrets.this.client_configuration
}
```

## Upgrade example

Change `image` to trigger an in-place OS upgrade. Leave `talos_version` in the data source untouched: it is the config-generation contract pinned to the version the cluster was created with, not the installed Talos version, and it is independent of `image` — bumping it regenerates the machine configuration and is a separate, deliberate action, not something you do on every OS upgrade:

```terraform
data "talos_machine_configuration" "this" {
  # ...
  talos_version = "v1.10.0" # pinned to the version used to create the cluster; do not bump on every upgrade
}

resource "talos_machine" "this" {
  # ...
  image = "ghcr.io/siderolabs/installer:v1.11.0"
}
```

## Draining nodes before upgrade

When `drain_on_upgrade = true` (the default) and `image` is set, a kubeconfig must be supplied via `kubeconfig_wo` (or `kubeconfig`) so the provider can cordon and drain the node before rebooting:

```terraform
ephemeral "talos_cluster_kubeconfig" "this" {
  machine_secrets = talos_machine_secrets.this.machine_secrets
  cluster_name    = "mycluster"
  endpoint        = "https://<cp-ip>:6443"
}

resource "talos_machine" "worker" {
  # ...
  drain_on_upgrade = true
  kubeconfig_wo    = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
}
```

## Upgrading multiple nodes safely

When managing a multi-node cluster, upgrading all nodes in parallel risks losing etcd quorum. Use `depends_on` to chain upgrades sequentially, so each node is fully back before the next one starts:

```terraform
resource "talos_machine" "cp0" {
  node             = "10.5.0.1"
  image            = "ghcr.io/siderolabs/installer:v1.10.0"
  drain_on_upgrade = true
  # ...
}

resource "talos_machine" "cp1" {
  depends_on       = [talos_machine.cp0]
  node             = "10.5.0.2"
  image            = "ghcr.io/siderolabs/installer:v1.10.0"
  drain_on_upgrade = true
  # ...
}

resource "talos_machine" "cp2" {
  depends_on       = [talos_machine.cp1]
  node             = "10.5.0.3"
  image            = "ghcr.io/siderolabs/installer:v1.10.0"
  drain_on_upgrade = true
  # ...
}
```

Alternatively, use `terraform apply -parallelism=1` to force all resource operations to run one at a time without modifying `depends_on`.

## Restarting the control plane one node at a time

A configuration change that touches kube-apiserver, kube-controller-manager or kube-scheduler (extra args, audit policy, service account issuer, ...) makes Talos restart those static pods after `ApplyConfiguration` has already returned. Serializing the applies alone therefore still restarts every kube-apiserver at about the same time, and the cluster endpoint has no ready backend for that window.

Set `wait_for_control_plane = true` on control plane nodes to have `talos_machine` wait until the node's control plane pods run the applied configuration and are Ready again. Together with `depends_on` or `-parallelism=1` this restarts the control plane one node at a time:

```terraform
resource "talos_machine" "controlplane" {
  for_each = toset(["10.5.0.1", "10.5.0.2", "10.5.0.3"])

  node                   = each.value
  wait_for_control_plane = true
  kubeconfig_wo          = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw
  # ...
}
```

The wait only runs when the node's kube-apiserver was Ready before the apply, so the initial apply before etcd bootstrap and worker nodes are not affected. The wait never fails the apply: if it cannot run, for example because the kubeconfig is unusable, or the control plane is not Ready again within 10 minutes, Terraform shows a warning and continues. A change that does not touch the control plane adds a settle period of about 10 seconds: Talos re-renders the static pods asynchronously after the apply, so the wait first lets them settle. On Talos before 1.14, a change that makes the apply reboot the node is not waited for. With `wait_for_control_plane = true`, the default create and update timeouts grow by 11 minutes to cover the wait.

## Try mode

Set `try_mode = true` to apply configuration changes in Talos try mode, as `talosctl apply-config --mode=try` does. Talos then rolls a change back automatically unless the provider confirms it. The provider confirms the change once the node is still reachable through the Talos API 10 seconds after the apply. A change that cuts the node off from the provider, such as a network change that breaks the route between them, is rolled back after 1 minute and the apply fails, so the node keeps its previous configuration. Try mode only checks that the provider can still reach the node's Talos API, not that the node can still reach the rest of the cluster: with a load balancer or VIP endpoint, a node that lost its cluster network can still be confirmed.

Try mode adds about 10 seconds to every configuration change, and the default create and update timeouts grow by 1 minute. Do not use it for a change that moves the address the provider connects to (`endpoint` or `node`): the provider cannot confirm it and Talos rolls it back. Nodes in maintenance mode, such as during the first apply that installs Talos, are configured without try mode. On Talos before 1.14, a change that needs a reboot is applied without try mode and Terraform shows a warning.

## Kubernetes component image management

When used together with [`talos_cluster`](cluster.md), set `ignore_kubernetes_upgrade_drift = true` on `talos_machine`. This prevents `talos_machine` from re-applying the five Kubernetes component image fields managed by `upgrade-k8s`:

- `machine.kubelet.image`
- `cluster.apiServer.image`
- `cluster.controllerManager.image`
- `cluster.scheduler.image`
- `cluster.proxy.image`

[Talos's `upgrade-k8s` procedure](https://docs.siderolabs.com/kubernetes-guides/advanced-guides/upgrading-kubernetes/) upgrades these component-by-component and node-by-node with health gating. Applying them in parallel from `talos_machine` would bypass that safety procedure.

With `ignore_kubernetes_upgrade_drift = true`:

- `kubernetes_version` in `talos_machine_configuration` and `talos_cluster.kubernetes_version` can be bumped in the same `terraform apply` without conflict.
- Drift in these five fields is not detected by `talos_machine`. Only the version tag is stripped from the hash — a registry change (e.g. switching to a private mirror) is still detected as drift.
- `kubernetes_version` in `talos_machine_configuration` still matters at **bootstrap** — new nodes added later (scale-up) come up at that version. Keep it in sync with `talos_cluster.kubernetes_version`.

> **Experimental**: `ignore_kubernetes_upgrade_drift` is safe to use but cannot be guaranteed to work with all future versions of Talos. The list of image fields managed by `upgrade-k8s` may grow, and we cannot predict in advance which new fields will need to be added. That said, adding a new field is a trivial change, and enabling or disabling this attribute at any time is safe — the only consequence is a one-time apply to refresh the machine configuration hash in state.

If you use `talos_machine` without `talos_cluster`, leave `ignore_kubernetes_upgrade_drift` unset and run `talosctl upgrade-k8s` manually to upgrade Kubernetes.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `node` (String) The IP address or hostname of the Talos node.

### Optional

> **NOTE**: [Write-only arguments](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments) are supported in Terraform 1.11 and later.

- `client_configuration` (Attributes) The Talos client configuration. Use client_configuration_wo when using ephemeral resources. (see [below for nested schema](#nestedatt--client_configuration))
- `client_configuration_wo` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of client_configuration for use with ephemeral resources. Requires Terraform 1.11+. (see [below for nested schema](#nestedatt--client_configuration_wo))
- `drain_on_upgrade` (Boolean) Drain the node before rebooting during an upgrade, then uncordon after. Requires a healthy Kubernetes cluster. Use depends_on to sequence upgrades across nodes.
- `drain_timeout` (String) Maximum time to wait for pod eviction to complete when draining the node, as a Go duration string (e.g. "10m"). Only used when drain_on_upgrade = true. Raise this if pods take longer than 5m to evict, e.g. when a CSI driver like Longhorn relocates volume replicas. This runs inside the update (or create) timeout, so also raise `timeouts.update` (or `timeouts.create`) if it does not already leave enough headroom for drain_timeout.
- `endpoint` (String) The endpoint to use when connecting to the node. Defaults to node.
- `ignore_kubernetes_upgrade_drift` (Boolean) Experimental: when true, talos_machine ignores Kubernetes component image tag changes owned by talos_cluster/upgrade-k8s, preventing drift detection from interfering with graceful Kubernetes upgrades. Safe to use — enabling or disabling causes at most a one-time apply to refresh the config hash. Cannot be guaranteed to work with all future Talos versions: if upgrade-k8s manages additional image fields in a future release, this attribute must be updated to match.
- `image` (String) Talos installer image (e.g. `ghcr.io/siderolabs/installer:v1.9.0`). When set, upgrades if running version differs. When omitted, OS version is not managed.
- `kubeconfig` (String, Sensitive) Kubeconfig used to drain and uncordon the node during upgrades and for wait_for_control_plane. Required when drain_on_upgrade = true and image is set, or when wait_for_control_plane = true. Provide talos_cluster_kubeconfig.this.kubeconfig_raw. Use kubeconfig_wo when using ephemeral resources.
- `kubeconfig_wo` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of kubeconfig. Requires Terraform 1.11+.
- `machine_configuration` (String, Sensitive) The machine configuration YAML to apply. Use machine_configuration_wo when using ephemeral resources.
- `machine_configuration_wo` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of machine_configuration for use with ephemeral resources. Requires Terraform 1.11+.
- `on_destroy` (Attributes) Actions to be taken on destroy, if *reset* is not set this is a no-op.

> Note: Any changes to *on_destroy* block has to be applied first by running *terraform apply* first,
then a subsequent *terraform destroy* for the changes to take effect due to limitations in Terraform provider framework. (see [below for nested schema](#nestedatt--on_destroy))
- `reboot_mode` (String) Reboot mode for OS upgrades: DEFAULT or POWERCYCLE.
- `timeouts` (Attributes) (see [below for nested schema](#nestedatt--timeouts))
- `try_mode` (Boolean) Apply configuration changes to a running node in Talos try mode. Talos rolls the change back after 1 minute unless the provider confirms it, which the provider does once the node is still reachable through the Talos API 10 seconds after the apply. A change that cuts the node off from the provider is rolled back and the apply fails; reachability from other nodes is not checked. Adds about 10 seconds to every configuration change. Do not use it for changes that move the address the provider connects to (endpoint or node). Nodes in maintenance mode and, on Talos before 1.14, changes that need a reboot are applied without try mode.
- `wait_for_control_plane` (Boolean) After applying configuration to a control plane node whose kube-apiserver is Ready, wait until kube-apiserver, kube-controller-manager and kube-scheduler run the applied configuration and are Ready again. Combined with -parallelism=1 or depends_on between control plane nodes, this restarts the control plane one node at a time. Requires kubeconfig or kubeconfig_wo. No-op on workers and before the cluster is bootstrapped. The wait never fails the apply: if it cannot run or does not finish within 10 minutes, Terraform shows a warning. On Talos before 1.14, an apply that reboots the node does not wait.

### Read-Only

- `id` (String) The ID of this resource.
- `machine_configuration_hash` (String) SHA256 hex digest of the machine configuration currently applied on the node. Changes when configuration drifts, triggering a re-apply on the next `terraform apply`.

<a id="nestedatt--client_configuration"></a>
### Nested Schema for `client_configuration`

Required:

- `ca_certificate` (String) The client CA certificate.
- `client_certificate` (String) The client certificate.
- `client_key` (String, Sensitive) The client key.


<a id="nestedatt--client_configuration_wo"></a>
### Nested Schema for `client_configuration_wo`

Required:

- `ca_certificate` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The client CA certificate.
- `client_certificate` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The client certificate.
- `client_key` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The client key.


<a id="nestedatt--on_destroy"></a>
### Nested Schema for `on_destroy`

Optional:

- `graceful` (Boolean) Graceful indicates whether node should leave etcd before the reset.
- `reboot` (Boolean) Reboot indicates whether node should reboot or halt after resetting.
- `reset` (Boolean) Reset the machine to the initial state (STATE and EPHEMERAL will be wiped).


<a id="nestedatt--timeouts"></a>
### Nested Schema for `timeouts`

Optional:

- `create` (String) A string that can be [parsed as a duration](https://pkg.go.dev/time#ParseDuration) consisting of numbers and unit suffixes, such as "30s" or "2h45m". Valid time units are "s" (seconds), "m" (minutes), "h" (hours).
- `delete` (String) A string that can be [parsed as a duration](https://pkg.go.dev/time#ParseDuration) consisting of numbers and unit suffixes, such as "30s" or "2h45m". Valid time units are "s" (seconds), "m" (minutes), "h" (hours). Setting a timeout for a Delete operation is only applicable if changes are saved into state before the destroy operation occurs.
- `update` (String) A string that can be [parsed as a duration](https://pkg.go.dev/time#ParseDuration) consisting of numbers and unit suffixes, such as "30s" or "2h45m". Valid time units are "s" (seconds), "m" (minutes), "h" (hours).


## Import

Talos machines can be imported using the node IP address or hostname:

```shell
terraform import talos_machine.example 10.5.0.2
```

**The import ID must match `node` exactly.** Importing by IP address while using
a hostname in configuration, or vice versa, causes Terraform to plan replacement
of the machine.

Import records node identity only. Provide existing client credentials and the
desired machine configuration in HCL. The first plan shows an in-place update;
applying it fills state and reconciles the node. A following plan should be empty
when the configuration matches the node.

When `image` is set, the first apply compares the running Talos version and any
requested Image Factory schematic. Matches skip the OS upgrade; digest-pinned
images are installed because the running digest cannot be verified. Matching
versions for other repositories do not verify image content. Later changes to
`image` trigger an upgrade.

Matching machine configuration is not reapplied. A configuration-read failure
stops configuration apply, but does not undo an earlier OS upgrade.

Credentials must allow reading the active machine configuration through COSI
and, for Image Factory images, the node's extension statuses.
