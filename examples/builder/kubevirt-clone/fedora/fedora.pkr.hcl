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

variable "namespace" {
  type    = string
  default = "images"
}

variable "source_volume_name" {
  type    = string
  default = "fedora-base"
}

variable "source_volume_namespace" {
  type    = string
  default = "images"
}

source "kubevirt-clone" "fedora-customized" {

  kube_config = var.kube_config
  # Name of the output DV
  name        = "fedora-customized"
  # Namespace of the output DV
  namespace   = var.namespace

  # Name of DV to be cloned
  source_volume_name = var.source_volume_name
  # Name namespace where the the DV to be cloned lives
  source_volume_namespace = var.source_volume_namespace

  disk_size     = "10Gi"
  instance_type = "o1.medium"
  preference    = "fedora"

  # Inject cloud-init credentials via media
  media_content = {
    "user-data" = <<-EOF
      #cloud-config
      chpasswd:
        expire: false
      password: password
      user: fedora
    EOF
    "meta-data" = <<-EOF
      instance-id: fedora-customized
      local-hostname: fedora-customized
    EOF
  }
  media_label = "cidata"

  communicator = "ssh"
  ssh_username = "fedora"
  ssh_password = "password"
  ssh_timeout  = "10m"
}

build {
  sources = ["source.kubevirt-clone.fedora-customized"]

  provisioner "shell" {
    inline = [
      "echo 'Clone builder provisioner ran successfully'",
      "uname -a",
    ]
  }
}
