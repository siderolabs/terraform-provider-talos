resource "talos_machine_secrets" "this" {}

data "talos_machine_disks" "this" {
  client_configuration = talos_machine_secrets.this.client_configuration
  node                 = "10.5.0.2"
  filters = {
    size = "> 100GB"
    type = "nvme"
  }
}

# for example, this could be used to pass in a list of disks to rook-ceph
output "nvme_disks" {
  value = data.talos_machine_disks.this.disks.*.name
}
