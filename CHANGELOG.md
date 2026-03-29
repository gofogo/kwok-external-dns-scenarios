# Changelog

All notable changes to the KWOK benchmarking tool are documented here.

## [0.5.1] - 2026-03-29

### Changed
- `runnersForScenario` signature simplified: shared cluster state extracted into `RunnerEnv`
  struct (`BenchKubeClient`, `BenchIstioClient`, `BenchCfg`, `DirectCfg`, `Concurrency`,
  `KubeAPIQPS`, `KubeAPIBurst`), replacing eight positional parameters.

## [0.5.0] - 2026-03-29

### Added
- `kube-api-qps` / `kube-api-burst` config fields (and `--kube-api-qps` / `--kube-api-burst` flags):
  rate-limiter settings for all Kubernetes API clients. `0` (default) falls back to client-go
  built-in defaults (5 QPS / 10 burst). Replaces the CRD-only `crd-client-qps`/`crd-client-burst`.
- `fork-external-dns`: `--kube-api-qps` / `--kube-api-burst` flags and `KubeAPIQPS`/`KubeAPIBurst`
  fields on `externaldns.Config` and `source.Config`; applied in `InstrumentedRESTConfig` so all
  sources (service, ingress, CRD, etc.) share the same rate-limiter setting.
- `fork-external-dns`: `init()` in `source/crd.go` calls `crlog.SetLogger(logr.Discard())` to
  silence the controller-runtime `log.SetLogger was never called` warning.

### Fixed
- `[controller-runtime] log.SetLogger(...) was never called; logs will not be displayed` warning
  printed on every benchmark run. controller-runtime's internal logs are noise alongside logrus;
  silenced globally with `logr.Discard()`.

### Changed
- `crd-client-qps` / `crd-client-burst` renamed to `kube-api-qps` / `kube-api-burst`; scope
  widened from CRD source only to all Kubernetes API clients. Update `bench.yaml` accordingly.
- `fork-external-dns`: `KubeClient()` in `SingletonClientGenerator` now routes through
  `RESTConfig()` instead of calling `InstrumentedRESTConfig` independently, so QPS/burst and
  transport instrumentation are applied consistently from a single config.
- `fork-external-dns`: `NewKubeClient` helper removed (unused after `KubeClient()` refactor).
- `fork-external-dns`: `InstrumentedRESTConfig` gains `qps float32, burst int` parameters;
  `FlagBinder` interface gains `Float32Var` (implemented by `KingpinBinder`).

## [0.4.0] - 2026-03-29

### Added
- `warmup-timeout` config field (and `--warmup-timeout` flag): bounds the first `Endpoints()` call;
  defaults to `0` (no timeout). Prevents the benchmark from hanging when the CRD source stalls on
  its initial call.
- `crd-client-qps` / `crd-client-burst` config fields (and matching flags): explicit rate-limiter
  settings for the CRD REST client. Defaults match kube defaults (5 QPS / 10 burst); raise for
  large `dnsendpoint` scenarios to avoid stalling on `UpdateStatus` calls.
- `DNSEndpointConfig` struct in `internal/runner/crd.go`: groups CRD-runner-specific settings
  (`NEndpoints`, `NsDist`, `ClientQPS`, `ClientBurst`, `BenchCfg`) replacing the positional
  parameter list on `NewDNSEndpointRunner`.

### Fixed
- CRD source API calls (informer LIST/WATCH + `UpdateStatus`) were not tracked by the benchmark
  counter. Fixed by passing `rest.CopyConfig(benchCfg)` as the base config for the CRD source so
  it inherits both the toxiproxy endpoint and the `CountingTransport` wrap hook.
- `CountingTransport` had a shared `Base http.RoundTripper` field causing a data race when multiple
  clients used the same instance. Replaced with a `WrapTransport()` method returning a per-client
  `delegatingTransport` closure; all clients share the same atomic counter safely.
- First `Endpoints()` call on large `dnsendpoint` scenarios appeared frozen: the CRD source calls
  `UpdateStatus` on every fresh endpoint, and the default 5 QPS limiter serialised thousands of
  PUTs. Addressed by exposing `crd-client-qps`/`crd-client-burst` and adding `warmup-timeout`.

### Changed
- `fork-external-dns` CRD source migrated to controller-runtime cache. `NewCRDSource` signature
  changed to `(ctx context.Context, restConfig *rest.Config, cfg *Config)`. `Endpoints()` now
  reads from an in-memory cache (zero API calls per iteration); `UpdateStatus` still goes through
  a direct client. `go.mod` pinned to fork commit `ea4d2d16`.
- `bench.yaml` `distribution` is now a named-axis map rather than a flat weight map.
  All resource types (`services`, `pods`, `dnsendpoints`) use the same struct form:
  ```yaml
  distribution:
    service-type:      # headless vs node-port split
      headless: 1
      node-port: 2
    namespaces:        # spread across namespaces
      dev: 10
      staging: 5
      default: 5
  ```
  Existing scenarios that used the flat `distribution: { headless: 1, node-port: 2 }` form
  must be migrated to `distribution: { service-type: { headless: 1, node-port: 2 } }`.
- `config.go`: `ResourceCount.Distribution` changed from `distribute.Weights` to a `Distribution`
  struct with `service-type` and `namespaces` axes; `DNSEndpointCount`/`DNSEndpointDist` types
  removed — `Resources.DNSEndpoints` is now `ResourceCount` like all other resource fields.
- `internal/fixtures/dnsendpoint`: `DNSEndpointFixture` gains `KubeClient` and `NsDist` fields;
  endpoints are distributed across namespaces when `NsDist` is set, with namespaces created
  automatically via `helpers.EnsureNamespace`.
- Progress bar label renamed from `"<source>"` to `"<source> (iter)"` to clarify it counts
  benchmark iterations, not resources.
- Source-ready log message changed from `"synced in %v"` to
  `"source ready (informer cache synced) in %v"` for clarity.

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
