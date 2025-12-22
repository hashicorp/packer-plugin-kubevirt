# Copyright IBM Corp. 2025
# SPDX-License-Identifier: MPL-2.0

integration {
  name = "KubeVirt"
  description = "The KubeVirt plugin can be used with HashiCorp Packer to create KubeVirt images."
  identifier = "packer/hashicorp/kubevirt"
  component {
    type = "builder"
    name = "KubeVirt ISO"
    slug = "iso"
  }
}
