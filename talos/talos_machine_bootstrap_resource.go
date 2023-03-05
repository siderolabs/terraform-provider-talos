// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/retry"
	machineapi "github.com/siderolabs/talos/pkg/machinery/api/machine"
	"github.com/siderolabs/talos/pkg/machinery/client"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type talosMachineBootstrapResource struct{}

type talosMachineBootstrapResourceModel struct {
	node     string
	endpoint string
	talosClientConfig
}

func NewTalosMachineBootstrapResource() tfprotov6.ResourceServer {
	return &talosMachineBootstrapResource{}
}

func (p *talosMachineBootstrapResource) ValidateResourceConfig(ctx context.Context, req *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
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

func (p *talosMachineBootstrapResource) UpgradeResourceState(ctx context.Context, req *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	resp := &tfprotov6.UpgradeResourceStateResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	val, err := req.RawState.Unmarshal(machineBootstrapSchemaObject())
	if err != nil {
		return nil, err
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		return nil, err
	}
	state, err := tfprotov6.NewDynamicValue(machineBootstrapSchemaObject(), tftypes.NewValue(machineBootstrapSchemaObject(), valMap))
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

func (p *talosMachineBootstrapResource) ReadResource(ctx context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	resp := &tfprotov6.ReadResourceResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	resp.NewState = req.CurrentState

	return resp, nil
}

func (p *talosMachineBootstrapResource) PlanResourceChange(ctx context.Context, req *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	resp := &tfprotov6.PlanResourceChangeResponse{
		Diagnostics:  []*tfprotov6.Diagnostic{},
		PlannedState: req.Config,
	}

	val, err := req.ProposedNewState.Unmarshal(machineBootstrapSchemaObject())
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
		"id":                   valMap["id"],
		"node":                 valMap["node"],
		"endpoint":             valMap["endpoint"],
		"client_configuration": valMap["client_configuration"],
	}

	if valMap["id"].IsNull() {
		stateValues["id"] = tftypes.NewValue(tftypes.String, tftypes.UnknownValue)
	}

	// If endpoint is not set, default to node
	if valMap["endpoint"].IsNull() {
		stateValues["endpoint"] = valMap["node"]
	}

	state, err := tfprotov6.NewDynamicValue(machineBootstrapSchemaObject(), tftypes.NewValue(machineBootstrapSchemaObject(), stateValues))
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

func (p *talosMachineBootstrapResource) ApplyResourceChange(ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	resp := &tfprotov6.ApplyResourceChangeResponse{
		Diagnostics: []*tfprotov6.Diagnostic{},
	}

	val, err := req.PlannedState.Unmarshal(machineBootstrapSchemaObject())
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
	priorVal, err := req.PriorState.Unmarshal(machineBootstrapSchemaObject())
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
			if err := c.Bootstrap(opFuncCtx, &machineapi.BootstrapRequest{}); err != nil {
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
		"id":                   tftypes.NewValue(tftypes.String, "machine_bootstrap"),
		"node":                 valMap["node"],
		"endpoint":             valMap["endpoint"],
		"client_configuration": valMap["client_configuration"],
	}

	state, err := tfprotov6.NewDynamicValue(machineBootstrapSchemaObject(), tftypes.NewValue(machineBootstrapSchemaObject(), stateValues))
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

func (p *talosMachineBootstrapResource) ImportResourceState(ctx context.Context, req *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	return nil, nil
}

func (p *talosMachineBootstrapResource) readConfigValues(config *tfprotov6.DynamicValue) (talosMachineBootstrapResourceModel, error) {
	var model talosMachineBootstrapResourceModel

	val, err := config.Unmarshal(machineBootstrapSchemaObject())
	if err != nil {
		return model, err
	}

	var valMap map[string]tftypes.Value

	if err := val.As(&valMap); err != nil {
		return model, err
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

func machineBootstrapSchemaObject() tftypes.Object {
	return tftypes.Object{
		AttributeTypes: map[string]tftypes.Type{
			"id":                   tftypes.String,
			"node":                 tftypes.String,
			"endpoint":             tftypes.String,
			"client_configuration": clientConfgurationSchemaObject(),
		},
	}
}
