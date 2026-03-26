package runner

import (
	"context"
	"fmt"

	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/external-dns/source"

	istiofixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/istio"
)

// GatewayRunner benchmarks source.NewIstioGatewaySource.
type GatewayRunner struct {
	benchKubeClient   kubernetes.Interface
	benchIstioClient  istioclient.Interface
	directKubeClient  kubernetes.Interface
	directIstioClient istioclient.Interface
	nServices         int
	nGateways         int
	concurrency       int
}

func NewGatewayRunner(
	benchKubeClient kubernetes.Interface,
	benchIstioClient istioclient.Interface,
	directCfg *rest.Config,
	nServices, nGateways, concurrency int,
) (*GatewayRunner, error) {
	kubeClient, err := kubernetes.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("gateway runner: build setup kube client: %w", err)
	}
	istioClient, err := istioclient.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("gateway runner: build setup istio client: %w", err)
	}
	return &GatewayRunner{
		benchKubeClient:   benchKubeClient,
		benchIstioClient:  benchIstioClient,
		directKubeClient:  kubeClient,
		directIstioClient: istioClient,
		nServices:         nServices,
		nGateways:         nGateways,
		concurrency:       concurrency,
	}, nil
}

func (r *GatewayRunner) Label() string      { return "gateway" }
func (r *GatewayRunner) ResourceCount() int { return r.nGateways }

func (r *GatewayRunner) Setup(ctx context.Context) error {
	return (&istiofixture.GatewayFixture{
		KubeClient:  r.directKubeClient,
		IstioClient: r.directIstioClient,
		NServices:   r.nServices,
		NGateways:   r.nGateways,
		Concurrency: r.concurrency,
	}).Setup(ctx)
}

func (r *GatewayRunner) NewSource(ctx context.Context) (source.Source, error) {
	return source.NewIstioGatewaySource(ctx, r.benchKubeClient, r.benchIstioClient, &source.Config{LabelFilter: labels.Everything()})
}

// VirtualServiceRunner benchmarks source.NewIstioVirtualServiceSource.
// GatewayRunner.Setup must run first to create the ingress services.
type VirtualServiceRunner struct {
	benchKubeClient   kubernetes.Interface
	benchIstioClient  istioclient.Interface
	directIstioClient istioclient.Interface
	nVirtualSvcs      int
	concurrency       int
}

func NewVirtualServiceRunner(
	benchKubeClient kubernetes.Interface,
	benchIstioClient istioclient.Interface,
	directCfg *rest.Config,
	nVirtualSvcs, concurrency int,
) (*VirtualServiceRunner, error) {
	istioClient, err := istioclient.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("virtualservice runner: build setup istio client: %w", err)
	}
	return &VirtualServiceRunner{
		benchKubeClient:   benchKubeClient,
		benchIstioClient:  benchIstioClient,
		directIstioClient: istioClient,
		nVirtualSvcs:      nVirtualSvcs,
		concurrency:       concurrency,
	}, nil
}

func (r *VirtualServiceRunner) Label() string      { return "virtualservice" }
func (r *VirtualServiceRunner) ResourceCount() int { return r.nVirtualSvcs }

func (r *VirtualServiceRunner) Setup(ctx context.Context) error {
	return (&istiofixture.VirtualServiceFixture{
		IstioClient: r.directIstioClient,
		NVS:         r.nVirtualSvcs,
		Concurrency: r.concurrency,
	}).Setup(ctx)
}

func (r *VirtualServiceRunner) NewSource(ctx context.Context) (source.Source, error) {
	return source.NewIstioVirtualServiceSource(ctx, r.benchKubeClient, r.benchIstioClient, &source.Config{LabelFilter: labels.Everything()})
}
