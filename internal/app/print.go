package app

import (
	"fmt"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/runner"
)

func printInspectCommands(kubeconfigPath string, runners []runner.SourceRunner) {
	fmt.Println()
	fmt.Println("=== Inspect created resources ===")
	for _, sr := range runners {
		for _, cmd := range sr.Commands(kubeconfigPath) {
			fmt.Println(cmd)
		}
	}
	fmt.Println()
}

func printClusterSummary(kubeconfigPath, clusterName string, runners []runner.SourceRunner, cleanup bool) {
	if cleanup {
		return
	}
	fmt.Println()
	fmt.Println("=== Cluster ready — interact with it ===")
	fmt.Printf("  export KUBECONFIG=%s\n", kubeconfigPath)
	fmt.Println()
	seen := make(map[string]bool)
	for _, sr := range runners {
		for _, cmd := range sr.Commands("") {
			if !seen[cmd] {
				fmt.Println(cmd)
				seen[cmd] = true
			}
		}
	}
	fmt.Println()
	fmt.Println("=== Remove cluster when done ===")
	fmt.Printf("  kwokctl delete cluster --name %s\n", clusterName)
}
