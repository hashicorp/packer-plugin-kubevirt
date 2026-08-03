Type: `kubevirt-iso`
Artifact BuilderId: `kubevirt.iso`

The KubeVirt ISO builder creates VM image inside a Kubernetes cluster from
ISO file. The builder supports Linux and Windows operating systems. Provisioning is done
through SSH or WinRM once the guest is installed.

---

## Basic Example

Here is a basic example showing how to build a Linux VM image using a Fedora ISO:

```hcl
source "kubevirt-iso" "fedora" {
  # Kubernetes configuration
  kube_config     = "~/.kube/config"
  name            = "fedora-42-rand-85"
  namespace       = "vm-images"
  iso_volume_name = "fedora-42-x86-64-iso"

  # Temporary VM type and preferences
  disk_size     = "10Gi"
  instance_type = "o1.medium"
  preference    = "fedora"

  # Timeout for installation to complete
  installation_wait_timeout = "15m"
}

build {
  sources = ["source.kubevirt-iso.fedora"]
}
```

## KubeVirt-ISO Builder Configuration Reference

### Required Configuration

<!-- Code generated from the comments of the Config struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

- `kube_config` (string) - KubeConfig is the path to the kubeconfig file.

- `name` (string) - Name is the name of the VM image.

- `namespace` (string) - Namespace is the namespace in which to create the VM image.

- `iso_volume_name` (string) - ISO Volume Name is the name of the DataVolume resource that contains the installation ISO.
  This DataVolume must already exist in the namespace.

- `disk_size` (string) - DiskSize is the size of the root disk to of the temporary VM.

- `instance_type` (string) - InstanceType is the name of the InstanceType resource to use in the temporary VM.

- `preference` (string) - Preference is the name of the Preference resource to use in the temporary VM.

- `installation_wait_timeout` (duration string | ex: "1h5m2s") - InstallationWaitTimeout is the amount of time to wait for the installation to be completed.

<!-- End of code generated from the comments of the Config struct in builder/kubevirt/iso/config.go; -->


### Not Required Configuration

<!-- Code generated from the comments of the Config struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

- `instance_type_kind` (string) - InstanceTypeKind is the kind of the InstanceType resource to use in the temporary VM.
  Other supported value is "virtualmachineclusterinstancetype".

- `preference_kind` (string) - PreferenceKind is the kind of the Preference resource to use in the temporary VM.
  Other supported value is "virtualmachineclusterpreference".

- `os_type` (string) - OperatingSystemType is the type of operating system to install.
  Supported values are "linux" and "windows". Default is "linux".

- `disk_bus` (string) - DiskBus is the bus type to use for CD-ROM disk devices on the temporary VM.
  Supported values are "scsi", "sata", and "virtio".
  Defaults to "scsi", which is compatible with both x86 and arm64 architectures.
  Use "sata" on x86 clusters if required by your storage configuration.

- `networks` ([]Network) - Networks is a list of networks to attach to the temporary VM.
  If no networks are specified, a single pod network will be used.

- `media_files` ([]string) - MediaFiles is a path list of files to be copied and used during the ISO installation.

- `boot_command` ([]string) - BootCommand is a list of strings that represent the keystrokes to be sent to the VM console
  to automate the installation via a new VNC connection.

- `boot_wait` (duration string | ex: "1h5m2s") - BootWait is the amount of time to wait before sending the boot command.
  This is useful if the VM takes some time to boot and be ready to accept keystrokes.

- `communicator` (string) - Communicator is the type of communicator to use to connect to the VM.
  Supported values are "ssh" and "winrm".

- `ssh_host` (string) - SSHHost is the hostname or IP address to use to connect via SSH.

- `ssh_local_port` (int) - SSHLocalPort is the local port to use to connect via SSH.

- `ssh_remote_port` (int) - SSHRemotePort is the remote port to use to connect via SSH.

- `ssh_username` (string) - SSHUsername is the username to use to connect via SSH.

