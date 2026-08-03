package platform

// Magento workload shell contracts. Cloud adapters pass these into ECS tasks /
// GKE Jobs/Deployments; they must not invent alternate Magento CLI sequences.

// MagentoMigrationShell is the pre-traffic migrate candidate command
// (config import, setup:upgrade, cache clean+flush).
func MagentoMigrationShell() []string {
	return []string{
		"/bin/sh", "-ec",
		"bin/magento app:config:import --no-interaction && " +
			"bin/magento setup:upgrade --keep-generated --no-interaction && " +
			"bin/magento cache:clean && bin/magento cache:flush",
	}
}

// MagentoCronShell is the long-running cron loop used by certified runtimes.
func MagentoCronShell() []string {
	return []string{"/bin/sh", "-ec", "while true; do bin/magento cron:run; sleep 60; done"}
}

// MagentoQueueArgs starts Magento message-queue consumers.
func MagentoQueueArgs() []string {
	return []string{"bin/magento", "queue:consumers:start", "--max-messages=10000"}
}
