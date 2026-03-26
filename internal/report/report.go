// Package report formats and writes benchmark results.
package report

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/bench"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/metrics"
)

// CallStats holds timing statistics for one normalized API call type.
type CallStats struct {
	Count          int
	Min, Max, Mean time.Duration
	P50, P99       time.Duration
}

// ComputeCallStats computes timing statistics for a slice of durations.
func ComputeCallStats(ds []time.Duration) CallStats {
	if len(ds) == 0 {
		return CallStats{}
	}
	sorted := make([]time.Duration, len(ds))
	copy(sorted, ds)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	var total time.Duration
	for _, d := range ds {
		total += d
	}
	return CallStats{
		Count: len(ds),
		Min:   sorted[0],
		Max:   sorted[len(sorted)-1],
		Mean:  total / time.Duration(len(ds)),
		P50:   sorted[int(float64(len(sorted))*0.50)],
		P99:   sorted[int(float64(len(sorted))*0.99)],
	}
}

// SourceStats holds all benchmark results for a single source.
type SourceStats struct {
	Stats           bench.Stats
	Snapshots       []metrics.Sample
	Deltas          []map[string]float64
	SourceCount     int // number of source objects benchmarked
	EndpointCount   int // endpoints returned by the last Endpoints() call
	SetupBreakdown  map[string]CallStats   // kube API calls during source creation (LIST + WATCH)
	WarmupBreakdown map[string]CallStats   // kube API calls during first Endpoints() call
	IterBreakdowns  []map[string]CallStats // kube API calls per iteration (iter 1 = warmup, iter 2+ = steady)
}

// NamedSourceStats pairs a runner label with its benchmark results.
type NamedSourceStats struct {
	Label string // "gateway", "virtualservice", "pod", "dnsendpoint"
	Stats SourceStats
}

// ScenarioResult holds everything produced by one scenario run.
type ScenarioResult struct {
	Name        string
	Description string
	SourceType  string // "istio", "pod", "dnsendpoint"
	Sources     []NamedSourceStats
	WallTime    time.Duration
}

// Print writes a human-readable result to stdout.
func Print(res ScenarioResult, metricsVerbose bool) {
	fmt.Println()
	fmt.Printf("=== Scenario: %s — %s ===\n", res.Name, res.Description)
	var totalActive time.Duration
	for _, ns := range res.Sources {
		fmt.Printf("--- %s (sources: %d, endpoints: %d) ---\n",
			sourceDisplayName(ns.Label), ns.Stats.SourceCount, ns.Stats.EndpointCount)
		printStats(ns.Stats, metricsVerbose)
		fmt.Println()
		totalActive += ns.Stats.Stats.ActiveTime + ns.Stats.Stats.FirstCall
	}
	fmt.Printf("  total elapsed   : %v  (calls: %v + pauses+sync: %v)\n",
		res.WallTime.Round(time.Second),
		totalActive.Round(time.Millisecond),
		(res.WallTime - totalActive).Round(time.Second),
	)
}