- `ssh_password` (string) - SSHPassword is the password to use to connect via SSH.

- `ssh_wait_timeout` (duration string | ex: "1h5m2s") - SSHWaitTimeout is the amount of time to wait for the SSH service to be available.

- `winrm_host` (string) - WinRMHost is the hostname or IP address to use to connect via WinRM.

- `winrm_local_port` (int) - WinRMLocalPort is the local port to use to connect via WinRM.

- `winrm_remote_port` (int) - WinRMRemotePort is the remote port to use to connect via WinRM.

- `winrm_username` (string) - WinRMUsername is the username to use to connect via WinRM.

- `winrm_password` (string) - WinRMPassword is the password to use to connect via WinRM.

- `winrm_wait_timeout` (duration string | ex: "1h5m2s") - WinRMWaitTimeout is the amount of time to wait for the WinRM service to be available.

- `keep_vm` (bool) - KeepVM indicates whether to keep the temporary VM after the image has been created.
  If false, the VM and all its resources will be deleted after the image is created.
  If true, only the VM resource will be kept, all other resources will be deleted.
  Default is false.
  
  This can be useful for debugging purposes, to inspect the VM and its disks.
  However, it is recommended to set this to false in production environments to avoid
  resource leaks.

- `skip_create_image` (bool) - SkipCreateImage when set to true skips creating the final bootable volume
  DataSource and does not register the artifact with HCP Packer. This is
  useful for iterative debugging when you do not want to produce a final image.
  Default is false.

<!-- End of code generated from the comments of the Config struct in builder/kubevirt/iso/config.go; -->


### Network Configuration

<!-- Code generated from the comments of the Network struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

Network represents a network type and a resource that should be connected to the VM.
Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_network

<!-- End of code generated from the comments of the Network struct in builder/kubevirt/iso/config.go; -->

<!-- Code generated from the comments of the Network struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

- `name` (string) - Network name.
  Must be a DNS_LABEL and unique within the VM.
  More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names

<!-- End of code generated from the comments of the Network struct in builder/kubevirt/iso/config.go; -->


<!-- Code generated from the comments of the NetworkSource struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

Represents the source resource that will be connected to the VM.
Only one of its members may be specified.

<!-- End of code generated from the comments of the NetworkSource struct in builder/kubevirt/iso/config.go; -->

<!-- Code generated from the comments of the NetworkSource struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

- `pod` (\*PodNetwork) - Pod

- `multus` (\*MultusNetwork) - Multus

<!-- End of code generated from the comments of the NetworkSource struct in builder/kubevirt/iso/config.go; -->


<!-- Code generated from the comments of the PodNetwork struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

Represents the stock pod network interface.
Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_podnetwork

<!-- End of code generated from the comments of the PodNetwork struct in builder/kubevirt/iso/config.go; -->

<!-- Code generated from the comments of the PodNetwork struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

- `vmNetworkCIDR` (string) - CIDR for VM network.
  Default 10.0.2.0/24 if not specified.

- `vmIPv6NetworkCIDR` (string) - IPv6 CIDR for the VM network.
  Defaults to fd10:0:2::/120 if not specified.

<!-- End of code generated from the comments of the PodNetwork struct in builder/kubevirt/iso/config.go; -->


<!-- Code generated from the comments of the MultusNetwork struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

Represents the multus CNI network.
Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_multusnetwork

<!-- End of code generated from the comments of the MultusNetwork struct in builder/kubevirt/iso/config.go; -->

<!-- Code generated from the comments of the MultusNetwork struct in builder/kubevirt/iso/config.go; DO NOT EDIT MANUALLY -->

- `networkName` (string) - References to a NetworkAttachmentDefinition CRD object. Format:
  <networkName>, <namespace>/<networkName>. If namespace is not
  specified, VMI namespace is assumed.

- `default` (bool) - Select the default network and add it to the
  multus-cni.io/default-network annotation.

<!-- End of code generated from the comments of the MultusNetwork struct in builder/kubevirt/iso/config.go; -->
