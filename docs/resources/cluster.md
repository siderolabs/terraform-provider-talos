---
page_title: "talos_cluster Resource - talos"
subcategory: ""
description: |-
  Manages a Talos cluster: bootstraps etcd and tracks Kubernetes version. This resource completes once the Talos layer (etcd, apid, kubelet) is healthy across all control plane nodes. It does not wait for Kubernetes components to be ready — use talos_cluster_health for that before depending on the Kubernetes API.
---

# talos_cluster (Resource)

Manages a Talos cluster: bootstraps etcd and tracks Kubernetes version. This resource completes once the Talos layer (etcd, apid, kubelet) is healthy across all control plane nodes. It does not wait for Kubernetes components to be ready — use talos_cluster_health for that before depending on the Kubernetes API.

## Example Usage

```terraform
resource "talos_machine_secrets" "this" {}

data "talos_machine_configuration" "this" {
  cluster_name       = "example-cluster"
  machine_type       = "controlplane"
  cluster_endpoint   = "https://10.5.0.2:6443"
  machine_secrets    = talos_machine_secrets.this.machine_secrets
  kubernetes_version = "v1.32.0"
}

resource "talos_machine_configuration_apply" "this" {
  client_configuration        = talos_machine_secrets.this.client_configuration
  machine_configuration_input = data.talos_machine_configuration.this.machine_configuration
  node                        = "10.5.0.2"
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

resource "talos_cluster" "this" {
  depends_on           = [talos_machine_configuration_apply.this]
  node                 = "10.5.0.2"
  client_configuration = talos_machine_secrets.this.client_configuration
  kubernetes_version   = "v1.32.0"
}

data "talos_cluster_kubeconfig" "this" {
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = "10.5.0.2"
  depends_on           = [talos_cluster.this]
}
```

## Upgrading Kubernetes

When `kubernetes_version` changes, `talos_cluster` runs Talos's `upgrade-k8s` procedure: it pre-pulls images, upgrades static pods component-by-component with health gating, updates the kube-proxy DaemonSet, upgrades kubelet sequentially across nodes, and re-applies bootstrap manifests.

The `kubernetes_version` argument in `talos_machine_configuration` controls the image tags embedded in the generated YAML. Without `ignore_kubernetes_upgrade_drift = true` on `talos_machine`, bumping it causes `talos_machine` to re-apply the config with the new image tags directly — bypassing `upgrade-k8s`'s sequencing. With the flag set, bumping this value has no effect on running nodes; only `talos_cluster.kubernetes_version` drives the actual upgrade. Both can then be changed in the same `terraform apply` (see [`talos_machine`](machine.md)).

`kubernetes_version` in `talos_machine_configuration` still matters for scale-up: new nodes bootstrap at that version. Keep it in sync with `talos_cluster.kubernetes_version`.

## Exposing the cluster CA certificates

`kubernetes_ca_certificate`, `talos_ca_certificate`, `etcd_ca_certificate` and `kubernetes_aggregator_ca_certificate` are the CA certificates of the cluster, computed from the optional `machine_secrets` or `machine_secrets_wo` argument. Terraform does not let a value derived from an ephemeral value flow into a regular argument, so the CA certificates of a secrets bundle read from an ephemeral source, a secret manager for example, cannot be used elsewhere. Passing the bundle to `machine_secrets_wo` makes them ordinary attributes, already known at plan time. Only the certificates are exposed, never a key.

The bundle must be the one the cluster was created with. An ephemeral `talos_machine_secrets` generates a new bundle each time it is opened, so it is not a source for an existing cluster.

```terraform
# The secrets bundle and the client configuration the cluster was created with,
# as ephemeral values, read from a secret manager for example.
variable "machine_secrets" {
  type      = any
  ephemeral = true
  sensitive = true
}

variable "client_configuration" {
  type      = any
  ephemeral = true
  sensitive = true
}

resource "talos_cluster" "this" {
  node                    = "10.5.0.2"
  client_configuration_wo = var.client_configuration
  machine_secrets_wo      = var.machine_secrets
  kubernetes_version      = "v1.32.0"
}

resource "vault_kubernetes_auth_backend_config" "this" {
  kubernetes_host    = "https://10.5.0.2:6443"
  kubernetes_ca_cert = base64decode(talos_cluster.this.kubernetes_ca_certificate)
}
```

