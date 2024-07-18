// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosMachineDisksDataSource(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source: "dmacvicar/libvirt",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test default config
			{
				Config: testAccTalosMachineDisksDataSourceConfigV0("talos", rName, "> 6GB"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "id", "machine_disks"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "node"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "endpoint"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "filters.size", "> 6GB"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "disks.#", "1"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "disks.0.name", "/dev/vda"),
				),
			},
			// test a filter
			{
				Config: testAccTalosMachineDisksDataSourceConfigV0("talos", rName, "== 2GB"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "id", "machine_disks"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "node"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "endpoint"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_machine_disks.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "filters.size", "== 2GB"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "disks.#", "1"),
					resource.TestCheckResourceAttr("data.talos_machine_disks.this", "disks.0.name", "/dev/vdb"),
				),
			},
		},
	})
}

func testAccTalosMachineDisksDataSourceConfigV0(providerName, rName, sizeFilter string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		DiskSizeFilter:  sizeFilter,
		WithApplyConfig: false,
		WithBootstrap:   false,
	}

	return config.render()
}
