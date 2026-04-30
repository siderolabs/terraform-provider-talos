// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	cosiresource "github.com/cosi-project/runtime/pkg/resource"
	"github.com/cosi-project/runtime/pkg/safe"
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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/action"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/kubeclient"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/nodedrain"
	commonapi "github.com/siderolabs/talos/pkg/machinery/api/common"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	configresource "github.com/siderolabs/talos/pkg/machinery/resources/config"
	talosreporter "github.com/siderolabs/talos/pkg/reporter"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type talosMachineResource struct{}

var (
	_ resource.Resource                   = &talosMachineResource{}
	_ resource.ResourceWithModifyPlan     = &talosMachineResource{}
	_ resource.ResourceWithValidateConfig = &talosMachineResource{}
)

type talosMachineResourceModel struct {
	OnDestroy                *onDestroyOptions     `tfsdk:"on_destroy"`
	MachineConfigurationWO   types.String          `tfsdk:"machine_configuration_wo"`
	Endpoint                 types.String          `tfsdk:"endpoint"`
	ClientConfiguration      basetypes.ObjectValue `tfsdk:"client_configuration"`
	ClientConfigurationWO    basetypes.ObjectValue `tfsdk:"client_configuration_wo"`
	MachineConfiguration     types.String          `tfsdk:"machine_configuration"`
	ID                       types.String          `tfsdk:"id"`
	Image                    types.String          `tfsdk:"image"`
	MachineConfigurationHash types.String          `tfsdk:"machine_configuration_hash"`
	RebootMode               types.String          `tfsdk:"reboot_mode"`
	Timeouts                 timeouts.Value        `tfsdk:"timeouts"`
	Node                     types.String          `tfsdk:"node"`
	DrainOnUpgrade           types.Bool            `tfsdk:"drain_on_upgrade"`
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

	sum := sha256.Sum256(cfgBytes)
	desiredHash := hex.EncodeToString(sum[:])

	var state talosMachineResourceModel

	req.State.Get(ctx, &state) //nolint:errcheck // empty on first apply

	if state.MachineConfigurationHash.ValueString() != desiredHash {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration_hash"), types.StringUnknown())...)
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

	talosConfig, resolvedClientConfig, err := resolveTalosMachineClientConfig(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config", err.Error())

		return
	}

	// Always persist resolved client config so Read() can connect without _wo.
	plan.ClientConfiguration = resolvedClientConfig

	timeout, diags := plan.Timeouts.Create(ctx, 15*time.Minute)
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

	sum := sha256.Sum256(cfgBytes)
	plan.MachineConfigurationHash = types.StringValue(hex.EncodeToString(sum[:]))

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
				base = "ghcr.io/siderolabs/installer"
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

		sum := sha256.Sum256(yamlBytes)
		state.MachineConfigurationHash = types.StringValue(hex.EncodeToString(sum[:]))

		return nil
	})

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

	talosConfig, resolvedClientConfig, err := resolveTalosMachineClientConfig(ctx, &plan)
	if err != nil {
		resp.Diagnostics.AddError("failed to build talos config", err.Error())

		return
	}

	plan.ClientConfiguration = resolvedClientConfig

	timeout, diags := plan.Timeouts.Update(ctx, 20*time.Minute)
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
		if err := talosMachineUpgradeIfNeeded(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig, &plan); err != nil {
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

		if err := talosMachineApplyConfig(ctxDeadline, endpoint, plan.Node.ValueString(), talosConfig, cfgBytes); err != nil {
			resp.Diagnostics.AddError("error applying machine configuration", err.Error())

			return
		}

		sum := sha256.Sum256(cfgBytes)
		plan.MachineConfigurationHash = types.StringValue(hex.EncodeToString(sum[:]))
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *talosMachineResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state talosMachineResourceModel

	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)

	if resp.Diagnostics.HasError() {
		return
	}

	if state.OnDestroy == nil || !state.OnDestroy.Reset {
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
		Graceful: state.OnDestroy.Graceful,
		Reboot:   state.OnDestroy.Reboot,
		SystemPartitionsToWipe: []*machineapi.ResetPartitionSpec{
			{Label: "STATE", Wipe: true},
			{Label: "EPHEMERAL", Wipe: true},
		},
	}

	actionFn := func(ctx context.Context, c *client.Client) (string, error) {
		return resetGetActorID(ctx, c, resetRequest)
	}

	if err := talosClientOp(ctx, endpoint, state.Node.ValueString(), talosConfig, func(_ context.Context, c *client.Client) error {
		executor := newClientExecutor(c, []string{state.Node.ValueString()})

		return action.NewTracker(
			executor,
			action.StopAllServicesEventFn,
			actionFn,
			action.WithDebug(false),
			action.WithTimeout(deleteTimeout),
		).Run()
	}); err != nil {
		resp.Diagnostics.AddError("error resetting machine", err.Error())
	}
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
			return talosMachineUpgradeLegacy(ctx, endpoint, node, talosConfig, state, rebootModeStr)
		}

		return fmt.Errorf("pulling installer image: %w", pullErr)
	}

	// LifecycleService (Talos v1.13+): installs without rebooting, then reboot separately.
	if installErr := talosMachineInstallImage(ctx, endpoint, node, talosConfig, state.Image.ValueString(), containerdInst); installErr != nil {
		return fmt.Errorf("installing new OS image: %w", installErr)
	}

	k8sNodeName, err := talosMachineCordonAndDrain(ctx, endpoint, node, talosConfig, state.DrainOnUpgrade.ValueBool())
	if err != nil {
		return fmt.Errorf("draining node: %w", err)
	}

	// Uncordon in defer so the node is never left cordoned, even if the reboot fails.
	defer func() {
		if k8sNodeName == "" {
			return
		}

		if err := talosMachineUncordon(ctx, endpoint, node, talosConfig, k8sNodeName); err != nil {
			retErr = errors.Join(retErr, fmt.Errorf("uncordoning node: %w", err))
		}
	}()

	if err := talosMachineReboot(ctx, endpoint, node, talosConfig, rebootModeStr); err != nil {
		return fmt.Errorf("waiting for node after reboot: %w", err)
	}

	return nil
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

