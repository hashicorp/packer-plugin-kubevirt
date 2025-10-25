// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
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
		logBuffer   *strings.Builder
		ctrl        *gomock.Controller
		state       *multistep.BasicStateBag
		step        *iso.StepWaitForIp
		virtClient  kubecli.KubevirtClient
		vmClient    *kubevirtfake.Clientset
		mockVirt    *kubecli.MockKubevirtClient
		vmi         *v1.VirtualMachineInstance
	)

	BeforeEach(func() {
		logBuffer = &strings.Builder{}
		log.SetOutput(logBuffer)

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
				Interfaces: []v1.VirtualMachineInstanceNetworkInterface{},
			},
		}

		step = &iso.StepWaitForIp{
			Config: iso.Config{
				VMName:       name,
				Namespace:    namespace,
				WaitIpConfig: iso.WaitIpConfig{},
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
		It("waits for the IP to become available and continues", func() {
			step.Config.WaitIpConfig.WaitTimeout = 3 * time.Second

			vmi.Status.Interfaces = []v1.VirtualMachineInstanceNetworkInterface{
				{IP: "1.2.3.4"},
			}

			vmClient.KubevirtV1().VirtualMachineInstances(namespace).Create(context.Background(),
				vmi, metav1.CreateOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(state.Get("ip")).To(Equal("1.2.3.4"))
		})

		It("continues immediately when no wait time specified", func() {
			step.Config.WaitIpConfig.WaitTimeout = 0 * time.Second

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(state.GetOk("ip")).To(BeNil())
		})

		It("halts when context is cancelled before wait time elapses", func() {
			step.Config.WaitIpConfig.WaitTimeout = 4 * time.Second

			vmClient.KubevirtV1().VirtualMachineInstances(namespace).Create(context.Background(),
				vmi, metav1.CreateOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.After(1 * time.Second)
				cancel()
			}()

			action := step.Run(ctx, state)
			Expect(logBuffer.String()).To(ContainSubstring("Interrupt detected"))
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("halts with an error when the VMI is not found", func() {
			step.Config.WaitIpConfig.WaitTimeout = 4 * time.Second

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			action := step.Run(ctx, state)
			Expect(errorBuffer.String()).To(ContainSubstring("error getting VMI instance"))
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("waits for the IP to settle", func() {
			step.Config.WaitIpConfig.WaitTimeout = 20 * time.Second
			step.Config.WaitIpConfig.SettleTimeout = 5 * time.Second

			vmClient.KubevirtV1().VirtualMachineInstances(namespace).Create(context.Background(),
				vmi, metav1.CreateOptions{})

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			go func() {
				time.Sleep(2 * time.Second)

				vmi.Status.Interfaces = []v1.VirtualMachineInstanceNetworkInterface{
					{IP: "1.2.3.4"},
				}
				vmClient.KubevirtV1().VirtualMachineInstances(namespace).Update(context.Background(),
					vmi, metav1.UpdateOptions{})
				time.Sleep(3 * time.Second)

				vmi.Status.Interfaces = []v1.VirtualMachineInstanceNetworkInterface{
					{IP: "8.8.8.8"},
				}
				vmClient.KubevirtV1().VirtualMachineInstances(namespace).Update(context.Background(),
					vmi, metav1.UpdateOptions{})
			}()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(state.Get("ip")).To(Equal("8.8.8.8"))
		})
	})
})
