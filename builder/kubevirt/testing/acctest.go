// Copyright (c) Red Hat, Inc.
// SPDX-License-Identifier: MPL-2.0

package testing

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/acctest"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8srand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	kubevirtv1 "kubevirt.io/api/core/v1"
	"kubevirt.io/client-go/kubecli"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
)

// testWindowsISODefaultURL is a Windows 11 ISO download URL that worked at
// the time of writing. Microsoft rotates these URLs, so override it with the
// PACKER_ACC_WINDOWS_ISO_URL environment variable when it stops working. See
// the README for how to generate a fresh URL.
const testWindowsISODefaultURL = "https://software-static.download.prss.microsoft.com/dbazure/888969d5-f34g-4e03-ac9d-1f9786c66749/26200.6584.250915-1905.25h2_ge_release_svc_refresh_CLIENT_CONSUMER_x64FRE_en-us.iso"

// WindowsISOURL returns the Windows ISO download URL from the
// PACKER_ACC_WINDOWS_ISO_URL environment variable, or testWindowsISODefaultURL
// if it is not set.
func WindowsISOURL() string {
	if url := os.Getenv("PACKER_ACC_WINDOWS_ISO_URL"); url != "" {
		return url
	}
	return testWindowsISODefaultURL
}

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
		if dataVolume != nil && dataVolume.Status.Phase == cdiv1.Succeeded {
			return true, nil
		}
		return false, nil
	}
	return wait.PollUntilContextTimeout(ctx, pollInterval, pollTimeout, true, poller)
}

