resource "talos_machine_secrets" "this" {}

data "talos_client_configuration" "this" {
  cluster_name         = "example-cluster"
  client_configuration = talos_machine_secrets.this.client_configuration
  nodes                = ["10.5.0.2"]
}
