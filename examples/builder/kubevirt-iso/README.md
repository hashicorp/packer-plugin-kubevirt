# Example of Packer Templates

- [Fedora](./fedora/)
- [RHEL](./rhel/)
- [SLES](./sles/)
- [Ubuntu](./ubuntu/)
- [Windows](./windows/)

### Example Usage

> 💡 Ensure you are logged in to the Kubernetes cluster and that KubeVirt is installed.

Change to the directory that contains the relevant files, and then run:

```shell
# Export variable below that is used by the Packer builder
$ export KUBECONFIG=~/.kube/config

# Deploy a DataVolume to import the ISO to the Kubernetes cluster
$ kubectl apply -f ${ISO_NAME}.yaml

# Run the Packer builder
$ packer build ${TEMPLATE_NAME}.pkr.hcl
```

**Note**: Ensure you have deployed everything in the same Kubernetes namespace.
