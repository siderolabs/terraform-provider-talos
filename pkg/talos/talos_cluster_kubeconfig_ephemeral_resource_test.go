// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosClusterKubeconfigEphemeralResource(t *testing.T) {
	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)

	resource.Test(t, resource.TestCase{
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {
				Source: "dmacvicar/libvirt",
				uri = ""
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClusterKubeconfigEphemeralResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_cluster_kubeconfig_ephemeral.this", "id", "example-cluster"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "node"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "endpoint"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "kubeconfig_raw"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "kubernetes_client_configuration.host"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "kubernetes_client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "kubernetes_client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_cluster_kubeconfig_ephemeral.this", "kubernetes_client_configuration.client_key"),
				),
			},
			// On the second plan, we expect a new resource to be created.
			{
				Config:             testAccTalosClusterKubeconfigEphemeralResourceConfig(rName),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func testAccTalosClusterKubeconfigEphemeralResourceConfig(rName string) string {
	config := dynamicConfig{
		Provider:               "talos",
		ResourceName:           rName,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
	}

	return config.render()
}
