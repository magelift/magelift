// Package platform defines Magento-shaped ports shared by cloud adapters.
// Adapters under internal/cloud/<provider> implement topology; this package
// must not import AWS or GCP SDKs.
package platform

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
