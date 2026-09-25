// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/iso"
)

var _ = Describe("Config", func() {
	Context("Prepare media_label", func() {
		It("defaults to OEMDRV", func() {
			c := &iso.Config{}
			_, err := c.Prepare(map[string]interface{}{})
			Expect(err).NotTo(HaveOccurred())
			Expect(c.MediaLabel).To(Equal(iso.DefaultMediaLabel))
		})

		It("keeps a user-provided label", func() {
			c := &iso.Config{}
			_, err := c.Prepare(map[string]interface{}{"media_label": "cidata"})
			Expect(err).NotTo(HaveOccurred())
			Expect(c.MediaLabel).To(Equal("cidata"))
		})

		It("rejects labels longer than 32 characters", func() {
			c := &iso.Config{}
			_, err := c.Prepare(map[string]interface{}{"media_label": strings.Repeat("a", 33)})
			Expect(err).To(MatchError(ContainSubstring("media_label")))
		})
	})
})
