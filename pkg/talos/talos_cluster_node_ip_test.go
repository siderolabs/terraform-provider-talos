// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccTalosClusterNodeMustBeIP drives the real provider schema to prove the validator
// is wired up, not merely correct in isolation: a hostname must fail during plan, before
// anything contacts a node. Reproduces the config from issue #382.
//
// IsUnitTest: schema validation is client-side, so no cluster is involved.
func TestAccTalosClusterNodeMustBeIP(t *testing.T) {
	for _, tc := range []struct {
		name   string
		config string
	}{
		{
			name: "node",
			config: `
resource "talos_cluster" "this" {
  node               = "host.example.com"
  kubernetes_version = "v1.35.3"
  client_configuration = {
    ca_certificate     = "Zm9v"
    client_certificate = "YmFy"
    client_key         = "YmF6"
  }
}
`,
		},
		{
			name: "control_plane_nodes",
			config: `
resource "talos_cluster" "this" {
  node                = "10.0.0.1"
  control_plane_nodes = ["10.0.0.1", "cp2.example.com"]
  kubernetes_version  = "v1.35.3"
  client_configuration = {
    ca_certificate     = "Zm9v"
    client_certificate = "YmFy"
    client_key         = "YmF6"
  }
}
`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resource.UnitTest(t, resource.TestCase{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Steps: []resource.TestStep{
					{
						Config:      tc.config,
						ExpectError: regexp.MustCompile(`is not an IP address`),
					},
				},
			})
		})
	}
}
