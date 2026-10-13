// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type BuilderConfig,StorageConfig,PortForwardConfig

package common

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/pathing"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

	v1 "kubevirt.io/api/core/v1"
	instancetypeapi "kubevirt.io/api/instancetype"
)

const (
	// DefaultMediaLabel is the volume label that Anaconda (kickstart) auto-discovers.
	DefaultMediaLabel   = "OEMDRV"
	MaxMediaLabelLength = 32

	// MaxMediaSize is the maximum size of the data stored in a ConfigMap.
	MaxMediaSize = 1024 * 1024
)

// BuilderConfig holds the configuration fields shared by all KubeVirt builders.
// Each builder's Config embeds this struct with `mapstructure:",squash"` so that
// packer-sdc inlines the fields into the builder's HCL2 spec.
type BuilderConfig struct {
	// KubeConfig is the path to the kubeconfig file used to connect to the cluster.
	// A leading `~` is expanded to the home directory of the current user.
	KubeConfig string `mapstructure:"kube_config" required:"true"`
	// Name is the name of the VM image, used for the resulting DataVolume and DataSource
	// as well as for the temporary VM. The build stops before creating anything if a
	// DataVolume, DataSource or PersistentVolumeClaim with this name already exists.
	Name string `mapstructure:"name" required:"true"`
	// Namespace is the namespace in which to create the VM image.
	Namespace string `mapstructure:"namespace" required:"true"`
	// DiskSize is the size of the root disk of the temporary VM, as a Kubernetes
	// quantity, e.g. "10Gi".
	DiskSize string `mapstructure:"disk_size" required:"true"`
	// InstanceType is the name of the InstanceType resource to use in the temporary VM.
	// It is also recorded on the resulting DataSource as its default instance type,
	// which VMs created from it can infer.
	InstanceType string `mapstructure:"instance_type" required:"true"`
	// InstanceTypeKind is the kind of the InstanceType resource to use in the temporary VM.
	// Supported values are "virtualmachineclusterinstancetype" and "virtualmachineinstancetype".
	// Defaults to "virtualmachineclusterinstancetype".
	InstanceTypeKind string `mapstructure:"instance_type_kind" required:"false"`
	// Preference is the name of the Preference resource to use in the temporary VM.
	// It is also recorded on the resulting DataSource as its default preference,
	// which VMs created from it can infer.
	Preference string `mapstructure:"preference" required:"true"`
	// PreferenceKind is the kind of the Preference resource to use in the temporary VM.
	// Supported values are "virtualmachineclusterpreference" and "virtualmachinepreference".
	// Defaults to "virtualmachineclusterpreference".
	PreferenceKind string `mapstructure:"preference_kind" required:"false"`
	// DiskBus is the bus type to use for CD-ROM disk devices on the temporary VM.
	// Supported values are "scsi", "sata", and "usb". KubeVirt does not support
	// "virtio" for CD-ROM devices.
	// Defaults to "scsi", which is compatible with both x86 and arm64 architectures.
	// Use "sata" on x86 clusters if required by your storage configuration.
	DiskBus string `mapstructure:"disk_bus" required:"false"`
	// MediaFiles is a path list of files to be copied and used during the build.
	// The files are stored in a ConfigMap and attached to the VM as a disk, where each
	// file is named after its base name. The file names must therefore be unique, and
	// the files must add up to at most 1 MiB.
	MediaFiles []string `mapstructure:"media_files" required:"false"`
	// MediaContent is a map of file names to file contents to add to the media disk,
	// alongside `media_files`. This is useful to render configuration from
	// the template, for example with the `templatefile` function:
	//
	// ```hcl
	// media_content = {
	//   "ks.cfg" = templatefile("ks.cfg.pkrtpl", { password = var.password })
	// }
	// ```
	//
	// The content is used as is, without Packer template interpolation. Packer still
	// checks that it parses as a Go template though, so content such as Jinja
	// expressions (e.g. `{{ v1.local_hostname }}`) is rejected: use `media_files` for it.
	MediaContent map[string]string `mapstructure:"media_content" required:"false"`
	// MediaLabel is the volume label of the disk that holds the `media_files` and `media_content`.
	// Different installers discover their configuration through different labels, e.g.
	// "OEMDRV" for Anaconda kickstart (RHEL, Fedora) or "cidata" for cloud-init
	// NoCloud / Subiquity autoinstall (Ubuntu). Only applies when `os_type` is "linux".
	// Must be at most 32 characters long. Defaults to "OEMDRV".
	MediaLabel string `mapstructure:"media_label" required:"false"`

	StorageConfig     `mapstructure:",squash"`
	Comm              communicator.Config `mapstructure:",squash"`
	PortForwardConfig `mapstructure:",squash"`

	// KeepVM indicates whether to keep the temporary VM after the image has been created.
	// Default is false.
	KeepVM bool `mapstructure:"keep_vm" required:"false"`

	// SkipCreateImage when set to true skips creating the final bootable volume
	// DataSource and does not register the artifact with HCP Packer.
	// Default is false.
	SkipCreateImage bool `mapstructure:"skip_create_image" required:"false"`
}

