// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestClone(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Clone Builder Suite")
}
