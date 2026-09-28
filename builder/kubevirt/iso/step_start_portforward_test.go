// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	kubecli "kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
)

type mockPortForwarder struct {
	called  bool
	closed  bool
	address *net.IPAddr
	port    common.ForwardedPort
	err     error
	lastErr error
}

// StartForwarding mimics the real forwarder: a local port of 0 is replaced by
// an allocated one.
func (m *mockPortForwarder) StartForwarding(address *net.IPAddr, port common.ForwardedPort) (net.Addr, error) {
	m.called = true
	m.address = address
	m.port = port
	if m.err != nil {
		return nil, m.err
	}

	local := port.Local
	if local == 0 {
		local = 40123
	}
	return &net.TCPAddr{IP: address.IP, Port: local}, nil
}

func (m *mockPortForwarder) LastError() error {
	return m.lastErr
}

func (m *mockPortForwarder) Close() error {
	m.closed = true
	return nil
}

var _ = Describe("StepStartPortForward", func() {
	const (
		namespace = "test-ns"
		name      = "test-vm"
	)

	var (
		mockCtrl   *gomock.Controller
		vmClient   *kubevirtfake.Clientset
		virtClient kubecli.KubevirtClient
		mockVirt   *kubecli.MockKubevirtClient
		state      *multistep.BasicStateBag
		uiErr      *strings.Builder
		step       *iso.StepStartPortForward
		mockFwd    *mockPortForwarder
	)

	BeforeEach(func() {
		uiErr = &strings.Builder{}
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: uiErr,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		mockCtrl = gomock.NewController(GinkgoT())
		vmClient = kubevirtfake.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		mockVirt = kubecli.NewMockKubevirtClient(mockCtrl)
		kubecli.MockKubevirtClientInstance = mockVirt

		mockVirt.EXPECT().
			VirtualMachine(namespace).
			Return(vmClient.KubevirtV1().VirtualMachines(namespace)).
			AnyTimes()

		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		mockFwd = &mockPortForwarder{}
		step = &iso.StepStartPortForward{
			Config: iso.Config{
				Name:      name,
				Namespace: namespace,
				Comm: communicator.Config{
					Type: "ssh",
					SSH:  communicator.SSH{SSHHost: "127.0.0.1", SSHPort: 22},
				},
				PortForwardConfig: iso.PortForwardConfig{SSHLocalPort: 2222},
			},
			Client: virtClient,
			ForwarderFunc: func(kind, ns, n string, resource common.PortforwardableResource) iso.PortForwarder {
				return mockFwd
			},
			Comm: &communicator.Config{Type: "ssh"},
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Run", func() {
		It("continues when forwarding succeeds", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(mockFwd.called).To(BeTrue())
			Expect(mockFwd.port).To(Equal(common.ForwardedPort{Local: 2222, Remote: 22, Protocol: common.ProtocolTCP}))
		})

		It("points the communicator at the local end of the tunnel", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(step.Comm.SSHHost).To(Equal("127.0.0.1"))
			Expect(step.Comm.SSHPort).To(Equal(2222))
		})

		It("listens on the loopback address and an allocated port by default", func() {
			step.Config.Comm.SSHHost = ""
			step.Config.SSHLocalPort = 0

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(mockFwd.address.IP.String()).To(Equal("127.0.0.1"))
			Expect(mockFwd.port.Local).To(Equal(0))
			Expect(step.Comm.SSHHost).To(Equal("127.0.0.1"))
			Expect(step.Comm.SSHPort).To(Equal(40123))
		})

		It("halts when the forwarding address cannot be resolved", func() {
			step.Config.Comm.SSHHost = "invalid host name"

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(mockFwd.called).To(BeFalse())
			Expect(state.Get("error")).To(MatchError(ContainSubstring("failed to resolve the port forwarding address")))
		})

		It("halts when forwarding returns an error", func() {
			mockFwd.err = fmt.Errorf("simulated forward error")
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("simulated forward error")))
		})

		It("halts when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(mockFwd.called).To(BeFalse())
		})

		It("works with WinRM configuration", func() {
			step.Config.Comm = communicator.Config{
				Type:  "winrm",
				WinRM: communicator.WinRM{WinRMHost: "127.0.0.1", WinRMPort: 5985},
			}
			step.Config.WinRMLocalPort = 5000
			step.Comm = &communicator.Config{Type: "winrm"}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(mockFwd.called).To(BeTrue())
			Expect(mockFwd.port).To(Equal(common.ForwardedPort{Local: 5000, Remote: 5985, Protocol: common.ProtocolTCP}))
			Expect(step.Comm.WinRMHost).To(Equal("127.0.0.1"))
			Expect(step.Comm.WinRMPort).To(Equal(5000))
		})

		It("does nothing without an SSH or WinRM communicator", func() {
			step.Config.Comm = communicator.Config{Type: "none"}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(mockFwd.called).To(BeFalse())
		})
	})

	Context("Cleanup", func() {
		It("stops forwarding", func() {
			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))

			step.Cleanup(state)
			Expect(mockFwd.closed).To(BeTrue())
		})

		It("does nothing when forwarding was not started", func() {
			step.Cleanup(state)
			Expect(mockFwd.closed).To(BeFalse())
		})

		It("reports the last tunnel error when the communicator could not connect", func() {
			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			mockFwd.lastErr = fmt.Errorf("Websocket failed with http status: 403 Forbidden")

			step.Cleanup(state)
			Expect(uiErr.String()).To(ContainSubstring(
				"Last port forwarding error for the VirtualMachine (test-ns/test-vm): Websocket failed with http status: 403 Forbidden"))
		})

		It("does not report tunnel errors once the communicator connected", func() {
			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			mockFwd.lastErr = fmt.Errorf("stream closed")
			state.Put("communicator", "connected")

			step.Cleanup(state)
			Expect(uiErr.String()).To(BeEmpty())
		})
	})
})
