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

type StepValidateIsoDataVolume struct {
	config Config
	client kubecli.KubevirtClient
}

func (s *StepValidateIsoDataVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	isoVolumeNamespace := s.config.Namespace
	isoVolumeName := s.config.IsoVolumeName

	ui.Sayf("Validating the existence of the ISO DataVolume (%s/%s)...", isoVolumeNamespace, isoVolumeName)

	_, err := s.client.CdiClient().CdiV1beta1().DataVolumes(isoVolumeNamespace).Get(ctx, isoVolumeName, metav1.GetOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if err := waitUntilDataVolumeSucceeded(ctx, s.client, isoVolumeNamespace, isoVolumeName); err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepValidateIsoDataVolume) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
