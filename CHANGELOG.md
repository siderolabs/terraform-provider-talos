## [terraform-provider-talos 0.2.0-alpha.0](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.2.0-alpha.0) (2023-04-10)

Welcome to the v0.2.0-alpha.0 release of terraform-provider-talos!  
*This is a pre-release of terraform-provider-talos*



Please try out the release binaries and report any issues at
https://github.com/siderolabs/terraform-provider-talos/issues.

### Provider Changes

This version of the provider includes some breaking changes. Make sure to follow the provider upgrade guide at https://registry.terraform.io/providers/siderolabs/talos/latest/docs/guides/version-0.2-upgrade


### Component Updates

Talos sdk: v1.4.0-beta.0


### Contributors

* Andrey Smirnov
* Noel Georgi
* Alexey Palazhchenko
* Andrey Smirnov
* Spencer Smith
* Andrew Rynhard
* Artem Chernyshev
* Robert Wunderer
* Serge Logvinov

### Changes
<details><summary>10 commits</summary>
<p>

* [`6b2182b`](https://github.com/siderolabs/terraform-provider-talos/commit/6b2182be4015eb21d936930cfa108159b3fa16f6) chore: ci cleanup
* [`ea07caa`](https://github.com/siderolabs/terraform-provider-talos/commit/ea07caa6f39d159b286db76c525ed7cb1892f1e6) feat: code cleanup and tests
* [`4e5c210`](https://github.com/siderolabs/terraform-provider-talos/commit/4e5c210c219ddeb0ccf68cd80cb2988ce0ab5ccd) feat: use new tf sdk
* [`d1438b7`](https://github.com/siderolabs/terraform-provider-talos/commit/d1438b71d94b0ee0d0f3f5feb12317344c6e2a3e) chore: bump talos machinery
* [`aed502e`](https://github.com/siderolabs/terraform-provider-talos/commit/aed502e51945bad2493c521672a52111fcd1eca6) fix: state update required two runs
* [`dc00baa`](https://github.com/siderolabs/terraform-provider-talos/commit/dc00baad8d85e2786789ea03ac8e35d0b613bd70) fix: force new talosconfig if endpoints or nodes change
* [`b7d84ba`](https://github.com/siderolabs/terraform-provider-talos/commit/b7d84ba4e297271b1599de20310640c4ebffccb9) chore: update registry index file, remove example
* [`158dbbd`](https://github.com/siderolabs/terraform-provider-talos/commit/158dbbde42326fed9ada2a97a2a25c625c0a783e) docs: re-word `talos_version`
* [`f3b4a5b`](https://github.com/siderolabs/terraform-provider-talos/commit/f3b4a5b24362c264a642e5bc5a28bba0e70a86fc) chore: bump deps
* [`d2b6df0`](https://github.com/siderolabs/terraform-provider-talos/commit/d2b6df03e44a9150445209d3bd118c5d4417547c) docs: clarify meaning of `talos_version` in `machine_configuration` resources
</p>
</details>

### Changes from siderolabs/crypto
<details><summary>27 commits</summary>
<p>

* [`c3225ee`](https://github.com/siderolabs/crypto/commit/c3225eee603a8d1218c67e1bfe33ddde7953ed74) feat: allow CSR template subject field to be overridden
* [`8570669`](https://github.com/siderolabs/crypto/commit/85706698dac8cddd0e9f41006bed059347d2ea26) chore: rename to siderolabs/crypto
* [`e9df1b8`](https://github.com/siderolabs/crypto/commit/e9df1b8ca74c6efdc7f72191e5d2613830162fd5) feat: add support for generating keys from RSA-SHA256 CAs
* [`510b0d2`](https://github.com/siderolabs/crypto/commit/510b0d2753a89170d0c0f60e052a66484997a5b2) chore: add json tags
* [`6fa2d93`](https://github.com/siderolabs/crypto/commit/6fa2d93d0382299d5471e0de8e831c923398aaa8) fix: deepcopy nil fields as `nil`
* [`9a63cba`](https://github.com/siderolabs/crypto/commit/9a63cba8dabd278f3080fa8c160613efc48c43f8) fix: add back support for generating ECDSA keys with P-256 and SHA512
* [`893bc66`](https://github.com/siderolabs/crypto/commit/893bc66e4716a4cb7d1d5e66b5660ffc01f22823) fix: use SHA256 for ECDSA-P256
* [`deec8d4`](https://github.com/siderolabs/crypto/commit/deec8d47700e10e3ea813bdce01377bd93c83367) chore: implement DeepCopy methods for PEMEncoded* types
* [`d3cb772`](https://github.com/siderolabs/crypto/commit/d3cb77220384b3a3119a6f3ddb1340bbc811f1d1) feat: make possible to change KeyUsage
* [`6bc5bb5`](https://github.com/siderolabs/crypto/commit/6bc5bb50c52767296a1b1cab6580e3fcf1358f34) chore: remove unused argument
* [`cd18ef6`](https://github.com/siderolabs/crypto/commit/cd18ef62eb9f65d8b6730a2eb73e47e629949e1b) feat: add support for several organizations
* [`97c888b`](https://github.com/siderolabs/crypto/commit/97c888b3924dd5ac70b8d30dd66b4370b5ab1edc) chore: add options to CSR
* [`7776057`](https://github.com/siderolabs/crypto/commit/7776057f5086157873f62f6a21ec23fa9fd86e05) chore: fix typos
* [`80df078`](https://github.com/siderolabs/crypto/commit/80df078327030af7e822668405bb4853c512bd7c) chore: remove named result parameters
* [`15bdd28`](https://github.com/siderolabs/crypto/commit/15bdd282b74ac406ab243853c1b50338a1bc29d0) chore: minor updates
* [`4f80b97`](https://github.com/siderolabs/crypto/commit/4f80b976b640d773fb025d981bf85bcc8190815b) fix: verify CSR signature before issuing a certificate
* [`39584f1`](https://github.com/siderolabs/crypto/commit/39584f1b6e54e9966db1f16369092b2215707134) feat: support for key/certificate types RSA, Ed25519, ECDSA
* [`cf75519`](https://github.com/siderolabs/crypto/commit/cf75519cab82bd1b128ae9b45107c6bb422bd96a) fix: function NewKeyPair should create certificate with proper subject
* [`751c95a`](https://github.com/siderolabs/crypto/commit/751c95aa9434832a74deb6884cff7c5fd785db0b) feat: add 'PEMEncodedKey' which allows to transport keys in YAML
* [`562c3b6`](https://github.com/siderolabs/crypto/commit/562c3b66f89866746c0ba47927c55f41afed0f7f) feat: add support for public RSA key in RSAKey
* [`bda0e9c`](https://github.com/siderolabs/crypto/commit/bda0e9c24e80c658333822e2002e0bc671ac53a3) feat: enable more conversions between encoded and raw versions
* [`e0dd56a`](https://github.com/siderolabs/crypto/commit/e0dd56ac47456f85c0b247999afa93fb87ebc78b) feat: add NotBefore option for x509 cert creation
* [`12a4897`](https://github.com/siderolabs/crypto/commit/12a489768a6bb2c13e16e54617139c980f99a658) feat: add support for SPKI fingerprint generation and matching
* [`d0c3eef`](https://github.com/siderolabs/crypto/commit/d0c3eef149ec9b713e7eca8c35a6214bd0a64bc4) fix: implement NewKeyPair
* [`196679e`](https://github.com/siderolabs/crypto/commit/196679e9ec77cb709db54879ddeddd4eaafaea01) feat: move `pkg/grpc/tls` from `github.com/talos-systems/talos` as `./tls`
* [`1ff6242`](https://github.com/siderolabs/crypto/commit/1ff6242c91bb298ceeb4acd65685cba952fe4178) chore: initial version as imported from talos-systems/talos
* [`835063e`](https://github.com/siderolabs/crypto/commit/835063e055b28a525038b826a6d80cbe76402414) chore: initial commit
</p>
</details>

### Dependency Changes

* **github.com/hashicorp/terraform-plugin-framework**             v1.2.0 **_new_**
* **github.com/hashicorp/terraform-plugin-framework-timeouts**    v0.3.1 **_new_**
* **github.com/hashicorp/terraform-plugin-framework-validators**  v0.10.0 **_new_**
* **github.com/hashicorp/terraform-plugin-go**                    v0.15.0 **_new_**
* **github.com/hashicorp/terraform-plugin-sdk/v2**                v2.25.0 -> v2.26.1
* **github.com/hashicorp/terraform-plugin-testing**               v1.2.0 **_new_**
* **github.com/siderolabs/crypto**                                v0.4.0 **_new_**
* **github.com/siderolabs/talos/pkg/machinery**                   v1.3.6 -> v1.4.0-beta.0
* **golang.org/x/mod**                                            v0.10.0 **_new_**
* **google.golang.org/grpc**                                      v1.51.0 -> v1.54.0
* **k8s.io/client-go**                                            v0.26.3 **_new_**

Previous release can be found at [v0.1.2](https://github.com/siderolabs/terraform-provider-talos/releases/tag/v0.1.2)

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

