package runner

import "fmt"

func kubeconfigFlag(path string) string {
	if path == "" {
		return ""
	}
	return " --kubeconfig " + path
}

func (r *GatewayRunner) Commands(kubeconfigPath string) []string {
	kc := kubeconfigFlag(kubeconfigPath)
	return []string{
		fmt.Sprintf("  kubectl get svc -n istio-system%s   # %d LoadBalancer services", kc, r.nServices),
		fmt.Sprintf("  kubectl get gateway -A%s            # %d gateways", kc, r.nGateways),
	}
}

func (r *VirtualServiceRunner) Commands(kubeconfigPath string) []string {
	kc := kubeconfigFlag(kubeconfigPath)
	return []string{
		fmt.Sprintf("  kubectl get virtualservice -A%s     # %d virtualservices", kc, r.nVirtualSvcs),
	}
}

func (r *PodRunner) Commands(kubeconfigPath string) []string {
	kc := kubeconfigFlag(kubeconfigPath)
	return []string{
		fmt.Sprintf("  kubectl get pods -n default%s       # %d benchmark pods", kc, r.nPods),
	}
}

func (r *DNSEndpointRunner) Commands(kubeconfigPath string) []string {
	kc := kubeconfigFlag(kubeconfigPath)
	return []string{
		fmt.Sprintf("  kubectl get dnsendpoints -A%s       # %d DNSEndpoint objects", kc, r.nEndpoints),
	}
}

func (r *ServiceRunner) Commands(kubeconfigPath string) []string {
	kc := kubeconfigFlag(kubeconfigPath)
	return []string{
		fmt.Sprintf("  kubectl get nodes%s                 # %d benchmark nodes", kc, r.nNodes),
		fmt.Sprintf("  kubectl get svc -n default%s        # %d services (headless + NodePort)", kc, r.nServices),
		fmt.Sprintf("  kubectl get pods -n default%s       # %d benchmark pods", kc, r.nPods),
	}
}
