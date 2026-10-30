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

	"github.com/hashicorp/packer-plugin-sdk/acctest"

	kubevirttest "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/testing"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"kubevirt.io/client-go/kubecli"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
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
	testWindowsDiskSize     = "32Gi"
	testWindowsImageName    = "windows-acc-test"
	testWindowsInstanceType = "u1.large"
	testWindowsPreference   = "windows.11.virtio"
	testWindowsWinRMUser    = "Administrator"
	testWindowsWinRMPass    = "shadowman"
)

const testKickstart = `cdrom
text
firstboot --disable
lang en_US.UTF-8
keyboard us
timezone Europe/Paris --utc
selinux --enforcing
rootpw root
firewall --enabled --ssh
network --bootproto dhcp
user --groups=wheel --name=user --password=root --uid=1000 --gecos="user" --gid=1000

bootloader --location=mbr --append="net.ifnames=0 biosdevname=0 crashkernel=no"

zerombr
clearpart --all --initlabel
autopart --type=lvm

poweroff

%packages --excludedocs
@core
qemu-guest-agent
openssh-server
%end

%post
systemctl enable --now sshd
systemctl enable --now qemu-guest-agent
%end
`

const testUserData = `#cloud-config
autoinstall:
  version: 1
  locale: en_US.UTF-8
  keyboard:
    layout: us
  timezone: Europe/Paris
  identity:
    hostname: ubuntu
    username: ubuntu
    password: "$6$kubevirtpk$bK.ydp07YBZfv8LErv4f.QhWy.U5dlLSR/Bkr8tcoJHZgRddygFxnYcKPBGBXvzgQr6lUM3v9SjjUHVklh64g1"
  ssh:
    install-server: true
    allow-pw: true
  storage:
    layout:
      name: lvm
  packages:
    - qemu-guest-agent
  late-commands:
    - curtin in-target --target=/target -- systemctl enable qemu-guest-agent
  shutdown: reboot
`

const testMetaData = `instance-id: ubuntu-2404
local-hostname: ubuntu
`

const testAutounattend = `<?xml version="1.0" encoding="utf-8"?>
<unattend xmlns="urn:schemas-microsoft-com:unattend" xmlns:wcm="http://schemas.microsoft.com/WMIConfig/2002/State">
  <settings pass="windowsPE">
    <component name="Microsoft-Windows-International-Core-WinPE" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <SetupUILanguage>
        <UILanguage>en-US</UILanguage>
      </SetupUILanguage>
      <InputLocale>0409:00000409</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
    <component name="Microsoft-Windows-PnpCustomizationsWinPE" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <DriverPaths>
        <PathAndCredentials wcm:action="add" wcm:keyValue="1">
          <Path>E:\viostor\w11\amd64</Path>
        </PathAndCredentials>
        <PathAndCredentials wcm:action="add" wcm:keyValue="2">
          <Path>E:\NetKVM\w11\amd64</Path>
        </PathAndCredentials>
      </DriverPaths>
    </component>
    <component name="Microsoft-Windows-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <DiskConfiguration>
        <WillShowUI>Never</WillShowUI>
        <Disk wcm:action="add">
          <CreatePartitions>
              <CreatePartition wcm:action="add">
                  <Order>1</Order>
                  <Type>Primary</Type>
                  <Size>700</Size>
              </CreatePartition>
              <CreatePartition wcm:action="add">
                  <Order>2</Order>
                  <Type>EFI</Type>
                  <Size>100</Size>
              </CreatePartition>
              <CreatePartition wcm:action="add">
                  <Order>3</Order>
                  <Type>MSR</Type>
                  <Size>128</Size>
              </CreatePartition>
              <CreatePartition wcm:action="add">
                  <Order>4</Order>
                  <Type>Primary</Type>
                  <Extend>true</Extend>
              </CreatePartition>
          </CreatePartitions>
          <ModifyPartitions>
            <ModifyPartition wcm:action="add">
                <Order>1</Order>
                <PartitionID>1</PartitionID>
                <Label>WINRE</Label>
                <Format>NTFS</Format>
                <TypeID>DE94BBA4-06D1-4D40-A16A-BFD50179D6AC</TypeID>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
                <Order>2</Order>
                <PartitionID>2</PartitionID>
                <Label>EFI</Label>
                <Format>FAT32</Format>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
                <Order>3</Order>
                <PartitionID>3</PartitionID>
            </ModifyPartition>
            <ModifyPartition wcm:action="add">
                <Order>4</Order>
                <PartitionID>4</PartitionID>
                <Label>Windows</Label>
                <Letter>C</Letter>
                <Format>NTFS</Format>
            </ModifyPartition>
          </ModifyPartitions>
          <DiskID>0</DiskID>
          <WillWipeDisk>true</WillWipeDisk>
        </Disk>
      </DiskConfiguration>
      <ImageInstall>
        <OSImage>
          <InstallFrom>
            <MetaData wcm:action="add">
              <Key>/Image/Description</Key>
              <Value>Windows 11 Pro</Value>
            </MetaData>
          </InstallFrom>
          <InstallTo>
            <DiskID>0</DiskID>
            <PartitionID>4</PartitionID>
          </InstallTo>
        </OSImage>
      </ImageInstall>
      <UserData>
        <AcceptEula>true</AcceptEula>
        <ProductKey>
          <Key/>
        </ProductKey>
      </UserData>
    </component>
  </settings>
  <settings pass="specialize">
    <component name="Microsoft-Windows-International-Core" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <InputLocale>0409:00000409</InputLocale>
      <SystemLocale>en-US</SystemLocale>
      <UILanguage>en-US</UILanguage>
      <UserLocale>en-US</UserLocale>
    </component>
  </settings>
  <settings pass="oobeSystem">
    <component name="Microsoft-Windows-Deployment" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <Reseal>
        <Mode>Audit</Mode>
      </Reseal>
    </component>
    <component name="Microsoft-Windows-Shell-Setup" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <UserAccounts>
        <AdministratorPassword>
          <Value>shadowman</Value>
          <PlainText>true</PlainText>
        </AdministratorPassword>
      </UserAccounts>
      <AutoLogon>
        <Password>
          <Value>shadowman</Value>
          <PlainText>true</PlainText>
        </Password>
        <Username>Administrator</Username>
        <Enabled>true</Enabled>
      </AutoLogon>
    </component>
  </settings>
  <settings pass="auditUser">
    <component name="Microsoft-Windows-Deployment" processorArchitecture="amd64" publicKeyToken="31bf3856ad364e35" language="neutral" versionScope="nonSxS">
      <RunSynchronous>
        <RunSynchronousCommand wcm:action="add">
          <Order>1</Order>
          <Description>Install Misc (e.g. QEMU)</Description>
          <Path>PowerShell -ExecutionPolicy Bypass -NoProfile F:\install-misc.ps1</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>2</Order>
          <Description>Fix Network Category</Description>
          <Path>PowerShell -ExecutionPolicy Bypass -NoProfile F:\set-network.ps1</Path>
        </RunSynchronousCommand>
        <RunSynchronousCommand wcm:action="add">
          <Order>3</Order>
          <Description>Enable WinRM</Description>
          <Path>PowerShell -ExecutionPolicy Bypass -NoProfile F:\enable-winrm.ps1</Path>
        </RunSynchronousCommand>
      </RunSynchronous>
    </component>
  </settings>
</unattend>
`

