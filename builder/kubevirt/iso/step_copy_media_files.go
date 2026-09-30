// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

type StepCopyMediaFiles struct {
	Config Config
	Client kubernetes.Interface

	created bool
	uid     types.UID
}

func (s *StepCopyMediaFiles) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	mediaFiles := s.Config.MediaFiles

	ui.Sayf("Creating a new ConfigMap to store media files (%s/%s)...", namespace, name)

	configMap, err := configMap(name, mediaFiles)
	if err != nil {
		return halt(state, fmt.Errorf("failed to read the media files: %w", err))
	}

	cm, err := s.Client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to create the ConfigMap (%s/%s): %w", namespace, name, err))
	}
	s.created = true
	s.uid = cm.UID
	return multistep.ActionContinue
}

func (s *StepCopyMediaFiles) Cleanup(state multistep.StateBag) {
	// Never delete a ConfigMap that this build did not create, e.g. one that
	// already existed with the same name and made the creation fail.
	if !s.created {
		return
	}

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace

	ui.Sayf("Deleting ConfigMap (%s/%s)...", namespace, name)

	_ = s.Client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), name, deleteOnlyUID(s.uid))
}
