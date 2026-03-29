package app

import (
	"fmt"

	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/config"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/runner"
)

// runnersForScenario builds the ordered list of SourceRunners for a scenario.
// For "istio" returns [GatewayRunner, VirtualServiceRunner].
// For "pod" and "dnsendpoint" returns a single-element slice.
func runnersForScenario(
	s config.Scenario,
	benchKubeClient kubernetes.Interface,
	benchIstioClient istioclient.Interface,
	benchCfg *rest.Config,
	directCfg *rest.Config,
	kubeconfigPath string,
	concurrency int,
	crdClientQPS float32,
	crdClientBurst int,
) ([]runner.SourceRunner, error) {
	r := s.Resources
	switch s.Source {
	case config.SourceIstio:
		gw, err := runner.NewGatewayRunner(benchKubeClient, benchIstioClient, directCfg, r.Services.Count, r.Gateways, concurrency)
		if err != nil {
			return nil, err
		}
		vs, err := runner.NewVirtualServiceRunner(benchKubeClient, benchIstioClient, directCfg, r.VirtualSvcs, concurrency)
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{gw, vs}, nil
	case config.SourcePod:
		pr, err := runner.NewPodRunner(benchKubeClient, directCfg, r.Pods.Count, concurrency)
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{pr}, nil
	case config.SourceDNSEndpoint:
		ep, err := runner.NewDNSEndpointRunner(benchKubeClient, directCfg, concurrency, runner.DNSEndpointConfig{
			NEndpoints:  r.DNSEndpoints.Count,
			NsDist:      r.DNSEndpoints.Distribution.Namespaces,
			ClientQPS:   crdClientQPS,
			ClientBurst: crdClientBurst,
			BenchCfg:    benchCfg,
		})
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{ep}, nil
	case config.SourceService:
		sr, err := runner.NewServiceRunner(benchKubeClient, directCfg, r.Services.Count, r.Nodes, r.Pods.Count, concurrency,
			runner.WithSvcDist(r.Services.Distribution.ServiceType),
			runner.WithPodDist(r.Pods.Distribution.ServiceType),
		)
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{sr}, nil
	default:
		return nil, fmt.Errorf("unknown source %q in scenario %q; valid values: %s, %s, %s, %s",
			s.Source, s.Name, config.SourceIstio, config.SourcePod, config.SourceDNSEndpoint, config.SourceService)
	}
}
