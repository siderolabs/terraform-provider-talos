// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

func TestAccTalosClusterKubeconfigResource(t *testing.T) {
	testTime := time.Now()

	testDir, err := os.MkdirTemp("", "talos-cluster-kubeconfig-resource")
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
				Config: testAccTalosClusterKubeconfigResourceConfig("talos", rName, isoPath),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_cluster_kubeconfig.this", "id", "example-cluster"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "node"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "endpoint"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "kubeconfig_raw"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "kubernetes_client_configuration.host"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "kubernetes_client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "kubernetes_client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig.this", "kubernetes_client_configuration.client_key"),
				),
			},
			// make sure there are no changes
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}
				},
				Config:   testAccTalosClusterKubeconfigResourceConfig("talos", rName, isoPath),
				PlanOnly: true,
			},
			// test kubeconfig regeneration
			{
				PreConfig: func() {
					if err := downloadTalosISO(isoPath); err != nil {
						t.Fatal(err)
					}

					talos.OverridableTimeFunc = func() time.Time {
						return testTime.AddDate(0, 12, 5)
					}
				},
				Config:             testAccTalosClusterKubeconfigResourceConfig("talos", rName, isoPath),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})

	talos.OverridableTimeFunc = func() time.Time {
		return testTime
	}
}

func testAccTalosClusterKubeconfigResourceConfig(providerName, rName, isoPath string) string {
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
