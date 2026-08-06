// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"strings"
	"testing"

	"github.com/siderolabs/talos/pkg/machinery/gendata"
)

// TestInstallDiskPatchFormPerTalosVersion renders the acceptance test fixtures with the
// Talos versions their tests actually use and asserts the install patch is in the form
// that version accepts.
//
// Talos 1.14 moved the install settings into an UnattendedInstallConfig document. Earlier
// versions do not register that kind and reject the document ("not registered"). On 1.14+
// .machine.install still works on its own (deprecated, still validated), but not together
// with the generated UnattendedInstallConfig: that combination is rejected with
// "incompatible with v1alpha1 config". Getting this wrong only shows up against a real
// node, so pin it here instead.
func TestInstallDiskPatchFormPerTalosVersion(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		cfg     string
		wantDoc bool
	}{
		{"workerDrain v1.13.0-rc.0", testAccTalosMachineWorkerDrainConfig("n", "v1.13.0-rc.0", "v1.13.0"), false},
		{"machineConfig v1.12.7", testAccTalosMachineConfig("n", "img", "v1.12.7", "v1.12.7"), false},
		{"machineConfig v1.13.0", testAccTalosMachineConfig("n", "img", "v1.13.0", "v1.13.0"), false},
		{"upgradeAndK8sBump v1.12.7", testAccTalosMachineConfigUpgradeAndK8sBump("n", "v1.12.7", "v1.13.0", "v1.34.0", true), false},
		{"machineConfig current", testAccTalosMachineConfig("n", "img", gendata.VersionTag, gendata.VersionTag), true},
		{"writeOnlyAttrs current", testAccTalosMachineConfigWithWriteOnlyAttrs("n", "img", gendata.VersionTag, gendata.VersionTag), true},
		{"k8sBumpNoBootstrap current", testAccTalosMachineConfigK8sBumpNoBootstrap("n", gendata.VersionTag, "v1.34.0", true), true},
		// The apply-mode upgrade fixture is applied by three different provider versions in
		// turn, so it renders both forms. The current-provider step must carry the document:
		// its generated config has no .machine.install for 1.14 to fall back on, and the
		// generator's /dev/sda default matches no disk on the test VM.
		{"autoStagedUpgrade current", testAccTalosMachineConfigurationApplyResourceConfigAutoStagedUpgrade("n", "staged_if_needing_reboot", true), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			hasDoc := strings.Contains(tc.cfg, "UnattendedInstallConfig")
			hasLegacy := strings.Contains(tc.cfg, "install = {")

			if hasDoc == hasLegacy {
				t.Fatalf("expected exactly one install patch form, got document=%v legacy=%v", hasDoc, hasLegacy)
			}

			if hasDoc != tc.wantDoc {
				t.Errorf("wrong install patch form: got document=%v, want document=%v", hasDoc, tc.wantDoc)
			}

			if strings.Contains(tc.cfg, "%!") {
				t.Errorf("format verb error in rendered config, a positional verb likely collided")
			}
		})
	}
}
