// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/pkg/machinery/cel"
	"github.com/siderolabs/talos/pkg/machinery/cel/celenv"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config/types/block/blockhelpers"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

//go:generate go run internal/gen/diskspec.go block.DiskSpec talos_machine_disks_data_source

func (e nodiskFoundError) Error() string {
	return "no disk matching the filter found"
}

type nodiskFoundError struct{}

type talosMachineDisksDataSource struct{}

type talosMachineDisksDataSourceModelV1 struct { //nolint:govet
	ID                  types.String        `tfsdk:"id"`
	Node                types.String        `tfsdk:"node"`
	Endpoint            types.String        `tfsdk:"endpoint"`
	Selector            types.String        `tfsdk:"selector"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
	Disks               []diskspec          `tfsdk:"disks"`
	Timeouts            timeouts.Value      `tfsdk:"timeouts"`
}

var _ datasource.DataSource = &talosMachineDisksDataSource{}

// NewTalosMachineDisksDataSource implements the datasource.DataSource interface.
func NewTalosMachineDisksDataSource() datasource.DataSource {
	return &talosMachineDisksDataSource{}
}

func (d *talosMachineDisksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_disks"
}

func (d *talosMachineDisksDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Generate a machine configuration for a node type",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The generated ID of this resource",
				Computed:    true,
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "controlplane node to retrieve the kubeconfig from",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "endpoint to use for the talosclient. If not set, the node value will be used",
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
			"selector": schema.StringAttribute{
				Optional: true,
				MarkdownDescription: `The CEL expression to filter the disks.
If not set, all disks will be returned.
See [CEL documentation](https://www.talos.dev/latest/talos-guides/configuration/disk-management/#disk-selector).`,
			},
			"disks": schema.ListNestedAttribute{
				Description: "The disks that match the filters",
				NestedObject: schema.NestedAttributeObject{
					Attributes: diskspecAttributes,
				},
				Computed: true,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Read: true,
			}),
		},
	}
}

func (d *talosMachineDisksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) { //nolint:gocognit,gocyclo,cyclop
	var obj types.Object

	diags := req.Config.Get(ctx, &obj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state talosMachineDisksDataSourceModelV1

	diags = obj.As(ctx, &state, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		state.ClientConfiguration.CA.ValueString(),
		state.ClientConfiguration.Cert.ValueString(),
		state.ClientConfiguration.Key.ValueString(),
	)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate talos config", err.Error())

		return
	}

	if state.Endpoint.IsNull() {
		state.Endpoint = state.Node
	}

	readTimeout, diags := state.Timeouts.Read(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	selector := state.Selector.ValueString()

	if selector == "" {
		// if there is no selector, we can return all disks
		selector = "true"
	}

	exp, err := cel.ParseBooleanExpression(selector, celenv.DiskLocator())
	if err != nil {
		resp.Diagnostics.AddError("failed to parse celenv selector", err.Error())

		return
	}

	if err := retry.RetryContext(ctxDeadline, readTimeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			disks, err := blockhelpers.MatchDisks(nodeCtx, c.COSI, &exp)
			if err != nil {
				return err
			}

			for _, disk := range disks {
				state.Disks = append(state.Disks, diskspecToTFTypes(*disk.TypedSpec()))
			}

			return nil
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			if errors.Is(err, nodiskFoundError{}) {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics.AddError("failed to get list of disks", err.Error())

		return
	}

	state.ID = basetypes.NewStringValue("machine_disks")

	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}
