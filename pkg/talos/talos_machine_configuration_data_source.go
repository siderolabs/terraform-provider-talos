// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/crypto/x509"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/compatibility"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"
)

type talosMachineConfigurationDataSourceModelV0 struct {
	ID                   types.String   `tfsdk:"id"`
	ClusterName          types.String   `tfsdk:"cluster_name"`
	ClusterEndpoint      types.String   `tfsdk:"cluster_endpoint"`
	MachineType          types.String   `tfsdk:"machine_type"`
	KubernetesVersion    types.String   `tfsdk:"kubernetes_version"`
	TalosVersion         types.String   `tfsdk:"talos_version"`
	MachineSecrets       machineSecrets `tfsdk:"machine_secrets"`
	MachineConfiguration types.String   `tfsdk:"machine_configuration"`
	ConfigPatches        types.List     `tfsdk:"config_patches"`
	Docs                 types.Bool     `tfsdk:"docs"`
	Examples             types.Bool     `tfsdk:"examples"`
}

type talosMachineConfigurationDataSource struct{}

var (
	_ datasource.DataSource                   = &talosMachineConfigurationDataSource{}
	_ datasource.DataSourceWithValidateConfig = &talosMachineConfigurationDataSource{}
)

// NewTalosMachineConfigurationDataSource implements the datasource.DataSource interface.
func NewTalosMachineConfigurationDataSource() datasource.DataSource {
	return &talosMachineConfigurationDataSource{}
}

func (d *talosMachineConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_configuration"
}

func (d *talosMachineConfigurationDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate a machine configuration for a node type",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"cluster_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the talos kubernetes cluster",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"cluster_endpoint": schema.StringAttribute{
				Required:    true,
				Description: "The endpoint of the talos kubernetes cluster",
			},
			"machine_secrets": schema.SingleNestedAttribute{
				Description: "The secrets for the talos cluster",
				Attributes: map[string]schema.Attribute{
					"cluster": schema.SingleNestedAttribute{
						Description: "The cluster secrets",
						Attributes: map[string]schema.Attribute{
							"id": schema.StringAttribute{
								Required:    true,
								Description: "The cluster id",
							},
							"secret": schema.StringAttribute{
								Required:    true,
								Sensitive:   true,
								Description: "The cluster secret",
							},
						},
						Required: true,
					},
					"secrets": schema.SingleNestedAttribute{
						Description: "The secrets for the talos kubernetes cluster",
						Attributes: map[string]schema.Attribute{
							"bootstrap_token": schema.StringAttribute{
								Description: "The bootstrap token for the talos kubernetes cluster",
								Required:    true,
								Sensitive:   true,
							},
							"secretbox_encryption_secret": schema.StringAttribute{
								Description: "The secretbox encryption secret for the talos kubernetes cluster",
								Required:    true,
								Sensitive:   true,
							},
							"aescbc_encryption_secret": schema.StringAttribute{
								Description: "The aescbc encryption secret for the talos kubernetes cluster",
								Optional:    true,
								Sensitive:   true,
							},
						},
						Required: true,
					},
					"trustdinfo": schema.SingleNestedAttribute{
						Description: "The trustd info for the talos kubernetes cluster",
						Attributes: map[string]schema.Attribute{
							"token": schema.StringAttribute{
								Description: "The trustd token for the talos kubernetes cluster",
								Required:    true,
								Sensitive:   true,
							},
						},
						Required: true,
					},
					"certs": schema.SingleNestedAttribute{
						Description: "The certs for the talos kubernetes cluster",
						Attributes: map[string]schema.Attribute{
							"etcd":           certSchemaInput(),
							"k8s":            certSchemaInput(),
							"k8s_aggregator": certSchemaInput(),
							"k8s_serviceaccount": schema.SingleNestedAttribute{
								Attributes: map[string]schema.Attribute{
									"key": schema.StringAttribute{
										Description: "The key for the k8s service account",
										Required:    true,
										Sensitive:   true,
									},
								},
								Required: true,
							},
							"os": certSchemaInput(),
						},
						Required: true,
					},
				},
				Required: true,
			},
			"machine_type": schema.StringAttribute{
				Required:    true,
				Description: "The type of machine to generate the configuration for",
				Validators: []validator.String{
					stringvalidator.OneOf("controlplane", "worker"),
				},
			},
			"config_patches": schema.ListAttribute{
				Description: "The list of config patches to apply to the generated configuration",
				Optional:    true,
				ElementType: types.StringType,
			},
			"kubernetes_version": schema.StringAttribute{
				Description: "The version of kubernetes to use",
				Optional:    true,
			},
			"talos_version": schema.StringAttribute{
				Description: "The version of talos features to use in generated machine configuration",
				Optional:    true,
				Validators: []validator.String{
					talosVersionValid(),
				},
			},
			"docs": schema.BoolAttribute{
				Description: "Whether to generate documentation for the generated configuration. Defaults to false",
				Optional:    true,
			},
			"examples": schema.BoolAttribute{
				Description: "Whether to generate examples for the generated configuration. DFaults to false",
				Optional:    true,
			},
			"machine_configuration": schema.StringAttribute{
				Description: "The generated machine configuration",
				Computed:    true,
				Sensitive:   true,
			},
		},
	}
}

