// Package service provides fixture setup for ServiceSource benchmarking.
// It creates nodes, services (headless and NodePort), and pods to populate
// all informer caches that ServiceSource relies on.
package service

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/helpers"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

const (
	svcNamespace = "default"
	podSvcLabel  = "bench-svc-pod"

	// TypeHeadless and TypeNodePort are the distribution weight keys understood by ServiceFixture.
	TypeHeadless = "headless"
	TypeNodePort = "node-port"
)

// ServiceFixture creates nodes, services, and pods for ServiceSource benchmarking.
//
// SvcDist weight keys: TypeHeadless ("headless") and TypeNodePort ("node-port").
// PodDist weight keys: same — distributes pods across the same two types.
type ServiceFixture struct {
	KubeClient  kubernetes.Interface
	NServices   int
	SvcDist     distribute.Weights
	NNodes      int
	NPods       int
	PodDist     distribute.Weights
	Concurrency int
}

// Setup creates namespace, then nodes and services concurrently (they are independent),
// then pods (which reference node names and must follow node creation).
func (f *ServiceFixture) Setup(ctx context.Context) error {
	f.Concurrency = helpers.AtLeast(f.Concurrency, 1)
	if err := helpers.EnsureNamespace(ctx, f.KubeClient, svcNamespace); err != nil {
		return err
	}
	if err := helpers.EnsureDefaultServiceAccount(ctx, f.KubeClient, svcNamespace); err != nil {
		return err
	}

	// Nodes and services run sequentially so their progress bars don't overwrite each other.
	if f.NNodes > 0 {
		if err := f.createNodes(ctx); err != nil {
			return err
		}
	}
	if f.NServices > 0 {
		if err := f.createServices(ctx); err != nil {
			return err
		}
	}

	// Pods reference node names — must follow node creation.
	if f.NPods > 0 {
		return f.createPods(ctx)
	}
	return nil
}

func (f *ServiceFixture) createNodes(ctx context.Context) error {
	bar := progress.New("nodes", f.NNodes)
	skipped, err := helpers.RunConcurrent(ctx, f.NNodes, f.Concurrency, bar, func(i int) (bool, error) {
		node := &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("bench-node-%d", i),
				Labels:      helpers.CommonLabels(map[string]string{"node-index": fmt.Sprintf("%d", i)}),
				Annotations: helpers.CommonAnnotations(i),
			},
		}
		created, err := f.KubeClient.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("create node %d: %w", i, err)
		}
		// KWOK may patch the node between Create and UpdateStatus, bumping resourceVersion.
		// Retry on conflict by re-fetching the latest version.
		wantStatus := corev1.NodeStatus{
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeExternalIP, Address: helpers.NodeExternalIP(i)},
			},
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
		}
		if err := helpers.RetryUpdateStatus(ctx,
			func() error {
				created.Status = wantStatus
				_, err := f.KubeClient.CoreV1().Nodes().UpdateStatus(ctx, created, metav1.UpdateOptions{})
				return err
			},
			func() error {
				var err error
				created, err = f.KubeClient.CoreV1().Nodes().Get(ctx, created.Name, metav1.GetOptions{})
				return err
			},
		); err != nil {
			return false, fmt.Errorf("update node status %d: %w", i, err)
		}
		return false, nil
	})
	helpers.LogSkipped(skipped, "node")
	return err
}

