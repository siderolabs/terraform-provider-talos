// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

var _ ephemeral.EphemeralResource = &talosClientConfigurationEphemeralResource{}

type talosClientConfigurationEphemeralResource struct{}

type talosClientConfigurationEphemeralResourceModel struct {
	ClusterName         types.String        `tfsdk:"cluster_name"`
	MachineSecrets      machineSecrets      `tfsdk:"machine_secrets"`
	NotBefore           types.String        `tfsdk:"not_before"`
	CrtTTL              types.String        `tfsdk:"crt_ttl"`
	Endpoints           types.List          `tfsdk:"endpoints"`
	Nodes               types.List          `tfsdk:"nodes"`
	TalosConfig         types.String        `tfsdk:"talos_config"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
}

// NewTalosClientConfigurationEphemeralResource implements the ephemeral.EphemeralResource interface.
func NewTalosClientConfigurationEphemeralResource() ephemeral.EphemeralResource {
	return &talosClientConfigurationEphemeralResource{}
}

func (r *talosClientConfigurationEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_configuration"
}

func (r *talosClientConfigurationEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate client configuration for a Talos cluster from machine secrets. " +
			"This is an ephemeral resource that does not persist secrets in Terraform state. " +
			"The admin client certificate is generated with pinned timestamps so talos_config " +
			"is byte-identical on every open as long as machine_secrets and not_before are unchanged.",
		Attributes: map[string]schema.Attribute{
			"cluster_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the cluster in the generated config",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"machine_secrets": machineSecretsSchemaAttribute(),
			"not_before": schema.StringAttribute{
				Optional: true,
				Description: "RFC3339 timestamp to use as the NotBefore field of the generated admin client certificate. " +
					"When set, the certificate validity starts at this time and ends at not_before + crt_ttl. " +
					"Persist this value in a terraform_data resource so it is stable across plans and the " +
					"generated talos_config is byte-identical on every open. " +
					"When omitted, the certificate uses the OS CA's own NotBefore/NotAfter timestamps.",
				Validators: []validator.String{
					rfc3339Valid(),
				},
			},
			"crt_ttl": schema.StringAttribute{
				Optional: true,
				Description: "The lifetime of the generated admin client certificate as a Go duration string " +
					"(e.g. \"8760h\" for 1 year, \"87600h\" for 10 years). Defaults to \"87600h\" (10 years). " +
					"Only used when not_before is set; when not_before is omitted the cert uses the OS CA's NotAfter directly.",
				Validators: []validator.String{
					goDurationValid(),
				},
			},
			"endpoints": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "endpoints to set in the generated config",
			},
			"nodes": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "nodes to set in the generated config",
			},
			"talos_config": schema.StringAttribute{
				Computed:    true,
				Description: "The generated client configuration",
				Sensitive:   true,
			},
			"client_configuration": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The client CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Computed:    true,
						Description: "The client certificate",
					},
					"client_key": schema.StringAttribute{
						Computed:    true,
						Sensitive:   true,
						Description: "The client key",
					},
				},
				Computed:    true,
				Sensitive:   true,
				Description: "The generated client configuration data",
			},
		},
	}
}

func (r *talosClientConfigurationEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosClientConfigurationEphemeralResourceModel

	diags = obj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var endpoints []string

	var nodes []string

	resp.Diagnostics.Append(config.Endpoints.ElementsAs(ctx, &endpoints, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(config.Nodes.ElementsAs(ctx, &nodes, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	secretsBundle, err := machineSecretsToSecretsBundle(talosMachineSecretsResourceModelV1{
		MachineSecrets: config.MachineSecrets,
	})
	if err != nil {
		resp.Diagnostics.AddError("failed to convert machine secrets to secrets bundle", err.Error())

		return
	}

	notBefore, notAfter, tsErr := resolveClientConfigTimestamps(config.NotBefore.ValueString(), config.CrtTTL.ValueString(), secretsBundle.Certs.OS.Crt)
	if tsErr != nil {
		resp.Diagnostics.AddError(tsErr.summary, tsErr.detail)

		return
	}

	cc, err := generateClientConfiguration(secretsBundle, config.ClusterName.ValueString(), notBefore, notAfter)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate client configuration", err.Error())

		return
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		config.ClusterName.ValueString(),
		cc.CA.ValueString(),
		cc.Cert.ValueString(),
		cc.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

		return
	}

	if len(endpoints) > 0 {
		talosConfig.Contexts[config.ClusterName.ValueString()].Endpoints = endpoints
	}

	if len(nodes) > 0 {
		talosConfig.Contexts[config.ClusterName.ValueString()].Nodes = nodes
	}

	talosConfigStringBytes, err := talosConfig.Bytes()
	if err != nil {
		resp.Diagnostics.AddError("failed to serialize talos config", err.Error())

		return
	}

	result := talosClientConfigurationEphemeralResourceModel{
		ClusterName:         config.ClusterName,
		MachineSecrets:      config.MachineSecrets,
		NotBefore:           config.NotBefore,
		CrtTTL:              config.CrtTTL,
		Endpoints:           config.Endpoints,
		Nodes:               config.Nodes,
		TalosConfig:         basetypes.NewStringValue(string(talosConfigStringBytes)),
		ClientConfiguration: cc,
	}

	diags = resp.Result.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}
