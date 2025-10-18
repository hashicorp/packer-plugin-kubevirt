// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ptr "k8s.io/utils/ptr"

	v1 "kubevirt.io/api/core/v1"
	instancetypeapi "kubevirt.io/api/instancetype"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

func configMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data: data,
	}
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
	networks []Network,
	mediaLabel string,
	virtioContainer string,
	accessMode string,
	volumeMode string) *v1.VirtualMachine {

	vmNetworks := make([]v1.Network, len(networks))
	vmInterfaces := make([]v1.Interface, len(networks))

	if instanceTypeKind == "" {
		instanceTypeKind = instancetypeapi.ClusterSingularResourceName
	}

	if preferenceKind == "" {
		preferenceKind = instancetypeapi.ClusterSingularPreferenceResourceName
	}

	for i, n := range networks {
		vmNetworks[i], vmInterfaces[i] = convertToNetwork(n)
	}

	volModeType := convertVolumeMode(volumeMode)

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
							AccessModes: convertAccessMode(accessMode),
							VolumeMode:  &volModeType,
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
							Disks:      getVirtualMachineDisks(osType),
						},
					},
					Volumes: getVirtualMachineVolumes(name, isoVolumeName, osType, mediaLabel, virtioContainer),
				},
			},
		},
	}
}

func cloneVolume(volname, vmname, namespace, diskSize, accessMode, volumeMode string) *cdiv1.DataVolume {
	volModeType := convertVolumeMode(volumeMode)

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
				AccessModes: convertAccessMode(accessMode),
				VolumeMode:  &volModeType,
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

func getVirtualMachineDisks(osType string) []v1.Disk {
	rootdisk := uint(1)
	cdrom := uint(2)

	disks := []v1.Disk{
		{
			Name: "rootdisk",
			DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{},
			},
			BootOrder: &rootdisk,
		},
		{
			Name: "cdrom",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Tray: "closed",
					Bus:  "sata",
				},
			},
			BootOrder: &cdrom,
		},
	}

	// If Windows, we need to add the VirtIO container.
	// We do this here, instead of at the end of the list, to preserve
	// the Windows drive order with previous versions of this plugin
	// so references to drive letters in Autounattend.xml files
	// remain consistent:
	if osType == "windows" {
		disks = append(disks,
			v1.Disk{
				Name: "virtiocontainerdisk",
				DiskDevice: v1.DiskDevice{
					CDRom: &v1.CDRomTarget{
						Tray: "closed",
						Bus:  "sata",
					},
				},
			},
		)
	}

	// Finally, add the userdata disk:
	disks = append(disks,
		v1.Disk{
			Name: "userdata",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Tray: "closed",
					Bus:  "sata",
				},
			},
		},
	)

	return disks
}

func getVirtualMachineVolumes(name, isoVolumeName string, osType string, label string, virtio string) []v1.Volume {
	var osVols []v1.Volume

	volumes := []v1.Volume{
		{
			Name: "rootdisk",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: name + "-rootdisk",
				},
			},
		},
		{
			Name: "cdrom",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: isoVolumeName,
				},
			},
		},
	}

	if osType == "windows" {
		osVols = []v1.Volume{
			{
				Name: "userdata",
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
						Image: virtio,
					},
				},
			},
		}
	} else {
		osVols = []v1.Volume{
			{
				Name: "userdata",
				VolumeSource: v1.VolumeSource{
					ConfigMap: &v1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: name,
						},
						VolumeLabel: label,
					},
				},
			},
		}
	}

	return append(volumes, osVols...)
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

func convertAccessMode(accessMode string) []corev1.PersistentVolumeAccessMode {
	var mode corev1.PersistentVolumeAccessMode
	switch accessMode {
	case "", "ReadWriteOnce":
		mode = corev1.ReadWriteOnce
	case "ReadWriteMany":
		mode = corev1.ReadWriteMany
	}
	return []corev1.PersistentVolumeAccessMode{mode}
}

func convertVolumeMode(volumeMode string) corev1.PersistentVolumeMode {
	var mode corev1.PersistentVolumeMode
	switch volumeMode {
	case "", "Filesystem":
		mode = corev1.PersistentVolumeFilesystem
	case "Block":
		mode = corev1.PersistentVolumeBlock
	}
	return mode
}
