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
			libvirtProvider: {
				Source:            libvirtProviderSource,
				VersionConstraint: libvirtProviderVersionConstraint,
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test default config
			{
				Config: testAccTalosMachineDisksDataSourceConfigV0(providerName, rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosMachineDisks, "id", "machine_disks"),
					resource.TestCheckResourceAttrSet(dataTalosMachineDisks, fieldNode),
					resource.TestCheckResourceAttrSet(dataTalosMachineDisks, fieldEndpoint),
					resource.TestCheckResourceAttrSet(dataTalosMachineDisks, attrClientConfigCACert),
					resource.TestCheckResourceAttrSet(dataTalosMachineDisks, attrClientConfigClientCert),
					resource.TestCheckResourceAttrSet(dataTalosMachineDisks, attrClientConfigClientKey),
					resource.TestCheckResourceAttr(dataTalosMachineDisks, fieldSelector, "disk.size > 6u * GB"),
					resource.TestCheckResourceAttr(dataTalosMachineDisks, "disks.#", "1"),
					resource.TestCheckResourceAttr(dataTalosMachineDisks, "disks.0.dev_path", testDevVDA),
				),
			},
		},
	})
}

func testAccTalosMachineDisksDataSourceConfigV0(providerName, rName string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		WithApplyConfig: false,
		WithBootstrap:   false,
	}

	return config.render()
}
