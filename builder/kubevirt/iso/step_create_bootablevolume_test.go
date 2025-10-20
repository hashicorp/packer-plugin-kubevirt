// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"io"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/golang/mock/gomock"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/testing"
)

var _ = Describe("StepCreateBootableVolume", func() {
	const (
		namespace = "test-ns"
		name      = "boot-dv"
		vmname    = "test-vm"
	)

	var (
		ctrl       *gomock.Controller
		state      *multistep.BasicStateBag
		step       *iso.StepCreateBootableVolume
		cdiClient  *fakecdiclient.Clientset
		virtClient kubecli.KubevirtClient
	)

	JustBeforeEach(func() {
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
		step.Client = virtClient
	})

	BeforeEach(func() {
		step = &iso.StepCreateBootableVolume{
			Config: iso.Config{
				Name:         name,
				VMName:       vmname,
				Namespace:    namespace,
				DiskSize:     "10Gi",
				InstanceType: "cx1.large",
				Preference:   "fedora",
			},
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("Run", func() {
		When("DataVolume and DataSource creates successfully", func() {
			var action multistep.StepAction
			var dataVolume *cdiv1beta1.DataVolume
			var dataSource *cdiv1beta1.DataSource
			var ctx context.Context
			var err error

			JustBeforeEach(func() {
				ctx = context.Background()

				cdiClient.Fake.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
					create := action.(testing.CreateAction)
					dv := create.GetObject().(*cdiv1beta1.DataVolume)
					dv.Status.Phase = cdiv1beta1.Succeeded

					return false, dv, nil
				})

				cdiClient.Fake.PrependReactor("create", "datasources", func(action testing.Action) (bool, runtime.Object, error) {
					create := action.(testing.CreateAction)
					ds := create.GetObject().(*cdiv1beta1.DataSource)

					return false, ds, nil
				})

				action = step.Run(context.Background(), state)
				Expect(action).To(Equal(multistep.ActionContinue))

				dataVolume, err = cdiClient.CdiV1beta1().DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo((HaveOccurred()))
				dataSource, err = cdiClient.CdiV1beta1().DataSources(namespace).Get(ctx, name, metav1.GetOptions{})
				Expect(err).NotTo((HaveOccurred()))
			})

			It("continues when DataVolume and DataSource are created successfully", func() {
				Expect(action).To(Equal(multistep.ActionContinue))
			})

			It("sets the statebag bootable_volume_name key to the correct value", func() {
				Expect(state.Get("bootable_volume_name")).To(Equal("boot-dv"))
			})

			It("creates the DataVolume with the correct name", func() {
				Expect(dataVolume.ObjectMeta.Name).To(Equal(name))
			})

			It("uses the correct source PVC for the DataVolume", func() {
				Expect(dataVolume.Spec.Source.PVC.Name).To(Equal(vmname + "-rootdisk"))
				Expect(dataVolume.Spec.Source.PVC.Namespace).To(Equal(namespace))
			})

			It("configures the correct size for the DataVolume", func() {
				Expect(dataVolume.Spec.PVC.Resources.Requests.Storage().Value()).To(Equal(int64(10 * (1 << 30))))
			})

			It("uses the correct name for the DataSource", func() {
				Expect(dataSource.ObjectMeta.Name).To(Equal(name))
			})

			It("labels the DataSource default-instancetype correctly", func() {
				Expect(dataSource.ObjectMeta.Labels["instancetype.kubevirt.io/default-instancetype"]).To(Equal("cx1.large"))
			})

			It("labels the DataSource default-preference correctly", func() {
				Expect(dataSource.ObjectMeta.Labels["instancetype.kubevirt.io/default-preference"]).To(Equal("fedora"))
			})

			It("configures the DataSource source DataVolume correctly", func() {
				Expect(dataSource.Spec.Source.PVC.Name).To(Equal(name))
				Expect(dataSource.Spec.Source.PVC.Namespace).To(Equal(namespace))
			})

			When("the DataVolume uses the default access mode", func() {
				It("configures ReadWriteOnce", func() {
					Expect(len(dataVolume.Spec.PVC.AccessModes)).To(Equal(1))
					Expect(dataVolume.Spec.PVC.AccessModes[0]).To(Equal(corev1.ReadWriteOnce))
				})
			})

			When("the DataVolume is configured to use ReadWriteOnce", func() {
				BeforeEach(func() {
					step.Config.AccessMode = "ReadWriteOnce"
				})

				It("configures ReadWriteOnce", func() {
					Expect(len(dataVolume.Spec.PVC.AccessModes)).To(Equal(1))
					Expect(dataVolume.Spec.PVC.AccessModes[0]).To(Equal(corev1.ReadWriteOnce))
				})
			})

			When("the DataVolume is configured to use ReadWriteMany", func() {
				BeforeEach(func() {
					step.Config.AccessMode = "ReadWriteMany"
				})

				It("configures ReadWriteMany", func() {
					Expect(len(dataVolume.Spec.PVC.AccessModes)).To(Equal(1))
					Expect(dataVolume.Spec.PVC.AccessModes[0]).To(Equal(corev1.ReadWriteMany))
				})
			})

			When("the DataVolume uses the default volume mode", func() {
				It("sets the volume mode as 'Filesystem'", func() {
					Expect(dataVolume.Spec.PVC.VolumeMode).To(HaveValue(Equal(corev1.PersistentVolumeFilesystem)))
				})
			})

			When("the configuration specifies volume_mode=Filesystem", func() {
				BeforeEach(func() {
					step.Config.VolumeMode = "Filesystem"
				})

				It("sets the volume mode as 'Filesystem'", func() {
					Expect(dataVolume.Spec.PVC.VolumeMode).To(HaveValue(Equal(corev1.PersistentVolumeFilesystem)))
				})
			})

			When("the configuration specifies volume_mode=Block", func() {
				BeforeEach(func() {
					step.Config.VolumeMode = "Block"
				})

				It("sets the volume mode as 'Block'", func() {
					Expect(dataVolume.Spec.PVC.VolumeMode).To(HaveValue(Equal(corev1.PersistentVolumeBlock)))
				})
			})
		})

		When("DataVolume creation fails", func() {
			var action multistep.StepAction

			JustBeforeEach(func() {
				cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
					return true, nil, fmt.Errorf("boom: DV create failed")
				})
				action = step.Run(context.Background(), state)
			})

			It("halts when DataVolume creation fails", func() {
				Expect(action).To(Equal(multistep.ActionHalt))
			})
		})

		When("the DataVolume does not succeed", func() {
			var action multistep.StepAction

			JustBeforeEach(func() {
				var ctx context.Context

				cdiClient.Fake.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
					create := action.(testing.CreateAction)
					dv := create.GetObject().(*cdiv1beta1.DataVolume)
					dv.Status.Phase = cdiv1beta1.Pending

					return false, dv, nil
				})

				// Cancel context so wait ends
				ctx, cancel := context.WithCancel(context.Background())
				cancel()

				action = step.Run(ctx, state)
			})

			It("halts", func() {
				Expect(action).To(Equal(multistep.ActionHalt))
			})
		})

		When("DataSource creation fails", func() {
			var action multistep.StepAction

			JustBeforeEach(func() {
				cdiClient.Fake.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
					create := action.(testing.CreateAction)
					dv := create.GetObject().(*cdiv1beta1.DataVolume)
					dv.Status.Phase = cdiv1beta1.Succeeded

					return false, dv, nil
				})

				cdiClient.PrependReactor("create", "datasources", func(action testing.Action) (bool, runtime.Object, error) {
					return true, nil, fmt.Errorf("boom: DS create failed")
				})
				action = step.Run(context.Background(), state)
			})

			It("halts", func() {
				Expect(action).To(Equal(multistep.ActionHalt))
			})
		})
	})
})
