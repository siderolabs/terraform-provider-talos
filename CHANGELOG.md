## [terraform-provider-talos 0.1.0-alpha.5](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.5) (2022-09-15)

Welcome to the v0.1.0-alpha.5 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider now requires `endpoint` and `node` to be set for `talos_machine_configuration_apply`, `talos_machine_bootstrap`, `talos_cluster_kubeconfig` resources.
The `endpoints` and `nodes` arguments are removed for the above resources.

This release also fixes a bug when multiple endpoitns were specified in the Talos client config.



### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.4](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.4)

## [terraform-provider-talos 0.1.0-alpha.4](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.4) (2022-09-15)

Welcome to the v0.1.0-alpha.4 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider now requires `endpoint` and `node` to be set for `talos_machine_configuration_apply`, `talos_machine_bootstrap`, `talos_cluster_kubeconfig` resources.
The `endpoints` and `nodes` arguments are removed for the above resources.

This release also fixes a bug when multiple endpoitns were specified in the Talos client config.



### Contributors

* Noel Georgi
* Nahuel Pastorale

### Changes
<details><summary>3 commits</summary>
<p>

* [`ea5a4ba`](https://github.com/siderolabs/terraform-provider-talos/commit/ea5a4bab1d86f20fc832fd4ffacf5978699a716e) release(v0.1.0-alpha.4): prepare release
* [`b9111db`](https://github.com/siderolabs/terraform-provider-talos/commit/b9111db42dade59000b26b313f7b7f0d5e6dc083) fix: client operations
* [`e2d04ac`](https://github.com/siderolabs/terraform-provider-talos/commit/e2d04acbf10eb092effa60a1cbc0eb27325d4b19) docs: fix wrong resource reference
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.3](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.3)

## [terraform-provider-talos 0.1.0-alpha.3](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.3) (2022-09-08)

Welcome to the v0.1.0-alpha.3 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Talos Provider

The Talos provider supports generating configs, applying them and bootstrap the nodes.

Resources supported:

* `talos_machine_secrets`
* `talos_client_configuration`
* `talos_machine_configuration_controlplane`
* `talos_machine_configuration_worker`
* `talos_machine_configuration_apply`
* `talos_machine_bootstrap`
* `talos_cluster_kubeconfig`

Data sources supported:

* `talos_client_configuration`
* `talos_cluster_kubeconfig`

Data sources will always create a diff and might be removed in a future release.


### Contributors

* Andrew Rynhard
* Andrey Smirnov
* Dmitriy Matrenichev
* Noel Georgi

### Changes
<details><summary>2 commits</summary>
<p>

* [`e29e302`](https://github.com/siderolabs/terraform-provider-talos/commit/e29e3028b0b5dc3c56757a7724d1400d4d98b3e8) chore: add release workflow
* [`4ecfd4f`](https://github.com/siderolabs/terraform-provider-talos/commit/4ecfd4f52353495a8304b0530d85a73b757861c9) feat: add new resources
</p>
</details>

### Changes from siderolabs/go-pointer
<details><summary>2 commits</summary>
<p>

* [`71ccdf0`](https://github.com/siderolabs/go-pointer/commit/71ccdf0d65330596f4def36da37625e4f362f2a9) chore: implement main functionality
* [`c1c3b23`](https://github.com/siderolabs/go-pointer/commit/c1c3b235d30cb0de97ed0645809f2b21af3b021e) Initial commit
</p>
</details>

### Dependency Changes

* **github.com/cosi-project/runtime**   v0.1.1 **_new_**
* **github.com/siderolabs/go-pointer**  v1.0.0 **_new_**
* **google.golang.org/grpc**            v1.48.0 **_new_**

Previous release can be found at [v0.1.0-alpha.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.2)

