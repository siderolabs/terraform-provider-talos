// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/go-kubernetes/kubernetes/ssa"
	goupgrade "github.com/siderolabs/go-kubernetes/kubernetes/upgrade"
	"github.com/siderolabs/talos/pkg/cluster"
	"github.com/siderolabs/talos/pkg/cluster/check"
	k8s "github.com/siderolabs/talos/pkg/cluster/kubernetes"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"github.com/siderolabs/talos/pkg/machinery/constants"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type talosClusterResource struct{}

var (
	_ resource.Resource                   = &talosClusterResource{}
	_ resource.ResourceWithModifyPlan     = &talosClusterResource{}
	_ resource.ResourceWithValidateConfig = &talosClusterResource{}
)

type talosClusterResourceModel struct {
	ClientConfiguration   basetypes.ObjectValue `tfsdk:"client_configuration"`
	ClientConfigurationWO basetypes.ObjectValue `tfsdk:"client_configuration_wo"`
	ControlPlaneNodes     types.List            `tfsdk:"control_plane_nodes"`
	Endpoint              types.String          `tfsdk:"endpoint"`
	ID                    types.String          `tfsdk:"id"`
	KubernetesVersion     types.String          `tfsdk:"kubernetes_version"`
	Node                  types.String          `tfsdk:"node"`
	Timeouts              timeouts.Value        `tfsdk:"timeouts"`

	MachineSecrets          basetypes.ObjectValue `tfsdk:"machine_secrets"`
	MachineSecretsWO        basetypes.ObjectValue `tfsdk:"machine_secrets_wo"`
	KubernetesCACertificate types.String          `tfsdk:"kubernetes_ca_certificate"`
	TalosCACertificate      types.String          `tfsdk:"talos_ca_certificate"`
	EtcdCACertificate       types.String          `tfsdk:"etcd_ca_certificate"`

	KubernetesAggregatorCACertificate types.String `tfsdk:"kubernetes_aggregator_ca_certificate"`
}

// NewTalosClusterResource implements the resource.Resource interface.
func NewTalosClusterResource() resource.Resource {
	return &talosClusterResource{}
}

func (r *talosClusterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_cluster"
}

func (r *talosClusterResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Talos cluster: bootstraps etcd and tracks Kubernetes version. " +
			"This resource completes once the Talos layer (etcd, apid, kubelet) is healthy across all control plane nodes. " +
			"It does not wait for Kubernetes components to be ready — use talos_cluster_health for that before depending on the Kubernetes API.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"node": schema.StringAttribute{
				Required: true,
				Description: "The IP address of the control plane node to bootstrap etcd on. Must be an IP: it defaults into " +
					"`control_plane_nodes`, which the health checks compare against the addresses Kubernetes reports. " +
					"Use `endpoint` to connect through a hostname.",
				Validators: []validator.String{
					ipAddressValid(),
				},
			},
			"control_plane_nodes": schema.ListAttribute{
				Optional:    true,
				Computed:    true,
				ElementType: types.StringType,
				Description: "List of all control plane node IPs used for etcd health checks. Defaults to [node]. Required for HA clusters where all control plane IPs must be listed.",
				Validators: []validator.List{
					listvalidator.ValueStringsAre(ipAddressValid()),
				},
				PlanModifiers: []planmodifier.List{
					listplanmodifier.UseStateForUnknown(),
				},
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The endpoint to use when connecting to the node. Defaults to node.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"client_configuration": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "The Talos client configuration. Use client_configuration_wo when using ephemeral resources.",
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Required:    true,
						Description: "The client CA certificate.",
					},
					"client_certificate": schema.StringAttribute{
						Required:    true,
						Description: "The client certificate.",
					},
					"client_key": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						Description: "The client key.",
					},
				},
			},
			"client_configuration_wo": schema.SingleNestedAttribute{
				Optional:    true,
				WriteOnly:   true,
				Description: "Write-only variant of client_configuration for use with ephemeral resources. Requires Terraform 1.11+.",
				Attributes: map[string]schema.Attribute{
					"ca_certificate": schema.StringAttribute{
						Required:    true,
						WriteOnly:   true,
						Description: "The client CA certificate.",
					},
					"client_certificate": schema.StringAttribute{
						Required:    true,
						WriteOnly:   true,
						Description: "The client certificate.",
					},
					"client_key": schema.StringAttribute{
						Required:    true,
						Sensitive:   true,
						WriteOnly:   true,
						Description: "The client key.",
					},
				},
			},
			clusterMachineSecretsAttribute: clusterMachineSecretsSchemaAttribute(false,
				"Optional machine secrets bundle of the cluster, used only to compute the public CA certificates "+
					"exposed by this resource (`kubernetes_ca_certificate` and the other `*_ca_certificate` attributes). The whole bundle is stored in the state: its "+
					"keys and tokens are marked sensitive but are still in the state file. Use `machine_secrets_wo` to keep "+
					"it out of the state. Any change to the bundle is shown as an in-place update. Only one of "+
					"`machine_secrets` and `machine_secrets_wo` can be set."),
			clusterMachineSecretsWOAttribute: clusterMachineSecretsSchemaAttribute(true,
				"Write-only variant of `machine_secrets` for use with ephemeral resources. Requires Terraform 1.11+."),
			"kubernetes_version": schema.StringAttribute{
				Required: true,
				Description: "Desired Kubernetes version (e.g. `v1.32.0`). " +
					"This is the canonical driver for Kubernetes upgrades: changing it runs Talos's " +
					"`upgrade-k8s` procedure, which sequentially upgrades kube-apiserver, " +
					"kube-controller-manager, kube-scheduler, kube-proxy, and kubelet on each node " +
					"with health gating. " +
					"Set `ignore_kubernetes_upgrade_drift = true` on `talos_machine` so that image tag " +
					"changes owned by `upgrade-k8s` are excluded from drift detection and `talos_cluster` " +
					"fully owns the upgrade sequencing.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},
	}

	for _, ca := range clusterCACertificates {
		resp.Schema.Attributes[ca.attribute] = clusterCACertificateAttribute(ca)
	}
}

