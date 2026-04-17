// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

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
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/talos/cmd/talosctl/pkg/talos/action"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type talosMachineConfigurationApplyResource struct{}

var (
	_ resource.Resource                   = &talosMachineConfigurationApplyResource{}
	_ resource.ResourceWithModifyPlan     = &talosMachineConfigurationApplyResource{}
	_ resource.ResourceWithUpgradeState   = &talosMachineConfigurationApplyResource{}
	_ resource.ResourceWithValidateConfig = &talosMachineConfigurationApplyResource{}
)

var onDestroyMarkDownDescription = `Actions to be taken on destroy, if *reset* is not set this is a no-op.

> Note: Any changes to *on_destroy* block has to be applied first by running *terraform apply* first,
then a subsequent *terraform destroy* for the changes to take effect due to limitations in Terraform provider framework.
`

type talosMachineConfigurationApplyResourceModelV0 struct {
	Mode                 types.String `tfsdk:"mode"`
	Node                 types.String `tfsdk:"node"`
	Endpoint             types.String `tfsdk:"endpoint"`
	TalosConfig          types.String `tfsdk:"talos_config"`
	MachineConfiguration types.String `tfsdk:"machine_configuration"`
	ConfigPatches        types.List   `tfsdk:"config_patches"`
}

type talosMachineConfigurationApplyResourceModelV1 struct { //nolint:govet
	ID                          types.String          `tfsdk:"id"`
	ApplyMode                   types.String          `tfsdk:"apply_mode"`
	ResolvedApplyMode           types.String          `tfsdk:"resolved_apply_mode"`
	Node                        types.String          `tfsdk:"node"`
	Endpoint                    types.String          `tfsdk:"endpoint"`
	ClientConfiguration         basetypes.ObjectValue `tfsdk:"client_configuration"`
	ClientConfigurationWO       basetypes.ObjectValue `tfsdk:"client_configuration_wo"`
	MachineConfigurationInput   types.String          `tfsdk:"machine_configuration_input"`
	MachineConfigurationInputWO types.String          `tfsdk:"machine_configuration_input_wo"`
	OnDestroy                   *onDestroyOptions     `tfsdk:"on_destroy"`
	MachineConfiguration        types.String          `tfsdk:"machine_configuration"`
	MachineConfigurationHash    types.String          `tfsdk:"machine_configuration_hash"`
	ConfigPatches               []types.String        `tfsdk:"config_patches"`
	Timeouts                    timeouts.Value        `tfsdk:"timeouts"`
}

type onDestroyOptions struct {
	Reset    bool `tfsdk:"reset"`
	Graceful bool `tfsdk:"graceful"`
	Reboot   bool `tfsdk:"reboot"`
}

// NewTalosMachineConfigurationApplyResource implements the resource.Resource interface.
func NewTalosMachineConfigurationApplyResource() resource.Resource {
	return &talosMachineConfigurationApplyResource{}
}

func (p *talosMachineConfigurationApplyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_machine_configuration_apply"
}

func (p *talosMachineConfigurationApplyResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Version:     1,
		Description: "The machine configuration apply resource allows to apply machine configuration to a node",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:    true,
				Description: "This is a unique identifier for the machine ",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"apply_mode": schema.StringAttribute{
				Optional: true,
				Computed: true,
				Description: "The mode of the apply operation. Use 'staged_if_needing_reboot' for automatic reboot prevention: " +
					"performs a dry-run and uses 'staged' mode if reboot is needed, 'auto' otherwise",
				Validators: []validator.String{
					stringvalidator.OneOf("auto", "reboot", "no_reboot", "staged", "staged_if_needing_reboot"),
				},
				Default: stringdefault.StaticString("auto"),
			},
			"resolved_apply_mode": schema.StringAttribute{
				Computed: true,
				Description: "The actual apply mode used. When apply_mode is 'staged_if_needing_reboot', " +
					"shows the resolved mode ('auto' or 'staged') based on dry-run analysis. Equals apply_mode for other modes.",
			},
			"node": schema.StringAttribute{
				Required:    true,
				Description: "The name of the node to bootstrap",
			},
			"endpoint": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "The endpoint of the machine to bootstrap",
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
			"machine_configuration_input": schema.StringAttribute{
				Description: "The machine configuration to apply",
				Optional:    true,
				Sensitive:   true,
			},
			"machine_configuration_input_wo": schema.StringAttribute{
				Description: "The machine configuration to apply (write-only). Use this instead of machine_configuration_input when using ephemeral resources. Requires Terraform 1.11+",
				Optional:    true,
				WriteOnly:   true,
			},
			"on_destroy": schema.SingleNestedAttribute{
				Description:         "Actions to be taken on destroy, if `reset` is not set this is a no-op.",
				MarkdownDescription: onDestroyMarkDownDescription,
				Optional:            true,
				Attributes: map[string]schema.Attribute{
					"reset": schema.BoolAttribute{
						Description: "Reset the machine to the initial state (STATE and EPHEMERAL will be wiped). Default false",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
					"graceful": schema.BoolAttribute{
						Description: "Graceful indicates whether node should leave etcd before the upgrade, it also enforces etcd checks before leaving. Default true",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(true),
					},
					"reboot": schema.BoolAttribute{
						Description: "Reboot indicates whether node should reboot or halt after resetting. Default false",
						Optional:    true,
						Computed:    true,
						Default:     booldefault.StaticBool(false),
					},
				},
			},
			"machine_configuration": schema.StringAttribute{
				Description: "The generated machine configuration after applying patches",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"machine_configuration_hash": schema.StringAttribute{
				Description: "SHA256 hex digest of the rendered machine configuration (input plus patches). " +
					"Persisted in state so that changes to machine_configuration_input_wo — which is write-only " +
					"and itself invisible to state — still surface as plan diffs.",
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"config_patches": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true,
				Description: "The list of config patches to apply",
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{
				Create: true,
				Update: true,
				Delete: true,
			}),
		},
	}
}

func (p *talosMachineConfigurationApplyResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var config talosMachineConfigurationApplyResourceModelV1

	diags := req.Config.Get(ctx, &config)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	inputSet := !config.MachineConfigurationInput.IsNull()
	inputWOSet := !config.MachineConfigurationInputWO.IsNull()

	if !inputSet && !inputWOSet {
		resp.Diagnostics.AddError(
			"Missing machine configuration input",
			"Exactly one of machine_configuration_input or machine_configuration_input_wo must be set",
		)
	}

	if inputSet && inputWOSet {
		resp.Diagnostics.AddError(
			"Conflicting machine configuration input",
			"Only one of machine_configuration_input or machine_configuration_input_wo can be set, not both",
		)
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

// getMachineConfigurationInput returns the effective machine configuration input value,
// preferring the write-only attribute if set.
func getMachineConfigurationInput(state *talosMachineConfigurationApplyResourceModelV1) types.String {
	if !state.MachineConfigurationInputWO.IsNull() && !state.MachineConfigurationInputWO.IsUnknown() {
		return state.MachineConfigurationInputWO
	}

	return state.MachineConfigurationInput
}

// computeMachineConfiguration applies patches to the input configuration and returns the result.
func computeMachineConfiguration(state *talosMachineConfigurationApplyResourceModelV1) (string, error) {
	machineConfigInput := getMachineConfigurationInput(state)
	if machineConfigInput.IsNull() {
		return "", fmt.Errorf("machine configuration input is null")
	}

	configPatches := make([]string, len(state.ConfigPatches))
	for i, patch := range state.ConfigPatches {
		if !patch.IsNull() {
			configPatches[i] = patch.ValueString()
		}
	}

	patches, err := configpatcher.LoadPatches(configPatches)
	if err != nil {
		return "", fmt.Errorf("error loading config patches: %w", err)
	}

	cfg, err := configpatcher.Apply(configpatcher.WithBytes([]byte(machineConfigInput.ValueString())), patches)
	if err != nil {
		return "", fmt.Errorf("error applying config patches: %w", err)
	}

	cfgBytes, err := cfg.Bytes()
	if err != nil {
		return "", fmt.Errorf("error converting config to bytes: %w", err)
	}

	return string(cfgBytes), nil
}

// getClientConfiguration returns the effective client configuration,
// preferring the write-only attribute if set.
func getClientConfiguration(state *talosMachineConfigurationApplyResourceModelV1) (config basetypes.ObjectValue, diagMsg string) {
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

// getClientConfigurationValues extracts the client configuration values from the ObjectValue.
// Returns empty strings and error message if extraction fails.
// Handles both properly-typed ObjectValues and plain maps with correct keys (from Vault reconstruction).
func getClientConfigurationValues(ctx context.Context, clientConfig basetypes.ObjectValue) (ca, cert, key, errMsg string, ok bool) {
	if clientConfig.IsUnknown() {
		return "", "", "", "client configuration is unknown", false
	}

	if clientConfig.IsNull() {
		return "", "", "", "client configuration is null", false
	}

	var config clientConfiguration
	if diags := clientConfig.As(ctx, &config, basetypes.ObjectAsOptions{}); diags.HasError() {
		// Fallback: Extract values directly from the ObjectValue's attributes
		// This handles cases where the object is a plain map with correct keys (e.g., reconstructed from Vault)
		// without full schema metadata attached at runtime
		attrs := clientConfig.Attributes()

		caAttr, caOk := attrs["ca_certificate"]
		certAttr, certOk := attrs["client_certificate"]
		keyAttr, keyOk := attrs["client_key"]

		if !caOk || !certOk || !keyOk {
			missingKeys := []string{}
			if !caOk {
				missingKeys = append(missingKeys, "ca_certificate")
			}

			if !certOk {
				missingKeys = append(missingKeys, "client_certificate")
			}

			if !keyOk {
				missingKeys = append(missingKeys, "client_key")
			}

			return "", "", "", fmt.Sprintf("missing required keys: %v (available keys: %v)", missingKeys, getMapKeys(attrs)), false
		}

		// Type assert to StringValue and extract string
		caVal, caIsString := caAttr.(basetypes.StringValue)
		certVal, certIsString := certAttr.(basetypes.StringValue)
		keyVal, keyIsString := keyAttr.(basetypes.StringValue)

		if !caIsString || !certIsString || !keyIsString {
			wrongTypes := []string{}

			if !caIsString {
				wrongTypes = append(wrongTypes, fmt.Sprintf("ca_certificate is %T", caAttr))
			}

			if !certIsString {
				wrongTypes = append(wrongTypes, fmt.Sprintf("client_certificate is %T", certAttr))
			}

			if !keyIsString {
				wrongTypes = append(wrongTypes, fmt.Sprintf("client_key is %T", keyAttr))
			}

			return "", "", "", fmt.Sprintf("wrong types for client configuration attributes: %s", strings.Join(wrongTypes, ", ")), false
		}

		return caVal.ValueString(), certVal.ValueString(), keyVal.ValueString(), "", true
	}

	return config.CA.ValueString(), config.Cert.ValueString(), config.Key.ValueString(), "", true
}

// getMapKeys returns the keys of a map for diagnostic purposes.
func getMapKeys(m map[string]attr.Value) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	return keys
}

func (p *talosMachineConfigurationApplyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) { //nolint:dupl
	var state talosMachineConfigurationApplyResourceModelV1

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// CRITICAL: Write-only attributes are NOT in Plan, only in Config!
	// We need to read write-only attributes from Config
	var configState talosMachineConfigurationApplyResourceModelV1

	configDiags := req.Config.Get(ctx, &configState)
	resp.Diagnostics.Append(configDiags...)

	if configDiags.HasError() {
		return
	}

	// Use write-only attributes from Config, everything else from Plan
	if !configState.ClientConfigurationWO.IsNull() {
		state.ClientConfigurationWO = configState.ClientConfigurationWO
	}

	if !configState.MachineConfigurationInputWO.IsNull() {
		state.MachineConfigurationInputWO = configState.MachineConfigurationInputWO
	}

	clientConfig, configDiag := getClientConfiguration(&state)
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
			"Error reading client configuration",
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

	effectiveMode := getEffectiveMode(&state)

	// Get the machine configuration to apply
	// When using write-only inputs, machine_configuration is not stored in state,
	// so we need to compute it here from the input and patches
	machineConfigToApply := state.MachineConfiguration.ValueString()
	if state.MachineConfiguration.IsNull() && !state.MachineConfigurationInputWO.IsNull() {
		// Compute configuration from write-only input and patches
		computed, err := computeMachineConfiguration(&state)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error computing machine configuration",
				err.Error(),
			)

			return
		}

		machineConfigToApply = computed
	}

	if err := retry.RetryContext(ctxDeadline, createTimeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosClientConfig, func(nodeCtx context.Context, c *client.Client) error {
			_, err := c.ApplyConfiguration(nodeCtx, &machineapi.ApplyConfigurationRequest{
				Mode: machineapi.ApplyConfigurationRequest_Mode(machineapi.ApplyConfigurationRequest_Mode_value[strings.ToUpper(effectiveMode)]),
				Data: []byte(machineConfigToApply),
			})
			if err != nil {
				return err
			}

			return nil
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"Error applying configuration",
			err.Error(),
		)

		return
	}

	state.ID = basetypes.NewStringValue("machine_configuration_apply")

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (p *talosMachineConfigurationApplyResource) Read(_ context.Context, _ resource.ReadRequest, _ *resource.ReadResponse) {
}

func (p *talosMachineConfigurationApplyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) { //nolint:dupl
	var state talosMachineConfigurationApplyResourceModelV1

	diags := req.Plan.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	// CRITICAL: Write-only attributes are NOT in Plan, only in Config!
	var configState talosMachineConfigurationApplyResourceModelV1

	configDiags := req.Config.Get(ctx, &configState)
	resp.Diagnostics.Append(configDiags...)

	if configDiags.HasError() {
		return
	}

	// Use write-only attributes from Config
	if !configState.ClientConfigurationWO.IsNull() {
		state.ClientConfigurationWO = configState.ClientConfigurationWO
	}

	if !configState.MachineConfigurationInputWO.IsNull() {
		state.MachineConfigurationInputWO = configState.MachineConfigurationInputWO
	}

	clientConfig, configDiag := getClientConfiguration(&state)
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
			"Error reading client configuration",
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

	updateTimeout, diags := state.Timeouts.Update(ctx, 10*time.Minute)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	ctxDeadline, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	effectiveMode := getEffectiveMode(&state)

	// Get the machine configuration to apply
	// When using write-only inputs, machine_configuration is not stored in state,
	// so we need to compute it here from the input and patches
	machineConfigToApply := state.MachineConfiguration.ValueString()
	if state.MachineConfiguration.IsNull() && !state.MachineConfigurationInputWO.IsNull() {
		// Compute configuration from write-only input and patches
		computed, err := computeMachineConfiguration(&state)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error computing machine configuration",
				err.Error(),
			)

			return
		}

		machineConfigToApply = computed
	}

	if err := retry.RetryContext(ctxDeadline, updateTimeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosClientConfig, func(nodeCtx context.Context, c *client.Client) error {
			_, err := c.ApplyConfiguration(nodeCtx, &machineapi.ApplyConfigurationRequest{
				Mode: machineapi.ApplyConfigurationRequest_Mode(machineapi.ApplyConfigurationRequest_Mode_value[strings.ToUpper(effectiveMode)]),
				Data: []byte(machineConfigToApply),
			})
			if err != nil {
				return err
			}

			return nil
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics.AddError(
			"Error applying configuration",
			err.Error(),
		)

		return
	}

	state.ID = basetypes.NewStringValue("machine_configuration_apply")

	// Set state to fully populated data
	diags = resp.State.Set(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}
}

