// Package runner defines the SourceRunner interface and its implementations.
// Source types map to runners as follows:
//   - "istio"       → GatewayRunner + VirtualServiceRunner (two runners, one source type)
//   - "pod"         → PodRunner
//   - "dnsendpoint" → DNSEndpointRunner
//
// Each runner owns fixture setup, source construction, and kubectl command output.
package runner

import (
	"context"

	"sigs.k8s.io/external-dns/source"
)

// SourceRunner encapsulates all logic for benchmarking one external-dns source type.
type SourceRunner interface {
	// Label is a short identifier used in progress bars and log messages.
	// e.g. "gateway", "virtualservice", "pod", "dnsendpoint"
	Label() string

	// ResourceCount is the N shown in benchmark logs (number of source objects).
	ResourceCount() int

	// Setup creates the Kubernetes objects the source will observe.
	// Called once per scenario; skipped when --skip-setup is true.
	Setup(ctx context.Context) error

	// NewSource constructs the external-dns Source and blocks until its informer cache syncs.
	NewSource(ctx context.Context) (source.Source, error)

	// Commands returns kubectl command lines for the given cluster.
	// Pass kubeconfigPath to include --kubeconfig (for mid-run inspect output).
	// Pass "" to omit it (for end-of-run summary where KUBECONFIG is already exported).
	Commands(kubeconfigPath string) []string
}
