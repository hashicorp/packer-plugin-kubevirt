// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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
	name := s.Config.Name
	namespace := s.Config.Namespace

	ui.Sayf("Stopping the temporary VirtualMachine (%s/%s)...", namespace, name)

	// The VM status is updated concurrently by KubeVirt, e.g. while the guest
	// shuts down after provisioning, so retry when the update conflicts.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		vm, err := s.Client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("failed to get the VirtualMachine (%s/%s): %w", namespace, name, err)
		}
		vm.Spec.RunStrategy = ptr.To(v1.RunStrategyHalted)

		if _, err := s.Client.VirtualMachine(vm.Namespace).Update(ctx, vm, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("failed to stop the VirtualMachine (%s/%s): %w", namespace, name, err)
		}
		return nil
	})
	if err != nil {
		return halt(state, err)
	}
	return multistep.ActionContinue
}

func (s *StepStopVirtualMachine) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
