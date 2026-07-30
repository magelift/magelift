// Package platform defines Magento-shaped ports shared by cloud adapters.
// Adapters under internal/cloud/<provider> implement topology; this package
// must not import AWS or GCP SDKs.
package platform

import (
	"fmt"
	"strings"
)

// Stable stack output keys read by portable CLI commands (outputs, health,
// deploy orchestration). Adapters must export every RequiredOutputKey from
// Program. Cloud-specific handles may be exported in addition.
const (
	OutputApplicationURL   = "applicationURL"
	OutputDatabaseWriter   = "databaseWriter"
	OutputCacheEndpoint    = "cacheEndpoint"
	OutputNetworkVpcID     = "networkVpcId"
	OutputClusterName      = "clusterName"
	OutputServiceName      = "serviceName"
	OutputPrivateSubnetIDs = "privateSubnetIds"
	// OutputMediaBucket is the Magento media object-storage bucket (env media-sync).
	// Not in RequiredOutputKeys yet — AWS historically exported mediaURL only; adapters
	// that support media-sync must export this key (GCP already does; AWS added in 05-05).
	OutputMediaBucket = "mediaBucket"
	// OutputKubeconfig is the cluster kubeconfig for shared Kubernetes day-2
	// (Observe/Steps). Optional Magento-facing key — not in RequiredOutputKeys so
	// ECS and other non-K8s stacks stay free of it. K8s adapters export it as a
	// Pulumi secret.
	OutputKubeconfig = "kubeconfig"
)

// RequiredOutputKeys are the Magento-facing exports every StackModule must
// produce. Providers may add extras (for example ECS task definition ARNs).
func RequiredOutputKeys() []string {
	return []string{
		OutputApplicationURL,
		OutputDatabaseWriter,
		OutputCacheEndpoint,
		OutputNetworkVpcID,
		OutputClusterName,
		OutputServiceName,
		OutputPrivateSubnetIDs,
	}
}

// RequireStringOutput reads a Magento-facing stack output as a non-empty string.
func RequireStringOutput(outputs map[string]any, key string) (string, error) {
	value, ok := outputs[key]
	if !ok {
		return "", fmt.Errorf("Pulumi output %q is required", key)
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("Pulumi output %q must be a non-empty string", key)
	}
	return text, nil
}

// RequireStringListOutput reads a Magento-facing stack output as a non-empty string list.
func RequireStringListOutput(outputs map[string]any, key string) ([]string, error) {
	value, ok := outputs[key]
	if !ok {
		return nil, fmt.Errorf("Pulumi output %q is required", key)
	}
	var values []string
	switch typed := value.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("Pulumi output %q contains a non-string value", key)
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("Pulumi output %q must be a string list", key)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("Pulumi output %q must not be empty", key)
	}
	return values, nil
}
