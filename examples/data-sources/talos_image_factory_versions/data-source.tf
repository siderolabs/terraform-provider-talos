provider "talos" {}

data "talos_image_factory_versions" "this" {}

output "latest" {
  value = element(data.talos_image_factory_versions.this.talos_versions, length(data.talos_image_factory_versions.this.talos_versions) - 1)
}
