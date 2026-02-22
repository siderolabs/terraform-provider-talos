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
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
	Endpoints           types.List          `tfsdk:"endpoints"`
	Nodes               types.List          `tfsdk:"nodes"`
	TalosConfig         types.String        `tfsdk:"talos_config"`
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
		Description: "Generate client configuration for a Talos cluster. This is an ephemeral resource that does not persist secrets in Terraform state.",
		Attributes: map[string]schema.Attribute{
			"cluster_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the cluster in the generated config",
				Validators: []validator.String{
					stringvalidator.LengthAtLeast(1),
				},
			},
			"client_configuration": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Required:    true,
						Description: "The client CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Required:    true,
						Description: "The client certificate",
					},
					"client_key": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						Description: "The client key",
					},
				},
				Required:    true,
				Description: "The client configuration data",
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

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		config.ClusterName.ValueString(),
		config.ClientConfiguration.CA.ValueString(),
		config.ClientConfiguration.Cert.ValueString(),
		config.ClientConfiguration.Key.ValueString(),
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
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

		return
	}

	// Build result
	result := talosClientConfigurationEphemeralResourceModel{
		ClusterName:         config.ClusterName,
		ClientConfiguration: config.ClientConfiguration,
		Endpoints:           config.Endpoints,
		Nodes:               config.Nodes,
		TalosConfig:         basetypes.NewStringValue(string(talosConfigStringBytes)),
	}

	diags = resp.Result.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}
