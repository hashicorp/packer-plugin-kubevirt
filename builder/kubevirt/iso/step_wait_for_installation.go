// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

type StepWaitForInstallation struct {
	config Config
}

func (s *StepWaitForInstallation) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	installationWaitTimeout := s.config.InstallationWaitTimeout

	if int64(installationWaitTimeout) > 0 {
		ui.Sayf("Waiting %s to complete ISO installation...", installationWaitTimeout.String())

		select {
		case <-time.After(installationWaitTimeout):
			break
		case <-ctx.Done():
			return multistep.ActionHalt
		}
	}
	return multistep.ActionContinue
}

func (s *StepWaitForInstallation) Cleanup(multistep.StateBag) {
	// Left blank intentionally
}
