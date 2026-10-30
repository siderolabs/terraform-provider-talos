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

	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
)

// testDigest is a fixture SHA-256 digest reused by tests covering digest-pinned
// image references.
const testDigest = "sha256:b9a485250c4d1a3f6d3b1c1b0a1f6a1c1e1a1b1c1d1e1f1a1b1c1d1e1f1a1b1c"

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

// TestReplaceImageTag is the red-phase test for
// https://github.com/siderolabs/terraform-provider-talos/issues/393: for a
// digest-pinned reference, replacing everything after the last ':' also eats
// into the "sha256:<hex>" digest, producing a corrupted reference such as
// "repo@sha256:v1.13.9". That corrupted value gets written back into
// talos_machine's state on every Read (and compared against the desired image
// on every Update), so digest-pinned images never reach a stable plan.
func TestReplaceImageTag(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		imageRef string
		newTag   string
		want     string
	}{
		"tag reference": {
			imageRef: "ghcr.io/siderolabs/installer:v1.13.0",
			newTag:   "v1.13.9",
			want:     "ghcr.io/siderolabs/installer:v1.13.9",
		},
		"no tag": {
			imageRef: "ghcr.io/siderolabs/installer",
			newTag:   "v1.13.9",
			want:     "ghcr.io/siderolabs/installer:v1.13.9",
		},
		"digest reference is left untouched": {
			imageRef: "ghcr.io/siderolabs/installer@" + testDigest,
			newTag:   "v1.13.9",
			want:     "ghcr.io/siderolabs/installer@" + testDigest,
		},
		"registry host:port with no tag": {
			imageRef: "registry.example.com:5000/siderolabs/installer",
			newTag:   "v1.13.9",
			want:     "registry.example.com:5000/siderolabs/installer:v1.13.9",
		},
		"registry host:port with a tag": {
			imageRef: "registry.example.com:5000/siderolabs/installer:v1.13.0",
			newTag:   "v1.13.9",
			want:     "registry.example.com:5000/siderolabs/installer:v1.13.9",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := replaceImageTag(test.imageRef, test.newTag); got != test.want {
				t.Fatalf("replaceImageTag(%q, %q) = %q, want %q", test.imageRef, test.newTag, got, test.want)
			}
		})
	}
}

// TestTalosMachineReconcileRunningImage is the red-phase test for the Create-time
// regression the naive digest fix introduced: if a digest-pinned desiredImage were
// compared using replaceImageTag directly, it would always equal desiredImage
// itself, so talosMachineUpgradeIfNeeded would conclude the node already runs the
// desired image and skip installing it — even on a freshly provisioned node that
// has never run it. talosMachineReconcileRunningImage must return "" for digest
// pins so the caller always installs instead.
func TestTalosMachineReconcileRunningImage(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		desiredImage string
		reportedTag  string
		want         string
	}{
		"tag reference reflects the reported version": {
			desiredImage: "ghcr.io/siderolabs/installer:v1.13.0",
			reportedTag:  "v1.13.9",
			want:         "ghcr.io/siderolabs/installer:v1.13.9",
		},
		"digest reference never matches, forcing install": {
			desiredImage: "ghcr.io/siderolabs/installer@" + testDigest,
			reportedTag:  "v1.13.9",
			want:         "",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := talosMachineReconcileRunningImage(test.desiredImage, test.reportedTag); got != test.want {
				t.Fatalf("talosMachineReconcileRunningImage(%q, %q) = %q, want %q",
					test.desiredImage, test.reportedTag, got, test.want)
			}
		})
	}
}

func TestTalosMachineImageFactorySchematic(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		imageRef  string
		schematic string
		ok        bool
	}{
		"bare installer": {
			imageRef:  "factory.talos.dev/installer/abc123:v1.13.0",
			schematic: "abc123",
			ok:        true,
		},
		"bare secure boot installer": {
			imageRef:  "factory.talos.dev/installer-secureboot/abc123:v1.13.0",
			schematic: "abc123",
			ok:        true,
		},
		"default factory": {
			imageRef:  "factory.talos.dev/metal-installer/abc123:v1.13.0",
			schematic: "abc123",
			ok:        true,
		},
		"custom registry": {
			imageRef:  "images.example.com/talos/openstack-installer/def456:v1.13.0",
			schematic: "def456",
			ok:        true,
		},
		"registry with port and secure boot": {
			imageRef:  "images.example.com:5000/metal-installer-secureboot/789abc:v1.13.0",
			schematic: "789abc",
			ok:        true,
		},
		"standard installer": {
			imageRef: "ghcr.io/siderolabs/installer:v1.13.0",
		},
		"factory artifact": {
			imageRef: "factory.talos.dev/image/abc123/v1.13.0/metal-amd64.raw.xz",
		},
		"digest": {
			imageRef: "factory.talos.dev/metal-installer/abc123@sha256:1234",
		},
		"empty schematic": {
			imageRef: "factory.talos.dev/metal-installer/:v1.13.0",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schematic, ok := talosMachineImageFactorySchematic(test.imageRef)
			if schematic != test.schematic || ok != test.ok {
				t.Fatalf("talosMachineImageFactorySchematic(%q) = (%q, %t), want (%q, %t)",
					test.imageRef, schematic, ok, test.schematic, test.ok)
			}
		})
	}
}

// TestTalosMachineImageManaged covers
// https://github.com/siderolabs/terraform-provider-talos/issues/420: an omitted
// image plans as unknown on create (Optional+Computed, nothing for
// UseStateForUnknown to pin), and Create/Update must treat that the same as null
// ("OS version is not managed") instead of running the upgrade path with an
// empty image reference.
func TestTalosMachineImageManaged(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name  string
		image types.String
		want  bool
	}{
		{name: "null", image: types.StringNull(), want: false},
		{name: "unknown", image: types.StringUnknown(), want: false},
		{name: "known", image: types.StringValue("ghcr.io/siderolabs/installer:v1.13.0"), want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := talosMachineImageManaged(tc.image); got != tc.want {
				t.Fatalf("talosMachineImageManaged(%s) = %v, want %v", tc.image, got, tc.want)
			}
		})
	}
}
