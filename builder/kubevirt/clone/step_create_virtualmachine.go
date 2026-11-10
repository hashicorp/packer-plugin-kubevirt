// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone

import (
	"context"

	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"

	"k8s.io/apimachinery/pkg/types"

	"kubevirt.io/client-go/kubecli"
)

type StepCreateVirtualMachine struct {
	Config Config
	Client kubecli.KubevirtClient

	created bool
	uid     types.UID
}

func (s *StepCreateVirtualMachine) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	hasMedia := len(s.Config.MediaContent) > 0 || len(s.Config.MediaFiles) > 0

	vm := virtualMachine(
		s.Config.Name,
		s.Config.Namespace,
		s.Config.SourceVolumeName,
		s.Config.SourceVolumeNamespace,
		s.Config.DiskSize,
		s.Config.InstanceType,
		s.Config.Preference,
		s.Config.InstanceTypeKind,
		s.Config.PreferenceKind,
		s.Config.DiskBus,
		s.Config.MediaLabel,
		hasMedia,
		s.Config.Networks,
		s.Config.StorageConfig)

	uid, err := kubevirtcommon.CreateVirtualMachine(ctx, state, s.Client, vm)
	s.uid = uid
	if err != nil {
		// The VM was created but failed to become ready; cleanup must delete it.
		s.created = uid != ""
		return kubevirtcommon.Halt(state, err)
	}
	s.created = true
	return multistep.ActionContinue
}

func (s *StepCreateVirtualMachine) Cleanup(state multistep.StateBag) {
	kubevirtcommon.CleanupVirtualMachine(state, s.Client, s.Config.BuilderConfig, s.created, s.uid)
}
