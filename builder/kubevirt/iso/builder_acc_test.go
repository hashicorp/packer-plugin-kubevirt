// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"text/template"

	"github.com/hashicorp/packer-plugin-sdk/acctest"

	kubevirttest "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/testing"
)

const (
	testDiskSize = "10Gi"

	testFedoraNamespacePrefix = "packer-acc-fedora-"
	testFedoraISOName         = "fedora-45-x86-64-iso"
	testFedoraISOURL          = "https://download.fedoraproject.org/pub/fedora/linux/releases/45/Server/x86_64/iso/Fedora-Server-dvd-x86_64-45-1.4.iso"
	testFedoraISOSize         = "5Gi"
	testFedoraImageName       = "fedora-acc-test"
	testFedoraInstanceType    = "o1.medium"
	testFedoraPreference      = "fedora"
	testFedoraSSHUsername     = "user"
	testFedoraSSHPassword     = "root"

	testUbuntuNamespacePrefix = "packer-acc-ubuntu-"
	testUbuntuISOName         = "ubuntu-2404-x86-64-iso"
	testUbuntuISOURL          = "https://releases.ubuntu.com/noble/ubuntu-24.04.5-live-server-amd64.iso"
	testUbuntuISOSize         = "10Gi"
	testUbuntuImageName       = "ubuntu-acc-test"
	testUbuntuInstanceType    = "o1.medium"
	testUbuntuPreference      = "ubuntu"
	testUbuntuSSHUsername     = "ubuntu"
	testUbuntuSSHPassword     = "ubuntu"

	testWindowsNamespacePrefix = "packer-acc-windows-"
	testWindowsISOName         = "windows-11-x86-64-iso"
	testWindowsISOSize         = "8Gi"
	testWindowsDiskSize        = "64Gi"
	testWindowsImageName       = "windows-acc-test"
	testWindowsInstanceType    = "u1.large"
	testWindowsPreference      = "windows.11.virtio"
	testWindowsWinRMUser       = "Administrator"
	testWindowsWinRMPass       = "shadowman"
)

const fedoraTemplateHCL = `
source "kubevirt-iso" "fedora" {
  kube_config     = "{{ .KubeConfig }}"
  name            = "{{ .Name }}"
  namespace       = "{{ .Namespace }}"
  iso_volume_name = "{{ .ISOName }}"

  disk_size     = "{{ .DiskSize }}"
  instance_type = "{{ .InstanceType }}"
  preference    = "{{ .Preference }}"
  os_type       = "linux"

  networks {
    name = "default"
    pod {}
  }

  media_content = {
  {{- range $name, $content := .MediaContent }}
    "{{ $name }}" = <<-EOF
{{ $content }}
    EOF
  {{- end }}
  }

  # No boot_command: Anaconda auto-detects ks.cfg on the OEMDRV-labeled
  installation_wait_timeout = "20m"

  communicator = "ssh"
  ssh_username = "{{ .SSHUsername }}"
  ssh_password = "{{ .SSHPassword }}"
  ssh_timeout  = "1h"
}

build {
  sources = ["source.kubevirt-iso.fedora"]

  provisioner "shell" {
    inline = [
      "echo 'Acceptance test provisioner ran successfully'"
    ]
  }
}
`

const ubuntuTemplateHCL = `
source "kubevirt-iso" "ubuntu" {
  kube_config     = "{{ .KubeConfig }}"
  name            = "{{ .Name }}"
  namespace       = "{{ .Namespace }}"
  iso_volume_name = "{{ .ISOName }}"

  disk_size     = "{{ .DiskSize }}"
  instance_type = "{{ .InstanceType }}"
  preference    = "{{ .Preference }}"
  os_type       = "linux"

  networks {
    name = "default"
    pod {}
  }

  media_content = {
  {{- range $name, $content := .MediaContent }}
    "{{ $name }}" = <<-EOF
{{ $content }}
    EOF
  {{- end }}
  }
  media_label = "cidata"

  # subiquity loads the autoinstall config from the cidata seed but, without
  # the "autoinstall" kernel arg, waits at a "Continue with autoinstall?
  # (yes|no)" confirmation that blocks indefinitely. Rather than race GRUB
  # timing to inject the kernel arg, we answer "yes". The answers start early
  # and repeat every 30s for two reasons: KubeVirt drops an idle VNC
  # connection after ~2 min (so we must keep sending keystrokes), and the
  # exact time subiquity reaches the prompt varies. The burst ends well before
  # the install completes and reboots, which would drop VNC mid-type.
  boot_command = [
    "<wait30>yes<enter>", "<wait30>yes<enter>", "<wait30>yes<enter>",
    "<wait30>yes<enter>", "<wait30>yes<enter>", "<wait30>yes<enter>"
  ]
  boot_wait                 = "5s"
  installation_wait_timeout = "20m"

  communicator = "ssh"
  ssh_username = "{{ .SSHUsername }}"
  ssh_password = "{{ .SSHPassword }}"
  ssh_timeout  = "1h"
}

build {
  sources = ["source.kubevirt-iso.ubuntu"]

  provisioner "shell" {
    inline = [
      "echo 'Acceptance test provisioner ran successfully'"
    ]
  }
}
`

