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
		It("reports source_volume_name as required", func() {
			c := &clone.Config{}
			raw := validRawConfig(nil)
			delete(raw, "source_volume_name")
			_, err := c.Prepare(raw)
			Expect(err).To(MatchError(ContainSubstring("source_volume_name must be specified")))
		})

		It("rejects source and target referring to the same volume", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"source_volume_name": "fedora-clone",
			}))
			Expect(err).To(MatchError(ContainSubstring("source_volume_name and name cannot refer to the same volume in the same namespace")))
		})

		It("allows same name in different namespaces", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"source_volume_name":      "fedora-clone",
				"source_volume_namespace": "other-ns",
			}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("rejects an invalid source_volume_namespace", func() {
			c := &clone.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"source_volume_namespace": "INVALID",
			}))
			Expect(err).To(MatchError(ContainSubstring("source_volume_namespace")))
		})

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
