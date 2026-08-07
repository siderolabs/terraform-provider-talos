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

	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/action"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/nodedrain"
	"github.com/siderolabs/talos/pkg/images"
	commonapi "github.com/siderolabs/talos/pkg/machinery/api/common"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	configresource "github.com/siderolabs/talos/pkg/machinery/resources/config"
	talosreporter "github.com/siderolabs/talos/pkg/reporter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// DefaultCreateTimeout and DefaultUpdateTimeout are the out-of-the-box defaults
// for talos_machine timeouts. Exported so tests can assert they cover the
// worst-case internal retry budgets without duplicating magic numbers.
const (
	DefaultCreateTimeout = 25 * time.Minute // 10m apply + 10m wait-for-node + margin
	DefaultUpdateTimeout = 90 * time.Minute // above + 60m legacy upgrade poll + margin
)

// clientOpFunc matches the signature of talosClientOp and lets unit tests inject
// a mock into talosMachineUpgradeLegacy without touching any pre-existing file.
type clientOpFunc func(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, fn func(nodeCtx context.Context, c *client.Client) error) error

type talosMachineResource struct{}

var (
	_ resource.Resource                   = &talosMachineResource{}
	_ resource.ResourceWithModifyPlan     = &talosMachineResource{}
	_ resource.ResourceWithValidateConfig = &talosMachineResource{}
	_ resource.ResourceWithImportState    = &talosMachineResource{}
)

type talosMachineResourceModel struct {
	OnDestroy                    *onDestroyOptions     `tfsdk:"on_destroy"`
	MachineConfigurationWO       types.String          `tfsdk:"machine_configuration_wo"`
	Kubeconfig                   types.String          `tfsdk:"kubeconfig"`
	KubeconfigWO                 types.String          `tfsdk:"kubeconfig_wo"`
	Endpoint                     types.String          `tfsdk:"endpoint"`
	ClientConfiguration          basetypes.ObjectValue `tfsdk:"client_configuration"`
	ClientConfigurationWO        basetypes.ObjectValue `tfsdk:"client_configuration_wo"`
	MachineConfiguration         types.String          `tfsdk:"machine_configuration"`
	ID                           types.String          `tfsdk:"id"`
	Image                        types.String          `tfsdk:"image"`
	MachineConfigurationHash     types.String          `tfsdk:"machine_configuration_hash"`
	RebootMode                   types.String          `tfsdk:"reboot_mode"`
	Timeouts                     timeouts.Value        `tfsdk:"timeouts"`
	Node                         types.String          `tfsdk:"node"`
	DrainOnUpgrade               types.Bool            `tfsdk:"drain_on_upgrade"`
	IgnoreKubernetesUpgradeDrift types.Bool            `tfsdk:"ignore_kubernetes_upgrade_drift"`
}

// NewTalosMachineResource implements the resource.Resource interface.
func NewTalosMachineResource() resource.Resource {
	return &talosMachineResource{}
}

func (r *talosMachineResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine"
}

func (r *talosMachineResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Talos node: applies machine configuration and keeps the Talos OS version in sync.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "The IP address or hostname of the Talos node.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
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
			"machine_configuration": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The machine configuration YAML to apply. Use machine_configuration_wo when using ephemeral resources.",
			},
			"machine_configuration_wo": schema.StringAttribute{
				Optional:    true,
				WriteOnly:   true,
				Description: "Write-only variant of machine_configuration for use with ephemeral resources. Requires Terraform 1.11+.",
			},
			"image": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Talos installer image (e.g. `ghcr.io/siderolabs/installer:v1.9.0`). When set, upgrades if running version differs. When omitted, OS version is not managed.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"machine_configuration_hash": schema.StringAttribute{
				Computed:    true,
				Description: "SHA256 hex digest of the machine configuration currently applied on the node. Changes when configuration drifts, triggering a re-apply on the next `terraform apply`.",
			},
			"reboot_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Default:  stringdefault.StaticString("DEFAULT"),
				Validators: []validator.String{
					stringvalidator.OneOf("DEFAULT", "POWERCYCLE"),
				},
				Description: "Reboot mode for OS upgrades: DEFAULT or POWERCYCLE.",
			},
			"drain_on_upgrade": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Drain the node before rebooting during an upgrade, then uncordon after. Requires a healthy Kubernetes cluster. Use depends_on to sequence upgrades across nodes.",
			},
			"ignore_kubernetes_upgrade_drift": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				Description: "Experimental: when true, talos_machine ignores Kubernetes component image " +
					"tag changes owned by talos_cluster/upgrade-k8s, preventing drift detection from " +
					"interfering with graceful Kubernetes upgrades. Safe to use — enabling or disabling " +
					"causes at most a one-time apply to refresh the config hash. Cannot be guaranteed " +
					"to work with all future Talos versions: if upgrade-k8s manages additional image " +
					"fields in a future release, this attribute must be updated to match.",
			},
			"kubeconfig": schema.StringAttribute{
				Optional:  true,
				Sensitive: true,
				Description: "Kubeconfig used to drain and uncordon the node during upgrades. " +
					"Required when drain_on_upgrade = true and image is set. " +
					"Provide talos_cluster_kubeconfig.this.kubeconfig_raw. " +
					"Use kubeconfig_wo when using ephemeral resources.",
			},
			"kubeconfig_wo": schema.StringAttribute{
				Optional:    true,
				WriteOnly:   true,
				Sensitive:   true,
				Description: "Write-only variant of kubeconfig. Requires Terraform 1.11+.",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
			"on_destroy": schema.SingleNestedAttribute{
				Description:         "Actions to be taken on destroy, if `reset` is not set this is a no-op.",
				MarkdownDescription: onDestroyMarkDownDescription,
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"reset": schema.BoolAttribute{
						Description: "Reset the machine to the initial state (STATE and EPHEMERAL will be wiped).",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
					"graceful": schema.BoolAttribute{
						Description: "Graceful indicates whether node should leave etcd before the reset.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"reboot": schema.BoolAttribute{
						Description: "Reboot indicates whether node should reboot or halt after resetting.",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
				},
			},
		},
	}
}

