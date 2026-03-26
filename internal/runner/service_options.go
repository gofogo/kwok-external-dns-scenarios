package runner

import "github.com/kubernetes-sigs-issues/iac/kwok/internal/distribute"

// ServiceRunnerOption is a functional option for NewServiceRunner.
type ServiceRunnerOption func(*ServiceRunner)

// WithSvcDist sets the service type distribution (headless vs node-port).
func WithSvcDist(d distribute.Weights) ServiceRunnerOption {
	return func(r *ServiceRunner) { r.svcDist = d }
}

// WithPodDist sets the pod type distribution (headless-backing vs node-port-backing).
func WithPodDist(d distribute.Weights) ServiceRunnerOption {
	return func(r *ServiceRunner) { r.podDist = d }
}
