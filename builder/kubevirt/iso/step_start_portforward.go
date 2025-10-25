// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc struct-markdown
//go:generate packer-sdc mapstructure-to-hcl2 -type PortForwardConfig

package iso

import (
	"context"
	"net"
	"strconv"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"kubevirt.io/client-go/kubecli"
)

type PortForwardConfig struct {
	// If true, disable the built-in port forwarding via Kubernetes control-plane.
	// By default, the Kubernetes control-plane forwarding is used.
	DisableForwarding bool `mapstructure:"disable_forwarding" required:"false"`
	// ForwardingPort is the local port used for port-forwarding to the VM for the
	// appropriate communicator. If this is not set, or set to 0, then a local ephemeral
	// port will be allocated during the build process and used as the forwarding port.
	ForwardingPort int `mapstructure:"forwarding_port" required:"false"`
}

type StepStartPortForward struct {
	Config        Config
	Client        kubecli.KubevirtClient
	ForwarderFunc PortForwarderFactory
}

type PortForwarder interface {
	StartForwarding(address *net.IPAddr, port common.ForwardedPort) (net.Addr, error)
}

type PortForwarderFactory func(kind, namespace, name string, resource common.PortforwardableResource) PortForwarder

func (s *StepStartPortForward) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var localAddress string
	var localPort int
	var remotePort int

	ui := state.Get("ui").(packer.Ui)
	vmname := s.Config.VMName
	namespace := s.Config.Namespace

	if s.Config.Comm.Type == "none" {
		// This should not get called, but regardless:
		ui.Say("communicator type 'none', skipping port forwarding setup")
		return multistep.ActionContinue
	}

	if s.Config.PortForwardConfig.DisableForwarding {
		ui.Say("disable_forwarding=true, skipping port forwarding setup")
		return multistep.ActionContinue
	}

	localAddress = "localhost"
	remotePort = s.Config.Comm.Port()

	ui.Sayf("Preparing port forwarding from %s:%d to VM on port %d", localAddress, localPort, remotePort)

	address, _ := net.ResolveIPAddr("ip", localAddress)
	vmi := s.Client.VirtualMachineInstance(namespace)

	// Use the factory if provided, otherwise fallback to default
	factory := s.ForwarderFunc
	if factory == nil {
		factory = DefaultPortForwarder
	}
	forwarder := factory("vmi", namespace, vmname, vmi)

	errChan := make(chan error, 1)
	portChan := make(chan int, 1)
	go func() {
		addr, err := forwarder.StartForwarding(address, common.ForwardedPort{
			Local:    localPort,
			Remote:   remotePort,
			Protocol: common.ProtocolTCP,
		})
		if err != nil {
			errChan <- err
			return
		}
		_, port, splitErr := net.SplitHostPort(addr.String())
		if splitErr != nil {
			errChan <- splitErr
			return
		}
		intport, convErr := strconv.Atoi(port)
		if convErr != nil {
			errChan <- convErr
			return
		}
		portChan <- intport
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
	case listenPort := <-portChan:
		ui.Sayf("Port forwarding enabled, listening on %s:%d, forwarding to VM port %d", localAddress, listenPort, remotePort)
		state.Put("forwarding_host", localAddress)
		state.Put("forwarding_port", listenPort)
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
