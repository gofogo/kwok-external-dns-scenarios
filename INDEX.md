# kwok bench — codebase index

## Purpose

Benchmarks external-dns source implementations against a KWOK (Kubernetes Without Kubelet) cluster.
Measures API call counts and latency under configurable toxiproxy-injected delay to compare
indexer-cached vs. direct-API code paths.

## Quick orientation

```
main.go                              # Entry point: wire-up only (signal handling, app.Run)
bench.yaml                           # Default config + scenarios
internal/
  app/
    app.go                           # Run(): full benchmark lifecycle (cluster, clients, loop)
    runners.go                       # runnersForScenario() — maps config.Source → []SourceRunner
    benchmark.go                     # runSourceBenchmark(), withMetrics, withProxy, deltaCollector
    print.go                         # printInspectCommands, printClusterSummary
  cli/flags.go                       # Flags struct + MustParse(args) — all CLI flag parsing
  config/
    config.go                        # Config types, LoadConfig, SelectScenarios, source constants
    embed.go                         # //go:embed for IstioCRDs and DNSEndpointCRD
    crds/                            # Embedded CRD YAML files (istio.yaml, dnsendpoint.yaml)
  transport/transport.go             # CountingTransport — counts & times every HTTP call
  cluster/cluster.go                 # KWOK cluster lifecycle (kwokctl)
  bench/bench.go                     # Two-phase runner: warmup + steady, Stats struct
  distribute/distribute.go           # Weighted distribution of labels across N slots
  fixtures/
    fixtures.go                      # Package doc only — see subpackages below
    helpers/helpers.go               # Shared: RunConcurrent, CommonLabels, IP generators,
                                     #         EnsureNamespace, LogSkipped, MergeStringMaps,
                                     #         RetryUpdateStatus, RetryAfter
    istio/istio.go                   # GatewayFixture, VirtualServiceFixture
    pod/pod.go                       # PodFixture
    dnsendpoint/dnsendpoint.go       # DNSEndpointFixture
    service/service.go               # ServiceFixture (nodes + services + pods)
  proxy/proxy.go                     # Embedded toxiproxy (latency injection)
  runner/runner.go                   # SourceRunner interface
  runner/istio.go                    # GatewayRunner, VirtualServiceRunner
  runner/core.go                     # PodRunner, ServiceRunner
  runner/crd.go                      # DNSEndpointRunner
  runner/commands.go                 # Commands() for all runners + kubeconfigFlag helper
  runner/service_options.go          # WithSvcDist, WithPodDist functional options
  metrics/
    scraper.go                       # Prometheus scrape + delta math
    server.go                        # StartServer() — embedded Prometheus HTTP endpoint
  report/report.go                   # Human + machine-readable output
  progress/progress.go               # ASCII progress bar
crds/                                # Original CRD source files (also embedded via config/crds/)
  istio.yaml
  dnsendpoint.yaml
```

## Fixture architecture

Fixtures are split into subpackages, one per external-dns source type. Each subpackage owns
a single exported struct with a `Setup(ctx) error` method.

```
fixtures/helpers    ← imported by all four below
     ↓         ↓         ↓          ↓
  istio/      pod/  dnsendpoint/  service/
    ↑           ↑        ↑           ↑
  runner/istio.go  runner/core.go  runner/crd.go
  (each runner inline-constructs its fixture inside Setup())
```

**Why subpackages?** `istio/` is the only package that imports `istio.io/client-go`; `service/`
is the only one that imports `distribute`. Dependency scope is enforced by the package boundary
rather than by convention.

**Runner package layout** — two splits applied together:
- *By source family* (`istio.go` / `core.go` / `crd.go`): groups runners that share client dependencies. `istio.go` is the only file that imports `istio.io/client-go`; `crd.go` is the only one that imports `k8s.io/client-go/dynamic`. Mirrors the fixture subpackage layout.
- *By concern* (`commands.go`): all `Commands()` methods live in one file — the kubectl inspect strings are presentation logic, not source logic, and are easiest to maintain together.
- `service_options.go` holds `WithSvcDist`/`WithPodDist` functional options for `NewServiceRunner`, keeping the constructor signature stable as distribution options grow.

**Why inline construction?** Runners already hold all the parameters needed to build a fixture.
Rather than storing an extra field, each `Setup()` constructs the fixture inline and calls it
immediately — no duplication, no stored state.

