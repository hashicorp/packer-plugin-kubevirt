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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
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
		kubeClient *fakek8sclient.Clientset
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

		kubeClient = fakek8sclient.NewSimpleClientset()
		cdiClient = fakecdiclient.NewSimpleClientset()
		vmClient = kubevirtfake.NewSimpleClientset()

		kubecli.GetKubevirtClientFromClientConfig = kubecli.GetMockKubevirtClientFromClientConfig
		kubecli.MockKubevirtClientInstance = kubecli.NewMockKubevirtClient(ctrl)
		kubecli.MockKubevirtClientInstance.EXPECT().CoreV1().Return(kubeClient.CoreV1()).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().
			VirtualMachine(gomock.Any()).
			DoAndReturn(func(ns string) kubecli.VirtualMachineInterface {
				return vmClient.KubevirtV1().VirtualMachines(ns)
			}).AnyTimes()
		kubecli.MockKubevirtClientInstance.EXPECT().CdiClient().Return(cdiClient).AnyTimes()

		virtClient, _ = kubecli.GetKubevirtClientFromClientConfig(nil)

		step = &iso.StepCreateVirtualMachine{
			Config: iso.Config{
				VMName:              name,
				Namespace:           namespace,
				IsoVolumeName:       "iso-vol",
				DiskSize:            "1Gi",
				InstanceType:        "cx1.medium",
				InstanceTypeKind:    "instancetype.kubevirt.io",
				Preference:          "fedora",
				PreferenceKind:      "instancetype.kubevirt.io",
				OperatingSystemType: "linux",
				KeepVM:              false,
				Media: iso.MediaConfig{
					Label: "OEMDRV",
				},
				VirtIOContainer: "registry.example.com/virtio",
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

		It("continues when VM is created and becomes Ready", func() {
			// Let Run create the VM, then mark it Ready
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			// Watch for VM creation and patch Ready status
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				create := action.(k8stesting.CreateAction)
				obj := create.GetObject().(*v1.VirtualMachine)
				// Simulate that VM is created and becomes Ready
				obj.Status.Ready = true
				return false, obj, nil
			})

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionContinue))
		})

		When("creating a Linux VM", func() {
			var (
				ctx context.Context
				vm  *v1.VirtualMachine
			)

			BeforeEach(func() {
				var cancel func()
				var err error

				// Let Run create the VM, then mark it Ready
				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				// Watch for VM creation and patch Ready status
				vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
					create := action.(k8stesting.CreateAction)
					obj := create.GetObject().(*v1.VirtualMachine)
					// Simulate that VM is created and becomes Ready
					obj.Status.Ready = true
					return false, obj, nil
				})

				action := step.Run(ctx, state)
				Expect(action).To(Equal(multistep.ActionContinue))
				vm, err = vmClient.KubevirtV1().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("sets the instance type correctly", func() {
				Expect(vm.Spec.Instancetype.Kind).To(Equal("instancetype.kubevirt.io"))
				Expect(vm.Spec.Instancetype.Name).To(Equal("cx1.medium"))
			})

			It("sets the preference correctly", func() {
				Expect(vm.Spec.Preference.Kind).To(Equal("instancetype.kubevirt.io"))
				Expect(vm.Spec.Preference.Name).To(Equal("fedora"))
			})

			It("sets the DataVolumeTemplate root disk name correctly", func() {
				Expect(vm.Spec.DataVolumeTemplates[0].ObjectMeta.Name).To(Equal(name + "-rootdisk"))
				Expect(vm.Spec.DataVolumeTemplates[0].Spec.PVC.Resources.Requests.Storage().Value()).To(Equal(int64(1 << 30)))
				Expect(vm.Spec.DataVolumeTemplates[0].Spec.PVC.AccessModes[0]).To(Equal(corev1.ReadWriteOnce))
			})

			It("configures no networks when none are specified", func() {
				Expect(vm.Spec.Template.Spec.Networks).To(BeEmpty())
				Expect(vm.Spec.Template.Spec.Domain.Devices.Interfaces).To(BeEmpty())
			})

			It("configures the rootdisk device correctly", func() {
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[0].Name).To(Equal("rootdisk"))
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[0].DiskDevice.Disk).To(Equal(&v1.DiskTarget{}))
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[0].BootOrder).To(HaveValue(Equal(uint(1))))
			})

			It("configures the install ISO device correctly", func() {
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[1].Name).To(Equal("cdrom"))
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[1].DiskDevice.CDRom).To(
					Equal(&v1.CDRomTarget{Tray: "closed", Bus: "sata"}),
				)
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[1].BootOrder).To(HaveValue(Equal(uint(2))))
			})

			It("configures the userdata device correctly", func() {
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[2].Name).To(Equal("userdata"))
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[2].DiskDevice.CDRom).To(
					Equal(&v1.CDRomTarget{Tray: "closed", Bus: "sata"}),
				)
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[2].BootOrder).To(BeNil())
			})

			It("does not configure the virtio container disk device", func() {
				Expect(len(vm.Spec.Template.Spec.Domain.Devices.Disks)).To(Equal(3))
			})

			It("configures the rootdisk volume correctly", func() {
				Expect(vm.Spec.Template.Spec.Volumes[0].Name).To(Equal("rootdisk"))
				Expect(vm.Spec.Template.Spec.Volumes[0].VolumeSource.DataVolume.Name).To(Equal(name + "-rootdisk"))
			})

			It("configures the ISO install volume correctly", func() {
				Expect(vm.Spec.Template.Spec.Volumes[1].Name).To(Equal("cdrom"))
				Expect(vm.Spec.Template.Spec.Volumes[1].VolumeSource.DataVolume.Name).To(Equal("iso-vol"))
			})

			It("configures the userdata volume correctly", func() {
				Expect(vm.Spec.Template.Spec.Volumes[2].Name).To(Equal("userdata"))
				Expect(vm.Spec.Template.Spec.Volumes[2].VolumeSource.ConfigMap.LocalObjectReference.Name).To(Equal(name))
				Expect(vm.Spec.Template.Spec.Volumes[2].VolumeSource.ConfigMap.VolumeLabel).To(Equal("OEMDRV"))
			})
		})

		When("creating a Windows VM", func() {
			var (
				ctx context.Context
				vm  *v1.VirtualMachine
			)

			BeforeEach(func() {
				var cancel func()
				var err error

				// Let Run create the VM, then mark it Ready
				ctx, cancel = context.WithCancel(context.Background())
				defer cancel()

				// Watch for VM creation and patch Ready status
				vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
					create := action.(k8stesting.CreateAction)
					obj := create.GetObject().(*v1.VirtualMachine)
					// Simulate that VM is created and becomes Ready
					obj.Status.Ready = true
					return false, obj, nil
				})

				step.Config.OperatingSystemType = "windows"

				action := step.Run(ctx, state)
				Expect(action).To(Equal(multistep.ActionContinue))
				vm, err = vmClient.KubevirtV1().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
				Expect(err).NotTo(HaveOccurred())
			})

			It("configures the virtio container disk correctly", func() {
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[2].Name).To(Equal("virtiocontainerdisk"))
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[2].DiskDevice.CDRom).To(
					Equal(&v1.CDRomTarget{Tray: "closed", Bus: "sata"}),
				)
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[2].BootOrder).To(BeNil())
			})

			It("configures the userdata/sysprep device correctly", func() {
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[3].Name).To(Equal("userdata"))
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[3].DiskDevice.CDRom).To(
					Equal(&v1.CDRomTarget{Tray: "closed", Bus: "sata"}),
				)
				Expect(vm.Spec.Template.Spec.Domain.Devices.Disks[3].BootOrder).To(BeNil())
			})

			It("configures the sysprep volume correctly", func() {
				Expect(vm.Spec.Template.Spec.Volumes[2].Name).To(Equal("userdata"))
				Expect(vm.Spec.Template.Spec.Volumes[2].VolumeSource.Sysprep.ConfigMap.Name).To(Equal(name))
			})

			It("configures the virtio volume correctly", func() {
				Expect(vm.Spec.Template.Spec.Volumes[3].Name).To(Equal("virtiocontainerdisk"))
				Expect(vm.Spec.Template.Spec.Volumes[3].VolumeSource.ContainerDisk.Image).To(Equal("registry.example.com/virtio"))
			})
		})

		It("halts when VM creation fails", func() {
			// Inject error into fake client
			vmClient.Fake.PrependReactor("create", "virtualmachines", func(action k8stesting.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("simulated create error")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("halts when VM never becomes Ready", func() {
			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Create(context.Background(),
				&v1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
					Status: v1.VirtualMachineStatus{Ready: false},
				},
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})
	})

	Context("Cleanup", func() {
		It("keeps VM when KeepVM is true", func() {
			step.Config.KeepVM = true
			step.Cleanup(state) // should not panic
		})

		It("deletes VM when KeepVM is false", func() {
			_, err := vmClient.KubevirtV1().VirtualMachines(namespace).Create(context.Background(),
				&v1.VirtualMachine{
					ObjectMeta: metav1.ObjectMeta{
						Name:      name,
						Namespace: namespace,
					},
				},
				metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			step.Cleanup(state)

			_, err = vmClient.KubevirtV1().VirtualMachines(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).To(HaveOccurred()) // deleted
		})
	})
})
