// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Network,NetworkSource,PodNetwork,MultusNetwork,PortForwardConfig

package iso

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/pathing"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/validation"

	v1 "kubevirt.io/api/core/v1"
	instancetypeapi "kubevirt.io/api/instancetype"
)

// Network represents a network type and a resource that should be connected to the VM.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_network
type Network struct {
	// Network name.
	// Can only contain letters, digits, '-' and '_', and must be unique within the VM.
	Name string `mapstructure:"name"`

	// NetworkSource represents the network type and the source interface that should be connected to the VM.
	// Defaults to Pod, if no type is specified.
	NetworkSource `mapstructure:",squash"`
}

// Represents the source resource that will be connected to the VM.
// Only one of its members may be specified.
type NetworkSource struct {
	Pod    *PodNetwork    `mapstructure:"pod"`
	Multus *MultusNetwork `mapstructure:"multus"`
}

// Represents the stock pod network interface.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_podnetwork
type PodNetwork struct {
	// CIDR for VM network.
	// Default 10.0.2.0/24 if not specified.
	VMNetworkCIDR string `mapstructure:"vmNetworkCIDR,omitempty"`

	// IPv6 CIDR for the VM network.
	// Defaults to fd10:0:2::/120 if not specified.
	VMIPv6NetworkCIDR string `mapstructure:"vmIPv6NetworkCIDR,omitempty"`
}

// Represents the multus CNI network.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_multusnetwork
type MultusNetwork struct {
	// References to a NetworkAttachmentDefinition CRD object. Format:
	// <networkName>, <namespace>/<networkName>. If namespace is not
	// specified, VMI namespace is assumed.
	NetworkName string `mapstructure:"networkName"`

	// Select the default network and add it to the
	// multus-cni.io/default-network annotation.
	Default bool `mapstructure:"default,omitempty"`
}

