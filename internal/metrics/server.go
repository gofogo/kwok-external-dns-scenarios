package metrics

import (
	"fmt"
	"log"
	"net/http"
	"net/url"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// StartServer starts an HTTP server on the host:port of metricsURL,
// serving /healthz and /metrics from the default Prometheus registry.
// This exposes the same metrics that external-dns sources register during the benchmark.
func StartServer(metricsURL string) error {
	u, err := url.Parse(metricsURL)
	if err != nil {
		return fmt.Errorf("invalid metrics-url %q: %w", metricsURL, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	mux.Handle("/metrics", promhttp.Handler())
	go func() {
		if err := http.ListenAndServe(u.Host, mux); err != nil {
			log.Printf("WARN: metrics server exited: %v", err)
		}
	}()
	return nil
}
