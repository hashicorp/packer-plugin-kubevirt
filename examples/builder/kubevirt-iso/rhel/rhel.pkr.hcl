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

source "kubevirt-iso" "rhel" {
  # Kubernetes configuration
  kube_config   = var.kube_config
  name          = "rhel-10-rand-95"
  namespace     = "images"

  # ISO configuration
  iso_volume_name = "rhel-10-x86-64-iso"

  # PVC configuratoin
  access_mode = "ReadWriteMany"
  volume_mode = "Block"

  # VM type and preferences
  disk_size          = "10Gi"
  instance_type      = "o1.medium"
  instance_type_kind = "virtualmachineclusterinstancetype" # or "virtualmachineinstancetype"
  preference         = "rhel.10"
  preference_kind    = "virtualmachineclusterpreference" # or "virtualmachinepreference"
  os_type            = "linux"

  # Files to include in the ISO installation
  # This shows a `media_content` example, but in this case
  # a file is just being read so it could have used the `media_files`
  # option instead
  media_content = {
    "ks.cfg" = file("./ks.cfg")
  }
  # The equivalent media_files specification:
  # media_files = [
  #   "./ks.cfg"
  # ]

  # Boot process configuration
  # A set of commands to send over VNC connection
  # This boots the installer which will look for `ks.cfg` on the
  # media volume defined via `media_content` above.
  boot_command = [
    "<up><enter>",
  ]

  # Alternative boot_command, explicitly specifying the ks.cfg location:
  # boot_command = [
  #   "<up>e",                            # Modify GRUB entry
  #   "<down><down><end>",                # Navigate to kernel line
  #   " inst.ks=hd:LABEL=OEMDRV:/ks.cfg", # Set kickstart file location
  #   "<leftCtrlOn>x<leftCtrlOff>"        # Boot with modified command line
  # ]
  boot_wait       = "10s"     # Time to wait after boot starts
  ip_wait_timeout = "15m"     # Timeout waiting for an IP address to be returned


  # SSH configuration
  communicator = "ssh"
  ssh_username = "user"
  ssh_password = "root"
  ssh_timeout  = "20m"
}

build {
  sources = ["source.kubevirt-iso.rhel"]

  provisioner "shell" {
    inline = [
      "ls -la"
    ]
  }
}