func (r *talosMachineResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg talosMachineResourceModel

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

	cfgSet := !cfg.MachineConfiguration.IsNull()
	cfgWOSet := !cfg.MachineConfigurationWO.IsNull()

	if !cfgSet && !cfgWOSet {
		resp.Diagnostics.AddError(
			"Missing machine configuration",
			"Exactly one of machine_configuration or machine_configuration_wo must be set.",
		)
	}

	if cfgSet && cfgWOSet {
		resp.Diagnostics.AddError(
			"Conflicting machine configuration",
			"Only one of machine_configuration or machine_configuration_wo can be set, not both.",
		)
	}

	if !cfg.Kubeconfig.IsNull() && !cfg.KubeconfigWO.IsNull() {
		resp.Diagnostics.AddError(
			"Conflicting kubeconfig",
			"Only one of kubeconfig or kubeconfig_wo can be set, not both.",
		)
	}

	// drain only runs during OS upgrades (when image is managed), so only require
	// kubeconfig when image is also set. drain_on_upgrade defaults to true, so treat
	// null the same as true. Skip unknown — can't evaluate references at validate time.
	imageManaged := !cfg.Image.IsNull() && !cfg.Image.IsUnknown()
	drainEnabled := cfg.DrainOnUpgrade.IsNull() || (!cfg.DrainOnUpgrade.IsUnknown() && cfg.DrainOnUpgrade.ValueBool())

	if imageManaged && drainEnabled && kubeconfigMissing(&cfg) {
		resp.Diagnostics.AddError(
			"Missing kubeconfig for drain",
			"drain_on_upgrade = true requires kubeconfig or kubeconfig_wo when image is set. "+
				"Provide ephemeral.talos_cluster_kubeconfig.this.kubeconfig_raw via kubeconfig_wo.",
		)
	}
}

// kubeconfigMissing reports whether neither kubeconfig nor kubeconfig_wo carries a
// usable value. Empty strings are treated the same as null — they would fail to parse
// at runtime and produce a less clear error than the ValidateConfig message.
func kubeconfigMissing(cfg *talosMachineResourceModel) bool {
	// unknown = reference not yet resolved (e.g. ephemeral resource); skip validation.
	absent := func(s types.String) bool {
		return !s.IsUnknown() && (s.IsNull() || strings.TrimSpace(s.ValueString()) == "")
	}

	return absent(cfg.Kubeconfig) && absent(cfg.KubeconfigWO)
}

func (r *talosMachineResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan talosMachineResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Read config to distinguish "endpoint omitted" (null) from "endpoint unknown reference".
	// Write-only attrs are also only available in Config, not Plan.
	var cfgFromConfig talosMachineResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfgFromConfig)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Default endpoint to node only when the user didn't provide endpoint in config at all.
	// When endpoint is an unknown reference (e.g. libvirt IP not yet known), cfgFromConfig.Endpoint
	// is unknown (not null), so we skip defaulting and leave the plan value as-is.
	if cfgFromConfig.Endpoint.IsNull() && !plan.Node.IsUnknown() && !plan.Node.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("endpoint"), plan.Node)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	if !cfgFromConfig.MachineConfigurationWO.IsNull() {
		plan.MachineConfigurationWO = cfgFromConfig.MachineConfigurationWO
	}

	cfgBytes := resolveMachineConfigBytesFromModel(&plan)
	if len(cfgBytes) == 0 {
		// Input is unknown or absent — mark hash unknown so Terraform expects a change.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration_hash"), types.StringUnknown())...)

		return
	}

	desiredHash, stripped := computeConfigHash(cfgBytes, plan.IgnoreKubernetesUpgradeDrift.ValueBool())
	if !stripped {
		tflog.Warn(ctx, "computeConfigHash: failed to normalize config; hash covers raw config bytes")
	}

	var state talosMachineResourceModel

	if !req.State.Raw.IsNull() {
		resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

		if resp.Diagnostics.HasError() {
			return
		}
	}

	if state.MachineConfigurationHash.ValueString() != desiredHash {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration_hash"), types.StringUnknown())...)
	} else {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration_hash"), state.MachineConfigurationHash)...)
	}
}

