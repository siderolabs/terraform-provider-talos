// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
)

func TestAccTalosMachineSecrets(t *testing.T) {
	rString := acctest.RandStringFromCharSet(10, acctest.CharSetAlpha)
	name := fmt.Sprintf("talos_machine_secrets.%s", rString)

	resource.ParallelTest(t, resource.TestCase{
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccTalosMachineSecretsConfig(rString, ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckNoResourceAttr(name, "talos_version"),
					resource.TestCheckResourceAttrSet(name, "machine_secrets"),
				),
			},
			{
				Config: testAccTalosMachineSecretsConfig(rString, "v1.2"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(name, "talos_version", "v1.2"),
					resource.TestCheckResourceAttrSet(name, "machine_secrets"),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccTalosMachineSecretsConfig(rString, "invalid version"),
				ExpectError: regexp.MustCompile("error parsing version \"invalid version\""),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		Providers: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config:      testAccTalosMachineSecretsConfig(rString, "v1.1"),
				ExpectError: regexp.MustCompile("config generation only supported for Talos >= v1.2"),
			},
		},
	})
}

func testAccTalosMachineSecretsConfig(rName, talosVersion string) string {
	if talosVersion == "" {
		return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {}
`, rName)
	}

	return fmt.Sprintf(`
resource "talos_machine_secrets" "%s" {
	talos_version = "%s"
}
`, rName, talosVersion)
}
