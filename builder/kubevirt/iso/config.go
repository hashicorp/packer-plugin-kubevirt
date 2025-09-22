// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Network,NetworkSource,PodNetwork,MultusNetwork

package iso

import (
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// Network represents a network type and a resource that should be connected to the VM.
// Source: https://kubevirt.io/api-reference/v1.6.0/definitions.html#_v1_network
type Network struct {
	// Network name.
	// Must be a DNS_LABEL and unique within the VM.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
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

type Config struct {
	common.PackerConfig `mapstructure:",squash"`

	Comm         communicator.Config `mapstructure:",squash"`
	WaitIpConfig `mapstructure:",squash"`

	// KubeConfig is the path to the kubeconfig file.
	KubeConfig string `mapstructure:"kube_config" required:"true"`
	// Name is the name of the VM image.
	Name string `mapstructure:"name" required:"true"`
	// Namespace is the namespace in which to create the VM image.
	Namespace string `mapstructure:"namespace" required:"true"`
	// ISO Volume Name is the name of the DataVolume resource that contains the installation ISO.
	// This DataVolume must already exist in the namespace.
	IsoVolumeName string `mapstructure:"iso_volume_name" required:"true"`
	// DiskSize is the size of the root disk to of the temporary VM.
	DiskSize string `mapstructure:"disk_size" required:"true"`
	// InstanceType is the name of the InstanceType resource to use in the temporary VM.
	InstanceType string `mapstructure:"instance_type" required:"true"`
	// InstanceTypeKind is the kind of the InstanceType resource to use in the temporary VM.
	// Other supported value is "virtualmachineclusterinstancetype".
	InstanceTypeKind string `mapstructure:"instance_type_kind" required:"false"`
	// Preference is the name of the Preference resource to use in the temporary VM.
	Preference string `mapstructure:"preference" required:"true"`
	// PreferenceKind is the kind of the Preference resource to use in the temporary VM.
	// Other supported value is "virtualmachineclusterpreference".
	PreferenceKind string `mapstructure:"preference_kind" required:"false"`
	// OperatingSystemType is the type of operating system to install.
	// Supported values are "linux" and "windows". Default is "linux".
	OperatingSystemType string `mapstructure:"os_type" required:"false"`
	// Networks is a list of networks to attach to the temporary VM.
	// If no networks are specified, a single pod network will be used.
	Networks []Network `mapstructure:"networks" required:"false"`
	// MediaFiles is a path list of files to be copied and used during the ISO installation.
	MediaFiles []string `mapstructure:"media_files" required:"false"`
	// BootCommand is a list of strings that represent the keystrokes to be sent to the VM console
	// to automate the installation via a new VNC connection.
	BootCommand []string `mapstructure:"boot_command" required:"false"`
	// BootWait is the amount of time to wait before sending the boot command.
	// This is useful if the VM takes some time to boot and be ready to accept keystrokes.
	BootWait time.Duration `mapstructure:"boot_wait" required:"false"`
	// InstallationWaitTimeout is the amount of time to wait for the installation to be completed.
	InstallationWaitTimeout time.Duration `mapstructure:"installation_wait_timeout" required:"true"`
	// SSHLocalPort is the local port to use to connect via SSH.
	SSHLocalPort int `mapstructure:"ssh_local_port" required:"false"`
	// SSHRemotePort is the remote port to use to connect via SSH.
	// This has been deprecated in favor of ssh_port.
	SSHRemotePort int `mapstructure:"ssh_remote_port" required:"false" undocumented:"true"`
	// WinRMLocalPort is the local port to use to connect via WinRM.
	WinRMLocalPort int `mapstructure:"winrm_local_port" required:"false"`
	// WinRMRemotePort is the remote port to use to connect via WinRM.
	// This has been deprecated in favor of WinRMPort
	WinRMRemotePort int `mapstructure:"winrm_remote_port" required:"false" undocumented:"true"`
	// WinRMWaitTimeout is the amount of time to wait for the WinRM service to be available.
	// This has been deprecated in favor of WinRMTimeout
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

	ctx interpolate.Context
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         "builder.kubevirt.iso",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
	}, raws...)
	if err != nil {
		return nil, err
	}

	warnings := make([]string, 0)
	errs := new(packersdk.MultiError)

	errs = packersdk.MultiErrorAppend(errs, c.WaitIpConfig.Prepare()...)
	errs = packersdk.MultiErrorAppend(errs, c.Comm.Prepare(&c.ctx)...)

	for _, n := range c.Networks {
		if n.Pod != nil && n.Multus != nil {
			errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("network %q: only one of pod or multus can be defined", n.Name))
		}
	}

	if len(errs.Errors) > 0 {
		return warnings, errs
	}

	// Remap deprecated values for backwards compatibility:
	warnings = append(warnings, c.backwardsCompat()...)

	return warnings, nil
}

func (c *Config) backwardsCompat() []string {
	depmsg := make([]string, 0)

	if c.SSHRemotePort != 0 {
		depmsg = append(depmsg, "ssh_remote_port is deprecated - use ssh_port instead")
		c.Comm.SSHPort = c.SSHRemotePort
	}

	if c.WinRMRemotePort != 0 {
		depmsg = append(depmsg, "winrm_remote_port is deprecated - use winrm_port instead")
		c.Comm.WinRMPort = c.WinRMRemotePort
	}

	if c.WinRMWaitTimeout != 0 {
		depmsg = append(depmsg, "winrm_wait_timeout is deprecated - use winrm_timeout instead")
		c.Comm.WinRMTimeout = c.WinRMWaitTimeout
	}

	return depmsg
}
