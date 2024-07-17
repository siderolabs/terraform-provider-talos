// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosImageFactorySchematicResource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// vanilla image
			{
				Config: testAccTalosTalosImageFactorySchematicConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_image_factory_schematic.this", "id", "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"),
				),
			},
			// empty schematic
			{
				Config: testAccTalosTalosImageFactorySchematicEmptySchematicConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_image_factory_schematic.this", "id", "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"),
				),
			},
			// empty customization
			{
				Config: testAccTalosTalosImageFactorySchematicEmptyCustomizationConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_image_factory_schematic.this", "id", "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"),
				),
			},
			// vanilla image
			{
				Config: testAccTalosTalosImageFactorySchematicConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_image_factory_schematic.this", "id", "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"),
				),
			},
			// known extension
			{
				Config: testAccTalosTalosImageFactorySchematicKnownExtensionConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_image_factory_schematic.this", "id", "d01dbf04a51b44a41d62b7c9692da0a74889277651600da6b602582654e4b402"),
				),
			},
		},
	})
}

func testAccTalosTalosImageFactorySchematicConfig() string {
	return `
provider "talos" {}

resource "talos_image_factory_schematic" "this" {}
`
}

func testAccTalosTalosImageFactorySchematicEmptySchematicConfig() string {
	return `
provider "talos" {}

resource "talos_image_factory_schematic" "this" {
	schematic = yamlencode({})
}
`
}

func testAccTalosTalosImageFactorySchematicEmptyCustomizationConfig() string {
	return `
provider "talos" {}

resource "talos_image_factory_schematic" "this" {
	schematic = yamlencode(
		{
			customization = {}
		}
	)
}
`
}

func testAccTalosTalosImageFactorySchematicKnownExtensionConfig() string {
	return `
provider "talos" {}

resource "talos_image_factory_schematic" "this" {
	schematic = yamlencode(
		{
			customization = {
				systemExtensions = {
					officialExtensions = ["siderolabs/amdgpu-firmware"]
				}
			}
		}
	)
}
`
}
