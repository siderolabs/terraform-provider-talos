// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// Field name aliases for use in talos_test package.
const (
	fieldCACertificate          = talos.FieldCACertificate
	fieldClientCertificate      = talos.FieldClientCertificate
	fieldClientKey              = talos.FieldClientKey
	fieldClientConfiguration    = talos.FieldClientConfiguration
	fieldTalosVersion           = talos.FieldTalosVersion
	fieldMachineSecrets         = talos.FieldMachineSecrets
	fieldEndpoint               = talos.FieldEndpoint
	fieldEndpoints              = talos.FieldEndpoints
	fieldNode                   = talos.FieldNode
	fieldMachineConfiguration   = talos.FieldMachineConfiguration
	fieldMachineConfigInput     = talos.FieldMachineConfigurationInput
	fieldMachineConfigApply     = talos.FieldMachineConfigurationApply
	fieldKubernetesClientConfig = talos.FieldKubernetesClientConfig
	fieldClusterName            = talos.FieldClusterName
	fieldClusterEndpoint        = talos.FieldClusterEndpoint
	fieldTalosConfig            = talos.FieldTalosConfig
	fieldKubeconfigRaw          = talos.FieldKubeconfigRaw
	fieldApplyMode              = talos.FieldApplyMode
	fieldResolvedApplyMode      = talos.FieldResolvedApplyMode
	fieldConfigPatches          = talos.FieldConfigPatches
	fieldStagedIfNeedingReboot  = talos.FieldStagedIfNeedingReboot
	fieldControlplane           = talos.FieldControlplane
	fieldWorker                 = talos.FieldWorker
	fieldWorkerNodes            = talos.FieldWorkerNodes
	fieldControlPlaneNodes      = talos.FieldControlPlaneNodes
	fieldMachineType            = talos.FieldMachineType
	fieldKubernetesVersion      = talos.FieldKubernetesVersion
	fieldHost                   = talos.FieldHost
	fieldMetal                  = talos.FieldMetal
	fieldDocs                   = talos.FieldDocs
	fieldExamples               = talos.FieldExamples
	fieldExtensionsInfo         = talos.FieldExtensionsInfo
	fieldOverlaysInfo           = talos.FieldOverlaysInfo
	fieldDigest                 = talos.FieldDigest
	fieldRef                    = talos.FieldRef
	fieldSelector               = talos.FieldSelector
	fieldPlatform               = talos.FieldPlatform
	fieldFilters                = talos.FieldFilters
	fieldNames                  = talos.FieldNames
	fieldDynamic                = talos.FieldDynamic
	fieldAuto                   = talos.FieldAuto
	fieldStaged                 = talos.FieldStaged
	fieldReboot                 = talos.FieldReboot
	fieldTalosVersions          = talos.FieldTalosVersions
	fieldSBC                    = talos.FieldSBC
	fieldRPIGeneric             = talos.FieldRPIGeneric
	fieldTalosClientConf        = talos.FieldTalosClientConfiguration
	fieldTalosMachineConf       = talos.FieldTalosMachineConfiguration
	fieldTalosMachineBootstrap  = talos.FieldTalosMachineBootstrap
	fieldTalosClusterKubeconf   = talos.FieldTalosClusterKubeconfig
	fieldTalosImageFactoryURLs  = talos.FieldTalosImageFactoryURLs
	fieldTalosImageSchematic    = talos.FieldTalosImageFactorySchematic
	fieldTalosImageExtVersions  = talos.FieldTalosImageExtVersions
)

// Resource address constants for acceptance tests.
const (
	resTalosMachineSecrets           = "talos_machine_secrets.this"
	resTalosMachineConfigApply       = "talos_machine_configuration_apply.this"
	resTalosMachineConfigApplyStaged = "talos_machine_configuration_apply.staged_if_needing_reboot"
	resTalosClusterKubeconfig        = "talos_cluster_kubeconfig.this"
	resTalosMachineBootstrap         = "talos_machine_bootstrap.this"
	resTalosImageFactorySchematic    = "talos_image_factory_schematic.this"
	dataTalosClientConf              = "data.talos_client_configuration.this"
	dataTalosMachineConf             = "data.talos_machine_configuration.this"
	dataTalosImageFactoryURLs        = "data.talos_image_factory_urls.this"
	dataTalosMachineDisks            = "data.talos_machine_disks.this"
	dataTalosImageFactoryExtVersions = "data.talos_image_factory_extensions_versions.this"
	dataTalosImageFactoryOverlayVers = "data.talos_image_factory_overlays_versions.this"
	resEchoTest                      = "echo.test"
	resEcho                          = "echo"
)

