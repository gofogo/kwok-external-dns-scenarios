# Changelog

All notable changes to the KWOK benchmarking tool are documented here.

## [Unreleased]

## [0.3.0] - 2026-03-25

### Added
- `internal/app` package: orchestration layer (`app.go`, `benchmark.go`, `print.go`, `runners.go`)
- `internal/cli/flags.go`: dedicated CLI flag parsing, decoupled from `main.go`
- `internal/config/config.go`: YAML-driven configuration with embedded CRD assets
- `internal/config/crds/`: embedded `dnsendpoint.yaml` and `istio.yaml` CRDs
- `internal/metrics/server.go`: Prometheus metrics HTTP server
- `internal/transport/transport.go`: HTTP transport layer for kube client
- `INDEX.md` updated with full architecture diagram and usage examples

### Changed
- `main.go` reduced from ~860 lines to a thin entrypoint; logic moved into `internal/app`
- `bench.yaml` extended with additional config fields

## [0.2.0] - 2026-03-25

### Added
- `internal/distribute/distribute.go`: workload distribution helper
- `internal/fixtures/dnsendpoint/`: DNSEndpoint fixture generator
- `internal/fixtures/istio/`: Istio Gateway + VirtualService fixture generator
- `internal/fixtures/service/`: Service fixture generator
- `internal/fixtures/pod/`: Pod fixture generator
- `internal/fixtures/helpers/`: shared fixture utilities
- `internal/runner/commands.go`: runner command wrappers
- `INDEX.md`: comprehensive project index with benchmarking notes
- `bench.yaml`: extended with service, pod, and DNSEndpoint fixture counts
- Cluster setup expanded with multi-resource support (`cluster.go`)

### Changed
- `internal/fixtures/fixtures.go` refactored into per-resource sub-packages

## [0.1.1] - 2026-03-23

### Added
- `internal/runner/runner.go`: benchmark runner abstraction
- `internal/runner/sources.go`: external-dns source wiring (Istio, Services, etc.)
- `crds/dnsendpoint.yaml`: DNSEndpoint CRD
- `bench.yaml`: extended with latency/jitter and source configuration
- `.claude/settings.local.json`: project-local Claude Code settings
- `Makefile` targets for kwok

### Changed
- `main.go` refactored with improved flag handling and toxiproxy wiring
- `internal/fixtures/fixtures.go` extended with more fixture types
- `internal/report/report.go` improved output formatting

## [0.1.0] - 2026-03-22

### Added
- Initial project scaffold
- `main.go`: CLI entrypoint with flags (`--gateways`, `--virtualservices`, `--iterations`, `--latency-ms`, `--jitter-ms`, `--skip-setup`, `--cleanup`, `--config`)
- `internal/cluster/cluster.go`: kwokctl cluster lifecycle (create, delete, kubeconfig, CRD apply)
- `internal/fixtures/fixtures.go`: Istio Gateway and VirtualService fixture creation
- `internal/bench/bench.go`: benchmark runner — executes `Endpoints()` N times and computes p50/p95/p99 stats
- `internal/metrics/scraper.go`: Prometheus metrics scraper
- `crds/istio.yaml`: embedded Gateway + VirtualService CRDs
- `bench.yaml`: base configuration file
- `ARCHITECTURE.md`: architecture overview with toxiproxy latency injection design
- `cli-flags.md`: CLI flag reference
- `readme.md`: quickstart and usage guide
