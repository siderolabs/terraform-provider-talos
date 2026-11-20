// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"maps"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// talosClusterStateV0_12 is talos_cluster state as written by v0.12.0, before the
// upgrade-k8s image attributes were added to the schema.
const talosClusterStateV0_12 = `{
  "id": "10.0.0.1",
  "node": "10.0.0.1",
  "endpoint": "10.0.0.1",
  "control_plane_nodes": ["10.0.0.1"],
  "kubernetes_version": "v1.35.4",
  "client_configuration": {
    "ca_certificate": "Zm9v",
    "client_certificate": "YmFy",
    "client_key": "YmF6"
  },
  "timeouts": null
}`

var talosClusterNewAttributes = []string{
	"kubelet_image",
	"apiserver_image",
	"controller_manager_image",
	"scheduler_image",
	"proxy_image",
	"pre_pull_images",
}

// TestTalosClusterSchemaCompatWithExistingState drives the provider's protocol
// server the way Terraform does on the first plan after a provider upgrade: the
// v0.12.0 state goes through UpgradeResourceState, then PlanResourceChange with a
// config that doesn't set any of the new attributes. The planned state must equal
// the prior state, i.e. no diff and no upgrade-k8s run.
func TestTalosClusterSchemaCompatWithExistingState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	server, err := providerserver.NewProtocol6WithError(talos.New())()
	if err != nil {
		t.Fatal(err)
	}

	schemaResp, err := server.GetProviderSchema(ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		t.Fatal(err)
	}

	assertNoDiagnostics(t, "GetProviderSchema", schemaResp.Diagnostics)

	resourceSchema := schemaResp.ResourceSchemas["talos_cluster"]
	objType := resourceSchema.ValueType()

	upgradeResp, err := server.UpgradeResourceState(ctx, &tfprotov6.UpgradeResourceStateRequest{
		TypeName: "talos_cluster",
		Version:  resourceSchema.Version,
		RawState: &tfprotov6.RawState{JSON: []byte(talosClusterStateV0_12)},
	})
	if err != nil {
		t.Fatal(err)
	}

	assertNoDiagnostics(t, "UpgradeResourceState", upgradeResp.Diagnostics)

	priorState, err := upgradeResp.UpgradedState.Unmarshal(objType)
	if err != nil {
		t.Fatal(err)
	}

	var priorAttrs map[string]tftypes.Value
	if err = priorState.As(&priorAttrs); err != nil {
		t.Fatal(err)
	}

	for _, name := range talosClusterNewAttributes {
		if !priorAttrs[name].IsNull() {
			t.Errorf("upgraded state: %s = %s, want null", name, priorAttrs[name])
		}
	}

	// The config is the one that produced the v0.12.0 state: the computed
	// attributes (id, endpoint, control_plane_nodes) and the new attributes are
	// not set.
	obj, ok := objType.(tftypes.Object)
	if !ok {
		t.Fatalf("talos_cluster schema type is %T, want tftypes.Object", objType)
	}

	configAttrs := maps.Clone(priorAttrs)

	for _, name := range []string{"id", "endpoint", "control_plane_nodes"} {
		configAttrs[name] = tftypes.NewValue(obj.AttributeTypes[name], nil)
	}

	config := tftypes.NewValue(objType, configAttrs)

	validateResp, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{
		TypeName: "talos_cluster",
		Config:   mustDynamicValue(t, objType, config),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertNoDiagnostics(t, "ValidateResourceConfig", validateResp.Diagnostics)

	// Terraform core proposes the config value for every non-computed attribute
	// and the prior value for unset computed ones, which here is the prior state.
	planResp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName:         "talos_cluster",
		PriorState:       upgradeResp.UpgradedState,
		ProposedNewState: upgradeResp.UpgradedState,
		Config:           mustDynamicValue(t, objType, config),
	})
	if err != nil {
		t.Fatal(err)
	}

	assertNoDiagnostics(t, "PlanResourceChange", planResp.Diagnostics)

	if len(planResp.RequiresReplace) > 0 {
		t.Errorf("RequiresReplace = %v, want none", planResp.RequiresReplace)
	}

	plannedState, err := planResp.PlannedState.Unmarshal(objType)
	if err != nil {
		t.Fatal(err)
	}

	if diffs, err := priorState.Diff(plannedState); err != nil {
		t.Fatal(err)
	} else if len(diffs) > 0 {
		for _, d := range diffs {
			t.Errorf("planned state differs from prior state at %s: %s -> %s", d.Path, d.Value1, d.Value2)
		}
	}
}

func mustDynamicValue(t *testing.T, typ tftypes.Type, v tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()

	dv, err := tfprotov6.NewDynamicValue(typ, v)
	if err != nil {
		t.Fatal(err)
	}

	return &dv
}

func assertNoDiagnostics(t *testing.T, call string, diags []*tfprotov6.Diagnostic) {
	t.Helper()

	for _, d := range diags {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s: %s", call, d.Summary, d.Detail)
		}
	}
}
