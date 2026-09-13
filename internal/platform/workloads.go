package platform

// Magento workload shell contracts. Cloud adapters pass these into ECS tasks /
// GKE Jobs/Deployments; they must not invent alternate Magento CLI sequences.

// MagentoMigrationShell is the pre-traffic migrate candidate command
// (config import, setup:upgrade, static content deploy, cache clean+flush).
func MagentoMigrationShell() []string {
	return []string{
		"/bin/sh", "-ec",
		"bin/magento app:config:import --no-interaction && " +
			"bin/magento setup:upgrade --keep-generated --no-interaction && " +
			"bin/magento setup:static-content:deploy --no-interaction && " +
			"bin/magento cache:clean && bin/magento cache:flush",
	}
}

// MagentoCronShell is the long-running cron loop used by certified runtimes.
func MagentoCronShell() []string {
	return []string{"/bin/sh", "-ec", "while true; do bin/magento cron:run; sleep 60; done"}
}

// MagentoQueueArgs starts Magento message-queue consumers.
func MagentoQueueArgs() []string {
	return MagentoQueueArgsFor(nil)
}

// MagentoQueueArgsFor starts the named Magento consumers, or Magento's default async worker.
func MagentoQueueArgsFor(names []string) []string {
	command := []string{"bin/magento", "queue:consumers:start"}
	if trimmed := nonEmptyStrings(names); len(trimmed) > 0 {
		command = append(command, trimmed...)
	} else {
		command = append(command, "async.operations.all")
	}
	return append(command, "--max-messages=10000")
}
