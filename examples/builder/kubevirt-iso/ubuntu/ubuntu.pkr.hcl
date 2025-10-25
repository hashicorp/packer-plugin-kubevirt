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

local "random-suffix" {
  expression = substr(uuidv4(), 0, 4)
}

source "kubevirt-iso" "fedora" {
  # Kubernetes configuration
  kube_config   = var.kube_config
  name          = "ubuntu-24-04-packer-test"
  # Setting an alternate name here with a random component allows failed builds
  # to be restarted without waiting for Kubevirt to finish cleaning up the prior one:
  vm_name       = "ubuntu-24-04-rand-${local.random-suffix}"
  namespace     = "images"

  # ISO configuration
  iso_volume_name = "ubuntu-24-04-x86-64-iso"

  # VM type and preferences
  disk_size          = "10Gi"
  instance_type      = "o1.medium"
  instance_type_kind = "virtualmachineclusterinstancetype" # or "virtualmachineinstancetype"
  preference         = "ubuntu"
  preference_kind    = "virtualmachineclusterpreference" # or "virtualmachinepreference"
  os_type            = "linux"

  # Default network configuration
  networks {
    name = "default"

    pod {}
  }

  # Ubuntu auto-installer requires a cloud-init NoCloud-compatible volume
  # See https://cloudinit.readthedocs.io/en/latest/reference/datasources/nocloud.html#source-2-drive-with-labeled-filesystem

  # A NoCloud volume must have a volume label of "CIDATA"
  media_label = "CIDATA"

  # Files to include in the cloud-init NoCloud source volume
  # Note that all four must be present or the installer will not use any of them
  media_content = {
    "user-data" = file("./autoinstall.yml")
    "meta-data" = ""
    "vendor-data" = ""
    "network-config" = ""
  }

  # If your storage supports a ReadWriteMany Block device uncomment
  # these:
  access_mode = "ReadWriteMany"
  volume_mode = "Block"

  # Boot process configuration
  # A set of commands to send over VNC connection
  boot_command = [
    "e",
    "<down><down><down>",
    "<end>",
    " autoinstall",
    "<leftCtrlOn>x<leftCtrlOff>"
  ]
  boot_wait          = "10s"     # Time to wait after boot starts
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
