package app

import (
	"fmt"

	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/config"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/runner"
)


// RunnerEnv holds the shared cluster clients and settings used by all runners.
type RunnerEnv struct {
	BenchKubeClient  kubernetes.Interface
	BenchIstioClient istioclient.Interface
	BenchCfg         *rest.Config
	DirectCfg        *rest.Config
	KubeconfigPath   string
	Concurrency      int
	KubeAPIQPS       float32
	KubeAPIBurst     int
}

// runnersForScenario builds the ordered list of SourceRunners for a scenario.
// For "istio" returns [GatewayRunner, VirtualServiceRunner].
// For "pod" and "dnsendpoint" returns a single-element slice.
func runnersForScenario(s config.Scenario, env RunnerEnv) ([]runner.SourceRunner, error) {
	r := s.Resources
	switch s.Source {
	case config.SourceIstio:
		gw, err := runner.NewGatewayRunner(env.BenchKubeClient, env.BenchIstioClient, env.DirectCfg, r.Services.Count, r.Gateways, env.Concurrency)
		if err != nil {
			return nil, err
		}
		vs, err := runner.NewVirtualServiceRunner(env.BenchKubeClient, env.BenchIstioClient, env.DirectCfg, r.VirtualSvcs, env.Concurrency)
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{gw, vs}, nil
	case config.SourcePod:
		pr, err := runner.NewPodRunner(env.BenchKubeClient, env.DirectCfg, r.Pods.Count, env.Concurrency)
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{pr}, nil
	case config.SourceDNSEndpoint:
		ep, err := runner.NewDNSEndpointRunner(env.BenchKubeClient, env.DirectCfg, env.Concurrency, runner.DNSEndpointConfig{
			NEndpoints:     r.DNSEndpoints.Count,
			NsDist:         r.DNSEndpoints.Distribution.Namespaces,
			BenchCfg:       env.BenchCfg,
			KubeconfigPath: env.KubeconfigPath,
			KubeAPIQPS:     env.KubeAPIQPS,
			KubeAPIBurst:   env.KubeAPIBurst,
		})
		if err != nil {
			return nil, err
		}
		return []runner.SourceRunner{ep}, nil
	case config.SourceService:
		sr, err := runner.NewServiceRunner(env.BenchKubeClient, env.DirectCfg, r.Services.Count, r.Nodes, r.Pods.Count, env.Concurrency,
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
