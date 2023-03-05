// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	"github.com/siderolabs/gen/slices"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"github.com/siderolabs/talos/pkg/machinery/config/configpatcher"
	"github.com/siderolabs/terraform-provider-talos/talos/internal/tfutils"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gopkg.in/yaml.v3"
)

var (
	defaultCreateTimeout = "10m"
	defaultUpdateTimeout = "10m"
	defaultDeleteTimeout = "10m"
)

type talosMachineConfigurationApplyResource struct{}

type talosMachineConfigurationApplyResourceModel struct {
	mode     string
	node     string
	endpoint string
	talosClientConfig
	machineConfiguration string
	configPatches        []string

	// clientConfigurationType map[string]tftypes.Value
	// configPatchesType       []tftypes.Value
}

type talosClientConfig struct {
	ca  string
	crt string
	key string
}

func NewTalosMachineConfigurationApplyResource() tfprotov6.ResourceServer {
	return &talosMachineConfigurationApplyResource{}
}

func (p *talosMachineConfigurationApplyResource) ValidateResourceConfig(ctx context.Context, req *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
	resp := &tfprotov6.ValidateResourceConfigResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	if _, err := p.readConfigValues(req.Config); err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error unmarshaling config",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	return resp, nil
}

func (p *talosMachineConfigurationApplyResource) UpgradeResourceState(ctx context.Context, req *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	resp := &tfprotov6.UpgradeResourceStateResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	val, err := req.RawState.Unmarshal(machineConfigurationApplySchemaObject())
	if err != nil {
		return nil, err
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		return nil, err
	}
	state, err := tfprotov6.NewDynamicValue(machineConfigurationApplySchemaObject(), tftypes.NewValue(machineConfigurationApplySchemaObject(), valMap))
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error generating plan state",
			Detail:   err.Error(),
		})

		return resp, nil
	}
	resp.UpgradedState = &state

	return resp, nil
}

func (p *talosMachineConfigurationApplyResource) ReadResource(ctx context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	resp := &tfprotov6.ReadResourceResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	resp.NewState = req.CurrentState

	return resp, nil
}

