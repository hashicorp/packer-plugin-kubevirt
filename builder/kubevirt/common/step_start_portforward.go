// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"kubevirt.io/client-go/kubecli"
)

// defaultForwardingAddress is the local address the port forward listens on
// when no host is configured. The communicator runs in the plugin process, so
// there is no reason to expose the tunnel to the VM on other interfaces.
const defaultForwardingAddress = "127.0.0.1"

// PortForwardStep is a PortForwarder that can start forwarding and report errors.
type PortForwardStep interface {
	StartForwarding(address *net.IPAddr, port ForwardedPort) (net.Addr, error)
	LastError() error
	Close() error
}

type PortForwarderFactory func(kind, namespace, name string, resource PortforwardableResource) PortForwardStep

type StepStartPortForward struct {
	Config        BuilderConfig
	Client        kubecli.KubevirtClient
	ForwarderFunc PortForwarderFactory
	// Comm is the communicator configuration used to connect to the VM. Once
	// forwarding has started, its host and port are set to the local end of
	// the tunnel.
	Comm *communicator.Config

	forwarder PortForwardStep
}

func (s *StepStartPortForward) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var host string
	var localPort int

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace
	remotePort := s.Config.Comm.Port()

	switch s.Config.Comm.Type {
	case "ssh":
		host = s.Config.Comm.SSHHost
		localPort = s.Config.SSHLocalPort
	case "winrm":
		host = s.Config.Comm.WinRMHost
		localPort = s.Config.WinRMLocalPort
	default:
		log.Printf("[INFO] communicator %q does not need port forwarding", s.Config.Comm.Type)
		return multistep.ActionContinue
	}

	if host == "" {
		host = defaultForwardingAddress
	}

	if ctx.Err() != nil {
		return multistep.ActionHalt
	}

	address, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return Halt(state, fmt.Errorf("failed to resolve the port forwarding address %q: %w", host, err))
	}

	factory := s.ForwarderFunc
	if factory == nil {
		factory = DefaultPortForwarderFactory
	}
	forwarder := factory("vm", namespace, name, s.Client.VirtualMachine(namespace))

	listenAddr, err := forwarder.StartForwarding(address, ForwardedPort{
		Local:    localPort,
		Remote:   remotePort,
		Protocol: ProtocolTCP,
	})
	if err != nil {
		return Halt(state, fmt.Errorf("failed to start port forwarding to the VirtualMachine (%s/%s): %w", namespace, name, err))
	}
	s.forwarder = forwarder

	tcpAddr, ok := listenAddr.(*net.TCPAddr)
	if !ok {
		return Halt(state, fmt.Errorf("unexpected port forwarding listener address %v", listenAddr))
	}

	if s.Comm != nil {
		switch s.Config.Comm.Type {
		case "ssh":
			s.Comm.SSHHost = tcpAddr.IP.String()
			s.Comm.SSHPort = tcpAddr.Port
		case "winrm":
			s.Comm.WinRMHost = tcpAddr.IP.String()
			s.Comm.WinRMPort = tcpAddr.Port
		}
	}

	ui.Sayf("Forwarding %s to port %d of the VirtualMachine (%s/%s)", tcpAddr, remotePort, namespace, name)
	return multistep.ActionContinue
}

func (s *StepStartPortForward) Cleanup(state multistep.StateBag) {
	if s.forwarder == nil {
		return
	}

	if _, connected := state.GetOk("communicator"); !connected {
		if err := s.forwarder.LastError(); err != nil {
			state.Get("ui").(packer.Ui).Errorf("Last port forwarding error for the VirtualMachine (%s/%s): %s",
				s.Config.Namespace, s.Config.Name, err)
		}
	}

	if err := s.forwarder.Close(); err != nil {
		log.Printf("[WARN] failed to stop port forwarding: %s", err)
	}
	s.forwarder = nil
}

func DefaultPortForwarderFactory(kind, namespace, name string, resource PortforwardableResource) PortForwardStep {
	return &PortForwarder{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		Resource:  resource,
	}
}
