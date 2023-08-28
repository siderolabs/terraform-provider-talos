// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/machinery/api/cluster"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
)

type talosClusterHealthDataSource struct{}

var _ datasource.DataSource = &talosClusterHealthDataSource{}

type talosClusterHealthDataSourceModelV0 struct {
	ID                  types.String        `tfsdk:"id"`
	Endpoints           types.List          `tfsdk:"endpoints"`
	ControlPlaneNodes   types.List          `tfsdk:"control_plane_nodes"`
	WorkerNodes         types.List          `tfsdk:"worker_nodes"`
	ClientConfiguration clientConfiguration `tfsdk:"client_configuration"`
	Timeouts            timeouts.Value      `tfsdk:"timeouts"`
}

// NewTalosClusterHealthDataSource implements the datasource.DataSource interface.
func NewTalosClusterHealthDataSource() datasource.DataSource {
	return &talosClusterHealthDataSource{}
}

func (d *talosClusterHealthDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_health"
}

func (d *talosClusterHealthDataSource) Schema(ctx context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Checks the health of a Talos cluster",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
			},
			"endpoints": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "endpoints to use for the health check client. Use at least one control plane endpoint.",
			},
			"control_plane_nodes": schema.ListAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "List of control plane nodes to check for health.",
			},
			"worker_nodes": schema.ListAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "List of worker nodes to check for health.",
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
				Read: true,
			}),
		},
	}
}

func (d *talosClusterHealthDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state talosClusterHealthDataSourceModelV0

	diags := req.Config.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var (
		endpoints         []string
		controlPlaneNodes []string
		workerNodes       []string
	)

	resp.Diagnostics.Append(state.Endpoints.ElementsAs(ctx, &endpoints, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(state.ControlPlaneNodes.ElementsAs(ctx, &controlPlaneNodes, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(state.WorkerNodes.ElementsAs(ctx, &workerNodes, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	readTimeout, diags := state.Timeouts.Read(ctx, 10*time.Minute)
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

	// since we're going to run this on a fully, this needs full PKI and all nodes/endpoins, so directly creating client
	// and running the health check
	c, err := client.New(ctx, client.WithConfig(talosConfig), client.WithEndpoints(endpoints...))
	if err != nil {
		resp.Diagnostics.AddError("failed to create talos client", err.Error())

		return
	}

	defer c.Close() //nolint:errcheck

	healthCheckClient, err := c.ClusterHealthCheck(ctx, readTimeout, &cluster.ClusterInfo{
		ControlPlaneNodes: controlPlaneNodes,
		WorkerNodes:       workerNodes,
	})
	if err != nil {
		resp.Diagnostics.AddError("failed to create health check client", err.Error())

		return
	}

	if err = healthCheckClient.CloseSend(); err != nil {
		resp.Diagnostics.AddError("failed to close health check client", err.Error())

		return
	}

	var messages []string

	for {
		msg, err := healthCheckClient.Recv()
		if err != nil {
			if errors.Is(err, io.EOF) || client.StatusCode(err) == codes.Canceled {
				state.ID = basetypes.NewStringValue("cluster_health")

				resp.State.Set(ctx, state)

				return
			}

			resp.Diagnostics.AddError(fmt.Sprintf("health check messages:\n%s\n", strings.Join(messages, "\n")), err.Error())

			return
		}

		if msg.GetMetadata().GetError() != "" {
			resp.Diagnostics.AddError("healthcheck error", msg.GetMetadata().GetError())

			return
		}

		messages = append(messages, msg.GetMessage())
	}
}
