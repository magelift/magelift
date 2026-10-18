package stack

import (
	"github.com/magelift/magelift/internal/platform"
)

// OutputKeys lists the stack output keys the GCP program exports. The
// protocol server advertises it so the core module shim never duplicates
// the provider list.
func OutputKeys() []string {
	keys := append([]string(nil), platform.RequiredOutputKeys()...)
	return append(keys, "mediaURL", "mediaBucket", platform.OutputSearchEndpoint, platform.OutputQueueHost, platform.OutputQueueReplicas, platform.OutputDatabaseConnectionName, "queueMode", platform.OutputDatabaseSecretName, platform.OutputQueuePasswordSecretName, "securityPolicyName")
}
