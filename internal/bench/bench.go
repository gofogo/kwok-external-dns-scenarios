package bench

import (
	"context"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

// Stats holds benchmark timing statistics split into the first iteration
// (informer/cache warmup) and all subsequent steady-state iterations.
type Stats struct {
	// First iteration — may be slower due to lazy init or cache warming.
	FirstCall time.Duration
	FirstQPS  float64 // 1 / FirstCall.Seconds()

	// Subsequent iterations (steady-state).
	Count      int           // iterations - 1
	Min        time.Duration
	Max        time.Duration
	Mean       time.Duration
	P50        time.Duration
	P99        time.Duration
	ActiveTime time.Duration // sum of steady-state call durations (excludes pauses)
	WallTime   time.Duration // elapsed clock time for the full run (active calls + pauses + first call)
	QPS        float64       // Count / ActiveTime.Seconds()

	// Total HTTP requests sent to the API server during the entire run.
	APIRequests int64
}

// Runner manages a two-phase benchmark: a single warmup iteration followed by
// steady-state iterations, all rendered on one shared progress bar.
type Runner struct {
	bar        *progress.Bar
	wallStart  time.Time
	apiCounter *atomic.Int64
	startReqs  int64
}

// NewRunner creates a Runner and starts the progress bar for totalIters iterations.
func NewRunner(label string, totalIters int, apiCounter *atomic.Int64) *Runner {
	return &Runner{
		bar:        progress.New(label, totalIters),
		wallStart:  time.Now(),
		apiCounter: apiCounter,
		startReqs:  snapshotCounter(apiCounter),
	}
}

// First executes the warmup iteration and returns its duration.
func (r *Runner) First(ctx context.Context, fn func(context.Context) error) (time.Duration, error) {
	t0 := time.Now()
	if err := fn(ctx); err != nil {
		r.bar.Done()
		return 0, fmt.Errorf("warmup iteration failed: %w", err)
	}
	r.bar.Inc()
	return time.Since(t0), nil
}

// IterObserver is notified after each steady-state iteration completes.
// Implementations use it to capture per-iteration metrics, API breakdowns, etc.
// Pass nil to Steady to skip observation.
type IterObserver interface {
	AfterIter()
}

// Steady executes count steady-state iterations and returns timing statistics.
// WallTime and APIRequests in the returned Stats cover the entire run
// (warmup + steady), matching the shared bar and counter started in NewRunner.
// obs.AfterIter is called after each iteration completes; pass nil to skip.
func (r *Runner) Steady(ctx context.Context, fn func(context.Context) error, count int, pause time.Duration, obs IterObserver) (Stats, error) {
	durations := make([]time.Duration, 0, count)
	for i := 0; i < count; i++ {
		if pause > 0 {
			select {
			case <-time.After(pause):
			case <-ctx.Done():
				r.bar.Done()
				return Stats{}, ctx.Err()
			}
		}
		start := time.Now()
		if err := fn(ctx); err != nil {
			r.bar.Done()
			return Stats{}, fmt.Errorf("steady-state iteration %d failed: %w", i+1, err)
		}
		durations = append(durations, time.Since(start))
		r.bar.Inc()
		if obs != nil {
			obs.AfterIter()
		}
	}
	r.bar.Done()

	s := computeStats(durations)
	s.WallTime = time.Since(r.wallStart)
	s.APIRequests = snapshotCounter(r.apiCounter) - r.startReqs
	return s, nil
}

func snapshotCounter(c *atomic.Int64) int64 {
	if c == nil {
		return 0
	}
	return c.Load()
}

// computeStats computes timing statistics for steady-state benchmark iterations.
// The sort+percentile algorithm mirrors report.ComputeCallStats; a shared helper
// would create an import cycle (report imports bench), so both are kept in sync manually.
func computeStats(d []time.Duration) Stats {
	if len(d) == 0 {
		return Stats{}
	}

	sorted := make([]time.Duration, len(d))
	copy(sorted, d)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	var total time.Duration
	for _, v := range d {
		total += v
	}

	return Stats{
		Count:      len(d),
		Min:        sorted[0],
		Max:        sorted[len(sorted)-1],
		Mean:       total / time.Duration(len(d)),
		P50:        sorted[int(float64(len(sorted))*0.50)],
		P99:        sorted[int(float64(len(sorted))*0.99)],
		ActiveTime: total,
		QPS:        float64(len(d)) / total.Seconds(),
	}
}
