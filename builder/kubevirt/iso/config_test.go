// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"os/user"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
)

// validRawConfig returns the minimal set of options accepted by Config.Prepare,
// merged with the given overrides.
func validRawConfig(overrides map[string]interface{}) map[string]interface{} {
	raw := map[string]interface{}{
		"kube_config":     "/tmp/kubeconfig",
		"name":            "fedora-image",
		"namespace":       "images",
		"iso_volume_name": "fedora-iso",
		"disk_size":       "10Gi",
		"instance_type":   "u1.medium",
		"preference":      "fedora",
		// Required without a communicator.
		"installation_wait_timeout": "15m",
	}
	for k, v := range overrides {
		raw[k] = v
	}
	return raw
}

var _ = Describe("Config", func() {
	Context("Prepare media_label", func() {
		It("defaults to OEMDRV", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.MediaLabel).To(Equal(iso.DefaultMediaLabel))
		})

		It("keeps a user-provided label", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{"media_label": "cidata"}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.MediaLabel).To(Equal("cidata"))
		})

		It("rejects labels longer than 32 characters", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{"media_label": strings.Repeat("a", 33)}))
			Expect(err).To(MatchError(ContainSubstring("media_label")))
		})
	})

	Context("Prepare kube_config", func() {
		It("is required", func() {
			c := &iso.Config{}
			raw := validRawConfig(nil)
			delete(raw, "kube_config")
			_, err := c.Prepare(raw)
			Expect(err).To(MatchError(ContainSubstring("kube_config must be specified")))
		})

		It("expands a leading ~ to the home directory", func() {
			current, err := user.Current()
			Expect(err).NotTo(HaveOccurred())

			c := &iso.Config{}
			_, err = c.Prepare(validRawConfig(map[string]interface{}{"kube_config": "~/.kube/config"}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.KubeConfig).To(Equal(filepath.Join(current.HomeDir, ".kube", "config")))
		})
	})

	Context("Prepare defaults", func() {
		It("applies the documented defaults", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.OperatingSystemType).To(Equal("linux"))
			Expect(c.DiskBus).To(Equal("scsi"))
			Expect(c.InstanceTypeKind).To(Equal("virtualmachineclusterinstancetype"))
			Expect(c.PreferenceKind).To(Equal("virtualmachineclusterpreference"))
		})

		It("accepts network names that KubeVirt accepts", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"networks": []map[string]interface{}{{"name": "Net_1", "pod": map[string]interface{}{}}},
			}))
			Expect(err).NotTo(HaveOccurred())
		})

		It("accepts namespaced kinds the way KubeVirt resolves them", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"instance_type_kind": "VirtualMachineInstancetype",
				"preference_kind":    "virtualmachinepreferences",
			}))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Prepare installation_wait_timeout", func() {
		It("is required without a communicator", func() {
			c := &iso.Config{}
			raw := validRawConfig(nil)
			delete(raw, "installation_wait_timeout")
			_, err := c.Prepare(raw)
			Expect(err).To(MatchError(ContainSubstring("installation_wait_timeout must be set when no communicator is configured")))
		})

		It("is optional with a communicator", func() {
			c := &iso.Config{}
			raw := validRawConfig(map[string]interface{}{"communicator": "ssh"})
			delete(raw, "installation_wait_timeout")
			_, err := c.Prepare(raw)
			Expect(err).NotTo(HaveOccurred())
		})

		It("must not be negative", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{"installation_wait_timeout": "-5m"}))
			Expect(err).To(MatchError(ContainSubstring("installation_wait_timeout must not be negative")))
		})
	})

	Context("Prepare validation", func() {
		It("reports every missing required option at once", func() {
			c := &iso.Config{}
			_, err := c.Prepare(map[string]interface{}{})
			Expect(err).To(HaveOccurred())
			for _, option := range []string{"kube_config", "name", "namespace", "iso_volume_name", "disk_size", "instance_type", "preference"} {
				Expect(err.Error()).To(ContainSubstring(option + " must be specified"))
			}
		})

		DescribeTable("rejects invalid values",
			func(overrides map[string]interface{}, expected string) {
				c := &iso.Config{}
				_, err := c.Prepare(validRawConfig(overrides))
				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("disk_size that is not a quantity", map[string]interface{}{"disk_size": "10 GB"}, "disk_size \"10 GB\" is not a valid Kubernetes quantity"),
			Entry("disk_size that is not positive", map[string]interface{}{"disk_size": "0"}, "disk_size \"0\" must be greater than zero"),
			Entry("name too long for the root disk", map[string]interface{}{"name": strings.Repeat("a", 250)}, "name is too long: the root disk DataVolume"),
			Entry("name that is not a DNS subdomain", map[string]interface{}{"name": "Fedora_Image"}, "name \"Fedora_Image\" is invalid"),
			Entry("namespace that is not a DNS label", map[string]interface{}{"namespace": "my.images"}, "namespace \"my.images\" is invalid"),
			Entry("unsupported os_type", map[string]interface{}{"os_type": "bsd"}, "os_type \"bsd\" is not supported"),
			Entry("unsupported instance_type_kind", map[string]interface{}{"instance_type_kind": "instancetype.kubevirt.io"}, "instance_type_kind \"instancetype.kubevirt.io\" is not supported"),
			Entry("unsupported preference_kind", map[string]interface{}{"preference_kind": "preference"}, "preference_kind \"preference\" is not supported"),
			Entry("virtio disk_bus for CD-ROMs", map[string]interface{}{"disk_bus": "virtio"}, "use \"scsi\", \"sata\" or \"usb\""),
			Entry("unknown disk_bus", map[string]interface{}{"disk_bus": "ide"}, "disk_bus \"ide\" is not supported"),
			Entry("malformed boot_command", map[string]interface{}{"boot_command": []string{"<wait0s>"}}, "boot_command is invalid"),
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
			Entry("network name KubeVirt rejects", map[string]interface{}{
				"networks": []map[string]interface{}{{"name": "net.1", "pod": map[string]interface{}{}}},
			}, "network \"net.1\": the name can only contain letters, digits, '-' and '_'"),
			Entry("networks with duplicate names", map[string]interface{}{
				"networks": []map[string]interface{}{
					{"name": "default", "pod": map[string]interface{}{}},
					{"name": "default", "multus": map[string]interface{}{"networkName": "net1"}},
				},
			}, "network \"default\": names must be unique"),
		)
	})
})
