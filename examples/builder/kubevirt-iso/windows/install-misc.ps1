# Copyright IBM Corp. 2025
# SPDX-License-Identifier: MPL-2.0

# Install QEMU Tools (Drivers)
Start-Process msiexec -Wait -ArgumentList "/i E:\virtio-win-gt-x64.msi /qn /passive /norestart"

# Install QEMU Guest Agent
Start-Process msiexec -Wait -ArgumentList "/i E:\guest-agent\qemu-ga-x86_64.msi /qn /passive /norestart"

# Rename cached unattend.xml to avoid it is picked up by sysprep
mv C:\Windows\Panther\unattend.xml C:\Windows\Panther\unattend.install.xml
