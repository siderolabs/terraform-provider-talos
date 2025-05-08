data "talos_image_factory_urls" "this" {
  talos_version = "v1.7.5"
  schematic_id  = "376567988ad370138ad8b2698212367b8edcb69b5fd68c80be1f2ec7d603b4ba"
  platform      = "metal"
}

output "installer_image" {
  value = data.talos_image_factory_urls.this.urls.installer
}
