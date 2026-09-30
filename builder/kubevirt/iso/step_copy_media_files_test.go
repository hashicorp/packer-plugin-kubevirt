// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
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

		mediaDir := GinkgoT().TempDir()
		file1 := filepath.Join(mediaDir, "file1.iso")
		file2 := filepath.Join(mediaDir, "file2.iso")
		Expect(os.WriteFile(file1, []byte("fake iso data 1"), 0o644)).To(Succeed())
		Expect(os.WriteFile(file2, []byte("fake iso data 2"), 0o644)).To(Succeed())

		step = &iso.StepCopyMediaFiles{
			Config: iso.Config{
				Name:       name,
				Namespace:  namespace,
				MediaFiles: []string{file1, file2},
			},
			Client: kubeClient,
		}
	})

	Context("Run", func() {
		It("continues when ConfigMap is created successfully", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKeyWithValue("file1.iso", "fake iso data 1"))
			Expect(cm.Data).To(HaveKeyWithValue("file2.iso", "fake iso data 2"))
		})

		It("halts when ConfigMap creation fails due to invalid media files", func() {
			// Simulate invalid media file by injecting empty name
			step.Config.MediaFiles = []string{""}

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("failed to read the media files")))
		})

		It("halts when ConfigMap creation fails due to API error", func() {
			// Simulate API failure with reactor
			kubeClient.PrependReactor("create", "configmaps", func(action testing.Action) (bool, runtime.Object, error) {
				gr := schema.GroupResource{Group: "", Resource: "configmaps"}
				return true, nil, errors.NewNotFound(gr, "fail")
			})

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("failed to create the ConfigMap")))
		})
	})

	Context("Cleanup", func() {
		It("deletes the ConfigMap it created", func() {
			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionContinue))

			step.Cleanup(state)

			_, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})

		It("only deletes the ConfigMap instance it created", func() {
			kubeClient.PrependReactor("create", "configmaps", func(action testing.Action) (bool, runtime.Object, error) {
				cm := action.(testing.CreateAction).GetObject().(*corev1.ConfigMap)
				cm.UID = "created-uid"
				return false, cm, nil
			})
			var deleteOptions metav1.DeleteOptions
			kubeClient.PrependReactor("delete", "configmaps", func(action testing.Action) (bool, runtime.Object, error) {
				deleteOptions = action.(testing.DeleteAction).GetDeleteOptions()
				return false, nil, nil
			})

			Expect(step.Run(context.Background(), state)).To(Equal(multistep.ActionContinue))
			step.Cleanup(state)

			Expect(deleteOptions.Preconditions).NotTo(BeNil())
			Expect(deleteOptions.Preconditions.UID).To(HaveValue(BeEquivalentTo("created-uid")))
		})

		It("does not delete a pre-existing ConfigMap with the same name", func() {
			_, err := kubeClient.CoreV1().ConfigMaps(namespace).Create(context.Background(), &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Data: map[string]string{"user-data": "unrelated"},
			}, metav1.CreateOptions{})
			Expect(err).NotTo(HaveOccurred())

			action := step.Run(context.Background(), state)
			Expect(action).To(Equal(multistep.ActionHalt))
			Expect(state.Get("error")).To(MatchError(ContainSubstring("already exists")))

			step.Cleanup(state)

			cm, err := kubeClient.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
			Expect(err).NotTo(HaveOccurred())
			Expect(cm.Data).To(HaveKeyWithValue("user-data", "unrelated"))
		})
	})
})
