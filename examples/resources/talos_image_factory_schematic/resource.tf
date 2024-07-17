provider "talos" {}

data "talos_image_factory_extensions_versions" "this" {
  # get the latest talos version
  talos_version = "v1.7.5"
  filters = {
    names = [
      "amdgpu",
      "tailscale",
    ]
  }
}

resource "talos_image_factory_schematic" "this" {
  schematic = yamlencode(
    {
      customization = {
        systemExtensions = {
          officialExtensions = data.talos_image_factory_extensions_versions.this.extensions_info.*.name
        }
      }
    }
  )
}

output "schematic_id" {
  value = talos_image_factory_schematic.this.id
}