const (
	// DefaultMediaLabel is the volume label that Anaconda (kickstart) auto-discovers.
	DefaultMediaLabel   = "OEMDRV"
	maxMediaLabelLength = 32

	// DefaultVirtIOContainerImage provides the VirtIO drivers to Windows installations.
	DefaultVirtIOContainerImage = "quay.io/kubevirt/virtio-container-disk:v1.5.2"
)

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

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	// KubeConfig is the path to the kubeconfig file used to connect to the cluster.
	// A leading `~` is expanded to the home directory of the current user.
	KubeConfig string `mapstructure:"kube_config" required:"true"`
	// Name is the name of the VM image.
	Name string `mapstructure:"name" required:"true"`
	// Namespace is the namespace in which to create the VM image.
	Namespace string `mapstructure:"namespace" required:"true"`
	// ISO Volume Name is the name of the DataVolume resource that contains the installation ISO.
	// This DataVolume must already exist in the namespace.
	IsoVolumeName string `mapstructure:"iso_volume_name" required:"true"`
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
	// OperatingSystemType is the type of operating system to install.
	// Supported values are "linux" and "windows". Default is "linux".
	OperatingSystemType string `mapstructure:"os_type" required:"false"`
	// DiskBus is the bus type to use for CD-ROM disk devices on the temporary VM.
	// Supported values are "scsi", "sata", and "usb". KubeVirt does not support
	// "virtio" for CD-ROM devices.
	// Defaults to "scsi", which is compatible with both x86 and arm64 architectures.
	// Use "sata" on x86 clusters if required by your storage configuration.
	DiskBus string `mapstructure:"disk_bus" required:"false"`
	// Networks is a list of networks to attach to the temporary VM.
	// If no networks are specified, a single pod network will be used.
	Networks []Network `mapstructure:"networks" required:"false"`
	// MediaFiles is a path list of files to be copied and used during the ISO installation.
	// The files are stored in a ConfigMap and attached to the VM as a disk, where each
	// file is named after its base name. The file names must therefore be unique, and
	// the files must add up to at most 1 MiB.
	MediaFiles []string `mapstructure:"media_files" required:"false"`
	// MediaContent is a map of file names to file contents to add to the media disk,
	// alongside `media_files`. This is useful to render installer configuration from
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
	// VirtIOContainerImage is the container disk image with the VirtIO drivers,
	// attached as a CD-ROM to Windows VMs so that the installer can use VirtIO
	// devices. Set it to use a registry mirror in disconnected clusters, or the
	// drivers image of your distribution. Only applies when `os_type` is "windows".
	// Defaults to "quay.io/kubevirt/virtio-container-disk:v1.5.2".
	VirtIOContainerImage string `mapstructure:"virtio_container_image" required:"false"`
	// BootCommand is a list of strings that represent the keystrokes to be sent to the VM console
	// to automate the installation via a new VNC connection. The connection is closed once the
	// keystrokes are sent. When no boot command is set, the builder does not connect to VNC.
	BootCommand []string `mapstructure:"boot_command" required:"false"`
	// BootWait is the amount of time to wait before sending the boot command.
	// This is useful if the VM takes some time to boot and be ready to accept keystrokes.
	BootWait time.Duration `mapstructure:"boot_wait" required:"false"`
	// InstallationWaitTimeout is the amount of time to wait for the installation to be completed.
	// It is required when no communicator is configured, since the builder has no other way
	// to know when the installation has finished. With a communicator, the builder connects
	// to the VM once this time has elapsed.
	InstallationWaitTimeout time.Duration `mapstructure:"installation_wait_timeout" required:"false"`

	Comm              communicator.Config `mapstructure:",squash"`
	PortForwardConfig `mapstructure:",squash"`

	// Deprecated: use ssh_port.
	SSHRemotePort int `mapstructure:"ssh_remote_port" required:"false" undocumented:"true"`
	// Deprecated: use winrm_port.
	WinRMRemotePort int `mapstructure:"winrm_remote_port" required:"false" undocumented:"true"`
	// Deprecated: use winrm_timeout.
	WinRMWaitTimeout time.Duration `mapstructure:"winrm_wait_timeout" required:"false" undocumented:"true"`

	// KeepVM indicates whether to keep the temporary VM after the image has been created.
	// If false, the VM and all its resources will be deleted after the image is created.
	// If true, only the VM resource will be kept, all other resources will be deleted.
	// Default is false.
	//
	// This can be useful for debugging purposes, to inspect the VM and its disks.
	// However, it is recommended to set this to false in production environments to avoid
	// resource leaks.
	KeepVM bool `mapstructure:"keep_vm" required:"false"`

	// SkipCreateImage when set to true skips creating the final bootable volume
	// DataSource and does not register the artifact with HCP Packer. This is
	// useful for iterative debugging when you do not want to produce a final image.
	// Default is false.
	SkipCreateImage bool `mapstructure:"skip_create_image" required:"false"`

	ctx interpolate.Context
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         "builder.kubevirt.iso",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			// Keep the media content as is, like the content of media_files. The
			// SDK still validates excluded values as templates.
			Exclude: []string{"media_content"},
		},
	}, raws...)
	if err != nil {
		return nil, err
	}

	var errs *packersdk.MultiError
	warnings := c.applyDeprecatedOptions()

	if c.KubeConfig == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("kube_config must be specified"))
	} else if c.KubeConfig, err = pathing.ExpandUser(c.KubeConfig); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("kube_config is invalid: %w", err))
	}

	errs = packersdk.MultiErrorAppend(errs, validateName("name", c.Name, validation.IsDNS1123Subdomain)...)
	// The root disk DataVolume of the VM is named after the image.
	if rootDisk := c.Name + "-rootdisk"; len(rootDisk) > validation.DNS1123SubdomainMaxLength {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("name is too long: the root disk DataVolume %q must be no more than %d characters",
			rootDisk, validation.DNS1123SubdomainMaxLength))
	}
	errs = packersdk.MultiErrorAppend(errs, validateName("namespace", c.Namespace, validation.IsDNS1123Label)...)
	errs = packersdk.MultiErrorAppend(errs, validateName("iso_volume_name", c.IsoVolumeName, validation.IsDNS1123Subdomain)...)

	if c.DiskSize == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("disk_size must be specified"))
	} else if size, err := resource.ParseQuantity(c.DiskSize); err != nil {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_size %q is not a valid Kubernetes quantity (e.g. \"10Gi\"): %w", c.DiskSize, err))
	} else if size.Sign() <= 0 {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_size %q must be greater than zero", c.DiskSize))
	}

	if c.InstanceType == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("instance_type must be specified"))
	}
	if c.InstanceTypeKind == "" {
		c.InstanceTypeKind = instancetypeapi.ClusterSingularResourceName
	} else if !isSupportedKind(c.InstanceTypeKind, instancetypeapi.ClusterSingularResourceName, instancetypeapi.SingularResourceName) {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("instance_type_kind %q is not supported, use %q or %q",
			c.InstanceTypeKind, instancetypeapi.ClusterSingularResourceName, instancetypeapi.SingularResourceName))
	}

	if c.Preference == "" {
		errs = packersdk.MultiErrorAppend(errs, errors.New("preference must be specified"))
	}
	if c.PreferenceKind == "" {
		c.PreferenceKind = instancetypeapi.ClusterSingularPreferenceResourceName
	} else if !isSupportedKind(c.PreferenceKind, instancetypeapi.ClusterSingularPreferenceResourceName, instancetypeapi.SingularPreferenceResourceName) {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("preference_kind %q is not supported, use %q or %q",
			c.PreferenceKind, instancetypeapi.ClusterSingularPreferenceResourceName, instancetypeapi.SingularPreferenceResourceName))
	}

	// The instance type and preference are recorded as labels on the DataSource
	// created at the end of the build, so check them before the installation.
	if !c.SkipCreateImage {
		for _, label := range []struct{ field, value string }{
			{"instance_type", c.InstanceType},
			{"preference", c.Preference},
		} {
			for _, msg := range validation.IsValidLabelValue(label.value) {
				errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("%s %q cannot be used as a DataSource label: %s", label.field, label.value, msg))
			}
		}
	}

	if c.OperatingSystemType == "" {
		c.OperatingSystemType = "linux"
	}
	if c.OperatingSystemType != "linux" && c.OperatingSystemType != "windows" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("os_type %q is not supported, use \"linux\" or \"windows\"", c.OperatingSystemType))
	}

	if c.DiskBus == "" {
		c.DiskBus = "scsi"
	}
	switch v1.DiskBus(c.DiskBus) {
	case v1.DiskBusSCSI, v1.DiskBusSATA, v1.DiskBusUSB:
	case v1.DiskBusVirtio:
		errs = packersdk.MultiErrorAppend(errs, errors.New("disk_bus \"virtio\" is not supported by KubeVirt for CD-ROM devices, use \"scsi\", \"sata\" or \"usb\""))
	default:
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("disk_bus %q is not supported, use \"scsi\", \"sata\" or \"usb\"", c.DiskBus))
	}

	if c.MediaLabel == "" {
		c.MediaLabel = DefaultMediaLabel
	}

	if c.VirtIOContainerImage == "" {
		c.VirtIOContainerImage = DefaultVirtIOContainerImage
	}

	// The media disk is an ISO 9660 image, whose volume identifier is limited to 32 characters.
	if len(c.MediaLabel) > maxMediaLabelLength {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("media_label %q must be at most %d characters long", c.MediaLabel, maxMediaLabelLength))
	}

	if len(c.BootCommand) > 0 {
		errs = packersdk.MultiErrorAppend(errs, validateBootCommand(c.BootCommand)...)
	}

	errs = packersdk.MultiErrorAppend(errs, validateMedia(c.MediaFiles, c.MediaContent)...)

	// Keep the historical behaviour of this builder, which only connected to
	// the VM when a communicator was explicitly configured.
	if c.Comm.Type == "" {
		c.Comm.Type = "none"
	}
	switch c.Comm.Type {
	case "none", "ssh", "winrm":
		errs = packersdk.MultiErrorAppend(errs, c.Comm.Prepare(&c.ctx)...)
	default:
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("communicator %q is not supported, use \"ssh\", \"winrm\" or \"none\"", c.Comm.Type))
	}

	// The communicator connects to a port forward listening on this machine,
	// which a bastion host or a proxy would try to reach on their own loopback.
	if c.Comm.Type == "ssh" {
		if c.Comm.SSHBastionHost != "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New("ssh_bastion_host is not supported: the builder connects to the VM through a port forward on this machine"))
		}
		if c.Comm.SSHProxyHost != "" {
			errs = packersdk.MultiErrorAppend(errs, errors.New("ssh_proxy_host is not supported: the builder connects to the VM through a port forward on this machine"))
		}
	}

	// Without a communicator the build stops the VM right after the boot
	// command, so the installation would be interrupted.
	switch {
	case c.InstallationWaitTimeout < 0:
		errs = packersdk.MultiErrorAppend(errs, errors.New("installation_wait_timeout must not be negative"))
	case c.InstallationWaitTimeout == 0 && c.Comm.Type == "none":
		errs = packersdk.MultiErrorAppend(errs, errors.New("installation_wait_timeout must be set when no communicator is configured"))
	}

	networkNames := make(map[string]bool, len(c.Networks))
	for i, n := range c.Networks {
		if n.Name == "" {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("networks[%d]: name must be specified", i))
		} else if !networkNameFormat.MatchString(n.Name) {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("network %q: the name can only contain letters, digits, '-' and '_'", n.Name))
		} else if networkNames[n.Name] {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("network %q: names must be unique", n.Name))
		}
		networkNames[n.Name] = true

		if n.Pod != nil && n.Multus != nil {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("network %q: only one of pod or multus can be defined", n.Name))
		}
	}

	if errs != nil && len(errs.Errors) > 0 {
		return warnings, errs
	}
	return warnings, nil
}

