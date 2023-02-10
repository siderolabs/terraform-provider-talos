## [terraform-provider-talos 0.1.1](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.1) (2023-02-10)

Welcome to the v0.1.1 release of terraform-provider-talos!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.3.1


### Contributors

* Noel Georgi
* Dmitriy Matrenichev
* Robert Wunderer
* Spencer Smith

### Changes
<details><summary>5 commits</summary>
<p>

* [`fc424ec`](https://github.com/siderolabs/terraform-provider-talos/commit/fc424ece36d0c174413504766a8b9525acf375d9) fix: force new talosconfig if endpoints or nodes change
* [`b6f1fdd`](https://github.com/siderolabs/terraform-provider-talos/commit/b6f1fdd22f0a5b229e75f7296d4a5d3e053bda97) release(v0.1.0): prepare release
* [`ef1c72c`](https://github.com/siderolabs/terraform-provider-talos/commit/ef1c72cc149c7d02c3cc19e7d299b2adefab118b) docs: re-word `talos_version`
* [`086b1e1`](https://github.com/siderolabs/terraform-provider-talos/commit/086b1e10c6aaeb71ff90dfd2f2feef6e99a1c294) chore: bump deps
* [`acee3f9`](https://github.com/siderolabs/terraform-provider-talos/commit/acee3f9ff414a0620795b424481c2c37aa272aea) docs: clarify meaning of `talos_version` in `machine_configuration` resources
</p>
</details>

### Changes since v0.1.0
<details><summary>1 commit</summary>
<p>

* [`fc424ec`](https://github.com/siderolabs/terraform-provider-talos/commit/fc424ece36d0c174413504766a8b9525acf375d9) fix: force new talosconfig if endpoints or nodes change
</p>
</details>

### Changes from siderolabs/gen
<details><summary>1 commit</summary>
<p>

* [`214c1ef`](https://github.com/siderolabs/gen/commit/214c1efe795cf426e5ebcc48cb305bfc7a16fdb8) chore: set `slice.Filter` result slice cap to len
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/go-cty**                d3edf31b6320 -> 85980079f637
* **github.com/siderolabs/gen**                  v0.4.2 -> v0.4.3
* **github.com/siderolabs/talos/pkg/machinery**  v1.3.0 -> v1.3.1

Previous release can be found at [v0.1.0-beta.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-beta.0)

## [terraform-provider-talos 0.1.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0) (2023-01-04)

Welcome to the v0.1.0 release of terraform-provider-talos!



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.3.1


### Contributors

* Noel Georgi
* Dmitriy Matrenichev
* Robert Wunderer

### Changes
<details><summary>3 commits</summary>
<p>

* [`ef1c72c`](https://github.com/siderolabs/terraform-provider-talos/commit/ef1c72cc149c7d02c3cc19e7d299b2adefab118b) docs: re-word `talos_version`
* [`086b1e1`](https://github.com/siderolabs/terraform-provider-talos/commit/086b1e10c6aaeb71ff90dfd2f2feef6e99a1c294) chore: bump deps
* [`acee3f9`](https://github.com/siderolabs/terraform-provider-talos/commit/acee3f9ff414a0620795b424481c2c37aa272aea) docs: clarify meaning of `talos_version` in `machine_configuration` resources
</p>
</details>

### Changes from siderolabs/gen
<details><summary>1 commit</summary>
<p>

* [`214c1ef`](https://github.com/siderolabs/gen/commit/214c1efe795cf426e5ebcc48cb305bfc7a16fdb8) chore: set `slice.Filter` result slice cap to len
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/go-cty**                d3edf31b6320 -> 85980079f637
* **github.com/siderolabs/gen**                  v0.4.2 -> v0.4.3
* **github.com/siderolabs/talos/pkg/machinery**  v1.3.0 -> v1.3.1

Previous release can be found at [v0.1.0-beta.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-beta.0)

## [terraform-provider-talos 0.1.0-beta.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-beta.0) (2022-12-16)

Welcome to the v0.1.0-beta.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.3.0


### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`a53881b`](https://github.com/siderolabs/terraform-provider-talos/commit/a53881babbb6849024a12cd6840b5070f4cbc5b0) feat: bump Talos sdk to v1.3.0
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/talos/pkg/machinery**  v1.3.0 **_new_**

Previous release can be found at [v0.1.0-alpha.12](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.12)

## [terraform-provider-talos 0.1.0-alpha.12](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.12) (2022-12-16)

Welcome to the v0.1.0-alpha.12 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Fixes

Fixed an issue with the provider when using a secure Talos client.


### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`3c80f59`](https://github.com/siderolabs/terraform-provider-talos/commit/3c80f59ad7a8514b5481c84ca0d210c62ea06bcc) fix: handling talos secure client
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.11](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.11)

## [terraform-provider-talos 0.1.0-alpha.11](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.11) (2022-12-09)

Welcome to the v0.1.0-alpha.11 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.7


### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.10](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.10)

## [terraform-provider-talos 0.1.0-alpha.10](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.10) (2022-11-02)

Welcome to the v0.1.0-alpha.10 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.6


### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.9](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.9)

## [terraform-provider-talos 0.1.0-alpha.9](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.9) (2022-10-18)

Welcome to the v0.1.0-alpha.9 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.5


### Contributors


### Changes
<details><summary>0 commit</summary>
<p>

</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.8](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.8)

## [terraform-provider-talos 1.2.4](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v1.2.4) (2022-10-18)

Welcome to the v1.2.4 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Component Updates

Talos sdk: v1.2.4


### Contributors

* Dmitriy Matrenichev
* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`1c7975d`](https://github.com/siderolabs/terraform-provider-talos/commit/1c7975d1392406fa10a1a225e426e005d699c73e) chore: move to `gen` go pkg
</p>
</details>

### Changes from siderolabs/gen
<details><summary>2 commits</summary>
<p>

* [`338a650`](https://github.com/siderolabs/gen/commit/338a65065f92eb6426a66c4a88a0cc02cc02e529) chore: add initial implementation and documentation
* [`4fd8667`](https://github.com/siderolabs/gen/commit/4fd866707052c792a6adccbc28efec5debdd18a8) Initial commit
</p>
</details>

### Dependency Changes

* **github.com/siderolabs/gen**  v0.1.0 **_new_**

Previous release can be found at [v0.1.0-alpha.7](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.7)

## [terraform-provider-talos 0.1.0-alpha.7](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.7) (2022-09-20)

Welcome to the v0.1.0-alpha.7 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Contributors

* Noel Georgi

### Changes
<details><summary>1 commit</summary>
<p>

* [`79594a6`](https://github.com/siderolabs/terraform-provider-talos/commit/79594a60b231966f68be2e39c01990e209176d6b) chore: bump talos to v1.2.3
</p>
</details>

### Dependency Changes

This release has no dependency changes

Previous release can be found at [v0.1.0-alpha.6](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.6)

## [terraform-provider-talos 0.1.0-alpha.6](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.6) (2022-09-15)

Welcome to the v0.1.0-alpha.6 release of terraform-provider-talos!  
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

Previous release can be found at [v0.1.0-alpha.5](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.0-alpha.5)

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

