// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/clone"
)

func validRawConfig(overrides map[string]interface{}) map[string]interface{} {
	raw := map[string]interface{}{
		"kube_config":        "/tmp/kubeconfig",
		"name":               "fedora-clone",
		"namespace":          "images",
		"source_volume_name": "fedora-base",
		"disk_size":          "10Gi",
		"instance_type":      "u1.medium",
		"preference":         "fedora",
		"ssh_username":       "fedora",
	}
	for k, v := range overrides {
		raw[k] = v
	}
	return raw
}

var _ = Describe("Config", func() {
	Context("Prepare defaults", func() {
		It("defaults communicator to ssh", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.Comm.Type).To(Equal("ssh"))
		})

		It("defaults source_volume_namespace to namespace", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.SourceVolumeNamespace).To(Equal("images"))
		})

		It("accepts an explicit source_volume_namespace", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"source_volume_namespace": "other-ns",
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.SourceVolumeNamespace).To(Equal("other-ns"))
		})
	})

	Context("Prepare validation", func() {
		DescribeTable("validates the source volume",
			func(overrides map[string]interface{}, expectedErr string) {
				c := &clone.Config{}
				_, err := c.Prepare(validRawConfig(overrides))
				if expectedErr == "" {
					Expect(err).NotTo(HaveOccurred())
				} else {
					Expect(err).To(MatchError(ContainSubstring(expectedErr)))
				}
			},
			Entry("reports source_volume_name as required",
				map[string]interface{}{"source_volume_name": ""},
				"source_volume_name must be specified"),
			Entry("rejects source and target referring to the same volume",
				map[string]interface{}{"source_volume_name": "fedora-clone"},
				"source_volume_name and name cannot refer to the same volume in the same namespace"),
			Entry("rejects source equal to the temporary disk name in the same namespace",
				map[string]interface{}{"source_volume_name": "fedora-clone-rootdisk"},
				"source_volume_name cannot equal the temporary disk name"),
			Entry("allows same name in different namespaces",
				map[string]interface{}{
					"source_volume_name":      "fedora-clone",
					"source_volume_namespace": "other-ns",
				},
				""),
			Entry("allows temporary disk name as source in a different namespace",
				map[string]interface{}{
					"source_volume_name":      "fedora-clone-rootdisk",
					"source_volume_namespace": "other-ns",
				},
				""),
			Entry("rejects an invalid source_volume_namespace",
				map[string]interface{}{"source_volume_namespace": "INVALID"},
				"source_volume_namespace"),
		)

		DescribeTable("rejects invalid network configurations",
			func(overrides map[string]interface{}, expected string) {
				c := &clone.Config{}
				_, err := c.Prepare(validRawConfig(overrides))
				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("network with both pod and multus", map[string]interface{}{
				"networks": []map[string]interface{}{{
					"name":   "default",
					"pod":    map[string]interface{}{},
					"multus": map[string]interface{}{"networkName": "net1"},
				}},
			}, "only one of pod or multus can be defined"),
			Entry("network without a name", map[string]interface{}{
				"networks": []map[string]interface{}{{"pod": map[string]interface{}{}}},
			}, "networks[0]: name must be specified"),
			Entry("network name with invalid characters", map[string]interface{}{
				"networks": []map[string]interface{}{{"name": "net.1", "pod": map[string]interface{}{}}},
			}, "the name can only contain letters, digits, '-' and '_'"),
			Entry("networks with duplicate names", map[string]interface{}{
				"networks": []map[string]interface{}{
					{"name": "default", "pod": map[string]interface{}{}},
					{"name": "default", "multus": map[string]interface{}{"networkName": "net1"}},
				},
			}, "names must be unique"),
		)

		It("accepts valid network names", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"networks": []map[string]interface{}{{"name": "Net_1", "pod": map[string]interface{}{}}},
			}))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
