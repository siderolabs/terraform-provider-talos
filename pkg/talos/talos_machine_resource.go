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
	cosistate "github.com/cosi-project/runtime/pkg/state"
	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
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
	runtimeres "github.com/siderolabs/talos/pkg/machinery/resources/runtime"
	serviceres "github.com/siderolabs/talos/pkg/machinery/resources/v1alpha1"
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
	DefaultCreateTimeout = 40 * time.Minute // 10m apply + 10m wait-for-node + 15m create reboot + margin
	DefaultUpdateTimeout = 90 * time.Minute // above + 60m legacy upgrade poll + margin
)

// ReadLivenessTimeout bounds how long Read() retries the liveness probe before
// giving up on a node. It only needs to absorb an ordinary reboot window
// (netboot, kernel upgrade, a brief network partition), not a full boot cycle —
// exported so tests can assert against it without duplicating the value.
const ReadLivenessTimeout = 2 * time.Minute

// clientOpFunc matches the signature of talosClientOp and lets unit tests inject
// a mock into talosMachineUpgradeLegacy without touching any pre-existing file.
type clientOpFunc func(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, fn func(nodeCtx context.Context, c *client.Client) error) error

type talosMachineResource struct{}

var (
	_ resource.Resource                   = &talosMachineResource{}
	_ resource.ResourceWithModifyPlan     = &talosMachineResource{}
	_ resource.ResourceWithValidateConfig = &talosMachineResource{}
)

