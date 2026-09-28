// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	v1 "kubevirt.io/api/core/v1"
	kubecli "kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
)

var _ = Describe("StepStopVirtualMachine", func() {
	const (
		namespace = "test-ns"
		name      = "test-vm"
	)

	var (
		state      *multistep.BasicStateBag
		step       *iso.StepStopVirtualMachine
		vmClient   *kubevirtfake.Clientset
		virtClient kubecli.KubevirtClient
		mockCtrl   *gomock.Controller
		mockVirt   *kubecli.MockKubevirtClient
	)

	BeforeEach(func() {
		uiErr := &strings.Builder{}
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

		step = &iso.StepStopVirtualMachine{
			Config: iso.Config{
				Name:      name,
				Namespace: namespace,
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		mockCtrl.Finish()
	})

	Context("Run", func() {
		It("continues when VM is retrieved and updated successfully", func() {
			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Create(context.Background(),
				&v1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		It("retries when the update conflicts with a concurrent change", func() {
			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Create(context.Background(),
				&v1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			conflicts := 0
			vmClient.Fake.PrependReactor("update", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				if conflicts < 2 {
					conflicts++
					return true, nil, apierrors.NewConflict(v1.Resource("virtualmachines"), name, fmt.Errorf("the object has been modified"))
				}
				return false, nil, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(conflicts).To(Equal(2))

			vm, err := vmClient.KubevirtV1().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(vm.Spec.RunStrategy).To(HaveValue(Equal(v1.RunStrategyHalted)))
		})

		It("halts when VM cannot be retrieved", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("failed to get the VirtualMachine")))
		})

		It("halts when VM update fails", func() {
			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Create(context.Background(),
				&v1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Inject update error
			vmClient.Fake.PrependReactor("update", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("simulated update error")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("simulated update error")))
		})
	})
})
