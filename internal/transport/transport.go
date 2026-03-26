// Package transport provides a counting HTTP transport that records per-request
// durations broken down by normalized method + API path.
package transport

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/report"
)

// callRecord accumulates durations for one normalized call type.
type callRecord struct {
	mu        sync.Mutex
	durations []time.Duration
}

// CountingTransport wraps an http.RoundTripper, counts every request made, and
// records per-call durations broken down by normalized method + API path.
type CountingTransport struct {
	Base  http.RoundTripper
	Count atomic.Int64
	calls sync.Map // string → *callRecord
}

// NewCountingTransport creates a new CountingTransport.
func NewCountingTransport() *CountingTransport {
	return &CountingTransport{}
}

func (t *CountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.Base.RoundTrip(req)
	elapsed := time.Since(start)

	t.Count.Add(1)
	key := requestKey(req)
	v, _ := t.calls.LoadOrStore(key, &callRecord{})
	cr := v.(*callRecord)
	cr.mu.Lock()
	cr.durations = append(cr.durations, elapsed)
	cr.mu.Unlock()
	return resp, err
}

// SnapshotIndexes returns the current length of each call type's duration slice.
// Used as a "before" marker to later extract only the durations from a window.
func (t *CountingTransport) SnapshotIndexes() map[string]int {
	out := make(map[string]int)
	t.calls.Range(func(k, v any) bool {
		cr := v.(*callRecord)
		cr.mu.Lock()
		out[k.(string)] = len(cr.durations)
		cr.mu.Unlock()
		return true
	})
	return out
}

// CallWindow returns per-call-type stats for durations recorded after before.
func (t *CountingTransport) CallWindow(before map[string]int) map[string]report.CallStats {
	out := make(map[string]report.CallStats)
	t.calls.Range(func(k, v any) bool {
		cr := v.(*callRecord)
		cr.mu.Lock()
		start := before[k.(string)]
		ds := make([]time.Duration, len(cr.durations)-start)
		copy(ds, cr.durations[start:])
		cr.mu.Unlock()
		if len(ds) > 0 {
			out[k.(string)] = report.ComputeCallStats(ds)
		}
		return true
	})
	return out
}

// requestKey returns a normalized "METHOD /path" key for grouping API calls.
// Watch requests are labelled WATCH regardless of HTTP method.
func requestKey(req *http.Request) string {
	op := req.Method
	if strings.Contains(req.URL.RawQuery, "watch=true") {
		op = "WATCH"
	}
	return op + " " + normalizeAPIPath(req.URL.Path)
}

// normalizeAPIPath collapses variable segments in Kubernetes API paths.
// Structural segments (pure lowercase letters: api, apis, gateways, …) are kept;
// everything else (versions like v1, group names with dots, namespace values,
// resource instance names) is replaced with *.
//
//	/apis/networking.istio.io/v1/namespaces/istio-system/gateways
//	→ /apis/*/*/namespaces/*/gateways
func normalizeAPIPath(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(parts))
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if p == "namespaces" && i+1 < len(parts) {
			out = append(out, "namespaces/*")
			i++ // skip the namespace value
			continue
		}
		if isAPISegment(p) {
			out = append(out, p)
		} else {
			out = append(out, "*")
		}
	}
	return "/" + strings.Join(out, "/")
}

// isAPISegment reports whether s is a structural k8s path segment (resource
// collection or prefix), identified as pure lowercase ASCII letters with no
// digits, dots, or hyphens.
func isAPISegment(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c < 'a' || c > 'z' {
			return false
		}
	}
	return true
}
