// Package metrics scrapes a Prometheus /metrics endpoint and extracts named metrics.
package metrics

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// scrapeClient has a fixed timeout so scrape calls never hang if the
// metrics server is unreachable or stalls mid-response.
var scrapeClient = &http.Client{Timeout: 5 * time.Second}

// Sample holds one scrape result.
type Sample struct {
	Values map[string]float64 // key: "metric_name" or "metric_name{labels}"
	Types  map[string]string  // key: base metric name → "gauge"/"counter"/"summary"/etc.
}

// Scraper fetches and parses metrics from a Prometheus exposition endpoint.
type Scraper struct {
	url   string
	names map[string]bool // base metric family names to capture
}

// New creates a Scraper that captures the given metric family names.
func New(metricsURL string, names []string) *Scraper {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[baseName(n)] = true
	}
	return &Scraper{url: metricsURL, names: m}
}

// Ping checks that external-dns is healthy via /healthz on the same host:port.
func (s *Scraper) Ping() error {
	u, err := url.Parse(s.url)
	if err != nil {
		return fmt.Errorf("invalid metrics URL %q: %w", s.url, err)
	}
	u.Path = "/healthz"
	resp, err := scrapeClient.Get(u.String())
	if err != nil {
		return fmt.Errorf("external-dns not reachable at %s: %w", u.String(), err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("external-dns /healthz returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// Snapshot fetches the metrics endpoint and returns a Sample.
func (s *Scraper) Snapshot() (Sample, error) {
	resp, err := scrapeClient.Get(s.url)
	if err != nil {
		return Sample{}, fmt.Errorf("scrape %s: %w", s.url, err)
	}
	defer resp.Body.Close()
	return parse(resp.Body, s.names)
}

// IsGauge reports whether the metric key is a gauge type.
func IsGauge(types map[string]string, key string) bool {
	return types[baseName(key)] == "gauge"
}

// Delta returns element-wise difference after.Values − before.Values.
func Delta(before, after Sample) map[string]float64 {
	d := make(map[string]float64, len(after.Values))
	for k, v := range after.Values {
		d[k] = v - before.Values[k]
	}
	return d
}

// MeanValues returns the mean absolute value for each key across samples.
func MeanValues(snapshots []Sample, key string) float64 {
	if len(snapshots) == 0 {
		return 0
	}
	var sum float64
	for _, s := range snapshots {
		sum += s.Values[key]
	}
	return sum / float64(len(snapshots))
}

// SumDeltas returns the total delta across all per-iteration deltas.
func SumDeltas(deltas []map[string]float64) map[string]float64 {
	total := make(map[string]float64)
	for _, d := range deltas {
		for k, v := range d {
			total[k] += v
		}
	}
	return total
}

// AllKeys returns a sorted deduplicated set of keys across all snapshots.
func AllKeys(snapshots []Sample) []string {
	seen := make(map[string]bool)
	for _, s := range snapshots {
		for k := range s.Values {
			seen[k] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	return keys
}

// — internal —

// parse reads Prometheus text-format exposition line by line.
// It captures `# TYPE` lines to populate Sample.Types and metric lines for Sample.Values.
func parse(r io.Reader, names map[string]bool) (Sample, error) {
	values := make(map[string]float64)
	types := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		if line[0] == '#' {
			// # TYPE <name> <type>
			if strings.HasPrefix(line, "# TYPE ") {
				parts := strings.Fields(line)
				if len(parts) == 4 && names[parts[2]] {
					types[parts[2]] = parts[3]
				}
			}
			continue
		}
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		key := line[:sp]
		rest := strings.TrimSpace(line[sp+1:])
		if i := strings.IndexByte(rest, ' '); i >= 0 {
			rest = rest[:i]
		}
		if !names[baseName(key)] {
			continue
		}
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			continue
		}
		values[key] = v
	}
	return Sample{Values: values, Types: types}, scanner.Err()
}

func baseName(n string) string {
	if i := strings.IndexByte(n, '{'); i >= 0 {
		return n[:i]
	}
	return n
}
