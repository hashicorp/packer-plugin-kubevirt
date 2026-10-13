// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

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
	osType := s.Config.OperatingSystemType
	if osType == "" || (osType != "linux" && osType != "windows") {
		return kubevirtcommon.Halt(state, fmt.Errorf("OS type of '%s' is not supported, set 'linux' or 'windows'", osType))
	}

	vm := virtualMachine(
		s.Config.Name,
		s.Config.Namespace,
		s.Config.IsoVolumeName,
		s.Config.DiskSize,
		s.Config.InstanceType,
		s.Config.Preference,
		s.Config.InstanceTypeKind,
		s.Config.PreferenceKind,
		osType,
		s.Config.DiskBus,
		s.Config.MediaLabel,
		s.Config.VirtIOContainerImage,
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
