// Package kube holds Magento-shaped Kubernetes helpers shared by the GKE, MKS,
// and Kapsule runtime adapters (ADR 0008). Cloud topology stays per-provider;
// only the provider-agnostic Magento wiring lives here.
package kube

import (
	"fmt"

	"github.com/magelift/magelift/internal/platform"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// SkipAwaitAnnotations tells the Kubernetes provider not to block the Pulumi
// deployment on rollout/LoadBalancer/Job readiness. Magento boot (setup:upgrade,
// LB provisioning, migration Jobs) outlasts a preview budget, so we register the
// resources and let readiness settle out of band.
func SkipAwaitAnnotations() pulumi.StringMap {
	return pulumi.StringMap{"pulumi.com/skipAwait": pulumi.String("true")}
}

// ToStringArray lifts a plain string slice (e.g. a Magento container command)
// into a pulumi.StringArray.
func ToStringArray(values []string) pulumi.StringArray {
	out := make(pulumi.StringArray, 0, len(values))
	for _, value := range values {
		out = append(out, pulumi.String(value))
	}
	return out
}

// EnvVars converts resolved platform bindings into corev1 container env vars.
// Each Value needs its own address, so we copy per iteration.
func EnvVars(bindings []platform.EnvBinding) []corev1.EnvVar {
	env := make([]corev1.EnvVar, 0, len(bindings))
	for _, binding := range bindings {
		value := binding.Value
		env = append(env, corev1.EnvVar{Name: binding.Name, Value: &value})
	}
	return env
}

// BuildStaticTokenKubeconfig renders a kubeconfig that authenticates with a
// static bearer token instead of an exec auth plugin, so Magento runtime pods
// never depend on a provider CLI. Callers pass a fully-formed context name and
// server URL; caData is base64 CA cert data.
func BuildStaticTokenKubeconfig(contextName, server, caData, token string) string {
	return fmt.Sprintf(`apiVersion: v1
clusters:
- cluster:
    certificate-authority-data: %s
    server: %s
  name: %s
contexts:
- context:
    cluster: %s
    user: %s
  name: %s
current-context: %s
kind: Config
users:
- name: %s
  user:
    token: %s
`, caData, server, contextName, contextName, contextName, contextName, contextName, contextName, token)
}