func (r *talosMachineResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan talosMachineResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Write-only attrs live only in Config.
	var cfgModel talosMachineResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfgModel)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !cfgModel.ClientConfigurationWO.IsNull() {
		plan.ClientConfigurationWO = cfgModel.ClientConfigurationWO
	}

	if !cfgModel.MachineConfigurationWO.IsNull() {
		plan.MachineConfigurationWO = cfgModel.MachineConfigurationWO
	}

	if !cfgModel.KubeconfigWO.IsNull() {
		plan.KubeconfigWO = cfgModel.KubeconfigWO
	}

	talosConfig, resolvedClientConfig, err := resolveTalosMachineClientConfig(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config", err.Error())

		return
	}

	// Only persist client_configuration when the non-write-only variant was used.
	// When client_configuration_wo is used the planned value is null; setting it here
	// would produce an "inconsistent values for sensitive attribute" error from Terraform.
	if cfgModel.ClientConfigurationWO.IsNull() {
		plan.ClientConfiguration = resolvedClientConfig
	}

	timeout, diags := plan.Timeouts.Create(ctx, DefaultCreateTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cfgBytes := resolveMachineConfigBytesFromModel(&plan)
	if len(cfgBytes) == 0 {
		resp.Diagnostics.AddError("missing machine configuration", "machine_configuration or machine_configuration_wo must be provided")

		return
	}

	endpoint := talosMachineEffectiveEndpoint(&plan)

	if err := talosMachineApplyConfig(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig, cfgBytes); err != nil {
		resp.Diagnostics.AddError("error applying machine configuration", err.Error())

		return
	}

	cfgHash, stripped := computeConfigHash(cfgBytes, plan.IgnoreKubernetesUpgradeDrift.ValueBool())
	if !stripped {
		tflog.Warn(ctx, "computeConfigHash: failed to normalize config; hash covers raw config bytes")
	}

	plan.MachineConfigurationHash = types.StringValue(cfgHash)

	if !plan.Image.IsNull() {
		if err := talosMachineUpgradeIfNeeded(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig, &plan); err != nil {
			resp.Diagnostics.AddError("error upgrading Talos", err.Error())

			return
		}
	}

	plan.ID = types.StringValue(plan.Node.ValueString())
	plan.Endpoint = types.StringValue(endpoint)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *talosMachineResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state talosMachineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Write-only credentials are not persisted to state. Skip the live refresh
	// rather than failing — drift detection is unavailable in this mode.
	if state.ClientConfiguration.IsNull() {
		return
	}

	talosConfig, _, err := resolveTalosMachineClientConfig(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config from state", err.Error())

		return
	}

	endpoint := talosMachineEffectiveEndpoint(&state)

	var runningImage string

	if err := talosClientOp(ctx, endpoint, state.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		versionResp, err := c.Version(nodeCtx)
		if err != nil {
			return err
		}

		if len(versionResp.Messages) > 0 {
			base := state.Image.ValueString()
			if base == "" {
				base = images.InstallerImageRepository("metal")
			}

			runningImage = replaceImageTag(base, versionResp.Messages[0].Version.Tag)
		}

		return nil
	}); err != nil {
		// Node unreachable — let Terraform re-create.
		resp.State.RemoveResource(ctx)

		return
	}

	state.Image = types.StringValue(runningImage)

	// Fetch the applied config hash from COSI to detect out-of-band drift.
	// Non-fatal: leave hash stale if COSI is unavailable.
	//
	// Note: the hash format changed from raw sha256(bytes) to
	// NormalizedConfigHash (sha256 of YAML-normalized config) in
	// v0.12.0-alpha.5. Existing state from earlier alphas holds the old format.
	// On the first plan after upgrade the hashes will not match, triggering a
	// one-time config re-apply. The re-apply is safe: the config is unchanged
	// and Talos will not reboot for a no-op. No StateUpgraders migration is
	// provided since only alpha versions are affected.
	if cfgHash, err := talosMachineLiveConfigHash(
		ctx,
		endpoint,
		state.Node.ValueString(),
		talosConfig,
		state.IgnoreKubernetesUpgradeDrift.ValueBool(),
	); err == nil {
		state.MachineConfigurationHash = types.StringValue(cfgHash)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *talosMachineResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state talosMachineResourceModel

	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Write-only attrs live only in Config.
	var cfgModel talosMachineResourceModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &cfgModel)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if !cfgModel.ClientConfigurationWO.IsNull() {
		plan.ClientConfigurationWO = cfgModel.ClientConfigurationWO
	}

	if !cfgModel.MachineConfigurationWO.IsNull() {
		plan.MachineConfigurationWO = cfgModel.MachineConfigurationWO
	}

	if !cfgModel.KubeconfigWO.IsNull() {
		plan.KubeconfigWO = cfgModel.KubeconfigWO
	}

	talosConfig, resolvedClientConfig, err := resolveTalosMachineClientConfig(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config", err.Error())

		return
	}

	if cfgModel.ClientConfigurationWO.IsNull() {
		plan.ClientConfiguration = resolvedClientConfig
	}

	timeout, diags := plan.Timeouts.Update(ctx, DefaultUpdateTimeout)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	endpoint := talosMachineEffectiveEndpoint(&plan)
	plan.Endpoint = types.StringValue(endpoint)

	// Upgrade OS first so the new config is accepted by the upgraded node.
	// Post-import state has a null image (identity-only ImportState). Use
	// UpgradeIfNeeded only then so filling the same running tag into state does
	// not pull/install/drain/reboot. For every other image change (including
	// same tag / different schematic), always upgrade — UpgradeIfNeeded is
	// tag-only via replaceImageTag and would skip schematic updates (b812b76).
	imageChanged := !plan.Image.IsNull() && !plan.Image.Equal(state.Image)
	postImportAdopt := state.Image.IsNull()

	if imageChanged {
		if err := talosMachineUpgradeOnUpdate(
			ctxDeadline,
			endpoint,
			plan.Node.ValueString(),
			talosConfig,
			&plan,
			postImportAdopt,
		); err != nil {
			resp.Diagnostics.AddError("error upgrading Talos", err.Error())

			return
		}
	}

	// machine_configuration_hash is Unknown when ModifyPlan detected a change.
	configChanged := plan.MachineConfigurationHash.IsUnknown()

	if configChanged || imageChanged {
		cfgBytes := resolveMachineConfigBytesFromModel(&plan)
		if len(cfgBytes) == 0 {
			resp.Diagnostics.AddError("missing machine configuration", "machine_configuration or machine_configuration_wo must be provided")

			return
		}

		// Skip config apply if the normalized hash is unchanged. This covers:
		// (1) write-only machine_configuration_wo, where ModifyPlan cannot read
		// the config and always marks hash Unknown; (2) a simultaneous Talos OS
		// upgrade where kubernetes_version also changed — with
		// ignore_kubernetes_upgrade_drift=true, K8s images are excluded from the
		// hash and must not be re-applied here; (3) post-import adopt, where
		// state hash is empty — compare against the live COSI config so filling
		// HCL into state does not re-apply an identical machine config.
		configHash, stripped := computeConfigHash(cfgBytes, plan.IgnoreKubernetesUpgradeDrift.ValueBool())
		if !stripped {
			tflog.Warn(ctx, "computeConfigHash: failed to normalize config; hash covers raw config bytes")
		}

		baselineHash, baselineErr := talosMachineConfigBaselineHash(
			ctxDeadline,
			endpoint,
			plan.Node.ValueString(),
			talosConfig,
			state.MachineConfigurationHash.ValueString(),
			plan.IgnoreKubernetesUpgradeDrift.ValueBool(),
			postImportAdopt,
		)
		if baselineErr != nil {
			resp.Diagnostics.AddError(
				"error reading live machine configuration for adopt/update",
				baselineErr.Error(),
			)

			return
		}

		if configHash == baselineHash {
			plan.MachineConfigurationHash = types.StringValue(configHash)
			resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)

			return
		}

		if err := talosMachineApplyConfig(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig, cfgBytes); err != nil {
			resp.Diagnostics.AddError("error applying machine configuration", err.Error())

			return
		}

		plan.MachineConfigurationHash = types.StringValue(configHash)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *talosMachineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state talosMachineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.OnDestroy == nil || !state.OnDestroy.Reset.ValueBool() {
		return
	}

	// During Delete, write-only attrs are not in state; client_configuration (non-wo) is required.
	talosConfig, _, err := resolveTalosMachineClientConfig(ctx, &state)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config for destroy", err.Error())

		return
	}

	endpoint := talosMachineEffectiveEndpoint(&state)

	deleteTimeout, diags := state.Timeouts.Delete(ctx, 5*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	resetRequest := &machineapi.ResetRequest{
		Graceful: state.OnDestroy.Graceful.ValueBool(),
		Reboot:   state.OnDestroy.Reboot.ValueBool(),
		SystemPartitionsToWipe: []*machineapi.ResetPartitionSpec{
			{Label: "STATE", Wipe: true},
			{Label: "EPHEMERAL", Wipe: true},
		},
	}

	actionFn := func(ctx context.Context, c *client.Client) (string, error) {
		return resetGetActorID(ctx, c, resetRequest)
	}

	if err := action.NewTracker(
		newTalosClientFactory(talosConfig, endpoint, []string{state.Node.ValueString()}),
		action.StopAllServicesEventFn,
		actionFn,
		action.WithDebug(false),
		action.WithTimeout(deleteTimeout),
	).Run(ctx); err != nil {
		resp.Diagnostics.AddError("error resetting machine", err.Error())
	}
}

