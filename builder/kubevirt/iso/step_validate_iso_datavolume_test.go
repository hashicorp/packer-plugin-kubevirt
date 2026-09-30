// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	"github.com/golang/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

var _ = Describe("StepValidateIsoDataVolume", func() {
	const (
		namespace = "test-ns"
		isoName   = "test-iso-dv"
	)

	var (
		ctrl       *gomock.Controller
		state      *multistep.BasicStateBag
		step       *iso.StepValidateIsoDataVolume
		cdiClient  *fakecdiclient.Clientset
		virtClient kubecli.KubevirtClient
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

		ctrl = gomock.NewController(GinkgoT())
		cdiClient = fakecdiclient.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()
		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		step = &iso.StepValidateIsoDataVolume{
			Config: iso.Config{
				Namespace:     namespace,
				IsoVolumeName: isoName,
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("Run", func() {
		It("continues when DataVolume exists and succeeds", func() {
			_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(context.Background(), &cdiv1beta1.DataVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isoName,
					Namespace: namespace,
				},
				Status: cdiv1beta1.DataVolumeStatus{Phase: cdiv1beta1.Succeeded},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		It("halts when DataVolume not found", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("failed to get the ISO DataVolume")))
		})

		It("halts when DataVolume never succeeds", func() {
			_, err := cdiClient.CdiV1beta1().DataVolumes(namespace).Create(context.Background(), &cdiv1beta1.DataVolume{
				ObjectMeta: metav1.ObjectMeta{
					Name:      isoName,
					Namespace: namespace,
				},
				Status: cdiv1beta1.DataVolumeStatus{Phase: cdiv1beta1.Pending},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Cancel context early to simulate stuck Pending
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("is not ready")))
		})
	})
})