// applyDeprecatedOptions maps the communicator options that predate the use
// of the Packer SDK communicator configuration to their SDK equivalents. The
// SDK options take precedence when both are set, except for ssh_wait_timeout,
// which the SDK itself lets override ssh_timeout.
func (c *Config) applyDeprecatedOptions() []string {
	var warnings []string

	if c.SSHRemotePort != 0 {
		warnings = append(warnings, "ssh_remote_port is deprecated, use ssh_port instead")
		if c.Comm.SSHPort == 0 {
			c.Comm.SSHPort = c.SSHRemotePort
		}
	}
	if c.Comm.SSHWaitTimeout != 0 {
		warnings = append(warnings, "ssh_wait_timeout is deprecated, use ssh_timeout instead")
		// Set ssh_timeout before the SDK applies its defaults: with neither
		// option set, it also limits the number of SSH handshake attempts,
		// which the builder never did when ssh_wait_timeout was used.
		if c.Comm.SSHTimeout == 0 {
			c.Comm.SSHTimeout = c.Comm.SSHWaitTimeout
		}
	}
	if c.WinRMRemotePort != 0 {
		warnings = append(warnings, "winrm_remote_port is deprecated, use winrm_port instead")
		if c.Comm.WinRMPort == 0 {
			c.Comm.WinRMPort = c.WinRMRemotePort
		}
	}
	if c.WinRMWaitTimeout != 0 {
		warnings = append(warnings, "winrm_wait_timeout is deprecated, use winrm_timeout instead")
		if c.Comm.WinRMTimeout == 0 {
			c.Comm.WinRMTimeout = c.WinRMWaitTimeout
		}
	}
	return warnings
}

