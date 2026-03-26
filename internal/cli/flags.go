// Package cli parses command-line flags for the benchmark binary.
package cli

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/config"
)

// Flags holds all resolved flag values.
type Flags struct {
	ClusterName     string
	ScenarioName    string
	MetricsEnabled  bool
	MetricsURL      string
	MetricsVerbose  bool
	PauseMs         int
	SkipSetup       bool
	CleanupEnabled  bool
	CleanupOnError  bool
	LatencyMs       int
	JitterMs        int
	WaitAttempts    int
	Concurrency     int
	SaveResults     bool
	ConfigFile      string
}

// MustParse parses args and returns resolved Flags and the loaded Config.
// Exits on --help or bad flags.
func MustParse(args []string) (Flags, config.Config) {
	configFile := configFileFromArgs(args)

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load config: %v\n", err)
		os.Exit(1)
	}

	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	fs.Usage = usage(fs)
	fs.String("config", configFile, "path to YAML config file")
	clusterName    := fs.String("cluster-name", cfg.ClusterName, "KWOK cluster name")
	scenarioFlag   := fs.String("scenario", "", fmt.Sprintf("run only this named scenario (default: all with default:true); available: %s", config.ScenarioNames(cfg.Scenarios)))
	metricsEnabled := fs.Bool("metrics-enabled", cfg.Metrics.Enabled, "start an in-process metrics server and scrape per-iteration Prometheus metrics")
	metricsURL     := fs.String("metrics-url", cfg.Metrics.URL, "address for the in-process metrics server and scrape endpoint")
	metricsVerbose := fs.Bool("metrics-verbose", false, "show per-iteration metric deltas in addition to per-test totals")
	pauseMs        := fs.Int("pause-ms", cfg.PauseMsInt(), "delay between iterations in ms (0 = no delay); accepts 500 or 500ms")
	skipSetup      := fs.Bool("skip-setup", cfg.SkipSetup, "skip cluster creation and fixture loading (reuse existing cluster)")
	cleanupEnabled := fs.Bool("cleanup", cfg.Cleanup.Enabled, "delete cluster after benchmarking")
	cleanupOnError := fs.Bool("cleanup-on-error", cfg.Cleanup.OnError, "delete cluster even when the benchmark exits with an error")
	latencyMs      := fs.Int("latency-ms", cfg.LatencyMsInt(), "API server latency via toxiproxy in ms (0 = disabled); accepts 50 or 50ms")
	jitterMs       := fs.Int("jitter-ms", cfg.JitterMsInt(), "latency jitter added on top of latency-ms; same format")
	waitAttempts   := fs.Int("wait-attempts", cfg.WaitAttempts, "max attempts when polling for API server readiness (2s between each)")
	concurrency    := fs.Int("concurrency", cfg.Concurrency, "number of concurrent API requests during fixture creation")
	saveResults    := fs.Bool("save-results", cfg.SaveResults, "append machine-readable results to <cluster-name>-results.txt after each scenario")
	fs.Parse(args) //nolint:errcheck // ExitOnError handles errors

	flags := Flags{
		ClusterName:    *clusterName,
		ScenarioName:   *scenarioFlag,
		MetricsEnabled: *metricsEnabled,
		MetricsURL:     *metricsURL,
		MetricsVerbose: *metricsVerbose,
		PauseMs:        *pauseMs,
		SkipSetup:      *skipSetup,
		CleanupEnabled: *cleanupEnabled,
		CleanupOnError: *cleanupOnError,
		LatencyMs:      *latencyMs,
		JitterMs:       *jitterMs,
		WaitAttempts:   *waitAttempts,
		Concurrency:    *concurrency,
		SaveResults:    *saveResults,
		ConfigFile:     configFile,
	}
	return flags, cfg
}

// configFileFromArgs scans args for --config or -config (with = or space separator)
// without invoking flag.Parse, so that --help is always handled by the full flag set.
func configFileFromArgs(args []string) string {
	for i, arg := range args {
		for _, prefix := range []string{"--config=", "-config="} {
			if strings.HasPrefix(arg, prefix) {
				return strings.TrimPrefix(arg, prefix)
			}
		}
		if (arg == "--config" || arg == "-config") && i+1 < len(args) {
			return args[i+1]
		}
	}
	return "bench.yaml"
}

func usage(fs *flag.FlagSet) func() {
	return func() {
		fmt.Fprintf(os.Stderr, `ext-dns-kwok-bench — benchmark external-dns sources against a KWOK cluster.

Usage:
  go run . [flags]

Flags:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Config file (bench.yaml):
  All flags can be set in a YAML file loaded via --config (default: bench.yaml).
  CLI flags take precedence over the file. Example:

    cluster-name: ext-dns-bench   # KWOK cluster name
    skip-setup:   false           # true = reuse existing cluster + fixtures
    cleanup:      false           # true = delete cluster after run
    latency-ms:   50ms            # injected API server latency (0 = off)
    jitter-ms:    10ms            # ± jitter on top of latency-ms
    scenarios:
      - name: istio-prod-scale
        source: istio
        description: Production-scale Istio load
        default: true
        iterations: 20
        resources:
          services: 4000
          gateways: 3000
          virtualservices: 1000

      - name: pod-quick-smoke
        source: pod
        default: false
        iterations: 5
        resources:
          pods: 20

      - name: dnsendpoint-quick-smoke
        source: dnsendpoint
        default: false
        iterations: 5
        resources:
          dnsendpoints: 20

Cross-branch comparison:
  1. Run on branch A (skip-setup: false) — creates cluster and fixtures.
  2. Edit bench.yaml: skip-setup: true
  3. Check out branch B, recompile, run again — reuses the same cluster.
  4. Compare <cluster-name>-results.txt.
`)
	}
}
