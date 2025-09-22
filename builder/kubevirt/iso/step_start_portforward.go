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
	Config        Config
	Client        kubecli.KubevirtClient
	ForwarderFunc PortForwarderFactory
}

type PortForwarder interface {
	StartForwarding(address *net.IPAddr, port common.ForwardedPort) error
}

type PortForwarderFactory func(kind, namespace, name string, resource common.PortforwardableResource) PortForwarder

func (s *StepStartPortForward) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var ipAddress string
	var localPort int
	var remotePort int

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace

	switch s.Config.Comm.Type {
	case "none":
		ui.Sayf("communicator type 'none', skipping port forwarding setup")
		return multistep.ActionContinue
	case "ssh":
		localPort = s.Config.SSHLocalPort
	case "winrm":
		localPort = s.Config.WinRMLocalPort
	}
	state.Put("forwarding_port", localPort)

	ipAddress = s.Config.Comm.Host()
	remotePort = s.Config.Comm.Port()

	address, _ := net.ResolveIPAddr("", ipAddress)
	vm := s.Client.VirtualMachine(namespace)

	// Use the factory if provided, otherwise fallback to default
	factory := s.ForwarderFunc
	if factory == nil {
		factory = DefaultPortForwarder
	}
	forwarder := factory("vm", namespace, name, vm)

	errChan := make(chan error, 1)
	go func() {
		err := forwarder.StartForwarding(address, common.ForwardedPort{
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

func DefaultPortForwarder(kind, namespace, name string, resource common.PortforwardableResource) PortForwarder {
	return &common.PortForwarder{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		Resource:  resource,
	}
}