func (p *talosMachineConfigurationApplyResource) PlanResourceChange(ctx context.Context, req *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	resp := &tfprotov6.PlanResourceChangeResponse{
		Diagnostics:  []*tfprotov6.Diagnostic{},
		PlannedState: req.Config,
	}

	val, err := req.ProposedNewState.Unmarshal(machineConfigurationApplySchemaObject())
	if err != nil {
		return nil, err
	}

	// delete operation
	if val.IsNull() {
		resp.PlannedState = req.ProposedNewState

		return resp, nil
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		return nil, err
	}

	stateValues := map[string]tftypes.Value{
		"id":                          valMap["id"],
		"mode":                        valMap["mode"],
		"node":                        valMap["node"],
		"endpoint":                    valMap["endpoint"],
		"client_configuration":        valMap["client_configuration"],
		"machine_configuration":       valMap["machine_configuration"],
		"machine_configuration_final": valMap["machine_configuration_final"],
		"config_patches":              valMap["config_patches"],
	}

	if valMap["id"].IsNull() {
		stateValues["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}

	if valMap["machine_configuration_final"].IsNull() {
		stateValues["machine_configuration_final"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}

	// If mode is not set, default to auto
	if valMap["mode"].IsNull() {
		stateValues["mode"] = tftypes.NewValue(tftypes.String, "auto")
	}

	// If endpoint is not set, default to node
	if valMap["endpoint"].IsNull() {
		stateValues["endpoint"] = valMap["node"]
	}

	state, err := tfprotov6.NewDynamicValue(machineConfigurationApplySchemaObject(), tftypes.NewValue(machineConfigurationApplySchemaObject(), stateValues))
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error generating plan state",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	resp.PlannedState = &state

	return resp, nil
}

func (p *talosMachineConfigurationApplyResource) ApplyResourceChange(ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	resp := &tfprotov6.ApplyResourceChangeResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	val, err := req.PlannedState.Unmarshal(machineConfigurationApplySchemaObject())
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error unmarshaling config",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	if val.IsNull() {
		// delete operation
		resp.NewState = req.PlannedState

		return resp, nil
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error unmarshaling config into tftypes.Value",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	var priorState map[string]tftypes.Value
	priorVal, err := req.PriorState.Unmarshal(machineConfigurationApplySchemaObject())
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error unmarshaling prior config",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	if err := priorVal.As(&priorState); err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error unmarshaling prior config into tftypes.Value",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	timeouts := getTimeouts(valMap)
	var timeout time.Duration
	if priorVal.IsNull() {
		timeout, _ = time.ParseDuration(timeouts["create"])

	} else {
		timeout, _ = time.ParseDuration(timeouts["update"])
	}

	deadline := time.Now().Add(timeout)
	ctxDeadline, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	vals, err := p.readConfigValues(req.PlannedState)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error reading config values",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	patches, err := configpatcher.LoadPatches(vals.configPatches)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error loading config patches",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	cfg, err := configpatcher.Apply(configpatcher.WithBytes([]byte(vals.machineConfiguration)), patches)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error applying config patches",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	cfgBytes, err := cfg.Bytes()
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error converting config to bytes",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	talosConfig, err := talosClientTFConfigToTalosClientConfig(
		"dynamic",
		vals.ca,
		vals.crt,
		vals.key,
	)
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error converting config to talos client config",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	if err := retry.RetryContext(ctxDeadline, timeout, func() *retry.RetryError {
		if err := talosClientOp(ctx, vals.endpoint, vals.node, talosConfig, func(opFuncCtx context.Context, c *client.Client) error {
			_, err := c.ApplyConfiguration(opFuncCtx, &machineapi.ApplyConfigurationRequest{
				Mode: machineapi.ApplyConfigurationRequest_Mode(machineapi.ApplyConfigurationRequest_Mode_value[strings.ToUpper(vals.mode)]),
				Data: cfgBytes,
			})
			if err != nil {
				return err
			}

			return nil
		}); err != nil {
			if s := status.Code(err); s == codes.InvalidArgument || s == codes.Unavailable {
				return retry.NonRetryableError(err)
			}

			return retry.RetryableError(err)
		}

		return nil
	}); err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error applying configuration",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	stateValues := map[string]tftypes.Value{
		"id":                          tftypes.NewValue(tftypes.String, "machine_configuration_apply"),
		"mode":                        valMap["mode"],
		"node":                        valMap["node"],
		"endpoint":                    valMap["endpoint"],
		"client_configuration":        valMap["client_configuration"],
		"machine_configuration":       valMap["machine_configuration"],
		"machine_configuration_final": tftypes.NewValue(tftypes.String, string(cfgBytes)),
		"config_patches":              valMap["config_patches"],
	}

	state, err := tfprotov6.NewDynamicValue(machineConfigurationApplySchemaObject(), tftypes.NewValue(machineConfigurationApplySchemaObject(), stateValues))
	if err != nil {
		resp.Diagnostics = append(resp.Diagnostics, &tfprotov6.Diagnostic{
			Severity: tfprotov6.DiagnosticSeverityError,
			Summary:  "Error saving state",
			Detail:   err.Error(),
		})

		return resp, nil
	}

	resp.NewState = &state

	return resp, nil
}

func (p *talosMachineConfigurationApplyResource) ImportResourceState(ctx context.Context, req *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	return nil, nil
}

