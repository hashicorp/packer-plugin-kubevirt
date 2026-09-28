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