// Write appends a machine-readable result line to <clusterName>-results.txt.
func Write(clusterName string, res ScenarioResult, latencyMs, jitterMs int) error {
	f, err := os.OpenFile(clusterName+"-results.txt", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	fmt.Fprintf(f, "# %s  scenario=%s  source=%s  latency=%dms  jitter=%dms  time=%s\n",
		clusterName, res.Name, res.SourceType, latencyMs, jitterMs, time.Now().Format(time.RFC3339))
	for _, ns := range res.Sources {
		s := ns.Stats.Stats
		fmt.Fprintf(f, "%s/N=%d  first=%v(%.2f qps)  mean=%v  p50=%v  p99=%v  min=%v  max=%v  qps=%.2f  wall=%v  api_reqs=%d\n",
			sourceDisplayName(ns.Label), ns.Stats.SourceCount,
			s.FirstCall.Round(time.Millisecond), s.FirstQPS,
			s.Mean.Round(time.Millisecond), s.P50.Round(time.Millisecond),
			s.P99.Round(time.Millisecond), s.Min.Round(time.Millisecond), s.Max.Round(time.Millisecond),
			s.QPS, s.WallTime.Round(time.Second), s.APIRequests)
	}
	return nil
}

// sourceDisplayName maps a runner label to a human-readable source class name.
func sourceDisplayName(label string) string {
	switch label {
	case "gateway":
		return "GatewaySource"
	case "virtualservice":
		return "VirtualServiceSource"
	case "pod":
		return "PodSource"
	case "dnsendpoint":
		return "CRDSource/DNSEndpoint"
	default:
		return label
	}
}

func printStats(src SourceStats, metricsVerbose bool) {
	s := src.Stats
	n := s.Count + 1 // total iterations including warmup

	fmt.Println("  --- Endpoints() call latency ---")
	fmt.Printf("  warmup  (iter 1)    %v\n", s.FirstCall.Round(time.Millisecond))
	if s.Count > 0 {
		wall := s.FirstCall + s.WallTime
		active := s.FirstCall + s.ActiveTime
		fmt.Printf("  steady  (iter 2–%d)  min=%v  max=%v  mean=%v  p50=%v  p99=%v  %.0f calls/s\n",
			n,
			s.Min.Round(time.Millisecond), s.Max.Round(time.Millisecond),
			s.Mean.Round(time.Millisecond), s.P50.Round(time.Millisecond), s.P99.Round(time.Millisecond),
			s.QPS,
		)
		fmt.Printf("  wall                %v  (active: %v + pauses: %v)\n",
			wall.Round(time.Second),
			active.Round(time.Millisecond),
			(wall - active).Round(time.Second),
		)
	}

	apiPerCall := float64(s.APIRequests) / float64(n)
	fmt.Println()
	fmt.Println("  --- kube api requests ---")
	fmt.Printf("  total: %d   per call: %.2f  (%d kube requests / %d Endpoints() calls)\n",
		s.APIRequests, apiPerCall, s.APIRequests, n)

	// Method-level totals across all phases (setup + all benchmark iterations).
	all := src.SetupBreakdown
	for _, bd := range src.IterBreakdowns {
		all = mergeMaps(all, bd)
	}
	byMethod, methods := aggregateByMethod(all)
	for _, m := range methods {
		cs := byMethod[m]
		fmt.Printf("  %-10s %d calls  mean=%v  p50=%v  p99=%v\n",
			m+":", cs.Count,
			cs.Mean.Round(time.Millisecond), cs.P50.Round(time.Millisecond), cs.P99.Round(time.Millisecond),
		)
	}

	fmt.Println()
	fmt.Println("  --- kube api breakdown ---")
	printCallBreakdown("source setup (before benchmark)", src.SetupBreakdown)
	fmt.Println()
	benchmarkBreakdown := make(map[string]CallStats)
	for _, bd := range src.IterBreakdowns {
		benchmarkBreakdown = mergeMaps(benchmarkBreakdown, bd)
	}
	printCallBreakdown(fmt.Sprintf("iter 1–%d (benchmark)", n), benchmarkBreakdown)
	if len(src.IterBreakdowns) > 0 {
		fmt.Println()
		fmt.Printf("  iter 1–%d (per-iteration):\n", n)
		for i, bd := range src.IterBreakdowns {
			if len(bd) == 0 {
				fmt.Printf("    iter %d: none\n", i+1)
				continue
			}
			fmt.Printf("    iter %d:\n", i+1)
			printCallEntries(bd)
		}
	}

	if len(src.Snapshots) == 0 {
		return
	}

	keys := metrics.AllKeys(src.Snapshots)
	sort.Strings(keys)
	var types map[string]string
	for _, snap := range src.Snapshots {
		if len(snap.Types) > 0 {
			types = snap.Types
			break
		}
	}
	totalDeltas := metrics.SumDeltas(src.Deltas)

	fmt.Println()
	fmt.Println("  --- memory ---")
	for _, k := range keys {
		name := stripMetricPrefix(k)
		if metrics.IsGauge(types, k) {
			fmt.Printf("  %-30s mean=%s\n", name, formatMetricValue(k, metrics.MeanValues(src.Snapshots, k)))
		} else {
			nd := float64(len(src.Deltas))
			total := totalDeltas[k]
			fmt.Printf("  %-30s delta=%s  per-iter=%s\n", name, formatMetricValue(k, total), formatMetricValue(k, total/nd))
		}
	}

	if metricsVerbose {
		fmt.Println()
		fmt.Println("  --- memory (per-iteration) ---")
		for i, snap := range src.Snapshots {
			fmt.Printf("  iter %d:\n", i+1)
			for _, k := range keys {
				name := stripMetricPrefix(k)
				if metrics.IsGauge(types, k) {
					fmt.Printf("    %-28s %s\n", name, formatMetricValue(k, snap.Values[k]))
				} else {
					fmt.Printf("    %-28s delta=%s\n", name, formatMetricValue(k, src.Deltas[i][k]))
				}
			}
		}
	}
}

func printCallBreakdown(label string, breakdown map[string]CallStats) {
	fmt.Printf("  %s:\n", label)
	if len(breakdown) == 0 {
		fmt.Println("    none")
		return
	}

	byMethod, methods := aggregateByMethod(breakdown)
	for _, m := range methods {
		cs := byMethod[m]
		fmt.Printf("    %-10s %d calls  mean=%v  p50=%v  p99=%v\n",
			m+":", cs.Count,
			cs.Mean.Round(time.Millisecond), cs.P50.Round(time.Millisecond), cs.P99.Round(time.Millisecond),
		)
	}

	printCallEntries(breakdown)
}

// aggregateByMethod groups a call breakdown map by HTTP method (GET, WATCH, etc.),
// merging stats for all paths under each method. Returns the grouped map and sorted method keys.
func aggregateByMethod(breakdown map[string]CallStats) (map[string]CallStats, []string) {
	byMethod := make(map[string]CallStats)
	for k, v := range breakdown {
		method := strings.SplitN(k, " ", 2)[0]
		byMethod[method] = mergeCallStats(byMethod[method], v)
	}
	methods := make([]string, 0, len(byMethod))
	for m := range byMethod {
		methods = append(methods, m)
	}
	sort.Strings(methods)
	return byMethod, methods
}

// printCallEntries prints per-path call stats sorted by count descending then key ascending.
func printCallEntries(entries map[string]CallStats) {
	type kv struct {
		k string
		v CallStats
	}
	pairs := make([]kv, 0, len(entries))
	for k, v := range entries {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v.Count != pairs[j].v.Count {
			return pairs[i].v.Count > pairs[j].v.Count
		}
		return pairs[i].k < pairs[j].k
	})
	for _, p := range pairs {
		cs := p.v
		note := ""
		if strings.HasPrefix(p.k, "WATCH ") {
			note = "  (background reconnect)"
		}
		if cs.Count == 1 {
			fmt.Printf("      %-43s 1 call   mean=%v%s\n", p.k, cs.Mean.Round(time.Millisecond), note)
		} else {
			fmt.Printf("      %-43s %d calls  mean=%v  p50=%v  p99=%v%s\n",
				p.k, cs.Count,
				cs.Mean.Round(time.Millisecond), cs.P50.Round(time.Millisecond), cs.P99.Round(time.Millisecond),
				note,
			)
		}
	}
}

