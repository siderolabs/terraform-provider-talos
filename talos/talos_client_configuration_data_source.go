// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
)

type talosClientConfigurationDataSource struct{}

type talosClientConfigurationDataSourceModel struct {
	ClusterName         types.String        `tfsdk:"cluster_name"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
	Endpoints           types.List          `tfsdk:"endpoints"`
	Nodes               types.List          `tfsdk:"nodes"`
	TalosConfig         types.String        `tfsdk:"talos_config"`
}

var (
	_ datasource.DataSource = &talosClientConfigurationDataSource{}
)

func NewTalosClientConfigurationDataSource() datasource.DataSource {
	return &talosClientConfigurationDataSource{}
}

func (d *talosClientConfigurationDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_configuration"
}

func (d *talosClientConfigurationDataSource) Schema(_ context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate client configuration for a Talos cluster",
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

func (d *talosClientConfigurationDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state talosClientConfigurationDataSourceModel

	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	var endpoints []string
	var nodes []string

	resp.Diagnostics.Append(state.Endpoints.ElementsAs(ctx, &endpoints, false)...)
	resp.Diagnostics.Append(state.Nodes.ElementsAs(ctx, &nodes, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		state.ClusterName.ValueString(),
		state.ClientConfiguration.CA.ValueString(),
		state.ClientConfiguration.Cert.ValueString(),
		state.ClientConfiguration.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

		return
	}

	if len(endpoints) > 0 {
		talosConfig.Contexts[state.ClusterName.ValueString()].Endpoints = endpoints
	}

	if len(nodes) > 0 {
		talosConfig.Contexts[state.ClusterName.ValueString()].Nodes = nodes
	}

	talosConfigStringBytes, err := talosConfig.Bytes()
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

		return
	}

	state.TalosConfig = basetypes.NewStringValue(string(talosConfigStringBytes))

	diags = resp.State.Set(ctx, state)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}
