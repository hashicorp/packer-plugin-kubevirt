// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"fmt"

	registryimage "github.com/hashicorp/packer-plugin-sdk/packer/registry/image"
)

type Artifact struct {
	// Name is the DataSource name of the bootable volume.
	Name string
	// Namespace is the Kubernetes namespace the DataSource lives in.
	Namespace string
	// StateData holds generated_data and any other state shared with
	// post-processors and HCP Packer.
	StateData map[string]any
}

func (a *Artifact) BuilderId() string {
	return "packer.kubevirt.iso"
}

func (a *Artifact) Files() []string {
	return nil
}

// Id returns a namespace-qualified identifier so HCP Packer can track the
// artifact unambiguously across clusters.
func (a *Artifact) Id() string {
	return fmt.Sprintf("%s/%s", a.Namespace, a.Name)
}

func (a *Artifact) String() string {
	return fmt.Sprintf("Bootable volume: %s/%s", a.Namespace, a.Name)
}

// State returns HCP Packer registry metadata when queried with
// registryimage.ArtifactStateURI, and falls back to StateData for all other keys.
// Reading from a nil map is safe and returns nil.
func (a *Artifact) State(name string) any {
	if name == registryimage.ArtifactStateURI {
		return a.hcpRegistryMetadata()
	}
	return a.StateData[name]
}

func (a *Artifact) hcpRegistryMetadata() any {
	labels := map[string]any{
		"namespace": a.Namespace,
	}

	if genData, ok := a.StateData["generated_data"].(map[string]any); ok {
		for k, v := range genData {
			if s, ok := v.(string); ok {
				labels[k] = s
			}
		}
	}

	img, _ := registryimage.FromArtifact(a,
		registryimage.WithProvider("kubevirt"),
		registryimage.WithRegion(a.Namespace),
		registryimage.SetLabels(labels),
	)
	return img
}

func (a *Artifact) Destroy() error {
	return nil
}
