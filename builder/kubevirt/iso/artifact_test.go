// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"testing"

	registryimage "github.com/hashicorp/packer-plugin-sdk/packer/registry/image"
	"github.com/mitchellh/mapstructure"
)

func TestArtifact_BuilderId(t *testing.T) {
	a := &Artifact{Name: "my-image", Namespace: "images"}
	if got := a.BuilderId(); got != "packer.kubevirt.iso" {
		t.Errorf("BuilderId() = %q, want %q", got, "packer.kubevirt.iso")
	}
}

func TestArtifact_Id(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		imageName string
		want      string
	}{
		{"both set", "images", "my-image", "images/my-image"},
		{"empty namespace", "", "my-image", "/my-image"},
		{"empty name", "images", "", "images/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Artifact{Name: tt.imageName, Namespace: tt.namespace}
			if got := a.Id(); got != tt.want {
				t.Errorf("Id() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestArtifact_String(t *testing.T) {
	a := &Artifact{Name: "my-image", Namespace: "images"}
	want := "Bootable volume: images/my-image"
	if got := a.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

func TestArtifact_Files(t *testing.T) {
	a := &Artifact{}
	if files := a.Files(); files != nil {
		t.Errorf("Files() = %v, want nil", files)
	}
}

func TestArtifact_Destroy(t *testing.T) {
	a := &Artifact{}
	if err := a.Destroy(); err != nil {
		t.Errorf("Destroy() = %v, want nil", err)
	}
}

// decodeRegistryImage is a test helper that asserts result is a non-nil
// *registryimage.Image and decodes it.
func decodeRegistryImage(t *testing.T, result any) registryimage.Image {
	t.Helper()
	if result == nil {
		t.Fatal("State(ArtifactStateURI) returned nil")
	}
	var img registryimage.Image
	if err := mapstructure.Decode(result, &img); err != nil {
		t.Fatalf("mapstructure.Decode: %v", err)
	}
	return img
}

func TestArtifact_State_HCP(t *testing.T) {
	tests := []struct {
		name         string
		artifact     *Artifact
		wantImageID  string
		wantProvider string
		wantRegion   string
		wantLabels   map[string]string
	}{
		{
			name: "no generated_data",
			artifact: &Artifact{
				Name:      "my-image",
				Namespace: "images",
				StateData: map[string]any{},
			},
			wantImageID:  "images/my-image",
			wantProvider: "kubevirt",
			wantRegion:   "images",
			wantLabels:   map[string]string{"namespace": "images"},
		},
		{
			name: "with generated_data",
			artifact: &Artifact{
				Name:      "my-image",
				Namespace: "images",
				StateData: map[string]any{
					"generated_data": map[string]any{
						"BootableVolumeName": "my-image",
					},
				},
			},
			wantImageID:  "images/my-image",
			wantProvider: "kubevirt",
			wantRegion:   "images",
			wantLabels: map[string]string{
				"namespace":          "images",
				"BootableVolumeName": "my-image",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.artifact.State(registryimage.ArtifactStateURI)
			img := decodeRegistryImage(t, result)

			if img.ImageID != tt.wantImageID {
				t.Errorf("ImageID = %q, want %q", img.ImageID, tt.wantImageID)
			}
			if img.ProviderName != tt.wantProvider {
				t.Errorf("ProviderName = %q, want %q", img.ProviderName, tt.wantProvider)
			}
			if img.ProviderRegion != tt.wantRegion {
				t.Errorf("ProviderRegion = %q, want %q", img.ProviderRegion, tt.wantRegion)
			}
			for k, want := range tt.wantLabels {
				if got := img.Labels[k]; got != want {
					t.Errorf("Labels[%q] = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestArtifact_State_OtherKeys(t *testing.T) {
	tests := []struct {
		name      string
		stateData map[string]any
		key       string
		want      any
	}{
		{
			name:      "known key returns value",
			stateData: map[string]any{"some_key": "some_value"},
			key:       "some_key",
			want:      "some_value",
		},
		{
			name:      "missing key returns nil",
			stateData: map[string]any{},
			key:       "missing_key",
			want:      nil,
		},
		{
			name:      "nil StateData returns nil",
			stateData: nil,
			key:       "any_key",
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Artifact{StateData: tt.stateData}
			if got := a.State(tt.key); got != tt.want {
				t.Errorf("State(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}