const windowsTemplateHCL = `
source "kubevirt-iso" "windows" {
  kube_config     = "{{ .KubeConfig }}"
  name            = "{{ .Name }}"
  namespace       = "{{ .Namespace }}"
  iso_volume_name = "{{ .ISOName }}"

  disk_size     = "{{ .DiskSize }}"
  instance_type = "{{ .InstanceType }}"
  preference    = "{{ .Preference }}"
  os_type       = "windows"

  networks {
    name = "default"
    pod {}
  }

  media_content = {
  {{- range $name, $content := .MediaContent }}
    "{{ $name }}" = <<-EOF
{{ $content }}
    EOF
  {{- end }}
  }

  # Windows' "Press any key to boot from CD..." prompt appears only ~3-6s into
  # OVMF's boot and is gone before a single late keypress can land, so OVMF
  # drops into its firmware menu. Start tapping space very early (low boot_wait)
  # and densely (every 500ms) so a keypress lands inside that brief window.
  boot_command = [
    "<spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms>",
    "<spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms>",
    "<spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms>",
    "<spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms>",
    "<spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms>",
    "<spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms><spacebar><wait500ms>"
  ]
  boot_wait                 = "1s"
  installation_wait_timeout = "40m"

  communicator   = "winrm"
  winrm_username = "{{ .WinRMUsername }}"
  winrm_password = "{{ .WinRMPassword }}"
  winrm_timeout  = "1h"
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
`

// isoTemplateData holds the values interpolated into an acceptance-test
// template. Each template only references the fields relevant to it.
type isoTemplateData struct {
	KubeConfig    string
	Name          string
	Namespace     string
	ISOName       string
	DiskSize      string
	InstanceType  string
	Preference    string
	SSHUsername   string
	SSHPassword   string
	WinRMUsername string
	WinRMPassword string
	// MediaContent maps each media file name to its content. It is rendered as
	// the media_content block, with the keys emitted in sorted order.
	MediaContent map[string]string
}

// renderISOTemplate executes the named template against data and returns the
// resulting Packer HCL. Media content is injected as template field values, so
// content that itself contains "{{" (e.g. cloud-init) is passed through as is.
func renderISOTemplate(t *testing.T, name, tmpl string, data isoTemplateData) string {
	t.Helper()
	parsed, err := template.New(name).Parse(tmpl)
	if err != nil {
		t.Fatalf("failed to parse %s template: %v", name, err)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		t.Fatalf("failed to render %s template: %v", name, err)
	}
	return buf.String()
}

func testFedoraTemplate(t *testing.T, namespace string) string {
	return renderISOTemplate(t, "fedora", fedoraTemplateHCL, isoTemplateData{
		KubeConfig:   os.Getenv("KUBECONFIG"),
		Name:         testFedoraImageName,
		Namespace:    namespace,
		ISOName:      testFedoraISOName,
		DiskSize:     testDiskSize,
		InstanceType: testFedoraInstanceType,
		Preference:   testFedoraPreference,
		SSHUsername:  testFedoraSSHUsername,
		SSHPassword:  testFedoraSSHPassword,
		MediaContent: map[string]string{
			"ks.cfg": kubevirttest.ReadTestData(t, "ks.cfg"),
		},
	})
}

