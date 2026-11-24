// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

var _ = Describe("StepWaitForInstallation", func() {
	const (
		namespace = "test-ns"
		name      = "test-vm"
	)

	var (
		mockClient  *kubecli.MockKubevirtClient
		mockVMI     *kubecli.MockVirtualMachineInstanceInterface
		state       *multistep.BasicStateBag
		errorBuffer *strings.Builder
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockClient = kubecli.NewMockKubevirtClient(ctrl)
		mockVMI = kubecli.NewMockVirtualMachineInstanceInterface(ctrl)

		errorBuffer = &strings.Builder{}
		ui := &packer.BasicUi{
			Reader:      strings.NewReader(""),
			Writer:      io.Discard,
			ErrorWriter: errorBuffer,
		}
		state = new(multistep.BasicStateBag)
		state.Put("ui", ui)
	})

	Context("with a communicator configured", func() {
		DescribeTable("continues immediately without polling",
			func(commType string) {
				step := &iso.StepWaitForInstallation{
					Config: iso.Config{
						Name:                    name,
						Namespace:               namespace,
						Comm:                    communicator.Config{Type: commType},
						InstallationWaitTimeout: 5 * time.Minute,
					},
					Client: mockClient,
				}

				action := step.Run(context.Background(), state)
				Expect(action).To(Equal(multistep.ActionContinue))
			},
			Entry("when ssh communicator is configured", "ssh"),
			Entry("when winrm communicator is configured", "winrm"),
		)
	})

	Context("with communicator none", func() {
		It("continues when the guest agent connects before the timeout", func() {
			mockClient.EXPECT().
				VirtualMachineInstance(namespace).
				Return(mockVMI).
				AnyTimes()
			mockVMI.EXPECT().
				Get(gomock.Any(), name, gomock.Any()).
				Return(&v1.VirtualMachineInstance{
					Status: v1.VirtualMachineInstanceStatus{
						Conditions: []v1.VirtualMachineInstanceCondition{
							{
								Type:   v1.VirtualMachineInstanceAgentConnected,
								Status: corev1.ConditionTrue,
							},
						},
					},
				}, nil).
				AnyTimes()

			step := &iso.StepWaitForInstallation{
				Config: iso.Config{
					Name:                    name,
					Namespace:               namespace,
					Comm:                    communicator.Config{Type: "none"},
					InstallationWaitTimeout: 10 * time.Second,
				},
				Client: mockClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(errorBuffer.String()).NotTo(ContainSubstring("WARNING"))
		})

		It("continues with a warning when the guest agent never connects", func() {
			mockClient.EXPECT().
				VirtualMachineInstance(namespace).
				Return(mockVMI).
				AnyTimes()
			mockVMI.EXPECT().
				Get(gomock.Any(), name, gomock.Any()).
				Return(&v1.VirtualMachineInstance{
					Status: v1.VirtualMachineInstanceStatus{
						Conditions: []v1.VirtualMachineInstanceCondition{},
					},
				}, nil).
				AnyTimes()

			step := &iso.StepWaitForInstallation{
				Config: iso.Config{
					Name:                    name,
					Namespace:               namespace,
					Comm:                    communicator.Config{Type: "none"},
					InstallationWaitTimeout: 2 * time.Second,
				},
				Client: mockClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))
			Expect(errorBuffer.String()).To(ContainSubstring("WARNING"))
			Expect(errorBuffer.String()).To(ContainSubstring("guest agent did not connect"))
		})

		It("halts when no wait time is specified", func() {
			step := &iso.StepWaitForInstallation{
				Config: iso.Config{
					Name:                    name,
					Namespace:               namespace,
					Comm:                    communicator.Config{Type: "none"},
					InstallationWaitTimeout: 0,
				},
				Client: mockClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("installation_wait_timeout must be set")))
		})

		It("halts when Get returns an error", func() {
			mockClient.EXPECT().
				VirtualMachineInstance(namespace).
				Return(mockVMI).
				AnyTimes()
			mockVMI.EXPECT().
				Get(gomock.Any(), name, gomock.Any()).
				Return(nil, fmt.Errorf("connection refused")).
				AnyTimes()

			step := &iso.StepWaitForInstallation{
				Config: iso.Config{
					Name:                    name,
					Namespace:               namespace,
					Comm:                    communicator.Config{Type: "none"},
					InstallationWaitTimeout: 10 * time.Second,
				},
				Client: mockClient,
			}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("connection refused")))
		})

		It("halts when context is cancelled", func() {
			mockClient.EXPECT().
				VirtualMachineInstance(namespace).
				Return(mockVMI).
				AnyTimes()
			mockVMI.EXPECT().
				Get(gomock.Any(), name, gomock.Any()).
				Return(&v1.VirtualMachineInstance{
					ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
					Status: v1.VirtualMachineInstanceStatus{
						Conditions: []v1.VirtualMachineInstanceCondition{},
					},
				}, nil).
				AnyTimes()

			step := &iso.StepWaitForInstallation{
				Config: iso.Config{
					Name:                    name,
					Namespace:               namespace,
					Comm:                    communicator.Config{Type: "none"},
					InstallationWaitTimeout: 30 * time.Second,
				},
				Client: mockClient,
			}

			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				time.Sleep(1 * time.Second)
				cancel()
			}()

			action := step.Run(ctx, state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})
	})
})
