provider "talos" {}

data "talos_image_factory_overlays_versions" "this" {
  # get the latest talos version
  talos_version = "v1.7.5"
  filters = {
    name = "rock4cplus"
  }
}
