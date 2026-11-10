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
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	v1 "kubevirt.io/api/core/v1"
	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	kubevirtfake "kubevirt.io/client-go/kubevirt/fake"
)

var _ = Describe("StepCreateVirtualMachine", func() {
	const (
		namespace             = "test-ns"
		name                  = "test-vm"
		sourceVolumeName      = "source-dv"
		sourceVolumeNamespace = "source-ns"
	)

	var (
		ctrl       *gomock.Controller
		cdiClient  *fakecdiclient.Clientset
		virtClient kubecli.KubevirtClient
		vmClient   *kubevirtfake.Clientset
		state      *multistep.BasicStateBag
		step       *clone.StepCreateVirtualMachine
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: io.Discard,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)

		cdiClient = fakecdiclient.NewSimpleClientset()
		vmClient = kubevirtfake.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().
			VirtualMachine(gomock.Any()).
			DoAndReturn(func(ns string) kubecli.VirtualMachineInterface {
				return vmClient.KubevirtV1().VirtualMachines(ns)
			}).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()

		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		step = &clone.StepCreateVirtualMachine{
			Config: clone.Config{
				BuilderConfig: kubevirtcommon.BuilderConfig{
					Name:             name,
					Namespace:        namespace,
					DiskSize:         "10Gi",
					InstanceType:     "u1.medium",
					InstanceTypeKind: "instancetype.kubevirt.io",
					Preference:       "fedora",
					PreferenceKind:   "instancetype.kubevirt.io",
					MediaLabel:       "OEMDRV",
				},
				SourceVolumeName:      sourceVolumeName,
				SourceVolumeNamespace: sourceVolumeNamespace,
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	interceptCreate := func() **v1.VirtualMachine {
		var created *v1.VirtualMachine
		vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
			created = action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
			created.Status.Ready = true
			return false, created, nil
		})
		return &created
	}

	Context("Run", func() {
		It("clones from the source DataVolume via Source.PVC", func() {
			created := interceptCreate()
			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			Expect(*created).NotTo(BeNil())

			Expect((*created).Spec.DataVolumeTemplates).To(HaveLen(1))
			dvt := (*created).Spec.DataVolumeTemplates[0]
			Expect(dvt.Name).To(Equal(name + "-rootdisk"))
			Expect(dvt.Spec.Source.PVC).NotTo(BeNil())
			Expect(dvt.Spec.Source.PVC.Name).To(Equal(sourceVolumeName))
			Expect(dvt.Spec.Source.PVC.Namespace).To(Equal(sourceVolumeNamespace))
		})

		It("attaches the media ConfigMap when media files are present", func() {
			step.Config.MediaFiles = []string{"unattend.xml"}
			created := interceptCreate()
			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			Expect(*created).NotTo(BeNil())

			var mediaVolume *v1.Volume
			for i, vol := range (*created).Spec.Template.Spec.Volumes {
				if vol.ConfigMap != nil {
					mediaVolume = &(*created).Spec.Template.Spec.Volumes[i]
				}
			}
			Expect(mediaVolume).NotTo(BeNil())
			Expect(mediaVolume.ConfigMap.Name).To(Equal(name))
			Expect(mediaVolume.ConfigMap.VolumeLabel).To(Equal("OEMDRV"))
		})

		It("does not attach media when no media files or content exist", func() {
			step.Config.MediaFiles = nil
			step.Config.MediaContent = nil
			created := interceptCreate()
			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			Expect(*created).NotTo(BeNil())

			for _, vol := range (*created).Spec.Template.Spec.Volumes {
				Expect(vol.ConfigMap).To(BeNil(), "no ConfigMap volume should be attached without media")
			}
		})

		DescribeTable("configures the root disk storage",
			func(storage kubevirtcommon.StorageConfig, storageClass *string, accessMode corev1.PersistentVolumeAccessMode, volumeMode *corev1.PersistentVolumeMode) {
				step.Config.StorageConfig = storage
				created := interceptCreate()

				Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
				Expect(*created).NotTo(BeNil())
				Expect((*created).Spec.DataVolumeTemplates).To(HaveLen(1))

				pvc := (*created).Spec.DataVolumeTemplates[0].Spec.PVC
				Expect(pvc.StorageClassName).To(Equal(storageClass))
				Expect(pvc.AccessModes).To(ConsistOf(accessMode))
				Expect(pvc.VolumeMode).To(Equal(volumeMode))
			},
			Entry("defaults", kubevirtcommon.StorageConfig{}, nil, corev1.ReadWriteOnce, nil),
			Entry("configured",
				kubevirtcommon.StorageConfig{StorageClassName: "ceph-rbd", AccessMode: "ReadWriteMany", VolumeMode: "Block"},
				ptr.To("ceph-rbd"), corev1.ReadWriteMany, ptr.To(corev1.PersistentVolumeBlock)),
		)

		It("wires networks and interfaces", func() {
			step.Config.Networks = []clone.Network{
				{Name: "default"},
				{Name: "net1", NetworkSource: clone.NetworkSource{Multus: &clone.MultusNetwork{NetworkName: "multus-01"}}},
			}
			created := interceptCreate()

			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			Expect(*created).NotTo(BeNil())

			networks := (*created).Spec.Template.Spec.Networks
			interfaces := (*created).Spec.Template.Spec.Domain.Devices.Interfaces
			Expect(networks).To(HaveLen(2))
			Expect(interfaces).To(HaveLen(2))

			Expect(networks[0].Pod).NotTo(BeNil())
			Expect(interfaces[0].Masquerade).NotTo(BeNil())

			Expect(networks[1].Multus).NotTo(BeNil())
			Expect(networks[1].Multus.NetworkName).To(Equal("multus-01"))
			Expect(interfaces[1].Bridge).NotTo(BeNil())
		})
	})
})