// Attribute path constants for acceptance test checks.
const (
	attrMachineSecretsClusterID       = "machine_secrets.cluster.id"
	attrMachineSecretsClusterSecret   = "machine_secrets.cluster.secret"
	attrMachineSecretsTrustdToken     = "machine_secrets.trustdinfo.token"
	attrMachineSecretsBootstrapToken  = "machine_secrets.secrets.bootstrap_token"
	attrMachineSecretsAESCBCSecret    = "machine_secrets.secrets.aescbc_encryption_secret"
	attrMachineSecretsSecretboxSecret = "machine_secrets.secrets.secretbox_encryption_secret"
	attrMachineSecretsCertsEtcdCert   = "machine_secrets.certs.etcd.cert"
	attrMachineSecretsCertsEtcdKey    = "machine_secrets.certs.etcd.key"
	attrMachineSecretsCertsK8sCert    = "machine_secrets.certs.k8s.cert"
	attrMachineSecretsCertsK8sKey     = "machine_secrets.certs.k8s.key"
	attrMachineSecretsCertsK8sAggCert = "machine_secrets.certs.k8s_aggregator.cert"
	attrMachineSecretsCertsK8sAggKey  = "machine_secrets.certs.k8s_aggregator.key"
	attrMachineSecretsCertsK8sSvcKey  = "machine_secrets.certs.k8s_serviceaccount.key"
	attrMachineSecretsCertsOSCert     = "machine_secrets.certs.os.cert"
	attrMachineSecretsCertsOSKey      = "machine_secrets.certs.os.key"
	attrClientConfigCACert            = "client_configuration.ca_certificate"
	attrClientConfigClientCert        = "client_configuration.client_certificate"
	attrClientConfigClientKey         = "client_configuration.client_key"
	attrMachineSecretsPercent         = "machine_secrets.%"
	attrConfigPatchesCount            = "config_patches.#"
	attrConfigPatchesFirst            = "config_patches.0"
	attrExtensionsInfoFirstName       = "extensions_info.0.name"
)

// Test value constants.
const (
	testCluster            = "test-cluster"
	testCluster1           = "test-cluster-1"
	exampleCluster         = "example-cluster"
	exampleCluster1        = "example-cluster-1"
	exampleCluster2        = "example-cluster-2"
	exampleCluster3        = "example-cluster-3"
	testEndpoint           = "https://10.0.0.1:6443"
	testClusterEndpoint1   = "https://cluster-1.local:6443"
	testClusterEndpoint2   = "https://cluster-2.local:6443"
	testClusterEndpoint3   = "https://cluster-3.local:6443"
	testClusterLocal       = "https://cluster.local"
	testClusterLocalFull   = "https://cluster.local:6443"
	testSchematicHash      = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	testNotBefore          = "2024-01-01T00:00:00Z"
	testVersionConstraint  = "=0.1.2"
	testSiderolabsTalos    = "siderolabs/talos"
	testIP1                = "10.5.0.2"
	testIP2                = "10.5.0.3"
	testIP3                = "10.5.0.4"
	testV1p2               = "v1.2"
	testV1p3               = "v1.3"
	testV1p7p0             = "v1.7.0"
	testV1p7p5             = "v1.7.5"
	testV1p2p0             = "v1.2.0"
	testDevVDA             = "/dev/vda"
	testDevSDA             = "/dev/sda"
	testMachineConfigApply = "machine_configuration_apply"
	testMachineSecrets     = "machine_secrets"
)

// URL attribute path constants for image factory tests.
const (
	attrURLsISO                 = "urls.iso"
	attrURLsISOSecureBoot       = "urls.iso_secureboot"
	attrURLsKernel              = "urls.kernel"
	attrURLsInitramfs           = "urls.initramfs"
	attrURLsKernelCmdLine       = "urls.kernel_command_line"
	attrURLsUKI                 = "urls.uki"
	attrURLsPXE                 = "urls.pxe"
	attrURLsDiskImage           = "urls.disk_image"
	attrURLsDiskImageSecureBoot = "urls.disk_image_secureboot"
	attrURLsInstaller           = "urls.installer"
	attrURLsInstallerSecureBoot = "urls.installer_secureboot"
)

// Terraform template name constants.
const (
	tfConfigTemplateName = "tf_config"
	tfTerraformData      = "terraform_data"
	tfTalosV1Provider    = "talosv1"
	tfMachineConfigCtrl  = "talos_machine_configuration_controlplane"
)

// Provider and resource name constants.
const (
	providerName    = talos.ProviderName
	libvirtProvider = "libvirt"
	echoDataKey     = "data"
	resourceThis    = "this"
)

// Boolean string value constants.
const (
	boolTrue  = "true"
	boolFalse = "false"
)