func (r *talosClusterResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg talosClusterResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)

	if resp.Diagnostics.HasError() {
		return
	}

	clientSet := !cfg.ClientConfiguration.IsNull()
	clientWOSet := !cfg.ClientConfigurationWO.IsNull()

	if !clientSet && !clientWOSet {
		resp.Diagnostics.AddError(
			"Missing client configuration",
			"Exactly one of client_configuration or client_configuration_wo must be set.",
		)
	}

	if clientSet && clientWOSet {
		resp.Diagnostics.AddError(
			"Conflicting client configuration",
			"Only one of client_configuration or client_configuration_wo can be set, not both.",
		)
	}

	resp.Diagnostics.Append(validateClusterMachineSecrets(ctx, &cfg, req.Config)...)

	if !cfg.ControlPlaneNodes.IsNull() && !cfg.Node.IsNull() && !cfg.Node.IsUnknown() {
		var nodes []string

		if diags := cfg.ControlPlaneNodes.ElementsAs(ctx, &nodes, false); !diags.HasError() {
			if !slices.Contains(nodes, cfg.Node.ValueString()) {
				resp.Diagnostics.AddAttributeError(
					path.Root("control_plane_nodes"),
					"node not in control_plane_nodes",
					fmt.Sprintf("node %q must be included in control_plane_nodes", cfg.Node.ValueString()),
				)
			}
		}
	}
}

func (r *talosClusterResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan talosClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var cfgFromConfig talosClusterResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfgFromConfig)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if cfgFromConfig.Endpoint.IsNull() && !plan.Node.IsUnknown() && !plan.Node.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("endpoint"), plan.Node)...)
	}

	if cfgFromConfig.ControlPlaneNodes.IsNull() && !plan.Node.IsUnknown() && !plan.Node.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("control_plane_nodes"),
			types.ListValueMust(types.StringType, []attr.Value{plan.Node}))...)
	}

	// The CA certificates come from the machine secrets input in the config
	// (write-only values are only available there), so they are known at plan
	// time. Without an input they stay null, whatever else changes in this plan.
	input, diags := clusterMachineSecretsInput(ctx, req.Config)

	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	for _, ca := range clusterCACertificates {
		cert, certDiags := clusterCACertificateFromInput(ctx, req.Config, input, ca)

		resp.Diagnostics.Append(certDiags...)

		if resp.Diagnostics.HasError() {
			return
		}

		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root(ca.attribute), cert)...)
	}
}

