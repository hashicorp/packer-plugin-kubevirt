Customize a Windows 11 golden Image

This example clones an existing generalized (sysprepped) Windows 11 DataVolume,
boots it with `unattend.xml` attached as media to skip OOBE and enable WinRM,
runs a PowerShell provisioner, and re-generalizes the result with sysprep.

1. Follow all steps described in 
   [kubevirt-iso Windows example](../../kubevirt-iso/windows/).
2. Ensure the source DataVolume is ready: `oc wait --for=condition=Ready --timeout=600s dv/windows-11-rand-575 -n images`
3. Set `HideEULAPage` to `true` in `unattend.xml` to accept the EULA
4. Trigger the build: `packer build -var "source_volume_name=windows-11-rand-575" windows.pkr.hcl`