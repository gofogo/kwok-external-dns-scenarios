package runner

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/external-dns/source"

	dnsepfixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/dnsendpoint"
)

// DNSEndpointRunner benchmarks source.NewCRDSource (kind: DNSEndpoint).
// The CRD REST client is built once at construction to avoid a kubeconfig
// disk read and discovery API call on every NewSource invocation.
type DNSEndpointRunner struct {
	directDynClient dynamic.Interface
	crdClient       rest.Interface
	crdScheme       *runtime.Scheme
	crdCfg          *source.Config
	nEndpoints      int
	concurrency     int
}

func NewDNSEndpointRunner(
	benchKubeClient kubernetes.Interface,
	kubeconfigPath string,
	directCfg *rest.Config,
	nEndpoints, concurrency int,
) (*DNSEndpointRunner, error) {
	dynClient, err := dynamic.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("dnsendpoint runner: build dynamic client: %w", err)
	}
	cfg := &source.Config{
		KubeConfig:          kubeconfigPath,
		CRDSourceAPIVersion: "externaldns.k8s.io/v1alpha1",
		CRDSourceKind:       "DNSEndpoint",
		LabelFilter:         labels.Everything(),
	}
	crdClient, scheme, err := source.NewCRDClientForAPIVersionKind(benchKubeClient, cfg)
	if err != nil {
		return nil, fmt.Errorf("dnsendpoint runner: build CRD client: %w", err)
	}
	return &DNSEndpointRunner{
		directDynClient: dynClient,
		crdClient:       crdClient,
		crdScheme:       scheme,
		crdCfg:          cfg,
		nEndpoints:      nEndpoints,
		concurrency:     concurrency,
	}, nil
}

func (r *DNSEndpointRunner) Label() string      { return "dnsendpoint" }
func (r *DNSEndpointRunner) ResourceCount() int { return r.nEndpoints }

func (r *DNSEndpointRunner) Setup(ctx context.Context) error {
	return (&dnsepfixture.DNSEndpointFixture{
		DynClient:   r.directDynClient,
		N:           r.nEndpoints,
		Concurrency: r.concurrency,
	}).Setup(ctx)
}

func (r *DNSEndpointRunner) NewSource(ctx context.Context) (source.Source, error) {
	return source.NewCRDSource(r.crdClient, r.crdCfg, r.crdScheme)
}
