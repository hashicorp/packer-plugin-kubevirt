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
	called bool
	err    error
}

func (m *mockPortForwarder) StartForwarding(address *net.IPAddr, port common.ForwardedPort) error {
	m.called = true
	return m.err
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
					SSH: communicator.SSH{
						SSHHost: "127.0.0.1",
					},
				},
				SSHLocalPort:  2222,
				SSHRemotePort: 22,
			},
			Client: virtClient,
			ForwarderFunc: func(kind, ns, n string, resource common.PortforwardableResource) iso.PortForwarder {
				return mockFwd
			},
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
		})

		It("halts when forwarding returns an error", func() {
			mockFwd.err = fmt.Errorf("simulated forward error")
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("halts when context is cancelled", func() {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("works with WinRM configuration", func() {
			step.Config.Comm = communicator.Config{
				Type: "winrm",
				WinRM: communicator.WinRM{
					WinRMHost: "127.0.0.1",
				},
			}
			step.Config.WinRMLocalPort = 5985
			step.Config.WinRMRemotePort = 5985

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(mockFwd.called).To(BeTrue())
		})
	})
})
