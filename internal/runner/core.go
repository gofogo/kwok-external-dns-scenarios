package runner

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/external-dns/source"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
	podfixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/pod"
	svcfixture "github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/service"
)

// PodRunner benchmarks source.NewPodSource.
type PodRunner struct {
	benchKubeClient  kubernetes.Interface
	directKubeClient kubernetes.Interface
	nPods            int
	concurrency      int
}

func NewPodRunner(
	benchKubeClient kubernetes.Interface,
	directCfg *rest.Config,
	nPods, concurrency int,
) (*PodRunner, error) {
	kubeClient, err := kubernetes.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("pod runner: build setup kube client: %w", err)
	}
	return &PodRunner{
		benchKubeClient:  benchKubeClient,
		directKubeClient: kubeClient,
		nPods:            nPods,
		concurrency:      concurrency,
	}, nil
}

func (r *PodRunner) Label() string      { return "pod" }
func (r *PodRunner) ResourceCount() int { return r.nPods }

func (r *PodRunner) Setup(ctx context.Context) error {
	return (&podfixture.PodFixture{
		KubeClient:  r.directKubeClient,
		NPods:       r.nPods,
		Concurrency: r.concurrency,
	}).Setup(ctx)
}

func (r *PodRunner) NewSource(ctx context.Context) (source.Source, error) {
	return source.NewPodSource(ctx, r.benchKubeClient, &source.Config{LabelFilter: labels.Everything()})
}

// ServiceRunner benchmarks source.NewServiceSource with a mix of headless and NodePort services.
// Nodes populate the node informer cache (NodePort target resolution).
// Pods populate the pod informer cache (headless service endpoint resolution).
type ServiceRunner struct {
	benchKubeClient  kubernetes.Interface
	directKubeClient kubernetes.Interface
	nServices        int
	svcDist          distribute.Weights
	nNodes           int
	nPods            int
	podDist          distribute.Weights
	concurrency      int
}

func NewServiceRunner(
	benchKubeClient kubernetes.Interface,
	directCfg *rest.Config,
	nServices, nNodes, nPods, concurrency int,
	opts ...ServiceRunnerOption,
) (*ServiceRunner, error) {
	kubeClient, err := kubernetes.NewForConfig(directCfg)
	if err != nil {
		return nil, fmt.Errorf("service runner: build setup kube client: %w", err)
	}
	r := &ServiceRunner{
		benchKubeClient:  benchKubeClient,
		directKubeClient: kubeClient,
		nServices:        nServices,
		nNodes:           nNodes,
		nPods:            nPods,
		concurrency:      concurrency,
	}
	for _, o := range opts {
		o(r)
	}
	return r, nil
}

func (r *ServiceRunner) Label() string      { return "service" }
func (r *ServiceRunner) ResourceCount() int { return r.nServices }

func (r *ServiceRunner) Setup(ctx context.Context) error {
	return (&svcfixture.ServiceFixture{
		KubeClient:  r.directKubeClient,
		NServices:   r.nServices,
		SvcDist:     r.svcDist,
		NNodes:      r.nNodes,
		NPods:       r.nPods,
		PodDist:     r.podDist,
		Concurrency: r.concurrency,
	}).Setup(ctx)
}

func (r *ServiceRunner) NewSource(ctx context.Context) (source.Source, error) {
	// Empty ServiceTypeFilter → all informers (service, endpointslice, pod, node) are started.
	return source.NewServiceSource(ctx, r.benchKubeClient, &source.Config{LabelFilter: labels.Everything()})
}