func (r *talosMachineResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	node := strings.TrimSpace(req.ID)
	if node == "" {
		resp.Diagnostics.AddError(
			"failed to import state",
			"import ID must be the Talos node IP address or hostname",
		)

		return
	}

	timeout, diag := basetypes.NewObjectValue(map[string]attr.Type{
		"create": types.StringType,
		"update": types.StringType,
		"delete": types.StringType,
	}, map[string]attr.Value{
		"create": basetypes.NewStringNull(),
		"update": basetypes.NewStringNull(),
		"delete": basetypes.NewStringNull(),
	})
	resp.Diagnostics.Append(diag...)

	if resp.Diagnostics.HasError() {
		return
	}

	// Import only records identity. Read cannot see HCL client_configuration, so
	// image / machine_configuration_hash are filled on the first Update by
	// observing the live node (UpgradeIfNeeded + live COSI hash compare) before
	// any apply/upgrade. Do not treat a post-import plan "update in-place" as a
	// license to mutate the node when HCL already matches reality.
	clientConfig := basetypes.NewObjectNull(map[string]attr.Type{
		"ca_certificate":     types.StringType,
		"client_certificate": types.StringType,
		"client_key":         types.StringType,
	})

	state := talosMachineResourceModel{
		ID:                    basetypes.NewStringValue(node),
		Node:                  basetypes.NewStringValue(node),
		Endpoint:              basetypes.NewStringValue(node),
		ClientConfiguration:   clientConfig,
		ClientConfigurationWO: clientConfig, // write-only: always null in state; typed null required by ObjectType
		Timeouts: timeouts.Value{
			Object: timeout,
		},
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// talosMachineApplyConfig applies the machine configuration with retry and waits for
// the node to be reachable afterwards (it reboots on first config apply).
func talosMachineApplyConfig(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, cfgBytes []byte) error {
	if err := retry.RetryContext(ctx, 10*time.Minute, func() *retry.RetryError {
		if err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			_, err := c.ApplyConfiguration(nodeCtx, &machineapi.ApplyConfigurationRequest{
				Mode: machineapi.ApplyConfigurationRequest_AUTO,
				Data: cfgBytes,
			})

			return err
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("applying configuration: %w", err)
	}

	// Poll until node is back up — it may have rebooted after first config apply.
	return retry.RetryContext(ctx, 10*time.Minute, func() *retry.RetryError {
		if err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			_, err := c.Version(nodeCtx)

			return err
		}); err != nil {
			return retry.RetryableError(err)
		}

		return nil
	})
}

// talosMachineUpgrade upgrades the Talos OS to the desired installer image
// by performing: pull → install → drain → reboot → uncordon.
func talosMachineUpgrade(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel) (retErr error) {
	rebootModeStr := strings.ToUpper(state.RebootMode.ValueString())

	containerdInst := &commonapi.ContainerdInstance{
		Driver:    commonapi.ContainerDriver_CRI,
		Namespace: commonapi.ContainerdNamespace_NS_SYSTEM,
	}

	// Pull the installer image into containerd before upgrading.
	// LifecycleService.Upgrade requires the image to already be present in the containerd store.
	// codes.Unimplemented here means the node is Talos < v1.13 — fall back to legacy upgrade.
	pullErr := talosMachinePullImage(ctx, endpoint, node, talosConfig, state.Image.ValueString(), containerdInst)
	if pullErr != nil {
		if st, _ := status.FromError(pullErr); st.Code() == codes.Unimplemented {
			return talosMachineUpgradeLegacy(ctx, endpoint, node, talosConfig, state, rebootModeStr, talosClientOp)
		}

		return fmt.Errorf("pulling installer image: %w", pullErr)
	}

	// LifecycleService (Talos v1.13+): installs without rebooting, then reboot separately.
	if installErr := talosMachineInstallImage(ctx, endpoint, node, talosConfig, state.Image.ValueString(), containerdInst); installErr != nil {
		return fmt.Errorf("installing new OS image: %w", installErr)
	}

	rawKubeconfig := state.KubeconfigWO.ValueString()
	if rawKubeconfig == "" {
		rawKubeconfig = state.Kubeconfig.ValueString()
	}

	k8sNodeName, err := talosMachineCordonAndDrain(ctx, endpoint, node, talosConfig, state.DrainOnUpgrade.ValueBool(), rawKubeconfig)
	if err != nil {
		return fmt.Errorf("draining node: %w", err)
	}

	// Uncordon in defer so the node is never left cordoned, even if the reboot fails.
	defer func() {
		if k8sNodeName == "" {
			return
		}

		if err := talosMachineUncordon(ctx, k8sNodeName, rawKubeconfig); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("uncordoning node: %w", err))
		}
	}()

	if err := talosMachineReboot(ctx, endpoint, node, talosConfig, rebootModeStr); err != nil {
		return fmt.Errorf("waiting for node after reboot: %w", err)
	}

	return nil
}

