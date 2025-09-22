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

- `networks` ([]Network) - Networks is a list of networks to attach to the temporary VM.
  If no networks are specified, a single pod network will be used.

- `media_files` ([]string) - MediaFiles is a path list of files to be copied and used during the ISO installation.

- `boot_command` ([]string) - BootCommand is a list of strings that represent the keystrokes to be sent to the VM console
  to automate the installation via a new VNC connection.

- `boot_wait` (duration string | ex: "1h5m2s") - BootWait is the amount of time to wait before sending the boot command.
  This is useful if the VM takes some time to boot and be ready to accept keystrokes.

- `ssh_local_port` (int) - SSHLocalPort is the local port to use to connect via SSH.

- `winrm_local_port` (int) - WinRMLocalPort is the local port to use to connect via WinRM.

- `keep_vm` (bool) - KeepVM indicates whether to keep the temporary VM after the image has been created.
  If false, the VM and all its resources will be deleted after the image is created.
  If true, only the VM resource will be kept, all other resources will be deleted.
  Default is false.
  
  This can be useful for debugging purposes, to inspect the VM and its disks.
  However, it is recommended to set this to false in production environments to avoid
  resource leaks.

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


### Communicator Configuration

**Optional**:

##### Common

<!-- Code generated from the comments of the Config struct in communicator/config.go; DO NOT EDIT MANUALLY -->

- `communicator` (string) - Packer currently supports three kinds of communicators:
  
  -   `none` - No communicator will be used. If this is set, most
      provisioners also can't be used.
  
  -   `ssh` - An SSH connection will be established to the machine. This
      is usually the default.
  
  -   `winrm` - A WinRM connection will be established.
  
  In addition to the above, some builders have custom communicators they
  can use. For example, the Docker builder has a "docker" communicator
  that uses `docker exec` and `docker cp` to execute scripts and copy
  files.

- `pause_before_connecting` (duration string | ex: "1h5m2s") - We recommend that you enable SSH or WinRM as the very last step in your
  guest's bootstrap script, but sometimes you may have a race condition
  where you need Packer to wait before attempting to connect to your
  guest.
  
  If you end up in this situation, you can use the template option
  `pause_before_connecting`. By default, there is no pause. For example if
  you set `pause_before_connecting` to `10m` Packer will check whether it
  can connect, as normal. But once a connection attempt is successful, it
  will disconnect and then wait 10 minutes before connecting to the guest
  and beginning provisioning.

<!-- End of code generated from the comments of the Config struct in communicator/config.go; -->


##### SSH

<!-- Code generated from the comments of the SSH struct in communicator/config.go; DO NOT EDIT MANUALLY -->

- `ssh_host` (string) - The address to SSH to. This usually is automatically configured by the
  builder.

- `ssh_port` (int) - The port to connect to SSH. This defaults to `22`.

- `ssh_username` (string) - The username to connect to SSH with. Required if using SSH.

- `ssh_password` (string) - A plaintext password to use to authenticate with SSH.

- `ssh_ciphers` ([]string) - This overrides the value of ciphers supported by default by Golang.
  The default value is [
    "aes128-gcm@openssh.com",
    "chacha20-poly1305@openssh.com",
    "aes128-ctr", "aes192-ctr", "aes256-ctr",
  ]
  
  Valid options for ciphers include:
  "aes128-ctr", "aes192-ctr", "aes256-ctr", "aes128-gcm@openssh.com",
  "chacha20-poly1305@openssh.com",
  "arcfour256", "arcfour128", "arcfour", "aes128-cbc", "3des-cbc",

- `ssh_clear_authorized_keys` (bool) - If true, Packer will attempt to remove its temporary key from
  `~/.ssh/authorized_keys` and `/root/.ssh/authorized_keys`. This is a
  mostly cosmetic option, since Packer will delete the temporary private
  key from the host system regardless of whether this is set to true
  (unless the user has set the `-debug` flag). Defaults to "false";
  currently only works on guests with `sed` installed.

- `ssh_key_exchange_algorithms` ([]string) - If set, Packer will override the value of key exchange (kex) algorithms
  supported by default by Golang. Acceptable values include:
  "curve25519-sha256@libssh.org", "ecdh-sha2-nistp256",
  "ecdh-sha2-nistp384", "ecdh-sha2-nistp521",
  "diffie-hellman-group14-sha1", and "diffie-hellman-group1-sha1".

- `ssh_certificate_file` (string) - Path to user certificate used to authenticate with SSH.
  The `~` can be used in path and will be expanded to the
  home directory of current user.

- `ssh_pty` (bool) - If `true`, a PTY will be requested for the SSH connection. This defaults
  to `false`.

- `ssh_timeout` (duration string | ex: "1h5m2s") - The time to wait for SSH to become available. Packer uses this to
  determine when the machine has booted so this is usually quite long.
  Example value: `10m`.
  This defaults to `5m`, unless `ssh_handshake_attempts` is set.

- `ssh_disable_agent_forwarding` (bool) - If true, SSH agent forwarding will be disabled. Defaults to `false`.

