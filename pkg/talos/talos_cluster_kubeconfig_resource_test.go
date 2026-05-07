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
			libvirtProvider: {
				Source:            libvirtProviderSource,
				VersionConstraint: libvirtProviderVersionConstraint,
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClusterKubeconfigResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resTalosClusterKubeconfig, "id", exampleCluster),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, fieldNode),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, fieldEndpoint),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, attrClientConfigCACert),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, attrClientConfigClientCert),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, attrClientConfigClientKey),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, fieldKubeconfigRaw),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.host"),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.client_key"),
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
			libvirtProvider: {
				Source:            libvirtProviderSource,
				VersionConstraint: libvirtProviderVersionConstraint,
			},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosClusterKubeconfigResourceConfig(rName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resTalosClusterKubeconfig, "id", exampleCluster),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, fieldNode),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, fieldEndpoint),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, attrClientConfigCACert),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, attrClientConfigClientCert),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, attrClientConfigClientKey),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, fieldKubeconfigRaw),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.host"),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet(resTalosClusterKubeconfig, "kubernetes_client_configuration.client_key"),
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
		Provider:               providerName,
		ResourceName:           rName,
		WithApplyConfig:        true,
		WithBootstrap:          true,
		WithRetrieveKubeConfig: true,
	}

	return config.render()
}