// The following options configure the persistent volumes created by the builder:
// the root disk of the temporary VM and the bootable volume cloned from it.
type StorageConfig struct {
	// StorageClassName is the name of the StorageClass of the volumes.
	// Defaults to the default StorageClass of the cluster.
	StorageClassName string `mapstructure:"storage_class_name" required:"false"`
	// AccessMode is the access mode of the volumes.
	// Supported values are "ReadWriteOnce" and "ReadWriteMany". Defaults to "ReadWriteOnce".
	AccessMode string `mapstructure:"access_mode" required:"false"`
	// VolumeMode is the volume mode of the volumes.
	// Supported values are "Filesystem" and "Block". Defaults to "Filesystem".
	VolumeMode string `mapstructure:"volume_mode" required:"false"`
}

// The communicator reaches the VM through a port forward opened with the
// KubeVirt API, which listens on `ssh_host` or `winrm_host` (defaults to
// `127.0.0.1`) and forwards connections to `ssh_port` or `winrm_port` in the VM.
type PortForwardConfig struct {
	// SSHLocalPort is the local port the port forward to the VM listens on
	// when using the SSH communicator. Defaults to a free port allocated by
	// the operating system, so that concurrent builds do not conflict.
	SSHLocalPort int `mapstructure:"ssh_local_port" required:"false"`
	// WinRMLocalPort is the local port the port forward to the VM listens on
	// when using the WinRM communicator. Defaults to a free port allocated by
	// the operating system, so that concurrent builds do not conflict.
	WinRMLocalPort int `mapstructure:"winrm_local_port" required:"false"`
}

// Validate checks the common configuration fields shared by all KubeVirt
// builders. Each builder calls this from its own Prepare method after setting
// builder-specific defaults (e.g. communicator type).
func (c *BuilderConfig) Validate(ctx *interpolate.Context) []error {
	var errs []error

	errs = append(errs, c.validateKubeConfig()...)
	errs = append(errs, ValidateName("name", c.Name, validation.IsDNS1123Subdomain)...)
	errs = append(errs, ValidateName("namespace", c.Namespace, validation.IsDNS1123Label)...)
	errs = append(errs, c.validateDisks()...)
	errs = append(errs, c.validateMedia()...)
	errs = append(errs, c.validateStorage()...)
	errs = append(errs, c.validatePreferencesAndInstanceTypes()...)
	errs = append(errs, c.validateCommunicator(ctx)...)

	return errs
}

func (c *BuilderConfig) validateKubeConfig() []error {
	if c.KubeConfig == "" {
		return []error{errors.New("kube_config must be specified")}
	}
	expanded, err := pathing.ExpandUser(c.KubeConfig)
	if err != nil {
		return []error{fmt.Errorf("kube_config is invalid: %w", err)}
	}
	c.KubeConfig = expanded
	return nil
}