- `ssh_handshake_attempts` (int) - The number of handshakes to attempt with SSH once it can connect.
  This defaults to `10`, unless a `ssh_timeout` is set.

- `ssh_bastion_host` (string) - A bastion host to use for the actual SSH connection.

- `ssh_bastion_port` (int) - The port of the bastion host. Defaults to `22`.

- `ssh_bastion_agent_auth` (bool) - If `true`, the local SSH agent will be used to authenticate with the
  bastion host. Defaults to `false`.

- `ssh_bastion_username` (string) - The username to connect to the bastion host.

- `ssh_bastion_password` (string) - The password to use to authenticate with the bastion host.

- `ssh_bastion_interactive` (bool) - If `true`, the keyboard-interactive used to authenticate with bastion host.

- `ssh_bastion_private_key_file` (string) - Path to a PEM encoded private key file to use to authenticate with the
  bastion host. The `~` can be used in path and will be expanded to the
  home directory of current user.

- `ssh_bastion_certificate_file` (string) - Path to user certificate used to authenticate with bastion host.
  The `~` can be used in path and will be expanded to the
  home directory of current user.

- `ssh_file_transfer_method` (string) - `scp` or `sftp` - How to transfer files, Secure copy (default) or SSH
  File Transfer Protocol.
  
  **NOTE**: Guests using Windows with Win32-OpenSSH v9.1.0.0p1-Beta, scp
  (the default protocol for copying data) returns a a non-zero error code since the MOTW
  cannot be set, which cause any file transfer to fail. As a workaround you can override the transfer protocol
  with SFTP instead `ssh_file_transfer_method = "sftp"`.

- `ssh_proxy_host` (string) - A SOCKS proxy host to use for SSH connection

- `ssh_proxy_port` (int) - A port of the SOCKS proxy. Defaults to `1080`.

- `ssh_proxy_username` (string) - The optional username to authenticate with the proxy server.

- `ssh_proxy_password` (string) - The optional password to use to authenticate with the proxy server.

- `ssh_keep_alive_interval` (duration string | ex: "1h5m2s") - How often to send "keep alive" messages to the server. Set to a negative
  value (`-1s`) to disable. Example value: `10s`. Defaults to `5s`.

- `ssh_read_write_timeout` (duration string | ex: "1h5m2s") - The amount of time to wait for a remote command to end. This might be
  useful if, for example, packer hangs on a connection after a reboot.
  Example: `5m`. Disabled by default.

- `ssh_remote_tunnels` ([]string) - Remote tunnels forward a port from your local machine to the instance.
  Format: ["REMOTE_PORT:LOCAL_HOST:LOCAL_PORT"]
  Example: "9090:localhost:80" forwards localhost:9090 on your machine to port 80 on the instance.

- `ssh_local_tunnels` ([]string) - Local tunnels forward a port from the instance to your local machine.
  Format: ["LOCAL_PORT:REMOTE_HOST:REMOTE_PORT"]
  Example: "8080:localhost:3000" allows the instance to access your local machine’s port 3000 via localhost:8080.

<!-- End of code generated from the comments of the SSH struct in communicator/config.go; -->


- `ssh_private_key_file` (string) - Path to a PEM encoded private key file to use to authenticate with SSH.
  The `~` can be used in path and will be expanded to the home directory
  of current user.


##### Windows Remote Management (WinRM)

<!-- Code generated from the comments of the WinRM struct in communicator/config.go; DO NOT EDIT MANUALLY -->

- `winrm_username` (string) - The username to use to connect to WinRM.

- `winrm_password` (string) - The password to use to connect to WinRM.

- `winrm_host` (string) - The address for WinRM to connect to.
  
  NOTE: If using an Amazon EBS builder, you can specify the interface
  WinRM connects to via
  [`ssh_interface`](/packer/integrations/hashicorp/amazon/latest/components/builder/ebs#ssh_interface)

- `winrm_no_proxy` (bool) - Setting this to `true` adds the remote
  `host:port` to the `NO_PROXY` environment variable. This has the effect of
  bypassing any configured proxies when connecting to the remote host.
  Default to `false`.

- `winrm_port` (int) - The WinRM port to connect to. This defaults to `5985` for plain
  unencrypted connection and `5986` for SSL when `winrm_use_ssl` is set to
  true.

- `winrm_timeout` (duration string | ex: "1h5m2s") - The amount of time to wait for WinRM to become available. This defaults
  to `30m` since setting up a Windows machine generally takes a long time.

- `winrm_use_ssl` (bool) - If `true`, use HTTPS for WinRM.

- `winrm_insecure` (bool) - If `true`, do not check server certificate chain and host name.

- `winrm_use_ntlm` (bool) - If `true`, NTLMv2 authentication (with session security) will be used
  for WinRM, rather than default (basic authentication), removing the
  requirement for basic authentication to be enabled within the target
  guest. Further reading for remote connection authentication can be found
  [here](https://msdn.microsoft.com/en-us/library/aa384295(v=vs.85).aspx).

<!-- End of code generated from the comments of the WinRM struct in communicator/config.go; -->
