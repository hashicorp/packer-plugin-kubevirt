// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone_test

import (
	"context"
	"io"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/clone"
	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakek8s "k8s.io/client-go/kubernetes/fake"

	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

var _ = Describe("StepValidateVolumeSource", func() {
	const (
		sourceNamespace = "source-ns"
		sourceName      = "source-dv"
		pvcName         = "source-dv"
	)

	var (
		ctrl       *gomock.Controller
		state      *multistep.BasicStateBag
		step       *clone.StepValidateVolumeSource
		cdiClient  *fakecdiclient.Clientset
		k8sClient  *fakek8s.Clientset
		virtClient kubecli.KubevirtClient
	)

	BeforeEach(func() {
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: io.Discard,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		ctrl = gomock.NewController(GinkgoT())
		cdiClient = fakecdiclient.NewSimpleClientset()
		k8sClient = fakek8s.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().CoreV1().Return(k8sClient.CoreV1()).AnyTimes()
		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		step = &clone.StepValidateVolumeSource{
			Config: clone.Config{
				BuilderConfig: kubevirtcommon.BuilderConfig{
					Namespace: "target-ns",
					DiskSize:  "20Gi",
				},
				SourceVolumeName:      sourceName,
				SourceVolumeNamespace: sourceNamespace,
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	createSourceDV := func(phase cdiv1beta1.DataVolumePhase, claimName string) {
		_, err := cdiClient.CdiV1beta1().DataVolumes(sourceNamespace).Create(context.Background(), &cdiv1beta1.DataVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name:      sourceName,
				Namespace: sourceNamespace,
			},
			Status: cdiv1beta1.DataVolumeStatus{
				Phase:     phase,
				ClaimName: claimName,
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	createSourcePVC := func(size string) {
		_, err := k8sClient.CoreV1().PersistentVolumeClaims(sourceNamespace).Create(context.Background(), &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: sourceNamespace,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(size),
					},
				},
			},
		}, metav1.CreateOptions{})
		Expect(err).NotTo(HaveOccurred())
	}

	Context("Run", func() {
		It("continues when source is valid and disk_size is sufficient", func() {
			createSourceDV(cdiv1beta1.Succeeded, pvcName)
			createSourcePVC("10Gi")

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		It("halts when the source DataVolume does not exist", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("failed to get the source DataVolume")))
		})

		It("halts when the source DataVolume is not ready", func() {
			createSourceDV(cdiv1beta1.Pending, pvcName)

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("is not ready")))
		})

		It("halts when disk_size is smaller than the source", func() {
			createSourceDV(cdiv1beta1.Succeeded, pvcName)
			createSourcePVC("30Gi")

			step.Config.DiskSize = "10Gi"

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("disk_size \"10Gi\" is smaller than the source volume size")))
		})

		It("continues when disk_size equals the source", func() {
			createSourceDV(cdiv1beta1.Succeeded, pvcName)
			createSourcePVC("20Gi")

			step.Config.DiskSize = "20Gi"

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})
	})
})
