// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

func TestAccTalosClusterKubeconfigResource(t *testing.T) {
	testTime := time.Now()

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.Test(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClusterKubeconfigResourceConfig(rName),
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
			// test kubeconfig regeneration
			{
				PreConfig: func() {
					talos.OverridableTimeFunc = func() time.Time {
						return testTime.AddDate(0, 12, 5)
					}
				},
				Config:             testAccTalosClusterKubeconfigResourceConfig(rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})

	talos.OverridableTimeFunc = func() time.Time {
		return testTime
	}

	resource.ParallelTest(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source:            "dmacvicar/libvirt",
				VersionConstraint: "= 0.8.3",
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClusterKubeconfigResourceConfig(rName),
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
				Config:   testAccTalosClusterKubeconfigResourceConfig(rName),
				PlanOnly: true,
			},
		},
	})
}

func testAccTalosClusterKubeconfigResourceConfig(rName string) string {
	config := dynamicConfig{
		Provider:               "talos",
		ResourceName:           rName,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
	}

	return config.render()
}
