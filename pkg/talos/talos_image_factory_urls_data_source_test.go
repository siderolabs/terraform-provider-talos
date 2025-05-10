// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccTalosImageFactoryURLsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      testAccTalosImageFactoryURLsBothSBCAndPlatformNotSetConfig(),
				ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
			},
			{
				Config:      testAccTalosImageFactoryURLsBothSBCAndPlatformSetConfig(),
				ExpectError: regexp.MustCompile("Invalid Attribute Combination"),
			},
			// Invalid Version
			{
				Config:      testAccTalosImageFactoryURLsInvalidVersionConfig(),
				ExpectError: regexp.MustCompile("talos_version is not valid"),
			},
		},
	})

	//nolint:lll
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// metal platform
			{
				Config: testAccTalosImageFactoryURLsMetalPlatformConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer", "factory.talos.dev/metal-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer_secureboot", "factory.talos.dev/metal-installer-secureboot/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.iso", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64.iso"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.iso_secureboot", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64-secureboot.iso"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64.raw.zst"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image_secureboot", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64-secureboot.raw.zst"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.pxe", "https://pxe.factory.talos.dev/pxe/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.kernel", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/kernel-amd64"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.kernel_command_line", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/cmdline-metal-amd64"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.initramfs", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/initramfs-amd64.xz"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.uki", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64-secureboot-uki.efi"),
				),
			},
			// metal platform arm64
			{
				Config: testAccTalosImageFactoryURLsMetalPlatformArm64Config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer", "factory.talos.dev/metal-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer_secureboot", "factory.talos.dev/metal-installer-secureboot/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.iso", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64.iso"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.iso_secureboot", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64-secureboot.iso"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64.raw.zst"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image_secureboot", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64-secureboot.raw.zst"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.pxe", "https://pxe.factory.talos.dev/pxe/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.kernel", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/kernel-arm64"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.kernel_command_line", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/cmdline-metal-arm64"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.initramfs", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/initramfs-arm64.xz"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.uki", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64-secureboot-uki.efi"),
				),
			},
			// aws platform
			{
				Config: testAccTalosImageFactoryURLsAWSPlatformConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer", "factory.talos.dev/aws-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.installer_secureboot"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.iso"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.iso_secureboot"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/aws-amd64.raw.xz"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image_secureboot"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.pxe"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.kernel"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.kernel_command_line"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.initramfs"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.uki"),
				),
			},

			// nocloud platform
			{
				Config: testAccTalosImageFactoryURLsNoCloudPlatformConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer", "factory.talos.dev/nocloud-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer_secureboot", "factory.talos.dev/installer-secureboot/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.iso", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64.iso"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.iso_secureboot", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64-secureboot.iso"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64.raw.xz"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image_secureboot", "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64-secureboot.raw.xz"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.pxe", "https://pxe.factory.talos.dev/pxe/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.kernel"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.kernel_command_line"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.initramfs"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.uki"),
				),
			},
			// rpigeneric sbc
			{
				Config: testAccTalosImageFactoryURLsSBCConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.installer", "factory.talos.dev/metal-installer/ee21ef4a5ef808a9b7484cc0dda0f25075021691c8c09a276591eedb638ea1f9:v1.7.5"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.installer_secureboot"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.iso"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.iso_secureboot"),
					resource.TestCheckResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image", "https://factory.talos.dev/image/ee21ef4a5ef808a9b7484cc0dda0f25075021691c8c09a276591eedb638ea1f9/v1.7.5/metal-arm64.raw.xz"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.disk_image_secureboot"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.pxe"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.kernel"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.kernel_command_line"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.initramfs"),
					resource.TestCheckNoResourceAttr("data.talos_image_factory_urls.this", "urls.uki"),
				),
			},
		},
	})
}

func testAccTalosImageFactoryURLsBothSBCAndPlatformNotSetConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "v1.7.0"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
}
`
}

func testAccTalosImageFactoryURLsBothSBCAndPlatformSetConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "v1.7.0"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	platform = "metal"
	sbc = "rpi_generic"
}
`
}

func testAccTalosImageFactoryURLsMetalPlatformConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "1.7.5"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	platform = "metal"
}
`
}

func testAccTalosImageFactoryURLsMetalPlatformArm64Config() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	architecture = "arm64"
	talos_version = "v1.7.5"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	platform = "metal"
}
`
}

func testAccTalosImageFactoryURLsAWSPlatformConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "v1.7.5"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	platform = "aws"
}
`
}

func testAccTalosImageFactoryURLsNoCloudPlatformConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "v1.7.5"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	platform = "nocloud"
}
`
}

func testAccTalosImageFactoryURLsSBCConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "v1.7.5"
	schematic_id = "ee21ef4a5ef808a9b7484cc0dda0f25075021691c8c09a276591eedb638ea1f9"
	sbc = "rpi_generic"
}
`
}

func testAccTalosImageFactoryURLsInvalidVersionConfig() string {
	return `
provider "talos" {}

data "talos_image_factory_urls" "this" {
	talos_version = "invalid_version"
	schematic_id = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
	platform = "metal"
}
`
}
