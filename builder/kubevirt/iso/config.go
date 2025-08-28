// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Network,NetworkSource,PodNetwork,MultusNetwork

package iso

import (
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
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

	KubeConfig              string        `mapstructure:"kube_config"`
	Name                    string        `mapstructure:"name"`
	Namespace               string        `mapstructure:"namespace"`
	IsoVolumeName           string        `mapstructure:"iso_volume_name"`
	DiskSize                string        `mapstructure:"disk_size"`
	InstanceType            string        `mapstructure:"instance_type"`
	InstanceTypeKind        string        `mapstructure:"instance_type_kind"`
	Preference              string        `mapstructure:"preference"`
	PreferenceKind          string        `mapstructure:"preference_kind"`
	OperatingSystemType     string        `mapstructure:"os_type"`
	Networks                []Network     `mapstructure:"networks"`
	MediaFiles              []string      `mapstructure:"media_files"`
	BootCommand             []string      `mapstructure:"boot_command"`
	BootWait                time.Duration `mapstructure:"boot_wait"`
	InstallationWaitTimeout time.Duration `mapstructure:"installation_wait_timeout"`
	Communicator            string        `mapstructure:"communicator"`
	SSHHost                 string        `mapstructure:"ssh_host"`
	SSHLocalPort            int           `mapstructure:"ssh_local_port"`
	SSHRemotePort           int           `mapstructure:"ssh_remote_port"`
	SSHUsername             string        `mapstructure:"ssh_username"`
	SSHPassword             string        `mapstructure:"ssh_password"`
	SSHWaitTimeout          time.Duration `mapstructure:"ssh_wait_timeout"`
	WinRMHost               string        `mapstructure:"winrm_host"`
	WinRMLocalPort          int           `mapstructure:"winrm_local_port"`
	WinRMRemotePort         int           `mapstructure:"winrm_remote_port"`
	WinRMUsername           string        `mapstructure:"winrm_username"`
	WinRMPassword           string        `mapstructure:"winrm_password"`
	WinRMWaitTimeout        time.Duration `mapstructure:"winrm_wait_timeout"`

	// Keep the temporary VM after the build
	KeepVM bool `mapstructure:"keep_vm"`
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:  "builder.kubevirt.iso",
		Interpolate: true,
	}, raws...)
	if err != nil {
		return nil, err
	}

	for _, n := range c.Networks {
		if n.Pod != nil && n.Multus != nil {
			return nil, fmt.Errorf("network %q: only one of pod or multus can be defined", n.Name)
		}
	}
	return nil, err
}
