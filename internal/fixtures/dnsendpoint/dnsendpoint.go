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

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/helpers"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

const ns = "default"

var gvr = schema.GroupVersionResource{
	Group:    "externaldns.k8s.io",
	Version:  "v1alpha1",
	Resource: "dnsendpoints",
}

// DNSEndpointFixture creates DNSEndpoint CRD objects for CRDSource benchmarking.
// Requires the DNSEndpoint CRD to be installed in the cluster before Setup is called.
type DNSEndpointFixture struct {
	DynClient   dynamic.Interface
	N           int
	Concurrency int
}

// Setup creates N DNSEndpoint objects in the default namespace.
func (f *DNSEndpointFixture) Setup(ctx context.Context) error {
	f.Concurrency = helpers.AtLeast(f.Concurrency, 1)
	bar := progress.New("dnsendpoints", f.N)
	skipped, err := helpers.RunConcurrent(ctx, f.N, f.Concurrency, bar, func(i int) (bool, error) {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "externaldns.k8s.io/v1alpha1",
				"kind":       "DNSEndpoint",
				"metadata": map[string]interface{}{
					"name":        fmt.Sprintf("bench-ep-%d", i),
					"namespace":   ns,
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
		_, err := f.DynClient.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{})
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
