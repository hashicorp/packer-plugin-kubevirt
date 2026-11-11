# Example of Packer Templates

- [Fedora](./fedora/)
- [Windows](./windows/)

### Prerequisites

The `kubevirt-clone` builder clones an existing DataVolume to create a
customized bootable volume. The source DataVolume must already exist and
be in `Succeeded` phase before running the builder.

The source can live in the same namespace as the output or in a different
one (e.g. a shared images namespace). Set `source_volume_namespace` when
the source is in a different namespace than the build.

### Example Usage

> Ensure you are logged in to the Kubernetes cluster and that KubeVirt is installed.

```shell
# Export variable below that is used by the Packer builder
$ export KUBECONFIG=~/.kube/config

# Ensure the source DataVolume exists in the target namespace
$ kubectl get dv <source-volume-name> -n <namespace>

# Run the Packer builder
$ packer build -var "source_volume_name=<source-volume-name>" <template>.pkr.hcl
```

**Note**: The `disk_size` must be equal to or greater than the source
volume size. When the source and target are in different namespaces, the
cluster must support cross-namespace cloning.
