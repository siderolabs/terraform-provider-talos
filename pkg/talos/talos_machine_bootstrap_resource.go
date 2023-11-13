// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type talosMachineBootstrapResource struct{}

var (
	_ resource.Resource                 = &talosMachineBootstrapResource{}
	_ resource.ResourceWithModifyPlan   = &talosMachineBootstrapResource{}
	_ resource.ResourceWithUpgradeState = &talosMachineBootstrapResource{}
	_ resource.ResourceWithImportState  = &talosMachineBootstrapResource{}
)

type talosMachineBootstrapResourceModelV0 struct {
	ID          types.String `tfsdk:"id"`
	Endpoint    types.String `tfsdk:"endpoint"`
	Node        types.String `tfsdk:"node"`
	TalosConfig types.String `tfsdk:"talos_config"`
}

type talosMachineBootstrapResourceModelV1 struct {
	ID                  types.String        `tfsdk:"id"`
	Endpoint            types.String        `tfsdk:"endpoint"`
	Node                types.String        `tfsdk:"node"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
	Timeouts            timeouts.Value      `tfsdk:"timeouts"`
}

// NewTalosMachineBootstrapResource implements the resource.Resource interface.
func NewTalosMachineBootstrapResource() resource.Resource {
	return &talosMachineBootstrapResource{}
}

func (r *talosMachineBootstrapResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_bootstrap"
}

func (r *talosMachineBootstrapResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     1,
		Description: "The machine bootstrap resource allows you to bootstrap a Talos node.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "This is a unique identifier for the machine ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The endpoint of the machine to bootstrap",
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "The name of the node to bootstrap",
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
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
			}),
		},
	}
}

func (r *talosMachineBootstrapResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var state talosMachineBootstrapResourceModelV1

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	talosClientConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		state.ClientConfiguration.CA.ValueString(),
		state.ClientConfiguration.Cert.ValueString(),
		state.ClientConfiguration.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError(
			"Error converting config to talos client config",
			err.Error(),
		)

		return
	}

	createTimeout, diags := state.Timeouts.Create(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	if err := retry.RetryContext(ctxDeadline, createTimeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosClientConfig, func(nodeCtx context.Context, c *client.Client) error {
			return c.Bootstrap(nodeCtx, &machineapi.BootstrapRequest{})
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"Error bootstrapping node",
			err.Error(),
		)

		return
	}

	state.ID = basetypes.NewStringValue("machine_bootstrap")

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *talosMachineBootstrapResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (r *talosMachineBootstrapResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var state talosMachineBootstrapResourceModelV1

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r *talosMachineBootstrapResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

func (r talosMachineBootstrapResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	// delete is a no-op
	if req.Plan.Raw.IsNull() {
		return
	}

	var configObj types.Object

	diags := req.Config.Get(ctx, &configObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var config talosMachineBootstrapResourceModelV1

	diags = configObj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// if either endpoint or node is unknown return early
	if config.Endpoint.IsUnknown() || config.Node.IsUnknown() {
		return
	}

	var planObj types.Object

	diags = req.Plan.Get(ctx, &planObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var planState talosMachineBootstrapResourceModelV1

	diags = configObj.As(ctx, &planState, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if planState.Endpoint.IsUnknown() || planState.Endpoint.IsNull() {
		diags = resp.Plan.SetAttribute(ctx, path.Root("endpoint"), planState.Node.ValueString())
		resp.Diagnostics.Append(diags...)

		if diags.HasError() {
			return
		}
	}
}

func (r *talosMachineBootstrapResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"id": schema.StringAttribute{
						Computed: true,
					},
					"endpoint": schema.StringAttribute{
						Required: true,
					},
					"node": schema.StringAttribute{
						Required: true,
					},
					"talos_config": schema.StringAttribute{
						Required: true,
					},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var priorStateData talosMachineBootstrapResourceModelV0

				diags := req.State.Get(ctx, &priorStateData)
				resp.Diagnostics.Append(diags...)
				if diags.HasError() {
					return
				}

				timeout, diag := basetypes.NewObjectValue(map[string]attr.Type{
					"create": types.StringType,
				}, map[string]attr.Value{
					"create": basetypes.NewStringNull(),
				})
				resp.Diagnostics.Append(diag...)
				if resp.Diagnostics.HasError() {
					return
				}

				state := talosMachineBootstrapResourceModelV1{
					ID:       basetypes.NewStringValue("machine_bootstrap"),
					Endpoint: priorStateData.Endpoint,
					Node:     priorStateData.Node,
					Timeouts: timeouts.Value{
						Object: timeout,
					},
				}

				// Set state to fully populated data
				diags = resp.State.Set(ctx, state)
				resp.Diagnostics.Append(diags...)
				if resp.Diagnostics.HasError() {
					return
				}
			},
		},
	}
}

func (r *talosMachineBootstrapResource) ImportState(ctx context.Context, _ resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	timeout, diag := basetypes.NewObjectValue(map[string]attr.Type{
		"create": types.StringType,
	}, map[string]attr.Value{
		"create": basetypes.NewStringNull(),
	})
	resp.Diagnostics.Append(diag...)

	if resp.Diagnostics.HasError() {
		return
	}

	state := talosMachineBootstrapResourceModelV1{
		ID: basetypes.NewStringValue("machine_bootstrap"),
		Timeouts: timeouts.Value{
			Object: timeout,
		},
	}

	// Set state to fully populated data
	diags := resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}