func (r *talosClusterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan talosClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var cfgModel talosClusterResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfgModel)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Fill the CA certificates that were unknown at plan time first: a bundle that
	// turns out to be invalid or in conflict must fail before anything is done to
	// the cluster.
	resp.Diagnostics.Append(resolveClusterCACertificates(ctx, req.Config, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !cfgModel.ClientConfigurationWO.IsNull() {
		plan.ClientConfigurationWO = cfgModel.ClientConfigurationWO
	}

	talosConfig, err := resolveTalosClusterClientConfig(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config", err.Error())

		return
	}

	timeout, diags := plan.Timeouts.Create(ctx, 20*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := talosClusterEffectiveEndpoint(&plan)
	plan.Endpoint = types.StringValue(endpoint)

	if bootstrapErr := talosClusterBootstrap(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig); bootstrapErr != nil {
		resp.Diagnostics.AddError("error bootstrapping etcd", bootstrapErr.Error())

		return
	}

	var controlPlaneNodes []string

	resp.Diagnostics.Append(plan.ControlPlaneNodes.ElementsAs(ctx, &controlPlaneNodes, true)...)

	if resp.Diagnostics.HasError() {
		return
	}

	clusterReporter := newReporter()

	if err = talosClusterWaitForK8s(ctxDeadline, endpoint, controlPlaneNodes, talosConfig, clusterReporter); err != nil {
		if progress := clusterReporter.String(); progress != "" {
			resp.Diagnostics.AddWarning("failed checks", progress)
		}

		resp.Diagnostics.AddError("error waiting for cluster health", err.Error())

		return
	}

	plan.ID = types.StringValue(plan.Node.ValueString())

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *talosClusterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state talosClusterResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *talosClusterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state talosClusterResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	var cfgModel talosClusterResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfgModel)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Fill the CA certificates that were unknown at plan time first: a bundle that
	// turns out to be invalid or in conflict must fail before anything is done to
	// the cluster.
	resp.Diagnostics.Append(resolveClusterCACertificates(ctx, req.Config, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !cfgModel.ClientConfigurationWO.IsNull() {
		plan.ClientConfigurationWO = cfgModel.ClientConfigurationWO
	}

	talosConfig, err := resolveTalosClusterClientConfig(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config", err.Error())

		return
	}

	timeout, diags := plan.Timeouts.Update(ctx, 30*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := talosClusterEffectiveEndpoint(&plan)
	plan.Endpoint = types.StringValue(endpoint)

	if !plan.KubernetesVersion.Equal(state.KubernetesVersion) {
		if upgradeErr := talosClusterUpgradeKubernetes(ctxDeadline, endpoint, talosConfig, plan.KubernetesVersion.ValueString()); upgradeErr != nil {
			resp.Diagnostics.AddError("error upgrading Kubernetes", upgradeErr.Error())

			return
		}
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *talosClusterResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// talosClusterBootstrap sends the Bootstrap RPC, retrying until etcd is bootstrapped.
// codes.AlreadyExists is treated as success — bootstrap is idempotent.
// The retry budget matches the caller's context deadline so the full configured
// timeout is available rather than a hard-coded sub-window.
func talosClusterBootstrap(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config) error {
	timeout := 10 * time.Minute // fallback when ctx has no deadline

	if deadline, ok := ctx.Deadline(); ok {
		timeout = time.Until(deadline)
	}

	return retry.RetryContext(ctx, timeout, func() *retry.RetryError {
		c, err := client.New(ctx, client.WithConfig(talosConfig), client.WithEndpoints(endpoint))
		if err != nil {
			return retry.RetryableError(err)
		}

		defer c.Close() //nolint:errcheck

		if err := c.Bootstrap(client.WithNode(ctx, node), nil); err != nil {
			if s := status.Code(err); s == codes.AlreadyExists {
				return nil
			} else if s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	})
}

// talosClusterWaitForK8s waits for the cluster to pass all default health checks.
func talosClusterWaitForK8s(ctx context.Context, endpoint string, controlPlaneNodes []string, talosConfig *clientconfig.Config, progress *reporter) error {
	c, err := client.New(ctx, client.WithConfig(talosConfig), client.WithEndpoints(endpoint))
	if err != nil {
		return err
	}

	defer c.Close() //nolint:errcheck

	clientProvider := &cluster.ConfigClientProvider{DefaultClient: c}
	defer clientProvider.Close() //nolint:errcheck

	nodeInfos, err := newClusterNodes(controlPlaneNodes, nil)
	if err != nil {
		return err
	}

	clusterState := struct {
		cluster.ClientProvider
		cluster.K8sProvider
		cluster.Info
	}{
		ClientProvider: clientProvider,
		K8sProvider:    &cluster.KubernetesClient{ClientProvider: clientProvider},
		Info:           nodeInfos,
	}

	return check.Wait(ctx, &clusterState, check.PreBootSequenceChecks(), progress)
}

// talosClusterUpgradeKubernetes runs a rolling Kubernetes upgrade via the talos cluster package.
func talosClusterUpgradeKubernetes(ctx context.Context, endpoint string, talosConfig *clientconfig.Config, toVersion string) error {
	c, err := client.New(ctx, client.WithConfig(talosConfig), client.WithEndpoints(endpoint))
	if err != nil {
		return err
	}

	defer c.Close() //nolint:errcheck

	clientProvider := &cluster.ConfigClientProvider{DefaultClient: c}
	defer clientProvider.Close() //nolint:errcheck

	clusterState := struct {
		cluster.ClientProvider
		cluster.K8sProvider
	}{
		ClientProvider: clientProvider,
		K8sProvider: &cluster.KubernetesClient{
			ClientProvider: clientProvider,
			ForceEndpoint:  endpoint,
		},
	}

	opts := k8s.UpgradeOptions{
		ControlPlaneEndpoint:   endpoint,
		PrePullImages:          true,
		UpgradeKubelet:         true,
		DryRun:                 false,
		EncoderOpt:             encoder.WithComments(encoder.CommentsDisabled),
		InventoryPolicy:        ssa.InventoryPolicyAdoptIfNoInventory,
		ReconcileTimeout:       5 * time.Minute,
		KubeletImage:           constants.KubeletImage,
		APIServerImage:         constants.KubernetesAPIServerImage,
		ControllerManagerImage: constants.KubernetesControllerManagerImage,
		SchedulerImage:         constants.KubernetesSchedulerImage,
		ProxyImage:             constants.KubeProxyImage,
	}

	fromVersion, err := k8s.DetectLowestVersion(ctx, &clusterState, opts)
	if err != nil {
		return err
	}

	opts.Path, err = goupgrade.NewPath(fromVersion, strings.TrimPrefix(toVersion, "v"))
	if err != nil {
		return err
	}

	return k8s.Upgrade(ctx, &clusterState, opts)
}

// resolveTalosClusterClientConfig builds the Talos client config from the write-only or
// regular client_configuration attribute. Write-only credentials are never persisted
// in state; callers must not assign client_configuration from this return value.
func resolveTalosClusterClientConfig(ctx context.Context, state *talosClusterResourceModel) (*clientconfig.Config, error) {
	var clientObj basetypes.ObjectValue

	switch {
	case !state.ClientConfigurationWO.IsNull() && !state.ClientConfigurationWO.IsUnknown():
		clientObj = state.ClientConfigurationWO
	case !state.ClientConfiguration.IsNull():
		clientObj = state.ClientConfiguration
	default:
		return nil, errors.New("no client configuration available")
	}

	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, clientObj)
	if !ok {
		return nil, errors.New(errMsg)
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig("dynamic", ca, cert, key)
	if err != nil {
		return nil, err
	}

	return talosConfig, nil
}

// talosClusterEffectiveEndpoint returns the endpoint, defaulting to node.
func talosClusterEffectiveEndpoint(state *talosClusterResourceModel) string {
	if !state.Endpoint.IsNull() && !state.Endpoint.IsUnknown() && state.Endpoint.ValueString() != "" {
		return state.Endpoint.ValueString()
	}

	return state.Node.ValueString()
}