// EnsureRegistryDataVolume creates a DataVolume that imports a disk image from
// a container registry (e.g. quay.io/containerdisks/fedora) if it doesn't
// already exist, and waits for the import to complete.
func EnsureRegistryDataVolume(ctx context.Context, client kubecli.KubevirtClient, namespace, name, image, size string) error {
	cdi := client.CdiClient().CdiV1beta1()

	_, err := cdi.DataVolumes(namespace).Get(ctx, name, metav1.GetOptions{})
	if err == nil {
		return waitUntilDataVolumeSucceeded(ctx, client, namespace, name)
	}

	dv := &cdiv1.DataVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"cdi.kubevirt.io/storage.bind.immediate.requested": "true",
			},
		},
		Spec: cdiv1.DataVolumeSpec{
			Source: &cdiv1.DataVolumeSource{
				Registry: &cdiv1.DataVolumeSourceRegistry{
					URL: ptr.To("docker://" + image),
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

	if _, err := cdi.DataVolumes(namespace).Create(ctx, dv, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("failed to create registry DataVolume %s/%s: %w", namespace, name, err)
	}

	return waitUntilDataVolumeSucceeded(ctx, client, namespace, name)
}

// CleanupBootableVolume deletes the DataSource, DataVolume, PVC, and rootdisk
// resources created by a builder run.
func CleanupBootableVolume(client kubecli.KubevirtClient, namespace, name string) {
	ctx := context.Background()
	cdi := client.CdiClient().CdiV1beta1()

	_ = cdi.DataSources(namespace).Delete(ctx, name, metav1.DeleteOptions{})
	_ = cdi.DataVolumes(namespace).Delete(ctx, name, metav1.DeleteOptions{})

	rootDisk := name + "-rootdisk"
	_ = cdi.DataVolumes(namespace).Delete(ctx, rootDisk, metav1.DeleteOptions{})
}

// CleanupDataVolume deletes a DataVolume and its PVC.
func CleanupDataVolume(client kubecli.KubevirtClient, namespace, name string) {
	ctx := context.Background()
	_ = client.CdiClient().CdiV1beta1().DataVolumes(namespace).Delete(ctx, name, metav1.DeleteOptions{})
}

// EnsureNamespace creates a namespace if it doesn't already exist.
func EnsureNamespace(ctx context.Context, client kubecli.KubevirtClient, namespace string) error {
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

// ReadTestData reads a file from the testdata directory relative to the
// test's working directory.
func ReadTestData(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("failed to read testdata/%s: %v", name, err)
	}
	return string(data)
}

func RandomNamespace(prefix string) string {
	return fmt.Sprintf("%s%s", prefix, k8srand.String(5))
}

const (
	// clusterStateTimeout bounds how long dumping the cluster state may take,
	// so that an unhealthy cluster cannot hang the test cleanup.
	clusterStateTimeout = 2 * time.Minute
	// podLogTailLines is the number of log lines collected per container.
	podLogTailLines = 500
)

// DumpNamespaceOnFailure writes the state of the KubeVirt and CDI resources in
// the namespace, the logs of its pods and its events to a file in the test's
// working directory, if the test failed. Call it in t.Cleanup before deleting
// any resource, so that the state is collected before it is gone.
func DumpNamespaceOnFailure(t *testing.T, client kubecli.KubevirtClient, namespace string) {
	t.Helper()
	if !t.Failed() {
		return
	}

	path := "cluster_state_" + strings.ReplaceAll(t.Name(), "/", "_") + ".txt"
	file, err := os.Create(path)
	if err != nil {
		t.Logf("failed to create %s: %v", path, err)
		return
	}
	defer file.Close()

	ctx, cancel := context.WithTimeout(context.Background(), clusterStateTimeout)
	defer cancel()
	dumpNamespace(ctx, file, client, namespace)
	t.Logf("state of namespace %s written to %s", namespace, path)
}

func dumpNamespace(ctx context.Context, w io.Writer, client kubecli.KubevirtClient, namespace string) {
	listOptions := metav1.ListOptions{}

	dumpObjects(w, "VirtualMachines", func() ([]kubevirtv1.VirtualMachine, error) {
		list, err := client.VirtualMachine(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
	dumpObjects(w, "VirtualMachineInstances", func() ([]kubevirtv1.VirtualMachineInstance, error) {
		list, err := client.VirtualMachineInstance(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
	dumpObjects(w, "DataVolumes", func() ([]cdiv1.DataVolume, error) {
		list, err := client.CdiClient().CdiV1beta1().DataVolumes(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
	dumpObjects(w, "PersistentVolumeClaims", func() ([]corev1.PersistentVolumeClaim, error) {
		list, err := client.CoreV1().PersistentVolumeClaims(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
	pods := dumpObjects(w, "Pods", func() ([]corev1.Pod, error) {
		list, err := client.CoreV1().Pods(namespace).List(ctx, listOptions)
		if err != nil {
			return nil, err
		}
		return list.Items, nil
	})
	dumpPodLogs(ctx, w, client, pods)
	dumpEvents(ctx, w, client, namespace)
}

// dumpObjects writes the objects returned by list as YAML, and returns them.
func dumpObjects[T any, PT interface {
	*T
	metav1.Object
}](
	w io.Writer,
	title string,
	list func() ([]T, error)) []T {
	writeSection(w, title)
	items, err := list()
	if err != nil {
		dumpf(w, "failed to list %s: %v\n", title, err)
		return nil
	}
	for i := range items {
		obj := PT(&items[i])
		obj.SetManagedFields(nil)
		dumpf(w, "--- %s ---\n", obj.GetName())
		data, err := yaml.Marshal(obj)
		if err != nil {
			dumpf(w, "failed to marshal %s: %v\n", obj.GetName(), err)
			continue
		}
		_, _ = w.Write(data)
	}
	return items
}

// dumpPodLogs writes the last log lines of every container of the pods. For
// virt-launcher pods this includes the guest serial console log.
func dumpPodLogs(ctx context.Context, w io.Writer, client kubecli.KubevirtClient, pods []corev1.Pod) {
	writeSection(w, "Pod logs")
	for _, pod := range pods {
		containers := append(append([]corev1.Container{}, pod.Spec.InitContainers...), pod.Spec.Containers...)
		for _, container := range containers {
			dumpf(w, "--- %s/%s ---\n", pod.Name, container.Name)
			logs, err := client.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, &corev1.PodLogOptions{
				Container: container.Name,
				TailLines: ptr.To[int64](podLogTailLines),
			}).DoRaw(ctx)
			if err != nil {
				dumpf(w, "failed to get logs: %v\n", err)
				continue
			}
			_, _ = w.Write(logs)
		}
	}
}

// dumpEvents writes the events of the namespace in chronological order.
func dumpEvents(ctx context.Context, w io.Writer, client kubecli.KubevirtClient, namespace string) {
	writeSection(w, "Events")
	list, err := client.CoreV1().Events(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		dumpf(w, "failed to list events: %v\n", err)
		return
	}
	events := list.Items
	sort.SliceStable(events, func(i, j int) bool {
		return eventTime(events[i]).Before(eventTime(events[j]))
	})
	for _, event := range events {
		dumpf(w, "%s %s %s %s/%s: %s\n",
			eventTime(event).UTC().Format(time.RFC3339), event.Type, event.Reason,
			event.InvolvedObject.Kind, event.InvolvedObject.Name, event.Message)
	}
}

// eventTime returns the last time the event was observed. Depending on the
// API used to record it, an event sets either LastTimestamp or EventTime.
func eventTime(event corev1.Event) time.Time {
	if !event.LastTimestamp.IsZero() {
		return event.LastTimestamp.Time
	}
	if !event.EventTime.IsZero() {
		return event.EventTime.Time
	}
	return event.CreationTimestamp.Time
}

func writeSection(w io.Writer, title string) {
	dumpf(w, "\n=== %s ===\n", title)
}

// dumpf writes to the cluster state dump. Write errors are ignored because
// collecting the state is best-effort and must never fail the test.
func dumpf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}
