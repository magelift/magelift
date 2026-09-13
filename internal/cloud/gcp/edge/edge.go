// Package edge adds the GKE-native Google Cloud edge path. Cloud Armor is a
// provider resource; the GKE BackendConfig, NEG-backed Ingress, Cloud CDN,
// and managed certificate stay in the Kubernetes adapter because that is the
// API which owns the workload Service.
package edge

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	networkingv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/networking/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:Edge"

type Args struct {
	Project         string
	Enabled         bool
	NativeProvider  string
	DomainName      string
	TLS             bool
	TLSMode         string
	DNSMode         string
	OwnershipMarker string
	Labels          map[string]string
}

type Component struct {
	pulumi.ResourceState
	SecurityPolicyName pulumi.StringOutput
	BackendConfigName  pulumi.StringOutput
	IngressName        pulumi.StringOutput
	CertificateName    pulumi.StringOutput
	ApplicationURL     pulumi.StringOutput
	NativeEdgeEnabled  bool
	NativeProvider     string
	DomainName         string
	TLS                bool
	SecurityEnabled    bool
}

type AttachArgs struct {
	KubernetesProvider *kubernetes.Provider
	ServiceName        pulumi.StringInput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("edge name is required")
	}
	native := strings.TrimSpace(args.NativeProvider)
	if native != "" && native != "none" {
		if strings.TrimSpace(args.DomainName) == "" {
			return nil, errors.New("GCP native edge requires a domain")
		}
		if !validNativeProvider(native) {
			return nil, fmt.Errorf("unsupported GCP native edge provider %q", native)
		}
		if args.TLS && strings.TrimSpace(args.TLSMode) == "" {
			return nil, errors.New("GCP native edge TLS requires an explicit certificate mode")
		}
		if args.TLS && strings.TrimSpace(args.DNSMode) == "" {
			return nil, errors.New("GCP native edge TLS requires an explicit DNS ownership mode")
		}
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	component.NativeEdgeEnabled = native != "" && native != "none"
	component.NativeProvider = native
	component.DomainName = args.DomainName
	component.TLS = args.TLS
	component.SecurityEnabled = args.Enabled
	if component.NativeEdgeEnabled {
		component.BackendConfigName = pulumi.String(name + "-backend").ToStringOutput()
		component.IngressName = pulumi.String(name + "-ingress").ToStringOutput()
		if args.TLS {
			component.CertificateName = pulumi.String(name + "-certificate").ToStringOutput()
		}
	}
	if !args.Enabled {
		component.SecurityPolicyName = pulumi.String("").ToStringOutput()
		_ = ctx.RegisterResourceOutputs(component, pulumi.Map{"securityPolicyName": component.SecurityPolicyName, "backendConfigName": component.BackendConfigName, "ingressName": component.IngressName, "certificateName": component.CertificateName, "applicationURL": component.ApplicationURL})
		return component, nil
	}
	if strings.TrimSpace(args.Project) == "" {
		return nil, errors.New("GCP project is required for Cloud Armor")
	}
	parent := pulumi.Parent(component)
	policy, err := compute.NewSecurityPolicy(ctx, name+"-armor", &compute.SecurityPolicyArgs{
		Project:               pulumi.String(args.Project),
		Name:                  pulumi.String(name + "-armor"),
		Type:                  pulumi.String("CLOUD_ARMOR"),
		Description:           pulumi.String("Magento-safe Cloud Armor WAF"),
		AdvancedOptionsConfig: magentoArmorAdvancedOptions(),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Cloud Armor policy: %w", err)
	}
	// Body exclusions are beta in the Pulumi schema and are accepted by the
	// provider's dedicated SecurityPolicyRule resource. The nested rules field
	// on SecurityPolicy is rejected during provider Check when it contains
	// requestBodies, so keep every rule as an independently managed child.
	for _, rule := range magentoArmorRules() {
		ruleArgs := rule.Args
		ruleArgs.SecurityPolicy = policy.Name
		if _, ruleErr := compute.NewSecurityPolicyRule(ctx, name+"-armor-"+rule.Name, &ruleArgs, parent, pulumi.DependsOn([]pulumi.Resource{policy})); ruleErr != nil {
			return nil, fmt.Errorf("create Cloud Armor rule %q: %w", rule.Name, ruleErr)
		}
	}
	component.SecurityPolicyName = policy.Name
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"securityPolicyName": component.SecurityPolicyName, "backendConfigName": component.BackendConfigName, "ingressName": component.IngressName, "certificateName": component.CertificateName, "applicationURL": component.ApplicationURL}); err != nil {
		return nil, err
	}
	return component, nil
}

