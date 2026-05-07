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
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstaller, "factory.talos.dev/metal-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstallerSecureBoot, "factory.talos.dev/metal-installer-secureboot/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsISO, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64.iso"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsISOSecureBoot, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64-secureboot.iso"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImage, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64.raw.zst"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImageSecureBoot, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64-secureboot.raw.zst"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsPXE, "https://pxe.factory.talos.dev/pxe/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsKernel, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/kernel-amd64"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsKernelCmdLine, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/cmdline-metal-amd64"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInitramfs, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/initramfs-amd64.xz"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsUKI, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-amd64-secureboot-uki.efi"),
				),
			},
			// metal platform arm64
			{
				Config: testAccTalosImageFactoryURLsMetalPlatformArm64Config(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstaller, "factory.talos.dev/metal-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstallerSecureBoot, "factory.talos.dev/metal-installer-secureboot/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsISO, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64.iso"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsISOSecureBoot, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64-secureboot.iso"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImage, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64.raw.zst"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImageSecureBoot, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64-secureboot.raw.zst"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsPXE, "https://pxe.factory.talos.dev/pxe/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsKernel, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/kernel-arm64"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsKernelCmdLine, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/cmdline-metal-arm64"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInitramfs, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/initramfs-arm64.xz"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsUKI, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/metal-arm64-secureboot-uki.efi"),
				),
			},
			// aws platform
			{
				Config: testAccTalosImageFactoryURLsAWSPlatformConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstaller, "factory.talos.dev/aws-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsInstallerSecureBoot),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsISO),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsISOSecureBoot),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImage, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/aws-amd64.raw.xz"),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImageSecureBoot),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsPXE),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsKernel),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsKernelCmdLine),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsInitramfs),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsUKI),
				),
			},

			// nocloud platform
			{
				Config: testAccTalosImageFactoryURLsNoCloudPlatformConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstaller, "factory.talos.dev/nocloud-installer/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstallerSecureBoot, "factory.talos.dev/nocloud-installer-secureboot/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba:v1.7.5"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsISO, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64.iso"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsISOSecureBoot, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64-secureboot.iso"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImage, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64.raw.xz"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImageSecureBoot, "https://factory.talos.dev/image/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64-secureboot.raw.xz"),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsPXE, "https://pxe.factory.talos.dev/pxe/376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba/v1.7.5/nocloud-amd64"),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsKernel),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsKernelCmdLine),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsInitramfs),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsUKI),
				),
			},
			// rpigeneric sbc
			{
				Config: testAccTalosImageFactoryURLsSBCConfig(),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsInstaller, "factory.talos.dev/metal-installer/ee21ef4a5ef808a9b7484cc0dda0f25075021691c8c09a276591eedb638ea1f9:v1.7.5"),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsInstallerSecureBoot),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsISO),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsISOSecureBoot),
					resource.TestCheckResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImage, "https://factory.talos.dev/image/ee21ef4a5ef808a9b7484cc0dda0f25075021691c8c09a276591eedb638ea1f9/v1.7.5/metal-arm64.raw.xz"),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsDiskImageSecureBoot),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsPXE),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsKernel),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsKernelCmdLine),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsInitramfs),
					resource.TestCheckNoResourceAttr(dataTalosImageFactoryURLs, attrURLsUKI),
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
