# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

integration {
  name = "KubeVirt"
  description = "The KubeVirt plugin can be used with HashiCorp Packer to create KubeVirt images."
  identifier = "packer/hashicorp/kubevirt"
  flags = [""]
  component {
    type = "builder"
    name = "KubeVirt ISO"
    slug = "kubevirt-iso"
  }
}
