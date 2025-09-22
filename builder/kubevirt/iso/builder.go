// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	ssh "golang.org/x/crypto/ssh"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"kubevirt.io/client-go/kubecli"
)

type Builder struct {
	config    Config
	runner    multistep.Runner
	client    kubecli.KubevirtClient
	clientset *kubernetes.Clientset
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec {
	return b.config.FlatMapstructure().HCL2Spec()
}

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	warnings, errs := b.config.Prepare(raws...)
	if errs != nil {
		return nil, warnings, errs
	}

	kubeConfig := b.config.KubeConfig
	if kubeConfig == "" {
		return nil, warnings, fmt.Errorf("KUBECONFIG environment variable is not set")
	}

	client, err := kubecli.GetKubevirtClientFromFlags("", kubeConfig)
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to get kubevirt client: %w", err)
	}
	b.client = client

	config, err := clientcmd.BuildConfigFromFlags("", kubeConfig)
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}
	b.clientset = clientset
	return nil, warnings, nil
}

func (b *Builder) Run(ctx context.Context, ui packer.Ui, hook packer.Hook) (packer.Artifact, error) {
	steps := []multistep.Step{}
	steps = append(steps,
		&StepValidateIsoDataVolume{
			Config: b.config,
			Client: b.client,
		},
		&StepCopyMediaFiles{
			Config: b.config,
			Client: b.clientset,
		},
		&StepCreateVirtualMachine{
			Config: b.config,
			Client: b.client,
		},
		&StepBootCommand{
			config: b.config,
			client: b.client,
		},
		&StepWaitForInstallation{
			Config: b.config,
		},
	)

	if b.config.Comm.Type == "ssh" {
		sshSteps, err := b.buildSSHSteps()
		if err != nil {
			ui.Errorf("SSH communicator config error: %v", err)
			return nil, nil
		}
		steps = append(steps, sshSteps...)
	}

	if b.config.Comm.Type == "winrm" {
		winRMSteps, err := b.buildWinRMSteps()
		if err != nil {
			ui.Errorf("WinRM communicator config error: %v", err)
			return nil, nil
		}
		steps = append(steps, winRMSteps...)
	}

	steps = append(steps,
		&StepStopVirtualMachine{
			Config: b.config,
			Client: b.client,
		},
		&StepCreateBootableVolume{
			Config: b.config,
			Client: b.client,
		},
	)

	state := new(multistep.BasicStateBag)
	state.Put("hook", hook)
	state.Put("ui", ui)

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	bootableVolumeName, ok := state.Get("bootable_volume_name").(string)
	if !ok || bootableVolumeName == "" {
		return nil, fmt.Errorf("bootable volume name not found in state")
	}
	return &Artifact{Name: bootableVolumeName}, nil
}

func (b *Builder) buildSSHSteps() ([]multistep.Step, []error) {
	commConfig := &communicator.Config{
		Type: b.config.Comm.Type,
		SSH: communicator.SSH{
			SSHHost:     b.config.Comm.SSHHost,
			SSHPort:     b.config.SSHLocalPort,
			SSHUsername: b.config.Comm.SSHUsername,
			SSHPassword: b.config.Comm.SSHPassword,
			SSHTimeout:  b.config.Comm.SSHTimeout,
		},
	}

	if err := commConfig.Prepare(&interpolate.Context{}); err != nil {
		return nil, err
	}

	steps := []multistep.Step{
		&StepStartPortForward{
			Config:        b.config,
			Client:        b.client,
			ForwarderFunc: DefaultPortForwarder,
		},
		&communicator.StepConnect{
			Config: commConfig,
			Host: func(state multistep.StateBag) (string, error) {
				return commConfig.SSH.SSHHost, nil
			},
			SSHConfig: func(state multistep.StateBag) (*ssh.ClientConfig, error) {
				return &ssh.ClientConfig{
					User: b.config.Comm.SSHUsername,
					Auth: []ssh.AuthMethod{
						ssh.Password(b.config.Comm.SSHPassword),
					},
					HostKeyCallback: ssh.InsecureIgnoreHostKey(),
				}, nil
			},
			SSHPort: func(state multistep.StateBag) (int, error) {
				return b.config.SSHLocalPort, nil
			},
		},
		&commonsteps.StepProvision{},
	}
	return steps, nil
}

func (b *Builder) buildWinRMSteps() ([]multistep.Step, []error) {
	commConfig := &communicator.Config{
		Type: b.config.Comm.Type,
		WinRM: communicator.WinRM{
			WinRMHost:     b.config.Comm.WinRMHost,
			WinRMPort:     b.config.WinRMLocalPort,
			WinRMUser:     b.config.Comm.WinRMUser,
			WinRMPassword: b.config.Comm.WinRMPassword,
			WinRMTimeout:  b.config.Comm.WinRMTimeout,
		},
	}

	if err := commConfig.Prepare(&interpolate.Context{}); err != nil {
		return nil, err
	}

	steps := []multistep.Step{
		&StepStartPortForward{
			Config:        b.config,
			Client:        b.client,
			ForwarderFunc: DefaultPortForwarder,
		},
		&communicator.StepConnect{
			Config: commConfig,
			Host: func(state multistep.StateBag) (string, error) {
				return commConfig.WinRMHost, nil
			},
			WinRMConfig: func(state multistep.StateBag) (*communicator.WinRMConfig, error) {
				return &communicator.WinRMConfig{
					Username: b.config.Comm.WinRMUser,
					Password: b.config.Comm.WinRMPassword,
				}, nil
			},
			WinRMPort: func(state multistep.StateBag) (int, error) {
				return b.config.WinRMLocalPort, nil
			},
		},
		&commonsteps.StepProvision{},
	}
	return steps, nil
}
