// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // exercises the unexported ipAddressValidator

import (
	"context"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// TestIPAddressValidator covers the node/control_plane_nodes constraint from issue #382:
// a hostname used to reach cluster.IPsToNodeInfos and fail mid-apply with a bare
// ParseAddr error, so this must be rejected at plan time instead.
func TestIPAddressValidator(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		value   types.String
		wantErr bool
	}{
		{"IPv4", types.StringValue("192.168.1.10"), false},
		{"IPv6", types.StringValue("2001:db8::1"), false},
		{"hostname", types.StringValue("host.example.com"), true},
		{"bare hostname", types.StringValue("node1"), true},
		{"URL", types.StringValue("https://192.168.1.10"), true},
		{"IP with port", types.StringValue("192.168.1.10:50000"), true},
		{"empty", types.StringValue(""), true},
		// null/unknown are the caller's business, not the validator's
		{"null", types.StringNull(), false},
		{"unknown", types.StringUnknown(), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			resp := &validator.StringResponse{}
			ipAddressValid().ValidateString(
				context.Background(),
				validator.StringRequest{
					Path:        path.Root("node"),
					ConfigValue: tc.value,
				},
				resp,
			)

			if got := resp.Diagnostics.HasError(); got != tc.wantErr {
				t.Fatalf("HasError() = %v, want %v (diags: %v)", got, tc.wantErr, resp.Diagnostics)
			}

			if tc.wantErr {
				// The bare ParseAddr message is what made #382 hard to act on, so the
				// diagnostic must say what is wrong and where.
				detail := resp.Diagnostics.Errors()[0].Detail()
				if !strings.Contains(detail, "not an IP address") {
					t.Errorf("detail does not explain the problem: %q", detail)
				}

				if !strings.Contains(detail, "endpoint") {
					t.Errorf("detail does not point at the hostname-friendly alternative: %q", detail)
				}
			}
		})
	}
}

// TestNewClusterNodesRejectsHostname covers the apply-time half of issue #382. Attribute
// validators only fire when the value is known at plan time, and node addresses usually
// come from another resource, so this path has to give a usable error of its own rather
// than a bare ParseAddr failure.
func TestNewClusterNodesRejectsHostname(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name         string
		wantIn       string
		controlPlane []string
		workers      []string
	}{
		{"control plane hostname", "control plane nodes must be IP addresses", []string{"host.example.com"}, nil},
		{"worker hostname", "worker nodes must be IP addresses", []string{"10.0.0.1"}, []string{"worker.example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := newClusterNodes(tc.controlPlane, tc.workers)
			if err == nil {
				t.Fatal("expected an error for a hostname, got nil")
			}

			if !strings.Contains(err.Error(), tc.wantIn) {
				t.Errorf("error does not name the field or explain the rule: %v", err)
			}

			// the underlying cause should still be visible
			if !strings.Contains(err.Error(), "ParseAddr") {
				t.Errorf("error loses the underlying cause: %v", err)
			}
		})
	}
}

// TestNewClusterNodesAcceptsIPs guards against the wrapping above rejecting valid input.
func TestNewClusterNodesAcceptsIPs(t *testing.T) {
	t.Parallel()

	nodes, err := newClusterNodes([]string{"10.0.0.1", "2001:db8::1"}, []string{"10.0.0.2"})
	if err != nil {
		t.Fatalf("valid IPs rejected: %v", err)
	}

	if got := len(nodes.Nodes()); got != 3 {
		t.Errorf("got %d nodes, want 3", got)
	}
}
