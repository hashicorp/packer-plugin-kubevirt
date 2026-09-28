// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"
	"log"
	"net"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"kubevirt.io/client-go/kubecli"
)

// defaultForwardingAddress is the local address the port forward listens on
// when no host is configured. The communicator runs in the plugin process, so
// there is no reason to expose the tunnel to the VM on other interfaces.
const defaultForwardingAddress = "127.0.0.1"

type StepStartPortForward struct {
	Config        Config
	Client        kubecli.KubevirtClient
	ForwarderFunc PortForwarderFactory
	// Comm is the communicator configuration used to connect to the VM. Once
	// forwarding has started, its host and port are set to the local end of
	// the tunnel.
	Comm *communicator.Config

	forwarder PortForwarder
}

type PortForwarder interface {
	StartForwarding(address *net.IPAddr, port common.ForwardedPort) (net.Addr, error)
	Close() error
}

type PortForwarderFactory func(kind, namespace, name string, resource common.PortforwardableResource) PortForwarder

func (s *StepStartPortForward) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	var host string
	var localPort int
	var remotePort int

	ui := state.Get("ui").(packer.Ui)
	name := s.Config.Name
	namespace := s.Config.Namespace

	if s.Config.Communicator == "ssh" {
		host = s.Config.SSHHost
		localPort = s.Config.SSHLocalPort
		remotePort = s.Config.SSHRemotePort
	}

	if s.Config.Communicator == "winrm" {
		host = s.Config.WinRMHost
		localPort = s.Config.WinRMLocalPort
		remotePort = s.Config.WinRMRemotePort
	}

	if host == "" {
		host = defaultForwardingAddress
	}

	if ctx.Err() != nil {
		return multistep.ActionHalt
	}

	address, err := net.ResolveIPAddr("ip", host)
	if err != nil {
		return halt(state, fmt.Errorf("failed to resolve the port forwarding address %q: %w", host, err))
	}

	// Use the factory if provided, otherwise fallback to default
	factory := s.ForwarderFunc
	if factory == nil {
		factory = DefaultPortForwarder
	}
	forwarder := factory("vm", namespace, name, s.Client.VirtualMachine(namespace))

	// A local port of 0 lets the operating system allocate a free port, which
	// avoids conflicts between concurrent builds.
	listenAddr, err := forwarder.StartForwarding(address, common.ForwardedPort{
		Local:    localPort,
		Remote:   remotePort,
		Protocol: common.ProtocolTCP,
	})
	if err != nil {
		return halt(state, fmt.Errorf("failed to start port forwarding to the VirtualMachine (%s/%s): %w", namespace, name, err))
	}
	s.forwarder = forwarder

	tcpAddr, ok := listenAddr.(*net.TCPAddr)
	if !ok {
		return halt(state, fmt.Errorf("unexpected port forwarding listener address %v", listenAddr))
	}

	if s.Comm != nil {
		switch s.Comm.Type {
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

	if err := s.forwarder.Close(); err != nil {
		log.Printf("[WARN] failed to stop port forwarding: %s", err)
	}
	s.forwarder = nil
}

func DefaultPortForwarder(kind, namespace, name string, resource common.PortforwardableResource) PortForwarder {
	return &common.PortForwarder{
		Kind:      kind,
		Namespace: namespace,
		Name:      name,
		Resource:  resource,
	}
}
