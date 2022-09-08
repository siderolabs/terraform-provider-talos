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

