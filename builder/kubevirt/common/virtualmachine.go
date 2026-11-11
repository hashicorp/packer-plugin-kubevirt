// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package common

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/packer"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	ptr "k8s.io/utils/ptr"

	v1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
)

// CreateVirtualMachine creates the VM, waits for it to become ready, and
// returns its UID for cleanup. Both builders delegate to this after
// constructing their VM spec.
func CreateVirtualMachine(ctx context.Context, state multistep.StateBag, client kubecli.KubevirtClient, vm *v1.VirtualMachine) (types.UID, error) {
	ui := state.Get("ui").(packer.Ui)
	name := vm.Name
	namespace := vm.Namespace

	ui.Sayf("Creating a new temporary VirtualMachine (%s/%s)...", namespace, name)

	created, err := client.VirtualMachine(namespace).Create(ctx, vm, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to create the VirtualMachine (%s/%s): %w", namespace, name, err)
	}

	if err := WaitUntilVirtualMachineReady(ctx, client, namespace, name); err != nil {
		return created.UID, fmt.Errorf("the VirtualMachine (%s/%s) did not become ready: %w", namespace, name, err)
	}
	return created.UID, nil
}

// CleanupVirtualMachine deletes the VM unless keep_vm is set. Only deletes a
// VM that was actually created (identified by UID) to avoid removing an
// unrelated VM with the same name.
func CleanupVirtualMachine(state multistep.StateBag, client kubecli.KubevirtClient, config BuilderConfig, created bool, uid types.UID) {
	if !created {
		return
	}

	ui := state.Get("ui").(packer.Ui)

	if config.KeepVM {
		ui.Sayf("Keeping VirtualMachine (%s/%s).", config.Namespace, config.Name)
		return
	}

	ui.Sayf("Deleting VirtualMachine (%s/%s)...", config.Namespace, config.Name)

	deleteOptions := DeleteOnlyUID(uid)
	deleteOptions.GracePeriodSeconds = ptr.To(int64(0))
	_ = client.VirtualMachine(config.Namespace).Delete(context.Background(), config.Name, deleteOptions)
}

// WaitUntilVirtualMachineReady polls until the VM reports Ready status.
func WaitUntilVirtualMachineReady(ctx context.Context, client kubecli.KubevirtClient, namespace, name string) error {
	pollInterval := 5 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		vm, err := client.VirtualMachine(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		return vm.Status.Ready, nil
	}
	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}
