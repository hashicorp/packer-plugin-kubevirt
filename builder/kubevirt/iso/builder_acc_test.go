// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

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
	testDiskSize = "10Gi"

	testFedoraNamespace    = "packer-acc-fedora"
	testFedoraISOName      = "fedora-42-x86-64-iso"
	testFedoraISOURL       = "https://download.fedoraproject.org/pub/fedora/linux/releases/42/Server/x86_64/iso/Fedora-Server-dvd-x86_64-42-1.1.iso"
	testFedoraISOSize      = "3Gi"
	testFedoraImageName    = "fedora-acc-test"
	testFedoraInstanceType = "o1.medium"
	testFedoraPreference   = "fedora"
	testFedoraSSHUsername  = "user"
	testFedoraSSHPassword  = "root"

	testUbuntuNamespace    = "packer-acc-ubuntu"
	testUbuntuISOName      = "ubuntu-2404-x86-64-iso"
	testUbuntuISOURL       = "https://releases.ubuntu.com/noble/ubuntu-24.04.5-live-server-amd64.iso"
	testUbuntuISOSize      = "10Gi"
	testUbuntuImageName    = "ubuntu-acc-test"
	testUbuntuInstanceType = "o1.medium"
	testUbuntuPreference   = "ubuntu"
	testUbuntuSSHUsername  = "ubuntu"
	testUbuntuSSHPassword  = "ubuntu"

	testWindowsNamespace    = "packer-acc-windows"
	testWindowsISOName      = "windows-11-x86-64-iso"
	testWindowsISOURL       = "https://software-static.download.prss.microsoft.com/dbazure/888969d5-f34g-4e03-ac9d-1f9786c66749/26200.6584.250915-1905.25h2_ge_release_svc_refresh_CLIENT_CONSUMER_x64FRE_en-us.iso"
	testWindowsISOSize      = "8Gi"
	testWindowsDiskSize     = "64Gi"
	testWindowsImageName    = "windows-acc-test"
	testWindowsInstanceType = "u1.large"
	testWindowsPreference   = "windows.11.virtio"
	testWindowsWinRMUser    = "Administrator"
	testWindowsWinRMPass    = "shadowman"
)

func testFedoraTemplate(t *testing.T) string {
	return fmt.Sprintf(`
source "kubevirt-iso" "fedora" {
  kube_config     = "%s"
  name            = "%s"
  namespace       = "%s"
  iso_volume_name = "%s"

  disk_size     = "%s"
  instance_type = "%s"
  preference    = "%s"
  os_type       = "linux"

  media_content = {
    "ks.cfg" = <<-EOF
%s
    EOF
  }

  boot_command = [
    "<up>e",
    "<down><down><end>",
    " inst.ks=hd:LABEL=OEMDRV:/ks.cfg",
    "<leftCtrlOn>x<leftCtrlOff>"
  ]
  boot_wait                 = "10s"
  installation_wait_timeout = "20m"

  communicator = "ssh"
  ssh_username = "%s"
  ssh_password = "%s"
  ssh_timeout  = "20m"
}

build {
  sources = ["source.kubevirt-iso.fedora"]

  provisioner "shell" {
    inline = [
      "echo 'Acceptance test provisioner ran successfully'"
    ]
  }
}
`,
		os.Getenv("KUBECONFIG"),
		testFedoraImageName,
		testFedoraNamespace,
		testFedoraISOName,
		testDiskSize,
		testFedoraInstanceType,
		testFedoraPreference,
		kubevirttest.ReadTestData(t, "ks.cfg"),
		testFedoraSSHUsername,
		testFedoraSSHPassword,
	)
}

func testUbuntuTemplate(t *testing.T) string {
	return fmt.Sprintf(`
source "kubevirt-iso" "ubuntu" {
  kube_config     = "%s"
  name            = "%s"
  namespace       = "%s"
  iso_volume_name = "%s"

  disk_size     = "%s"
  instance_type = "%s"
  preference    = "%s"
  os_type       = "linux"

  media_content = {
    "user-data" = <<-EOF
%s
    EOF
    "meta-data" = <<-EOF
%s
    EOF
  }
  media_label = "cidata"

  boot_command = [
    "<wait>c<wait>",
    "linux /casper/vmlinuz autoinstall ds=nocloud ---<enter>",
    "initrd /casper/initrd<enter>",
    "boot<enter>"
  ]
  boot_wait                 = "10s"
  installation_wait_timeout = "20m"

  communicator = "ssh"
  ssh_username = "%s"
  ssh_password = "%s"
  ssh_timeout  = "20m"
}

build {
  sources = ["source.kubevirt-iso.ubuntu"]

  provisioner "shell" {
    inline = [
      "echo 'Acceptance test provisioner ran successfully'"
    ]
  }
}
`,
		os.Getenv("KUBECONFIG"),
		testUbuntuImageName,
		testUbuntuNamespace,
		testUbuntuISOName,
		testDiskSize,
		testUbuntuInstanceType,
		testUbuntuPreference,
		kubevirttest.ReadTestData(t, "user-data"),
		kubevirttest.ReadTestData(t, "meta-data"),
		testUbuntuSSHUsername,
		testUbuntuSSHPassword,
	)
}