// talosMachineUpgradeIfNeeded checks the running Talos version and, if it differs from
// the desired image, performs: pull → install → drain → reboot → uncordon.
func talosMachineUpgradeIfNeeded(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel) (retErr error) {
	runningImage, err := talosMachineRunningVersion(ctx, endpoint, node, talosConfig, state.Image.ValueString())
	if err != nil {
		return fmt.Errorf("reading running version: %w", err)
	}

	if runningImage == state.Image.ValueString() {
		return nil
	}

	return talosMachineUpgrade(ctx, endpoint, node, talosConfig, state)
}

// talosMachineUpgradeOnUpdate chooses UpgradeIfNeeded only for post-import adopt
// (null prior image). All other Update image changes use unconditional Upgrade
// so same-tag schematic changes are not skipped.
func talosMachineUpgradeOnUpdate(
	ctx context.Context,
	endpoint, node string,
	talosConfig *clientconfig.Config,
	plan *talosMachineResourceModel,
	postImportAdopt bool,
) error {
	if postImportAdopt {
		return talosMachineUpgradeIfNeeded(ctx, endpoint, node, talosConfig, plan)
	}

	return talosMachineUpgrade(ctx, endpoint, node, talosConfig, plan)
}

func talosMachineRunningVersion(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, desiredImage string) (string, error) {
	var runningImage string

	// Retry: after config apply the node may still be rebooting, so we wait for it to come up.
	if err := retry.RetryContext(ctx, 10*time.Minute, func() *retry.RetryError {
		err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			versionResp, err := c.Version(nodeCtx)
			if err != nil {
				return err
			}

			for _, msg := range versionResp.Messages {
				runningImage = replaceImageTag(desiredImage, msg.Version.Tag)

				break
			}

			return nil
		})
		if err != nil {
			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		return "", err
	}

	return runningImage, nil
}

