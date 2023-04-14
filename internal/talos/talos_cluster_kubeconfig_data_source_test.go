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

func TestAccTalosClusterKubeconfigDataSource(t *testing.T) {
	testDir, err := os.MkdirTemp("", "talos-cluster-kubeconfig-data-source")
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
				Config: testAccTalosClusterKubeconfigDataSourceConfig("talos", rName, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_cluster_kubeconfig.this", "id", "example-cluster"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "node"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "endpoint"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "kubeconfig_raw"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "kubernetes_client_configuration.host"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "kubernetes_client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "kubernetes_client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("data.talos_cluster_kubeconfig.this", "kubernetes_client_configuration.client_key"),
					resource.TestCheckResourceAttr("data.talos_cluster_kubeconfig.this", "wait", "true"),
				),
			},
			// make sure there are no changes
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config:   testAccTalosClusterKubeconfigDataSourceConfig("talos", rName, isoPath),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosClusterKubeconfigDataSourceConfig(providerName, rName, isoPath string) string {
	config := dynamicConfig{
		Provider:               providerName,
		ResourceName:           rName,
		IsoPath:                isoPath,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
	}

	return config.render()
}
