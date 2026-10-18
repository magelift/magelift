package edge

import (
	"testing"

	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
)

func TestNativeEdgeURLProjectsConfiguredDomain(t *testing.T) {
	ip := "192.0.2.10"
	status := ingressStatus(&ip, nil)
	if got := nativeEdgeIngressAddress(status); got != ip {
		t.Fatalf("ingress address = %q", got)
	}
	if got := nativeEdgeApplicationURL("shop.example.com", true, ip); got != "https://shop.example.com" {
		t.Fatalf("tls application URL = %q", got)
	}
	if got := nativeEdgeApplicationURL("shop.example.com", false, ip); got != "http://shop.example.com" {
		t.Fatalf("plaintext application URL = %q", got)
	}
	if got := nativeEdgeApplicationURL("shop.example.com", true, ip); got == "https://"+ip || got == "http://"+ip {
		t.Fatalf("application URL used the load balancer IP: %q", got)
	}
}

func TestNativeEdgeURLStaysUnavailableWithoutAddress(t *testing.T) {
	for _, status := range []*networkingv1.IngressStatus{nil, {}, {LoadBalancer: &networkingv1.IngressLoadBalancerStatus{}}, ingressStatus(str(""), str("  "))} {
		if got := nativeEdgeIngressAddress(status); got != "" {
			t.Fatalf("ingress address = %q, want empty", got)
		}
		if got := nativeEdgeApplicationURL("shop.example.com", true, nativeEdgeIngressAddress(status)); got != "" {
			t.Fatalf("application URL = %q, want empty", got)
		}
	}
	if got := nativeEdgeApplicationURL("  ", true, "192.0.2.10"); got != "" {
		t.Fatalf("blank domain application URL = %q", got)
	}
}

func TestNativeEdgeURLKeepsHostnameAddress(t *testing.T) {
	host := "edge.example.net"
	status := ingressStatus(nil, &host)
	address := nativeEdgeIngressAddress(status)
	if address != host {
		t.Fatalf("ingress address = %q", address)
	}
	if got := nativeEdgeApplicationURL("shop.example.com", true, address); got != "https://shop.example.com" {
		t.Fatalf("application URL = %q", got)
	}
}

func ingressStatus(ip, hostname *string) *networkingv1.IngressStatus {
	return &networkingv1.IngressStatus{
		LoadBalancer: &networkingv1.IngressLoadBalancerStatus{
			Ingress: []networkingv1.IngressLoadBalancerIngress{{Ip: ip, Hostname: hostname}},
		},
	}
}

func str(value string) *string { return &value }
