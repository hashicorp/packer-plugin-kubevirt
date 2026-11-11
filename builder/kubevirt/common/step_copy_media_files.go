// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

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
	Config BuilderConfig
	Client kubernetes.Interface

	created bool
	uid     types.UID
}

func (s *StepCopyMediaFiles) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace

	ui.Sayf("Creating a new ConfigMap to store media files (%s/%s)...", namespace, name)

	cm, err := ConfigMap(name, s.Config.MediaFiles, s.Config.MediaContent)
	if err != nil {
		return Halt(state, fmt.Errorf("failed to read the media files: %w", err))
	}

	created, err := s.Client.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{})
	if err != nil {
		return Halt(state, fmt.Errorf("failed to create the ConfigMap (%s/%s): %w", namespace, name, err))
	}
	s.created = true
	s.uid = created.UID
	return multistep.ActionContinue
}

func (s *StepCopyMediaFiles) Cleanup(state multistep.StateBag) {
	if !s.created {
		return
	}

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace

	ui.Sayf("Deleting ConfigMap (%s/%s)...", namespace, name)

	_ = s.Client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), name, DeleteOnlyUID(s.uid))
}
