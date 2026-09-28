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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

var _ = Describe("StepPreValidate", func() {
	const (
		namespace = "test-ns"
		name      = "fedora-image"
	)

	var (
		ctrl       *gomock.Controller
		state      *multistep.BasicStateBag
		step       *iso.StepPreValidate
		kubeClient *fakek8sclient.Clientset
		cdiClient  *fakecdiclient.Clientset
	)

	meta := metav1.ObjectMeta{Name: name, Namespace: namespace}

	BeforeEach(func() {
		state = new(multistep.BasicStateBag)
		state.Put("ui", &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: io.Discard,
		})

		ctrl = gomock.NewController(GinkgoT())
		kubeClient = fakek8sclient.NewSimpleClientset()
		cdiClient = fakecdiclient.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()
		virtClient, _ := kubecli.GetKubevirtClientFromClientConfig(nil)

		step = &iso.StepPreValidate{
			Config: iso.Config{Name: name, Namespace: namespace},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	It("continues when the bootable volume does not exist", func() {
		Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
	})

	DescribeTable("halts when part of the bootable volume already exists",
		func(create func() error, kind string) {
			Expect(create()).To(Succeed())

			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring(
				fmt.Sprintf("a %s named %s/%s already exists", kind, namespace, name))))
		},
		Entry("DataVolume", func() error {
			_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(context.Background(), &cdiv1beta1.DataVolume{ObjectMeta: meta}, metav1.CreateOptions{})
			return err
		}, "DataVolume"),
		Entry("DataSource", func() error {
			_, err := cdiClient.CdiV1beta1().DataSources(namespace).Create(context.Background(), &cdiv1beta1.DataSource{ObjectMeta: meta}, metav1.CreateOptions{})
			return err
		}, "DataSource"),
		Entry("PersistentVolumeClaim", func() error {
			_, err := kubeClient.CoreV1().PersistentVolumeClaims(namespace).Create(context.Background(), &corev1.PersistentVolumeClaim{ObjectMeta: meta}, metav1.CreateOptions{})
			return err
		}, "PersistentVolumeClaim"),
	)

	It("halts when the check fails", func() {
		cdiClient.PrependReactor("get", "datasources", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, fmt.Errorf("simulated API error")
		})

		Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionHalt))
		Expect(state.Get("error")).To(MatchError(ContainSubstring("simulated API error")))
	})

	It("skips checks it is not allowed to perform", func() {
		kubeClient.PrependReactor("get", "persistentvolumeclaims", func(action k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewForbidden(corev1.Resource("persistentvolumeclaims"), name, fmt.Errorf("RBAC"))
		})

		Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
	})
})
