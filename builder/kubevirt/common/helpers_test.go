// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common_test

import (
	"context"

	"github.com/golang/mock/gomock"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

var _ = Describe("WaitUntilDataVolumeSucceeded", func() {
	const (
		namespace = "test-ns"
		name      = "test-dv"
	)

	var (
		ctrl       *gomock.Controller
		virtClient kubecli.KubevirtClient
		cdiClient  *fakecdiclient.Clientset
		ctx        context.Context
		cancel     context.CancelFunc
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		cdiClient = fakecdiclient.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()

		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)
		ctx, cancel = context.WithCancel(context.Background())
	})

	AfterEach(func() {
		cancel()
		ctrl.Finish()
	})

	It("returns nil when DataVolume reaches Succeeded phase", func() {
		_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(ctx, &cdiv1beta1.DataVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: cdiv1beta1.DataVolumeStatus{
				Phase: cdiv1beta1.Succeeded,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		err = common.WaitUntilDataVolumeSucceeded(ctx, virtClient, namespace, name)
		Expect(err).NotTo(HaveOccurred())
	})

	It("returns error when DataVolume Get fails", func() {
		err := common.WaitUntilDataVolumeSucceeded(ctx, virtClient, namespace, "nonexistent-dv")
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not found"))
	})

	It("returns error when context is cancelled before success", func() {
		_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(ctx, &cdiv1beta1.DataVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Status: cdiv1beta1.DataVolumeStatus{
				Phase: cdiv1beta1.Pending,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())

		// Cancel quickly so poller doesn't succeed
		cancel()

		err = common.WaitUntilDataVolumeSucceeded(ctx, virtClient, namespace, name)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("context canceled"))
	})
})
