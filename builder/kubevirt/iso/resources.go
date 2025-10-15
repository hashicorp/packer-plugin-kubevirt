// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ptr "k8s.io/utils/ptr"

	v1 "kubevirt.io/api/core/v1"
	instancetypeapi "kubevirt.io/api/instancetype"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

func configMap(name string, mediaFiles []string) (*corev1.ConfigMap, error) {
	data := make(map[string]string)

	for _, path := range mediaFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		filename := filepath.Base(path)
		data[filename] = string(content)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: data,
	}, nil
}

func virtualMachine(
	name,
	isoVolumeName,
	diskSize,
	instanceType,
	preferenceName,
	instanceTypeKind,
	preferenceKind,
	osType string,
	networks []Network) *v1.VirtualMachine {
	var disks []v1.Disk
	var volumes []v1.Volume

	vmNetworks := make([]v1.Network, len(networks))
	vmInterfaces := make([]v1.Interface, len(networks))

	if instanceTypeKind == "" {
		instanceTypeKind = instancetypeapi.ClusterSingularResourceName
	}

	if preferenceKind == "" {
		preferenceKind = instancetypeapi.ClusterSingularPreferenceResourceName
	}

	if osType == "linux" {
		disks = getLinuxVirtualMachineDisks()
		volumes = getLinuxVirtualMachineVolumes(name, isoVolumeName)
	}

	if osType == "windows" {
		disks = getWindowsVirtualMachineDisks()
		volumes = getWindowsVirtualMachineVolumes(name, isoVolumeName)
	}

	for i, n := range networks {
		vmNetworks[i], vmInterfaces[i] = convertToNetwork(n)
	}

	return &v1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.GroupVersion.String(),
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: v1.VirtualMachineSpec{
			RunStrategy: ptr.To(v1.RunStrategyAlways),
			Instancetype: &v1.InstancetypeMatcher{
				Kind: instanceTypeKind,
				Name: instanceType,
			},
			Preference: &v1.PreferenceMatcher{
				Kind: preferenceKind,
				Name: preferenceName,
			},
			DataVolumeTemplates: []v1.DataVolumeTemplateSpec{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: name + "-rootdisk",
					},
					Spec: cdiv1.DataVolumeSpec{
						PVC: &corev1.PersistentVolumeClaimSpec{
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(diskSize),
								},
							},
							AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						},
						Source: &cdiv1.DataVolumeSource{
							Blank: &cdiv1.DataVolumeBlankImage{},
						},
					},
				},
			},
			Template: &v1.VirtualMachineInstanceTemplateSpec{
				Spec: v1.VirtualMachineInstanceSpec{
					Networks: vmNetworks,
					Domain: v1.DomainSpec{
						Devices: v1.Devices{
							Interfaces: vmInterfaces,
							Disks:      disks,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
}

func cloneVolume(volname, vmname, namespace, diskSize string) *cdiv1.DataVolume {
	return &cdiv1.DataVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cdiv1.CDIGroupVersionKind.GroupVersion().String(),
			Kind:       "DataVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: volname,
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				PVC: &cdiv1.DataVolumeSourcePVC{
					Name:      vmname + "-rootdisk",
					Namespace: namespace,
				},
			},
			PVC: &corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceName(corev1.ResourceStorage): resource.MustParse(diskSize),
					},
				},
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			},
		},
	}
}

func sourceVolume(name, namespace, instanceType, preferenceName string) *cdiv1.DataSource {
	return &cdiv1.DataSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cdiv1.CDIGroupVersionKind.GroupVersion().String(),
			Kind:       "DataSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				"instancetype.kubevirt.io/default-instancetype": instanceType,
				"instancetype.kubevirt.io/default-preference":   preferenceName,
			},
		},
		Spec: cdiv1.DataSourceSpec{
			Source: cdiv1.DataSourceSource{
				PVC: &cdiv1.DataVolumeSourcePVC{
					Name:      name,
					Namespace: namespace,
				},
			},
		},
	}
}

func getLinuxVirtualMachineDisks() []v1.Disk {
	rootdisk := uint(1)
	cdrom := uint(2)
	oemdrv := uint(3)

	return []v1.Disk{
		{
			Name: "cdrom",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Tray: "closed",
				},
			},
			BootOrder: &cdrom,
		},
		{
			Name: "oemdrv",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Tray: "closed",
				},
			},
			BootOrder: &oemdrv,
		},
		{
			Name: "rootdisk",
			DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{},
			},
			BootOrder: &rootdisk,
		},
	}
}

func getLinuxVirtualMachineVolumes(name, isoVolumeName string) []v1.Volume {
	return []v1.Volume{
		{
			Name: "cdrom",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: isoVolumeName,
				},
			},
		},
		{
			Name: "rootdisk",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: name + "-rootdisk",
				},
			},
		},
		{
			Name: "oemdrv",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					VolumeLabel: "OEMDRV",
				},
			},
		},
	}
}

func getWindowsVirtualMachineDisks() []v1.Disk {
	rootdisk := uint(1)
	cdrom := uint(2)

	return []v1.Disk{
		{
			Name: "cdrom",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Bus: "sata",
				},
			},
			BootOrder: &cdrom,
		},
		{
			Name: "rootdisk",
			DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{},
			},
			BootOrder: &rootdisk,
		},
		{
			Name: "virtiocontainerdisk",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Bus: "sata",
				},
			},
		},
		{
			Name: "sysprep",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Bus: "sata",
				},
			},
		},
	}
}

func getWindowsVirtualMachineVolumes(name, isoVolumeName string) []v1.Volume {
	return []v1.Volume{
		{
			Name: "cdrom",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: isoVolumeName,
				},
			},
		},
		{
			Name: "rootdisk",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: name + "-rootdisk",
				},
			},
		},
		{
			Name: "sysprep",
			VolumeSource: v1.VolumeSource{
				Sysprep: &v1.SysprepSource{
					ConfigMap: &corev1.LocalObjectReference{
						Name: name,
					},
				},
			},
		},
		{
			Name: "virtiocontainerdisk",
			VolumeSource: v1.VolumeSource{
				ContainerDisk: &v1.ContainerDiskSource{
					Image: "quay.io/kubevirt/virtio-container-disk:v1.5.2",
				},
			},
		},
	}
}

func convertToNetwork(n Network) (v1.Network, v1.Interface) {
	vmNetwork := v1.Network{Name: n.Name}
	vmInterface := v1.Interface{Name: n.Name}

	switch {
	case n.Pod != nil:
		// Pod network, and masquerade interface.
		vmNetwork.NetworkSource.Pod = &v1.PodNetwork{
			VMNetworkCIDR:     n.Pod.VMNetworkCIDR,
			VMIPv6NetworkCIDR: n.Pod.VMIPv6NetworkCIDR,
		}
		vmInterface.InterfaceBindingMethod.Masquerade = &v1.InterfaceMasquerade{}
	case n.Multus != nil:
		// Multus network, and bridge interface.
		vmNetwork.NetworkSource.Multus = &v1.MultusNetwork{
			NetworkName: n.Multus.NetworkName,
			Default:     n.Multus.Default,
		}
		vmInterface.InterfaceBindingMethod.Bridge = &v1.InterfaceBridge{}
	}
	return vmNetwork, vmInterface
}
