// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/bootcommand"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/mitchellh/go-vnc"

	"kubevirt.io/client-go/kubecli"
)

type StepBootCommand struct {
	config Config
	client kubecli.KubevirtClient
}

func (s *StepBootCommand) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packer.Ui)
	vmname := s.config.VMName
	namespace := s.config.Namespace
	bootCommand := strings.Join(s.config.BootCommand, "")
	bootWait := s.config.BootWait

	if int64(bootWait) > 0 {
		ui.Sayf("Waiting %s to boot...", bootWait.String())

		select {
		case <-time.After(bootWait):
			break
		case <-ctx.Done():
			return multistep.ActionHalt
		}
	}

	streamInterface, err := s.client.VirtualMachineInstance(namespace).VNC(vmname)
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	connection, err := vnc.Client(streamInterface.AsConn(), &vnc.ClientConfig{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	ui.Say("Typing the boot command... Keep only single VNC connection here!")

	command, err := interpolate.Render(bootCommand, &interpolate.Context{})
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	sequence, err := bootcommand.GenerateExpressionSequence(command)
	if err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}

	driver := bootcommand.NewVNCDriver(connection, time.Duration(0))
	if err := sequence.Do(ctx, driver); err != nil {
		ui.Error(err.Error())
		return multistep.ActionHalt
	}
	return multistep.ActionContinue
}

func (s *StepBootCommand) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