func talosMachinePullImage(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, imageRef string, containerdInst *commonapi.ContainerdInstance) error {
	return talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		stream, err := c.ImageClient.Pull(nodeCtx, &machineapi.ImageServicePullRequest{
			Containerd: containerdInst,
			ImageRef:   imageRef,
		})
		if err != nil {
			return err
		}

		for {
			_, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return err
			}
		}

		return nil
	})
}

func talosMachineInstallImage(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, imageRef string, containerdInst *commonapi.ContainerdInstance) error {
	return talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		stream, err := c.LifecycleClient.Upgrade(nodeCtx, &machineapi.LifecycleServiceUpgradeRequest{
			Containerd: containerdInst,
			Source: &machineapi.InstallArtifactsSource{
				ImageName: imageRef,
			},
		})
		if err != nil {
			return err
		}

		for {
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}

			if err != nil {
				return err
			}

			if ec := resp.GetProgress().GetExitCode(); ec != 0 {
				return fmt.Errorf("upgrade exited with code %d", ec)
			}
		}

		return nil
	})
}

func talosMachineCordonAndDrain(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, drain bool, rawKubeconfig string) (string, error) {
	if !drain {
		return "", nil
	}

	cs, err := kubeclientFromRaw([]byte(rawKubeconfig))
	if err != nil {
		return "", fmt.Errorf("building k8s client for drain: %w", err)
	}

	var k8sNodeName string

	if err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		name, err := nodedrain.GetKubernetesNodeName(nodeCtx, c)
		if err != nil {
			return fmt.Errorf("resolving k8s node name: %w", err)
		}

		k8sNodeName = name

		return nil
	}); err != nil {
		return "", err
	}

	noopReport := func(talosreporter.Update) {}

	return k8sNodeName, nodedrain.CordonAndDrain(ctx, cs, k8sNodeName, nodedrain.Options{}, noopReport)
}

