// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosImageFactoryOverlaysVersionsDataSource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosImageFactoryOverlaysVersionsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_overlays_versions.this", "overlays_info.0.name", "rpi_generic"),
				),
			},
			{
				Config: testAccTalosImageFactoryOverlaysVersionsDataSourceConfigWithFilters(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_overlays_versions.this", "overlays_info.#", "1"),
					resource.TestCheckResourceAttr("data.talos_image_factory_overlays_versions.this", "overlays_info.0.name", "rock4cplus"),
				),
			},
		},
	})
}

func testAccTalosImageFactoryOverlaysVersionsDataSourceConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_overlays_versions" "this" {
	talos_version = "v1.7.0"
}
`
}

func testAccTalosImageFactoryOverlaysVersionsDataSourceConfigWithFilters() string {
	return `
provider "talos" {}

data "talos_image_factory_overlays_versions" "this" {
	talos_version = "v1.7.0"
	filters = {
		name = "rock4cplus"
	}
}
`
}
