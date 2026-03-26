// Package fixtures is the parent of source-specific fixture subpackages.
// Each subpackage owns one external-dns source type:
//
//	fixtures/helpers     — shared: RunConcurrent, CommonLabels, IP generators, EnsureNamespace
//	fixtures/istio       — GatewayFixture, VirtualServiceFixture  (only importer of istioclient)
//	fixtures/pod         — PodFixture
//	fixtures/dnsendpoint — DNSEndpointFixture
//	fixtures/service     — ServiceFixture  (only importer of distribute)
package fixtures
