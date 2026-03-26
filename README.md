# KWOK

- https://kwok.sigs.k8s.io/
- https://kwok.sigs.k8s.io/docs/examples/prometheus/

```sh
brew install kwok

kwokctl config view
cat ~/.kwok/clusters/<cluster name>/kwok.yaml

kwokctl delete cluster --name ext-dns-bench
kwokctl delete cluster --name ext-dns-bench
```


```
# Run with config file
  cd ~/source/self/workshops/kubernetes-sigs-issues/iac/kwok && go run . --config bench.yaml

# Run everything in one command (creates cluster, installs CRDs, creates 3K fixtures, benchmarks, cleans up)
  cd /Users/ik/source/self/workshops/kubernetes-sigs-issues/iac/kwok
  go run . --cleanup

  # Compare branches: run on current branch, keep cluster, switch branch, re-run
  go run . --gateways 3000 --virtualservices 3000 --iterations 20
  # results written to ext-dns-bench-results.txt

  git checkout master  # in fork-external-dns
  go run . --skip-setup --cleanup --iterations 20
  # appends to ext-dns-bench-results.txt — compare manually

  The --skip-setup flag lets you reuse the same cluster across branch comparisons. Results are appended to <cluster-name>-results.txt for easy diffing.

  Want to do a test run now?
```

Build succeeded. And on the kwok question: kwok's delay/jitter support is for stage transitions — it controls when simulated resources change state (e.g. node heartbeat
  after 20s±5s). It has no mechanism to inject latency into API server request/response cycles.

  So our toxiproxy approach is the right one. The project is now fully working:

  kwok/
  ├── main.go                    # flags, orchestration, proxy wiring
  ├── crds/istio.yaml            # embedded Gateway + VirtualService CRDs
  ├── internal/
  │   ├── cluster/cluster.go     # kwokctl create/delete/kubeconfig/apply CRDs
  │   ├── fixtures/fixtures.go   # create services, gateways, virtualservices
  │   ├── bench/bench.go         # run Endpoints() N times, compute stats
  │   └── proxy/proxy.go         # embedded toxiproxy with latency toxic

  Usage:
  # Full run, no latency
  go run . --cleanup

  # With 50ms latency + 10ms jitter (highlights the old List() call penalty)
  go run . --latency-ms 50 --jitter-ms 10 --cleanup

  # Cross-branch comparison: keep cluster between runs
  go run . --gateways 3000 --virtualservices 3000 --iterations 20
  # switch to master in fork-external-dns, recompile
  go run . --skip-setup --cleanup --iterations 20
  # compare ext-dns-bench-results.txt

