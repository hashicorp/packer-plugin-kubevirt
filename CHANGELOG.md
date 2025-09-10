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

