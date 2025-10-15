// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type StepStopVirtualMachine struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepStopVirtualMachine) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	vmname := s.Config.VMName
	namespace := s.Config.Namespace

	ui.Sayf("Stopping the temporary VirtualMachine (%s/%s)...", namespace, vmname)

	vm, err := s.Client.VirtualMachine(namespace).Get(ctx, vmname, metav1.GetOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	vm.Spec.RunStrategy = ptr.To(v1.RunStrategyHalted)

	_, err = s.Client.VirtualMachine(vm.Namespace).Update(ctx, vm, metav1.UpdateOptions{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepStopVirtualMachine) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
