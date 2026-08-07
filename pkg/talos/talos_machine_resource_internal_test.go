// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // needs access to unexported talosMachineUpgradeLegacy and clientOpFunc

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
)

// TestLegacyUpgrade_ImmediateRPCError_AbortsPollEarly calls the real
// talosMachineUpgradeLegacy with a mock op that returns an error immediately.
// It verifies the function returns well within the 5-second context deadline
// with "upgrade RPC failed:" in the message, proving the goroutine error is
// propagated through rpcErrCh to abort the poll rather than being discarded.
func TestLegacyUpgrade_ImmediateRPCError_AbortsPollEarly(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rpcErr := errors.New("authentication failed")

	// Mock that always fails immediately — covers both the goroutine RPC call
	// and the poll loop's version check.
	failImmediately := clientOpFunc(func(_ context.Context, _, _ string, _ *clientconfig.Config, _ func(context.Context, *client.Client) error) error {
		return rpcErr
	})

	state := &talosMachineResourceModel{
		Image:      types.StringValue("ghcr.io/siderolabs/installer:v1.13.0"),
		RebootMode: types.StringValue("DEFAULT"),
	}

	err := talosMachineUpgradeLegacy(ctx, "10.0.0.1", "10.0.0.1", nil, state, "DEFAULT", failImmediately)
	if err == nil {
		t.Fatal("expected error from immediate RPC failure, got nil")
	}

	// The "upgrade RPC failed:" prefix is injected by the channel-based early-abort
	// path. Its presence proves the goroutine error reached the poll loop.
	if !strings.Contains(err.Error(), "upgrade RPC failed:") {
		t.Fatalf("expected 'upgrade RPC failed:' in error message, got: %v", err)
	}
}

func TestTalosMachineResource_ImportState(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	r := &talosMachineResource{}

	var schemaResp resource.SchemaResponse

	r.Schema(ctx, resource.SchemaRequest{}, &schemaResp)

	schTFType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("schema TerraformType is not tftypes.Object")
	}

	t.Run("sets id node and endpoint from import id", func(t *testing.T) {
		t.Parallel()

		importResp := &resource.ImportStateResponse{
			State: tfsdk.State{
				Schema: schemaResp.Schema,
				Raw:    tftypes.NewValue(schTFType, nil),
			},
		}

		r.ImportState(ctx, resource.ImportStateRequest{ID: "10.5.0.2"}, importResp)

		if importResp.Diagnostics.HasError() {
			t.Fatalf("unexpected diagnostics: %v", importResp.Diagnostics)
		}

		var state talosMachineResourceModel

		diags := importResp.State.Get(ctx, &state)
		if diags.HasError() {
			t.Fatalf("failed to read imported state: %v", diags)
		}

		if state.ID.ValueString() != "10.5.0.2" {
			t.Fatalf("expected id 10.5.0.2, got %q", state.ID.ValueString())
		}

		if state.Node.ValueString() != "10.5.0.2" {
			t.Fatalf("expected node 10.5.0.2, got %q", state.Node.ValueString())
		}

		if state.Endpoint.ValueString() != "10.5.0.2" {
			t.Fatalf("expected endpoint 10.5.0.2, got %q", state.Endpoint.ValueString())
		}

		if !state.Image.IsNull() {
			t.Fatalf("expected image null after import (post-import adopt signal), got %v", state.Image)
		}

		if !state.MachineConfigurationHash.IsNull() && state.MachineConfigurationHash.ValueString() != "" {
			t.Fatalf("expected empty machine_configuration_hash after import, got %q", state.MachineConfigurationHash.ValueString())
		}

		if !state.ClientConfigurationWO.IsNull() {
			t.Fatalf("expected client_configuration_wo null after import, got %v", state.ClientConfigurationWO)
		}
	})

	t.Run("rejects empty import id", func(t *testing.T) {
		t.Parallel()

		importResp := &resource.ImportStateResponse{
			State: tfsdk.State{
				Schema: schemaResp.Schema,
				Raw:    tftypes.NewValue(schTFType, nil),
			},
		}

		r.ImportState(ctx, resource.ImportStateRequest{ID: "  "}, importResp)

		if !importResp.Diagnostics.HasError() {
			t.Fatal("expected error for empty import id")
		}
	})
}

// TestReplaceImageTag_PreservesRepository documents why Update must not use
// UpgradeIfNeeded for non-import image changes: running version is reconstructed
// as desiredRepo:runningTag, so same tag / different schematic compares equal.
func TestReplaceImageTag_PreservesRepository(t *testing.T) {
	t.Parallel()

	desired := "factory.talos.dev/installer/c9078f94abcdef:v1.13.0"
	running := replaceImageTag(desired, "v1.13.0")

	if running != desired {
		t.Fatalf("replaceImageTag should keep desired repository, got %q want %q", running, desired)
	}

	otherSchematic := "factory.talos.dev/installer/376567abcdef:v1.13.0"
	runningFromOther := replaceImageTag(otherSchematic, "v1.13.0")

	if runningFromOther != otherSchematic {
		t.Fatalf("got %q want %q", runningFromOther, otherSchematic)
	}

	// UpgradeIfNeeded would treat these as equal when the node reports tag v1.13.0
	// while desired is otherSchematic — both become otherSchematic:v1.13.0.
	if replaceImageTag(otherSchematic, "v1.13.0") != otherSchematic {
		t.Fatal("unexpected replaceImageTag behavior")
	}
}
