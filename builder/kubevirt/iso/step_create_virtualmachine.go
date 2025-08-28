// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	ptr "k8s.io/utils/ptr"

	"kubevirt.io/client-go/kubecli"
)

type StepCreateVirtualMachine struct {
	config Config
	client kubecli.KubevirtClient
}

func (s *StepCreateVirtualMachine) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	name := s.config.Name
	namespace := s.config.Namespace
	isoVolumeName := s.config.IsoVolumeName
	diskSize := s.config.DiskSize
	instanceTypeName := s.config.InstanceType
	instanceTypeKind := s.config.InstanceTypeKind
	preferenceName := s.config.Preference
	preferenceKind := s.config.PreferenceKind
	osType := s.config.OperatingSystemType
	networks := s.config.Networks

	if osType == "" || (osType != "linux" && osType != "windows") {
		ui.Errorf("OS type of '%s' is not supported, set 'linux' or 'windows'.", osType)
		return multistep.ActionHalt
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
		networks)

	ui.Sayf("Creating a new temporary VirutalMachine (%s/%s)...", namespace, name)

	_, err := s.client.VirtualMachine(namespace).Create(ctx, virtualMachine, metav1.CreateOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	if err := s.waitUntilVirtualMachineReady(ctx); err != nil {
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepCreateVirtualMachine) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packer.Ui)
	name := s.config.Name
	namespace := s.config.Namespace
	keepVM := s.config.KeepVM

	if keepVM {
		ui.Sayf("Keeping VirutalMachine (%s/%s).", namespace, name)
		return
	}

	ui.Sayf("Deleting VirutalMachine (%s/%s)...", namespace, name)

	s.client.VirtualMachine(namespace).Delete(context.Background(), name, metav1.DeleteOptions{
		GracePeriodSeconds: ptr.To(int64(0)),
	})
}

func (s *StepCreateVirtualMachine) waitUntilVirtualMachineReady(ctx context.Context) error {
	name := s.config.Name
	namespace := s.config.Namespace
	pollInterval := 5 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		vm, err := s.client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
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
