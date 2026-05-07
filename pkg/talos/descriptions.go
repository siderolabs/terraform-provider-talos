// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

// Schema field descriptions for consistent usage across resources and data sources.
const (
	DescClientCACertificate           = "The client CA certificate"
	DescClientCertificate             = "The client certificate"
	DescClientKey                     = "The client key"
	DescClientConfig                  = "The client configuration data"
	DescGeneratedClientConfig         = "The generated client configuration"
	DescRawKubeconfig                 = "The raw kubeconfig"
	DescControlplaneNodeForKubeconfig = "controlplane node to retrieve the kubeconfig from"
	DescEndpointForTalosClient        = "endpoint to use for the talosclient. If not set, the node value will be used"

	DescKubernetesCACertificate     = "The kubernetes CA certificate"
	DescKubernetesClientCertificate = "The kubernetes client certificate"
	DescKubernetesClientKey         = "The kubernetes client key"
	DescKubernetesClientConfig      = "The kubernetes client configuration"
	DescKubernetesHost              = "The kubernetes host"
)

// Schema field name constants for use as map keys and path arguments.
const (
	FieldCACertificate               = "ca_certificate"
	FieldClientCertificate           = "client_certificate"
	FieldClientKey                   = "client_key"
	FieldClientConfiguration         = "client_configuration"
	FieldClientConfigurationWO       = "client_configuration_wo"
	FieldTalosVersion                = "talos_version"
	FieldMachineSecrets              = "machine_secrets"
	FieldEndpoint                    = "endpoint"
	FieldEndpoints                   = "endpoints"
	FieldNode                        = "node"
	FieldNodes                       = "nodes"
	FieldMachineConfiguration        = "machine_configuration"
	FieldMachineConfigurationInput   = "machine_configuration_input"
	FieldMachineConfigurationInputWO = "machine_configuration_input_wo"
	FieldMachineConfigurationHash    = "machine_configuration_hash"
	FieldMachineConfigurationApply   = "machine_configuration_apply"
	FieldMachineBootstrap            = "machine_bootstrap"
	FieldKubernetesClientConfig      = "kubernetes_client_configuration"
	FieldClusterName                 = "cluster_name"
	FieldClusterEndpoint             = "cluster_endpoint"
	FieldTalosConfig                 = "talos_config"
	FieldKubeconfigRaw               = "kubeconfig_raw"
	FieldTimeouts                    = "timeouts"
	FieldApplyMode                   = "apply_mode"
	FieldResolvedApplyMode           = "resolved_apply_mode"
	FieldConfigPatches               = "config_patches"
	FieldStagedIfNeedingReboot       = "staged_if_needing_reboot"
	FieldControlplane                = "controlplane"
	FieldWorker                      = "worker"
	FieldWorkerNodes                 = "worker_nodes"
	FieldControlPlaneNodes           = "control_plane_nodes"
	FieldSkipKubernetesChecks        = "skip_kubernetes_checks"
	FieldMachineType                 = "machine_type"
	FieldKubernetesVersion           = "kubernetes_version"
	FieldHost                        = "host"
	FieldMetal                       = "metal"
	FieldCreate                      = "create"
	FieldKey                         = "key"
	FieldCert                        = "cert"
	FieldCerts                       = "certs"
	FieldCluster                     = "cluster"
	FieldSecrets                     = "secrets"
	FieldSecret                      = "secret"
	FieldToken                       = "token"
	FieldTrustdInfo                  = "trustdinfo"
	FieldEtcd                        = "etcd"
	FieldK8s                         = "k8s"
	FieldK8sAggregator               = "k8s_aggregator"
	FieldK8sServiceAccount           = "k8s_serviceaccount"
	FieldAESCBCEncryptionSecret      = "aescbc_encryption_secret"
	FieldSecretboxEncryptionSecret   = "secretbox_encryption_secret"
	FieldBootstrapToken              = "bootstrap_token"
	FieldNotBefore                   = "not_before"
	FieldCRTTTL                      = "crt_ttl"
	FieldDigest                      = "digest"
	FieldRef                         = "ref"
	FieldSelector                    = "selector"
	FieldPlatform                    = "platform"
	FieldFilters                     = "filters"
	FieldOverlaysInfo                = "overlays_info"
	FieldExtensionsInfo              = "extensions_info"
	FieldName                        = "name"
	FieldNames                       = "names"
	FieldDynamic                     = "dynamic"
	FieldAuto                        = "auto"
	FieldStaged                      = "staged"
	FieldReboot                      = "reboot"
	FieldDefault                     = "default"
	FieldDocs                        = "docs"
	FieldExamples                    = "examples"
	FieldTalosVersions               = "talos_versions"
	FieldSBC                         = "sbc"
	FieldRPIGeneric                  = "rpi_generic"
	FieldTalosClientConfiguration    = "talos_client_configuration"
	FieldTalosMachineConfiguration   = "talos_machine_configuration"
	FieldTalosMachineBootstrap       = "talos_machine_bootstrap"
	FieldTalosClusterKubeconfig      = "talos_cluster_kubeconfig"
	FieldTalosImageFactoryURLs       = "talos_image_factory_urls"
	FieldTalosImageFactorySchematic  = "talos_image_factory_schematic"
	FieldTalosImageExtVersions       = "talos_image_factory_extensions_versions"
	FieldClusterKubeconfig           = "_cluster_kubeconfig"
	FieldAdmin                       = "admin@"
)

// Error message constants for consistent usage across resources and data sources.
const (
	ErrGenerateTalosConfig           = "failed to generate talos config"
	ErrGetImageFactoryClient         = "failed to get image factory client"
	ErrConvertSecretsBundleToMachine = "failed to convert secrets bundle to machine secrets"
	ErrConvertMachineToSecretsBundle = "failed to convert machine secrets to secrets bundle"
	ErrImageFactoryNotConfigured     = "image factory client is not configured"
	ErrConvertConfigToTalosClient    = "Error converting config to talos client config"
	ErrDecodeKubernetesClientCert    = "failed to decode kubernetes client certificate"
	ErrDecodeClientCert              = "failed to decode client certificate"
	ErrReadClientConfiguration       = "Error reading client configuration"
	ErrRetrieveKubeconfig            = "failed to retrieve kubeconfig"
	ErrParseKubeconfig               = "failed to parse kubeconfig"
	ErrClientConfigurationIssue      = "Client configuration issue"
	ErrSignAdminCertificate          = "error signing admin certificate: %w"
	ErrMarshalAdminPrivateKey        = "error marshaling admin private key: %w"
	ErrDeriveAdminKeyBytes           = "error deriving admin key bytes: %w"
	ErrPleaseReportIssue             = "Please report this issue to the provider developers."
)

// Provider name constant.
const (
	ProviderName = "talos"
)

// CEL expression constants.
const (
	CELExprTrue = "true"
)

// Format string constants.
const (
	FmtImageURL          = "%s/image/%s/%s/%s"
	FmtExpectedClientGot = "Expected *client.Client, got: %T. Please report this issue to the provider developers."
)
