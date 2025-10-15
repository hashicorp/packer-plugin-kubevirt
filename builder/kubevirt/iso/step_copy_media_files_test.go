// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakek8sclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/testing"
)

var _ = Describe("StepCopyMediaFiles", func() {
	const (
		namespace = "test-ns"
		name      = "media-config"
	)

	var (
		state      *multistep.BasicStateBag
		step       *iso.StepCopyMediaFiles
		kubeClient *fakek8sclient.Clientset
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

		kubeClient = fakek8sclient.NewSimpleClientset()

		step = &iso.StepCopyMediaFiles{
			Config: iso.Config{
				VMName:     name,
				Namespace:  namespace,
				MediaFiles: []string{"file1.iso", "file2.iso"},
			},
			Client: kubeClient,
		}
	})

	Context("Run", func() {
		It("continues when ConfigMap is created successfully", func() {
			// Create dummy files so configMap() can read them
			err := os.WriteFile("file1.iso", []byte("fake iso data 1"), 0644)
			Expect(err).NotTo(HaveOccurred())
			err = os.WriteFile("file2.iso", []byte("fake iso data 2"), 0644)
			Expect(err).NotTo(HaveOccurred())

			defer os.Remove("file1.iso")
			defer os.Remove("file2.iso")

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKey("file1.iso"))
			Expect(cm.Data).To(HaveKey("file2.iso"))
		})

		It("halts when ConfigMap creation fails due to invalid media files", func() {
			// Simulate invalid media file by injecting empty name
			step.Config.MediaFiles = []string{""}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})

		It("halts when ConfigMap creation fails due to API error", func() {
			// Simulate API failure with reactor
			kubeClient.PrependReactor("create", "configmaps", func(action testing.Action) (bool, runtime.Object, error) {
				gr := schema.GroupResource{Group: "", Resource: "configmaps"}
				return true, nil, errors.NewNotFound(gr, "fail")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
		})
	})

	Context("Cleanup", func() {
		It("deletes ConfigMap successfully", func() {
			// Pre-create ConfigMap
			_, err := kubeClient.CoreV1().ConfigMaps(namespace).Create(context.Background(), &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Data: map[string]string{"file1.iso": "data"},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			// Cleanup
			step.Cleanup(state)

			_, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).To(HaveOccurred()) // Should be deleted
		})
	})
})
