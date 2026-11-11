// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/acctest"

	kubevirttest "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/testing"
)

const (
	testDiskSize = "6Gi"

	testFedoraNamespace       = "packer-acc-clone-fedora"
	testFedoraSourceDVName    = "fedora-containerdisk"
	testFedoraSourceImage     = "quay.io/containerdisks/fedora:latest"
	testFedoraSourceDVSize    = "6Gi"
	testFedoraCloneName       = "fedora-clone-acc"
	testFedoraCloneInstance   = "u1.medium"
	testFedoraClonePreference = "fedora"
	testFedoraCloneSSHUser    = "fedora"
	testFedoraCloneSSHPass    = "password"

	testUbuntuNamespace       = "packer-acc-clone-ubuntu"
	testUbuntuSourceDVName    = "ubuntu-containerdisk"
	testUbuntuSourceImage     = "quay.io/containerdisks/ubuntu:24.04"
	testUbuntuSourceDVSize    = "6Gi"
	testUbuntuCloneName       = "ubuntu-clone-acc"
	testUbuntuCloneInstance   = "u1.medium"
	testUbuntuClonePreference = "ubuntu"
	testUbuntuCloneSSHUser    = "ubuntu"
	testUbuntuCloneSSHPass    = "password"
)

func testCloneTemplate(name, namespace, sourceDVName, diskSize, instanceType, preference, sshUser, sshPass string) string {
	return fmt.Sprintf(`
source "kubevirt-clone" "test" {
  kube_config = "%s"
  name        = "%s"
  namespace   = "%s"

  source_volume_name      = "%s"
  source_volume_namespace = "%s"

  disk_size     = "%s"
  instance_type = "%s"
  preference    = "%s"

  communicator = "ssh"
  ssh_username = "%s"
  ssh_password = "%s"
  ssh_timeout  = "10m"

  media_content = {
    "user-data" = <<-EOF
      #cloud-config
      chpasswd:
        expire: false
      password: %s
      user: %s
      ssh_pwauth: true
      sudo: ['ALL=(ALL) NOPASSWD:ALL']
    EOF
    "meta-data" = <<-EOF
      instance-id: %s
      local-hostname: %s
    EOF
  }
  media_label = "cidata"
}

build {
  sources = ["source.kubevirt-clone.test"]

  provisioner "shell" {
    inline = [
      "echo 'Acceptance test provisioner ran successfully'",
      "uname -a"
    ]
  }
}
`,
		os.Getenv("KUBECONFIG"),
		name,
		namespace,
		sourceDVName,
		namespace,
		diskSize,
		instanceType,
		preference,
		sshUser,
		sshPass,
		sshPass,
		sshUser,
		name,
		name,
	)
}

func runCloneAccTest(t *testing.T, testName, namespace, template, sourceDVName, sourceImage, sourceDVSize, cloneName, instanceType, preference string) {
	kubevirttest.RequireAcceptance(t)
	client := kubevirttest.KubevirtClient(t)

	t.Cleanup(func() {
		kubevirttest.CleanupBootableVolume(client, namespace, cloneName)
		kubevirttest.CleanupDataVolume(client, namespace, sourceDVName)
		kubevirttest.CleanupNamespace(client, namespace)
	})

	testCase := &acctest.PluginTestCase{
		Name:     testName,
		Template: template,
		Setup: func() error {
			ctx := context.Background()
			if err := kubevirttest.EnsureNamespace(ctx, client, namespace); err != nil {
				return err
			}
			return kubevirttest.EnsureRegistryDataVolume(ctx, client,
				namespace, sourceDVName, sourceImage, sourceDVSize,
			)
		},
		Check: func(buildCommand *exec.Cmd, logfile string) error {
			if buildCommand.ProcessState.ExitCode() != 0 {
				return fmt.Errorf("packer build failed with exit code %d, check logs at %s",
					buildCommand.ProcessState.ExitCode(), logfile)
			}

			if err := kubevirttest.CheckLogsFromProvisioner(logfile); err != nil {
				return err
			}
			if err := kubevirttest.CheckDataVolume(client, namespace, cloneName); err != nil {
				return err
			}
			if err := kubevirttest.CheckDataSourceLabels(client, namespace, cloneName, map[string]string{
				"instancetype.kubevirt.io/default-instancetype": instanceType,
				"instancetype.kubevirt.io/default-preference":   preference,
			}); err != nil {
				return err
			}
			if err := kubevirttest.CheckTemporaryVM(client, namespace, cloneName); err != nil {
				return err
			}
			if err := kubevirttest.CheckMediaConfigMap(client, namespace, cloneName); err != nil {
				return err
			}
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccCloneBuilder_Fedora(t *testing.T) {
	runCloneAccTest(t,
		"kubevirt_clone_fedora",
		testFedoraNamespace,
		testCloneTemplate(testFedoraCloneName, testFedoraNamespace, testFedoraSourceDVName, testDiskSize, testFedoraCloneInstance, testFedoraClonePreference, testFedoraCloneSSHUser, testFedoraCloneSSHPass),
		testFedoraSourceDVName, testFedoraSourceImage, testFedoraSourceDVSize,
		testFedoraCloneName, testFedoraCloneInstance, testFedoraClonePreference,
	)
}

func TestAccCloneBuilder_Ubuntu(t *testing.T) {
	runCloneAccTest(t,
		"kubevirt_clone_ubuntu",
		testUbuntuNamespace,
		testCloneTemplate(testUbuntuCloneName, testUbuntuNamespace, testUbuntuSourceDVName, testDiskSize, testUbuntuCloneInstance, testUbuntuClonePreference, testUbuntuCloneSSHUser, testUbuntuCloneSSHPass),
		testUbuntuSourceDVName, testUbuntuSourceImage, testUbuntuSourceDVSize,
		testUbuntuCloneName, testUbuntuCloneInstance, testUbuntuClonePreference,
	)
}
