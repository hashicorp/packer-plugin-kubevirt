// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/clone"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	clientcmdapi "k8s.io/client-go/tools/clientcmd/api/v1"
)

func unreachableKubeConfig() string {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	address := listener.Addr().String()
	Expect(listener.Close()).To(Succeed())

	config := clientcmdapi.Config{
		Clusters: []clientcmdapi.NamedCluster{{
			Name:    "test",
			Cluster: clientcmdapi.Cluster{Server: "http://" + address},
		}},
		AuthInfos: []clientcmdapi.NamedAuthInfo{{
			Name: "test",
		}},
		Contexts: []clientcmdapi.NamedContext{{
			Name: "test",
			Context: clientcmdapi.Context{
				Cluster:  "test",
				AuthInfo: "test",
			},
		}},
		CurrentContext: "test",
	}

	data, err := json.Marshal(config)
	Expect(err).NotTo(HaveOccurred())

	path := filepath.Join(GinkgoT().TempDir(), "kubeconfig")
	Expect(os.WriteFile(path, data, 0o600)).To(Succeed())
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

				builder := &clone.Builder{}
				_, _, err := builder.Prepare(map[string]interface{}{
					"kube_config":        unreachableKubeConfig(),
					"name":               "test-vm",
					"namespace":          "test-ns",
					"source_volume_name": "source-dv",
					"disk_size":          "10Gi",
					"instance_type":      "u1.medium",
					"preference":         "fedora",
					"ssh_username":       "fedora",
					"skip_create_image":  skipCreateImage,
				})
				Expect(err).NotTo(HaveOccurred())

				artifact, err := builder.Run(context.Background(), ui, &packer.MockHook{})
				Expect(artifact).To(BeNil())
				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("when a bootable volume is requested", false, "failed to check whether the DataVolume test-ns/test-vm exists"),
			Entry("when skip_create_image is set", true, "failed to get the source DataVolume"),
		)

		It("returns the context error when the build is cancelled", func() {
			builder := &clone.Builder{}
			_, _, err := builder.Prepare(map[string]interface{}{
				"kube_config":        unreachableKubeConfig(),
				"name":               "test-vm",
				"namespace":          "test-ns",
				"source_volume_name": "source-dv",
				"disk_size":          "10Gi",
				"instance_type":      "u1.medium",
				"preference":         "fedora",
				"ssh_username":       "fedora",
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
