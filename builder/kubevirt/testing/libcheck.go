// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package testing

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	kubecli "kubevirt.io/client-go/kubecli"

	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

func CheckLogsFromProvisioner(logfile string) error {
	logBytes, err := os.ReadFile(logfile)
	if err != nil {
		return fmt.Errorf("failed to read build log: %w", err)
	}
	if !strings.Contains(string(logBytes), "Acceptance test provisioner ran successfully") {
		return fmt.Errorf("expected provisioner output not found in build log")
	}
	return nil
}

func CheckDataVolume(client kubecli.KubevirtClient, namespace, name string) error {
	dataVolume, err := client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get DataVolume %s/%s: %w", namespace, name, err)
	}
	if dataVolume.Status.Phase != cdiv1.Succeeded {
		return fmt.Errorf("expected DataVolume %s/%s phase to be Succeeded, got %s", namespace, name, dataVolume.Status.Phase)
	}
	return nil
}

func CheckDataSourceLabels(client kubecli.KubevirtClient, namespace, name string, labels map[string]string) error {
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

func CheckTemporaryVM(client kubecli.KubevirtClient, namespace, name string) error {
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

func CheckMediaConfigMap(client kubecli.KubevirtClient, namespace, name string) error {
	_, err := client.CoreV1().ConfigMaps(namespace).Get(context.Background(), name, metav1.GetOptions{})
	if !errors.IsNotFound(err) {
		return fmt.Errorf("expected media ConfigMap %s/%s to be deleted after build", namespace, name)
	}
	return nil
}
