// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

type StepWaitForInstallation struct {
	Config Config
	Client kubecli.KubevirtClient
}

func (s *StepWaitForInstallation) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)

	// When a communicator is configured, it handles readiness detection
	// through its own retry loop -no need to wait here.
	if s.Config.Comm.Type != "none" {
		return multistep.ActionContinue
	}

	installationWaitTimeout := s.Config.InstallationWaitTimeout
	if installationWaitTimeout <= 0 {
		return halt(state, fmt.Errorf("installation_wait_timeout must be set when no communicator is configured"))
	}

	ui.Sayf("Waiting for the guest agent to report the installation is complete (timeout: %s)...", installationWaitTimeout)

	agentConnected, err := s.waitForAgentConnected(ctx, installationWaitTimeout)
	if err != nil {
		if ctx.Err() != nil {
			return multistep.ActionHalt
		}
		return halt(state, fmt.Errorf("error while waiting for the installation to complete: %w", err))
	}

	if agentConnected {
		ui.Say("Guest agent connected -installation is complete.")
	} else {
		ui.Error("WARNING: The guest agent did not connect within the timeout. " +
			"This may mean the guest agent is not installed, or the installation has not finished. " +
			"The build will continue, but the resulting image may contain an incomplete installation.")
	}

	return multistep.ActionContinue
}

func (s *StepWaitForInstallation) waitForAgentConnected(ctx context.Context, timeout time.Duration) (bool, error) {
	name := s.Config.Name
	namespace := s.Config.Namespace
	pollInterval := 30 * time.Second

	connected := false
	err := wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		vmi, err := s.Client.VirtualMachineInstance(namespace).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			// The VMI may be temporarily gone (e.g. after a guest poweroff
			// with RunStrategyAlways, KubeVirt deletes and recreates it).
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to get VirtualMachineInstance %s/%s: %w", namespace, name, err)
		}

		for _, condition := range vmi.Status.Conditions {
			if condition.Type == v1.VirtualMachineInstanceAgentConnected && condition.Status == corev1.ConditionTrue {
				connected = true
				return true, nil
			}
		}
		return false, nil
	})

	// Check context cancellation before wait.Interrupted: both timeout and
	// cancellation satisfy Interrupted, but only a cancellation should
	// propagate an error to halt the build.
	if err != nil {
		if ctx.Err() != nil {
			return false, errors.Join(err, ctx.Err())
		}
		if wait.Interrupted(err) {
			return false, nil
		}
	}
	return connected, err
}

func (s *StepWaitForInstallation) Cleanup(multistep.StateBag) {
	// Left blank intentionally
}
