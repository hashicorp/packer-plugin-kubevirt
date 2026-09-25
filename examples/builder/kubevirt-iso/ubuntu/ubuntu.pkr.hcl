# Copyright (c) Red Hat, Inc.
# SPDX-License-Identifier: MPL-2.0

packer {
  required_plugins {
    kubevirt = {
      source  = "github.com/hashicorp/kubevirt"
      version = ">= 1.0.0"
    }
  }
}

variable "kube_config" {
  type    = string
  default = "${env("KUBECONFIG")}"
}

source "kubevirt-iso" "ubuntu" {
  # Kubernetes configuration
  kube_config = var.kube_config
  name        = "ubuntu-2404-rand-24"
  namespace   = "images"

  # ISO configuration
  iso_volume_name = "ubuntu-2404-x86-64-iso"

  # VM type and preferences
  disk_size          = "10Gi"
  instance_type      = "o1.medium"
  instance_type_kind = "virtualmachineclusterinstancetype" # or "virtualmachineinstancetype"
  preference         = "ubuntu"
  preference_kind    = "virtualmachineclusterpreference" # or "virtualmachinepreference"
  os_type            = "linux"

  # Files to include in the ISO installation.
  # cloud-init NoCloud (used by the Ubuntu autoinstaller) expects
  # "user-data" and "meta-data" on a disk labelled "cidata".
  media_files = [
    "./user-data",
    "./meta-data"
  ]
  media_label = "cidata"

  # Boot process configuration
  # A set of commands to send over VNC connection
  boot_command = [
    "<wait>c<wait>",                                           # Open the GRUB command line
    "linux /casper/vmlinuz autoinstall ds=nocloud ---<enter>", # Skip the autoinstall confirmation prompt
    "initrd /casper/initrd<enter>",
    "boot<enter>"
  ]
  boot_wait                 = "10s" # Time to wait after boot starts
  installation_wait_timeout = "15m" # Timeout for installation to complete

  # SSH configuration
  communicator     = "ssh"
  ssh_host         = "127.0.0.1"
  ssh_local_port   = 2020
  ssh_remote_port  = 22
  ssh_username     = "ubuntu"
  ssh_password     = "ubuntu"
  ssh_wait_timeout = "20m"
}

build {
  sources = ["source.kubevirt-iso.ubuntu"]

  provisioner "shell" {
    inline = [
      "ls -la"
    ]
  }
}
