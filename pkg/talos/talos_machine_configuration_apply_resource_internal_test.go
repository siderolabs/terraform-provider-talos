// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

// TestTalosMachineConfigurationApplyModifyPlanConfigPatchesUnknownElement verifies
// that an unknown element in a known-length config_patches list defers the hash to
// apply instead of leaving the prior-state value (#388).
func TestTalosMachineConfigurationApplyModifyPlanConfigPatchesUnknownElement(t *testing.T) {
	ctx := context.Background()

	r := &talosMachineConfigurationApplyResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	objType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not tftypes.Object")
	}

	// build assembles a full object value from the schema type, defaulting every
	// attribute to null and applying the supplied overrides on top.
	build := func(overrides map[string]tftypes.Value) tftypes.Value {
		values := make(map[string]tftypes.Value, len(objType.AttributeTypes))

		for name, typ := range objType.AttributeTypes {
			values[name] = tftypes.NewValue(typ, nil)
		}

		maps.Copy(values, overrides)

		return tftypes.NewValue(objType, values)
	}

	str := func(s string) tftypes.Value {
		return tftypes.NewValue(tftypes.String, s)
	}

	// configPatches builds a known-length (2 element) list where the second
	// element is the supplied value — unknown in pass 1, resolved in pass 2.
	configPatches := func(second tftypes.Value) tftypes.Value {
		listType, ok := objType.AttributeTypes["config_patches"].(tftypes.List)
		if !ok {
			t.Fatal("config_patches type is not tftypes.List")
		}

		return tftypes.NewValue(listType, []tftypes.Value{
			str("machine:\n  network:\n    interfaces:\n      - interface: eth0\n        dhcp: false\n"),
			second,
		})
	}

	machineConfigInput := `version: v1alpha1
machine:
  network:
    hostname: hostname1
    interfaces:
      - interface: eth0
        dhcp: true
`

	configOverrides := func(patches tftypes.Value) map[string]tftypes.Value {
		return map[string]tftypes.Value{
			"node":                        str("10.0.0.1"),
			"endpoint":                    str("10.0.0.1"),
			"machine_configuration_input": str(machineConfigInput),
			"config_patches":              patches,
		}
	}

	// The plan carries a concrete machine_configuration_hash — what
	// UseStateForUnknown would have copied from prior state before ModifyPlan
	// runs. This is the precondition for the #388 inconsistency.
	plan := build(map[string]tftypes.Value{
		"machine_configuration_hash": str("old-hash"),
	})

	// Pass 1: config_patches has a known length (2) but element[1] is unknown.
	req1 := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Raw: build(configOverrides(configPatches(tftypes.NewValue(tftypes.String, tftypes.UnknownValue)))), Schema: schemaResp.Schema},
		State:  tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: schemaResp.Schema},
		Plan:   tfsdk.Plan{Raw: plan, Schema: schemaResp.Schema},
	}
	resp1 := resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Raw: req1.Plan.Raw, Schema: schemaResp.Schema},
	}
	r.ModifyPlan(ctx, req1, &resp1)

	if resp1.Diagnostics.HasError() {
		t.Fatalf("pass 1: unexpected diagnostics: %v", resp1.Diagnostics)
	}

	if value, unknown := planHash(ctx, t, resp1.Plan); !unknown {
		t.Fatalf("pass 1: expected hash to be unknown, got %q", value)
	}

	// Pass 2: element[1] resolves, so the hash is recomputed.
	req2 := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Raw: build(configOverrides(configPatches(str("machine:\n  network:\n    hostname: hostname2\n")))), Schema: schemaResp.Schema},
		State:  tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: schemaResp.Schema},
		Plan:   tfsdk.Plan{Raw: resp1.Plan.Raw, Schema: schemaResp.Schema},
	}
	resp2 := resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Raw: req2.Plan.Raw, Schema: schemaResp.Schema},
	}
	r.ModifyPlan(ctx, req2, &resp2)

	if resp2.Diagnostics.HasError() {
		t.Fatalf("pass 2: unexpected diagnostics: %v", resp2.Diagnostics)
	}

	if value, unknown := planHash(ctx, t, resp2.Plan); unknown || value == "old-hash" {
		t.Fatalf("pass 2: expected hash to be recomputed, got %q (unknown=%v)", value, unknown)
	}
}

