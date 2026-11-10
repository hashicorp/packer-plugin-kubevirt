# Copyright (c) Red Hat, Inc.
# SPDX-License-Identifier: MPL-2.0
#
# Test: clone a Windows base image, inject unattend.xml via media
# to skip OOBE, set Administrator password, and enable WinRM.

variable "kube_config" {
  type    = string
  default = env("KUBECONFIG")
}

variable "namespace" {
  type    = string
  default = "images"
}

variable "source_volume_name" {
  type = string
}

variable "source_volume_namespace" {
  type    = string
  default = "images"
}

variable "disk_size" {
  type    = string
  default = "64Gi"
}

source "kubevirt-clone" "windows" {
  kube_config = var.kube_config

  # Name of the output DV
  name        = "windows-cloned-1"
  # Namespace of the output DV
  namespace   = var.namespace

  # Name of the DV to be cloned
  source_volume_name      = var.source_volume_name
  # Name namespace where the the DV to be cloned lives
  source_volume_namespace = var.source_volume_namespace

  disk_size     = var.disk_size
  instance_type = "u1.large"
  preference    = "windows.11.virtio"
  disk_bus      = "sata"

  communicator   = "winrm"
  winrm_username = "Administrator"
  winrm_password = "shadowman"
  winrm_timeout  = "15m"

  media_files = [#
    # Note: To avoid License error, set "HideEULAPage" to "true" in the "unattend.xml" file.
    #
    # By setting "HideEULAPage" parameter to "true", you are agreeing to the
    # applicable Microsoft end user license agreement(s) for each deployment
    # or installation for the Microsoft product(s).
    #
    "./unattend.xml"
  ]
  media_label = "deployCD"
}

build {
  sources = ["source.kubevirt-clone.windows"]

  provisioner "powershell" {
    inline = [
      "Write-Output '=== Verifying Windows clone ==='",
      "Write-Output \"Hostname: $env:COMPUTERNAME\"",
      "Write-Output \"User: $env:USERNAME\"",
      "Write-Output \"TimeZone: $((Get-TimeZone).Id)\"",
      "Write-Output 'Windows clone provisioner ran successfully!'",
      "",
      # Windows 11 auto-enables BitLocker device encryption during OOBE when
      # it detects a TPM. Sysprep refuses to generalize an encrypted volume,
      # so we must disable BitLocker and wait for decryption to complete first.
      "Write-Output 'Disabling BitLocker before sysprep...'",
      "manage-bde -off C:",
      "do { Start-Sleep -Seconds 5; $s = manage-bde -status C: | Select-String 'Percentage Encrypted' } while ($s -notmatch '0\\.0%')",
      "Write-Output 'BitLocker decryption complete.'",
      "",
      "Write-Output 'Cleaning cached unattend files before sysprep...'",
      "Remove-Item -Force -ErrorAction SilentlyContinue C:\\Windows\\Panther\\unattend*.xml",
      "Remove-Item -Force -Recurse -ErrorAction SilentlyContinue C:\\Windows\\Panther\\Unattend\\",
      "Remove-Item -Force -Recurse -ErrorAction SilentlyContinue C:\\Windows\\Panther\\actionqueue\\",
      "Remove-Item -Force -ErrorAction SilentlyContinue C:\\Windows\\System32\\Sysprep\\unattend*.xml",
      "Remove-Item -Force -ErrorAction SilentlyContinue C:\\unattend*.xml",
    ]
  }

  # Re-sysprep so the final image is generalized. This resets the Administrator
  # password, disables AutoLogon, and removes the WinRM configuration set by
  # the unattend.xml used during this build.
  provisioner "windows-shell" {
    inline = [
      "C:\\Windows\\System32\\Sysprep\\sysprep.exe /generalize /oobe /quit /mode:vm"
    ]
  }
}
