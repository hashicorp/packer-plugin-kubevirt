// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	k8stesting "k8s.io/client-go/testing"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
)

func testVM(name, namespace string) *v1.VirtualMachine {
	return &v1.VirtualMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

var _ = Describe("VirtualMachine Lifecycle", func() {
	const (
		namespace = "test-ns"
		name      = "test-vm"
	)

	var (
		ctrl     *gomock.Controller
		vmClient *kubevirtfake.Clientset
		client   kubecli.KubevirtClient
		state    *multistep.BasicStateBag
		config   common.BuilderConfig
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		vmClient = kubevirtfake.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().
			VirtualMachine(gomock.Any()).
			DoAndReturn(func(ns string) kubecli.VirtualMachineInterface {
				return vmClient.KubevirtV1().VirtualMachines(ns)
			}).AnyTimes()
		client, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		uiErr := &strings.Builder{}
		state = new(multistep.BasicStateBag)
		state.Put("ui", &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: uiErr,
		})

		config = common.BuilderConfig{
			Name:      name,
			Namespace: namespace,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("CreateVirtualMachine", func() {
		It("creates the VM and returns its UID when it becomes Ready", func() {
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				obj := action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
				obj.UID = "test-uid"
				obj.Status.Ready = true
				return false, obj, nil
			})

			uid, err := common.CreateVirtualMachine(context.Background(), state, client, testVM(name, namespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(uid).NotTo(BeEmpty())
		})

		It("returns an error when VM creation fails", func() {
			const errorMessage = "simulated create error"
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf(errorMessage)
			})

			uid, err := common.CreateVirtualMachine(context.Background(), state, client, testVM(name, namespace))
			Expect(err).To(MatchError(ContainSubstring(errorMessage)))
			Expect(uid).To(BeEmpty())
		})

		It("returns the UID when VM is created but never becomes Ready", func() {
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				obj := action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
				obj.UID = "test-uid"
				return false, obj, nil
			})

			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			uid, err := common.CreateVirtualMachine(ctx, state, client, testVM(name, namespace))
			Expect(err).To(MatchError(ContainSubstring("did not become ready")))
			Expect(uid).NotTo(BeEmpty())
		})
	})

	Context("CleanupVirtualMachine", func() {
		createReadyVM := func(ObjUid string) {
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				obj := action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
				obj.UID = types.UID(ObjUid)
				obj.Status.Ready = true
				return false, obj, nil
			})
			uid, err := common.CreateVirtualMachine(context.Background(), state, client, testVM(name, namespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(uid).NotTo(BeEmpty())
		}

		It("keeps VM when KeepVM is true", func() {
			VMUid := "test-uid"
			createReadyVM(VMUid)
			config.KeepVM = true

			common.CleanupVirtualMachine(state, client, config, true, types.UID(VMUid))

			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
		})

		It("deletes VM when KeepVM is false", func() {
			VMUid := "test-uid"
			createReadyVM(VMUid)

			common.CleanupVirtualMachine(state, client, config, true, types.UID(VMUid))

			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("only deletes the VM instance it created", func() {
			VMUid := "created-uid"
			createReadyVM(VMUid)

			var deleteOptions metav1.DeleteOptions
			vmClient.Fake.PrependReactor("delete", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				deleteOptions = action.(k8stesting.DeleteAction).GetDeleteOptions()
				return false, nil, nil
			})

			uid, err := common.CreateVirtualMachine(context.Background(), state, client, testVM("secondary-vm", namespace))
			Expect(err).NotTo(HaveOccurred())

			common.CleanupVirtualMachine(state, client, config, true, uid)

			Expect(deleteOptions.Preconditions).NotTo(BeNil())
			Expect(deleteOptions.Preconditions.UID).To(HaveValue(BeEquivalentTo(VMUid)))
			Expect(deleteOptions.GracePeriodSeconds).To(HaveValue(BeZero()))
		})

		It("does not attempt deletion when created is false", func() {
			deleted := false
			vmClient.Fake.PrependReactor("delete", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				deleted = true
				return false, nil, nil
			})

			common.CleanupVirtualMachine(state, client, config, false, "")

			Expect(deleted).To(BeFalse())
		})
	})
})
