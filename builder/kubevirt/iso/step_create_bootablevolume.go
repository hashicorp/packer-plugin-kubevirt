// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

type StepCreateBootableVolume struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepCreateBootableVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	diskSize := s.Config.DiskSize
	instanceType := s.Config.InstanceType
	preferenceName := s.Config.Preference
	cloneVolume := cloneVolume(name, namespace, diskSize, s.Config.StorageClassName)
	sourceVolume := sourceVolume(name, namespace, instanceType, preferenceName)

	ui.Sayf("Creating a new bootable volume (%s/%s)...", namespace, name)

	dv, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(namespace).Create(ctx, cloneVolume, metav1.CreateOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if err = WaitUntilDataVolumeSucceeded(ctx, s.Client, dv.Namespace, dv.Name); err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ds, err := s.Client.CdiClient().CdiV1beta1().DataSources(namespace).Create(ctx, sourceVolume, metav1.CreateOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	state.Put("bootable_volume_name", ds.Name)
	return multistep.ActionContinue
}

func (s *StepCreateBootableVolume) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