func (p *talosMachineConfigurationApplyResource) readConfigValues(config *tfprotov6.DynamicValue) (talosMachineConfigurationApplyResourceModel, error) {
	var model talosMachineConfigurationApplyResourceModel

	val, err := config.Unmarshal(machineConfigurationApplySchemaObject())
	if err != nil {
		return model, err
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		return model, err
	}

	if !valMap["mode"].IsNull() && valMap["mode"].IsKnown() {
		if err := valMap["mode"].As(&model.mode); err != nil {
			return model, err
		}

		if !slices.Contains([]string{"auto", "no_reboot", "reboot", "staged"}, func(s string) bool {
			return s == model.mode
		}) {
			return model, fmt.Errorf("mode must be one of auto, no_reboot, reboot, staged")
		}
	}

	if !valMap["node"].IsNull() && valMap["node"].IsKnown() {
		if err := valMap["node"].As(&model.node); err != nil {
			return model, err
		}

		if model.node == "" {
			return model, fmt.Errorf("node must be set")
		}
	}

	if !valMap["endpoint"].IsNull() && valMap["endpoint"].IsKnown() {
		if err := valMap["endpoint"].As(&model.endpoint); err != nil {
			return model, err
		}
	}

	// if endpoint is not set, and node value is set, use node value as endpoint
	if model.endpoint == "" && model.node != "" {
		model.endpoint = model.node
	}

	if !valMap["machine_configuration"].IsNull() && valMap["machine_configuration"].IsKnown() {
		if err := valMap["machine_configuration"].As(&model.machineConfiguration); err != nil {
			return model, err
		}
	}

	if !valMap["config_patches"].IsNull() && valMap["config_patches"].IsFullyKnown() {
		var configPatchesType []tftypes.Value
		if err := valMap["config_patches"].As(&configPatchesType); err != nil {
			return model, err
		}

		for _, configPatch := range configPatchesType {
			intf, err := tfutils.TFTypesToInterface(configPatch, tftypes.NewAttributePath())
			if err != nil {
				return model, err
			}

			patchBytes, err := yaml.Marshal(intf)
			if err != nil {
				return model, err
			}

			model.configPatches = append(model.configPatches, string(patchBytes))
		}

	}

	if !valMap["client_configuration"].IsNull() && valMap["client_configuration"].IsFullyKnown() {
		var clientConfigurationType map[string]tftypes.Value
		if err := valMap["client_configuration"].As(&clientConfigurationType); err != nil {
			return model, err
		}

		if !clientConfigurationType["ca_certificate"].IsNull() && clientConfigurationType["ca_certificate"].IsKnown() {
			var caCertificate string
			if err := clientConfigurationType["ca_certificate"].As(&caCertificate); err != nil {
				return model, err
			}
			model.talosClientConfig.ca = caCertificate
		}

		if !clientConfigurationType["client_certificate"].IsNull() && clientConfigurationType["client_certificate"].IsKnown() {
			var clientCertificate string
			if err := clientConfigurationType["client_certificate"].As(&clientCertificate); err != nil {
				return model, err
			}
			model.talosClientConfig.crt = clientCertificate
		}

		if !clientConfigurationType["client_key"].IsNull() && clientConfigurationType["client_key"].IsKnown() {
			var clientKey string
			if err := clientConfigurationType["client_key"].As(&clientKey); err != nil {
				return model, err
			}
			model.talosClientConfig.key = clientKey
		}
	}

	return model, nil
}

func machineConfigurationApplySchemaObject() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                          tftypes.String,
			"mode":                        tftypes.String,
			"node":                        tftypes.String,
			"endpoint":                    tftypes.String,
			"client_configuration":        clientConfgurationSchemaObject(),
			"machine_configuration":       tftypes.String,
			"machine_configuration_final": tftypes.String,
			"config_patches":              tftypes.List{ElementType: tftypes.DynamicPseudoType},
		},
	}
}

func clientConfgurationSchemaObject() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"ca_certificate":     tftypes.String,
			"client_certificate": tftypes.String,
			"client_key":         tftypes.String,
		},
	}
}

func getTimeouts(v map[string]tftypes.Value) map[string]string {
	timeouts := map[string]string{
		"create": defaultCreateTimeout,
		"update": defaultUpdateTimeout,
		"delete": defaultDeleteTimeout,
	}
	if !v["timeouts"].IsNull() && v["timeouts"].IsKnown() {
		var timeoutsBlock []tftypes.Value
		v["timeouts"].As(&timeoutsBlock)
		if len(timeoutsBlock) > 0 {
			var t map[string]tftypes.Value
			timeoutsBlock[0].As(&t)
			var s string
			for _, k := range []string{"create", "update", "delete"} {
				if vv, ok := t[k]; ok && !vv.IsNull() {
					vv.As(&s)
					if s != "" {
						timeouts[k] = s
					}
				}
			}
		}
	}
	return timeouts
}
