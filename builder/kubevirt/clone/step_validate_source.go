// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone

import (
	"context"
	"fmt"

	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
)

type StepValidateVolumeSource struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepValidateVolumeSource) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	sourceNamespace := s.Config.SourceVolumeNamespace
	sourceName := s.Config.SourceVolumeName

	ui.Sayf("Validating the source DataVolume (%s/%s)...", sourceNamespace, sourceName)

	dv, err := s.Client.CdiClient().CdiV1beta1().DataVolumes(sourceNamespace).Get(ctx, sourceName, metav1.GetOptions{})
	if err != nil {
		return kubevirtcommon.Halt(state, fmt.Errorf("failed to get the source DataVolume (%s/%s): %w", sourceNamespace, sourceName, err))
	}

	if err := kubevirtcommon.WaitUntilDataVolumeSucceeded(ctx, s.Client, sourceNamespace, sourceName); err != nil {
		return kubevirtcommon.Halt(state, fmt.Errorf("the source DataVolume (%s/%s) is not ready: %w", sourceNamespace, sourceName, err))
	}

	if s.Config.DiskSize != "" {
		sourcePVC, err := s.Client.CoreV1().PersistentVolumeClaims(sourceNamespace).Get(ctx, dv.Status.ClaimName, metav1.GetOptions{})
		if err != nil {
			return kubevirtcommon.Halt(state, fmt.Errorf("failed to get the source PVC for DataVolume (%s/%s): %w", sourceNamespace, sourceName, err))
		}

		sourceSize := sourcePVC.Spec.Resources.Requests[corev1.ResourceStorage]
		requestedSize := resource.MustParse(s.Config.DiskSize)
		if requestedSize.Cmp(sourceSize) < 0 {
			return kubevirtcommon.Halt(state, fmt.Errorf("disk_size %q is smaller than the source volume size %q", s.Config.DiskSize, sourceSize.String()))
		}
	}

	return multistep.ActionContinue
}

func (s *StepValidateVolumeSource) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