func talosMachineUncordon(ctx context.Context, k8sNodeName, rawKubeconfig string) error {
	cs, err := kubeclientFromRaw([]byte(rawKubeconfig))
	if err != nil {
		return fmt.Errorf("building k8s client for uncordon: %w", err)
	}

	noopReport := func(talosreporter.Update) {}

	if waitErr := nodedrain.WaitForNodeReady(ctx, cs, k8sNodeName, 5*time.Minute); waitErr != nil {
		return fmt.Errorf("waiting for node ready: %w", waitErr)
	}

	return nodedrain.Uncordon(ctx, cs, k8sNodeName, noopReport)
}

func kubeclientFromRaw(kubeconfigBytes []byte) (kubernetes.Interface, error) {
	config, err := clientcmd.NewClientConfigFromBytes(kubeconfigBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing kubeconfig: %w", err)
	}

	restConfig, err := config.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building REST config: %w", err)
	}

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating k8s clientset: %w", err)
	}

	return cs, nil
}

func talosMachineReboot(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, rebootModeStr string) error {
	rebootModeVal, ok := machineapi.RebootRequest_Mode_value[rebootModeStr]
	if !ok {
		rebootModeVal = int32(machineapi.RebootRequest_DEFAULT)
	}

	rebootMode := machineapi.RebootRequest_Mode(rebootModeVal)

	return action.NewTracker(
		newTalosClientFactory(talosConfig, endpoint, []string{node}),
		action.MachineReadyEventFn,
		func(rebootCtx context.Context, c *client.Client) (string, error) {
			resp, err := c.RebootWithResponse(rebootCtx, client.WithRebootMode(rebootMode))
			if err != nil {
				return "", err
			}

			if len(resp.GetMessages()) == 0 {
				return "", errors.New("no messages returned from reboot")
			}

			return resp.GetMessages()[0].GetActorId(), nil
		},
		action.WithPostCheck(action.BootIDChangedPostCheckFn),
		action.WithTimeout(15*time.Minute),
	).Run(ctx)
}

// talosMachineUpgradeLegacy handles Talos < 1.13 nodes where LifecycleService is not available.
// MachineService.Upgrade combines install + reboot atomically. drain_on_upgrade is not applied
// here — talosctl upgrade does not drain on the legacy path either.
func talosMachineUpgradeLegacy(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel, rebootModeStr string, op clientOpFunc) error {
	upgradeRebootModeVal, ok := machineapi.UpgradeRequest_RebootMode_value[rebootModeStr]
	if !ok {
		upgradeRebootModeVal = int32(machineapi.UpgradeRequest_DEFAULT)
	}

	upgradeRebootMode := machineapi.UpgradeRequest_RebootMode(upgradeRebootModeVal)

	// On Talos < v1.13, UpgradeWithOptions holds the gRPC connection open for the entire
	// download+install duration (~30–45 min). The action.Tracker pattern can't be used here
	// because the action function doesn't return until done, causing RST_STREAM timeouts.
	// Instead, fire the RPC in a goroutine and independently poll for the version change.
	// The channel carries the first non-nil error so the poll loop can abort early if the
	// RPC fails before the node even reboots (bad image ref, auth failure, etc.).
	rpcErrCh := make(chan error, 1)

	go func() {
		err := op(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			opts := []client.UpgradeOption{
				client.WithUpgradeImage(state.Image.ValueString()),
				client.WithUpgradeRebootMode(upgradeRebootMode),
			}

			_, err := c.UpgradeWithOptions(nodeCtx, opts...) //nolint:staticcheck

			return err
		})
		if err != nil {
			rpcErrCh <- err
		}
	}()

	if err := retry.RetryContext(ctx, 60*time.Minute, func() *retry.RetryError {
		// Abort early if the upgrade RPC itself failed (e.g. bad image, auth error).
		select {
		case rpcErr := <-rpcErrCh:
			return retry.NonRetryableError(fmt.Errorf("upgrade RPC failed: %w", rpcErr))
		default:
		}

		err := op(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			versionResp, err := c.Version(nodeCtx)
			if err != nil {
				return err
			}

			if len(versionResp.Messages) == 0 {
				return fmt.Errorf("no version messages from node")
			}

			running := replaceImageTag(state.Image.ValueString(), versionResp.Messages[0].Version.Tag)
			if running == state.Image.ValueString() {
				return nil
			}

			return fmt.Errorf("node running %s, waiting for %s", running, state.Image.ValueString())
		})
		if err != nil {
			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("waiting for node after upgrade: %w", err)
	}

	return nil
}

// replaceImageTag replaces the tag portion of an image reference.
// "ghcr.io/siderolabs/installer:v1.8.0" + "v1.9.0" → "ghcr.io/siderolabs/installer:v1.9.0".
func replaceImageTag(imageRef, newTag string) string {
	if idx := strings.LastIndex(imageRef, ":"); idx != -1 {
		return imageRef[:idx+1] + newTag
	}

	return imageRef + ":" + newTag
}

