// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"

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
			Expect(c.Comm.Type).To(Equal("none"))
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
		It("uses the SDK defaults for SSH", func() {
			c := &iso.Config{}
			warnings, err := c.Prepare(validRawConfig(map[string]interface{}{
				"communicator": "ssh",
				"ssh_username": "fedora",
				"ssh_password": "fedora",
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(warnings).To(BeEmpty())
			Expect(c.Comm.SSHPort).To(Equal(22))
			Expect(c.Comm.SSHTimeout).To(Equal(5 * time.Minute))
		})

		It("uses the SDK defaults for WinRM", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{
				"communicator":   "winrm",
				"winrm_username": "Administrator",
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.Comm.WinRMPort).To(Equal(5985))
			Expect(c.Comm.WinRMTimeout).To(Equal(30 * time.Minute))
		})

		It("reports communicator errors at validation time", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{"communicator": "ssh"}))
			Expect(err).To(MatchError(ContainSubstring("An ssh_username must be specified")))
		})

		It("rejects communicators the builder cannot connect with", func() {
			c := &iso.Config{}
			_, err := c.Prepare(validRawConfig(map[string]interface{}{"communicator": "docker"}))
			Expect(err).To(MatchError(ContainSubstring("communicator \"docker\" is not supported")))
		})

		It("accepts an SSH private key", func() {
			_, key, err := ed25519.GenerateKey(rand.Reader)
			Expect(err).NotTo(HaveOccurred())
			block, err := ssh.MarshalPrivateKey(key, "")
			Expect(err).NotTo(HaveOccurred())
			keyFile := filepath.Join(GinkgoT().TempDir(), "id_ed25519")
			Expect(os.WriteFile(keyFile, pem.EncodeToMemory(block), 0o600)).To(Succeed())

			c := &iso.Config{}
			_, err = c.Prepare(validRawConfig(map[string]interface{}{
				"communicator":         "ssh",
				"ssh_username":         "fedora",
				"ssh_private_key_file": keyFile,
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(c.Comm.SSHPrivateKeyFile).To(Equal(keyFile))
		})

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
