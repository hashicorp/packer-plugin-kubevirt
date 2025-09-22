// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"fmt"

	"github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	"github.com/hashicorp/packer-plugin-sdk/packer"

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
	state := new(multistep.BasicStateBag)
	state.Put("hook", hook)
	state.Put("ui", ui)

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

	if b.config.Comm.Type != "none" {
		steps = append(steps,
			&StepStartPortForward{
				Config:        b.config,
				Client:        b.client,
				ForwarderFunc: DefaultPortForwarder,
			},
			&communicator.StepConnect{
				Config:    &b.config.Comm,
				Host:      common.CommHost(b.config.Comm.Host()),
				SSHPort:   common.CommPort(b.config.Comm.Port()),
				SSHConfig: b.config.Comm.SSHConfigFunc(),
				WinRMPort: common.CommPort(b.config.Comm.Port()),
			},
			&commonsteps.StepProvision{},
		)
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

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	bootableVolumeName, ok := state.Get("bootable_volume_name").(string)
	if !ok || bootableVolumeName == "" {
		return nil, fmt.Errorf("bootable volume name not found in state")
	}
	return &Artifact{Name: bootableVolumeName}, nil
}
