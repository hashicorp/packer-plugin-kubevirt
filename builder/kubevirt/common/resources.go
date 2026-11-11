// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ptr "k8s.io/utils/ptr"

	instancetypeapi "kubevirt.io/api/instancetype"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

const ImmediateBindingAnnotation = "cdi.kubevirt.io/storage.bind.immediate.requested"

// PersistentVolumeClaimSpec returns the claim of the root disk and of the
// bootable volume cloned from it. Unset options keep the historical defaults:
// ReadWriteOnce, and the default storage class and volume mode.
func PersistentVolumeClaimSpec(diskSize, storageClassName, accessMode, volumeMode string) *corev1.PersistentVolumeClaimSpec {
	am := corev1.ReadWriteOnce
	if accessMode != "" {
		am = corev1.PersistentVolumeAccessMode(accessMode)
	}

	spec := &corev1.PersistentVolumeClaimSpec{
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(diskSize),
			},
		},
		AccessModes: []corev1.PersistentVolumeAccessMode{am},
	}
	if storageClassName != "" {
		spec.StorageClassName = ptr.To(storageClassName)
	}
	if volumeMode != "" {
		spec.VolumeMode = ptr.To(corev1.PersistentVolumeMode(volumeMode))
	}
	return spec
}

func CloneVolume(name, namespace, diskSize, storageClassName, accessMode, volumeMode string) *cdiv1.DataVolume {
	return &cdiv1.DataVolume{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cdiv1.CDIGroupVersionKind.GroupVersion().String(),
			Kind:       "DataVolume",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Annotations: map[string]string{
				ImmediateBindingAnnotation: "true",
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				PVC: &cdiv1.DataVolumeSourcePVC{
					Name:      name + "-rootdisk",
					Namespace: namespace,
				},
			},
			PVC: PersistentVolumeClaimSpec(diskSize, storageClassName, accessMode, volumeMode),
		},
	}
}

func SourceVolume(name, namespace, instanceType, instanceTypeKind, preferenceName, preferenceKind string) *cdiv1.DataSource {
	labels := map[string]string{
		instancetypeapi.DefaultInstancetypeLabel: instanceType,
		instancetypeapi.DefaultPreferenceLabel:   preferenceName,
	}

	if instanceTypeKind != "" && !IsSupportedKind(instanceTypeKind, instancetypeapi.ClusterSingularResourceName) {
		labels[instancetypeapi.DefaultInstancetypeKindLabel] = instanceTypeKind
	}
	if preferenceKind != "" && !IsSupportedKind(preferenceKind, instancetypeapi.ClusterSingularPreferenceResourceName) {
		labels[instancetypeapi.DefaultPreferenceKindLabel] = preferenceKind
	}

	return &cdiv1.DataSource{
		TypeMeta: metav1.TypeMeta{
			APIVersion: cdiv1.CDIGroupVersionKind.GroupVersion().String(),
			Kind:       "DataSource",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
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

// IsSupportedKind mirrors how KubeVirt resolves instancetype and preference
// matcher kinds: case-insensitively, by singular or plural resource name.
func IsSupportedKind(kind string, singularNames ...string) bool {
	kind = strings.ToLower(kind)
	for _, name := range singularNames {
		if kind == name || kind == name+"s" {
			return true
		}
	}
	return false
}

// ConfigMap builds a ConfigMap from the media files and inline content.
func ConfigMap(name string, mediaFiles []string, mediaContent map[string]string) (*corev1.ConfigMap, error) {
	data := make(map[string]string, len(mediaContent))
	binaryData := make(map[string][]byte)

	for filename, content := range mediaContent {
		data[filename] = content
	}

	for _, path := range mediaFiles {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		// ConfigMap data must be UTF-8, other content (e.g. Windows driver
		// files) would be corrupted when serialized, so store it as binary data.
		filename := filepath.Base(path)
		if utf8.Valid(content) {
			data[filename] = string(content)
		} else {
			binaryData[filename] = content
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Data:       data,
		BinaryData: binaryData,
	}, nil
}
