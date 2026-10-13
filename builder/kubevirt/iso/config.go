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

	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
	"github.com/hashicorp/packer-plugin-sdk/common"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"k8s.io/apimachinery/pkg/util/validation"
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
	// DefaultVirtIOContainerImage provides the VirtIO drivers to Windows installations.
	DefaultVirtIOContainerImage = "quay.io/kubevirt/virtio-container-disk:v1.5.2"
)

type Config struct {
	common.PackerConfig          `mapstructure:",squash"`
	kubevirtcommon.BuilderConfig `mapstructure:",squash"`

	// ISO Volume Name is the name of the DataVolume resource that contains the installation ISO.
	// This DataVolume must already exist in the namespace.
	IsoVolumeName string `mapstructure:"iso_volume_name" required:"true"`
	// OperatingSystemType is the type of operating system to install.
	// Supported values are "linux" and "windows". Default is "linux".
	OperatingSystemType string `mapstructure:"os_type" required:"false"`
	// Networks is a list of networks to attach to the temporary VM.
	// If no networks are specified, a single pod network will be used.
	Networks []Network `mapstructure:"networks" required:"false"`
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

	// Deprecated: use ssh_port.
	SSHRemotePort int `mapstructure:"ssh_remote_port" required:"false" undocumented:"true"`
	// Deprecated: use winrm_port.
	WinRMRemotePort int `mapstructure:"winrm_remote_port" required:"false" undocumented:"true"`
	// Deprecated: use winrm_timeout.
	WinRMWaitTimeout time.Duration `mapstructure:"winrm_wait_timeout" required:"false" undocumented:"true"`

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

	// Keep the historical behaviour of this builder, which only connected to
	// the VM when a communicator was explicitly configured.
	if c.Comm.Type == "" {
		c.Comm.Type = "none"
	}

	errs = packersdk.MultiErrorAppend(errs, c.BuilderConfig.Validate(&c.ctx)...)
	errs = packersdk.MultiErrorAppend(errs, kubevirtcommon.ValidateName("iso_volume_name", c.IsoVolumeName, validation.IsDNS1123Subdomain)...)

	if c.OperatingSystemType == "" {
		c.OperatingSystemType = "linux"
	}
	if c.OperatingSystemType != "linux" && c.OperatingSystemType != "windows" {
		errs = packersdk.MultiErrorAppend(errs, fmt.Errorf("os_type %q is not supported, use \"linux\" or \"windows\"", c.OperatingSystemType))
	}

	if c.VirtIOContainerImage == "" {
		c.VirtIOContainerImage = DefaultVirtIOContainerImage
	}

	if len(c.BootCommand) > 0 {
		errs = packersdk.MultiErrorAppend(errs, validateBootCommand(c.BootCommand)...)
	}

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
