# ext-dns-kwok-bench

Benchmarks [external-dns](https://github.com/kubernetes-sigs/external-dns) source implementations against a [KWOK](https://kwok.sigs.k8s.io/) cluster. Measures API call counts and latency to compare informer-cached vs. direct-API code paths.

## Features

- Benchmarks all major external-dns source types: Service, Istio (Gateway + VirtualService), Pod, DNSEndpoint
- Measures API call counts and latency per iteration (warmup + steady-state)
- Injects configurable API server latency and jitter via embedded toxiproxy
- Scrapes per-iteration Prometheus metrics (memory, GC, custom counters)
- Cross-branch comparison: reuse fixtures across runs with `--skip-setup`
- Concurrent fixture creation with configurable parallelism
- Scenarios defined in `bench.yaml` — scale from smoke tests to production-scale loads
- Results saved to a file for easy diffing across runs

## Prerequisites

- [kwok](https://kwok.sigs.k8s.io/) — `brew install kwok`
- `kubectl`
- Docker (running)
- Go 1.21+

## Quick start

```sh
go run .
```

Runs the default scenario (`service-light`) — creates a KWOK cluster, loads fixtures, benchmarks, and leaves the cluster running.

```sh
go run . --cleanup        # delete cluster after the run
go run . --scenario istio-quick-smoke  # run a specific scenario
```

## Available scenarios

| Name | Source | Scale | Default |
|---|---|---|---|
| `service-light` | Service | 3 svc + 5 nodes + 10 pods | yes |
| `service-heavy` | Service | 2600 svc + 3000 nodes + 60k pods | no |
| `istio-quick-smoke` | Istio | 20 gw + 10 vs | no |
| `istio-prod-scale` | Istio | 3000 gw + 3000 vs | no |
| `pod-quick-smoke` | Pod | 20 pods | no |
| `dnsendpoint-quick-smoke` | DNSEndpoint | 20 dnsendpoints | no |

## Cross-branch comparison

The main use case: compare two branches of external-dns against identical fixtures.

```sh
# Branch A — create cluster, load fixtures, benchmark
go run . --scenario istio-prod-scale --save-results

# Switch branch in your external-dns fork, then reuse the same cluster
go run . --scenario istio-prod-scale --skip-setup --save-results

# Compare
cat ext-dns-bench-results.txt
```

`--skip-setup` reuses the existing cluster and fixtures — no teardown between runs.
`--save-results` appends results to `<cluster-name>-results.txt`.
`api_reqs=0` in steady state means the informer cache is fully effective.

## Key flags

| Flag | Default | Description |
|---|---|---|
| `--scenario` | all defaults | Run a single named scenario |
| `--skip-setup` | false | Reuse existing cluster and fixtures |
| `--cleanup` | false | Delete cluster after the run |
| `--latency-ms` | 500ms | Inject API server latency via toxiproxy |
| `--jitter-ms` | 100ms | Jitter on top of `--latency-ms` |
| `--save-results` | false | Append results to `<cluster-name>-results.txt` |

All flags can also be set in `bench.yaml`. CLI flags take precedence.
