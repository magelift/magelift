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
	// OutputDatabaseSecretName is the namespace-local Kubernetes Secret used by
	// K8s workloads and candidate migration Jobs for DB credentials.
	OutputDatabaseSecretName = "databaseSecretName"
	// OutputEncryptionKeySecretName is the namespace-local Kubernetes Secret
	// used by K8s workloads and candidate migration Jobs for Magento's stable
	// encryption key.
	OutputEncryptionKeySecretName = "encryptionKeySecretName"
	// OutputQueuePasswordSecretName is the namespace-local Kubernetes Secret
	// used by RabbitMQ workloads and candidate migration Jobs for the broker
	// password. It is present only when the selected queue mode needs AMQP.
	OutputQueuePasswordSecretName = "queuePasswordSecretName"
	// OutputSearchEndpoint is the internal OpenSearch endpoint consumed by
	// Magento web and candidate migration workloads. It is optional when
	// search is disabled.
	OutputSearchEndpoint = "searchEndpoint"
	// OutputQueueHost is the internal RabbitMQ service endpoint used by
	// Kubernetes runtimes. It is optional when database-backed messaging is
	// selected.
	OutputQueueHost = "queueHost"
	// OutputQueueReplicas is the live Magento broker replica count exported by
	// Kubernetes adapters that run RabbitMQ. Optional; missing means create.
	OutputQueueReplicas = "queueReplicas"
	// OutputDatabaseConnectionName is an optional provider-native connection
	// handle, such as a GCP Cloud SQL instance connection name, used by private
	// database tunnel adapters. It is never a credential.
	OutputDatabaseConnectionName = "databaseConnectionName"
	// OutputMediaBucket is the Magento media object-storage bucket (env media-sync).
	// Not in RequiredOutputKeys yet; AWS historically exported mediaURL only; adapters
	// that support media-sync must export this key (GCP already does; AWS added in 05-05).
	OutputMediaBucket = "mediaBucket"
	// OutputMediaURL is the base URL operators verify media delivery
	// through. The mechanism differs per provider: GCP serves through
	// the storefront (app-relative image URLs via get.php; the bucket
	// is private), so it exports the store media base. Direct bucket
	// URLs must never appear here for GCP.
	OutputMediaURL = "mediaURL"
	// OutputKubeconfig is the cluster kubeconfig for shared Kubernetes day-2
	// (Observe/Steps). Optional Magento-facing key; not in RequiredOutputKeys so
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