func (c *BuilderConfig) validateDisks() []error {
	var errs []error

	if rootDisk := c.Name + "-rootdisk"; len(rootDisk) > validation.DNS1123SubdomainMaxLength {
		errs = append(errs, fmt.Errorf("name is too long: the root disk DataVolume %q must be no more than %d characters",
			rootDisk, validation.DNS1123SubdomainMaxLength))
	}

	if c.DiskSize == "" {
		errs = append(errs, errors.New("disk_size must be specified"))
	} else if size, err := resource.ParseQuantity(c.DiskSize); err != nil {
		errs = append(errs, fmt.Errorf("disk_size %q is not a valid Kubernetes quantity (e.g. \"10Gi\"): %w", c.DiskSize, err))
	} else if size.Sign() <= 0 {
		errs = append(errs, fmt.Errorf("disk_size %q must be greater than zero", c.DiskSize))
	}

	if c.DiskBus == "" {
		c.DiskBus = "scsi"
	}
	switch v1.DiskBus(c.DiskBus) {
	case v1.DiskBusSCSI, v1.DiskBusSATA, v1.DiskBusUSB:
	case v1.DiskBusVirtio:
		errs = append(errs, errors.New("disk_bus \"virtio\" is not supported by KubeVirt for CD-ROM devices, use \"scsi\", \"sata\" or \"usb\""))
	default:
		errs = append(errs, fmt.Errorf("disk_bus %q is not supported, use \"scsi\", \"sata\" or \"usb\"", c.DiskBus))
	}

	return errs
}

func (c *BuilderConfig) validateMedia() []error {
	var errs []error

	if c.MediaLabel == "" {
		c.MediaLabel = DefaultMediaLabel
	}
	if len(c.MediaLabel) > MaxMediaLabelLength {
		errs = append(errs, fmt.Errorf("media_label %q must be at most %d characters long", c.MediaLabel, MaxMediaLabelLength))
	}

	errs = append(errs, validateMediaFiles(c.MediaFiles, c.MediaContent)...)

	return errs
}

func (c *BuilderConfig) validateStorage() []error {
	var errs []error

	if c.StorageClassName != "" {
		errs = append(errs, ValidateName("storage_class_name", c.StorageClassName, validation.IsDNS1123Subdomain)...)
	}
	switch corev1.PersistentVolumeAccessMode(c.AccessMode) {
	case "", corev1.ReadWriteOnce, corev1.ReadWriteMany:
	default:
		errs = append(errs, fmt.Errorf("access_mode %q is not supported, use %q or %q", c.AccessMode, corev1.ReadWriteOnce, corev1.ReadWriteMany))
	}
	switch corev1.PersistentVolumeMode(c.VolumeMode) {
	case "", corev1.PersistentVolumeFilesystem, corev1.PersistentVolumeBlock:
	default:
		errs = append(errs, fmt.Errorf("volume_mode %q is not supported, use %q or %q", c.VolumeMode, corev1.PersistentVolumeFilesystem, corev1.PersistentVolumeBlock))
	}

	return errs
}

