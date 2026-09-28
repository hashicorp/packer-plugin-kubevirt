// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package iso

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"

	"k8s.io/client-go/tools/clientcmd"

	"kubevirt.io/client-go/kubecli"
)

type Builder struct {
	config Config
	runner multistep.Runner
	client kubecli.KubevirtClient
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec {
	return b.config.FlatMapstructure().HCL2Spec()
}

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	warnings, errs := b.config.Prepare(raws...)
	if errs != nil {
		return nil, warnings, errs
	}

	restConfig, err := clientcmd.BuildConfigFromFlags("", b.config.KubeConfig)
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to load kube_config %q: %w", b.config.KubeConfig, err)
	}

	// The KubeVirt client also implements kubernetes.Interface, so it is used
	// for the core Kubernetes resources as well.
	client, err := kubecli.GetKubevirtClientFromRESTConfig(restConfig)
	if err != nil {
		return nil, warnings, fmt.Errorf("failed to create the KubeVirt client: %w", err)
	}
	b.client = client

	return []string{"BootableVolumeName"}, warnings, nil
}

func (b *Builder) Run(ctx context.Context, ui packer.Ui, hook packer.Hook) (packer.Artifact, error) {
	state := new(multistep.BasicStateBag)
	state.Put("hook", hook)
	state.Put("ui", ui)

	generatedData := &packerbuilderdata.GeneratedData{State: state}

	steps := []multistep.Step{}
	if !b.config.SkipCreateImage {
		steps = append(steps, &StepPreValidate{
			Config: b.config,
			Client: b.client,
		})
	}
	steps = append(steps,
		&StepValidateIsoDataVolume{
			Config: b.config,
			Client: b.client,
		},
		&StepCopyMediaFiles{
			Config: b.config,
			Client: b.client,
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

	steps = append(steps, b.provisionSteps()...)
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
	// Steps interrupted by a cancellation halt without recording an error, and
	// the runner may not have marked the state as cancelled yet.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("build was cancelled: %w", err)
	}
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("build was cancelled")
	}
	if _, ok := state.GetOk(multistep.StateHalted); ok {
		return nil, errors.New("build was halted")
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

// provisionSteps returns the steps that connect the communicator to the VM and
// run the provisioners.
func (b *Builder) provisionSteps() []multistep.Step {
	var steps []multistep.Step

	// The VM is only reachable through a port forward, so the communicator
	// connects with a copy of its configuration that StepStartPortForward
	// points at the local end of the tunnel.
	connComm := b.config.Comm
	if connComm.Type == "ssh" || connComm.Type == "winrm" {
		steps = append(steps, &StepStartPortForward{
			Config:        b.config,
			Client:        b.client,
			ForwarderFunc: DefaultPortForwarder,
			Comm:          &connComm,
		})
	}

	return append(steps,
		&communicator.StepConnect{
			Config: &connComm,
			Host: func(multistep.StateBag) (string, error) {
				return connComm.Host(), nil
			},
			SSHConfig: connComm.SSHConfigFunc(),
			SSHPort: func(multistep.StateBag) (int, error) {
				return connComm.Port(), nil
			},
			WinRMPort: func(multistep.StateBag) (int, error) {
				return connComm.Port(), nil
			},
		},
		&commonsteps.StepProvision{},
	)
}
