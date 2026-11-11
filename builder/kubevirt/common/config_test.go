// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common_test

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

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
)

// validBuilderConfig returns a BuilderConfig with all required fields set.
func validBuilderConfig(overrides func(*common.BuilderConfig)) common.BuilderConfig {
	c := common.BuilderConfig{
		KubeConfig:   "/tmp/kubeconfig",
		Name:         "fedora-image",
		Namespace:    "images",
		DiskSize:     "10Gi",
		InstanceType: "u1.medium",
		Preference:   "fedora",
	}
	c.Comm.Type = "none"
	if overrides != nil {
		overrides(&c)
	}
	return c
}

func validate(c *common.BuilderConfig) []error {
	return c.Validate(&interpolate.Context{})
}

var _ = Describe("Common config", func() {
	Context("Prepare media_label", func() {
		It("defaults to OEMDRV", func() {
			c := validBuilderConfig(nil)
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.MediaLabel).To(Equal(common.DefaultMediaLabel))
		})

		It("keeps a user-provided label", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) { c.MediaLabel = "cidata" })
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.MediaLabel).To(Equal("cidata"))
		})

		It("rejects labels longer than 32 characters", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) { c.MediaLabel = strings.Repeat("a", 33) })
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("media_label"))))
		})
	})

	Context("Prepare media_files", func() {
		var dir string

		writeFile := func(path string, size int) string {
			Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
			Expect(os.WriteFile(path, make([]byte, size), 0o644)).To(Succeed())
			return path
		}

		BeforeEach(func() {
			dir = GinkgoT().TempDir()
		})

		It("accepts existing files", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.MediaFiles = []string{writeFile(filepath.Join(dir, "ks.cfg"), 10), writeFile(filepath.Join(dir, "driver.cat"), 10)}
			})
			Expect(validate(&c)).To(BeEmpty())
		})

		DescribeTable("rejects files that cannot be stored in the media ConfigMap",
			func(files func() []string, expected string) {
				c := validBuilderConfig(func(c *common.BuilderConfig) { c.MediaFiles = files() })
				Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring(expected))))
			},
			Entry("missing file", func() []string { return []string{filepath.Join(dir, "missing.cfg")} }, "missing.cfg"),
			Entry("directory", func() []string { return []string{dir} }, "is not a regular file"),
			Entry("duplicate file names", func() []string {
				return []string{writeFile(filepath.Join(dir, "a", "ks.cfg"), 1), writeFile(filepath.Join(dir, "b", "ks.cfg"), 1)}
			}, "have the same file name"),
			Entry("invalid file name", func() []string { return []string{writeFile(filepath.Join(dir, "my ks.cfg"), 1)} }, "the file name of"),
			Entry("more than a ConfigMap can hold", func() []string {
				return []string{writeFile(filepath.Join(dir, "big.iso"), 1024*1024+1)}
			}, "a ConfigMap can hold at most 1048576 bytes"),
		)

		It("rejects media_content that clashes with media_files", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.MediaFiles = []string{writeFile(filepath.Join(dir, "ks.cfg"), 10)}
				c.MediaContent = map[string]string{"ks.cfg": "text"}
			})
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("media_content: \"ks.cfg\" is also provided by media_files"))))
		})

		It("rejects invalid media_content file names", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.MediaContent = map[string]string{"../ks.cfg": "text"}
			})
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("media_content: the file name \"../ks.cfg\" is invalid"))))
		})

		It("counts media_content towards the ConfigMap size limit", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.MediaFiles = []string{writeFile(filepath.Join(dir, "big.iso"), 1024*1024)}
				c.MediaContent = map[string]string{"ks.cfg": "x"}
			})
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("a ConfigMap can hold at most 1048576 bytes"))))
		})
	})

	Context("Prepare kube_config", func() {
		It("is required", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) { c.KubeConfig = "" })
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("kube_config must be specified"))))
		})

		It("expands a leading ~ to the home directory", func() {
			current, err := user.Current()
			Expect(err).NotTo(HaveOccurred())

			c := validBuilderConfig(func(c *common.BuilderConfig) { c.KubeConfig = "~/.kube/config" })
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.KubeConfig).To(Equal(filepath.Join(current.HomeDir, ".kube", "config")))
		})
	})

	Context("Prepare defaults", func() {
		It("applies the documented defaults", func() {
			c := validBuilderConfig(nil)
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.DiskBus).To(Equal("scsi"))
			Expect(c.InstanceTypeKind).To(Equal("virtualmachineclusterinstancetype"))
			Expect(c.PreferenceKind).To(Equal("virtualmachineclusterpreference"))
		})

		It("does not check label values when no DataSource is created", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.InstanceType = strings.Repeat("a", 64)
				c.SkipCreateImage = true
			})
			Expect(validate(&c)).To(BeEmpty())
		})

		It("accepts namespaced kinds the way KubeVirt resolves them", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.InstanceTypeKind = "VirtualMachineInstancetype"
				c.PreferenceKind = "virtualmachinepreferences"
			})
			Expect(validate(&c)).To(BeEmpty())
		})
	})

	Context("Prepare communicator", func() {
		It("uses the SDK defaults for SSH", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.Comm.Type = "ssh"
				c.Comm.SSHUsername = "fedora"
				c.Comm.SSHPassword = "fedora"
			})
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.Comm.SSHPort).To(Equal(22))
			Expect(c.Comm.SSHTimeout).To(Equal(5 * time.Minute))
		})

		It("uses the SDK defaults for WinRM", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.Comm.Type = "winrm"
				c.Comm.WinRMUser = "Administrator"
			})
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.Comm.WinRMPort).To(Equal(5985))
			Expect(c.Comm.WinRMTimeout).To(Equal(30 * time.Minute))
		})

		It("reports communicator errors at validation time", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) { c.Comm.Type = "ssh" })
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("An ssh_username must be specified"))))
		})

		DescribeTable("rejects options that cannot reach the port forward",
			func(setup func(*common.BuilderConfig)) {
				c := validBuilderConfig(func(c *common.BuilderConfig) {
					c.Comm.Type = "ssh"
					c.Comm.SSHUsername = "fedora"
					c.Comm.SSHPassword = "fedora"
					c.Comm.SSHBastionPassword = "bastion"
					setup(c)
				})
				Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("is not supported"))))
			},
			Entry("bastion host", func(c *common.BuilderConfig) { c.Comm.SSHBastionHost = "bastion.example.com" }),
			Entry("SOCKS proxy", func(c *common.BuilderConfig) { c.Comm.SSHProxyHost = "proxy.example.com" }),
		)

		It("rejects communicators the builder cannot connect with", func() {
			c := validBuilderConfig(func(c *common.BuilderConfig) { c.Comm.Type = "docker" })
			Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring("communicator \"docker\" is not supported"))))
		})

		It("accepts an SSH private key", func() {
			_, key, err := ed25519.GenerateKey(rand.Reader)
			Expect(err).NotTo(HaveOccurred())
			block, err := ssh.MarshalPrivateKey(key, "")
			Expect(err).NotTo(HaveOccurred())
			keyFile := filepath.Join(GinkgoT().TempDir(), "id_ed25519")
			Expect(os.WriteFile(keyFile, pem.EncodeToMemory(block), 0o600)).To(Succeed())

			c := validBuilderConfig(func(c *common.BuilderConfig) {
				c.Comm.Type = "ssh"
				c.Comm.SSHUsername = "fedora"
				c.Comm.SSHPrivateKeyFile = keyFile
			})
			Expect(validate(&c)).To(BeEmpty())
			Expect(c.Comm.SSHPrivateKeyFile).To(Equal(keyFile))
		})
	})

	Context("Prepare validation", func() {
		It("reports every missing required option at once", func() {
			c := common.BuilderConfig{}
			c.Comm.Type = "none"
			errs := validate(&c)
			errStr := ""
			for _, e := range errs {
				errStr += e.Error() + "\n"
			}
			for _, option := range []string{"kube_config", "name", "namespace", "disk_size", "instance_type", "preference"} {
				Expect(errStr).To(ContainSubstring(option + " must be specified"))
			}
		})

		DescribeTable("rejects invalid values",
			func(setup func(*common.BuilderConfig), expected string) {
				c := validBuilderConfig(setup)
				Expect(validate(&c)).To(ContainElement(MatchError(ContainSubstring(expected))))
			},
			Entry("disk_size that is not a quantity", func(c *common.BuilderConfig) { c.DiskSize = "10 GB" }, "disk_size \"10 GB\" is not a valid Kubernetes quantity"),
			Entry("disk_size that is not positive", func(c *common.BuilderConfig) { c.DiskSize = "0" }, "disk_size \"0\" must be greater than zero"),
			Entry("name too long for the root disk", func(c *common.BuilderConfig) { c.Name = strings.Repeat("a", 250) }, "name is too long: the root disk DataVolume"),
			Entry("name that is not a DNS subdomain", func(c *common.BuilderConfig) { c.Name = "Fedora_Image" }, "name \"Fedora_Image\" is invalid"),
			Entry("namespace that is not a DNS label", func(c *common.BuilderConfig) { c.Namespace = "my.images" }, "namespace \"my.images\" is invalid"),
			Entry("unsupported access_mode", func(c *common.BuilderConfig) { c.AccessMode = "ReadOnlyMany" }, "access_mode \"ReadOnlyMany\" is not supported"),
			Entry("unsupported volume_mode", func(c *common.BuilderConfig) { c.VolumeMode = "block" }, "volume_mode \"block\" is not supported"),
			Entry("invalid storage_class_name", func(c *common.BuilderConfig) { c.StorageClassName = "Fast SSD" }, "storage_class_name \"Fast SSD\" is invalid"),
			Entry("instance_type that cannot be a label value", func(c *common.BuilderConfig) { c.InstanceType = strings.Repeat("a", 64) }, "cannot be used as a DataSource label"),
			Entry("unsupported instance_type_kind", func(c *common.BuilderConfig) { c.InstanceTypeKind = "instancetype.kubevirt.io" }, "instance_type_kind \"instancetype.kubevirt.io\" is not supported"),
			Entry("unsupported preference_kind", func(c *common.BuilderConfig) { c.PreferenceKind = "preference" }, "preference_kind \"preference\" is not supported"),
			Entry("virtio disk_bus for CD-ROMs", func(c *common.BuilderConfig) { c.DiskBus = "virtio" }, "use \"scsi\", \"sata\" or \"usb\""),
			Entry("unknown disk_bus", func(c *common.BuilderConfig) { c.DiskBus = "ide" }, "disk_bus \"ide\" is not supported"),
		)
	})
})
