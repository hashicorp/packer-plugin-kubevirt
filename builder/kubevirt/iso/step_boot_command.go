// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"
	"log"
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
	name := s.config.Name
	namespace := s.config.Namespace
	bootCommand := strings.Join(s.config.BootCommand, "")
	bootWait := s.config.BootWait

	// Only connect to VNC when there is something to type, which also avoids
	// requiring access to the VNC subresource.
	if len(s.config.BootCommand) == 0 {
		log.Println("[INFO] No boot command given, skipping")
		return multistep.ActionContinue
	}

	if int64(bootWait) > 0 {
		ui.Sayf("Waiting %s to boot...", bootWait.String())

		select {
		case <-time.After(bootWait):
			break
		case <-ctx.Done():
			return multistep.ActionHalt
		}
	}

	streamInterface, err := s.client.VirtualMachineInstance(namespace).VNC(name)
	if err != nil {
		return halt(state, fmt.Errorf("failed to open a VNC connection to the VirtualMachineInstance (%s/%s): %w", namespace, name, err))
	}

	connection, err := vnc.Client(streamInterface.AsConn(), &vnc.ClientConfig{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to establish a VNC session: %w", err))
	}
	// KubeVirt allows a single VNC session per VM, release it once typing is
	// done so that it can be used to follow the installation.
	defer connection.Close()

	ui.Say("Typing the boot command... Keep only single VNC connection here!")

	command, err := interpolate.Render(bootCommand, &interpolate.Context{})
	if err != nil {
		return halt(state, fmt.Errorf("failed to render the boot command: %w", err))
	}

	sequence, err := bootcommand.GenerateExpressionSequence(command)
	if err != nil {
		return halt(state, fmt.Errorf("failed to parse the boot command: %w", err))
	}

	driver := bootcommand.NewVNCDriver(connection, time.Duration(0))
	if err := sequence.Do(ctx, driver); err != nil {
		return halt(state, fmt.Errorf("failed to type the boot command: %w", err))
	}
	return multistep.ActionContinue
}

func (s *StepBootCommand) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
