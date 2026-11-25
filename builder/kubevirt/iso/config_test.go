// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"os"
	"path/filepath"
	"time"

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

var _ = Describe("ISO config", func() {
	Context("Prepare defaults", func() {
		It("applies the documented defaults", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.OperatingSystemType).To(Equal("linux"))
			Expect(c.Comm.Type).To(Equal("none"))
			Expect(c.VirtIOContainerImage).To(Equal(iso.DefaultVirtIOContainerImage))
		})

		It("accepts network names that KubeVirt accepts", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"networks": []map[string]interface{}{{"name": "Net_1", "pod": map[string]interface{}{}}},
			}))
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("Prepare media_content", func() {
		It("accepts media_content alongside media_files", func() {
			c := &iso.Config{}
			path := filepath.Join(GinkgoT().TempDir(), "ks.cfg")

			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, make([]byte, 10), 0o644)).To(Succeed())

			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"media_files":   []string{path},
				"media_content": map[string]string{"user-data": "#cloud-config", "meta-data": ""},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.MediaContent).To(HaveKeyWithValue("user-data", "#cloud-config"))
		})

		It("keeps media_content as is", func() {
			content := "%post\npodman ps --format '{{ .Names }}'\n%end\n"

			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"media_content": map[string]string{"ks.cfg": content},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.MediaContent).To(HaveKeyWithValue("ks.cfg", content))
		})

		It("rejects media_content that does not parse as a Go template", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"media_content": map[string]string{"user-data": "## template: jinja\nhostname: {{ v1.local_hostname }}\n"},
			}))
			Expect(err).To(MatchError(ContainSubstring(`invalid 'media_content': template: root:2: function "v1" not defined`)))
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
			raw := validRawConfig(map[string]interface{}{"communicator": "ssh", "ssh_username": "fedora"})
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

	Context("Prepare communicator", func() {
		It("maps the deprecated SSH options and warns about them", func() {
			c := &iso.Config{}
			warnings, err := c.Prepare(validRawConfig(map[string]interface{}{
				"communicator":     "ssh",
				"ssh_username":     "fedora",
				"ssh_remote_port":  2222,
				"ssh_wait_timeout": "20m",
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ConsistOf(
				"ssh_remote_port is deprecated, use ssh_port instead",
				"ssh_wait_timeout is deprecated, use ssh_timeout instead",
			))
			Expect(c.Comm.SSHPort).To(Equal(2222))
			Expect(c.Comm.SSHTimeout).To(Equal(20 * time.Minute))
			// Like before the SDK configuration was used, and like ssh_timeout.
			Expect(c.Comm.SSHHandshakeAttempts).To(Equal(0))
		})

		It("maps the deprecated WinRM options and warns about them", func() {
			c := &iso.Config{}
			warnings, err := c.Prepare(validRawConfig(map[string]interface{}{
				"communicator":       "winrm",
				"winrm_username":     "Administrator",
				"winrm_remote_port":  5986,
				"winrm_wait_timeout": "25m",
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(ConsistOf(
				"winrm_remote_port is deprecated, use winrm_port instead",
				"winrm_wait_timeout is deprecated, use winrm_timeout instead",
			))
			Expect(c.Comm.WinRMPort).To(Equal(5986))
			Expect(c.Comm.WinRMTimeout).To(Equal(25 * time.Minute))
		})

		It("prefers the SDK options over the deprecated ones", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"communicator":    "ssh",
				"ssh_username":    "fedora",
				"ssh_port":        22,
				"ssh_remote_port": 2222,
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.Comm.SSHPort).To(Equal(22))
		})
	})

	Context("Prepare validation", func() {
		It("reports iso_volume_name as required", func() {
			c := &iso.Config{}
			raw := validRawConfig(nil)
			delete(raw, "iso_volume_name")
			_, err := c.Prepare(raw)
			Expect(err).To(MatchError(ContainSubstring("iso_volume_name must be specified")))
		})

		DescribeTable("rejects invalid values",
			func(overrides map[string]interface{}, expected string) {
				c := &iso.Config{}
				_, err := c.Prepare(validRawConfig(overrides))
				Expect(err).To(MatchError(ContainSubstring(expected)))
			},
			Entry("unsupported os_type", map[string]interface{}{"os_type": "bsd"}, "os_type \"bsd\" is not supported"),
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