func talosMachineCordonAndDrain(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, drain bool) (string, error) {
	if !drain {
		return "", nil
	}

	var k8sNodeName string

	noopReport := func(talosreporter.Update) {}

	if err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		cs, err := kubeclient.FromTalosClient(nodeCtx, c)
		if err != nil {
			return fmt.Errorf("building k8s client for drain: %w", err)
		}

		k8sName, err := nodedrain.GetKubernetesNodeName(nodeCtx, c)
		if err != nil {
			return fmt.Errorf("resolving k8s node name: %w", err)
		}

		k8sNodeName = k8sName

		return nodedrain.CordonAndDrain(ctx, cs, k8sNodeName, nodedrain.Options{}, noopReport)
	}); err != nil {
		return "", err
	}

	return k8sNodeName, nil
}

func talosMachineUncordon(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, k8sNodeName string) error {
	noopReport := func(talosreporter.Update) {}

	return talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
		cs, err := kubeclient.FromTalosClient(nodeCtx, c)
		if err != nil {
			return fmt.Errorf("building k8s client for uncordon: %w", err)
		}

		if waitErr := nodedrain.WaitForNodeReady(ctx, cs, k8sNodeName, 5*time.Minute); waitErr != nil {
			return fmt.Errorf("waiting for node ready: %w", waitErr)
		}

		return nodedrain.Uncordon(ctx, cs, k8sNodeName, noopReport)
	})
}

func talosMachineReboot(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, rebootModeStr string) error {
	rebootModeVal, ok := machineapi.RebootRequest_Mode_value[rebootModeStr]
	if !ok {
		rebootModeVal = int32(machineapi.RebootRequest_DEFAULT)
	}

	rebootMode := machineapi.RebootRequest_Mode(rebootModeVal)

	return talosClientOp(ctx, endpoint, node, talosConfig, func(_ context.Context, c *client.Client) error {
		executor := newClientExecutor(c, []string{node})

		return action.NewTracker(
			executor,
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
		).Run()
	})
}

// talosMachineUpgradeLegacy handles Talos < 1.13 nodes where LifecycleService is not available.
// MachineService.Upgrade combines install + reboot atomically. drain_on_upgrade is not applied
// here — talosctl upgrade does not drain on the legacy path either.
func talosMachineUpgradeLegacy(ctx context.Context, endpoint, node string, talosConfig *clientconfig.Config, state *talosMachineResourceModel, rebootModeStr string) error {
	upgradeRebootModeVal, ok := machineapi.UpgradeRequest_RebootMode_value[rebootModeStr]
	if !ok {
		upgradeRebootModeVal = int32(machineapi.UpgradeRequest_DEFAULT)
	}

	upgradeRebootMode := machineapi.UpgradeRequest_RebootMode(upgradeRebootModeVal)

	// On Talos < v1.13, UpgradeWithOptions holds the gRPC connection open for the entire
	// download+install duration (~30–45 min). The action.Tracker pattern can't be used here
	// because the action function doesn't return until done, causing RST_STREAM timeouts.
	// Instead, fire the RPC in a goroutine and independently poll for the version change.
	go func() {
		_ = talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error { //nolint:errcheck
			opts := []client.UpgradeOption{
				client.WithUpgradeImage(state.Image.ValueString()),
				client.WithUpgradeRebootMode(upgradeRebootMode),
			}

			_, err := c.UpgradeWithOptions(nodeCtx, opts...) //nolint:staticcheck

			return err
		})
	}()

	if err := retry.RetryContext(ctx, 60*time.Minute, func() *retry.RetryError {
		err := talosClientOp(ctx, endpoint, node, talosConfig, func(nodeCtx context.Context, c *client.Client) error {
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
