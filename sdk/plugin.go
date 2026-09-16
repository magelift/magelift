package sdk

import (
	"fmt"
	"strings"
)

// PluginMethods maps every protocol operation to its go-plugin net/rpc
// method name. The server registers these methods and the core client calls
// them; both sides use this table so the wire names cannot drift.
var PluginMethods = map[Operation]string{
	OpDescribe:               "Plugin.Describe",
	OpValidateConfig:         "Plugin.ValidateConfig",
	OpPlan:                   "Plugin.Plan",
	OpPreview:                "Plugin.Preview",
	OpApply:                  "Plugin.Apply",
	OpOutputs:                "Plugin.Outputs",
	OpDestroy:                "Plugin.Destroy",
	OpBootstrapVerify:        "Plugin.BootstrapVerify",
	OpBootstrapEnsure:        "Plugin.BootstrapEnsure",
	OpStateStatus:            "Plugin.StateStatus",
	OpStateLock:              "Plugin.StateLock",
	OpStateUnlock:            "Plugin.StateUnlock",
	OpStateBackup:            "Plugin.StateBackup",
	OpStateRestore:           "Plugin.StateRestore",
	OpSecretList:             "Plugin.SecretList",
	OpSecretSet:              "Plugin.SecretSet",
	OpSecretRemove:           "Plugin.SecretRemove",
	OpSecretRead:             "Plugin.SecretRead",
	OpTailLogs:               "Plugin.TailLogs",
	OpCheckRuntime:           "Plugin.CheckRuntime",
	OpPrepareExec:            "Plugin.PrepareExec",
	OpPrepareTunnel:          "Plugin.PrepareTunnel",
	OpCostInputs:             "Plugin.CostInputs",
	OpInventory:              "Plugin.Inventory",
	OpDelete:                 "Plugin.Delete",
	OpDestroyLeftoverBackups: "Plugin.DestroyLeftoverBackups",
	OpEdgePlan:               "Plugin.EdgePlan",
	OpEdgeExecute:            "Plugin.EdgeExecute",
	OpResiliencePlan:         "Plugin.ResiliencePlan",
	OpResilienceExecute:      "Plugin.ResilienceExecute",
}

// PluginMethod returns the net/rpc method for an operation, failing closed
// on unknown operations.
func PluginMethod(operation Operation) (string, error) {
	method, ok := PluginMethods[operation]
	if !ok || strings.TrimSpace(method) == "" {
		return "", fmt.Errorf("operation %q has no plugin method", operation)
	}
	return method, nil
}
