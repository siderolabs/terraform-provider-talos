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

## Experimental worker upgrade budgets

For worker OS upgrades, an optional `upgrade_policy` coordinates admission across
`talos_machine` resources in the **same provider process**. This allows workers
managed with `for_each` to upgrade within a pool budget while unrelated resources
continue concurrently:

```terraform
locals {
  worker_node_names = toset(["worker-1", "worker-2", "worker-3", "worker-4"])
}

resource "talos_machine" "worker" {
  for_each = local.worker_node_names
  # node, client_configuration, machine_configuration, image, etc.

  drain_on_upgrade = true
  kubeconfig_wo    = ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw

  upgrade_policy = {
    group           = "general-workers"
    node_names      = local.worker_node_names
    max_unavailable = "25%" # one of these four workers; alternatively "1"
  }
}
```

Every participating resource must specify the same group, complete membership,
and budget. Membership uses Kubernetes node names, not Talos addresses, and must
include unchanged workers. Members must already exist. Groups are scoped by the
UID of the cluster's `kube-system` namespace and must not overlap. The kubeconfig
needs permission to list Nodes and get the `kube-system` Namespace in addition
to the permissions needed for draining.

A budget accepts a positive integer or a percentage. Percentages round down;
a percentage that permits zero workers is rejected. Existing NotReady, Unknown,
cordoned, and deleting workers consume capacity. Reservations count before a
node is cordoned and are counted only once if that node becomes unavailable.
Admission requires the candidate itself to be Ready and schedulable. Missing
members and Kubernetes API errors prevent admission. Control-plane-labeled
members are rejected; keep the explicit control-plane dependency chain above.

The reservation covers image preparation, drain, reboot, and the existing
recovery/uncordon checks. It is released only after a fresh Kubernetes check
confirms that the worker is Ready and schedulable. An upgrade or recovery failure
stops further admission in that group for the lifetime of the provider process;
already admitted upgrades can finish. Waiting for capacity consumes the resource's
update timeout. Investigate a failed node before starting another apply. Existing
drain behavior, including PodDisruptionBudget handling, remains unchanged.

**Scope and limitations:** this is process-local coordination, not a distributed
lock. Separate provider processes (including aliases hosted in separate processes)
and concurrent applies do not share reservations. Use one provider configuration
and one apply for a pool. Workers without this policy can bypass the budget.
External failures may exceed the budget, and Kubernetes health observations may
lag actual failures. Node readiness does not guarantee application readiness.

The policy covers only OS image updates of existing `talos_machine` resources.
It does not gate initial creation, destruction, configuration application,
Kubernetes upgrades, or hypervisor changes. Keep other disruptive operations in
separate maintenance steps; a simultaneous machine configuration change can run
after the OS upgrade reservation has been released. Changing only the policy does
not trigger an OS upgrade.

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
- `endpoint` (String) The endpoint to use when connecting to the node. Defaults to node.
- `ignore_kubernetes_upgrade_drift` (Boolean) Experimental: when true, talos_machine ignores Kubernetes component image tag changes owned by talos_cluster/upgrade-k8s, preventing drift detection from interfering with graceful Kubernetes upgrades. Safe to use — enabling or disabling causes at most a one-time apply to refresh the config hash. Cannot be guaranteed to work with all future Talos versions: if upgrade-k8s manages additional image fields in a future release, this attribute must be updated to match.
- `image` (String) Talos installer image (e.g. `ghcr.io/siderolabs/installer:v1.9.0`). When set, upgrades if running version differs. When omitted, OS version is not managed.
- `kubeconfig` (String, Sensitive) Kubeconfig used to drain and uncordon the node during upgrades. Required when drain_on_upgrade = true and image is set. Provide talos_cluster_kubeconfig.this.kubeconfig_raw. Use kubeconfig_wo when using ephemeral resources.
- `kubeconfig_wo` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of kubeconfig. Requires Terraform 1.11+.
- `machine_configuration` (String, Sensitive) The machine configuration YAML to apply. Use machine_configuration_wo when using ephemeral resources.
- `machine_configuration_wo` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of machine_configuration for use with ephemeral resources. Requires Terraform 1.11+.
- `on_destroy` (Attributes) Actions to be taken on destroy, if *reset* is not set this is a no-op.

> Note: Any changes to *on_destroy* block has to be applied first by running *terraform apply* first,
then a subsequent *terraform destroy* for the changes to take effect due to limitations in Terraform provider framework. (see [below for nested schema](#nestedatt--on_destroy))
- `reboot_mode` (String) Reboot mode for OS upgrades: DEFAULT or POWERCYCLE.
- `timeouts` (Attributes) (see [below for nested schema](#nestedatt--timeouts))
- `upgrade_policy` (Attributes) Experimental worker OS upgrade budget shared within one provider process. Requires drain_on_upgrade and kubeconfig. Does not coordinate separate provider processes or concurrent applies. Applies to image updates of existing nodes only. (see [below for nested schema](#nestedatt--upgrade_policy))

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


<a id="nestedatt--upgrade_policy"></a>
### Nested Schema for `upgrade_policy`

Required:

- `group` (String) Group name, scoped to the Kubernetes cluster. All members must use identical membership and budget settings.
- `max_unavailable` (String) Positive integer (for example 1) or percentage (for example 25%). Percentages round down and must permit at least one node. Existing NotReady, Unknown, deleting, or cordoned nodes count against the budget.
- `node_names` (Set of String) Complete set of Kubernetes worker node names, including unchanged nodes. Groups must not overlap. Nodes must already exist.
