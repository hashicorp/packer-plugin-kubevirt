// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	k8sv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

var _ = Describe("StepWaitForIp", func() {
	const (
		namespace = "test-ns"
		name      = "test-vm"
	)

	var (
		errorBuffer *strings.Builder
		logBuffer   bytes.Buffer
		ctrl        *gomock.Controller
		state       *multistep.BasicStateBag
		step        *iso.StepWaitForAgent
		virtClient  kubecli.KubevirtClient
		vmClient    *kubevirtfake.Clientset
		mockVirt    *kubecli.MockKubevirtClient
		vmi         *v1.VirtualMachineInstance
	)

	BeforeEach(func() {
		log.SetOutput(&logBuffer)

		errorBuffer = &strings.Builder{}
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: errorBuffer,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		ctrl = gomock.NewController(GinkgoT())
		vmClient = kubevirtfake.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		mockVirt = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance = mockVirt

		mockVirt.EXPECT().
			VirtualMachineInstance(namespace).
			Return(vmClient.KubevirtV1().VirtualMachineInstances(namespace)).
			AnyTimes()

		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		vmi = &v1.VirtualMachineInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: v1.VirtualMachineInstanceStatus{
				Conditions: []v1.VirtualMachineInstanceCondition{},
			},
		}

		step = &iso.StepWaitForAgent{
			Config: iso.Config{
				Name:               name,
				Namespace:          namespace,
				WaitForAgentConfig: iso.WaitForAgentConfig{},
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		log.SetOutput(os.Stderr)
		log.Print(logBuffer.String())
		logBuffer.Reset()
		ctrl.Finish()
	})

	Context("Run", func() {
		It("continues immediately when no wait time specified", func() {
			step.Config.AgentWaitTimeout = 0 * time.Second

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(state.GetOk("ip")).To(BeNil())
		})

		It("halts when the VMI cannot be found", func() {
			step.Config.AgentWaitTimeout = 2 * time.Second

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(errorBuffer.String()).To(ContainSubstring("not found"))
		})

		It("halts when context is cancelled before wait time elapses", func() {
			step.Config.AgentWaitTimeout = 5 * time.Second

			vmClient.KubevirtV1().VirtualMachineInstances(namespace).Create(context.Background(),
				vmi, metav1.CreateOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(1 * time.Second)
				cancel()
			}()

			action := step.Run(ctx, state)
			Expect(errorBuffer.String()).To(ContainSubstring("wait cancelled"))
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("waits for the agent to become available and continues", func() {
			step.Config.AgentWaitTimeout = 5 * time.Second

			vmClient.KubevirtV1().VirtualMachineInstances(namespace).Create(context.Background(),
				vmi, metav1.CreateOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				time.Sleep(2 * time.Second)

				vmi.Status.Conditions = []v1.VirtualMachineInstanceCondition{
					{
						Type:   "AgentConnected",
						Status: k8sv1.ConditionTrue,
					},
				}
				vmClient.KubevirtV1().VirtualMachineInstances(namespace).Update(context.Background(),
					vmi, metav1.UpdateOptions{})
			}()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(state.Get("guest_agent")).To(BeTrue())
		})

		It("halts if the timeout is exceeded without the guest agent", func() {
			step.Config.AgentWaitTimeout = 2 * time.Second

			vmClient.KubevirtV1().VirtualMachineInstances(namespace).Create(context.Background(),
				vmi, metav1.CreateOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(errorBuffer.String()).To(ContainSubstring("wait timeout exceeded"))
		})
	})
})
