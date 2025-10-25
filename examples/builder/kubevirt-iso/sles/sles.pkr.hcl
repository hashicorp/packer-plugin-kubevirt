# Copyright (c) Red Hat, Inc.
# SPDX-License-Identifier: MPL-2.0

packer {
  required_plugins {
    kubevirt = {
      source  = "github.com/hashicorp/kubevirt"
      version = ">= 0.8.0"
    }
  }
}

variable "kube_config" {
  type    = string
  default = "${env("KUBECONFIG")}"
}

source "kubevirt-iso" "fedora" {
  # Kubernetes configuration
  kube_config   = var.kube_config
  name          = "sles-15-sp7-rand-85"
  namespace     = "images"

  # ISO configuration
  iso_volume_name = "sles-15-sp7-x86-64-iso"

  # VM type and preferences
  disk_size          = "20Gi"
  instance_type      = "o1.medium"
  instance_type_kind = "virtualmachineclusterinstancetype" # or "virtualmachineinstancetype"
  preference         = "sles"
  preference_kind    = "virtualmachineclusterpreference" # or "virtualmachinepreference"
  os_type            = "linux"

  # Default network configuration
  networks {
    name = "default"

    pod {}
  }

  # Files to include in the ISO installation
  media_files = [
    "./autoinst.xml"
  ]

  # If your storage supports a ReadWriteMany Block device uncomment
  # these:
  access_mode = "ReadWriteMany"
  volume_mode = "Block"

  # Boot process configuration
  # A set of commands to send over VNC connection
  boot_command = [
    "<down><enter>"
  ]

  boot_wait          = "5s"     # Time to wait after boot starts
  agent_wait_timeout = "15m"     # Timeout for QEMU Guest Agent to become available

  # SSH configuration
  ssh_username = "user"
  ssh_password = "root"
  ssh_timeout  = "20m"
}

build {
  sources = ["source.kubevirt-iso.fedora"]

  provisioner "shell" {
    inline = [
      "ls -la"
    ]
  }
}