const testInstallMisc = `Start-Process msiexec -Wait -ArgumentList "/i E:\virtio-win-gt-x64.msi /qn /passive /norestart"
Start-Process msiexec -Wait -ArgumentList "/i E:\guest-agent\qemu-ga-x86_64.msi /qn /passive /norestart"
mv C:\Windows\Panther\unattend.xml C:\Windows\Panther\unattend.install.xml
`

const testSetNetwork = `$profile = Get-NetConnectionProfile
Set-NetConnectionProfile -InterfaceIndex $profile.InterfaceIndex -NetworkCategory Private
`

const testEnableWinRM = `Enable-PSRemoting -Force
Enable-WSManCredSSP -Role Server -Force
Set-Item -Path WSMan:\localhost\Service\Auth\Basic -Value $true
Set-Item -Path WSMan:\localhost\Service\AllowUnencrypted -Value $true
New-NetFirewallRule -Name "WinRM_HTTP" -DisplayName "WinRM over HTTP" -Protocol TCP -LocalPort 5985 -Action Allow
`

func testFedoraTemplate() string {
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
		testKickstart,
		testFedoraSSHUsername,
		testFedoraSSHPassword,
	)
}

func testUbuntuTemplate() string {
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
		testUserData,
		testMetaData,
		testUbuntuSSHUsername,
		testUbuntuSSHPassword,
	)
}

func testWindowsTemplate() string {
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
		testAutounattend,
		testInstallMisc,
		testSetNetwork,
		testEnableWinRM,
		testWindowsWinRMUser,
		testWindowsWinRMPass,
	)
}

func runISOAccTest(t *testing.T, name, namespace, template, isoName, isoURL, isoSize, imageName, instanceType, preference string) {
	kubevirttest.RequireAcceptance(t)
	client := kubevirttest.KubevirtClient(t)

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
		Teardown: func() error {
			kubevirttest.CleanupBootableVolume(client, namespace, imageName)
			kubevirttest.CleanupDataVolume(client, namespace, isoName)
			kubevirttest.CleanupNamespace(client, namespace)
			return nil
		},
	}
	acctest.TestPlugin(t, testCase)
}

func TestAccISOBuilder_Fedora(t *testing.T) {
	runISOAccTest(t,
		"kubevirt_iso_fedora",
		testFedoraNamespace,
		testFedoraTemplate(),
		testFedoraISOName, testFedoraISOURL, testFedoraISOSize,
		testFedoraImageName, testFedoraInstanceType, testFedoraPreference,
	)
}

func TestAccISOBuilder_Ubuntu(t *testing.T) {
	runISOAccTest(t,
		"kubevirt_iso_ubuntu",
		testUbuntuNamespace,
		testUbuntuTemplate(),
		testUbuntuISOName, testUbuntuISOURL, testUbuntuISOSize,
		testUbuntuImageName, testUbuntuInstanceType, testUbuntuPreference,
	)
}

func TestAccISOBuilder_Windows(t *testing.T) {
	runISOAccTest(t,
		"kubevirt_iso_windows",
		testWindowsNamespace,
		testWindowsTemplate(),
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
	if dataVolume.Status.Phase != cdiv1.DataVolumePhase(cdiv1.Succeeded) {
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
	_, err := client.VirtualMachine(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		return fmt.Errorf("expected temporary VM %s/%s to be deleted after build", namespace, name)
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
