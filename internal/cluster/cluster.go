package cluster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CheckDocker returns an error if the Docker daemon is not reachable.
func CheckDocker() error {
	if _, err := exec.LookPath("docker"); err != nil {
		return fmt.Errorf("docker not found in PATH: %w", err)
	}
	if err := output2("docker", "info"); err != nil {
		return fmt.Errorf("docker daemon not reachable (is Docker running?): %w", err)
	}
	return nil
}

// Create creates a KWOK cluster with the given name.
// Extra kube-apiserver flags:
//   - service-cluster-ip-range=10.0.0.0/12 — /12 CIDR (~1M addresses) instead of the default
//     /24 (254 addresses) so large benchmarks with thousands of LoadBalancer services don't
//     exhaust the range. /12 is the largest prefix kube-apiserver allows for 32-bit addresses.
//   - max-requests-inflight=3000 / max-mutating-requests-inflight=3000 — raise the default
//     limits (400 / 200) so concurrent fixture creation at high concurrency doesn't trigger
//     429 TooManyRequests or unexpected EOF drops from the API server.
func Create(name string) error {
	return run("kwokctl", "create", "cluster", "--name", name,
		"--extra-args", "kube-apiserver=service-cluster-ip-range=10.0.0.0/12",
		"--extra-args", "kube-apiserver=max-requests-inflight=3000",
		"--extra-args", "kube-apiserver=max-mutating-requests-inflight=3000",
	)
}

const (
	deleteRetries  = 3
	deleteRetryWait = 3 * time.Second
)

// Delete deletes the KWOK cluster with the given name.
// Retries up to deleteRetries times with a pause between attempts: if the API
// server crashed (e.g. due to request overload), kwokctl may need a moment to
// finish its own cleanup before it can process the delete command.
func Delete(name string) error {
	var err error
	for i := range deleteRetries {
		if err = run("kwokctl", "delete", "cluster", "--name", name); err == nil {
			return nil
		}
		if i+1 < deleteRetries {
			log.Printf("WARN: delete attempt %d/%d failed (%v), retrying in %v...", i+1, deleteRetries, err, deleteRetryWait)
			time.Sleep(deleteRetryWait)
		}
	}
	return err
}

// KubeconfigPath exports the kubeconfig for the named cluster to a temp file
// and returns the file path.
func KubeconfigPath(name string) (string, error) {
	out, err := output("kwokctl", "get", "kubeconfig", "--name", name)
	if err != nil {
		return "", fmt.Errorf("get kubeconfig: %w", err)
	}

	path := filepath.Join(os.TempDir(), fmt.Sprintf("kwok-%s.yaml", name))
	if err := os.WriteFile(path, []byte(out), 0600); err != nil {
		return "", fmt.Errorf("write kubeconfig: %w", err)
	}
	return path, nil
}

// WaitReady polls until the API server is ready or maxAttempts is reached.
// It first probes /readyz; after halfAttempts failures it falls back to
// `kubectl cluster-info` in case the readyz endpoint is slow to register.
func WaitReady(kubeconfigPath, contextName string, maxAttempts int) error {
	half := maxAttempts / 2
	for i := range maxAttempts {
		var err error
		if i < half {
			err = output2("kubectl", "get", "--raw", "/readyz", "--kubeconfig", kubeconfigPath)
		} else {
			err = output2("kubectl", "cluster-info", "--context", contextName, "--kubeconfig", kubeconfigPath)
		}
		if err == nil {
			return nil
		}
		log.Printf("    API server not ready yet (attempt %d/%d), retrying in 2s...", i+1, maxAttempts)
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("API server did not become ready after %d attempts", maxAttempts)
}

// ApplyCRDs applies the given YAML to the cluster using kubectl.
func ApplyCRDs(kubeconfigPath, crdYAML string) error {
	tmp, err := os.CreateTemp("", "istio-crds-*.yaml")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.WriteString(crdYAML); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}

	return run("kubectl", "apply", "-f", tmp.Name(), "--kubeconfig", kubeconfigPath, "--validate=false")
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = &lineRewriter{}
	cmd.Stderr = &lineRewriter{}
	return cmd.Run()
}

// lineRewriter reformats command output for cleaner log lines.
// kwokctl emits logfmt-style lines like:
//
//	Cluster is starting                          elapsed=0.3s cluster=ext-dns-bench
//
// lineRewriter strips the trailing key=value fields and logs the message part as
// "        → cluster is starting".
type lineRewriter struct{ buf strings.Builder }

func (w *lineRewriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		s := w.buf.String()
		nl := strings.IndexByte(s, '\n')
		if nl < 0 {
			break
		}
		line := strings.TrimSpace(s[:nl])
		w.buf.Reset()
		w.buf.WriteString(s[nl+1:])
		if line == "" {
			continue
		}
		log.Printf("        → %s", extractMsg(line))
	}
	return len(p), nil
}

// extractMsg extracts a short human-readable message from a log line,
// preserving error details from err=/error=/reason= fields.
// Handles two formats emitted by kwokctl/kubectl:
//   - JSON:    {"msg":"execute exit","err":"signal: killed",...}  → "execute exit: signal: killed"
//   - logfmt:  Failed to start  err="addr in use"  elapsed=0.3s  → "failed to start: addr in use"
func extractMsg(line string) string {
	// Try JSON first.
	if len(line) > 0 && line[0] == '{' {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err == nil {
			var msg, cluster, errStr string
			for _, key := range []string{"msg", "message"} {
				if raw, ok := obj[key]; ok {
					json.Unmarshal(raw, &msg) //nolint:errcheck
					break
				}
			}
			if raw, ok := obj["cluster"]; ok {
				json.Unmarshal(raw, &cluster) //nolint:errcheck
			}
			for _, key := range []string{"err", "error", "reason"} {
				if raw, ok := obj[key]; ok {
					json.Unmarshal(raw, &errStr) //nolint:errcheck
					break
				}
			}
			if msg != "" {
				out := strings.ToLower(msg)
				if errStr != "" {
					out += ": " + errStr
				}
				if cluster != "" {
					out += "  cluster=" + cluster
				}
				return out
			}
		}
	}
	// Logfmt: collect text tokens (before any key=value), then surface error fields.
	fields := strings.Fields(line)
	var parts []string
	for _, f := range fields {
		if strings.ContainsRune(f, '=') {
			break
		}
		parts = append(parts, f)
	}
	var errStr string
	for _, key := range []string{"err", "error", "reason"} {
		if v := extractLogfmtField(line, key); v != "" {
			errStr = v
			break
		}
	}
	if len(parts) > 0 {
		out := strings.ToLower(strings.Join(parts, " "))
		if errStr != "" {
			out += ": " + errStr
		}
		return out
	}
	return strings.ToLower(line)
}

// extractLogfmtField extracts the value of key from a logfmt-style line.
// Handles both key=bare and key="quoted value" forms.
func extractLogfmtField(line, key string) string {
	prefix := key + "="
	idx := strings.Index(line, prefix)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(prefix):]
	if len(rest) == 0 {
		return ""
	}
	if rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			return rest[1:]
		}
		return rest[1 : end+1]
	}
	if end := strings.IndexByte(rest, ' '); end >= 0 {
		return rest[:end]
	}
	return rest
}

func output(name string, args ...string) (string, error) {
	var buf bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = &buf
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

// output2 runs a command discarding stdout — used for readiness probes.
// Stderr is captured and appended to the error message on failure.
func output2(name string, args ...string) error {
	var stderr bytes.Buffer
	cmd := exec.Command(name, args...)
	cmd.Stdout = nil
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