func getEffectiveMode(state *talosMachineConfigurationApplyResourceModelV1) string {
	effectiveMode := state.ResolvedApplyMode.ValueString()
	if effectiveMode == "" || state.ResolvedApplyMode.IsNull() {
		effectiveMode = state.ApplyMode.ValueString()
	}

	return effectiveMode
}

func (p *talosMachineConfigurationApplyResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state talosMachineConfigurationApplyResourceModelV1

	diags := req.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	if diags.HasError() {
		return
	}

	if state.OnDestroy != nil && state.OnDestroy.Reset {
		// NOTE: During Delete, write-only attributes are not available (not in state)
		// If using client_configuration_wo, the reset on destroy won't work
		// Users must use client_configuration (non-write-only) if they need on_destroy.reset
		clientConfig, configDiag := getClientConfiguration(&state)
		if configDiag != "" {
			resp.Diagnostics.AddError(
				"Client configuration issue during destroy",
				fmt.Sprintf("%s\n\nNote: If you're using client_configuration_wo (write-only), "+
					"it's not available during destroy. Use client_configuration instead if you need on_destroy.reset functionality.",
					configDiag),
			)

			return
		}

		ca, cert, key, errMsg, ok := getClientConfigurationValues(ctx, clientConfig)
		if !ok {
			resp.Diagnostics.AddError(
				"Error reading client configuration",
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

		deleteTimeout, diags := state.Timeouts.Delete(ctx, 10*time.Minute)
		resp.Diagnostics.Append(diags...)

		if resp.Diagnostics.HasError() {
			return
		}

		resetRequest := &machineapi.ResetRequest{
			Graceful: state.OnDestroy.Graceful,
			Reboot:   state.OnDestroy.Reboot,
			SystemPartitionsToWipe: []*machineapi.ResetPartitionSpec{
				{
					Label: "STATE",
					Wipe:  true,
				},
				{
					Label: "EPHEMERAL",
					Wipe:  true,
				},
			},
		}

		actionFn := func(ctx context.Context, c *client.Client) (string, error) {
			return resetGetActorID(ctx, c, resetRequest)
		}

		var postCheckFn func(context.Context, *client.Client, string) error

		if state.OnDestroy.Reboot {
			postCheckFn = func(ctx context.Context, c *client.Client, preActionBootID string) error {
				insecureClient, err := client.New(
					ctx,
					client.WithTLSConfig(&tls.Config{
						InsecureSkipVerify: true,
					}),
					client.WithEndpoints(state.Endpoint.ValueString()),
				)
				if err != nil {
					return err
				}

				_, err = insecureClient.Disks(client.WithNode(ctx, state.Node.ValueString()))

				// if we can get into maintenance mode, reset has succeeded
				if err == nil {
					return nil
				}

				// try to get the boot ID in the normal mode to see if the node has rebooted
				return action.BootIDChangedPostCheckFn(ctx, c, preActionBootID)
			}
		}

		if err := talosClientOp(ctx, state.Endpoint.ValueString(), state.Node.ValueString(), talosClientConfig, func(_ context.Context, c *client.Client) error {
			executor := newClientExecutor(c, []string{state.Node.ValueString()})

			return action.NewTracker(
				executor,
				action.StopAllServicesEventFn,
				actionFn,
				action.WithPostCheck(postCheckFn),
				action.WithDebug(false),
				action.WithTimeout(deleteTimeout),
			).Run()
		}); err != nil {
			resp.Diagnostics.AddError("Error resetting machine", err.Error())

			return
		}
	}
}

func setResolvedApplyMode(ctx context.Context, resp *resource.ModifyPlanResponse, mode string) {
	diags := resp.Plan.SetAttribute(ctx, path.Root("resolved_apply_mode"), mode)
	resp.Diagnostics.Append(diags...)
}

func dryRunNeedsReboot(cfgBytes []byte, needsReboot *bool) func(context.Context, *client.Client) error {
	return func(nodeCtx context.Context, c *client.Client) error {
		applyResp, err := c.ApplyConfiguration(nodeCtx, &machineapi.ApplyConfigurationRequest{
			Mode:   machineapi.ApplyConfigurationRequest_AUTO,
			Data:   cfgBytes,
			DryRun: true,
		})
		if err != nil {
			return err
		}

		if len(applyResp.Messages) > 0 {
			*needsReboot = (applyResp.Messages[0].Mode == machineapi.ApplyConfigurationRequest_REBOOT)
		}

		return nil
	}
}

func (p *talosMachineConfigurationApplyResource) handleRebootPrevention(
	ctx context.Context,
	req resource.ModifyPlanRequest,
	resp *resource.ModifyPlanResponse,
	planState *talosMachineConfigurationApplyResourceModelV1,
	cfgBytes []byte,
) {
	applyMode := strings.ToLower(planState.ApplyMode.ValueString())
	if applyMode == "" || planState.ApplyMode.IsNull() || planState.ApplyMode.IsUnknown() {
		applyMode = "auto"
	}

	if applyMode != "staged_if_needing_reboot" {
		setResolvedApplyMode(ctx, resp, applyMode)

		return
	}

	// Cannot perform dry-run if node address is unknown (computed from another resource)
	if planState.Node.IsUnknown() {
		setResolvedApplyMode(ctx, resp, "auto")

		return
	}

	// For updates: avoid unnecessary dry-run if configuration hasn't changed
	if !req.State.Raw.IsNull() {
		var currentState talosMachineConfigurationApplyResourceModelV1

		diags := req.State.Get(ctx, &currentState)
		if diags.HasError() {
			return
		}

		if currentState.MachineConfiguration.Equal(types.StringValue(string(cfgBytes))) {
			if !currentState.ResolvedApplyMode.IsNull() && currentState.ResolvedApplyMode.ValueString() != "" {
				setResolvedApplyMode(ctx, resp, currentState.ResolvedApplyMode.ValueString())

				return
			}
		}
	}

	endpoint := planState.Endpoint.ValueString()
	if endpoint == "" || planState.Endpoint.IsNull() || planState.Endpoint.IsUnknown() {
		endpoint = planState.Node.ValueString()
	}

	// Cannot perform dry-run if client configuration is unknown (from ephemeral resource)
	clientConfig, configDiag := getClientConfiguration(planState)
	if configDiag != "" {
		// If configuration is not available (unknown/null), fall back to auto mode
		setResolvedApplyMode(ctx, resp, "auto")

		return
	}

	ca, cert, key, _, ok := getClientConfigurationValues(ctx, clientConfig)
	if !ok {
		setResolvedApplyMode(ctx, resp, "auto")

		return
	}

	talosClientConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		ca,
		cert,
		key,
	)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Cannot check reboot requirement",
			fmt.Sprintf("Node %s: %v. Using 'auto' mode (may reboot).", planState.Node.ValueString(), err),
		)
		setResolvedApplyMode(ctx, resp, "auto")

		return
	}

	var needsReboot bool

	err = talosClientOp(ctx, endpoint, planState.Node.ValueString(), talosClientConfig,
		dryRunNeedsReboot(cfgBytes, &needsReboot),
	)
	if err != nil {
		resp.Diagnostics.AddWarning(
			"Cannot check reboot requirement",
			fmt.Sprintf("Node %s: %v. Using 'auto' mode (may reboot).", planState.Node.ValueString(), err),
		)
		setResolvedApplyMode(ctx, resp, "auto")

		return
	}

	if needsReboot {
		setResolvedApplyMode(ctx, resp, "staged")
		resp.Diagnostics.AddWarning(
			"Reboot prevented - using staged mode",
			fmt.Sprintf("Node %s: Configuration requires reboot. Using 'staged' mode. Manually reboot with: talosctl reboot --nodes %s",
				planState.Node.ValueString(), planState.Node.ValueString()),
		)
	} else {
		setResolvedApplyMode(ctx, resp, "auto")
	}
}

func (p *talosMachineConfigurationApplyResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) { //nolint:gocyclo,cyclop
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

	var config talosMachineConfigurationApplyResourceModelV1

	diags = configObj.As(ctx, &config, basetypes.ObjectAsOptions{
		UnhandledNullAsEmpty:    true,
		UnhandledUnknownAsEmpty: true,
	})
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	// if either endpoint or node is unknown return early
	if config.Endpoint.IsUnknown() || config.Node.IsUnknown() || config.MachineConfiguration.IsUnknown() {
		return
	}

	var planObj types.Object

	diags = req.Plan.Get(ctx, &planObj)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var planState talosMachineConfigurationApplyResourceModelV1

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

	machineConfigInput := getMachineConfigurationInput(&planState)

	// Only compute machine_configuration if inputs are available
	// If inputs are unknown (e.g., ephemeral values not yet resolved), the UseStateForUnknown
	// plan modifier will preserve the prior state value to prevent drift
	if !machineConfigInput.IsUnknown() && !machineConfigInput.IsNull() {
		configPatches := make([]string, len(planState.ConfigPatches))

		for i, patch := range planState.ConfigPatches {
			// if any of the patches is unknown, return early
			if patch.IsUnknown() {
				return
			}

			if !patch.IsUnknown() && !patch.IsNull() {
				configPatches[i] = patch.ValueString()
			}
		}

		patches, err := configpatcher.LoadPatches(configPatches)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error loading config patches",
				err.Error(),
			)

			return
		}

		cfg, err := configpatcher.Apply(configpatcher.WithBytes([]byte(machineConfigInput.ValueString())), patches)
		if err != nil {
			resp.Diagnostics.AddError(
				"Error applying config patches",
				err.Error(),
			)

			return
		}

		cfgBytes, err := cfg.Bytes()
		if err != nil {
			resp.Diagnostics.AddError(
				"Error converting config to bytes",
				err.Error(),
			)

			return
		}

		p.setPlanMachineConfiguration(ctx, resp, &planState, cfgBytes)

		if resp.Diagnostics.HasError() {
			return
		}

		p.handleRebootPrevention(ctx, req, resp, &planState, cfgBytes)
	}
}

