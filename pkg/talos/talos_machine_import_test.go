// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cosi-project/runtime/pkg/safe"
	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/hashicorp/terraform-plugin-testing/tfversion"
	"github.com/siderolabs/talos/pkg/images"
	"github.com/siderolabs/talos/pkg/machinery/client"
	clientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"
	configresource "github.com/siderolabs/talos/pkg/machinery/resources/config"
)

// TestAccTalosMachine_importAdopt exercises the first apply after identity-only
// import. Both imports forget the old address without destroying the live node;
// the rest of the infrastructure remains in state for normal test cleanup.
func TestAccTalosMachine_importAdopt(t *testing.T) {
	const (
		talosVersion = "v1.14.0"
		factoryImage = "factory.talos.dev/metal-installer/${talos_image_factory_schematic.this.id}"
	)

	rName := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
	baseImage := images.InstallerImageRepository("metal")
	factoryConfig := `
resource "talos_image_factory_schematic" "this" {
  schematic = yamlencode({
    customization = {
      extraKernelArgs = ["console=ttyS0"]
    }
  })
}
`
	initial := testAccTalosMachineConfig(rName, baseImage, talosVersion, talosVersion) + factoryConfig
	matching := testAccTalosMachineAdoptConfig(initial, "this", "adopted")
	changed := testAccTalosMachineAdoptConfig(
		testAccTalosMachineConfig(rName, factoryImage, talosVersion, talosVersion)+factoryConfig,
		"adopted", "schematic",
	)

	var before nodeAdoptionSnapshot

	resource.ParallelTest(t, resource.TestCase{
		TerraformVersionChecks: []tfversion.TerraformVersionCheck{
			tfversion.SkipBelow(tfversion.Version1_11_0),
		},
		ExternalProviders: map[string]resource.ExternalProvider{
			"libvirt": {Source: "dmacvicar/libvirt", VersionConstraint: "= 0.8.3"},
		},
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: initial,
				Check: checkNodeAdoptionSnapshot("talos_machine.this", func(snapshot nodeAdoptionSnapshot) error {
					before = snapshot

					return nil
				}),
			},
			{
				Config: matching,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine.adopted", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_machine.adopted", "machine_configuration_hash"),
					checkNodeAdoptionSnapshot("talos_machine.adopted", func(snapshot nodeAdoptionSnapshot) error {
						if snapshot != before {
							return fmt.Errorf("matching adoption changed boot/config versions: before %+v, after %+v", before, snapshot)
						}

						return nil
					}),
				),
			},
			{Config: matching, PlanOnly: true},
			{
				Config: changed,
				Check: resource.ComposeAggregateTestCheckFunc(
					func(s *terraform.State) error {
						schematic := s.RootModule().Resources["talos_image_factory_schematic.this"]
						if schematic == nil || schematic.Primary == nil || schematic.Primary.ID == "" {
							return fmt.Errorf("factory schematic is missing from state")
						}

						if schematic.Primary.ID == images.DefaultInstallerImageSchematic {
							return fmt.Errorf("test schematic must differ from the initial installer")
						}

						return checkNodeSchematic("talos_machine.schematic", schematic.Primary.ID)(s)
					},
					checkNodeAdoptionSnapshot("talos_machine.schematic", func(snapshot nodeAdoptionSnapshot) error {
						if snapshot.bootID == before.bootID {
							return fmt.Errorf("same-version schematic adoption did not reboot the node")
						}

						return nil
					}),
					resource.TestCheckResourceAttrSet("data.talos_cluster_health.this", "id"),
				),
			},
			{Config: changed, PlanOnly: true},
		},
	})
}

func testAccTalosMachineAdoptConfig(config, previous, next string) string {
	config = strings.Replace(config, `resource "talos_machine" "this"`, `resource "talos_machine" "`+next+`"`, 1)
	config = strings.ReplaceAll(config, "talos_machine.this", "talos_machine."+next)

	return config + fmt.Sprintf(`
removed {
  from = talos_machine.%s
  lifecycle {
    destroy = false
  }
}

import {
  to = talos_machine.%s
  id = libvirt_domain.cp.network_interface[0].addresses[0]
}
`, previous, next)
}

type nodeAdoptionSnapshot struct {
	bootID        string
	configVersion string
}

func checkNodeAdoptionSnapshot(resourceName string, check func(nodeAdoptionSnapshot) error) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		machine := s.RootModule().Resources[resourceName]
		if machine == nil || machine.Primary == nil {
			return fmt.Errorf("%s is missing from state", resourceName)
		}

		attrs := machine.Primary.Attributes
		talosConfig := &clientconfig.Config{
			Context: "import-test",
			Contexts: map[string]*clientconfig.Context{
				"import-test": {
					CA:  attrs["client_configuration.ca_certificate"],
					Crt: attrs["client_configuration.client_certificate"],
					Key: attrs["client_configuration.client_key"],
				},
			},
		}

		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()

		c, err := client.New(ctx, client.WithConfig(talosConfig), client.WithEndpoints(attrs["endpoint"]))
		if err != nil {
			return err
		}

		defer c.Close() //nolint:errcheck

		nodeCtx := client.WithNode(ctx, attrs["node"])

		reader, err := c.Read(nodeCtx, "/proc/sys/kernel/random/boot_id")
		if err != nil {
			return err
		}

		defer reader.Close() //nolint:errcheck

		bootID, err := io.ReadAll(reader)
		if err != nil {
			return err
		}

		if strings.TrimSpace(string(bootID)) == "" {
			return fmt.Errorf("node returned an empty boot ID")
		}

		cfg, err := safe.StateGet[*configresource.MachineConfig](nodeCtx, c.COSI, configresource.NewMachineConfig(nil).Metadata())
		if err != nil {
			return err
		}

		return check(nodeAdoptionSnapshot{bootID: strings.TrimSpace(string(bootID)), configVersion: cfg.Metadata().Version().String()})
	}
}