// networkNameFormat is the format KubeVirt accepts for network and interface names.
var networkNameFormat = regexp.MustCompile(`^[A-Za-z0-9-_]+$`)

// validateName reports an error if a Kubernetes object name is missing or
// does not satisfy the given apimachinery validation rule.
func validateName(field, value string, rule func(string) []string) []error {
	if value == "" {
		return []error{fmt.Errorf("%s must be specified", field)}
	}

	var errs []error
	for _, msg := range rule(value) {
		errs = append(errs, fmt.Errorf("%s %q is invalid: %s", field, value, msg))
	}
	return errs
}

// isSupportedKind mirrors how KubeVirt resolves instancetype and preference
// matcher kinds: case-insensitively, by singular or plural resource name.
func isSupportedKind(kind string, singularNames ...string) bool {
	kind = strings.ToLower(kind)
	for _, name := range singularNames {
		if kind == name || kind == name+"s" {
			return true
		}
	}
	return false
}

// validateBootCommand parses the boot command the same way StepBootCommand
// does, so that syntax errors are reported by `packer validate`.
func validateBootCommand(bootCommand []string) []error {
	command, err := interpolate.Render(strings.Join(bootCommand, ""), &interpolate.Context{})
	if err != nil {
		return []error{fmt.Errorf("boot_command is invalid: %w", err)}
	}

	sequence, err := bootcommand.GenerateExpressionSequence(command)
	if err != nil {
		return []error{fmt.Errorf("boot_command is invalid: %w", err)}
	}

	var errs []error
	for _, err := range sequence.Validate() {
		errs = append(errs, fmt.Errorf("boot_command is invalid: %w", err))
	}
	return errs
}

// maxMediaSize is the maximum size of the data stored in a ConfigMap.
const maxMediaSize = 1024 * 1024

// validateMedia checks that the media files and content can be stored in the
// ConfigMap backing the media disk, where each file is keyed by its name.
func validateMedia(paths []string, content map[string]string) []error {
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

	if totalSize > maxMediaSize {
		errs = append(errs, fmt.Errorf("the media files add up to %d bytes, but a ConfigMap can hold at most %d bytes", totalSize, maxMediaSize))
	}
	return errs
}
