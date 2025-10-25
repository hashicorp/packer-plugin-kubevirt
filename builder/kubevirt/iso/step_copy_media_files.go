// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type MediaConfig

package iso

import (
	"context"
	"os"
	"path/filepath"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type MediaConfig struct {
	// Label is the disk volume label to use on the virtual drive
	// constructed and attached to the VM. This is ignored for Windows systems.
	// Defaults to `OEMDRV`.
	Label string `mapstructure:"media_label" required:"false"`
	// Files is a list of files to be copied into the generated ConfigMap
	// which is used to provide additional install-time files to the VM.
	// Defaults to an empty list.
	Files []string `mapstructure:"media_files" required:"false"`
	// Content is a map of content to include in the generated media volume.
	// The map keys are the filenames and the map values are the file
	// contents.  This permits the use of HCL functions such as `file` and
	// `templatefile` to populate the ConfigMap.
	// If a filename matches a file included in `media_files` then the
	// contents specified here takes precedence.
	// Defaults to an empty list.
	Content map[string]string `mapstructure:"media_content" required:"false"`
	// If true, KeepMedia indicates that the created ConfigMap consisting
	// of files (provided via `media_files` or `media_content`) should not
	// be removed at the end of the build . If false, the ConfigMap will
	// be removed.
	//
	// This can be useful for debugging purposes, to inspect the generated
	// ConfigMap contents. However, it is recommended that this is set
	// to false for regular production use.
	Keep bool `mapstructure:"keep_media" required:"false"`
}

type StepCopyMediaFiles struct {
	Config Config
	Client kubernetes.Interface
}

func (c *MediaConfig) Prepare() []error {
	var errs []error

	if c.Label == "" {
		c.Label = "OEMDRV"
	}

	return errs
}

func (s *StepCopyMediaFiles) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	vmname := s.Config.VMName
	namespace := s.Config.Namespace
	media := s.Config.Media

	ui.Sayf("Creating a new ConfigMap to store media files (%s/%s)...", namespace, vmname)

	data := make(map[string]string)

	for _, path := range media.Files {
		content, err := os.ReadFile(path)
		if err != nil {
			ui.Error(err.Error())
			return multistep.ActionHalt
		}

		filename := filepath.Base(path)
		data[filename] = string(content)
	}

	for filename, content := range media.Content {
		data[filename] = content
	}

	configMap := configMap(vmname, data)

	_, err := s.Client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepCopyMediaFiles) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packer.Ui)
	vmname := s.Config.VMName
	namespace := s.Config.Namespace

	if s.Config.Media.Keep {
		ui.Sayf("Keeping ConfigMap (%s/%s) because keep_media = true", namespace, vmname)
	} else {
		ui.Sayf("Deleting ConfigMap (%s/%s)...", namespace, vmname)

		_ = s.Client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), vmname, metav1.DeleteOptions{})
	}
}
