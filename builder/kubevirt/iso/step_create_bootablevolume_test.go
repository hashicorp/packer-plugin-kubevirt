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
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"

	fakecdiclient "kubevirt.io/client-go/containerizeddataimporter/fake"
	"kubevirt.io/client-go/kubecli"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/testing"
)

var _ = Describe("StepCreateBootableVolume", func() {
	const (
		namespace = "test-ns"
		name      = "boot-dv"
	)

	var (
		ctrl       *gomock.Controller
		state      *multistep.BasicStateBag
		step       *iso.StepCreateBootableVolume
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

		step = &iso.StepCreateBootableVolume{
			Config: iso.Config{
				Name:         name,
				Namespace:    namespace,
				DiskSize:     "10Gi",
				InstanceType: "cx1.large",
				Preference:   "fedora",
			},
			Client:        virtClient,
			GeneratedData: &packerbuilderdata.GeneratedData{State: state},
		}
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Context("Run", func() {
		It("continues when DataVolume and DataSource are created successfully", func() {
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				dv := create.GetObject().(*cdiv1beta1.DataVolume)
				Expect(dv.Annotations).To(HaveKeyWithValue(
					"cdi.kubevirt.io/storage.bind.immediate.requested",
					"true",
				))
				dv.Status.Phase = cdiv1beta1.Succeeded

				// Important: store DV in the fake client's tracker
				_ = cdiClient.Tracker().Add(dv)

				return true, dv, nil
			})

			cdiClient.PrependReactor("create", "datasources", func(action testing.Action) (bool, runtime.Object, error) {
				create := action.(testing.CreateAction)
				ds := create.GetObject().(*cdiv1beta1.DataSource)

				// Also store DS in the fake client so state.Put sees it
				_ = cdiClient.Tracker().Add(ds)

				return true, ds, nil
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(state.Get("bootable_volume_name")).To(Equal("boot-dv"))
		})

		DescribeTable("labels the DataSource with the instance type and preference",
			func(instanceTypeKind, preferenceKind string, expectedLabels map[string]string, unexpectedLabels []string) {
				step.Config.InstanceTypeKind = instanceTypeKind
				step.Config.PreferenceKind = preferenceKind

				cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
					dv := action.(testing.CreateAction).GetObject().(*cdiv1beta1.DataVolume)
					dv.Status.Phase = cdiv1beta1.Succeeded
					return false, dv, nil
				})
				var created *cdiv1beta1.DataSource
				cdiClient.PrependReactor("create", "datasources", func(action testing.Action) (bool, runtime.Object, error) {
					created = action.(testing.CreateAction).GetObject().(*cdiv1beta1.DataSource)
					return false, created, nil
				})

				Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
				Expect(created).NotTo(BeNil())
				for key, value := range expectedLabels {
					Expect(created.Labels).To(HaveKeyWithValue(key, value))
				}
				for _, key := range unexpectedLabels {
					Expect(created.Labels).NotTo(HaveKey(key))
				}
			},
			Entry("cluster-wide kinds rely on the KubeVirt default",
				"virtualmachineclusterinstancetype", "virtualmachineclusterpreference",
				map[string]string{
					"instancetype.kubevirt.io/default-instancetype": "cx1.large",
					"instancetype.kubevirt.io/default-preference":   "fedora",
				},
				[]string{"instancetype.kubevirt.io/default-instancetype-kind", "instancetype.kubevirt.io/default-preference-kind"},
			),
			Entry("namespaced kinds are recorded",
				"virtualmachineinstancetype", "virtualmachinepreference",
				map[string]string{
					"instancetype.kubevirt.io/default-instancetype":      "cx1.large",
					"instancetype.kubevirt.io/default-instancetype-kind": "virtualmachineinstancetype",
					"instancetype.kubevirt.io/default-preference":        "fedora",
					"instancetype.kubevirt.io/default-preference-kind":   "virtualmachinepreference",
				},
				nil,
			),
		)

		It("creates the bootable volume with the configured storage", func() {
			step.Config.StorageConfig = iso.StorageConfig{StorageClassName: "ceph-rbd-virtualization", AccessMode: "ReadWriteMany", VolumeMode: "Block"}

			var created *cdiv1beta1.DataVolume
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				created = action.(testing.CreateAction).GetObject().(*cdiv1beta1.DataVolume)
				created.Status.Phase = cdiv1beta1.Succeeded
				return false, created, nil
			})

			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			Expect(created).NotTo(BeNil())
			Expect(created.Spec.Source.PVC.Name).To(Equal(name + "-rootdisk"))
			Expect(created.Spec.PVC.StorageClassName).To(HaveValue(Equal("ceph-rbd-virtualization")))
			Expect(created.Spec.PVC.AccessModes).To(ConsistOf(corev1.ReadWriteMany))
			Expect(created.Spec.PVC.VolumeMode).To(HaveValue(Equal(corev1.PersistentVolumeBlock)))
		})

		It("halts when DataVolume creation fails", func() {
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("boom: DV create failed")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("boom: DV create failed")))
		})

		It("halts when DataVolume does not succeed", func() {
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				dv := action.(testing.CreateAction).GetObject().(*cdiv1beta1.DataVolume)
				dv.Status.Phase = cdiv1beta1.Pending
				return false, dv, nil
			})

			// Cancel context so wait ends
			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("did not succeed")))
		})

		It("halts when DataSource creation fails", func() {
			cdiClient.PrependReactor("create", "datavolumes", func(action testing.Action) (bool, runtime.Object, error) {
				dv := action.(testing.CreateAction).GetObject().(*cdiv1beta1.DataVolume)
				dv.Status.Phase = cdiv1beta1.Succeeded
				return false, dv, nil
			})
			cdiClient.PrependReactor("create", "datasources", func(action testing.Action) (bool, runtime.Object, error) {
				return true, nil, fmt.Errorf("boom: DS create failed")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("boom: DS create failed")))
		})
	})
})
