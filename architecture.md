# Connection Architecture

```
external-dns source
  └─ Endpoints()
       └─ informer cache (in-process, no network)
            └─ populated once via List+Watch
                 └─ kubeClient / istioClient
                      └─ https://127.0.0.1:<proxy-port>   ← toxiproxy (in-process)
                                                               latency: 50ms ± 10ms jitter
                           └─ https://127.0.0.1:<kwok-port>  ← kwok API server
```

## Two REST configs

| Config | Used for | Latency |
|--------|----------|---------|
| `directCfg` | fixture creation (services, gateways, VSes) | none |
| `benchCfg` | informer sync + benchmark clients | toxiproxy latency |

## Why toxiproxy matters

- **Old code (master)**: `Endpoints()` calls `List()` on the API server every invocation → pays latency on every call.
- **New code (indexer)**: `Endpoints()` reads from the local informer cache → zero API calls after initial sync → latency irrelevant.

`api requests: 0` in benchmark output confirms the indexer path is active.

## Toxiproxy setup

Toxiproxy runs **in-process** (embedded, no external daemon). It is started after fixture creation so setup is unaffected by artificial latency.

```
proxy.Start(ctx, kwokAPIServerURL, latencyMs, jitterMs)
  └─ listens on 127.0.0.1:<random-port>
  └─ forwards to kwok API server
  └─ injects downstream latency toxic
```
