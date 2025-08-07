// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
)

func TestAccTalosImageFactoryVersionsDataSource(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosImageFactoryVersionsDataSourceConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_versions.this", "talos_versions.0", "v1.2.0"),
				),
			},
			{
				Config: testAccTalosImageFactoryVersionsDataSourceWithFilterConfig(),
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownOutputValue("talos_version", knownvalue.StringExact("v1.10.6")),
				},
			},
		},
	})
}

func testAccTalosImageFactoryVersionsDataSourceConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_versions" "this" {}
`
}

func testAccTalosImageFactoryVersionsDataSourceWithFilterConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_versions" "this" {
	filters = {
		stable_versions_only = true
	}
}

output "talos_version" {
	value = data.talos_image_factory_versions.this.talos_versions[length(data.talos_image_factory_versions.this.talos_versions) - 1]
}
`
}
