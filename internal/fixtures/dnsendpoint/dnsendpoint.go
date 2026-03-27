// Package dnsendpoint provides fixture setup for CRDSource (DNSEndpoint) benchmarking.
package dnsendpoint

import (
	"context"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/helpers"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

const defaultNs = "default"

var gvr = schema.GroupVersionResource{
	Group:    "externaldns.k8s.io",
	Version:  "v1alpha1",
	Resource: "dnsendpoints",
}

// DNSEndpointFixture creates DNSEndpoint CRD objects for CRDSource benchmarking.
// Requires the DNSEndpoint CRD to be installed in the cluster before Setup is called.
// When NsDist is set, endpoints are distributed across namespaces proportionally;
// otherwise all endpoints are created in the default namespace.
type DNSEndpointFixture struct {
	KubeClient  kubernetes.Interface
	DynClient   dynamic.Interface
	N           int
	NsDist      distribute.Weights
	Concurrency int
}

// Setup creates N DNSEndpoint objects, distributing across namespaces per NsDist.
func (f *DNSEndpointFixture) Setup(ctx context.Context) error {
	f.Concurrency = helpers.AtLeast(f.Concurrency, 1)

	// Build per-index namespace assignment.
	nsForIndex := distribute.Distribute(f.N, f.NsDist)
	if nsForIndex == nil {
		nsForIndex = make([]string, f.N)
		for i := range f.N {
			nsForIndex[i] = defaultNs
		}
	}

	// Ensure all target namespaces exist before creating endpoints.
	seen := map[string]bool{}
	for _, name := range nsForIndex {
		if !seen[name] {
			seen[name] = true
			if err := helpers.EnsureNamespace(ctx, f.KubeClient, name); err != nil {
				return err
			}
		}
	}

	bar := progress.New("dnsendpoints", f.N)
	skipped, err := helpers.RunConcurrent(ctx, f.N, f.Concurrency, bar, func(i int) (bool, error) {
		epNs := nsForIndex[i]
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "externaldns.k8s.io/v1alpha1",
				"kind":       "DNSEndpoint",
				"metadata": map[string]interface{}{
					"name":        fmt.Sprintf("bench-ep-%d", i),
					"namespace":   epNs,
					"labels":      helpers.StringsToInterface(helpers.CommonLabels(map[string]string{"ep-index": fmt.Sprintf("%d", i)})),
					"annotations": helpers.StringsToInterface(helpers.CommonAnnotations(i)),
				},
				"spec": map[string]interface{}{
					"endpoints": []interface{}{
						map[string]interface{}{
							"dnsName":    fmt.Sprintf("ep-%d.example.com", i),
							"targets":    []interface{}{fmt.Sprintf("192.168.%d.%d", (i/256)%256, i%256+1)},
							"recordType": "A",
							"recordTTL":  int64(300),
						},
					},
				},
			},
		}
		_, err := f.DynClient.Resource(gvr).Namespace(epNs).Create(ctx, obj, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("create dnsendpoint: %w", err)
	}
	helpers.LogSkipped(skipped, "dnsendpoint")
	return nil
}
