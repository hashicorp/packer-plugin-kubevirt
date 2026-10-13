// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"io"
	"strings"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
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
		namespace = "test-ns"
		name      = "test-vm"
	)

	var (
		ctrl       *gomock.Controller
		cdiClient  *fakecdiclient.Clientset
		virtClient kubecli.KubevirtClient
		vmClient   *kubevirtfake.Clientset
		state      *multistep.BasicStateBag
		step       *iso.StepCreateVirtualMachine
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		uiErr := &strings.Builder{}
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: uiErr,
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

		step = &iso.StepCreateVirtualMachine{
			Config: iso.Config{
				BuilderConfig: kubevirtcommon.BuilderConfig{
					Name:             name,
					Namespace:        namespace,
					DiskSize:         "1Gi",
					InstanceType:     "cx1.medium",
					InstanceTypeKind: "instancetype.kubevirt.io",
					Preference:       "fedora",
					PreferenceKind:   "instancetype.kubevirt.io",
				},
				IsoVolumeName:       "iso-vol",
				OperatingSystemType: "linux",
			},
			Client: virtClient,
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("Run", func() {
		It("halts when OS type is unsupported", func() {
			step.Config.OperatingSystemType = "bsd"
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		DescribeTable("sets the media volume label",
			func(mediaLabel, expectedLabel string) {
				step.Config.MediaLabel = mediaLabel

				var created *v1.VirtualMachine
				vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
					created = action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
					created.Status.Ready = true
					return false, created, nil
				})

				action := step.Run(context.Background(), state)
				Expect(action).To(Equal(multistep.ActionContinue))
				Expect(created).NotTo(BeNil())

				var mediaVolume *v1.Volume
				for i, vol := range created.Spec.Template.Spec.Volumes {
					if vol.ConfigMap != nil {
						mediaVolume = &created.Spec.Template.Spec.Volumes[i]
					}
				}
				Expect(mediaVolume).NotTo(BeNil())
				Expect(mediaVolume.ConfigMap.Name).To(Equal(name))
				Expect(mediaVolume.ConfigMap.VolumeLabel).To(Equal(expectedLabel))
			},
			Entry("defaults to OEMDRV when unset", "", "OEMDRV"),
			Entry("uses OEMDRV for kickstart", "OEMDRV", "OEMDRV"),
			Entry("uses cidata for cloud-init / Ubuntu autoinstall", "cidata", "cidata"),
		)

		DescribeTable("attaches the VirtIO drivers to Windows VMs",
			func(image, expectedImage string) {
				step.Config.OperatingSystemType = "windows"
				step.Config.VirtIOContainerImage = image

				var created *v1.VirtualMachine
				vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
					created = action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
					created.Status.Ready = true
					return false, created, nil
				})

				Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
				Expect(created).NotTo(BeNil())

				var images []string
				for _, vol := range created.Spec.Template.Spec.Volumes {
					if vol.ContainerDisk != nil {
						images = append(images, vol.ContainerDisk.Image)
					}
				}
				Expect(images).To(ConsistOf(expectedImage))
			},
			Entry("defaults to the upstream image", "", iso.DefaultVirtIOContainerImage),
			Entry("uses a configured image", "mirror.example.com/kubevirt/virtio-container-disk:v1.5.2", "mirror.example.com/kubevirt/virtio-container-disk:v1.5.2"),
		)

		DescribeTable("configures the root disk storage",
			func(storage kubevirtcommon.StorageConfig, storageClass *string, accessMode corev1.PersistentVolumeAccessMode, volumeMode *corev1.PersistentVolumeMode) {
				step.Config.StorageConfig = storage

				var created *v1.VirtualMachine
				vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
					created = action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
					created.Status.Ready = true
					return false, created, nil
				})

				Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
				Expect(created).NotTo(BeNil())
				Expect(created.Spec.DataVolumeTemplates).To(HaveLen(1))

				pvc := created.Spec.DataVolumeTemplates[0].Spec.PVC
				Expect(pvc.StorageClassName).To(Equal(storageClass))
				Expect(pvc.AccessModes).To(ConsistOf(accessMode))
				Expect(pvc.VolumeMode).To(Equal(volumeMode))
				Expect(pvc.Resources.Requests.Storage().String()).To(Equal("1Gi"))
			},
			Entry("defaults", kubevirtcommon.StorageConfig{}, nil, corev1.ReadWriteOnce, nil),
			Entry("configured",
				kubevirtcommon.StorageConfig{StorageClassName: "ceph-rbd-virtualization", AccessMode: "ReadWriteMany", VolumeMode: "Block"},
				ptr.To("ceph-rbd-virtualization"), corev1.ReadWriteMany, ptr.To(corev1.PersistentVolumeBlock)),
		)

		It("uses the pod network for networks without a source", func() {
			step.Config.Networks = []iso.Network{
				{Name: "default"},
				{Name: "net1", NetworkSource: iso.NetworkSource{Multus: &iso.MultusNetwork{NetworkName: "multus-01"}}},
			}

			var created *v1.VirtualMachine
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				created = action.(k8stesting.CreateAction).GetObject().(*v1.VirtualMachine)
				created.Status.Ready = true
				return false, created, nil
			})

			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			Expect(created).NotTo(BeNil())

			networks := created.Spec.Template.Spec.Networks
			interfaces := created.Spec.Template.Spec.Domain.Devices.Interfaces
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