```go
// example — GatewayRunner.Setup in runner/istio.go
func (r *GatewayRunner) Setup(ctx context.Context) error {
    return (&istiofixture.GatewayFixture{
        KubeClient:  r.directKubeClient,
        IstioClient: r.directIstioClient,
        NServices:   r.nServices,
        NGateways:   r.nGateways,
        Concurrency: r.concurrency,
    }).Setup(ctx)
}
```

**helpers package exports** (used across all fixture subpackages):

| Symbol | Purpose |
|---|---|
| `RunConcurrent` | Fan-out n tasks with semaphore + progress bar |
| `LogSkipped` | Prints skip count if > 0 (replaces repeated log pattern) |
| `CommonLabels / CommonAnnotations` | Shared benchmark metadata on every object |
| `IngressIP(i)` | IPv4/IPv6 alternating, ~130k unique addresses |
| `NodeExternalIP(i)` | RFC 6598 space, ~65k unique node IPs |
| `PodSvcIP(i)` | 10.x.x.x space, ~16M unique pod IPs |
| `EnsureNamespace` | Create namespace, ignore AlreadyExists |
| `EnsureDefaultServiceAccount` | Create `default` SA; KWOK admission requires it even when `AutomountServiceAccountToken=false` |
| `RetryUpdateStatus(ctx, fn, onConflict)` | Shared UpdateStatus retry loop — see below |
| `RetryAfter(err)` | Classifies errors into retry strategies — see below |
| `MergeStringMaps / StringsToInterface / BoolPtr` | Minor utilities |

**UpdateStatus retry strategy** — all three fixture packages call `UpdateStatus` to set fake status on KWOK objects. KWOK patches objects immediately after `Create`, causing three classes of failure:

| Error | Cause | Strategy |
|---|---|---|
| `Conflict` | KWOK bumped `resourceVersion` between Create and UpdateStatus | Re-fetch via `onConflict`, then retry |
| `unexpected EOF` | Transport-level drop (KWOK API server saturated at high concurrency) | Sleep 200 ms, retry in place (resourceVersion unchanged) |
| `429 TooManyRequests` | Server-side QPS limit hit | Log `WARN`, sleep 1 s, retry in place |

`RetryUpdateStatus` centralises this logic so callers only supply two closures (the update call and the re-fetch call). The sleep honours `ctx` cancellation so long 429 back-offs don't block shutdown.

## Execution flow

1. Load `bench.yaml` → CLI flags override
2. `cluster.Create()` + `cluster.ApplyCRDs()` (unless `--skip-setup`)
3. Build **two** REST configs:
   - `directCfg` — high QPS/burst (`QPS=concurrency×2, Burst=concurrency×4`), direct to API server → fixture creation only
   - `benchCfg` — default rate limits, routed through toxiproxy → benchmark measurements only
4. Toxiproxy starts **after** fixtures are created (intentional — prevents setup latency bias)
5. Per scenario: `Setup()` → `NewSource()` → `runSourceBenchmark()`
   - Warmup: one `First()` call (cold cache)
   - Steady: N iterations via `Steady()` with configurable pause between
6. `report.Print()` to stdout; `report.Write()` appends to `<cluster>-results.txt`

## Request routing

```
Fixture creation (setup):
  directKubeClient ──────────────────────────────► 127.0.0.1:<kwok-port> (kube-apiserver)

Benchmark measurement:
  src.Endpoints()
    └─► apiCounter (countingTransport)
          └─► TLS transport
                └─► toxiproxy (random port, latency+jitter injected)
                      └─► 127.0.0.1:<kwok-port> (kube-apiserver)
```

`apiCounter` wraps `benchCfg`'s transport — every benchmark HTTP call is counted and timed.
`directCfg` never touches the proxy — setup speed doesn't pollute benchmark numbers.

## Key design decisions

