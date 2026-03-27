# Changelog

All notable changes to the KWOK benchmarking tool are documented here.

## [Unreleased]

### Added
- `warmup-timeout` config field (and `--warmup-timeout` flag): bounds the first `Endpoints()` call;
  defaults to `0` (no timeout). Prevents the benchmark from hanging when the CRD source is slow
  on its initial call.
- `crd-client-qps` / `crd-client-burst` config fields (and matching flags): explicit rate-limiter
  settings for the CRD REST client used by `CRDSource`. Defaults match kube defaults (5 QPS / 10
  burst); raise for large `dnsendpoint` scenarios to avoid stalling on `UpdateStatus` calls.
- `fork-external-dns`: `source.Config.CRDClientQPS`, `CRDClientBurst`, `CRDClientWrapTransport`
  fields; `NewCRDClientForAPIVersionKind` applies them when building the REST client so the
  benchmark counting transport covers all CRD API calls.

### Fixed
- CRD source API calls (LIST + `UpdateStatus`) were not tracked by the benchmark counter because
  `NewCRDClientForAPIVersionKind` builds its own REST config from the kubeconfig, bypassing
  `benchCfg`'s `CountingTransport`. Fixed by threading `benchCfg.WrapTransport` through to the
  CRD client via the new `CRDClientWrapTransport` field.
- First `Endpoints()` call on large `dnsendpoint` scenarios appeared frozen: `CRDSource` calls
  `UpdateStatus` on every endpoint (to sync `ObservedGeneration`), and the default 5 QPS limiter
  serialised thousands of PUTs. Addressed by exposing `crd-client-qps`/`crd-client-burst` and the
  warmup timeout.

### Changed
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
- `internal/runner/crd.go`: `NewDNSEndpointRunner` accepts a `nsDist distribute.Weights`
  parameter and threads it through to the fixture.

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
