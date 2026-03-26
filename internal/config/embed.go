package config

import _ "embed"

//go:embed crds/istio.yaml
var IstioCRDs string

//go:embed crds/dnsendpoint.yaml
var DNSEndpointCRD string
