// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/knownvalue"
	"github.com/hashicorp/terraform-plugin-testing/statecheck"
	"github.com/hashicorp/terraform-plugin-testing/tfjsonpath"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/siderolabs/talos/pkg/machinery/config/machine"
)

// TestAccTalosMachineConfigurationEphemeralResource tests that the ephemeral
// resource generates machine configuration from machine secrets.
func TestAccTalosMachineConfigurationEphemeralResource(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: `
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_machine_configuration" "this" {
	cluster_name     = "test-cluster"
	cluster_endpoint = "https://10.0.0.1:6443"
	machine_type     = "controlplane"
	machine_secrets  = ephemeral.talos_machine_secrets.this.machine_secrets
}

provider "echo" {
	data = {
		machine_configuration = ephemeral.talos_machine_configuration.this.machine_configuration
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("machine_configuration"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosMachineConfigurationEphemeralResourceWorker tests generating
// a worker machine configuration.
func TestAccTalosMachineConfigurationEphemeralResourceWorker(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: `
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_machine_configuration" "this" {
	cluster_name     = "test-cluster"
	cluster_endpoint = "https://10.0.0.1:6443"
	machine_type     = "worker"
	machine_secrets  = ephemeral.talos_machine_secrets.this.machine_secrets
}

provider "echo" {
	data = {
		machine_configuration = ephemeral.talos_machine_configuration.this.machine_configuration
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("machine_configuration"), knownvalue.NotNull()),
				},
			},
		},
	})
}

// TestAccTalosMachineConfigurationEphemeralResourceComponents tests generating
// a machine configuration without Kubernetes and etcd.
func TestAccTalosMachineConfigurationEphemeralResourceComponents(t *testing.T) {
	t.Parallel()

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config: `
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_machine_configuration" "this" {
	cluster_name     = "test-cluster"
	cluster_endpoint = "https://10.0.0.1:6443"
	machine_type     = "controlplane"
	machine_secrets  = ephemeral.talos_machine_secrets.this.machine_secrets
	components = {
		kubernetes = false
		etcd       = false
	}
	config_patches = [
		yamlencode({
			apiVersion = "v1alpha1"
			kind       = "ContainerConfig"
			name       = "hello"
			image      = "docker.io/library/busybox:1.37.0"
		}),
	]
}

provider "echo" {
	data = {
		machine_configuration = ephemeral.talos_machine_configuration.this.machine_configuration
		components            = ephemeral.talos_machine_configuration.this.components
	}
}

resource "echo" "test" {}
`,
				ConfigStateChecks: []statecheck.StateCheck{
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("components"), knownvalue.ObjectExact(map[string]knownvalue.Check{
						"kubernetes": knownvalue.Bool(false),
						"etcd":       knownvalue.Bool(false),
					})),
					statecheck.ExpectKnownValue("echo.test", tfjsonpath.New("data").AtMapKey("machine_configuration"), knownvalue.StringFunc(func(v string) error {
						return validateComponentsTalosMachineConfig(machine.TypeControlPlane, false, false, v)
					})),
				},
			},
		},
	})
}

// TestAccTalosMachineConfigurationEphemeralResourceComponentsInvalid tests that invalid
// components combinations are rejected by ValidateConfig ("components are invalid";
// Open would fail with "failed to generate machine configuration").
func TestAccTalosMachineConfigurationEphemeralResourceComponentsInvalid(t *testing.T) {
	t.Parallel()

	config := func(talosVersion, components string) string {
		return fmt.Sprintf(`
ephemeral "talos_machine_secrets" "this" {}

ephemeral "talos_machine_configuration" "this" {
	cluster_name     = "test-cluster"
	cluster_endpoint = "https://10.0.0.1:6443"
	machine_type     = "controlplane"
	machine_secrets  = ephemeral.talos_machine_secrets.this.machine_secrets
	talos_version    = %q
	components       = %s
}

provider "echo" {
	data = {
		machine_configuration = ephemeral.talos_machine_configuration.this.machine_configuration
	}
}

resource "echo" "test" {}
`, talosVersion, components)
	}

	resource.UnitTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_10_0),
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactoriesWithEcho,
		Steps: []resource.TestStep{
			{
				Config:      config("v1.13", "{ kubernetes = false, etcd = false }"),
				ExpectError: regexp.MustCompile(`components are invalid[\s\S]*disabling kubernetes requires talos_version v1.14 or later`),
			},
			{
				Config:      config("v1.14", "{ etcd = false }"),
				ExpectError: regexp.MustCompile(`components are invalid[\s\S]*kubernetes requires etcd`),
			},
		},
	})
}
