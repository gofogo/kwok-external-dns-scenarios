package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"

	toxiproxy "github.com/Shopify/toxiproxy/v2"
	"github.com/rs/zerolog"
)

// Proxy wraps an embedded toxiproxy TCP proxy in front of the kwok API server.
type Proxy struct {
	listenAddr string
}

// Start creates an in-process toxiproxy in front of upstreamURL.
// latencyMs and jitterMs inject artificial delay; pass 0 for no delay.
func Start(ctx context.Context, upstreamURL string, latencyMs, jitterMs int) (*Proxy, error) {
	upstream, err := apiServerHostPort(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("parse upstream: %w", err)
	}

	listenAddr, err := freeAddr()
	if err != nil {
		return nil, fmt.Errorf("find free port: %w", err)
	}

	// ApiServer is required to properly initialize the Proxy struct via NewProxy
	// (started chan, Logger, Toxics, connections). It does not need to be listening.
	logger := zerolog.New(os.Stderr).Level(zerolog.ErrorLevel)
	srv := toxiproxy.NewServer(toxiproxy.NewMetricsContainer(nil), logger)

	// NewProxy initializes all internal fields (started chan, logger, toxics, connections).
	// Creating a Proxy struct literal directly leaves started as nil, causing a deadlock.
	p := toxiproxy.NewProxy(srv, "kwok-proxy", listenAddr, upstream)

	if err := srv.Collection.Add(p, true); err != nil {
		return nil, fmt.Errorf("start proxy: %w", err)
	}

	if latencyMs > 0 {
		toxicJSON := fmt.Sprintf(
			`{"type":"latency","stream":"downstream","toxicity":1.0,"attributes":{"latency":%d,"jitter":%d}}`,
			latencyMs, jitterMs,
		)
		if _, err := p.Toxics.AddToxicJson(io.NopCloser(strings.NewReader(toxicJSON))); err != nil {
			return nil, fmt.Errorf("add latency toxic: %w", err)
		}
	}

	go func() {
		<-ctx.Done()
		p.Stop()
	}()

	return &Proxy{listenAddr: listenAddr}, nil
}

// Endpoint returns the proxied API server address as https://host:port.
func (p *Proxy) Endpoint() string {
	return "https://" + p.listenAddr
}

func apiServerHostPort(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	return u.Host, nil
}

func freeAddr() (string, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	addr := l.Addr().String()
	l.Close()
	return addr, nil
}