func (d *talosMachineConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state talosMachineConfigurationDataSourceModelV0

	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !state.KubernetesVersion.IsUnknown() && state.KubernetesVersion.IsNull() {
		state.KubernetesVersion = basetypes.NewStringValue(constants.DefaultKubernetesVersion)
	}

	if !state.TalosVersion.IsUnknown() && state.TalosVersion.IsNull() {
		state.TalosVersion = basetypes.NewStringValue(semver.MajorMinor(gendata.VersionTag))
	}

	var machineType machine.Type

	switch state.MachineType.ValueString() {
	case "controlplane":
		machineType = machine.TypeControlPlane
	case "worker":
		machineType = machine.TypeWorker
	}

	machineSecrets := &secrets.Bundle{
		Clock: secrets.NewFixedClock(time.Now()),
		Cluster: &secrets.Cluster{
			ID:     state.MachineSecrets.Cluster.ID.ValueString(),
			Secret: state.MachineSecrets.Cluster.Secret.ValueString(),
		},
		Secrets: &secrets.Secrets{
			BootstrapToken:            state.MachineSecrets.Secrets.BootstrapToken.ValueString(),
			SecretboxEncryptionSecret: state.MachineSecrets.Secrets.SecretboxEncryptionSecret.ValueString(),
		},
		TrustdInfo: &secrets.TrustdInfo{
			Token: state.MachineSecrets.TrustdInfo.Token.ValueString(),
		},
	}

	if !state.MachineSecrets.Secrets.AESCBCEncryptionSecret.IsNull() {
		machineSecrets.Secrets.AESCBCEncryptionSecret = state.MachineSecrets.Secrets.AESCBCEncryptionSecret.ValueString()
	}

	machineSecretsCerts, err := machineSecretsCertsToSecretsBundleCerts(state.MachineSecrets.Certs)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to convert machine secrets certs to secrets bundle certs",
			err.Error(),
		)

		return
	}

	machineSecrets.Certs = machineSecretsCerts

	var configPatches []string

	resp.Diagnostics.Append(state.ConfigPatches.ElementsAs(ctx, &configPatches, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	genOptions := &machineConfigGenerateOptions{
		machineType:       machineType,
		clusterName:       state.ClusterName.ValueString(),
		clusterEndpoint:   state.ClusterEndpoint.ValueString(),
		machineSecrets:    machineSecrets,
		configPatches:     configPatches,
		kubernetesVersion: state.KubernetesVersion.ValueString(),
		talosVersion:      state.TalosVersion.ValueString(),
		docsEnabled:       state.Docs.ValueBool(),
		examplesEnabled:   state.Examples.ValueBool(),
	}

	machineConfiguration, err := genOptions.generate()
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to generate machine configuration",
			err.Error(),
		)

		return
	}

	state.MachineConfiguration = basetypes.NewStringValue(machineConfiguration)
	state.ID = state.ClusterName

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (d talosMachineConfigurationDataSource) ValidateConfig(ctx context.Context, req datasource.ValidateConfigRequest, resp *datasource.ValidateConfigResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosMachineConfigurationDataSourceModelV0

	diags = obj.As(ctx, &state, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !state.ClusterEndpoint.IsUnknown() && !state.ClusterEndpoint.IsNull() {
		if err := validateClusterEndpoint(state.ClusterEndpoint.ValueString()); err != nil {
			resp.Diagnostics.AddError(
				"cluster_endpoint is invalid",
				err.Error(),
			)
		}
	}

	var configPatches []string

	resp.Diagnostics.Append(state.ConfigPatches.ElementsAs(ctx, &configPatches, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if _, err := configpatcher.LoadPatches(configPatches); err != nil {
		resp.Diagnostics.AddError(
			"config_patches are invalid",
			err.Error(),
		)

		return
	}

	if !state.KubernetesVersion.IsUnknown() && !state.KubernetesVersion.IsNull() && !state.TalosVersion.IsUnknown() {
		k8sVersionCompatibility, err := compatibility.ParseKubernetesVersion(strings.TrimPrefix(state.KubernetesVersion.ValueString(), "v"))
		if err != nil {
			resp.Diagnostics.AddError(
				"kubernetes_version is invalid",
				err.Error(),
			)

			return
		}

		talosVersionInfo := &machineapi.VersionInfo{}

		if state.TalosVersion.IsNull() {
			talosVersionInfo.Tag = gendata.VersionTag
		}

		if !state.TalosVersion.IsNull() {
			talosVersionInfo.Tag = state.TalosVersion.ValueString()
		}

		talosVersionCompatibility, err := compatibility.ParseTalosVersion(talosVersionInfo)
		if err != nil {
			resp.Diagnostics.AddError(
				"talos_version is invalid",
				err.Error(),
			)

			return
		}

		if err := k8sVersionCompatibility.SupportedWith(talosVersionCompatibility); err != nil {
			resp.Diagnostics.AddError(
				"talos_version is not compatible with kubernetes_version",
				err.Error(),
			)

			return
		}
	}
}

func certSchemaInput() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "The certificate and key pair",
		Attributes: map[string]schema.Attribute{
			"cert": schema.StringAttribute{
				Description: "certificate data",
				Required:    true,
			},
			"key": schema.StringAttribute{
				Description: "key data",
				Required:    true,
				Sensitive:   true,
			},
		},
		Required: true,
	}
}

func machineSecretsCertsToSecretsBundleCerts(machineSecretsCerts machineSecretsCerts) (*secrets.Certs, error) {
	etcdCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(machineSecretsCerts.Etcd.Cert.ValueString(), machineSecretsCerts.Etcd.Key.ValueString())
	if err != nil {
		return nil, err
	}

	k8sCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(machineSecretsCerts.K8s.Cert.ValueString(), machineSecretsCerts.K8s.Key.ValueString())
	if err != nil {
		return nil, err
	}

	k8sAggregatorCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(machineSecretsCerts.K8sAggregator.Cert.ValueString(), machineSecretsCerts.K8sAggregator.Key.ValueString())
	if err != nil {
		return nil, err
	}

	k8sServiceAccountCertDataX509, err := certDataToX509PEMEncodedKey(machineSecretsCerts.K8sServiceAccount.Key.ValueString())
	if err != nil {
		return nil, err
	}

	osCertDataX509, err := certDataToX509PEMEncodedCertificateAndKey(machineSecretsCerts.OS.Cert.ValueString(), machineSecretsCerts.OS.Key.ValueString())
	if err != nil {
		return nil, err
	}

	return &secrets.Certs{
		Etcd:              etcdCertDataX509,
		K8s:               k8sCertDataX509,
		K8sAggregator:     k8sAggregatorCertDataX509,
		K8sServiceAccount: k8sServiceAccountCertDataX509,
		OS:                osCertDataX509,
	}, nil
}

func certDataToX509PEMEncodedCertificateAndKey(cert, key string) (*x509.PEMEncodedCertificateAndKey, error) {
	certBytes, err := base64ToBytes(cert)
	if err != nil {
		return nil, err
	}

	keyBytes, err := base64ToBytes(key)
	if err != nil {
		return nil, err
	}

	return &x509.PEMEncodedCertificateAndKey{
		Key: keyBytes,
		Crt: certBytes,
	}, nil
}

func certDataToX509PEMEncodedKey(key string) (*x509.PEMEncodedKey, error) {
	keyBytes, err := base64ToBytes(key)
	if err != nil {
		return nil, err
	}

	return &x509.PEMEncodedKey{
		Key: keyBytes,
	}, nil
}
