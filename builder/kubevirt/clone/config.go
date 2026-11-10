// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type Config,Network,NetworkSource,PodNetwork,MultusNetwork

package clone

import (
	"errors"
	"fmt"
	"regexp"

	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
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

type Config struct {
	common.PackerConfig          `mapstructure:",squash"`
	kubevirtcommon.BuilderConfig `mapstructure:",squash"`

	// SourceVolumeName is the name of the source DataVolume to clone.
	// This is the bootable volume to clone, typically the output of a
	// kubevirt-iso build.
	SourceVolumeName string `mapstructure:"source_volume_name" required:"true"`
	// SourceVolumeNamespace is the namespace of the source DataVolume.
	// Defaults to the value of `namespace`.
	SourceVolumeNamespace string `mapstructure:"source_volume_namespace" required:"false"`
	// Networks is a list of networks to attach to the temporary VM.
	// If no networks are specified, a single pod network will be used.
	Networks []Network `mapstructure:"networks" required:"false"`

	ctx interpolate.Context
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         "builder.kubevirt.clone",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{"media_content"},
		},
	}, raws...)
	if err != nil {
		return nil, err
	}

	var errs *packersdk.MultiError

	if c.Comm.Type == "" {
		c.Comm.Type = "ssh"
	}

	errs = packersdk.MultiErrorAppend(errs, c.BuilderConfig.Validate(&c.ctx)...)

	errs = packersdk.MultiErrorAppend(errs, kubevirtcommon.ValidateName("source_volume_name", c.SourceVolumeName, validation.IsDNS1123Subdomain)...)

	if c.SourceVolumeNamespace == "" {
		c.SourceVolumeNamespace = c.Namespace
	} else {
		errs = packersdk.MultiErrorAppend(errs, kubevirtcommon.ValidateName("source_volume_namespace", c.SourceVolumeNamespace, validation.IsDNS1123Label)...)
	}

	if c.SourceVolumeName == c.Name && c.SourceVolumeNamespace == c.Namespace {
		errs = packersdk.MultiErrorAppend(errs, errors.New("source_volume_name and name cannot refer to the same volume in the same namespace"))
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

var networkNameFormat = regexp.MustCompile(`^[A-Za-z0-9-_]+$`)
