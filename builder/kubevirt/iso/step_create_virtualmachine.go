// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ptr "k8s.io/utils/ptr"

	"kubevirt.io/client-go/kubecli"
)

type StepCreateVirtualMachine struct {
	Config Config
	Client kubecli.KubevirtClient

	created bool
	uid     types.UID
}

func (s *StepCreateVirtualMachine) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	isoVolumeName := s.Config.IsoVolumeName
	diskSize := s.Config.DiskSize
	instanceTypeName := s.Config.InstanceType
	instanceTypeKind := s.Config.InstanceTypeKind
	preferenceName := s.Config.Preference
	preferenceKind := s.Config.PreferenceKind
	osType := s.Config.OperatingSystemType
	diskBus := s.Config.DiskBus
	mediaLabel := s.Config.MediaLabel
	virtioContainerImage := s.Config.VirtIOContainerImage
	networks := s.Config.Networks

	if osType == "" || (osType != "linux" && osType != "windows") {
		return halt(state, fmt.Errorf("OS type of '%s' is not supported, set 'linux' or 'windows'", osType))
	}

	virtualMachine := virtualMachine(
		name,
		isoVolumeName,
		diskSize,
		instanceTypeName,
		preferenceName,
		instanceTypeKind,
		preferenceKind,
		osType,
		diskBus,
		mediaLabel,
		virtioContainerImage,
		networks,
		s.Config.StorageConfig)

	ui.Sayf("Creating a new temporary VirtualMachine (%s/%s)...", namespace, name)

	vm, err := s.Client.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to create the VirtualMachine (%s/%s): %w", namespace, name, err))
	}
	s.created = true
	s.uid = vm.UID

	if err := s.waitUntilVirtualMachineReady(ctx); err != nil {
		return halt(state, fmt.Errorf("the VirtualMachine (%s/%s) did not become ready: %w", namespace, name, err))
	}
	return multistep.ActionContinue
}

func (s *StepCreateVirtualMachine) Cleanup(state multistep.StateBag) {
	// Never delete a VirtualMachine that this build did not create, e.g. one
	// that already existed with the same name and made the creation fail.
	if !s.created {
		return
	}

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	keepVM := s.Config.KeepVM

	if keepVM {
		ui.Sayf("Keeping VirtualMachine (%s/%s).", namespace, name)
		return
	}

	ui.Sayf("Deleting VirtualMachine (%s/%s)...", namespace, name)

	deleteOptions := deleteOnlyUID(s.uid)
	deleteOptions.GracePeriodSeconds = ptr.To(int64(0))
	_ = s.Client.VirtualMachine(namespace).Delete(context.Background(), name, deleteOptions)
}

func (s *StepCreateVirtualMachine) waitUntilVirtualMachineReady(ctx context.Context) error {
	name := s.Config.Name
	namespace := s.Config.Namespace
	pollInterval := 5 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		vm, err := s.Client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if vm.Status.Ready {
			return true, nil
		}
		return false, nil
	}

	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}
