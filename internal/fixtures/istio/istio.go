// Package istio provides fixture setup for Istio-based external-dns source benchmarking.
// It is the only fixtures subpackage that imports istio.io/client-go.
package istio

import (
	"context"
	"fmt"

	istionetworking "istio.io/api/networking/v1beta1"
	networkingv1 "istio.io/client-go/pkg/apis/networking/v1"
	istioclient "istio.io/client-go/pkg/clientset/versioned"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/fixtures/helpers"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/progress"
)

const (
	ingressNamespace = "istio-system"
	gwNamespace      = "default"
	vsNamespace      = "default"
	sharedGWName     = "shared-gateway"
)

// GatewayFixture creates LoadBalancer services in istio-system and Istio Gateway objects.
type GatewayFixture struct {
	KubeClient  kubernetes.Interface
	IstioClient istioclient.Interface
	NServices   int
	NGateways   int
	Concurrency int
}

// Setup creates NServices ingress services and NGateways Gateway objects.
// Gateways are distributed round-robin across the ingress services.
func (f *GatewayFixture) Setup(ctx context.Context) error {
	f.NServices = helpers.AtLeast(f.NServices, 1)
	f.Concurrency = helpers.AtLeast(f.Concurrency, 1)
	if err := helpers.EnsureNamespace(ctx, f.KubeClient, ingressNamespace); err != nil {
		return err
	}
	if err := f.createIngressServices(ctx); err != nil {
		return err
	}
	return f.createGateways(ctx)
}

func (f *GatewayFixture) createIngressServices(ctx context.Context) error {
	bar := progress.New("services", f.NServices)
	skipped, err := helpers.RunConcurrent(ctx, f.NServices, f.Concurrency, bar, func(i int) (bool, error) {
		svc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        svcName(i),
				Namespace:   ingressNamespace,
				Labels:      helpers.CommonLabels(svcSelector(i)),
				Annotations: helpers.CommonAnnotations(i),
			},
			Spec: corev1.ServiceSpec{
				Type:                          corev1.ServiceTypeLoadBalancer,
				AllocateLoadBalancerNodePorts: helpers.BoolPtr(false),
				Selector:                      svcSelector(i),
				Ports:                         []corev1.ServicePort{{Port: 80}},
			},
		}
		created, err := f.KubeClient.CoreV1().Services(ingressNamespace).Create(ctx, svc, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("create ingress service %d: %w", i, err)
		}
		wantStatus := corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{{IP: helpers.IngressIP(i)}},
			},
		}
		if err := helpers.RetryUpdateStatus(ctx,
			func() error {
				created.Status = wantStatus
				_, err := f.KubeClient.CoreV1().Services(ingressNamespace).UpdateStatus(ctx, created, metav1.UpdateOptions{})
				return err
			},
			func() error {
				var err error
				created, err = f.KubeClient.CoreV1().Services(ingressNamespace).Get(ctx, created.Name, metav1.GetOptions{})
				return err
			},
		); err != nil {
			return false, fmt.Errorf("update service status %d: %w", i, err)
		}
		return false, nil
	})
	helpers.LogSkipped(skipped, "service")
	return err
}

func (f *GatewayFixture) createGateways(ctx context.Context) error {
	bar := progress.New("gateways", f.NGateways)
	skipped, err := helpers.RunConcurrent(ctx, f.NGateways, f.Concurrency, bar, func(i int) (bool, error) {
		gw := &networkingv1.Gateway{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("gateway-%d", i),
				Namespace:   gwNamespace,
				Labels:      helpers.CommonLabels(map[string]string{"gateway-index": fmt.Sprintf("%d", i)}),
				Annotations: helpers.CommonAnnotations(i),
			},
			Spec: istionetworking.Gateway{
				Selector: svcSelector(i % f.NServices),
				Servers:  []*istionetworking.Server{{Hosts: []string{fmt.Sprintf("gw-%d.example.com", i)}}},
			},
		}
		_, err := f.IstioClient.NetworkingV1().Gateways(gwNamespace).Create(ctx, gw, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("create gateway: %w", err)
	}
	helpers.LogSkipped(skipped, "gateway")
	return nil
}

// VirtualServiceFixture creates a shared wildcard Gateway and VirtualService objects.
// GatewayFixture.Setup must run first (the shared gateway references service-0).
type VirtualServiceFixture struct {
	IstioClient istioclient.Interface
	NVS         int
	Concurrency int
}

// Setup creates 1 shared wildcard Gateway and NVS VirtualService objects.
func (f *VirtualServiceFixture) Setup(ctx context.Context) error {
	f.Concurrency = helpers.AtLeast(f.Concurrency, 1)
	if err := f.createSharedGateway(ctx); err != nil {
		return err
	}
	return f.createVirtualServices(ctx)
}

func (f *VirtualServiceFixture) createSharedGateway(ctx context.Context) error {
	gw := &networkingv1.Gateway{
		ObjectMeta: metav1.ObjectMeta{
			Name:        sharedGWName,
			Namespace:   ingressNamespace,
			Labels:      helpers.CommonLabels(map[string]string{"role": "shared"}),
			Annotations: helpers.CommonAnnotations(0),
		},
		Spec: istionetworking.Gateway{
			Selector: svcSelector(0),
			Servers:  []*istionetworking.Server{{Hosts: []string{"*"}}},
		},
	}
	_, err := f.IstioClient.NetworkingV1().Gateways(ingressNamespace).Create(ctx, gw, metav1.CreateOptions{})
	if k8serrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func (f *VirtualServiceFixture) createVirtualServices(ctx context.Context) error {
	gwRef := fmt.Sprintf("%s/%s", ingressNamespace, sharedGWName)
	bar := progress.New("virtualservices", f.NVS)
	skipped, err := helpers.RunConcurrent(ctx, f.NVS, f.Concurrency, bar, func(i int) (bool, error) {
		vs := &networkingv1.VirtualService{
			ObjectMeta: metav1.ObjectMeta{
				Name:        fmt.Sprintf("vs-%d", i),
				Namespace:   vsNamespace,
				Labels:      helpers.CommonLabels(map[string]string{"vs-index": fmt.Sprintf("%d", i)}),
				Annotations: helpers.CommonAnnotations(i),
			},
			Spec: istionetworking.VirtualService{
				Gateways: []string{gwRef},
				Hosts:    []string{fmt.Sprintf("vs-%d.example.com", i)},
			},
		}
		_, err := f.IstioClient.NetworkingV1().VirtualServices(vsNamespace).Create(ctx, vs, metav1.CreateOptions{})
		if k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		return false, err
	})
	if err != nil {
		return fmt.Errorf("create virtualservice: %w", err)
	}
	helpers.LogSkipped(skipped, "virtualservice")
	return nil
}

func svcName(i int) string { return fmt.Sprintf("istio-ingressgateway-%d", i) }

func svcSelector(i int) map[string]string {
	return map[string]string{"app": "istio-ingressgateway", "instance": fmt.Sprintf("%d", i)}
}
