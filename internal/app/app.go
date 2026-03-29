// Package app contains the top-level benchmark orchestration logic.
package app

import (
	"context"
	"fmt"
	"log"
	"time"

	istioclient "istio.io/client-go/pkg/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/cli"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/cluster"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/config"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/metrics"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/report"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/runner"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/transport"
)

// Run executes the full benchmark lifecycle and returns any fatal error.
func Run(ctx context.Context, flags cli.Flags, cfg config.Config) error {
	selected, err := config.SelectScenarios(cfg.Scenarios, flags.ScenarioName)
	if err != nil {
		return fmt.Errorf("select scenarios: %w", err)
	}

	step := 0
	next := func(msg string, args ...any) {
		step++
		log.Printf("Step %d: "+msg, append([]any{step}, args...)...)
	}

	// clusterExists prevents cleanup attempts against clusters that failed to create.
	clusterExists := flags.SkipSetup

	deleteCluster := func() {
		if !clusterExists {
			return
		}
		next("deleting cluster %q", flags.ClusterName)
		if err := cluster.Delete(flags.ClusterName); err != nil {
			log.Printf("WARN: delete cluster: %v", err)
		}
	}
	if flags.CleanupEnabled {
		defer deleteCluster()
	}

	errorf := func(format string, args ...any) error {
		err := fmt.Errorf(format, args...)
		if flags.CleanupEnabled && flags.CleanupOnError {
			deleteCluster()
		}
		return err
	}

	if !flags.SkipSetup {
		if err := cluster.CheckDocker(); err != nil {
			return fmt.Errorf("preflight: %w", err)
		}
		next("creating KWOK cluster %q", flags.ClusterName)
		if err := cluster.Create(flags.ClusterName); err != nil {
			return errorf("create cluster: %w", err)
		}
		clusterExists = true
	}

	next("reading kubeconfig for cluster %q", flags.ClusterName)
	kubeconfigPath, err := cluster.KubeconfigPath(flags.ClusterName)
	if err != nil {
		return errorf("get kubeconfig: %w", err)
	}

	if !flags.SkipSetup {
		next("waiting for API server to be ready")
		if err := cluster.WaitReady(kubeconfigPath, "kwok-"+flags.ClusterName, flags.WaitAttempts); err != nil {
			return errorf("wait ready: %w", err)
		}

		if config.AnyScenarioHasSource(selected, config.SourceIstio) {
			next("installing Istio CRDs")
			if err := cluster.ApplyCRDs(kubeconfigPath, config.IstioCRDs); err != nil {
				return errorf("apply Istio CRDs: %w", err)
			}
		}
		if config.AnyScenarioHasSource(selected, config.SourceDNSEndpoint) {
			next("installing DNSEndpoint CRD")
			if err := cluster.ApplyCRDs(kubeconfigPath, config.DNSEndpointCRD); err != nil {
				return errorf("apply DNSEndpoint CRD: %w", err)
			}
		}
	}

	next("building setup REST config (high QPS/Burst for concurrent fixture creation)")
	directCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return errorf("build setup REST config: %w", err)
	}
	directCfg.QPS = float32(flags.Concurrency) * 2
	directCfg.Burst = flags.Concurrency * 4

	next("building benchmark REST config (default rate limits, counting transport%s)",
		func() string {
			if flags.LatencyMs > 0 {
				return fmt.Sprintf(", toxiproxy latency=%dms jitter=%dms", flags.LatencyMs, flags.JitterMs)
			}
			return ""
		}())
	benchCfg, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		return errorf("build benchmark REST config: %w", err)
	}
	if flags.LatencyMs > 0 {
		benchCfg, err = withProxy(ctx, benchCfg, flags.LatencyMs, flags.JitterMs)
		if err != nil {
			return errorf("setup proxy: %w", err)
		}
		log.Printf("        → proxy endpoint: %s", benchCfg.Host)
	}
	apiCounter := transport.NewCountingTransport()
	benchCfg.WrapTransport = apiCounter.WrapTransport()

	next("creating benchmark Kubernetes client")
	benchKubeClient, err := kubernetes.NewForConfig(benchCfg)
	if err != nil {
		return errorf("create kube client: %w", err)
	}

	var benchIstioClient istioclient.Interface
	if config.AnyScenarioHasSource(selected, config.SourceIstio) {
		next("creating benchmark Istio client")
		benchIstioClient, err = istioclient.NewForConfig(benchCfg)
		if err != nil {
			return errorf("create istio client: %w", err)
		}
	}

	metricsEnabled := flags.MetricsEnabled
	if metricsEnabled {
		if err := metrics.StartServer(flags.MetricsURL); err != nil {
			log.Printf("WARN: could not start metrics server: %v — metrics disabled for this run", err)
			metricsEnabled = false
		} else {
			log.Printf("        → metrics server started at %s", flags.MetricsURL)
			for _, s := range selected {
				if len(s.Metrics) > 0 {
					if err := metrics.New(flags.MetricsURL, s.Metrics).Ping(); err != nil {
						log.Printf("WARN: metrics healthcheck failed: %v — metrics disabled for this run", err)
						metricsEnabled = false
					} else {
						log.Printf("        → metrics endpoint healthy")
					}
					break
				}
			}
		}
	}

	interval := time.Duration(flags.PauseMs) * time.Millisecond
	benchStart := time.Now()
	results := make([]report.ScenarioResult, 0, len(selected))

	var allRunners []runner.SourceRunner

	for _, scenario := range selected {
		next("Scenario: %s — %s", scenario.Name, scenario.Description)

		runners, err := runnersForScenario(scenario, benchKubeClient, benchIstioClient, benchCfg, directCfg, kubeconfigPath, flags.Concurrency, flags.KubeAPIQPS, flags.KubeAPIBurst)
		if err != nil {
			return errorf("build runners for scenario %q: %w", scenario.Name, err)
		}
		allRunners = append(allRunners, runners...)

		if !flags.SkipSetup {
			next("[%s] creating fixtures", scenario.Name)
			for _, sr := range runners {
				log.Printf("        → setting up %s fixtures", sr.Label())
				if err := sr.Setup(ctx); err != nil {
					return errorf("setup %s fixtures: %w", sr.Label(), err)
				}
			}
		}

		printInspectCommands(kubeconfigPath, runners)

		var scraper *metrics.Scraper
		if metricsEnabled && len(scenario.Metrics) > 0 {
			scraper = metrics.New(flags.MetricsURL, scenario.Metrics)
			log.Printf("        → scraping metrics from %s after each iteration", flags.MetricsURL)
		}

		var sourceResults []report.NamedSourceStats
		for _, sr := range runners {
			next("[%s] creating %s source and syncing informer cache", scenario.Name, sr.Label())
			warmupTimeout := time.Duration(flags.WarmupTimeoutMs) * time.Millisecond
		stats, err := runSourceBenchmark(ctx, sr, scenario.Iterations, apiCounter, scraper, interval, warmupTimeout)
			if err != nil {
				return errorf("%w", err)
			}
			sourceResults = append(sourceResults, report.NamedSourceStats{
				Label: sr.Label(),
				Stats: stats,
			})
		}

		results = append(results, report.ScenarioResult{
			Name:        scenario.Name,
			Description: scenario.Description,
			SourceType:  string(scenario.Source),
			Sources:     sourceResults,
			WallTime:    time.Since(benchStart),
		})
	}

	for _, res := range results {
		report.Print(res, flags.MetricsVerbose)
		if flags.SaveResults {
			if err := report.Write(flags.ClusterName, res, flags.LatencyMs, flags.JitterMs); err != nil {
				log.Printf("WARN: write results: %v", err)
			}
		}
	}

	fmt.Println("\n=================================================================")
	fmt.Println("=== Benchmark complete ===")
	fmt.Println("=================================================================")

	printClusterSummary(kubeconfigPath, flags.ClusterName, allRunners, flags.CleanupEnabled)
	return nil
}
