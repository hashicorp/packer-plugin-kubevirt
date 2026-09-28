// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

type StepCreateBootableVolume struct {
	Config        Config
	Client        kubecli.KubevirtClient
	GeneratedData *packerbuilderdata.GeneratedData
}

func (s *StepCreateBootableVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	diskSize := s.Config.DiskSize
	instanceType := s.Config.InstanceType
	instanceTypeKind := s.Config.InstanceTypeKind
	preferenceName := s.Config.Preference
	preferenceKind := s.Config.PreferenceKind
	cloneVolume := cloneVolume(name, namespace, diskSize)
	sourceVolume := sourceVolume(name, namespace, instanceType, instanceTypeKind, preferenceName, preferenceKind)

	ui.Sayf("Creating a new bootable volume (%s/%s)...", namespace, name)

	dv, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(namespace).Create(ctx, cloneVolume, metav1.CreateOptions{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to create the DataVolume (%s/%s): %w", namespace, name, err))
	}

	if err = WaitUntilDataVolumeSucceeded(ctx, s.Client, dv.Namespace, dv.Name); err != nil {
		return halt(state, fmt.Errorf("the DataVolume (%s/%s) did not succeed: %w", namespace, name, err))
	}

	ds, err := s.Client.CdiClient().CdiV1beta1().DataSources(namespace).Create(ctx, sourceVolume, metav1.CreateOptions{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to create the DataSource (%s/%s): %w", namespace, name, err))
	}

	state.Put("bootable_volume_name", ds.Name)
	state.Put("bootable_volume_namespace", namespace)
	s.GeneratedData.Put("BootableVolumeName", ds.Name)
	return multistep.ActionContinue
}

func (s *StepCreateBootableVolume) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