| Decision | Where | Rationale |
|---|---|---|
| Dual REST configs | `app/app.go` | Isolate setup speed from benchmark latency |
| `directCfg` QPS/Burst = `concurrency×2` / `concurrency×4` | `app/app.go` | Prevents client-side rate limiter from throttling concurrent fixture creation |
| `max-requests-inflight=3000` on kube-apiserver | `cluster/cluster.go` | Default limits (400/200) trigger 429s and `unexpected EOF` drops at concurrency≥100; raised to match realistic fixture load |
| Toxiproxy after fixtures | `app/app.go` | Proxy latency only affects benchmark, not setup |
| `CountingTransport` | `transport/transport.go` | Counts & times every HTTP call; normalized via `normalizeAPIPath()` |
| `clusterExists` guard | `app/app.go` | Prevents `deleteCluster` from running when `cluster.Create` never succeeded |
| `--skip-setup` flag | `cli/flags.go` | Reuse fixtures across git branches for fair comparison |
| Idempotent fixtures | `fixtures/*/` | Skip already-existing resources; no auto-cleanup |
| `api_reqs=0` signal | results file | Zero API requests in steady state = informer cache working |

## Scenarios (bench.yaml)

| Name | Scale | Source | Default |
|---|---|---|---|
| `istio-prod-scale` | 3000 gw + 3000 vs | Istio | false |
| `istio-quick-smoke` | 20 gw + 10 vs | Istio | false |
| `pod-quick-smoke` | 20 pods | Pod | false |
| `service-light` | 3 svc + 5 nodes + 10 pods | Service | **true** |
| `service-heavy` | 2600 svc + 4000 nodes + 70276 pods | Service | false |
| `dnsendpoint-quick-smoke` | 20 dnsendpoints | DNSEndpoint | false |

Run one: `go run . --scenario istio-quick-smoke`

## Key types

- `transport.CountingTransport` — HTTP transport that records per-call durations, indexed for windowing
- `bench.Stats` — FirstCall, Mean, P50, P99, QPS, APIRequests
- `bench.Runner` — `First(ctx, fn)` + `Steady(ctx, fn, count, pause, afterEach)`
- `SourceRunner` interface — `Label()`, `ResourceCount()`, `Setup()`, `NewSource()`, `Commands()`
- `metrics.Scraper` — scrapes Prometheus text, computes deltas between snapshots
- `report.SourceStats` — combines bench.Stats + metric deltas + API call breakdown

## Active TODOs

1. **external_dns_* metrics** (`bench.yaml` lines 69-74, 87-92)
   — `source_endpoints_total` and `source_records` are registered in
   `controller/metrics.go` of external-dns, which the bench tool doesn't import.
   Fix: add a blank import of that package or refactor external-dns to expose metrics
   from the source package itself.

## Dependencies of note

| Dep | Version | Why |
|---|---|---|
| `github.com/Shopify/toxiproxy/v2` | v2.12.0 | In-process TCP proxy for latency injection |
| `istio.io/client-go` | v1.29.1 | Typed Istio clients |
| `sigs.k8s.io/external-dns` | v0.20.1-0.20260325233016-83e1bcf39d1a (master) | go.work → `../fork-external-dns` for local dev (gitignored) |
| `k8s.io/client-go` | v0.35.3 | Standard k8s client |
| `github.com/prometheus/client_golang` | v1.23.2 | Metrics scraping |

## Fixture internals

- Gateways live in `default` namespace; ingress Services in `istio-system`
- Services named `istio-ingressgateway-0..N`; Gateways `gateway-0..N`
- VirtualServices share one wildcard gateway (`shared-gateway`)
- Service-source services in `default` as `bench-svc-0..N`; nodes as `bench-node-0..N`
- `services` and `pods` in `bench.yaml` accept a plain integer **or** a struct with `count` + `distribution`:
  ```yaml
  services:           # struct form — count + weights
    count: 3
    distribution:
      headless: 1
      node-port: 2
  pods:               # struct form — count + distribution
    count: 10
  services: 3         # plain integer shorthand (no distribution)
  ```
  Parsed via `config.ResourceCount.UnmarshalJSON` in `internal/config/config.go` — both forms work anywhere `services`/`pods` appear.
- Distribution weights (`headless`/`node-port`) resolved via `distribute.Distribute(n, weights)`
- Concurrency controlled via semaphore in `helpers.RunConcurrent()`

## Results file format

```
# ext-dns-bench scenario=istio-quick-smoke gateways=20 virtualservices=10 latency=500ms jitter=100ms time=2025-...
first=NNNms  mean=NNNms  p50=NNNms  p99=NNNms  qps=N.NN  api_reqs=N
```

`api_reqs=0` in steady state indicates the informer cache is fully effective (target state for new code).
