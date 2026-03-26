// Package helpers provides shared utilities for benchmark fixture creation:
// concurrent fan-out, common labels/annotations, IP generators, and namespace setup.
// All fixture subpackages (istio, pod, dnsendpoint, service) depend on this package.
package helpers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"maps"
	"net/netip"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

var baseLabels = map[string]string{
	"managed-by":  "ext-dns-kwok-bench",
	"environment": "benchmark",
	"team":        "platform",
}

// CommonLabels merges extra into the shared benchmark metadata labels.
// extra wins on key collision.
func CommonLabels(extra map[string]string) map[string]string {
	out := make(map[string]string, len(baseLabels)+len(extra))
	maps.Copy(out, baseLabels)
	maps.Copy(out, extra)
	return out
}

// CommonAnnotations returns annotations applied to every fixture resource.
func CommonAnnotations(index int) map[string]string {
	return map[string]string{
		"bench.ext-dns/index":       fmt.Sprintf("%d", index),
		"bench.ext-dns/description": "created by ext-dns-kwok-bench for performance testing",
		"bench.ext-dns/issue":       "https://github.com/kubernetes-sigs/external-dns/issues/5016",
	}
}

// IngressIP returns a unique, valid IP for the i-th ingress service.
// Even indices produce IPv4 (10.a.b.1); odd indices produce IPv6 (fd00::n).
// This alternating scheme supports ~130k services without collision.
func IngressIP(i int) string {
	var addr netip.Addr
	if i%2 == 0 {
		n := i / 2
		addr = netip.AddrFrom4([4]byte{10, byte(n / 256), byte(n % 256), 1})
	} else {
		n := uint16(i/2 + 1)
		addr = netip.AddrFrom16([16]byte{0xfd, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, byte(n >> 8), byte(n)})
	}
	if !addr.Is4() && !addr.Is6() {
		panic(fmt.Sprintf("IngressIP(%d): generated invalid address %s", i, addr))
	}
	return addr.String()
}

// NodeExternalIP returns a unique external IP for the i-th node.
// Uses RFC 6598 shared address space (100.64.0.0/10); supports up to ~65k nodes.
func NodeExternalIP(i int) string {
	return fmt.Sprintf("100.64.%d.%d", i/256, i%256)
}

// PodSvcIP returns a unique pod IP for service-source pods using 10.x.x.x space.
func PodSvcIP(i int) string {
	return fmt.Sprintf("10.%d.%d.%d", i/65536, (i/256)%256, i%256)
}

// EnsureNamespace creates a namespace, silently ignoring AlreadyExists.
func EnsureNamespace(ctx context.Context, kubeClient kubernetes.Interface, name string) error {
	_, err := kubeClient.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create namespace %q: %w", name, err)
	}
	return nil
}

// EnsureDefaultServiceAccount creates the "default" service account in the given namespace,
// silently ignoring AlreadyExists. KWOK's admission validates the service account exists even
// when AutomountServiceAccountToken is false, so pods cannot be created without it.
func EnsureDefaultServiceAccount(ctx context.Context, kubeClient kubernetes.Interface, namespace string) error {
	_, err := kubeClient.CoreV1().ServiceAccounts(namespace).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: namespace},
	}, metav1.CreateOptions{})
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		return fmt.Errorf("create default service account in %q: %w", namespace, err)
	}
	return nil
}

// AtLeast returns val if val >= min, otherwise returns min.
// Used to normalise fixture fields (e.g. Concurrency, NNodes) before use.
func AtLeast(val, min int) int {
	if val < min {
		return min
	}
	return val
}

// RunConcurrent fans out n tasks across concurrency goroutines, ticking bar after each.
// fn returns (skipped bool, err error); skipped=true means the resource already existed.
// Returns the total skip count and the first non-nil error encountered.
// Respects ctx cancellation: goroutines that haven't started yet are skipped when ctx is done.
func RunConcurrent(ctx context.Context, n, concurrency int, bar *progress.Bar, fn func(i int) (bool, error)) (int, error) {
	sem := make(chan struct{}, concurrency)
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
		skipped  int
	)
	for i := range n {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			bar.Done()
			return skipped, ctx.Err()
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if ctx.Err() != nil {
				bar.Inc()
				return
			}
			skip, err := fn(i)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = err
			}
			if skip {
				skipped++
			}
			bar.Inc()
		}(i)
	}
	wg.Wait()
	bar.Done()
	return skipped, firstErr
}

// LogSkipped prints a skip notice if n > 0.
func LogSkipped(n int, kind string) {
	if n > 0 {
		log.Printf("    skipped %d %s(s) already existing", n, kind)
	}
}

// MergeStringMaps returns a new map with all entries from a and b; b wins on collision.
func MergeStringMaps(a, b map[string]string) map[string]string {
	out := make(map[string]string, len(a)+len(b))
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

// StringsToInterface converts map[string]string to map[string]interface{} for unstructured use.
func StringsToInterface(m map[string]string) map[string]interface{} {
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func BoolPtr(b bool) *bool { return &b }

// RetryUpdateStatus retries fn (which should set the desired status then call
// UpdateStatus) until it succeeds, applying the correct back-off strategy per
// error type. onConflict is called when the server returns a Conflict error so
// the caller can re-fetch the object before fn runs again.
// The sleep honours ctx cancellation so long 429 back-offs don't block shutdown.
func RetryUpdateStatus(ctx context.Context, fn func() error, onConflict func() error) error {
	for {
		err := fn()
		if err == nil {
			return nil
		}
		delay, inPlace, ok := RetryAfter(err)
		if !ok {
			return err
		}
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if !inPlace {
			if err := onConflict(); err != nil {
				return err
			}
		}
	}
}

// RetryAfter classifies an UpdateStatus error and returns how long to wait
// before retrying and whether the retry should use the existing object.
//
//   - unexpected EOF: transport-level drop; server never saw the request so
//     resourceVersion is unchanged — retry in place after a short pause.
//   - 429 TooManyRequests: server-side rate limit; back off longer before retrying.
//   - Conflict: resourceVersion was bumped by the server; caller must re-fetch
//     before retrying (inPlace=false, delay=0).
//   - anything else: ok=false, caller should surface the error.
func RetryAfter(err error) (delay time.Duration, inPlace bool, ok bool) {
	switch {
	case errors.Is(err, io.ErrUnexpectedEOF):
		return 200 * time.Millisecond, true, true
	case k8serrors.IsTooManyRequests(err):
		log.Printf("WARN: server-side rate limit (429) on UpdateStatus — backing off 1s")
		return 1 * time.Second, true, true
	case k8serrors.IsConflict(err):
		return 0, false, true // caller re-fetches
	default:
		return 0, false, false
	}
}
