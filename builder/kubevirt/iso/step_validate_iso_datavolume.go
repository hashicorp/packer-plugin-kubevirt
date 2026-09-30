// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

type StepValidateIsoDataVolume struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepValidateIsoDataVolume) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	isoVolumeNamespace := s.Config.Namespace
	isoVolumeName := s.Config.IsoVolumeName

	ui.Sayf("Validating the existence of the ISO DataVolume (%s/%s)...", isoVolumeNamespace, isoVolumeName)

	_, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(isoVolumeNamespace).Get(ctx, isoVolumeName, metav1.GetOptions{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to get the ISO DataVolume (%s/%s): %w", isoVolumeNamespace, isoVolumeName, err))
	}

	if err := WaitUntilDataVolumeSucceeded(ctx, s.Client, isoVolumeNamespace, isoVolumeName); err != nil {
		return halt(state, fmt.Errorf("the ISO DataVolume (%s/%s) is not ready: %w", isoVolumeNamespace, isoVolumeName, err))
	}
	return multistep.ActionContinue
}

func (s *StepValidateIsoDataVolume) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
