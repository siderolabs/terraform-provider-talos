// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"fmt"
	"net/url"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/stretchr/testify/assert"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/talos-systems/talos/pkg/machinery/constants"
	"gopkg.in/yaml.v3"
)

func TestAccTalosMachineConfigurationWorker(t *testing.T) {
	rString := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
	name := fmt.Sprintf("talos_machine_configuration_worker.%s", rString)

	resource.ParallelTest(t, resource.TestCase{
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineConfigurationWorkerDefaultConfig(rString, "https://example.com:6443"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "cluster_name", rString),
					resource.TestCheckResourceAttr(name, "cluster_endpoint", "https://example.com:6443"),
					resource.TestCheckResourceAttrSet(name, "machine_secrets"),
					resource.TestCheckResourceAttr(name, "kubernetes_version", constants.DefaultKubernetesVersion),
					resource.TestCheckNoResourceAttr(name, "config_patches.0"),
					resource.TestCheckNoResourceAttr(name, "talos_version"),
					resource.TestCheckResourceAttr(name, "config_version", "v1alpha1"),
					resource.TestCheckResourceAttr(name, "docs_enabled", "true"),
					resource.TestCheckResourceAttr(name, "examples_enabled", "true"),
					resource.TestCheckResourceAttrWith(name, "machine_config", func(value string) error {
						return validateGeneratedTalosMachineConfigWorkerDefaults(t, rString, "https://example.com:6443", value)
					}),
				),
			},
			{
				Config: testAccTalosMachineConfigurationWorkerOverrideConfig(rString, "https://example-1.com:6443"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "cluster_name", rString),
					resource.TestCheckResourceAttr(name, "cluster_endpoint", "https://example-1.com:6443"),
					resource.TestCheckResourceAttrSet(name, "machine_secrets"),
					resource.TestCheckResourceAttrSet(name, "config_patches.0"),
					resource.TestCheckResourceAttr(name, "kubernetes_version", "1.24.0"),
					resource.TestCheckResourceAttr(name, "talos_version", "v1.2"),
					resource.TestCheckResourceAttr(name, "config_version", "v1alpha1"),
					resource.TestCheckResourceAttr(name, "docs_enabled", "false"),
					resource.TestCheckResourceAttr(name, "examples_enabled", "false"),
					resource.TestCheckResourceAttrWith(name, "machine_config", func(value string) error {
						return validateGeneratedTalosMachineConfigWorkerOverride(t, rString, value)
					}),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccTalosMachineConfigurationWorkerInvalidPatchConfig(rString, "https://example.com:6443"),
				ExpectError: regexp.MustCompile("unknown keys found during decoding:"),
			},
		},
	})
}

func testAccTalosMachineConfigurationWorkerDefaultConfig(rName, endpoint string) string {
	return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {}

resource "talos_machine_configuration_worker" "%s" {
	cluster_name = "%s"
	cluster_endpoint = "%s"
	machine_secrets = talos_machine_secrets.%s.machine_secrets
}
`, rName, rName, rName, endpoint, rName)
}

func testAccTalosMachineConfigurationWorkerInvalidPatchConfig(rName, endpoint string) string {
	return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {}

resource "talos_machine_configuration_worker" "%s" {
	cluster_name = "%s"
	cluster_endpoint = "%s"
	machine_secrets = talos_machine_secrets.%s.machine_secrets
	config_patches = [
		file("${path.module}/testdata/patch-invalid.yaml"),
	]
}
`, rName, rName, rName, endpoint, rName)
}

func testAccTalosMachineConfigurationWorkerOverrideConfig(rName, endpoint string) string {
	return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {}

resource "talos_machine_configuration_worker" "%s" {
	cluster_name = "%s"
	cluster_endpoint = "%s"
	machine_secrets = talos_machine_secrets.%s.machine_secrets
	config_patches = [
		file("${path.module}/testdata/patch-worker.yaml"),
	]
	kubernetes_version = "1.24.0"
	talos_version = "v1.2"
	config_version = "v1alpha1"
	docs_enabled = false
	examples_enabled = false
}
`, rName, rName, rName, endpoint, rName)
}

func validateGeneratedTalosMachineConfigWorkerDefaults(t *testing.T, rName, endpoint, mc string) error {
	var machineConfig v1alpha1.Config

	if err := yaml.Unmarshal([]byte(mc), &machineConfig); err != nil {
		return err
	}

	installDisk, err := machineConfig.Machine().Install().Disk()
	if err != nil {
		return err
	}

	ep, err := url.Parse("https://example.com:6443")
	if err != nil {
		return err
	}

	assert.Equal(t, ep, machineConfig.Cluster().Endpoint())
	assert.Equal(t, constants.DefaultDNSDomain, machineConfig.Cluster().Network().DNSDomain())
	assert.Equal(t, "/dev/sda", installDisk)
	assert.Equal(t, generateInstallerImage(), machineConfig.Machine().Install().Image())
	assert.Equal(t, fmt.Sprintf("ghcr.io/siderolabs/kubelet:v%s", constants.DefaultKubernetesVersion), machineConfig.Machine().Kubelet().Image())
	assert.Equal(t, true, machineConfig.Persist())
	assert.Equal(t, "v1alpha1", machineConfig.Version())
	assert.Equal(t, true, machineConfig.Cluster().Discovery().Enabled())
	assert.Equal(t, "Indicates the schema used to decode the contents.", machineConfig.Doc().Field(0).Description)
	// verifying there's examples
	assert.Contains(t, mc, (`
        # diskSelector:
        #     size: 4GB # Disk size.
`))

	return nil
}

func validateGeneratedTalosMachineConfigWorkerOverride(t *testing.T, rName, mc string) error {
	var machineConfig v1alpha1.Config

	if err := yaml.Unmarshal([]byte(mc), &machineConfig); err != nil {
		return err
	}

	ep, err := url.Parse("https://example-1.com:6443")
	if err != nil {
		return err
	}

	assert.Equal(t, map[string]string{"cpu": "1"}, machineConfig.Machine().Sysfs())
	assert.Equal(t, map[string]string{"foo": "bar"}, machineConfig.Machine().Kubelet().ExtraArgs())

	assert.Equal(t, ep, machineConfig.Cluster().Endpoint())
	assert.Equal(t, fmt.Sprintf("ghcr.io/siderolabs/kubelet:v%s", "1.24.0"), machineConfig.Machine().Kubelet().Image())
	assert.Equal(t, "v1alpha1", machineConfig.Version())
	// verifying there's no examples
	assert.NotContains(t, mc, (`
	        # diskSelector:
	        #     size: 4GB # Disk size.
	`))
	return nil
}
