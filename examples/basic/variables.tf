variable "cluster_name" {
  description = "A name to provide for the Talos cluster"
  type        = string
}

variable "cluster_endpoint" {
  description = "The endpoint for the Talos cluster"
  type        = string
}

variable "node_data" {
  description = "A map of node data"
  type = object({
    controlplanes = map(object({
      install_disk = string
      hostname     = optional(string)
    }))
    workers = map(object({
      install_disk = string
      hostname     = optional(string)
    }))
  })
  default = {
    controlplanes = {
      "10.5.0.2" = {
        install_disk = "/dev/sda"
      },
      "10.5.0.3" = {
        install_disk = "/dev/sda"
      },
      "10.5.0.4" = {
        install_disk = "/dev/sda"
      }
    }
    workers = {
      "10.5.0.5" = {
        install_disk = "/dev/nvme0n1"
        hostname     = "worker-1"
      },
      "10.5.0.6" = {
        install_disk = "/dev/nvme0n1"
        hostname     = "worker-2"
      }
    }
  }
}
