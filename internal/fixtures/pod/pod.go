// Package pod provides fixture setup for PodSource benchmarking.
package pod

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/helpers"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

const podNamespace = "default"

// PodFixture creates annotated pods for PodSource benchmarking.
// Each pod carries hostname and target annotations so the source emits endpoints
// without needing real pod IPs in status.
type PodFixture struct {
	KubeClient  kubernetes.Interface
	NPods       int
	Concurrency int
}

// Setup creates NPods annotated pods in the default namespace.
func (f *PodFixture) Setup(ctx context.Context) error {
	f.Concurrency = helpers.AtLeast(f.Concurrency, 1)
	if err := helpers.EnsureNamespace(ctx, f.KubeClient, podNamespace); err != nil {
		return err
	}
	if err := helpers.EnsureDefaultServiceAccount(ctx, f.KubeClient, podNamespace); err != nil {
		return err
	}
	bar := progress.New("pods", f.NPods)
	skipped, err := helpers.RunConcurrent(ctx, f.NPods, f.Concurrency, bar, func(i int) (bool, error) {
		pod := &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("bench-pod-%d", i),
				Namespace: podNamespace,
				Labels:    helpers.CommonLabels(map[string]string{"pod-index": fmt.Sprintf("%d", i)}),
				Annotations: helpers.MergeStringMaps(helpers.CommonAnnotations(i), map[string]string{
					"external-dns.alpha.kubernetes.io/hostname": fmt.Sprintf("pod-%d.example.com", i),
					"external-dns.alpha.kubernetes.io/target":   fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256+1),
				}),
			},
			Spec: corev1.PodSpec{
				Containers:                   []corev1.Container{{Name: "bench", Image: "scratch"}},
				AutomountServiceAccountToken: helpers.BoolPtr(false),
			},
		}
		_, err := f.KubeClient.CoreV1().Pods(podNamespace).Create(ctx, pod, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("create pod: %w", err)
	}
	helpers.LogSkipped(skipped, "pod")
	return nil
}
