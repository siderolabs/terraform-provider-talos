// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"
	"time"

	"github.com/siderolabs/talos/pkg/machinery/config/generate/secrets"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

// TestGenerateKubeconfigDeterminism calls GenerateKubeconfig twice with
// identical inputs and asserts the output is byte-identical. This test
// fails if the implementation uses non-deterministic randomness (e.g.
// crypto/rand) for key generation or certificate serial numbers.
func TestGenerateKubeconfigDeterminism(t *testing.T) {
	fixedTime := time.Date(2024, 6, 15, 0, 0, 0, 0, time.UTC)

	bundle, err := secrets.NewBundle(secrets.NewFixedClock(fixedTime), nil)
	if err != nil {
		t.Fatalf("failed to create secrets bundle: %v", err)
	}

	clusterName := "determinism-test"
	endpoint := "https://10.0.0.1:6443"
	notBefore := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	notAfter := notBefore.Add(87600 * time.Hour)

	first, err := talos.GenerateKubeconfig(bundle, clusterName, endpoint, notBefore, notAfter)
	if err != nil {
		t.Fatalf("first GenerateKubeconfig failed: %v", err)
	}

	second, err := talos.GenerateKubeconfig(bundle, clusterName, endpoint, notBefore, notAfter)
	if err != nil {
		t.Fatalf("second GenerateKubeconfig failed: %v", err)
	}

	if first.Raw != second.Raw {
		t.Errorf("GenerateKubeconfig is not deterministic: two calls with identical inputs produced different output (%d vs %d bytes)", len(first.Raw), len(second.Raw))
	}
}
