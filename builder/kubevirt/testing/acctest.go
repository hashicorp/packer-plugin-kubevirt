// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package testing

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/acctest"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"

	"kubevirt.io/client-go/kubecli"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// RequireAcceptance skips the test unless PACKER_ACC is set.
func RequireAcceptance(t *testing.T) {
	t.Helper()
	if os.Getenv(acctest.TestEnvVar) == "" {
		t.Skipf("Acceptance tests skipped unless env '%s' set", acctest.TestEnvVar)
	}
}

// KubevirtClient returns a KubeVirt client configured from KUBECONFIG.
func KubevirtClient(t *testing.T) kubecli.KubevirtClient {
	t.Helper()
	kubeconfig := os.Getenv("KUBECONFIG")
	if kubeconfig == "" {
		t.Fatal("KUBECONFIG must be set for acceptance tests")
	}
	restConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		t.Fatalf("failed to load kubeconfig: %v", err)
	}
	client, err := kubecli.GetKubevirtClientFromRESTConfig(restConfig)
	if err != nil {
		t.Fatalf("failed to create KubeVirt client: %v", err)
	}
	return client
}

// EnsureISODataVolume creates a DataVolume that imports an ISO from an HTTP
// URL if it doesn't already exist, and waits for the import to complete.
func EnsureISODataVolume(ctx context.Context, client kubecli.KubevirtClient, namespace, name, url, size string) error {
	cdi := client.CdiClient().CdiV1beta1()

	_, err := cdi.DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return waitUntilDataVolumeSucceeded(ctx, client, namespace, name)
	}

	isoDV := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"cdi.kubevirt.io/storage.bind.immediate.requested": "true",
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				HTTP: &cdiv1.DataVolumeSourceHTTP{
					URL: url,
				},
			},
			PVC: &corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(size),
					},
				},
			},
		},
	}

	if _, err := cdi.DataVolumes(namespace).Create(ctx, isoDV, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create ISO DataVolume %s/%s: %w", namespace, name, err)
	}

	return waitUntilDataVolumeSucceeded(ctx, client, namespace, name)
}

// waitUntilDataVolumeSucceeded waits for a DataVolume to reach the Succeeded phase.
// It polls the DataVolume until it reaches the Succeeded phase or the timeout is reached.
func waitUntilDataVolumeSucceeded(ctx context.Context, client kubecli.KubevirtClient, namespace, name string) error {
	pollInterval := 15 * time.Second
	pollTimeout := 3600 * time.Second
	poller := func(ctx context.Context) (bool, error) {
		dataVolume, err := client.CdiClient().CdiV1beta1().DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if dataVolume != nil && dataVolume.Status.Phase == cdiv1.DataVolumePhase(cdiv1.Succeeded) {
			return true, nil
		}
		return false, nil
	}
	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}

// CleanupBootableVolume deletes the DataSource, DataVolume, PVC, and rootdisk
// resources created by a builder run.
func CleanupBootableVolume(client kubecli.KubevirtClient, namespace, name string) {
	ctx := context.Background()
	cdi := client.CdiClient().CdiV1beta1()

	_ = cdi.DataSources(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	_ = cdi.DataVolumes(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	_ = client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})

	rootDisk := name + "-rootdisk"
	_ = cdi.DataVolumes(namespace).Delete(ctx, rootDisk, metav1.DeleteOptions{})
	_ = client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, rootDisk, metav1.DeleteOptions{})
}

// CleanupDataVolume deletes a DataVolume and its PVC.
func CleanupDataVolume(client kubecli.KubevirtClient, namespace, name string) {
	ctx := context.Background()
	_ = client.CdiClient().CdiV1beta1().DataVolumes(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	_ = client.CoreV1().PersistentVolumeClaims(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// EnsureNamespace creates a namespace if it doesn't already exist.
func EnsureNamespace(ctx context.Context, client kubecli.KubevirtClient, namespace string) error {
	_, err := client.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if err == nil {
		return nil
	}
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}
	if _, err := client.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create namespace %s: %w", namespace, err)
	}
	return nil
}

// CleanupNamespace deletes a namespace and all its resources.
func CleanupNamespace(client kubecli.KubevirtClient, namespace string) {
	_ = client.CoreV1().Namespaces().Delete(context.Background(), namespace, metav1.DeleteOptions{})
}
