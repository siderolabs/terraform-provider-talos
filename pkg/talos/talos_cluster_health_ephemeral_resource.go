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

	"github.com/hashicorp/terraform-plugin-framework/ephemeral"
	"github.com/hashicorp/terraform-plugin-framework/ephemeral/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/talos/pkg/cluster"
	"github.com/siderolabs/talos/pkg/cluster/check"
	"github.com/siderolabs/talos/pkg/conditions"
	"github.com/siderolabs/talos/pkg/machinery/client"
)

var _ ephemeral.EphemeralResource = &talosClusterHealthEphemeralResource{}

type talosClusterHealthEphemeralResource struct{}

type talosClusterHealthEphemeralResourceModel struct {
	ClientConfiguration  clientConfiguration `tfsdk:"client_configuration"`
	Endpoints            types.List          `tfsdk:"endpoints"`
	ControlPlaneNodes    types.List          `tfsdk:"control_plane_nodes"`
	WorkerNodes          types.List          `tfsdk:"worker_nodes"`
	Timeout              types.String        `tfsdk:"timeout"`
	SkipKubernetesChecks types.Bool          `tfsdk:"skip_kubernetes_checks"`
}

type healthReporter struct {
	lastLine string
	s        strings.Builder
}

func newHealthReporter() *healthReporter {
	return &healthReporter{}
}

// Update implements the conditions.Reporter interface.
func (r *healthReporter) Update(condition conditions.Condition) {
	if condition.String() != r.lastLine {
		fmt.Fprintf(&r.s, "waiting for %s\n", condition.String())
		r.lastLine = condition.String()
	}
}

// String returns the string representation of the reporter.
func (r *healthReporter) String() string {
	return r.s.String()
}

// NewTalosClusterHealthEphemeralResource implements the ephemeral.EphemeralResource interface.
func NewTalosClusterHealthEphemeralResource() ephemeral.EphemeralResource {
	return &talosClusterHealthEphemeralResource{}
}

func (r *talosClusterHealthEphemeralResource) Metadata(_ context.Context, req ephemeral.MetadataRequest, resp *ephemeral.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster_health"
}

func (r *talosClusterHealthEphemeralResource) Schema(_ context.Context, _ ephemeral.SchemaRequest, resp *ephemeral.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Checks the health of a Talos cluster. This is an ephemeral resource that does not persist secrets in Terraform state.",
		Attributes: map[string]schema.Attribute{
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
			"timeout": schema.StringAttribute{
				Optional:    true,
				Description: "Timeout for the health check. Defaults to 10m. Valid time units are 'ns', 'us' (or 'µs'), 'ms', 's', 'm', 'h'.",
			},
		},
	}
}

func (r *talosClusterHealthEphemeralResource) Open(ctx context.Context, req ephemeral.OpenRequest, resp *ephemeral.OpenResponse) {
	var config talosClusterHealthEphemeralResourceModel

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var (
		endpoints         []string
		controlPlaneNodes []string
		workerNodes       []string
	)

	resp.Diagnostics.Append(config.Endpoints.ElementsAs(ctx, &endpoints, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(config.ControlPlaneNodes.ElementsAs(ctx, &controlPlaneNodes, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(config.WorkerNodes.ElementsAs(ctx, &workerNodes, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Parse timeout
	timeout := 10 * time.Minute

	if !config.Timeout.IsNull() && !config.Timeout.IsUnknown() {
		var err error

		timeout, err = time.ParseDuration(config.Timeout.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Invalid timeout", fmt.Sprintf("Unable to parse timeout: %s", err.Error()))

			return
		}
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		config.ClientConfiguration.CA.ValueString(),
		config.ClientConfiguration.Cert.ValueString(),
		config.ClientConfiguration.Key.ValueString(),
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
	checkCtx, checkCtxCancel := context.WithTimeout(ctx, timeout)
	defer checkCtxCancel()

	reporter := newHealthReporter()

	checks := slices.Concat(check.PreBootSequenceChecks(), check.K8sComponentsReadinessChecks())

	if !config.SkipKubernetesChecks.ValueBool() {
		checks = check.DefaultClusterChecks()
	}

	if err := check.Wait(checkCtx, &clusterState, checks, reporter); err != nil {
		resp.Diagnostics.AddWarning("failed checks", reporter.String())
		resp.Diagnostics.AddError("cluster health check failed", err.Error())

		return
	}

	// Set result - ephemeral resources can set a result or just complete successfully
	resp.Diagnostics.Append(resp.Result.Set(ctx, &config)...)

	if resp.Diagnostics.HasError() {
		return
	}
}
