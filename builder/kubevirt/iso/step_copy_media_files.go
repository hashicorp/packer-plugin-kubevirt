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
	Config Config
	Client kubernetes.Interface
}

func (s *StepCopyMediaFiles) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	vmname := s.Config.VMName
	namespace := s.Config.Namespace
	mediaFiles := s.Config.MediaFiles

	ui.Sayf("Creating a new ConfigMap to store media files (%s/%s)...", namespace, vmname)

	configMap, err := configMap(vmname, mediaFiles)
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	_, err = s.Client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
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

	ui.Sayf("Deleting ConfigMap (%s/%s)...", namespace, vmname)

	_ = s.Client.CoreV1().ConfigMaps(namespace).Delete(context.Background(), vmname, metav1.DeleteOptions{})
}
