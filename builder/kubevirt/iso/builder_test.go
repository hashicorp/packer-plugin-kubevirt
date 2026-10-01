// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
	"github.com/hashicorp/packer-plugin-sdk/packer"
)

// unreachableKubeConfig writes a kubeconfig pointing at a local port that
// nothing listens on, so every API request made by the builder fails fast.
func unreachableKubeConfig() string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	address := listener.Addr().String()
	Expect(listener.Close()).To(Succeed())

	kubeConfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters:
- name: test
  cluster:
    server: http://%s
contexts:
- name: test
  context:
    cluster: test
    user: test
current-context: test
users:
- name: test
  user: {}
`, address)

	path := filepath.Join(GinkgoT().TempDir(), "kubeconfig")
	Expect(os.WriteFile(path, []byte(kubeConfig), 0o600)).To(Succeed())
	return path
}

var _ = Describe("Builder", func() {
	Context("Run", func() {
		DescribeTable("returns the error of the failed step",
			func(skipCreateImage bool, expected string) {
				ui := &packer.BasicUi{
					Reader:      strings.NewReader(""),
					Writer:      io.Discard,
					ErrorWriter: io.Discard,
				}

				builder := &iso.Builder{}
				_, _, err := builder.Prepare(map[string]interface{}{
					"kube_config":               unreachableKubeConfig(),
					"name":                      "test-vm",
					"namespace":                 "test-ns",
					"iso_volume_name":           "test-iso",
					"disk_size":                 "10Gi",
					"instance_type":             "u1.medium",
					"preference":                "fedora",
					"os_type":                   "linux",
					"installation_wait_timeout": "1m",
					"skip_create_image":         skipCreateImage,
				})
				Expect(err).NotTo(HaveOccurred())

				artifact, err := builder.Run(context.Background(), ui, &packer.MockHook{})
				Expect(artifact).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("when a bootable volume is requested", false, "failed to check whether the DataVolume test-ns/test-vm exists"),
			Entry("when skip_create_image is set", true, "failed to get the ISO DataVolume (test-ns/test-iso)"),
		)

		It("returns the context error when the build is cancelled", func() {
			builder := &iso.Builder{}
			_, _, err := builder.Prepare(map[string]interface{}{
				"kube_config":               unreachableKubeConfig(),
				"name":                      "test-vm",
				"namespace":                 "test-ns",
				"iso_volume_name":           "test-iso",
				"disk_size":                 "10Gi",
				"instance_type":             "u1.medium",
				"preference":                "fedora",
				"os_type":                   "linux",
				"installation_wait_timeout": "1m",
			})
			Expect(err).NotTo(HaveOccurred())

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			ui := &packer.BasicUi{Reader: strings.NewReader(""), Writer: io.Discard, ErrorWriter: io.Discard}
			artifact, err := builder.Run(ctx, ui, &packer.MockHook{})
			Expect(artifact).To(BeNil())
			Expect(err).To(MatchError(context.Canceled))
		})
	})
})
