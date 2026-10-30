// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/acctest"

	kubevirttest "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"

	"kubevirt.io/client-go/kubecli"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

const (
	testDiskSize = "10Gi"

	testFedoraNamespace    = "packer-acc-fedora"
	testFedoraISOName      = "fedora-45-x86-64-iso"
	testFedoraISOURL       = "https://download.fedoraproject.org/pub/fedora/linux/releases/45/Server/x86_64/iso/Fedora-Server-dvd-x86_64-45-1.4.iso"
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
		strings.Replace(kubevirttest.ReadTestData(t, "autounattend.xml"), "<AcceptEula>false</AcceptEula>", "<AcceptEula>true</AcceptEula>", 1),
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

			if err := checkLogsFromProvisioner(logfile); err != nil {
				return err
			}
			if err := checkDataVolume(client, namespace, imageName); err != nil {
				return err
			}
			if err := checkDataSourceLabels(client, namespace, imageName, map[string]string{
				"instancetype.kubevirt.io/default-instancetype": instanceType,
				"instancetype.kubevirt.io/default-preference":   preference,
			}); err != nil {
				return err
			}
			if err := checkTemporaryVM(client, namespace, imageName); err != nil {
				return err
			}
			if err := checkMediaConfigMap(client, namespace, imageName); err != nil {
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
	if os.Getenv("PACKER_ACC_WINDOWS_ACCEPT_EULA") != "true" {
		t.Skip("Set PACKER_ACC_WINDOWS_ACCEPT_EULA=true to accept the Microsoft EULA and run this test")
	}
	runISOAccTest(t,
		"kubevirt_iso_windows",
		testWindowsNamespace,
		testWindowsTemplate(t),
		testWindowsISOName, testWindowsISOURL, testWindowsISOSize,
		testWindowsImageName, testWindowsInstanceType, testWindowsPreference,
	)
}

func checkLogsFromProvisioner(logfile string) error {
	logBytes, err := os.ReadFile(logfile)
	if err != nil {
		return fmt.Errorf("failed to read build log: %w", err)
	}
	if !strings.Contains(string(logBytes), "Acceptance test provisioner ran successfully") {
		return fmt.Errorf("expected provisioner output not found in build log")
	}
	return nil
}

func checkDataVolume(client kubecli.KubevirtClient, namespace, name string) error {
	dataVolume, err := client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get DataVolume %s/%s: %w", namespace, name, err)
	}
	if dataVolume.Status.Phase != cdiv1.Succeeded {
		return fmt.Errorf("expected DataVolume %s/%s phase to be Succeeded, got %s", namespace, name, dataVolume.Status.Phase)
	}
	return nil
}

func checkDataSourceLabels(client kubecli.KubevirtClient, namespace, name string, labels map[string]string) error {
	dataSource, err := client.CdiClient().CdiV1beta1().DataSources(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get DataSource %s/%s: %w", namespace, name, err)
	}
	for key, value := range labels {
		if dataSource.Labels[key] != value {
			return fmt.Errorf("expected DataSource %s/%s label %s to be %s, got %s", namespace, name, key, value, dataSource.Labels[key])
		}
	}
	return nil
}

func checkTemporaryVM(client kubecli.KubevirtClient, namespace, name string) error {
	pollInterval := 5 * time.Second
	pollTimeout := 2 * time.Minute
	poller := func(ctx context.Context) (bool, error) {
		_, err := client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	}
	if err := wait.PollUntilContextTimeout(context.Background(), pollInterval, pollTimeout, true, poller); err != nil {
		return fmt.Errorf("expected temporary VM %s/%s to be deleted after build: %w", namespace, name, err)
	}
	return nil
}

func checkMediaConfigMap(client kubecli.KubevirtClient, namespace, name string) error {
	_, err := client.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		return fmt.Errorf("expected media ConfigMap %s/%s to be deleted after build", namespace, name)
	}
	return nil
}
