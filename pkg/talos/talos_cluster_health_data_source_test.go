// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosClusterHealthDataSource(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClusterHealthDataSourceConfig("talos", rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_cluster_health.this", "id", "cluster_health"),
				),
			},
			// make sure there are no changes
			{
				Config:   testAccTalosClusterHealthDataSourceConfig("talos", rName),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosClusterHealthDataSourceConfig(providerName, rName string) string {
	config := dynamicConfig{
		Provider:               providerName,
		ResourceName:           rName,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
		WithClusterHealth:      true,
	}

	return config.render()
}