// mergeMaps combines two call breakdown maps, summing counts and recalculating stats.
func mergeMaps(a, b map[string]CallStats) map[string]CallStats {
	out := make(map[string]CallStats, len(a)+len(b))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if existing, ok := out[k]; ok {
			out[k] = mergeCallStats(existing, v)
		} else {
			out[k] = v
		}
	}
	return out
}

func mergeCallStats(a, b CallStats) CallStats {
	if a.Count == 0 {
		return b
	}
	if b.Count == 0 {
		return a
	}
	count := a.Count + b.Count
	mn := a.Min
	if b.Min < mn {
		mn = b.Min
	}
	mx := a.Max
	if b.Max > mx {
		mx = b.Max
	}
	mean := (a.Mean*time.Duration(a.Count) + b.Mean*time.Duration(b.Count)) / time.Duration(count)
	// p50/p99 can't be exactly recomputed without raw data; use weighted approximation
	p50 := (a.P50*time.Duration(a.Count) + b.P50*time.Duration(b.Count)) / time.Duration(count)
	p99 := (a.P99*time.Duration(a.Count) + b.P99*time.Duration(b.Count)) / time.Duration(count)
	return CallStats{Count: count, Min: mn, Max: mx, Mean: mean, P50: p50, P99: p99}
}

func formatMetricValue(key string, v float64) string {
	if strings.HasSuffix(strings.SplitN(key, "{", 2)[0], "_bytes") {
		return fmt.Sprintf("%.1f KB", v/1024)
	}
	return fmt.Sprintf("%.0f", v)
}

func stripMetricPrefix(key string) string {
	if i := strings.IndexByte(key, '{'); i >= 0 {
		key = key[:i]
	}
	for _, pfx := range []string{"go_memstats_", "go_", "process_"} {
		if strings.HasPrefix(key, pfx) {
			return strings.TrimPrefix(key, pfx)
		}
	}
	return key
}