func testUbuntuTemplate(t *testing.T, namespace string) string {
	return renderISOTemplate(t, "ubuntu", ubuntuTemplateHCL, isoTemplateData{
		KubeConfig:   os.Getenv("KUBECONFIG"),
		Name:         testUbuntuImageName,
		Namespace:    namespace,
		ISOName:      testUbuntuISOName,
		DiskSize:     testDiskSize,
		InstanceType: testUbuntuInstanceType,
		Preference:   testUbuntuPreference,
		SSHUsername:  testUbuntuSSHUsername,
		SSHPassword:  testUbuntuSSHPassword,
		MediaContent: map[string]string{
			"user-data": kubevirttest.ReadTestData(t, "user-data"),
			"meta-data": kubevirttest.ReadTestData(t, "meta-data"),
		},
	})
}

func testWindowsTemplate(t *testing.T, namespace string) string {
	return renderISOTemplate(t, "windows", windowsTemplateHCL, isoTemplateData{
		KubeConfig:    os.Getenv("KUBECONFIG"),
		Name:          testWindowsImageName,
		Namespace:     namespace,
		ISOName:       testWindowsISOName,
		DiskSize:      testWindowsDiskSize,
		InstanceType:  testWindowsInstanceType,
		Preference:    testWindowsPreference,
		WinRMUsername: testWindowsWinRMUser,
		WinRMPassword: testWindowsWinRMPass,
		MediaContent: map[string]string{
			"autounattend.xml": strings.Replace(kubevirttest.ReadTestData(t, "autounattend.xml"), "<AcceptEula>false</AcceptEula>", "<AcceptEula>true</AcceptEula>", 1),
			"install-misc.ps1": kubevirttest.ReadTestData(t, "install-misc.ps1"),
			"set-network.ps1":  kubevirttest.ReadTestData(t, "set-network.ps1"),
			"enable-winrm.ps1": kubevirttest.ReadTestData(t, "enable-winrm.ps1"),
		},
	})
}

func runISOAccTest(t *testing.T, name, namespace, template, isoName, isoURL, isoSize, imageName, instanceType, preference string) {
	kubevirttest.RequireAcceptance(t)
	client := kubevirttest.KubevirtClient(t)

	t.Cleanup(func() {
		kubevirttest.DumpNamespaceOnFailure(t, client, namespace)
		kubevirttest.CleanupBootableVolume(client, namespace, imageName)
		kubevirttest.CleanupDataVolume(client, namespace, isoName)
		kubevirttest.CleanupNamespace(client, namespace)
	})

	testCase := &acctest.PluginTestCase{
		// Keep the resources of a failed build so that DumpNamespaceOnFailure can
		// collect them; t.Cleanup deletes the namespace afterwards anyway.
		BuildExtraArgs: []string{"-on-error=abort"},
		Name:           name,
		Template:       template,
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
	namespace := kubevirttest.RandomNamespace(testFedoraNamespacePrefix)
	runISOAccTest(t,
		"kubevirt_iso_fedora",
		namespace,
		testFedoraTemplate(t, namespace),
		testFedoraISOName, testFedoraISOURL, testFedoraISOSize,
		testFedoraImageName, testFedoraInstanceType, testFedoraPreference,
	)
}

func TestAccISOBuilder_Ubuntu(t *testing.T) {
	namespace := kubevirttest.RandomNamespace(testUbuntuNamespacePrefix)
	runISOAccTest(t,
		"kubevirt_iso_ubuntu",
		namespace,
		testUbuntuTemplate(t, namespace),
		testUbuntuISOName, testUbuntuISOURL, testUbuntuISOSize,
		testUbuntuImageName, testUbuntuInstanceType, testUbuntuPreference,
	)
}

func TestAccISOBuilder_Windows(t *testing.T) {
	if os.Getenv("PACKER_ACC_WINDOWS_ACCEPT_EULA") != "true" {
		t.Skip("Set PACKER_ACC_WINDOWS_ACCEPT_EULA=true to accept the Microsoft EULA and run this test")
	}

	namespace := kubevirttest.RandomNamespace(testWindowsNamespacePrefix)
	runISOAccTest(t,
		"kubevirt_iso_windows",
		namespace,
		testWindowsTemplate(t, namespace),
		testWindowsISOName, kubevirttest.WindowsISOURL(), testWindowsISOSize,
		testWindowsImageName, testWindowsInstanceType, testWindowsPreference,
	)
}
