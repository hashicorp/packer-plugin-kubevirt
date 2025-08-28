// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

type StepCopyMediaFiles struct {
	config Config
	client *kubernetes.Clientset
}

func (s *StepCopyMediaFiles) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.config.Name
	namespace := s.config.Namespace
	mediaFiles := s.config.MediaFiles

	ui.Sayf("Creating a new ConfigMap to store media files (%s/%s)...", namespace, name)

	configMap, err := configMap(name, mediaFiles)
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	_, err = s.client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepCopyMediaFiles) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packer.Ui)
	name := s.config.Name
	namespace := s.config.Namespace

	ui.Sayf("Deleting ConfigMap (%s/%s)...", namespace, name)

	s.client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), name, metav1.DeleteOptions{})
}
