package runner

import (
	"context"
	"fmt"
	"net/http"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	kubeclient "sigs.k8s.io/external-dns/pkg/client"
	"sigs.k8s.io/external-dns/source"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
	dnsepfixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/dnsendpoint"
)

// DNSEndpointRunner benchmarks source.NewCRDSource (kind: DNSEndpoint).
// crdRestCfg is built via InstrumentedRESTConfig (same path as external-dns) so it
// carries Prometheus transport instrumentation. The benchmark counting transport and
// proxy settings from benchCfg are then layered on top.
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
	// KubeconfigPath is the path to the kubeconfig used to load cluster credentials
	// for InstrumentedRESTConfig.
	KubeconfigPath string
	// BenchCfg is the benchmark REST config (proxy endpoint + counting transport).
	// Its Host, TLS settings, and WrapTransport are applied on top of the instrumented
	// config so all CRD calls go through toxiproxy and the counting transport.
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

	// Build via InstrumentedRESTConfig — same instrumentation path as external-dns —
	// pointing at the benchmark proxy endpoint so all CRD calls go through toxiproxy.
	crdRestCfg, err := kubeclient.InstrumentedRESTConfig(
		cfg.KubeconfigPath, cfg.BenchCfg.Host, 0, cfg.KubeAPIQPS, cfg.KubeAPIBurst,
	)
	if err != nil {
		return nil, fmt.Errorf("dnsendpoint runner: instrument rest config: %w", err)
	}
	// Copy TLS settings from benchCfg (insecure when proxy is active).
	crdRestCfg.TLSClientConfig = cfg.BenchCfg.TLSClientConfig
	// Chain the counting transport on top of the Prometheus instrumentation.
	instrumentedWrap := crdRestCfg.WrapTransport
	countingWrap := cfg.BenchCfg.WrapTransport
	crdRestCfg.WrapTransport = func(rt http.RoundTripper) http.RoundTripper {
		return countingWrap(instrumentedWrap(rt))
	}

	return &DNSEndpointRunner{
		kubeClient:      benchKubeClient,
		directDynClient: dynClient,
		crdRestCfg:      crdRestCfg,
		crdCfg:          &source.Config{LabelFilter: labels.Everything()},
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
