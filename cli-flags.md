# CLI Flags

```
go run . [flags]
```

All flags can be set in `bench.yaml`; CLI flags override the file.

## Common flags

| Flag | Default | Description |
|------|---------|-------------|
| `--help` | — | Print all flags with defaults and exit |
| `--config` | `bench.yaml` | Path to YAML config file |
| `--scenario` | _(all default)_ | Run a specific named scenario; omit to run all scenarios with `default: true` |
| `--skip-setup` | `false` | Skip cluster creation and fixture loading — reuse an existing cluster |
| `--cleanup` | `false` | Delete the cluster after the benchmark completes |

## Scenario flags (per-scenario in YAML, not settable per-flag)

`iterations` lives inside each scenario in `bench.yaml`:

```yaml
scenarios:
  - name: istio-prod-scale
    iterations: 20
    resources: { services: 4000, gateways: 3000, virtualservices: 1000 }
```

## Tuning flags

| Flag | Default | Description |
|------|---------|-------------|
| `--pause-ms` | `10s` | Delay between iterations; accepts `500`, `500ms`, `1s` |
| `--latency-ms` | `500ms` | Artificial API server latency via toxiproxy; `0` disables it |
| `--jitter-ms` | `100ms` | Random ± jitter on top of `--latency-ms` |
| `--concurrency` | `100` | Parallel API requests during fixture creation |
| `--wait-attempts` | `10` | Polling attempts for API server readiness (2 s apart) |
| `--cluster-name` | `ext-dns-bench` | KWOK cluster name |

## Typical workflows

**First run — create cluster and benchmark:**
```sh
go run .
```

**Second run on a different branch — reuse same cluster:**
```sh
go run . --skip-setup
```

**Run a specific scenario:**
```sh
go run . --scenario istio-quick-smoke
```

**No artificial latency (measure raw throughput):**
```sh
go run . --latency-ms 0
```

**Cross-branch comparison:**
1. Run branch A → results written to `ext-dns-bench-results.txt`
2. `git checkout branch-B && go build .`
3. `go run . --skip-setup` → appends branch B results to same file
4. `diff` or `cat` the file to compare
