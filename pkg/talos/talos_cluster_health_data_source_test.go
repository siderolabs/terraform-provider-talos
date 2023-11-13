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

func TestAccTalosClusterHealthDataSource(t *testing.T) {
	testDir, err := os.MkdirTemp("", "talos-cluster-health-data-source")
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
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config: testAccTalosClusterHealthDataSourceConfig("talos", rName, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_cluster_health.this", "id", "cluster_health"),
				),
			},
			// make sure there are no changes
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config:   testAccTalosClusterHealthDataSourceConfig("talos", rName, isoPath),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosClusterHealthDataSourceConfig(providerName, rName, isoPath string) string {
	config := dynamicConfig{
		Provider:               providerName,
		ResourceName:           rName,
		IsoPath:                isoPath,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
		WithClusterHealth:      true,
	}

	return config.render()
}
