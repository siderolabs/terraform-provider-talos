// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"
)

var (
	_ ephemeral.EphemeralResource                   = &talosMachineConfigurationEphemeralResource{}
	_ ephemeral.EphemeralResourceWithValidateConfig = &talosMachineConfigurationEphemeralResource{}
)

type talosMachineConfigurationEphemeralResource struct{}

type talosMachineConfigurationEphemeralResourceModel struct {
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

// NewTalosMachineConfigurationEphemeralResource implements the ephemeral.EphemeralResource interface.
func NewTalosMachineConfigurationEphemeralResource() ephemeral.EphemeralResource {
	return &talosMachineConfigurationEphemeralResource{}
}

func (r *talosMachineConfigurationEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_configuration"
}

func (r *talosMachineConfigurationEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate a machine configuration for a node type. This is an ephemeral resource that does not persist secrets in Terraform state.",
		Attributes: map[string]schema.Attribute{
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
			"machine_secrets": machineSecretsSchemaAttribute(),
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
				Computed:    true,
			},
			"talos_version": schema.StringAttribute{
				Description: "The Talos version contract used to generate the machine configuration. This does not control the installed Talos version. Use `config_patches` to set `machine.install.image` to the desired value. Example values: `v1.12`, `v1.12.1`, `1.12`, `1.12.1`", // nolint:lll
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					talosVersionValid(),
				},
			},
			"docs": schema.BoolAttribute{
				Description: "Whether to generate documentation for the generated configuration. Defaults to false",
				Optional:    true,
			},
			"examples": schema.BoolAttribute{
				Description: "Whether to generate examples for the generated configuration. Defaults to false",
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

func (r *talosMachineConfigurationEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosMachineConfigurationEphemeralResourceModel

	diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	kubernetesVersion := config.KubernetesVersion.ValueString()
	if kubernetesVersion == "" {
		kubernetesVersion = constants.DefaultKubernetesVersion
	}

	talosVersion := config.TalosVersion.ValueString()
	if talosVersion == "" {
		talosVersion = semver.MajorMinor(gendata.VersionTag)
	}

	var machineType machine.Type

	switch config.MachineType.ValueString() {
	case "controlplane":
		machineType = machine.TypeControlPlane
	case "worker":
		machineType = machine.TypeWorker
	}

	machineSecretsBundle := &secrets.Bundle{
		Clock: secrets.NewFixedClock(time.Now()),
		Cluster: &secrets.Cluster{
			ID:     config.MachineSecrets.Cluster.ID.ValueString(),
			Secret: config.MachineSecrets.Cluster.Secret.ValueString(),
		},
		Secrets: &secrets.Secrets{
			BootstrapToken:            config.MachineSecrets.Secrets.BootstrapToken.ValueString(),
			SecretboxEncryptionSecret: config.MachineSecrets.Secrets.SecretboxEncryptionSecret.ValueString(),
		},
		TrustdInfo: &secrets.TrustdInfo{
			Token: config.MachineSecrets.TrustdInfo.Token.ValueString(),
		},
	}

	if !config.MachineSecrets.Secrets.AESCBCEncryptionSecret.IsNull() {
		machineSecretsBundle.Secrets.AESCBCEncryptionSecret = config.MachineSecrets.Secrets.AESCBCEncryptionSecret.ValueString()
	}

	machineSecretsCerts, err := machineSecretsCertsToSecretsBundleCerts(config.MachineSecrets.Certs)
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to convert machine secrets certs to secrets bundle certs",
			err.Error(),
		)

		return
	}

	machineSecretsBundle.Certs = machineSecretsCerts

	var configPatches []string

	resp.Diagnostics.Append(config.ConfigPatches.ElementsAs(ctx, &configPatches, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	genOptions := &machineConfigGenerateOptions{
		machineType:       machineType,
		clusterName:       config.ClusterName.ValueString(),
		clusterEndpoint:   config.ClusterEndpoint.ValueString(),
		machineSecrets:    machineSecretsBundle,
		configPatches:     configPatches,
		kubernetesVersion: kubernetesVersion,
		talosVersion:      talosVersion,
		docsEnabled:       config.Docs.ValueBool(),
		examplesEnabled:   config.Examples.ValueBool(),
	}

	machineConfiguration, err := genOptions.generate()
	if err != nil {
		resp.Diagnostics.AddError(
			"failed to generate machine configuration",
			err.Error(),
		)

		return
	}

	result := talosMachineConfigurationEphemeralResourceModel{
		ClusterName:          config.ClusterName,
		ClusterEndpoint:      config.ClusterEndpoint,
		MachineType:          config.MachineType,
		KubernetesVersion:    basetypes.NewStringValue(kubernetesVersion),
		TalosVersion:         basetypes.NewStringValue(talosVersion),
		MachineSecrets:       config.MachineSecrets,
		MachineConfiguration: basetypes.NewStringValue(machineConfiguration),
		ConfigPatches:        config.ConfigPatches,
		Docs:                 config.Docs,
		Examples:             config.Examples,
	}

	diags = resp.Result.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}

func (r *talosMachineConfigurationEphemeralResource) ValidateConfig(ctx context.Context, req ephemeral.ValidateConfigRequest, resp *ephemeral.ValidateConfigResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosMachineConfigurationEphemeralResourceModel

	diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	validateMachineConfigurationConfig(ctx, config.ClusterEndpoint, config.ConfigPatches, &resp.Diagnostics)
}
