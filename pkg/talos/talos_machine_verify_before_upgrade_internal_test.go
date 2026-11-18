// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos //nolint:testpackage // needs access to unexported talosMachineVerifyBeforeUpgrade

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

// A node can already run the planned image when Terraform never saw it happen:
// someone ran `talosctl upgrade`, and Read cannot refresh state (write-only
// client_configuration_wo, or an unreachable node). State then keeps the old tag
// while the manifest is bumped to the version the node already runs. Update must
// check the node first instead of upgrading, and so rebooting, it again.
func TestVerifyBeforeUpgrade(t *testing.T) {
	t.Parallel()

	const (
		factory      = "factory.talos.dev/opennebula-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
		otherFactory = "factory.talos.dev/opennebula-installer/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		vanilla      = "ghcr.io/siderolabs/installer"
		digest       = "@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	)

	for _, tc := range []struct {
		name  string
		state types.String
		plan  types.String
		want  bool
	}{
		{
			// The plan from a real cluster: state holds v1.13.4 while the node already runs v1.13.12.
			name:  "only the tag changed on a Factory image",
			state: types.StringValue(factory + ":v1.13.4"),
			plan:  types.StringValue(factory + ":v1.13.12"),
			want:  true,
		},
		{
			name:  "only the tag changed on a vanilla installer",
			state: types.StringValue(vanilla + ":v1.13.4"),
			plan:  types.StringValue(vanilla + ":v1.13.12"),
			want:  true,
		},
		{
			name:  "only the tag changed on a registry with a port",
			state: types.StringValue("registry.example.com:5000/installer:v1.13.4"),
			plan:  types.StringValue("registry.example.com:5000/installer:v1.13.12"),
			want:  true,
		},
		{
			name:  "post-import adopt: null prior image",
			state: types.StringNull(),
			plan:  types.StringValue(factory + ":v1.13.12"),
			want:  true,
		},
		{
			name:  "schematic changed, same tag: must always upgrade",
			state: types.StringValue(factory + ":v1.13.12"),
			plan:  types.StringValue(otherFactory + ":v1.13.12"),
			want:  false,
		},
		{
			name:  "schematic and tag changed",
			state: types.StringValue(factory + ":v1.13.4"),
			plan:  types.StringValue(otherFactory + ":v1.13.12"),
			want:  false,
		},
		{
			// The running-schematic check cannot see a switch to a vanilla image, so
			// verifying first would silently skip this upgrade.
			name:  "Factory image replaced by a vanilla installer, same tag",
			state: types.StringValue(factory + ":v1.13.12"),
			plan:  types.StringValue(vanilla + ":v1.13.12"),
			want:  false,
		},
		{
			name:  "registry changed, same tag",
			state: types.StringValue("registry.example.com/installer:v1.13.12"),
			plan:  types.StringValue("mirror.example.com/installer:v1.13.12"),
			want:  false,
		},
		{
			name:  "digest-pinned plan: the tag says nothing about the running image",
			state: types.StringValue(vanilla + ":v1.13.4"),
			plan:  types.StringValue(vanilla + digest),
			want:  false,
		},
		{
			name:  "digest-pinned state",
			state: types.StringValue(vanilla + digest),
			plan:  types.StringValue(vanilla + ":v1.13.12"),
			want:  false,
		},
		{
			name:  "no prior image tag and no planned tag",
			state: types.StringValue(vanilla),
			plan:  types.StringValue(vanilla),
			want:  false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := talosMachineVerifyBeforeUpgrade(tc.state, tc.plan); got != tc.want {
				t.Errorf("talosMachineVerifyBeforeUpgrade(%q, %q) = %v, want %v", tc.state.ValueString(), tc.plan.ValueString(), got, tc.want)
			}
		})
	}
}
