// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosMachineDisksDataSource(t *testing.T) {
	testDir, err := os.MkdirTemp("", "talos-machine-disks-source")
	if err != nil {
		t.Fatal(err)
	}

	defer os.RemoveAll(testDir) //nolint:errcheck

	if err := os.Chmod(testDir, 0o755); err != nil {
		t.Fatal(err)
	}

	isoPath := filepath.Join(testDir, "talos.iso")

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.ParallelTest(t, resource.TestCase{
		WorkingDir: testDir,
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source: "dmacvicar/libvirt",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test default config
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccTalosMachineDisksDataSourceConfigV0("talos", rName, isoPath, "> 6GB"),
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
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccTalosMachineDisksDataSourceConfigV0("talos", rName, isoPath, "== 2GB"),
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

func testAccTalosMachineDisksDataSourceConfigV0(providerName, rName, isoPath, sizeFilter string) string {
	config := dynamicConfig{
		Provider:        providerName,
		ResourceName:    rName,
		IsoPath:         isoPath,
		DiskSizeFilter:  sizeFilter,
		WithApplyConfig: false,
		WithBootstrap:   false,
	}

	return config.render()
}