// setPlanMachineConfiguration sets the machine_configuration attribute in the plan.
// When write-only inputs are used, it sets the value to null to avoid storing secrets in state.
// It also always sets machine_configuration_hash — a SHA256 fingerprint of the rendered
// config — so that changes to write-only inputs (invisible to state) surface as plan diffs.
func (p *talosMachineConfigurationApplyResource) setPlanMachineConfiguration(
	ctx context.Context,
	resp *resource.ModifyPlanResponse,
	planState *talosMachineConfigurationApplyResourceModelV1,
	cfgBytes []byte,
) {
	sum := sha256.Sum256(cfgBytes)
	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration_hash"), hex.EncodeToString(sum[:]))...)

	// When using write-only inputs (_wo variants), don't populate the computed
	// machine_configuration to prevent secrets from being stored in state.
	if !planState.MachineConfigurationInputWO.IsNull() {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration"), types.StringNull())...)

		return
	}

	resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("machine_configuration"), string(cfgBytes))...)
}

func (p *talosMachineConfigurationApplyResource) UpgradeState(_ context.Context) map[int64]resource.StateUpgrader {
	return map[int64]resource.StateUpgrader{
		0: {
			PriorSchema: &schema.Schema{
				Attributes: map[string]schema.Attribute{
					"mode": schema.StringAttribute{
						Optional: true,
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
					"machine_configuration": schema.StringAttribute{
						Required: true,
					},
					"config_patches": schema.ListAttribute{
						Optional:    true,
						ElementType: types.StringType,
					},
				},
			},
			StateUpgrader: func(ctx context.Context, req resource.UpgradeStateRequest, resp *resource.UpgradeStateResponse) {
				var priorStateData talosMachineConfigurationApplyResourceModelV0

				diags := req.State.Get(ctx, &priorStateData)
				resp.Diagnostics.Append(diags...)

				if diags.HasError() {
					return
				}

				var patches []string

				diags = append(diags, priorStateData.ConfigPatches.ElementsAs(ctx, &patches, true)...)
				resp.Diagnostics.Append(diags...)

				if diags.HasError() {
					return
				}

				configPatches := make([]basetypes.StringValue, len(patches))
				for i, patch := range patches {
					configPatches[i] = basetypes.NewStringValue(patch)
				}

				timeout, diag := basetypes.NewObjectValue(map[string]attr.Type{
					"create": types.StringType,
					"update": types.StringType,
				}, map[string]attr.Value{
					"create": basetypes.NewStringNull(),
					"update": basetypes.NewStringNull(),
				})
				resp.Diagnostics.Append(diag...)

				if resp.Diagnostics.HasError() {
					return
				}

				state := talosMachineConfigurationApplyResourceModelV1{
					ID:                        basetypes.NewStringValue("machine_configuration_apply"),
					ApplyMode:                 priorStateData.Mode,
					Node:                      priorStateData.Node,
					Endpoint:                  priorStateData.Endpoint,
					MachineConfigurationInput: priorStateData.MachineConfiguration,
					ConfigPatches:             configPatches,
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

func resetGetActorID(ctx context.Context, c *client.Client, req *machineapi.ResetRequest) (string, error) {
	resp, err := c.ResetGenericWithResponse(ctx, req)
	if err != nil {
		return "", err
	}

	if len(resp.GetMessages()) == 0 {
		return "", errors.New("no messages returned from action run")
	}

	return resp.GetMessages()[0].GetActorId(), nil
}

type clientExecutor struct {
	c     *client.Client
	nodes []string
}

func newClientExecutor(c *client.Client, nodes []string) *clientExecutor {
	return &clientExecutor{
		c:     c,
		nodes: nodes,
	}
}

func (c *clientExecutor) WithClient(action func(context.Context, *client.Client) error, _ ...grpc.DialOption) error {
	ctx := client.WithNodes(context.Background(), c.nodes...)

	return action(ctx, c.c)
}

func (c *clientExecutor) NodeList() []string {
	return c.nodes
}
