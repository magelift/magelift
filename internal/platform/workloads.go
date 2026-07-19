package platform

import sdk "github.com/acourtiol/magelift/sdk/v1"

// Magento workload IDs shared across certified and experimental targets.
const (
	WorkloadWeb           sdk.WorkloadID = "web"
	WorkloadDeploy        sdk.WorkloadID = "deploy"
	WorkloadCron          sdk.WorkloadID = "cron"
	WorkloadQueueConsumer sdk.WorkloadID = "queue-consumer"
)

// ExperimentalMinimumCapabilities is the capability set a first experimental
// cloud adapter (GCP) must satisfy before search, queue brokers, or edge.
func ExperimentalMinimumCapabilities() []sdk.CapabilityID {
	return []sdk.CapabilityID{
		sdk.CapabilityDatabaseMySQL,
		sdk.CapabilityCacheValkey,
	}
}
