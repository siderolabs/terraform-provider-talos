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
	ID                    types.String          `tfsdk:"id"`
	Endpoint              types.String          `tfsdk:"endpoint"`
	Node                  types.String          `tfsdk:"node"`
	ClientConfiguration   basetypes.ObjectValue `tfsdk:"client_configuration"`
	ClientConfigurationWO basetypes.ObjectValue `tfsdk:"client_configuration_wo"`
	Timeouts              timeouts.Value        `tfsdk:"timeouts"`
}

// NewTalosMachineBootstrapResource implements the resource.Resource interface.
func NewTalosMachineBootstrapResource() resource.Resource {
	return &talosMachineBootstrapResource{}
}

func (r *talosMachineBootstrapResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_bootstrap"
}

func (r *talosMachineBootstrapResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config talosMachineBootstrapResourceModelV1

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	clientConfigSet := !config.ClientConfiguration.IsNull()
	clientConfigWOSet := !config.ClientConfigurationWO.IsNull()

	if !clientConfigSet && !clientConfigWOSet {
		resp.Diagnostics.AddError(
			"Missing client configuration",
			"Exactly one of client_configuration or client_configuration_wo must be set",
		)
	}

	if clientConfigSet && clientConfigWOSet {
		resp.Diagnostics.AddError(
			"Conflicting client configuration",
			"Only one of client_configuration or client_configuration_wo can be set, not both",
		)
	}
}

// getClientConfiguration returns the effective client configuration,
// preferring the write-only attribute if set.
func getBootstrapClientConfiguration(state *talosMachineBootstrapResourceModelV1) (config basetypes.ObjectValue, diagMsg string) {
	woIsNull := state.ClientConfigurationWO.IsNull()
	woIsUnknown := state.ClientConfigurationWO.IsUnknown()
	regularIsNull := state.ClientConfiguration.IsNull()

	// Prefer write-only if available and known
	if !woIsNull && !woIsUnknown {
		return state.ClientConfigurationWO, ""
	}

	// If write-only was provided but is still unknown, that's a problem
	if !woIsNull && woIsUnknown {
		return basetypes.NewObjectNull(map[string]attr.Type{
			"ca_certificate":     types.StringType,
			"client_certificate": types.StringType,
			"client_key":         types.StringType,
		}), "client_configuration_wo is still unknown (ephemeral value not yet resolved)"
	}

	// Fall back to regular client_configuration
	if !regularIsNull {
		return state.ClientConfiguration, ""
	}

	// Both are null
	return basetypes.NewObjectNull(map[string]attr.Type{
		"ca_certificate":     types.StringType,
		"client_certificate": types.StringType,
		"client_key":         types.StringType,
	}), "both client_configuration and client_configuration_wo are null"
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
				Optional:    true,
				Description: "The client configuration data",
			},
			"client_configuration_wo": schema.SingleNestedAttribute{
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Required:    true,
						WriteOnly:   true,
						Description: "The client CA certificate",
					},
					"client_certificate": schema.StringAttribute{
						Required:    true,
						WriteOnly:   true,
						Description: "The client certificate",
					},
					"client_key": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						WriteOnly:   true,
						Description: "The client key",
					},
				},
				Optional:    true,
				WriteOnly:   true,
				Description: "The client configuration data (write-only). Use this instead of client_configuration when using ephemeral resources. Requires Terraform 1.11+",
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

	// CRITICAL: Write-only attributes are NOT in Plan, only in Config!
	var configState talosMachineBootstrapResourceModelV1

	configDiags := req.Config.Get(ctx, &configState)
	resp.Diagnostics.Append(configDiags...)

	if configDiags.HasError() {
		return
	}

	// Use write-only client_configuration from Config
	if !configState.ClientConfigurationWO.IsNull() {
		state.ClientConfigurationWO = configState.ClientConfigurationWO
	}

	clientConfig, configDiag := getBootstrapClientConfiguration(&state)
	if configDiag != "" {
		resp.Diagnostics.AddError(
			"Client configuration issue",
			configDiag,
		)

		return
	}

	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, clientConfig)
	if !ok {
		resp.Diagnostics.AddError(
			"Error extracting client configuration",
			errMsg,
		)

		return
	}

	talosClientConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		ca,
		cert,
		key,
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

func (r *talosMachineBootstrapResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
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

				// Create null client configuration with proper type
				clientConfig := basetypes.NewObjectNull(map[string]attr.Type{
					"ca_certificate":     types.StringType,
					"client_certificate": types.StringType,
					"client_key":         types.StringType,
				})

				state := talosMachineBootstrapResourceModelV1{
					ID:                    basetypes.NewStringValue("machine_bootstrap"),
					Endpoint:              priorStateData.Endpoint,
					Node:                  priorStateData.Node,
					ClientConfiguration:   clientConfig,
					ClientConfigurationWO: clientConfig,
					Timeouts: timeouts.Value{
						Object: timeout,
					},
				}

				// Set state to fully populated data
				diags = resp.State.Set(ctx, &state)
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

	// Create null client configuration with proper type
	clientConfig := basetypes.NewObjectNull(map[string]attr.Type{
		"ca_certificate":     types.StringType,
		"client_certificate": types.StringType,
		"client_key":         types.StringType,
	})

	state := talosMachineBootstrapResourceModelV1{
		ID:                    basetypes.NewStringValue("machine_bootstrap"),
		ClientConfiguration:   clientConfig,
		ClientConfigurationWO: clientConfig,
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
