// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"

	"kubevirt.io/client-go/kubecli"
	"kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// Halt reports err to the user and records it in the state bag, so that
// Builder.Run returns it to Packer instead of reporting a successful build.
func Halt(state multistep.StateBag, err error) multistep.StepAction {
	state.Put("error", err)
	state.Get("ui").(packer.Ui).Error(err.Error())
	return multistep.ActionHalt
}

// DeleteOnlyUID returns delete options that only match the object with the
// given UID, so that cleanup cannot remove an object that replaced the one
// the build created under the same name.
func DeleteOnlyUID(uid types.UID) metav1.DeleteOptions {
	if uid == "" {
		return metav1.DeleteOptions{}
	}
	return metav1.DeleteOptions{Preconditions: metav1.NewUIDPreconditions(string(uid))}
}

// ProvisionSteps returns the steps that open a port forward to the VM,
// connect the communicator, and run the provisioners.
func ProvisionSteps(config BuilderConfig, client kubecli.KubevirtClient, comm communicator.Config) []multistep.Step {
	var steps []multistep.Step

	// The VM is only reachable through a port forward, so the communicator
	// connects with a copy of its configuration that StepStartPortForward
	// points at the local end of the tunnel.
	connComm := comm
	if connComm.Type == "ssh" || connComm.Type == "winrm" {
		steps = append(steps, &StepStartPortForward{
			Config:        config,
			Client:        client,
			ForwarderFunc: DefaultPortForwarderFactory,
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

// NewKubevirtClient creates a KubeVirt client from the given kubeconfig path.
func NewKubevirtClient(kubeConfigPath string) (kubecli.KubevirtClient, error) {
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load kube_config %q: %w", kubeConfigPath, err)
	}

	// The KubeVirt client also implements kubernetes.Interface, so it is used
	// for the core Kubernetes resources as well.
	client, err := kubecli.GetKubevirtClientFromRESTConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create the KubeVirt client: %w", err)
	}
	return client, nil
}

// RunResult inspects the state bag after the runner finishes and returns the
// build artifact or an error. Both builders call this at the end of Run.
func RunResult(ctx context.Context, state multistep.StateBag, builderID string, skipCreateImage bool) (packer.Artifact, error) {
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

	if skipCreateImage {
		return nil, nil
	}

	bootableVolumeName, ok := state.Get("bootable_volume_name").(string)
	if !ok || bootableVolumeName == "" {
		return nil, fmt.Errorf("bootable volume name not found in state")
	}
	namespace, _ := state.Get("bootable_volume_namespace").(string)

	return &Artifact{
		BuilderIDValue: builderID,
		Name:           bootableVolumeName,
		Namespace:      namespace,
		StateData: map[string]any{
			"generated_data": state.Get("generated_data"),
		},
	}, nil
}

func WaitUntilDataVolumeSucceeded(ctx context.Context, client kubecli.KubevirtClient, namespace, name string) error {
	pollInterval := 15 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		dataVolume, err := client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}

		if dataVolume != nil && dataVolume.Status.Phase == v1beta1.Succeeded {
			return true, nil
		}
		return false, nil
	}
	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}
