// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Network,NetworkSource,PodNetwork,MultusNetwork

package iso

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
	"github.com/hashicorp/packer-plugin-sdk/common"
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
)

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
	InstanceType string `mapstructure:"instance_type" required:"true"`
	// InstanceTypeKind is the kind of the InstanceType resource to use in the temporary VM.
	// Supported values are "virtualmachineclusterinstancetype" and "virtualmachineinstancetype".
	// Defaults to "virtualmachineclusterinstancetype".
	InstanceTypeKind string `mapstructure:"instance_type_kind" required:"false"`
	// Preference is the name of the Preference resource to use in the temporary VM.
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
	MediaFiles []string `mapstructure:"media_files" required:"false"`
	// MediaLabel is the volume label of the disk that holds the `media_files`.
	// Different installers discover their configuration through different labels, e.g.
	// "OEMDRV" for Anaconda kickstart (RHEL, Fedora) or "cidata" for cloud-init
	// NoCloud / Subiquity autoinstall (Ubuntu). Only applies when `os_type` is "linux".
	// Must be at most 32 characters long. Defaults to "OEMDRV".
	MediaLabel string `mapstructure:"media_label" required:"false"`
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
	// Communicator is the type of communicator to use to connect to the VM.
	// Supported values are "ssh" and "winrm".
	Communicator string `mapstructure:"communicator" required:"false"`
	// SSHHost is the local address the port forward to the VM listens on, and
	// that the SSH communicator connects to. Defaults to "127.0.0.1".
	SSHHost string `mapstructure:"ssh_host" required:"false"`
	// SSHLocalPort is the local port the port forward to the VM listens on.
	// Defaults to a free port allocated by the operating system, so that
	// concurrent builds do not conflict.
	SSHLocalPort int `mapstructure:"ssh_local_port" required:"false"`
	// SSHRemotePort is the port of the SSH service in the VM. Defaults to 22.
	SSHRemotePort int `mapstructure:"ssh_remote_port" required:"false"`
	// SSHUsername is the username to use to connect via SSH.
	SSHUsername string `mapstructure:"ssh_username" required:"false"`
	// SSHPassword is the password to use to connect via SSH.
	SSHPassword string `mapstructure:"ssh_password" required:"false"`
	// SSHWaitTimeout is the amount of time to wait for the SSH service to be available.
	SSHWaitTimeout time.Duration `mapstructure:"ssh_wait_timeout" required:"false"`
	// WinRMHost is the local address the port forward to the VM listens on, and
	// that the WinRM communicator connects to. Defaults to "127.0.0.1".
	WinRMHost string `mapstructure:"winrm_host" required:"false"`
	// WinRMLocalPort is the local port the port forward to the VM listens on.
	// Defaults to a free port allocated by the operating system, so that
	// concurrent builds do not conflict.
	WinRMLocalPort int `mapstructure:"winrm_local_port" required:"false"`
	// WinRMRemotePort is the port of the WinRM service in the VM. Defaults to 5985.
	WinRMRemotePort int `mapstructure:"winrm_remote_port" required:"false"`
	// WinRMUsername is the username to use to connect via WinRM.
	WinRMUsername string `mapstructure:"winrm_username" required:"false"`
	// WinRMPassword is the password to use to connect via WinRM.
	WinRMPassword string `mapstructure:"winrm_password" required:"false"`
	// WinRMWaitTimeout is the amount of time to wait for the WinRM service to be available.
	WinRMWaitTimeout time.Duration `mapstructure:"winrm_wait_timeout" required:"false"`

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
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:  "builder.kubevirt.iso",
		Interpolate: true,
	}, raws...)
	if err != nil {
		return nil, err
	}

	var errs *packersdk.MultiError

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

	// The media disk is an ISO 9660 image, whose volume identifier is limited to 32 characters.
	if len(c.MediaLabel) > maxMediaLabelLength {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("media_label %q must be at most %d characters long", c.MediaLabel, maxMediaLabelLength))
	}

	if len(c.BootCommand) > 0 {
		errs = packersdk.MultiErrorAppend(errs, validateBootCommand(c.BootCommand)...)
	}

	// Without a communicator the build stops the VM right after the boot
	// command, so the installation would be interrupted.
	switch {
	case c.InstallationWaitTimeout < 0:
		errs = packersdk.MultiErrorAppend(errs, errors.New("installation_wait_timeout must not be negative"))
	case c.InstallationWaitTimeout == 0 && c.Communicator != "ssh" && c.Communicator != "winrm":
		errs = packersdk.MultiErrorAppend(errs, errors.New("installation_wait_timeout must be set when no communicator is configured"))
	}

	if c.SSHRemotePort == 0 {
		c.SSHRemotePort = 22
	}
	if c.WinRMRemotePort == 0 {
		c.WinRMRemotePort = 5985
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
		return nil, errs
	}
	return nil, nil
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