func (c *BuilderConfig) validatePreferencesAndInstanceTypes() []error {
	var errs []error

	if c.InstanceType == "" {
		errs = append(errs, errors.New("instance_type must be specified"))
	}
	if c.InstanceTypeKind == "" {
		c.InstanceTypeKind = instancetypeapi.ClusterSingularResourceName
	} else if !IsSupportedKind(c.InstanceTypeKind, instancetypeapi.ClusterSingularResourceName, instancetypeapi.SingularResourceName) {
		errs = append(errs, fmt.Errorf("instance_type_kind %q is not supported, use %q or %q",
			c.InstanceTypeKind, instancetypeapi.ClusterSingularResourceName, instancetypeapi.SingularResourceName))
	}

	if c.Preference == "" {
		errs = append(errs, errors.New("preference must be specified"))
	}
	if c.PreferenceKind == "" {
		c.PreferenceKind = instancetypeapi.ClusterSingularPreferenceResourceName
	} else if !IsSupportedKind(c.PreferenceKind, instancetypeapi.ClusterSingularPreferenceResourceName, instancetypeapi.SingularPreferenceResourceName) {
		errs = append(errs, fmt.Errorf("preference_kind %q is not supported, use %q or %q",
			c.PreferenceKind, instancetypeapi.ClusterSingularPreferenceResourceName, instancetypeapi.SingularPreferenceResourceName))
	}

	if !c.SkipCreateImage {
		for _, label := range []struct{ field, value string }{
			{"instance_type", c.InstanceType},
			{"preference", c.Preference},
		} {
			for _, msg := range validation.IsValidLabelValue(label.value) {
				errs = append(errs, fmt.Errorf("%s %q cannot be used as a DataSource label: %s", label.field, label.value, msg))
			}
		}
	}

	return errs
}

func (c *BuilderConfig) validateCommunicator(ctx *interpolate.Context) []error {
	var errs []error

	switch c.Comm.Type {
	case "none", "ssh", "winrm":
		errs = append(errs, c.Comm.Prepare(ctx)...)
	default:
		errs = append(errs, fmt.Errorf("communicator %q is not supported, use \"ssh\", \"winrm\" or \"none\"", c.Comm.Type))
	}

	if c.Comm.Type == "ssh" {
		if c.Comm.SSHBastionHost != "" {
			errs = append(errs, errors.New("ssh_bastion_host is not supported: the builder connects to the VM through a port forward on this machine"))
		}
		if c.Comm.SSHProxyHost != "" {
			errs = append(errs, errors.New("ssh_proxy_host is not supported: the builder connects to the VM through a port forward on this machine"))
		}
	}

	return errs
}

// ValidateName reports an error if a Kubernetes object name is missing or
// does not satisfy the given apimachinery validation rule.
func ValidateName(field, value string, rule func(string) []string) []error {
	if value == "" {
		return []error{fmt.Errorf("%s must be specified", field)}
	}

	var errs []error
	for _, msg := range rule(value) {
		errs = append(errs, fmt.Errorf("%s %q is invalid: %s", field, value, msg))
	}
	return errs
}

// ValidateMedia checks that the media files and content can be stored in the
// ConfigMap backing the media disk, where each file is keyed by its name.
func validateMediaFiles(paths []string, content map[string]string) []error {
	var errs []error
	var totalSize int64
	names := make(map[string]string, len(paths))

	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("media_files: %w", err))
			continue
		}
		if !info.Mode().IsRegular() {
			errs = append(errs, fmt.Errorf("media_files: %q is not a regular file", path))
			continue
		}
		totalSize += info.Size()

		name := filepath.Base(path)
		if other, ok := names[name]; ok {
			errs = append(errs, fmt.Errorf("media_files: %q and %q have the same file name, which must be unique", other, path))
		}
		names[name] = path

		for _, msg := range validation.IsConfigMapKey(name) {
			errs = append(errs, fmt.Errorf("media_files: the file name of %q is invalid: %s", path, msg))
		}
	}

	for _, name := range slices.Sorted(maps.Keys(content)) {
		totalSize += int64(len(content[name]))

		if path, ok := names[name]; ok {
			errs = append(errs, fmt.Errorf("media_content: %q is also provided by media_files (%q)", name, path))
		}
		for _, msg := range validation.IsConfigMapKey(name) {
			errs = append(errs, fmt.Errorf("media_content: the file name %q is invalid: %s", name, msg))
		}
	}

	if totalSize > MaxMediaSize {
		errs = append(errs, fmt.Errorf("the media files add up to %d bytes, but a ConfigMap can hold at most %d bytes", totalSize, MaxMediaSize))
	}
	return errs
}
