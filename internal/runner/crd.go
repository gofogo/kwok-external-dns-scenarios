package runner

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/external-dns/source"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
	dnsepfixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/dnsendpoint"
)

// DNSEndpointRunner benchmarks source.NewCRDSource (kind: DNSEndpoint).
// crdRestCfg is built once at construction: it connects directly to the API
// server (not through the proxy) but wraps the transport with the shared
// counting transport so all CRD calls appear in the benchmark breakdown.
type DNSEndpointRunner struct {
	kubeClient      kubernetes.Interface
	directDynClient dynamic.Interface
	crdRestCfg      *rest.Config
	crdCfg          *source.Config
	nEndpoints      int
	nsDist          distribute.Weights
	concurrency     int
}

// DNSEndpointConfig holds the CRD-source-specific settings for DNSEndpointRunner.
type DNSEndpointConfig struct {
	// NEndpoints is the total number of DNSEndpoint objects to create and benchmark.
	NEndpoints int
	// NsDist distributes endpoints across namespaces by weight; nil = all in default.
	NsDist distribute.Weights
	// KubeAPIQPS and KubeAPIBurst control the Kubernetes API client rate limiter.
	// 0 means use client-go built-in defaults (5 QPS / 10 burst).
	KubeAPIQPS   float32
	KubeAPIBurst int
	// BenchCfg is the benchmark REST config (proxy endpoint + counting transport).
	// It is copied and used as-is for the CRD source so all CRD calls go through
	// toxiproxy and are counted alongside other benchmark traffic.
	BenchCfg *rest.Config
}

func NewDNSEndpointRunner(
	benchKubeClient kubernetes.Interface,
	directCfg *rest.Config,
	concurrency int,
	cfg DNSEndpointConfig,
) (*DNSEndpointRunner, error) {
	dynClient, err := dynamic.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("dnsendpoint runner: build dynamic client: %w", err)
	}

	// Copy benchCfg so the CRD source uses the same proxy endpoint and counting
	// transport as all other benchmark clients.
	crdRestCfg := rest.CopyConfig(cfg.BenchCfg)

	srcCfg := &source.Config{
		LabelFilter:  labels.Everything(),
		KubeAPIQPS:   cfg.KubeAPIQPS,
		KubeAPIBurst: cfg.KubeAPIBurst,
	}

	return &DNSEndpointRunner{
		kubeClient:      benchKubeClient,
		directDynClient: dynClient,
		crdRestCfg:      crdRestCfg,
		crdCfg:          srcCfg,
		nEndpoints:      cfg.NEndpoints,
		nsDist:          cfg.NsDist,
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
	return source.NewCRDSource(ctx, r.crdRestCfg, r.crdCfg)
}
