// Package config provides benchmark configuration types, loading, and scenario selection.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"
)

// Source identifies the external-dns source type for a scenario.
type Source string

// Source constants correspond to bench.yaml source: values.
const (
	SourceIstio       Source = "istio"
	SourcePod         Source = "pod"
	SourceDNSEndpoint Source = "dnsendpoint"
	SourceService     Source = "service"
)

// millis unmarshals from either a plain integer (50) or a duration string ("50ms").
type millis int

func (m *millis) UnmarshalJSON(data []byte) error {
	// try plain integer first
	var i int
	if err := json.Unmarshal(data, &i); err == nil {
		*m = millis(i)
		return nil
	}
	// try duration string: "50ms", "1s", etc.
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q (use e.g. 50 or \"50ms\"): %w", s, err)
	}
	*m = millis(d.Milliseconds())
	return nil
}

// Distribution holds optional per-dimension weighting for a resource count.
// Each field is a separate distribution axis; omit any that are not needed.
//
//	distribution:
//	  service-type:      # headless vs node-port split (services and pods)
//	    headless: 1
//	    node-port: 2
//	  namespaces:        # spread across namespaces (all resource types)
//	    dev: 10
//	    staging: 5
//	    default: 5
type Distribution struct {
	ServiceType distribute.Weights `json:"service-type,omitempty"`
	Namespaces  distribute.Weights `json:"namespaces,omitempty"`
}

// ResourceCount holds a resource count and optional distribution weights.
// Unmarshals from either a plain integer or a struct form:
//
//	services: 3                      # plain int → {Count: 3}
//	services:                        # struct form
//	  count: 3
//	  distribution:
//	    service-type:
//	      headless: 1
//	      node-port: 2
//	    namespaces:
//	      dev: 2
//	      staging: 1
type ResourceCount struct {
	Count        int          `json:"count"`
	Distribution Distribution `json:"distribution,omitempty"`
}

func (r *ResourceCount) UnmarshalJSON(data []byte) error {
	var n int
	if err := json.Unmarshal(data, &n); err == nil {
		r.Count = n
		return nil
	}
	type alias ResourceCount
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*r = ResourceCount(a)
	return nil
}

// Resources holds the scale parameters for a single benchmark scenario.
type Resources struct {
	Services     ResourceCount `json:"services"`
	Gateways     int           `json:"gateways"`
	VirtualSvcs  int           `json:"virtualservices"`
	Pods         ResourceCount `json:"pods"`
	DNSEndpoints ResourceCount `json:"dnsendpoints"`
	Nodes        int           `json:"nodes"`
}

// Scenario is one named benchmark configuration.
type Scenario struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Source      Source    `json:"source"`
	Default     bool      `json:"default"`
	Iterations  int       `json:"iterations"`
	Resources   Resources `json:"resources"`
	Metrics     []string  `json:"metrics"` // Prometheus metric family names to capture per iteration
}

// MetricsConfig groups settings for the in-process metrics server and scraping.
type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

// CleanupConfig controls whether and when the KWOK cluster is deleted after the benchmark.
type CleanupConfig struct {
	Enabled bool `json:"enabled"`
	// OnError controls whether the cluster is deleted even when the benchmark exits with an error.
	// false (default): cluster is preserved on error so the state can be inspected.
	// true: cluster is always deleted.
	OnError bool `json:"onError"`
}

// Config holds all benchmark parameters. YAML keys match the long flag names.
type Config struct {
	ClusterName    string        `json:"cluster-name"`
	Scenarios      []Scenario    `json:"scenarios"`
	Metrics        MetricsConfig `json:"metrics"`
	PauseMs        millis        `json:"pause-ms"`
	WarmupTimeout  millis        `json:"warmup-timeout"`
	SkipSetup      bool          `json:"skip-setup"`
	Cleanup        CleanupConfig `json:"cleanup"`
	LatencyMs      millis        `json:"latency-ms"`
	JitterMs       millis        `json:"jitter-ms"`
	WaitAttempts   int           `json:"wait-attempts"`
	Concurrency    int           `json:"concurrency"`
	KubeAPIQPS     int           `json:"kube-api-qps"`
	KubeAPIBurst   int           `json:"kube-api-burst"`
	SaveResults    bool          `json:"save-results"`
}

// PauseMsInt returns PauseMs as an int.
func (c Config) PauseMsInt() int { return int(c.PauseMs) }

// WarmupTimeoutMs returns WarmupTimeout as an int (0 = no timeout).
func (c Config) WarmupTimeoutMsInt() int { return int(c.WarmupTimeout) }

// LatencyMsInt returns LatencyMs as an int.
func (c Config) LatencyMsInt() int { return int(c.LatencyMs) }

// JitterMsInt returns JitterMs as an int.
func (c Config) JitterMsInt() int { return int(c.JitterMs) }

// DefaultConfig returns the built-in default configuration.
func DefaultConfig() Config {
	return Config{
		ClusterName:    "ext-dns-bench",
		WaitAttempts:   10,
		Concurrency:    100,
		KubeAPIQPS:   0,
		KubeAPIBurst: 0,
		Metrics:      MetricsConfig{Enabled: false, URL: "http://localhost:7979/metrics"},
		Scenarios: []Scenario{
			{
				Name:        "istio-quick-smoke",
				Description: "Minimal Istio fixture set for fast iteration during development",
				Source:      SourceIstio,
				Default:     true,
				Iterations:  10,
				Resources:   Resources{Services: ResourceCount{Count: 1}, Gateways: 10, VirtualSvcs: 10},
			},
		},
	}
}

// LoadConfig reads and parses the config file at path, falling back to DefaultConfig
// if the file does not exist.
func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil // no file → pure defaults
		}
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %q: %w", path, err)
	}
	var defaults []string
	for _, s := range cfg.Scenarios {
		if s.Default {
			defaults = append(defaults, s.Name)
		}
	}
	if len(defaults) > 1 {
		return cfg, fmt.Errorf("at most one scenario may have default: true; found %d: %s", len(defaults), strings.Join(defaults, ", "))
	}
	return cfg, nil
}

// SelectScenarios returns the scenarios to run based on the --scenario flag.
// If scenarioName is set, returns the single matching scenario (error if not found).
// If empty, returns all scenarios with Default: true.
func SelectScenarios(scenarios []Scenario, scenarioName string) ([]Scenario, error) {
	if scenarioName != "" {
		for _, s := range scenarios {
			if s.Name == scenarioName {
				return []Scenario{s}, nil
			}
		}
		return nil, fmt.Errorf("scenario %q not found; available: %s", scenarioName, ScenarioNames(scenarios))
	}
	var out []Scenario
	for _, s := range scenarios {
		if s.Default {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no default scenarios defined; use --scenario <name> or set default: true on at least one scenario")
	}
	return out, nil
}

// ScenarioNames returns a comma-separated list of scenario names.
func ScenarioNames(scenarios []Scenario) string {
	names := make([]string, len(scenarios))
	for i, s := range scenarios {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

// AnyScenarioHasSource reports whether any selected scenario uses the given source type.
func AnyScenarioHasSource(scenarios []Scenario, src Source) bool {
	for _, s := range scenarios {
		if s.Source == src {
			return true
		}
	}
	return false
}