func testWindowsTemplate(t *testing.T) string {
	return fmt.Sprintf(`
source "kubevirt-iso" "windows" {
  kube_config     = "%s"
  name            = "%s"
  namespace       = "%s"
  iso_volume_name = "%s"

  disk_size     = "%s"
  instance_type = "%s"
  preference    = "%s"
  os_type       = "windows"

  media_content = {
    "autounattend.xml" = <<-EOF
%s
    EOF
    "install-misc.ps1" = <<-EOF
%s
    EOF
    "set-network.ps1" = <<-EOF
%s
    EOF
    "enable-winrm.ps1" = <<-EOF
%s
    EOF
  }

  boot_command = [
    "<spacebar><wait>"
  ]
  boot_wait                 = "5s"
  installation_wait_timeout = "20m"

  communicator   = "winrm"
  winrm_username = "%s"
  winrm_password = "%s"
  winrm_timeout  = "25m"
}

build {
  sources = ["source.kubevirt-iso.windows"]

  provisioner "powershell" {
    inline = [
      "Write-Output 'Acceptance test provisioner ran successfully'"
    ]
  }

  provisioner "windows-shell" {
    inline = [
      "C:\\Windows\\System32\\Sysprep\\sysprep.exe /generalize /oobe /shutdown /mode:vm"
    ]
  }
}
`,
		os.Getenv("KUBECONFIG"),
		testWindowsImageName,
		testWindowsNamespace,
		testWindowsISOName,
		testWindowsDiskSize,
		testWindowsInstanceType,
		testWindowsPreference,
		kubevirttest.ReadTestData(t, "autounattend.xml"),
		kubevirttest.ReadTestData(t, "install-misc.ps1"),
		kubevirttest.ReadTestData(t, "set-network.ps1"),
		kubevirttest.ReadTestData(t, "enable-winrm.ps1"),
		testWindowsWinRMUser,
		testWindowsWinRMPass,
	)
}

func runISOAccTest(t *testing.T, name, namespace, template, isoName, isoURL, isoSize, imageName, instanceType, preference string) {
	kubevirttest.RequireAcceptance(t)
	client := kubevirttest.KubevirtClient(t)

	t.Cleanup(func() {
		kubevirttest.CleanupBootableVolume(client, namespace, imageName)
		kubevirttest.CleanupDataVolume(client, namespace, isoName)
		kubevirttest.CleanupNamespace(client, namespace)
	})

	testCase := &acctest.PluginTestCase{
		Name:     name,
		Template: template,
		Setup: func() error {
			ctx := context.Background()
			if err := kubevirttest.EnsureNamespace(ctx, client, namespace); err != nil {
				return err
			}
			return kubevirttest.EnsureISODataVolume(ctx, client,
				namespace, isoName, isoURL, isoSize,
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
			if err := kubevirttest.CheckDataVolume(client, namespace, imageName); err != nil {
				return err
			}
			if err := kubevirttest.CheckDataSourceLabels(client, namespace, imageName, map[string]string{
				"instancetype.kubevirt.io/default-instancetype": instanceType,
				"instancetype.kubevirt.io/default-preference":   preference,
			}); err != nil {
				return err
			}
			if err := kubevirttest.CheckTemporaryVM(client, namespace, imageName); err != nil {
				return err
			}
			if err := kubevirttest.CheckMediaConfigMap(client, namespace, imageName); err != nil {
				return err
			}

			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccISOBuilder_Fedora(t *testing.T) {
	runISOAccTest(t,
		"kubevirt_iso_fedora",
		testFedoraNamespace,
		testFedoraTemplate(t),
		testFedoraISOName, testFedoraISOURL, testFedoraISOSize,
		testFedoraImageName, testFedoraInstanceType, testFedoraPreference,
	)
}

func TestAccISOBuilder_Ubuntu(t *testing.T) {
	runISOAccTest(t,
		"kubevirt_iso_ubuntu",
		testUbuntuNamespace,
		testUbuntuTemplate(t),
		testUbuntuISOName, testUbuntuISOURL, testUbuntuISOSize,
		testUbuntuImageName, testUbuntuInstanceType, testUbuntuPreference,
	)
}

func TestAccISOBuilder_Windows(t *testing.T) {
	runISOAccTest(t,
		"kubevirt_iso_windows",
		testWindowsNamespace,
		testWindowsTemplate(t),
		testWindowsISOName, testWindowsISOURL, testWindowsISOSize,
		testWindowsImageName, testWindowsInstanceType, testWindowsPreference,
	)
}
