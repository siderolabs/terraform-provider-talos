// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosImageFactoryExtensionsVersionsDataSource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosImageFactoryExtensionsVersionsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_extensions_versions.this", "extensions_info.0.name", "siderolabs/amdgpu-firmware"),
				),
			},
			{
				Config: testAccTalosImageFactoryExtensionsVersionsDataSourceConfigWithFilters(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_extensions_versions.this", "extensions_info.#", "5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_extensions_versions.this", "extensions_info.0.name", "siderolabs/nvidia-container-toolkit"),
				),
			},
		},
	})
}

func testAccTalosImageFactoryExtensionsVersionsDataSourceConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_extensions_versions" "this" {
	talos_version = "v1.7.0"
}
`
}

func testAccTalosImageFactoryExtensionsVersionsDataSourceConfigWithFilters() string {
	return `
provider "talos" {}

data "talos_image_factory_extensions_versions" "this" {
	talos_version = "v1.7.0"
	filters = {
		names = [
			"nvidia",
			"tailscale"
		]
	}
}
`
}
