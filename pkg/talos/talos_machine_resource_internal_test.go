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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
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

// TestTalosMachineDrainNode_HonorsConfiguredTimeout is the red-phase test for
// https://github.com/siderolabs/terraform-provider-talos/issues/411: the drain step
// of drain_on_upgrade hardcoded nodedrain.Options{}, whose zero DrainTimeout always
// falls back to go-kubernetes' 5-minute DefaultDrainTimeout. Raising timeouts.update
// never helped, because that budget is independent of — and always at least as long
// as — this inner cap. A drain blocked on a PodDisruptionBudget (e.g. Longhorn
// relocating volume replicas) was killed at 5 minutes no matter what the user set.
//
// This test blocks eviction forever (every eviction request returns 429 Too Many
// Requests) and asks for a 200ms drain timeout. If that value is ignored in favor of
// the hardcoded 5-minute default, the call will not return before the test's own
// 2-second bound and the test fails; only a caller that actually threads the
// configured duration into nodedrain.Options.DrainTimeout returns in time.
func TestTalosMachineDrainNode_HonorsConfiguredTimeout(t *testing.T) {
	t.Parallel()

	cs := fake.NewSimpleClientset(
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node1"}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "pod1", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "node1"},
		},
	)

	// Advertise eviction subresource support so kubectl's drain helper attempts a
	// real eviction instead of falling back to a plain delete, which would ignore
	// PodDisruptionBudgets and succeed immediately, defeating this test.
	cs.Resources = []*metav1.APIResourceList{
		{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				{Name: "pods/eviction", Kind: "Eviction", Group: "policy", Version: "v1"},
			},
		},
	}

	cs.PrependReactor("create", "pods", func(action ktesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "eviction" {
			return false, nil, nil
		}

		return true, nil, apierrors.NewTooManyRequests("blocked by PodDisruptionBudget", 1)
	})

	const (
		configuredTimeout = 200 * time.Millisecond
		testBound         = 2 * time.Second
	)

	// Bounded so that if this test ever regresses (the configured timeout ignored in
	// favor of nodedrain's hardcoded 5m default), the goroutine is canceled at the same
	// point the test gives up, instead of retrying evictions for up to 5 more minutes
	// after the test has already failed.
	ctx, cancel := context.WithTimeout(context.Background(), testBound)
	defer cancel()

	errCh := make(chan error, 1)

	go func() {
		errCh <- talosMachineDrainNode(ctx, cs, "node1", configuredTimeout)
	}()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected drain to fail once the configured timeout elapsed, got nil")
		}
	case <-time.After(testBound):
		t.Fatalf("drain did not return within %s; the configured %s drain_timeout was not honored "+
			"(still waiting on the hardcoded 5m default)", testBound, configuredTimeout)
	}
}

// TestParseDrainTimeout is the red-phase test for a code-review finding: the call site
// in talosMachineUpgrade parsed drain_timeout with `_ := time.ParseDuration(...)
// //nolint:errcheck`, silently discarding any parse failure and falling back to
// nodedrain's hardcoded 5m default instead of surfacing an error — diverging from this
// codebase's own convention for the same kind of field (see
// talos_cluster_kubeconfig_resource.go's CertificateRenewalDuration parse, which reports
// via resp.Diagnostics.AddError instead of swallowing the error).
func TestParseDrainTimeout(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		raw     string
		want    time.Duration
		wantErr bool
	}{
		{name: "valid", raw: "10m", want: 10 * time.Minute},
		{name: "invalid", raw: "not-a-duration", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseDrainTimeout(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDrainTimeout(%q) = nil error, want error", tc.raw)
				}

				if !strings.Contains(err.Error(), "drain_timeout") {
					t.Errorf("error does not name drain_timeout: %v", err)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseDrainTimeout(%q) unexpected error: %v", tc.raw, err)
			}

			if got != tc.want {
				t.Errorf("parseDrainTimeout(%q) = %v, want %v", tc.raw, got, tc.want)
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