func TestTalosMachineConfigurationApplyModifyPlanKnownConfigPatchesRecompute(t *testing.T) {
	ctx := context.Background()

	r := &talosMachineConfigurationApplyResource{}

	var schemaResp resource.SchemaResponse
	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	objType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema type is not tftypes.Object")
	}

	build := func(overrides map[string]tftypes.Value) tftypes.Value {
		values := make(map[string]tftypes.Value, len(objType.AttributeTypes))

		for name, typ := range objType.AttributeTypes {
			values[name] = tftypes.NewValue(typ, nil)
		}

		maps.Copy(values, overrides)

		return tftypes.NewValue(objType, values)
	}

	str := func(s string) tftypes.Value {
		return tftypes.NewValue(tftypes.String, s)
	}

	listType, ok := objType.AttributeTypes["config_patches"].(tftypes.List)
	if !ok {
		t.Fatal("config_patches type is not tftypes.List")
	}

	config := build(map[string]tftypes.Value{
		"node":                        str("10.0.0.1"),
		"endpoint":                    str("10.0.0.1"),
		"machine_configuration_input": str("version: v1alpha1\nmachine:\n  network:\n    hostname: hostname1\n    interfaces:\n      - interface: eth0\n        dhcp: true\n"),
		"config_patches": tftypes.NewValue(listType, []tftypes.Value{
			str("machine:\n  network:\n    hostname: hostname2\n"),
		}),
	})

	plan := build(map[string]tftypes.Value{
		"machine_configuration_hash": str("old-hash"),
	})

	req := resource.ModifyPlanRequest{
		Config: tfsdk.Config{Raw: config, Schema: schemaResp.Schema},
		State:  tfsdk.State{Raw: tftypes.NewValue(objType, nil), Schema: schemaResp.Schema},
		Plan:   tfsdk.Plan{Raw: plan, Schema: schemaResp.Schema},
	}
	resp := resource.ModifyPlanResponse{
		Plan: tfsdk.Plan{Raw: req.Plan.Raw, Schema: schemaResp.Schema},
	}
	r.ModifyPlan(ctx, req, &resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}

	if value, unknown := planHash(ctx, t, resp.Plan); unknown || value == "old-hash" {
		t.Fatalf("expected hash to change for known config_patches, got %q (unknown=%v)", value, unknown)
	}
}

func planHash(ctx context.Context, t *testing.T, plan tfsdk.Plan) (string, bool) {
	t.Helper()

	var hash types.String

	diags := plan.GetAttribute(ctx, path.Root("machine_configuration_hash"), &hash)
	if diags.HasError() {
		t.Fatalf("reading machine_configuration_hash: %v", diags)
	}

	return hash.ValueString(), hash.IsUnknown()
}

// TestGeneratePreservesEmptySearchDomains is the red-phase test for issue #414:
// a ResolverConfig patch clearing DHCP search domains with `domains: []` must
// survive rendering. With machinery v1.14.0 the empty list is dropped twice:
// merge treats an empty slice as zero (no `merge:"replace"` tag), and the
// encoder omits it (`omitempty` without `talos:"omitonlyifnil"`). The node
// then keeps the DHCPv4 search domain, defeating the patch. Fixed by bumping
// machinery to v1.14.1 (siderolabs/talos@20dcd515ab).
func TestGeneratePreservesEmptySearchDomains(t *testing.T) {
	secretsBundle, err := secrets.NewBundle(secrets.NewFixedClock(time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)), nil)
	if err != nil {
		t.Fatalf("failed to create secrets bundle: %v", err)
	}

	opts := &machineConfigGenerateOptions{
		machineSecrets:    secretsBundle,
		clusterName:       "issue-414",
		clusterEndpoint:   "https://10.0.0.1:6443",
		kubernetesVersion: "v1.34.0",
		talosVersion:      "v1.14.1",
		machineType:       machine.TypeControlPlane,
		configPatches: []string{`apiVersion: v1alpha1
kind: ResolverConfig
nameservers:
  - address: 100.100.100.100
searchDomains:
  domains: []
`},
	}

	rendered, err := opts.generate()
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	if !strings.Contains(rendered, "domains: []") {
		t.Errorf("rendered config dropped the empty search domain list; expected 'domains: []' in the ResolverConfig document.")
	}
}
