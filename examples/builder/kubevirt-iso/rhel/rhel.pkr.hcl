# Copyright (c) Red Hat, Inc.
# SPDX-License-Identifier: MPL-2.0

packer {
  required_plugins {
    kubevirt = {
      source  = "github.com/hashicorp/kubevirt"
      version = ">= 0.7.0"
    }
  }
}

variable "kube_config" {
  type    = string
  default = "${env("KUBECONFIG")}"
}

source "kubevirt-iso" "rhel" {
  # Kubernetes configuration
  kube_config   = var.kube_config
  name          = "rhel-10-rand-95"
  namespace     = "ben-dev"

  # ISO configuration
  iso_volume_name = "rhel-10-x86-64-iso"

  # VM type and preferences
  disk_size          = "10Gi"
  instance_type      = "o1.medium"
  instance_type_kind = "virtualmachineclusterinstancetype" # or "virtualmachineinstancetype"
  preference         = "rhel.10"
  preference_kind    = "virtualmachineclusterpreference" # or "virtualmachinepreference"
  os_type            = "linux"

  # Files to include in the ISO installation
  media_files = [
    "./ks.cfg"
  ]

  # Boot process configuration
  # A set of commands to send over VNC connection
  boot_command = [
    "<up>e",                            # Modify GRUB entry
    "<down><down><end>",                # Navigate to kernel line
    " inst.ks=hd:LABEL=OEMDRV:/ks.cfg", # Set kickstart file location
    "<leftCtrlOn>x<leftCtrlOff>"        # Boot with modified command line
  ]
  boot_wait                 = "10s"     # Time to wait after boot starts
  installation_wait_timeout = "15m"     # Timeout for installation to complete

  # SSH configuration
  communicator      = "ssh"
  ssh_host          = "127.0.0.1"
  ssh_local_port    = 2020
  ssh_remote_port   = 22
  ssh_username      = "user"
  ssh_password      = "root"
  ssh_wait_timeout  = "20m"
}

build {
  sources = ["source.kubevirt-iso.rhel"]

  provisioner "shell" {
    inline = [
      "ls -la"
    ]
  }
}
