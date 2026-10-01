## 1.0.0 (October 1, 2026)

### **NOTES:**
* The builder now uses the standard Packer communicator options. `ssh_remote_port`,
  `ssh_wait_timeout`, `winrm_remote_port` and `winrm_wait_timeout` are deprecated in favour of
  `ssh_port`, `ssh_timeout`, `winrm_port` and `winrm_timeout`; the old options keep working
  with a warning [GH-57](https://github.com/hashicorp/packer-plugin-kubevirt/pull/57)
* `installation_wait_timeout` is now required when no communicator is configured, since the
  VM would otherwise be stopped before the installation completes [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* The configuration is validated by `packer validate`, so templates with invalid values now
  fail validation instead of failing during the build [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* Provisioners now always run: `shell-local` works without a communicator, and remote
  provisioners fail instead of being skipped silently [GH-57](https://github.com/hashicorp/packer-plugin-kubevirt/pull/57)
* The artifact ID is now `<namespace>/<name>` [GH-49](https://github.com/hashicorp/packer-plugin-kubevirt/pull/49)

### **FEATURES:**
* HCP Packer registry support for the bootable volume, and `build.BootableVolumeName` for
  provisioners [GH-49](https://github.com/hashicorp/packer-plugin-kubevirt/pull/49)
* `skip_create_image` to build without creating the bootable volume [GH-49](https://github.com/hashicorp/packer-plugin-kubevirt/pull/49)
* `disk_bus` to select the bus of the CD-ROM devices (`scsi`, `sata` or `usb`) [GH-49](https://github.com/hashicorp/packer-plugin-kubevirt/pull/49)
* `media_label` to set the volume label of the media disk, e.g. `cidata` for cloud-init [GH-54](https://github.com/hashicorp/packer-plugin-kubevirt/pull/54)
* Standard Packer SSH and WinRM communicator options, such as `ssh_private_key_file`,
  `ssh_agent_auth`, `ssh_timeout` and `winrm_use_ssl` [GH-57](https://github.com/hashicorp/packer-plugin-kubevirt/pull/57)
* `media_content` to add inline files, e.g. rendered with `templatefile()`, to the media disk [GH-58](https://github.com/hashicorp/packer-plugin-kubevirt/pull/58)
* `virtio_container_image` to set the VirtIO drivers image of Windows builds [GH-58](https://github.com/hashicorp/packer-plugin-kubevirt/pull/58)
* `storage_class_name`, `access_mode` and `volume_mode` for the root disk and the bootable
  volume [GH-59](https://github.com/hashicorp/packer-plugin-kubevirt/pull/59)

### **IMPROVEMENTS:**
* Validate the configuration in `packer validate`: required options, Kubernetes names,
  `disk_size`, `os_type`, `disk_bus`, instance type and preference kinds, network names and
  `boot_command` [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* Expand `~` in `kube_config` [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* The port forward to the VM listens on `127.0.0.1` and a free local port by default, so
  `ssh_host`, `ssh_local_port`, `winrm_host` and `winrm_local_port` are optional [GH-57](https://github.com/hashicorp/packer-plugin-kubevirt/pull/57)
* Show the last port forwarding error when the communicator cannot connect [GH-57](https://github.com/hashicorp/packer-plugin-kubevirt/pull/57)
* Validate `media_files` up front: the files must exist, have unique names and fit in a
  ConfigMap [GH-58](https://github.com/hashicorp/packer-plugin-kubevirt/pull/58)
* Fail before the installation when a DataVolume, DataSource or PVC with the image name
  already exists [GH-59](https://github.com/hashicorp/packer-plugin-kubevirt/pull/59)

### **BUG FIXES:**
* Report the error of the failed step: failed builds ended with a misleading error, or were
  reported as successful with `skip_create_image` [GH-55](https://github.com/hashicorp/packer-plugin-kubevirt/pull/55)
* Do not delete an existing ConfigMap or VM with the build name when the build fails to
  create it [GH-55](https://github.com/hashicorp/packer-plugin-kubevirt/pull/55)
* Close the VNC connection once the boot command is typed, and do not open it without a
  boot command [GH-55](https://github.com/hashicorp/packer-plugin-kubevirt/pull/55)
* Retry stopping the VM when the update conflicts with a concurrent change [GH-55](https://github.com/hashicorp/packer-plugin-kubevirt/pull/55)
* Set a bus on the Linux CD-ROM devices, which KubeVirt rejected on arm64 clusters [GH-49](https://github.com/hashicorp/packer-plugin-kubevirt/pull/49)
* An invalid `disk_size` crashed the plugin [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* `os_type` now defaults to `linux`, as documented [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* Networks without a type use the pod network, as documented [GH-56](https://github.com/hashicorp/packer-plugin-kubevirt/pull/56)
* The port forward listened on all network interfaces when no host was set, used port 0
  when no local port was set, and was never closed [GH-57](https://github.com/hashicorp/packer-plugin-kubevirt/pull/57)
* Media files that are not UTF-8 text were corrupted, and files with the same name
  overwrote each other [GH-58](https://github.com/hashicorp/packer-plugin-kubevirt/pull/58)
* Record namespaced instance type and preference kinds on the DataSource [GH-59](https://github.com/hashicorp/packer-plugin-kubevirt/pull/59)

### **SECURITY:**
* deps: bump Go to 1.26.8, github.com/hashicorp/packer-plugin-sdk to v0.6.11 and
  golang.org/x/crypto to v0.57.0 [GH-53](https://github.com/hashicorp/packer-plugin-kubevirt/pull/53)

## 0.9.0 (July 20, 2026)

### **BUG FIXES:**
* Port Forwarding: Fixed port forwarding connection handling to properly close listeners and continue on errors instead of returning [GH-29](https://github.com/hashicorp/packer-plugin-kubevirt/pull/29)
* DataVolume Binding: Added immediate binding annotation for DataVolumes to ensure proper volume provisioning [GH-43](https://github.com/hashicorp/packer-plugin-kubevirt/pull/43)

### **SECURITY:**
* Vulnerability Fixes: Bumped golang.org/x/crypto to v0.52.0 to address security vulnerabilities [GH-38](https://github.com/hashicorp/packer-plugin-kubevirt/pull/38)
* deps: bump github.com/hashicorp/packer-plugin-sdk to 0.6.10 [GH-38](https://github.com/hashicorp/packer-plugin-kubevirt/pull/38)

## 0.8.0
Migrated codebase from [kv-infra/packer-plugin-kubevirt](https://github.com/kv-infra/packer-plugin-kubevirt)
### IMPROVEMENTS:

* feat: create artifact from builder
  Create an artifact from the builder that
  could be used to trigger the post-processors.
  [GH-4](https://github.com/hashicorp/packer-plugin-kubevirt/pull/4)


### BUG FIXES:

* fix: typo in log messages of VM creation
  Changed from 'VirutalMachine' to 'VirtualMachine'.
  [GH-4](https://github.com/hashicorp/packer-plugin-kubevirt/pull/4)
  
* fix: avoid crash if kubeconfig is not set
  Packer crashes if the KubeConfig environment
  variable is not, instead it should just show an
  error and ask the user to set this variable.
  [GH-4](https://github.com/hashicorp/packer-plugin-kubevirt/pull/4)

