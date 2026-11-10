// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone

import (
	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ptr "k8s.io/utils/ptr"

	v1 "kubevirt.io/api/core/v1"
	instancetypeapi "kubevirt.io/api/instancetype"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

func virtualMachine(
	name,
	namespace,
	sourceVolumeName,
	sourceVolumeNamespace,
	diskSize,
	instanceType,
	preferenceName,
	instanceTypeKind,
	preferenceKind,
	diskBus,
	mediaLabel string,
	hasMedia bool,
	networks []Network,
	storage kubevirtcommon.StorageConfig) *v1.VirtualMachine {

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

	disks := []v1.Disk{
		{
			Name: "rootdisk",
			DiskDevice: v1.DiskDevice{
				Disk: &v1.DiskTarget{},
			},
		},
	}

	volumes := []v1.Volume{
		{
			Name: "rootdisk",
			VolumeSource: v1.VolumeSource{
				DataVolume: &v1.DataVolumeSource{
					Name: name + "-rootdisk",
				},
			},
		},
	}

	if hasMedia {
		disks = append(disks, v1.Disk{
			Name: "media",
			DiskDevice: v1.DiskDevice{
				CDRom: &v1.CDRomTarget{
					Bus:  v1.DiskBus(diskBus),
					Tray: "closed",
				},
			},
		})
		volumes = append(volumes, v1.Volume{
			Name: "media",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: name,
					},
					VolumeLabel: mediaLabel,
				},
			},
		})
	}

	return &v1.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v1.GroupVersion.String(),
			Kind:       "VirtualMachine",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
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
						PVC: kubevirtcommon.PersistentVolumeClaimSpec(diskSize, storage.StorageClassName, storage.AccessMode, storage.VolumeMode),
						Source: &cdiv1.DataVolumeSource{
							PVC: &cdiv1.DataVolumeSourcePVC{
								Name:      sourceVolumeName,
								Namespace: sourceVolumeNamespace,
							},
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

func convertToNetwork(n Network) (v1.Network, v1.Interface) {
	vmNetwork := v1.Network{Name: n.Name}
	vmInterface := v1.Interface{Name: n.Name}

	switch {
	case n.Pod != nil:
		vmNetwork.NetworkSource.Pod = &v1.PodNetwork{
			VMNetworkCIDR:     n.Pod.VMNetworkCIDR,
			VMIPv6NetworkCIDR: n.Pod.VMIPv6NetworkCIDR,
		}
		vmInterface.InterfaceBindingMethod.Masquerade = &v1.InterfaceMasquerade{}
	case n.Multus != nil:
		vmNetwork.NetworkSource.Multus = &v1.MultusNetwork{
			NetworkName: n.Multus.NetworkName,
			Default:     n.Multus.Default,
		}
		vmInterface.InterfaceBindingMethod.Bridge = &v1.InterfaceBridge{}
	default:
		vmNetwork.NetworkSource.Pod = &v1.PodNetwork{}
		vmInterface.InterfaceBindingMethod.Masquerade = &v1.InterfaceMasquerade{}
	}
	return vmNetwork, vmInterface
}