// createServices distributes NServices across the type weights.
// NodePort services use node ExternalIPs for targets (via informer cache).
// Headless services carry an explicit target annotation to bypass EndpointSlice lookup.
func (f *ServiceFixture) createServices(ctx context.Context) error {
	dist := f.SvcDist
	if len(dist) == 0 {
		dist = distribute.Weights{TypeNodePort: 1}
	}
	types := distribute.Distribute(f.NServices, dist)
	bar := progress.New("services", f.NServices)
	skipped, err := helpers.RunConcurrent(ctx, f.NServices, f.Concurrency, bar, func(i int) (bool, error) {
		svcType := types[i]
		annos := helpers.MergeStringMaps(helpers.CommonAnnotations(i), map[string]string{
			"external-dns.alpha.kubernetes.io/hostname": fmt.Sprintf("svc-%d.example.com", i),
		})

		var spec corev1.ServiceSpec
		switch svcType {
		case TypeHeadless:
			// Explicit target annotation bypasses EndpointSlice lookup.
			annos["external-dns.alpha.kubernetes.io/target"] = helpers.PodSvcIP(i)
			spec = corev1.ServiceSpec{
				ClusterIP: "None",
				Selector:  map[string]string{podSvcLabel: "true"},
				Ports:     []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
			}
		default: // TypeNodePort
			// No target annotation — ServiceSource resolves targets from node ExternalIPs.
			spec = corev1.ServiceSpec{
				Type:  corev1.ServiceTypeNodePort,
				Ports: []corev1.ServicePort{{Port: 80, TargetPort: intstr.FromInt32(80)}},
			}
		}

		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("bench-svc-%d", i),
				Namespace:   svcNamespace,
				Labels:      helpers.CommonLabels(map[string]string{"svc-index": fmt.Sprintf("%d", i), "svc-type": svcType}),
				Annotations: annos,
			},
			Spec: spec,
		}
		_, err := f.KubeClient.CoreV1().Services(svcNamespace).Create(ctx, svc, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("create service: %w", err)
	}
	helpers.LogSkipped(skipped, "service")
	return nil
}

// createPods assigns pods round-robin to nodes and sets Running status.
// PodDist controls which pods carry the headless-service selector label (podSvcLabel).
// Pods without that label are still created and populate the node/pod informer caches
// but are not selected by any headless service.
func (f *ServiceFixture) createPods(ctx context.Context) error {
	nNodes := helpers.AtLeast(f.NNodes, 1)
	podDist := f.PodDist
	if len(podDist) == 0 {
		podDist = distribute.Weights{TypeHeadless: 1}
	}
	types := distribute.Distribute(f.NPods, podDist)
	bar := progress.New("svc-pods", f.NPods)
	skipped, err := helpers.RunConcurrent(ctx, f.NPods, f.Concurrency, bar, func(i int) (bool, error) {
		podLabels := helpers.CommonLabels(map[string]string{"pod-index": fmt.Sprintf("%d", i), "pod-type": types[i]})
		if types[i] == TypeHeadless {
			podLabels[podSvcLabel] = "true"
		}
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("bench-svc-pod-%d", i),
				Namespace:   svcNamespace,
				Labels:      podLabels,
				Annotations: helpers.CommonAnnotations(i),
			},
			Spec: corev1.PodSpec{
				NodeName:                     fmt.Sprintf("bench-node-%d", i%nNodes),
				Containers:                   []corev1.Container{{Name: "bench", Image: "scratch"}},
				AutomountServiceAccountToken: helpers.BoolPtr(false),
			},
		}
		created, err := f.KubeClient.CoreV1().Pods(svcNamespace).Create(ctx, pod, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("create svc pod %d: %w", i, err)
		}
		wantStatus := corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: helpers.PodSvcIP(i),
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		}
		if err := helpers.RetryUpdateStatus(ctx,
			func() error {
				created.Status = wantStatus
				_, err := f.KubeClient.CoreV1().Pods(svcNamespace).UpdateStatus(ctx, created, metav1.UpdateOptions{})
				return err
			},
			func() error {
				var err error
				created, err = f.KubeClient.CoreV1().Pods(svcNamespace).Get(ctx, created.Name, metav1.GetOptions{})
				return err
			},
		); err != nil {
			return false, fmt.Errorf("update svc pod status %d: %w", i, err)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("create svc pod: %w", err)
	}
	helpers.LogSkipped(skipped, "svc-pod")
	return nil
}
