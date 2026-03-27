package runner

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/external-dns/source"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
	dnsepfixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/dnsendpoint"
)

// DNSEndpointRunner benchmarks source.NewCRDSource (kind: DNSEndpoint).
// The CRD REST client is built once at construction to avoid a kubeconfig
// disk read and discovery API call on every NewSource invocation.
type DNSEndpointRunner struct {
	kubeClient      kubernetes.Interface
	directDynClient dynamic.Interface
	crdClient       rest.Interface
	crdScheme       *runtime.Scheme
	crdCfg          *source.Config
	nEndpoints      int
	nsDist          distribute.Weights
	concurrency     int
}

func NewDNSEndpointRunner(
	benchKubeClient kubernetes.Interface,
	kubeconfigPath string,
	directCfg *rest.Config,
	nEndpoints, concurrency int,
	nsDist distribute.Weights,
	crdClientQPS float32,
	crdClientBurst int,
	wrapTransport func(http.RoundTripper) http.RoundTripper,
) (*DNSEndpointRunner, error) {
	dynClient, err := dynamic.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("dnsendpoint runner: build dynamic client: %w", err)
	}
	cfg := &source.Config{
		KubeConfig:             kubeconfigPath,
		CRDSourceAPIVersion:    "externaldns.k8s.io/v1alpha1",
		CRDSourceKind:          "DNSEndpoint",
		LabelFilter:            labels.Everything(),
		CRDClientQPS:           crdClientQPS,
		CRDClientBurst:         crdClientBurst,
		CRDClientWrapTransport: wrapTransport,
	}
	crdClient, scheme, err := source.NewCRDClientForAPIVersionKind(benchKubeClient, cfg)
	if err != nil {
		return nil, fmt.Errorf("dnsendpoint runner: build CRD client: %w", err)
	}
	return &DNSEndpointRunner{
		kubeClient:      benchKubeClient,
		directDynClient: dynClient,
		crdClient:       crdClient,
		crdScheme:       scheme,
		crdCfg:          cfg,
		nEndpoints:      nEndpoints,
		nsDist:          nsDist,
		concurrency:     concurrency,
	}, nil
}

func (r *DNSEndpointRunner) Label() string      { return "dnsendpoint" }
func (r *DNSEndpointRunner) ResourceCount() int { return r.nEndpoints }

func (r *DNSEndpointRunner) Setup(ctx context.Context) error {
	return (&dnsepfixture.DNSEndpointFixture{
		KubeClient:  r.kubeClient,
		DynClient:   r.directDynClient,
		N:           r.nEndpoints,
		NsDist:      r.nsDist,
		Concurrency: r.concurrency,
	}).Setup(ctx)
}

func (r *DNSEndpointRunner) NewSource(ctx context.Context) (source.Source, error) {
	return source.NewCRDSource(r.crdClient, r.crdCfg, r.crdScheme)
}
