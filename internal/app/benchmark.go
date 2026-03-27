package app

import (
	"context"
	"fmt"
	"log"
	"time"

	"k8s.io/client-go/rest"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/bench"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/metrics"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/proxy"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/report"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/runner"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/transport"
)

// iterObs implements bench.IterObserver. After each steady-state iteration it:
//   - captures the API call breakdown for that iteration window
//   - takes a Prometheus snapshot and records the delta (if a scraper is configured)
//
// Both observations are collected outside the timed fn call so they don't
// inflate benchmark measurements.
type iterObs struct {
	apiCounter     *transport.CountingTransport
	iterIdx        map[string]int
	iterBreakdowns []map[string]report.CallStats

	scraper   *metrics.Scraper
	prevSnap  metrics.Sample
	snapshots []metrics.Sample
	deltas    []map[string]float64
}

func (o *iterObs) AfterIter() {
	o.iterBreakdowns = append(o.iterBreakdowns, o.apiCounter.CallWindow(o.iterIdx))
	o.iterIdx = o.apiCounter.SnapshotIndexes()

	if o.scraper == nil {
		return
	}
	snap, err := o.scraper.Snapshot()
	if err != nil {
		log.Printf("WARN: scrape metrics: %v", err)
		return
	}
	o.deltas = append(o.deltas, metrics.Delta(o.prevSnap, snap))
	o.snapshots = append(o.snapshots, snap)
	o.prevSnap = snap
}

// withProxy starts an embedded toxiproxy in front of the API server.
func withProxy(ctx context.Context, cfg *rest.Config, latencyMs, jitterMs int) (*rest.Config, error) {
	p, err := proxy.Start(ctx, cfg.Host, latencyMs, jitterMs)
	if err != nil {
		return nil, err
	}
	copy := *cfg
	copy.Host = p.Endpoint()
	copy.TLSClientConfig.Insecure = true
	copy.TLSClientConfig.CAFile = ""
	copy.TLSClientConfig.CAData = nil
	return &copy, nil
}

// runSourceBenchmark runs the full two-phase benchmark (warmup + steady) for one
// SourceRunner and returns the collected SourceStats.
func runSourceBenchmark(
	ctx context.Context,
	sr runner.SourceRunner,
	iterations int,
	apiCounter *transport.CountingTransport,
	scraper *metrics.Scraper,
	interval time.Duration,
	warmupTimeout time.Duration,
) (report.SourceStats, error) {
	t0 := time.Now()
	setupIdx := apiCounter.SnapshotIndexes()
	src, err := sr.NewSource(ctx)
	if err != nil {
		return report.SourceStats{}, fmt.Errorf("[%s] create source: %w", sr.Label(), err)
	}
	setupBreakdown := apiCounter.CallWindow(setupIdx)
	log.Printf("        → synced in %v", time.Since(t0).Round(time.Millisecond))

	// Baseline Prometheus snapshot before the warmup call.
	var initSnap metrics.Sample
	if scraper != nil {
		initSnap, _ = scraper.Snapshot()
	}

	var endpointCount int
	fn := func(ctx context.Context) error {
		eps, err := src.Endpoints(ctx)
		endpointCount = len(eps)
		return err
	}

	bRunner := bench.NewRunner(sr.Label(), iterations, &apiCounter.Count)
	warmupIdx := apiCounter.SnapshotIndexes()

	warmupCtx := ctx
	var warmupCancel context.CancelFunc
	if warmupTimeout > 0 {
		warmupCtx, warmupCancel = context.WithTimeout(ctx, warmupTimeout)
		defer warmupCancel()
	}
	first, err := bRunner.First(warmupCtx, fn)
	if err != nil {
		return report.SourceStats{}, fmt.Errorf("[%s] warmup: %w", sr.Label(), err)
	}
	warmupBreakdown := apiCounter.CallWindow(warmupIdx)

	obs := &iterObs{
		apiCounter:     apiCounter,
		iterIdx:        apiCounter.SnapshotIndexes(),
		iterBreakdowns: []map[string]report.CallStats{warmupBreakdown},
		scraper:        scraper,
		prevSnap:       initSnap,
	}

	// Capture warmup metrics outside the timing window, consistent with AfterIter.
	if scraper != nil {
		if snap, scrapeErr := scraper.Snapshot(); scrapeErr != nil {
			log.Printf("WARN: scrape metrics (warmup): %v", scrapeErr)
		} else {
			obs.deltas = append(obs.deltas, metrics.Delta(obs.prevSnap, snap))
			obs.snapshots = append(obs.snapshots, snap)
			obs.prevSnap = snap
		}
	}

	stats, err := bRunner.Steady(ctx, fn, iterations-1, interval, obs)
	if err != nil {
		return report.SourceStats{}, fmt.Errorf("[%s] benchmark: %w", sr.Label(), err)
	}
	stats.FirstCall = first
	if first > 0 {
		stats.FirstQPS = 1.0 / first.Seconds()
	}

	return report.SourceStats{
		Stats:           stats,
		Snapshots:       obs.snapshots,
		Deltas:          obs.deltas,
		SourceCount:     sr.ResourceCount(),
		EndpointCount:   endpointCount,
		SetupBreakdown:  setupBreakdown,
		WarmupBreakdown: warmupBreakdown,
		IterBreakdowns:  obs.iterBreakdowns,
	}, nil
}
