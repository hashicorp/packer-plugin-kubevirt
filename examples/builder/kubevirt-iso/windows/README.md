Create a Windows 11 golden Image


1. Configure commandline environment: `export KUBECONFIG=....`
2. Create new OpenShift project: `oc new-project images && oc project images`
3. Trigger import of the Windows 11 Iso: `oc apply -f windows11-iso.yaml`
4. Check if the import suceeded: `oc get pvc/windows-11-x86-64-iso -o yaml`
5. `AcceptEula` in `autounattend.xml`
6. Download the packer-plugin: `packer init windows.pkr.hcl`
7. Trigger the build: `packer build windows.pkr.hcl`
