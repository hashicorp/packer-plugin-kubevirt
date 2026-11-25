// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package clone

import (
	"context"

	"github.com/hashicorp/hcl/v2/hcldec"
	kubevirtcommon "github.com/hashicorp/packer-plugin-kubevirt/builder/kubevirt/common"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/packerbuilderdata"

	"kubevirt.io/client-go/kubecli"
)

const BuilderID = "packer.kubevirt.clone"

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

	client, err := kubevirtcommon.NewKubevirtClient(b.config.KubeConfig)
	if err != nil {
		return nil, warnings, err
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
		steps = append(steps, &kubevirtcommon.StepPreValidate{
			Config: b.config.BuilderConfig,
			Client: b.client,
		})
	}
	steps = append(steps,
		&StepValidateVolumeSource{
			Config: b.config,
			Client: b.client,
		},
	)

	if len(b.config.BuilderConfig.MediaFiles) > 0 || len(b.config.BuilderConfig.MediaContent) > 0 {
		steps = append(steps, &kubevirtcommon.StepCopyMediaFiles{
			Config: b.config.BuilderConfig,
			Client: b.client,
		})
	}

	steps = append(steps,
		&StepCreateVirtualMachine{
			Config: b.config,
			Client: b.client,
		},
	)

	steps = append(steps, kubevirtcommon.ProvisionSteps(b.config.BuilderConfig, b.client, b.config.Comm)...)
	steps = append(steps,
		&kubevirtcommon.StepStopVirtualMachine{
			Config: b.config.BuilderConfig,
			Client: b.client,
		},
	)

	if !b.config.SkipCreateImage {
		steps = append(steps, &kubevirtcommon.StepCreateBootableVolume{
			Config:        b.config.BuilderConfig,
			Client:        b.client,
			GeneratedData: generatedData,
		})
	}

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	return kubevirtcommon.RunResult(ctx, state, BuilderID, b.config.SkipCreateImage)
}
