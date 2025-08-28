// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"net"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"kubevirt.io/client-go/kubecli"
)

type StepStartPortForward struct {
	config Config
	client kubecli.KubevirtClient
}

func (s *StepStartPortForward) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var ipAddress string
	var localPort int
	var remotePort int

	ui := state.Get("ui").(packer.Ui)
	name := s.config.Name
	namespace := s.config.Namespace

	if s.config.Communicator == "ssh" {
		ipAddress = s.config.SSHHost
		localPort = s.config.SSHLocalPort
		remotePort = s.config.SSHRemotePort
	}

	if s.config.Communicator == "winrm" {
		ipAddress = s.config.WinRMHost
		localPort = s.config.WinRMLocalPort
		remotePort = s.config.WinRMRemotePort
	}

	address, _ := net.ResolveIPAddr("", ipAddress)
	vm := s.client.VirtualMachine(namespace)

	errChan := make(chan error, 1)
	go func() {
		forward := common.PortForward{
			Address:  address,
			Resource: vm,
		}
		forwarder := common.PortForwarder{
			Kind:      "vm",
			Namespace: namespace,
			Name:      name,
			Resource:  forward.Resource,
		}

		err := forwarder.StartForwarding(forward.Address, common.ForwardedPort{
			Local:    localPort,
			Remote:   remotePort,
			Protocol: common.ProtocolTCP,
		})
		errChan <- err
	}()

	select {
	case <-ctx.Done():
		ui.Say("Context cancelled, stopping port forwarding...")
		return multistep.ActionHalt
	case err := <-errChan:
		if err != nil {
			ui.Error(err.Error())
			return multistep.ActionHalt
		}
	}
	return multistep.ActionContinue
}

func (s *StepStartPortForward) Cleanup(state multistep.StateBag) {
	// Left blank intentionally
}