// talosMachineLiveConfigHash reads the active machine config from COSI and returns
// the same normalized hash ModifyPlan / Update use for drift detection.
func talosMachineLiveConfigHash(
	ctx context.Context,
	endpoint, node string,
	talosConfig *clientconfig.Config,
	suppressK8sDrift bool,
) (string, error) {
	var cfgHash string

	if err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		cfg, err := safe.StateGet[*configresource.MachineConfig](
			nodeCtx,
			c.COSI,
			cosiresource.NewMetadata(
				configresource.NamespaceName,
				configresource.MachineConfigType,
				configresource.ActiveID,
				cosiresource.VersionUndefined,
			),
		)
		if err != nil {
			return err
		}

		yamlBytes, err := cfg.Provider().Bytes()
		if err != nil {
			return err
		}

		// Both Read (COSI bytes) and ModifyPlan (provider-rendered bytes) go
		// through computeConfigHash's yaml.Marshal normalization, so key
		// ordering is consistent on both sides. The two byte sources are
		// semantically identical as long as Talos does not inject extra default
		// fields when serializing — which is the case for configs written by
		// this provider.
		hash, stripped := computeConfigHash(yamlBytes, suppressK8sDrift)
		if !stripped {
			tflog.Warn(nodeCtx, "computeConfigHash: failed to normalize config; hash covers raw config bytes")
		}

		cfgHash = hash

		return nil
	}); err != nil {
		return "", err
	}

	return cfgHash, nil
}

// talosMachineConfigBaselineHash returns the hash Update should compare against.
// Post-import adopt (empty state hash + null prior image) requires a live COSI
// read and errors if unavailable. Other empty-hash cases keep prior behavior:
// empty baseline so desired config is applied.
func talosMachineConfigBaselineHash(
	ctx context.Context,
	endpoint, node string,
	talosConfig *clientconfig.Config,
	stateHash string,
	suppressK8sDrift bool,
	postImportAdopt bool,
) (string, error) {
	if stateHash != "" || !postImportAdopt {
		return stateHash, nil
	}

	liveHash, err := talosMachineLiveConfigHash(ctx, endpoint, node, talosConfig, suppressK8sDrift)
	if err != nil {
		return "", fmt.Errorf("state has no machine_configuration_hash; refused to apply without comparing to the node: %w", err)
	}

	return liveHash, nil
}

// resolveTalosMachineClientConfig builds the Talos client config from either the
// write-only or regular client_configuration attribute. It also returns the resolved
// ObjectValue so callers can persist it in state.ClientConfiguration for Read().
func resolveTalosMachineClientConfig(ctx context.Context, state *talosMachineResourceModel) (*clientconfig.Config, basetypes.ObjectValue, error) {
	var clientObj basetypes.ObjectValue

	switch {
	case !state.ClientConfigurationWO.IsNull() && !state.ClientConfigurationWO.IsUnknown():
		clientObj = state.ClientConfigurationWO
	case !state.ClientConfiguration.IsNull():
		clientObj = state.ClientConfiguration
	default:
		return nil, basetypes.ObjectValue{}, errors.New("no client configuration available")
	}

	ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, clientObj)
	if !ok {
		return nil, basetypes.ObjectValue{}, errors.New(errMsg)
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig("dynamic", ca, cert, key)
	if err != nil {
		return nil, basetypes.ObjectValue{}, err
	}

	return talosConfig, clientObj, nil
}

// resolveMachineConfigBytesFromModel returns the raw YAML bytes from machine_configuration_wo
// (preferred) or machine_configuration.
func resolveMachineConfigBytesFromModel(state *talosMachineResourceModel) []byte {
	if !state.MachineConfigurationWO.IsNull() && !state.MachineConfigurationWO.IsUnknown() {
		return []byte(state.MachineConfigurationWO.ValueString())
	}

	if !state.MachineConfiguration.IsNull() && !state.MachineConfiguration.IsUnknown() {
		return []byte(state.MachineConfiguration.ValueString())
	}

	return nil
}

// talosMachineEffectiveEndpoint returns the endpoint, defaulting to node.
func talosMachineEffectiveEndpoint(state *talosMachineResourceModel) string {
	if !state.Endpoint.IsNull() && !state.Endpoint.IsUnknown() && state.Endpoint.ValueString() != "" {
		return state.Endpoint.ValueString()
	}

	return state.Node.ValueString()
}

// computeConfigHash returns the drift-detection hash for cfgBytes.
// When suppressK8sDrift is true (ignore_kubernetes_upgrade_drift), the
// upgrade-k8s-managed image tags are stripped before hashing so that a
// kubernetes_version bump does not trigger a machine-config re-apply.
func computeConfigHash(cfgBytes []byte, suppressK8sDrift bool) (string, bool) {
	if suppressK8sDrift {
		return K8sManagedConfigHash(cfgBytes)
	}

	return NormalizedConfigHash(cfgBytes)
}
