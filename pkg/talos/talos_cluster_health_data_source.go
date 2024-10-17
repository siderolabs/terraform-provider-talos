// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/siderolabs/talos/pkg/cluster"
	"github.com/siderolabs/talos/pkg/cluster/check"
	"github.com/siderolabs/talos/pkg/conditions"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

type talosClusterHealthDataSource struct{}

var _ datasource.DataSource = &talosClusterHealthDataSource{}

type talosClusterHealthDataSourceModelV0 struct {
	ID                   types.String        `tfsdk:"id"`
	Endpoints            types.List          `tfsdk:"endpoints"`
	ControlPlaneNodes    types.List          `tfsdk:"control_plane_nodes"`
	WorkerNodes          types.List          `tfsdk:"worker_nodes"`
	ClientConfiguration  clientConfiguration `tfsdk:"client_configuration"`
	Timeouts             timeouts.Value      `tfsdk:"timeouts"`
	SkipKubernetesChecks types.Bool          `tfsdk:"skip_kubernetes_checks"`
}

type clusterNodes struct {
	nodesByType map[machine.Type][]cluster.NodeInfo
	nodes       []cluster.NodeInfo
}

func newClusterNodes(controlPlaneNodes, workerNodes []string) (*clusterNodes, error) {
	controlPlaneNodeInfos, err := cluster.IPsToNodeInfos(controlPlaneNodes)
	if err != nil {
		return nil, err
	}

	workerNodeInfos, err := cluster.IPsToNodeInfos(workerNodes)
	if err != nil {
		return nil, err
	}

	nodesByType := make(map[machine.Type][]cluster.NodeInfo)
	nodesByType[machine.TypeControlPlane] = controlPlaneNodeInfos
	nodesByType[machine.TypeWorker] = workerNodeInfos

	return &clusterNodes{
		nodes:       slices.Concat(controlPlaneNodeInfos, workerNodeInfos),
		nodesByType: nodesByType,
	}, nil
}

// Nodes returns cluster nodeinfos.
func (c *clusterNodes) Nodes() []cluster.NodeInfo {
	return c.nodes
}

// NodesByType returns cluster nodeinfos by type.
func (c *clusterNodes) NodesByType(t machine.Type) []cluster.NodeInfo {
	return c.nodesByType[t]
}

type reporter struct {
	lastLine string
	s        strings.Builder
}

func newReporter() *reporter {
	return &reporter{}
}

// Update implements the conditions.Reporter interface.
func (r *reporter) Update(condition conditions.Condition) {
	if condition.String() != r.lastLine {
		r.s.WriteString(fmt.Sprintf("waiting for %s\n", condition.String()))
		r.lastLine = condition.String()
	}
}

// String returns the string representation of the reporter.
func (r *reporter) String() string {
	return r.s.String()
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
		Description:         "Checks the health of a Talos cluster",
		MarkdownDescription: "Waits for the Talos cluster to be healthy. Can be used as a dependency before running other operations on the cluster.",
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
			"skip_kubernetes_checks": schema.BoolAttribute{
				Optional:    true,
				Description: "Skip Kubernetes component checks, this is useful to check if the nodes has finished booting up and kubelet is running. Default is false.",
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

	c, err := client.New(ctx, client.WithConfig(talosConfig), client.WithEndpoints(endpoints...))
	if err != nil {
		resp.Diagnostics.AddError("failed to create talos client", err.Error())

		return
	}

	defer c.Close() //nolint:errcheck

	clientProvider := &cluster.ConfigClientProvider{
		DefaultClient: c,
	}
	defer clientProvider.Close() //nolint:errcheck

	nodeInfos, err := newClusterNodes(controlPlaneNodes, workerNodes)
	if err != nil {
		resp.Diagnostics.AddError("failed to generate node infos", err.Error())

		return
	}

	clusterState := struct {
		cluster.ClientProvider
		cluster.K8sProvider
		cluster.Info
	}{
		ClientProvider: clientProvider,
		K8sProvider: &cluster.KubernetesClient{
			ClientProvider: clientProvider,
		},
		Info: nodeInfos,
	}

	// Run cluster readiness checks
	checkCtx, checkCtxCancel := context.WithTimeout(ctx, readTimeout)
	defer checkCtxCancel()

	reporter := newReporter()

	checks := slices.Concat(check.PreBootSequenceChecks(), check.K8sComponentsReadinessChecks())

	if !state.SkipKubernetesChecks.ValueBool() {
		checks = check.DefaultClusterChecks()
	}

	if err := check.Wait(checkCtx, &clusterState, checks, reporter); err != nil {
		resp.Diagnostics.AddWarning("failed checks", reporter.String())
		resp.Diagnostics.AddError("cluster health check failed", err.Error())

		return
	}

	state.ID = basetypes.NewStringValue("cluster_health")

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
