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
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"
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
	return []string{"BootableVolumeName"}, warnings, nil
}

func (b *Builder) Run(ctx context.Context, ui packer.Ui, hook packer.Hook) (packer.Artifact, error) {
	state := new(multistep.BasicStateBag)
	state.Put("hook", hook)
	state.Put("ui", ui)

	generatedData := &packerbuilderdata.GeneratedData{State: state}

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

	if b.config.Communicator == "ssh" {
		sshSteps, err := b.buildSSHSteps()
		if err != nil {
			ui.Errorf("SSH communicator config error: %v", err)
			return nil, nil
		}
		steps = append(steps, sshSteps...)
	}

	if b.config.Communicator == "winrm" {
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
	)

	if !b.config.SkipCreateImage {
		steps = append(steps, &StepCreateBootableVolume{
			Config:        b.config,
			Client:        b.client,
			GeneratedData: generatedData,
		})
	}

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, nil
	}

	if b.config.SkipCreateImage {
		return nil, nil
	}

	bootableVolumeName, ok := state.Get("bootable_volume_name").(string)
	if !ok || bootableVolumeName == "" {
		return nil, fmt.Errorf("bootable volume name not found in state")
	}
	namespace, _ := state.Get("bootable_volume_namespace").(string)

	return &Artifact{
		Name:      bootableVolumeName,
		Namespace: namespace,
		StateData: map[string]any{
			"generated_data": state.Get("generated_data"),
		},
	}, nil
}

func (b *Builder) buildSSHSteps() ([]multistep.Step, []error) {
	commConfig := &communicator.Config{
		Type: b.config.Communicator,
		SSH: communicator.SSH{
			SSHHost:     b.config.SSHHost,
			SSHPort:     b.config.SSHLocalPort,
			SSHUsername: b.config.SSHUsername,
			SSHPassword: b.config.SSHPassword,
			SSHTimeout:  b.config.SSHWaitTimeout,
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
					User: b.config.SSHUsername,
					Auth: []ssh.AuthMethod{
						ssh.Password(b.config.SSHPassword),
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
		Type: b.config.Communicator,
		WinRM: communicator.WinRM{
			WinRMHost:     b.config.WinRMHost,
			WinRMPort:     b.config.WinRMLocalPort,
			WinRMUser:     b.config.WinRMUsername,
			WinRMPassword: b.config.WinRMPassword,
			WinRMTimeout:  b.config.WinRMWaitTimeout,
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
					Username: b.config.WinRMUsername,
					Password: b.config.WinRMPassword,
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