The certificates are base64 encoded, like the certificates in the secrets bundle. A field can hold several certificates, for example the old and the new CA during a rotation. All of them must be valid CA certificates. When the bundle changes, for example after a CA rotation, the certificates that changed show as a normal in-place change and do not start a Kubernetes upgrade. `machine_secrets` does the same but stores the whole bundle in the state, including the CA private keys and tokens (marked sensitive, but still in the state file), so prefer `machine_secrets_wo`. Without either argument, these attributes are null.

<!-- schema generated by tfplugindocs -->
## Schema

### Required

- `kubernetes_version` (String) Desired Kubernetes version (e.g. `v1.32.0`). This is the canonical driver for Kubernetes upgrades: changing it runs Talos's `upgrade-k8s` procedure, which sequentially upgrades kube-apiserver, kube-controller-manager, kube-scheduler, kube-proxy, and kubelet on each node with health gating. Set `ignore_kubernetes_upgrade_drift = true` on `talos_machine` so that image tag changes owned by `upgrade-k8s` are excluded from drift detection and `talos_cluster` fully owns the upgrade sequencing.
- `node` (String) The IP address of the control plane node to bootstrap etcd on. Must be an IP: it defaults into `control_plane_nodes`, which the health checks compare against the addresses Kubernetes reports. Use `endpoint` to connect through a hostname.

### Optional

> **NOTE**: [Write-only arguments](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments) are supported in Terraform 1.11 and later.