type talosMachineResourceModel struct {
	UpgradePolicy                types.Object          `tfsdk:"upgrade_policy"`
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
			"upgrade_policy": upgradePolicySchema(),
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

	if _, _, err := decodeUpgradePolicy(ctx, cfg.UpgradePolicy); err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("upgrade_policy"), "Invalid upgrade policy", err.Error())
	}

	if !cfg.UpgradePolicy.IsNull() && !cfg.DrainOnUpgrade.IsUnknown() && !cfg.DrainOnUpgrade.IsNull() && !cfg.DrainOnUpgrade.ValueBool() {
		resp.Diagnostics.AddAttributeError(path.Root("drain_on_upgrade"), "Upgrade policy requires draining", "Set drain_on_upgrade = true when using upgrade_policy.")
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

	versionResp, err := talosMachineWaitReachable(ctx, endpoint, state.Node.ValueString(), talosConfig, talosClientOp)
	if err != nil {
		if errors.Is(err, errInvalidClientCredentials) {
			// The stored client_configuration cannot be parsed into a certificate — no
			// amount of retrying the node fixes that. Fail loudly rather than spending
			// the whole ReadLivenessTimeout window retrying a guaranteed-fail dial and
			// then reporting it as if the node were merely unreachable.
			resp.Diagnostics.AddError("invalid talos_machine client credentials", err.Error())

			return
		}

		// Unreachable for the whole retry window: could be a node mid-reboot (netboot,
		// kernel upgrade, a network blip) or a genuinely decommissioned machine — a
		// Version RPC failure cannot tell those apart, so this is not a positive signal
		// the resource is gone. Leave state untouched rather than guessing, same reflex
		// as the COSI hash read below, which also treats its own failure as non-fatal.
		resp.Diagnostics.AddWarning(
			"could not refresh talos_machine",
			fmt.Sprintf(
				"The node did not become reachable within %s: %s. Leaving the prior state untouched. "+
					"If this machine was intentionally decommissioned, remove it from state manually "+
					"(terraform state rm) instead of letting the next apply recreate it.",
				ReadLivenessTimeout, err,
			),
		)

		return
	}

	if len(versionResp.Messages) > 0 {
		base := state.Image.ValueString()
		if base == "" {
			base = images.InstallerImageRepository("metal")
		}

		state.Image = types.StringValue(replaceImageTag(base, versionResp.Messages[0].Version.Tag))
	}

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
	_ = talosClientOp(ctx, endpoint, state.Node.ValueString(), talosConfig, func(nodeCtx context.Context, c *client.Client) error { //nolint:errcheck
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
		cfgHash, stripped := computeConfigHash(yamlBytes, state.IgnoreKubernetesUpgradeDrift.ValueBool())
		if !stripped {
			tflog.Warn(nodeCtx, "computeConfigHash: failed to normalize config; hash covers raw config bytes")
		}

		state.MachineConfigurationHash = types.StringValue(cfgHash)

		return nil
	})

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// errInvalidClientCredentials marks a client_configuration that cannot be parsed
// into a certificate. Unlike node unreachability, retrying never resolves this.
var errInvalidClientCredentials = errors.New("invalid client credentials")

// talosMachineWaitReachable retries a plain Version RPC for up to
// ReadLivenessTimeout, absorbing the kind of brief unreachability a normal
// reboot (netboot, kernel upgrade) causes. Past that point it does not classify
// errors — any failure is retried until the deadline, since there is no gRPC
// status that positively identifies "this machine no longer exists" as opposed
// to "this machine is temporarily unreachable".
//
// The one exception is checked once, up front: state.client_configuration
// (e.g. from a manual edit or a partial state write) that cannot be parsed into
// a certificate can never succeed no matter how long this retries, so it fails
// fast instead of behind a misleading ReadLivenessTimeout-long "unreachable".
func talosMachineWaitReachable(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, op clientOpFunc) (*machineapi.VersionResponse, error) {
	if cfgCtx, ok := talosConfig.Contexts[talosConfig.Context]; ok {
		if _, err := client.CertificateFromConfigContext(cfgCtx); err != nil {
			return nil, fmt.Errorf("%w: %w", errInvalidClientCredentials, err)
		}
	}

	var versionResp *machineapi.VersionResponse

	err := retry.RetryContext(ctx, ReadLivenessTimeout, func() *retry.RetryError {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := op(attemptCtx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
			resp, err := c.Version(nodeCtx)
			if err != nil {
				return err
			}

			versionResp = resp

			return nil
		}); err != nil {
			return retry.RetryableError(err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return versionResp, nil
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
	imageChanged := !plan.Image.IsNull() && !plan.Image.Equal(state.Image)

	if imageChanged {
		if err := talosMachineUpgradeWithBudget(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig, &plan); err != nil {
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

		// Skip config apply if the normalized hash is unchanged. This covers two
		// cases: (1) write-only machine_configuration_wo, where ModifyPlan cannot
		// read the config and always marks hash Unknown; and (2) a simultaneous
		// Talos OS upgrade where kubernetes_version also changed — the OS upgrade
		// already happened above; with ignore_kubernetes_upgrade_drift=true, K8s
		// images are excluded from the hash and must not be re-applied here, which
		// would bypass upgrade-k8s's sequential safety procedure.
		configHash, stripped := computeConfigHash(cfgBytes, plan.IgnoreKubernetesUpgradeDrift.ValueBool())
		if !stripped {
			tflog.Warn(ctx, "computeConfigHash: failed to normalize config; hash covers raw config bytes")
		}

		if configHash == state.MachineConfigurationHash.ValueString() {
			plan.MachineConfigurationHash = state.MachineConfigurationHash
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

// talosMachineApplyConfig applies the machine configuration with retry and waits for
// CRI to be ready during normal boot (first apply can install and reboot).
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

	// The API also responds during maintenance and early boot. Wait until the
	// node is out of maintenance and CRI is ready before reading extensions or pulling an installer.
	return talosMachineWaitForBoot(ctx, endpoint, node, talosConfig, talosClientOp)
}

func talosMachineWaitForBoot(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, op clientOpFunc) error {
	return retry.RetryContext(ctx, 10*time.Minute, func() *retry.RetryError {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if err := op(attemptCtx, endpoint, node, talosConfig, talosMachineCheckBootReadyOrVersion); err != nil {
			return talosMachineBootRetryError(err)
		}

		return nil
	})
}

func talosMachineBootRetryError(err error) *retry.RetryError {
	if code := status.Code(err); code == codes.PermissionDenied || code == codes.Unauthenticated || code == codes.InvalidArgument {
		return retry.NonRetryableError(err)
	}

	return retry.RetryableError(err)
}

// Fall back to Version when the role cannot read runtime resources.
func talosMachineCheckBootReadyOrVersion(ctx context.Context, c *client.Client) error {
	err := talosMachineCheckBootReady(ctx, c)
	if status.Code(err) != codes.PermissionDenied {
		return err
	}

	_, err = c.Version(ctx)

	return err
}

func talosMachineCheckBootReady(ctx context.Context, c *client.Client) error {
	machine, err := safe.StateGet[*runtimeres.MachineStatus](ctx, c.COSI, runtimeres.NewMachineStatus().Metadata())
	if err != nil {
		return fmt.Errorf("reading machine boot status: %w", err)
	}

	if stage := machine.TypedSpec().Stage; stage != runtimeres.MachineStageBooting && stage != runtimeres.MachineStageRunning {
		return fmt.Errorf("waiting for normal boot: stage is %s", stage)
	}

	// Talos 1.14 keeps this status at waiting-for-reboot until the first
	// install's reboot has actually happened. Older Talos has no such resource.
	install, err := safe.StateGet[*runtimeres.UnattendedInstallStatus](ctx, c.COSI, runtimeres.NewUnattendedInstallStatus().Metadata())
	// Older COSI servers reject unknown types with this specific PermissionDenied
	// response. Do not suppress actual authorization failures on supported types.
	unsupported := status.Code(err) == codes.PermissionDenied &&
		status.Convert(err).Message() == fmt.Sprintf("resource type %q is not supported", runtimeres.UnattendedInstallStatusType)
	if err != nil && !cosistate.IsNotFoundError(err) && !unsupported {
		return fmt.Errorf("reading unattended install status: %w", err)
	}

	if install != nil && install.TypedSpec().Phase != runtimeres.UnattendedInstallPhaseInstalled {
		return fmt.Errorf("waiting for unattended install: phase is %s", install.TypedSpec().Phase)
	}

	cri, err := safe.StateGet[*serviceres.Service](ctx, c.COSI, serviceres.NewService("cri").Metadata())
	if err != nil {
		return fmt.Errorf("reading CRI service status: %w", err)
	}

	if spec := cri.TypedSpec(); !spec.Running || !spec.Healthy || spec.Unknown {
		return fmt.Errorf("waiting for CRI containerd to be running and healthy")
	}

	// Neither the running stage nor overall Ready is required: finishing boot
	// can wait for etcd, whose bootstrap depends on this resource completing.
	return nil
}

// talosMachineUpgrade upgrades the Talos OS to the desired installer image
// by performing: pull → install → drain → reboot → uncordon.
func talosMachineUpgrade(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel, waitForKubernetes bool) (retErr error) {
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

	if err := talosMachineReboot(ctx, endpoint, node, talosConfig, rebootModeStr, waitForKubernetes); err != nil {
		return fmt.Errorf("waiting for node after reboot: %w", err)
	}

	return nil
}

// talosMachineUpgradeIfNeeded checks the running Talos version and, for Image Factory
// images, the active schematic. If either differs from the desired image, it performs:
// pull → install → drain → reboot → uncordon.
func talosMachineUpgradeIfNeeded(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel) (retErr error) {
	runningImage, err := talosMachineRunningVersion(ctx, endpoint, node, talosConfig, state.Image.ValueString())
	if err != nil {
		return fmt.Errorf("reading running version: %w", err)
	}

	if runningImage != state.Image.ValueString() {
		return talosMachineUpgrade(ctx, endpoint, node, talosConfig, state, false)
	}

	requestedSchematic, isImageFactoryImage := talosMachineImageFactorySchematic(state.Image.ValueString())
	if !isImageFactoryImage {
		return nil
	}

	runningSchematic, err := talosMachineRunningSchematic(ctx, endpoint, node, talosConfig)
	if err != nil {
		return fmt.Errorf("reading active Image Factory schematic: %w", err)
	}

	if runningSchematic == requestedSchematic {
		return nil
	}

	return talosMachineUpgrade(ctx, endpoint, node, talosConfig, state, false)
}

// talosMachineImageFactorySchematic returns the schematic ID embedded in the standard
// Image Factory installer repository layout:
// <registry>/[<platform>-]installer[-secureboot]/<schematic>:<version>.
func talosMachineImageFactorySchematic(imageRef string) (string, bool) {
	if strings.Contains(imageRef, "@") {
		return "", false
	}

	repository := imageRef
	lastSlash := strings.LastIndex(repository, "/")

	if tagStart := strings.LastIndex(repository, ":"); tagStart > lastSlash {
		repository = repository[:tagStart]
	}

	parts := strings.Split(repository, "/")
	if len(parts) < 2 {
		return "", false
	}

	installerRepository := parts[len(parts)-2]
	schematicID := parts[len(parts)-1]
	isFactoryInstaller := installerRepository == "installer" ||
		installerRepository == "installer-secureboot" ||
		strings.HasSuffix(installerRepository, "-installer") ||
		strings.HasSuffix(installerRepository, "-installer-secureboot")

	if schematicID == "" || !isFactoryInstaller {
		return "", false
	}

	return schematicID, true
}

// talosMachineRunningSchematic reads the Image Factory schematic reported by Talos as
// an ExtensionStatus pseudo-extension. A vanilla image has no such status and returns
// an empty ID, which intentionally differs from every requested Factory schematic.
func talosMachineRunningSchematic(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config) (string, error) {
	var schematicID string

	err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		items, err := safe.StateListAll[*runtimeres.ExtensionStatus](nodeCtx, c.COSI)
		if err != nil {
			return err
		}

		for item := range items.All() {
			if item.TypedSpec().Metadata.Name == "schematic" {
				schematicID = item.TypedSpec().Metadata.Version

				break
			}
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return schematicID, nil
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
				runningImage = talosMachineReconcileRunningImage(desiredImage, msg.Version.Tag)

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

func talosMachineReboot(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, rebootModeStr string, waitForKubernetes bool) error {
	rebootModeVal, ok := machineapi.RebootRequest_Mode_value[rebootModeStr]
	if !ok {
		rebootModeVal = int32(machineapi.RebootRequest_DEFAULT)
	}

	rebootMode := machineapi.RebootRequest_Mode(rebootModeVal)

	if !waitForKubernetes {
		return talosMachineRebootBeforeBootstrap(ctx, endpoint, node, talosConfig, rebootMode, talosClientOp)
	}

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

// Create can precede etcd bootstrap, so Kubernetes readiness cannot gate its
// reboot. Poll boot ID and CRI directly: shutdown events can be lost on reconnect.
// Restricted roles that cannot read boot ID fall back to checking API readiness.
func talosMachineRebootBeforeBootstrap(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, rebootMode machineapi.RebootRequest_Mode, op clientOpFunc) error {
	preBootID, err := talosMachineBootID(ctx, endpoint, node, talosConfig, op)
	if err != nil {
		if status.Code(err) != codes.PermissionDenied {
			return fmt.Errorf("reading boot ID before reboot: %w", err)
		}

		tflog.Warn(ctx, "could not read boot ID before reboot; falling back to API readiness", map[string]any{"error": err.Error()})
	}

	if err := op(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		return c.Reboot(nodeCtx, client.WithRebootMode(rebootMode))
	}); err != nil {
		return fmt.Errorf("requesting reboot: %w", err)
	}

	return retry.RetryContext(ctx, 15*time.Minute, func() *retry.RetryError {
		attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		if preBootID != "" {
			bootID, err := talosMachineBootID(attemptCtx, endpoint, node, talosConfig, op)
			switch {
			case status.Code(err) == codes.PermissionDenied:
				tflog.Warn(ctx, "could not read boot ID after reboot; falling back to API readiness", map[string]any{"error": err.Error()})

				preBootID = ""
			case err != nil:
				return retry.RetryableError(err)
			case bootID == preBootID:
				return retry.RetryableError(errors.New("waiting for boot ID to change"))
			}
		}

		if err := op(attemptCtx, endpoint, node, talosConfig, talosMachineCheckBootReadyOrVersion); err != nil {
			return talosMachineBootRetryError(err)
		}

		return nil
	})
}

// talosMachineBootID reads the node's boot ID, which the kernel regenerates on every boot.
// Used to detect that a node has actually rebooted, the same signal
// action.BootIDChangedPostCheckFn uses for the reboot and v1.13+ upgrade paths.
func talosMachineBootID(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, op clientOpFunc) (string, error) {
	var bootID string

	if err := op(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		reader, err := c.Read(nodeCtx, "/proc/sys/kernel/random/boot_id")
		if err != nil {
			return err
		}

		defer reader.Close() //nolint:errcheck

		body, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		bootID = strings.TrimSpace(string(body))

		return nil
	}); err != nil {
		return "", err
	}

	if bootID == "" {
		return "", errors.New("node returned an empty boot ID")
	}

	return bootID, nil
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

	// An upgrade always reboots, so a changed boot ID is what marks it complete. The
	// running Talos version cannot be used: the image can change without the version
	// changing (a new Image Factory schematic at the same tag), and replaceImageTag
	// rebuilds the "running" image from the desired one, so it compared equal on the first
	// poll and reported success before the upgrade had done anything. See issue #377.
	// A node that cannot report its boot ID (older node, restricted role) falls back to
	// waiting for the upgrade RPC itself to return.
	preBootID, bootIDErr := talosMachineBootID(ctx, endpoint, node, talosConfig, op)
	if bootIDErr != nil {
		tflog.Warn(ctx, "could not read boot ID before upgrade; waiting for the upgrade RPC to return instead", map[string]any{
			"error": bootIDErr.Error(),
		})
	}

	// On Talos < v1.13, UpgradeWithOptions holds the gRPC connection open for the entire
	// download+install duration (~30–45 min). The action.Tracker pattern can't be used here
	// because the action function doesn't return until done, causing RST_STREAM timeouts.
	// Instead, fire the RPC in a goroutine and independently poll for the reboot.
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
		rpcErrCh <- err
	}()

	var rpcDone bool

	if err := retry.RetryContext(ctx, 60*time.Minute, func() *retry.RetryError {
		// Abort early if the upgrade RPC itself failed (e.g. bad image, auth error).
		select {
		case rpcErr := <-rpcErrCh:
			if rpcErr != nil {
				return retry.NonRetryableError(fmt.Errorf("upgrade RPC failed: %w", rpcErr))
			}

			rpcDone = true
		default:
		}

		if preBootID == "" {
			if rpcDone {
				return nil
			}

			return retry.RetryableError(fmt.Errorf("waiting for the upgrade to %s to finish", state.Image.ValueString()))
		}

		currentBootID, err := talosMachineBootID(ctx, endpoint, node, talosConfig, op)
		if err != nil {
			return retry.RetryableError(err)
		}

		if currentBootID == preBootID {
			return retry.RetryableError(fmt.Errorf("node has not rebooted into %s yet", state.Image.ValueString()))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("waiting for node after upgrade: %w", err)
	}

	return nil
}

// replaceImageTag replaces the tag portion of an image reference.
// "ghcr.io/siderolabs/installer:v1.8.0" + "v1.9.0" → "ghcr.io/siderolabs/installer:v1.9.0".
// Digest-pinned references ("repo@sha256:<hex>") already fully identify the image and
// have no tag component to replace, so they are returned unchanged — otherwise the
// last ':' found belongs to the digest's "sha256:" prefix, corrupting it.
func replaceImageTag(imageRef, newTag string) string {
	if isDigestPinnedImage(imageRef) {
		return imageRef
	}

	// The tag separator is only the last ':' if it comes after the last '/' —
	// otherwise it's a registry host:port, e.g. "registry.example.com:5000/repo".
	if idx := strings.LastIndex(imageRef, ":"); idx > strings.LastIndex(imageRef, "/") {
		return imageRef[:idx+1] + newTag
	}

	return imageRef + ":" + newTag
}

// talosMachineReconcileRunningImage returns the image reference to compare against
// desiredImage when deciding whether a node already runs the desired install, given
// the version tag reported by that node. Digest-pinned images have no tag component
// to compare against the reported version, so there is no reliable way to tell
// whether the node already runs the desired image. Returning "" guarantees it never
// matches desiredImage, so the caller installs it rather than silently skipping a
// node that was never actually reconciled.
func talosMachineReconcileRunningImage(desiredImage, reportedTag string) string {
	if isDigestPinnedImage(desiredImage) {
		return ""
	}

	return replaceImageTag(desiredImage, reportedTag)
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