// Attach creates the Kubernetes-owned half of the GCP native edge. It is
// separate from New because the workload Service and Kubernetes provider are
// created by the runtime component after the Cloud Armor policy.
func Attach(ctx *pulumi.Context, name string, component *Component, args AttachArgs, opts ...pulumi.ResourceOption) error {
	if component == nil || !component.NativeEdgeEnabled {
		return nil
	}
	if args.KubernetesProvider == nil || args.ServiceName == nil {
		return errors.New("GCP native edge requires the runtime Kubernetes provider and Service name")
	}
	parent := pulumi.Parent(component)
	childOpts := append([]pulumi.ResourceOption{parent, pulumi.Provider(args.KubernetesProvider)}, opts...)
	backendSpec := kubernetes.UntypedArgs{"cdn": kubernetes.UntypedArgs{"enabled": pulumi.Bool(nativeCDNEnabled(component.NativeProvider))}}
	if component.SecurityEnabled {
		backendSpec["securityPolicy"] = kubernetes.UntypedArgs{"name": component.SecurityPolicyName}
	}
	backend, err := apiextensions.NewCustomResource(ctx, name+"-backend", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cloud.google.com/v1"), Kind: pulumi.String("BackendConfig"),
		Metadata: &metav1.ObjectMetaArgs{Name: component.BackendConfigName}, OtherFields: kubernetes.UntypedArgs{"spec": backendSpec},
	}, childOpts...)
	if err != nil {
		return fmt.Errorf("create GKE BackendConfig: %w", err)
	}
	annotations := pulumi.StringMap{
		"cloud.google.com/backend-config": pulumi.Sprintf(`{"default":"%s"}`, component.BackendConfigName),
		"cloud.google.com/neg":            pulumi.String(`{"ingress": true}`),
		// GKE's external Ingress controller still selects the classic
		// Application Load Balancer through this annotation. The
		// networking.k8s.io/v1 IngressClassName field is not sufficient for
		// GKE-managed Ingress and is explicitly unsupported by its TLS path.
		"kubernetes.io/ingress.class": pulumi.String("gce"),
	}
	certificateDependency := []pulumi.Resource{backend}
	if component.TLS {
		certificate, certificateErr := apiextensions.NewCustomResource(ctx, name+"-certificate", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("networking.gke.io/v1"), Kind: pulumi.String("ManagedCertificate"),
			Metadata: &metav1.ObjectMetaArgs{Name: component.CertificateName}, OtherFields: kubernetes.UntypedArgs{"spec": kubernetes.UntypedArgs{"domains": pulumi.StringArray{pulumi.String(component.DomainName)}}},
		}, childOpts...)
		if certificateErr != nil {
			return fmt.Errorf("create GKE managed certificate: %w", certificateErr)
		}
		certificateDependency = append(certificateDependency, certificate)
		annotations["networking.gke.io/managed-certificates"] = component.CertificateName
	}
	paths := networkingv1.HTTPIngressPathArray{&networkingv1.HTTPIngressPathArgs{
		Path: pulumi.StringPtr("/"), PathType: pulumi.String("Prefix"), Backend: &networkingv1.IngressBackendArgs{Service: &networkingv1.IngressServiceBackendArgs{Name: args.ServiceName, Port: &networkingv1.ServiceBackendPortArgs{Number: pulumi.IntPtr(80)}}},
	}}
	ingressSpec := &networkingv1.IngressSpecArgs{
		Rules: networkingv1.IngressRuleArray{&networkingv1.IngressRuleArgs{Host: pulumi.StringPtr(component.DomainName), Http: &networkingv1.HTTPIngressRuleValueArgs{Paths: paths}}},
	}
	ingressArgs := &networkingv1.IngressArgs{Metadata: &metav1.ObjectMetaArgs{Name: component.IngressName, Annotations: annotations}, Spec: ingressSpec}
	ingress, err := networkingv1.NewIngress(ctx, name+"-ingress", ingressArgs, append(childOpts, pulumi.DependsOn(certificateDependency))...)
	if err != nil {
		return fmt.Errorf("create GKE native edge Ingress: %w", err)
	}
	component.ApplicationURL = ingress.Status.ApplyT(func(status *networkingv1.IngressStatus) string {
		if status == nil || status.LoadBalancer == nil || len(status.LoadBalancer.Ingress) == 0 {
			return ""
		}
		entry := status.LoadBalancer.Ingress[0]
		scheme := "http"
		if component.TLS {
			scheme = "https"
		}
		if entry.Ip != nil && *entry.Ip != "" {
			return scheme + "://" + *entry.Ip
		}
		if entry.Hostname != nil && *entry.Hostname != "" {
			return scheme + "://" + *entry.Hostname
		}
		return ""
	}).(pulumi.StringOutput)
	return ctx.RegisterResourceOutputs(component, pulumi.Map{"securityPolicyName": component.SecurityPolicyName, "backendConfigName": component.BackendConfigName, "ingressName": component.IngressName, "certificateName": component.CertificateName, "applicationURL": component.ApplicationURL})
}

func validNativeProvider(provider string) bool {
	switch provider {
	case "cloud-cdn", "cloud-armor", "google-cloud-load-balancing":
		return true
	default:
		return false
	}
}

func nativeCDNEnabled(provider string) bool {
	return provider == "cloud-cdn"
}