- `client_configuration` (Attributes) The Talos client configuration. Use client_configuration_wo when using ephemeral resources. (see [below for nested schema](#nestedatt--client_configuration))
- `client_configuration_wo` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of client_configuration for use with ephemeral resources. Requires Terraform 1.11+. (see [below for nested schema](#nestedatt--client_configuration_wo))
- `control_plane_nodes` (List of String) List of all control plane node IPs used for etcd health checks. Defaults to [node]. Required for HA clusters where all control plane IPs must be listed.
- `endpoint` (String) The endpoint to use when connecting to the node. Defaults to node.
- `machine_secrets` (Attributes) Optional machine secrets bundle of the cluster, used only to compute the public CA certificates exposed by this resource (`kubernetes_ca_certificate` and the other `*_ca_certificate` attributes). The whole bundle is stored in the state: its keys and tokens are marked sensitive but are still in the state file. Use `machine_secrets_wo` to keep it out of the state. Any change to the bundle is shown as an in-place update. Only one of `machine_secrets` and `machine_secrets_wo` can be set. (see [below for nested schema](#nestedatt--machine_secrets))
- `machine_secrets_wo` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) Write-only variant of `machine_secrets` for use with ephemeral resources. Requires Terraform 1.11+. (see [below for nested schema](#nestedatt--machine_secrets_wo))
- `timeouts` (Attributes) (see [below for nested schema](#nestedatt--timeouts))

### Read-Only

- `etcd_ca_certificate` (String) The etcd CA certificate, PEM encoded and then base64 encoded like the certificates of a secrets bundle. It is `certs.etcd.cert` of `machine_secrets` or `machine_secrets_wo`, known at plan time, so it can be used where a value derived from an ephemeral resource cannot. Null when neither is set. Only the certificate is exposed, never the key.
- `id` (String) The ID of this resource.
- `kubernetes_aggregator_ca_certificate` (String) The Kubernetes aggregator CA certificate, PEM encoded and then base64 encoded like the certificates of a secrets bundle. It is `certs.k8s_aggregator.cert` of `machine_secrets` or `machine_secrets_wo`, known at plan time, so it can be used where a value derived from an ephemeral resource cannot. Null when neither is set. Only the certificate is exposed, never the key.
- `kubernetes_ca_certificate` (String) The Kubernetes CA certificate, PEM encoded and then base64 encoded like the certificates of a secrets bundle. It is `certs.k8s.cert` of `machine_secrets` or `machine_secrets_wo`, known at plan time, so it can be used where a value derived from an ephemeral resource cannot. Null when neither is set. Only the certificate is exposed, never the key.
- `talos_ca_certificate` (String) The Talos CA certificate, PEM encoded and then base64 encoded like the certificates of a secrets bundle. It is `certs.os.cert` of `machine_secrets` or `machine_secrets_wo`, known at plan time, so it can be used where a value derived from an ephemeral resource cannot. Null when neither is set. Only the certificate is exposed, never the key.

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


<a id="nestedatt--machine_secrets"></a>
### Nested Schema for `machine_secrets`

Required:

- `certs` (Attributes) The certs for the talos kubernetes cluster (see [below for nested schema](#nestedatt--machine_secrets--certs))
- `cluster` (Attributes) The cluster secrets (see [below for nested schema](#nestedatt--machine_secrets--cluster))
- `secrets` (Attributes) The secrets for the talos kubernetes cluster (see [below for nested schema](#nestedatt--machine_secrets--secrets))
- `trustdinfo` (Attributes) The trustd info for the talos kubernetes cluster (see [below for nested schema](#nestedatt--machine_secrets--trustdinfo))

<a id="nestedatt--machine_secrets--certs"></a>
### Nested Schema for `machine_secrets.certs`

Required:

- `etcd` (Attributes) The etcd CA (see [below for nested schema](#nestedatt--machine_secrets--certs--etcd))
- `k8s` (Attributes) The Kubernetes CA (see [below for nested schema](#nestedatt--machine_secrets--certs--k8s))
- `k8s_aggregator` (Attributes) The Kubernetes aggregator CA (see [below for nested schema](#nestedatt--machine_secrets--certs--k8s_aggregator))
- `k8s_serviceaccount` (Attributes) The Kubernetes service account key (see [below for nested schema](#nestedatt--machine_secrets--certs--k8s_serviceaccount))
- `os` (Attributes) The Talos OS CA (see [below for nested schema](#nestedatt--machine_secrets--certs--os))

<a id="nestedatt--machine_secrets--certs--etcd"></a>
### Nested Schema for `machine_secrets.certs.etcd`

Required:

- `cert` (String) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive) The key, PEM encoded and then base64 encoded.


<a id="nestedatt--machine_secrets--certs--k8s"></a>
### Nested Schema for `machine_secrets.certs.k8s`

Required:

- `cert` (String) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive) The key, PEM encoded and then base64 encoded.


<a id="nestedatt--machine_secrets--certs--k8s_aggregator"></a>
### Nested Schema for `machine_secrets.certs.k8s_aggregator`

Required:

- `cert` (String) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive) The key, PEM encoded and then base64 encoded.


<a id="nestedatt--machine_secrets--certs--k8s_serviceaccount"></a>
### Nested Schema for `machine_secrets.certs.k8s_serviceaccount`

Required:

- `key` (String, Sensitive) The key for the k8s service account


<a id="nestedatt--machine_secrets--certs--os"></a>
### Nested Schema for `machine_secrets.certs.os`

Required:

- `cert` (String) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive) The key, PEM encoded and then base64 encoded.



<a id="nestedatt--machine_secrets--cluster"></a>
### Nested Schema for `machine_secrets.cluster`

Required:

- `id` (String) The cluster id
- `secret` (String, Sensitive) The cluster secret


<a id="nestedatt--machine_secrets--secrets"></a>
### Nested Schema for `machine_secrets.secrets`

Required:

- `bootstrap_token` (String, Sensitive) The bootstrap token for the talos kubernetes cluster
- `secretbox_encryption_secret` (String, Sensitive) The secretbox encryption secret for the talos kubernetes cluster

Optional:

- `aescbc_encryption_secret` (String, Sensitive) The aescbc encryption secret for the talos kubernetes cluster


<a id="nestedatt--machine_secrets--trustdinfo"></a>
### Nested Schema for `machine_secrets.trustdinfo`

Required:

- `token` (String, Sensitive) The trustd token for the talos kubernetes cluster



<a id="nestedatt--machine_secrets_wo"></a>
### Nested Schema for `machine_secrets_wo`

Required:

- `certs` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The certs for the talos kubernetes cluster (see [below for nested schema](#nestedatt--machine_secrets_wo--certs))
- `cluster` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The cluster secrets (see [below for nested schema](#nestedatt--machine_secrets_wo--cluster))
- `secrets` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The secrets for the talos kubernetes cluster (see [below for nested schema](#nestedatt--machine_secrets_wo--secrets))
- `trustdinfo` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The trustd info for the talos kubernetes cluster (see [below for nested schema](#nestedatt--machine_secrets_wo--trustdinfo))

<a id="nestedatt--machine_secrets_wo--certs"></a>
### Nested Schema for `machine_secrets_wo.certs`

Required:

- `etcd` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The etcd CA (see [below for nested schema](#nestedatt--machine_secrets_wo--certs--etcd))
- `k8s` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The Kubernetes CA (see [below for nested schema](#nestedatt--machine_secrets_wo--certs--k8s))
- `k8s_aggregator` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The Kubernetes aggregator CA (see [below for nested schema](#nestedatt--machine_secrets_wo--certs--k8s_aggregator))
- `k8s_serviceaccount` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The Kubernetes service account key (see [below for nested schema](#nestedatt--machine_secrets_wo--certs--k8s_serviceaccount))
- `os` (Attributes, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The Talos OS CA (see [below for nested schema](#nestedatt--machine_secrets_wo--certs--os))

<a id="nestedatt--machine_secrets_wo--certs--etcd"></a>
### Nested Schema for `machine_secrets_wo.certs.etcd`

Required:

- `cert` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The key, PEM encoded and then base64 encoded.


<a id="nestedatt--machine_secrets_wo--certs--k8s"></a>
### Nested Schema for `machine_secrets_wo.certs.k8s`

Required:

- `cert` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The key, PEM encoded and then base64 encoded.


<a id="nestedatt--machine_secrets_wo--certs--k8s_aggregator"></a>
### Nested Schema for `machine_secrets_wo.certs.k8s_aggregator`

Required:

- `cert` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The key, PEM encoded and then base64 encoded.


<a id="nestedatt--machine_secrets_wo--certs--k8s_serviceaccount"></a>
### Nested Schema for `machine_secrets_wo.certs.k8s_serviceaccount`

Required:

- `key` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The key for the k8s service account


<a id="nestedatt--machine_secrets_wo--certs--os"></a>
### Nested Schema for `machine_secrets_wo.certs.os`

Required:

- `cert` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The certificate, PEM encoded and then base64 encoded.
- `key` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The key, PEM encoded and then base64 encoded.



<a id="nestedatt--machine_secrets_wo--cluster"></a>
### Nested Schema for `machine_secrets_wo.cluster`

Required:

- `id` (String, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The cluster id
- `secret` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The cluster secret


<a id="nestedatt--machine_secrets_wo--secrets"></a>
### Nested Schema for `machine_secrets_wo.secrets`

Required:

- `bootstrap_token` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The bootstrap token for the talos kubernetes cluster
- `secretbox_encryption_secret` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The secretbox encryption secret for the talos kubernetes cluster

Optional:

- `aescbc_encryption_secret` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The aescbc encryption secret for the talos kubernetes cluster


<a id="nestedatt--machine_secrets_wo--trustdinfo"></a>
### Nested Schema for `machine_secrets_wo.trustdinfo`

Required:

- `token` (String, Sensitive, [Write-only](https://developer.hashicorp.com/terraform/language/resources/ephemeral#write-only-arguments)) The trustd token for the talos kubernetes cluster



<a id="nestedatt--timeouts"></a>
### Nested Schema for `timeouts`

Optional:

- `create` (String) A string that can be [parsed as a duration](https://pkg.go.dev/time#ParseDuration) consisting of numbers and unit suffixes, such as "30s" or "2h45m". Valid time units are "s" (seconds), "m" (minutes), "h" (hours).
- `update` (String) A string that can be [parsed as a duration](https://pkg.go.dev/time#ParseDuration) consisting of numbers and unit suffixes, such as "30s" or "2h45m". Valid time units are "s" (seconds), "m" (minutes), "h" (hours).
