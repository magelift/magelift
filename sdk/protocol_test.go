package sdk

import (
	"bytes"
	"encoding/gob"
	"reflect"
	"strings"
	"testing"
)

func TestProtocolCompatibleRequiresMajorMatch(t *testing.T) {
	t.Parallel()
	for _, version := range []string{"1.0", "1.7", " 1.0 "} {
		if !ProtocolCompatible(version) {
			t.Errorf("ProtocolCompatible(%q) = false, want true", version)
		}
	}
	for _, version := range []string{"", " ", "2.0", "0.9", "abc", "1.x", ".0", "v1"} {
		if ProtocolCompatible(version) {
			t.Errorf("ProtocolCompatible(%q) = true, want false (fail closed)", version)
		}
	}
}

func TestValidateProtocolVersion(t *testing.T) {
	t.Parallel()
	if err := ValidateProtocolVersion(ProtocolV1); err != nil {
		t.Fatalf("current version rejected: %v", err)
	}
	for _, version := range []string{"", "   ", "abc", ".0"} {
		if err := ValidateProtocolVersion(version); err == nil {
			t.Errorf("version %q accepted, want rejection", version)
		}
	}
}

func TestRequireOperation(t *testing.T) {
	t.Parallel()
	operations := []OperationVersion{{Name: string(OpTailLogs), Version: "1.0"}}
	if err := RequireOperation(operations, OpTailLogs, "1.0"); err != nil {
		t.Fatalf("RequireOperation(tail-logs) = %v", err)
	}
	if err := RequireOperation(operations, OpTailLogs, "2.0"); err == nil {
		t.Fatal("RequireOperation accepted a major version mismatch")
	}
	if err := RequireOperation(operations, OpPrepareExec, "1.0"); err == nil || !strings.Contains(err.Error(), "prepare-exec") {
		t.Fatalf("RequireOperation(prepare-exec) = %v, want missing-operation error", err)
	}
}

func TestOperationErrorText(t *testing.T) {
	t.Parallel()
	var nilErr *OperationError
	if nilErr.Error() != "" {
		t.Fatalf("nil error text = %q", nilErr.Error())
	}
	err := &OperationError{Code: ErrCodeCredential, Message: "re-authenticate", Retryable: false}
	if got := err.Error(); !strings.Contains(got, "credential") || !strings.Contains(got, "re-authenticate") {
		t.Fatalf("error text = %q", got)
	}
}

func TestProtocolOperationsEnumerated(t *testing.T) {
	t.Parallel()
	for _, operation := range []Operation{
		OpDescribe, OpValidateConfig, OpPlan, OpPreview, OpApply, OpOutputs, OpDestroy,
		OpBootstrapVerify, OpBootstrapEnsure,
		OpStateStatus, OpStateLock, OpStateUnlock, OpStateBackup, OpStateRestore,
		OpSecretList, OpSecretSet, OpSecretRemove, OpSecretRead,
		OpTailLogs, OpCheckRuntime, OpPrepareExec, OpPrepareTunnel,
		OpCostInputs, OpInventory, OpDelete, OpDestroyLeftoverBackups,
		OpEdgePlan, OpEdgeExecute, OpResiliencePlan, OpResilienceExecute,
		OpDeployAppPhase,
	} {
		if strings.TrimSpace(string(operation)) == "" {
			t.Errorf("empty operation name in protocol set")
		}
	}
}

func TestProtocolMessagesGobRoundTrip(t *testing.T) {
	t.Parallel()
	envelope := Envelope{Project: "p", Environment: "e", Region: "r", EnvironmentClass: "preview", StackName: "s", StateBackendURL: "gs://b", Preset: "preview", MonthlyBudgetCents: 1, AppVersion: "2.4.9"}
	messages := map[string]any{
		"DescribeRequest":              &DescribeRequest{ProtocolVersion: ProtocolV1},
		"DescribeResponse":             &DescribeResponse{ProtocolVersion: ProtocolV1, ProviderID: "gcp", ProviderVersion: "v", Operations: []OperationVersion{{Name: "status", Version: "1.0"}}, Runtimes: []RuntimeAdvertisement{{Runtime: "r", Tier: ExtensionTierCertified}}, OutputKeys: []string{"k"}, Edge: &EdgeAdapterDescriptor{ID: "e"}, Resilience: &ResilienceAdapterDescriptor{ID: "r"}},
		"ValidateConfigRequest":        &ValidateConfigRequest{ProtocolVersion: ProtocolV1, TargetBlock: []byte("a: b")},
		"ValidateConfigResult":         &ValidateConfigResult{Valid: true, Problems: []string{"p"}},
		"PlanRequest":                  &PlanRequest{ProtocolVersion: ProtocolV1, Envelope: envelope, Application: Application{Edition: "e", Magento: MagentoSettings{FrontName: "a"}, Email: EmailSettings{Mode: "smtp", Host: "h", Port: 587}}, TargetBlock: []byte("a: b")},
		"PlanResult":                   &PlanResult{Plan: StoredPlan{StackName: "s", Opaque: []byte("o")}},
		"StackCall":                    &StackCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"LifecycleResult":              &LifecycleResult{Summary: ChangeSummary{Create: 1, Replace: 2}, Diagnostics: []string{"d"}},
		"OutputsResult":                &OutputsResult{ValuesJSON: []byte(`{"k":"v"}`)},
		"BootstrapVerifyCall":          &BootstrapVerifyCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"BootstrapVerifyResult":        &BootstrapVerifyResult{Verified: true},
		"BootstrapEnsureCall":          &BootstrapEnsureCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, RequestJSON: []byte(`{}`)},
		"BootstrapEnsureResult":        &BootstrapEnsureResult{ResultJSON: []byte(`{}`)},
		"StateStatusCall":              &StateStatusCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"StateStatusResult":            &StateStatusResult{Locked: true, InfoJSON: []byte(`{}`), BackendURL: "b"},
		"StateLockCall":                &StateLockCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, Owner: "o"},
		"StateLockResult":              &StateLockResult{Locked: true},
		"StateUnlockCall":              &StateUnlockCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"StateUnlockResult":            &StateUnlockResult{InfoJSON: []byte(`{}`)},
		"StateBackupCall":              &StateBackupCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"StateBackupResult":            &StateBackupResult{ResultJSON: []byte(`{}`)},
		"StateRestoreCall":             &StateRestoreCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, Location: "l"},
		"StateRestoreResult":           &StateRestoreResult{ResultJSON: []byte(`{}`)},
		"SecretListCall":               &SecretListCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"SecretListResult":             &SecretListResult{SecretsJSON: []byte(`[]`)},
		"SecretSetCall":                &SecretSetCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, Name: "n", Value: []byte("v")},
		"SecretSetResult":              &SecretSetResult{Written: true},
		"SecretRemoveCall":             &SecretRemoveCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, Name: "n"},
		"SecretRemoveResult":           &SecretRemoveResult{Removed: true},
		"SecretReadRequest":            &SecretReadRequest{ProtocolVersion: ProtocolV1, Envelope: envelope, Name: "n"},
		"SecretReadResult":             &SecretReadResult{Value: []byte("v")},
		"TailLogsCall":                 &TailLogsCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, QueryJSON: []byte(`{}`)},
		"TailLogsResult":               &TailLogsResult{EventsJSON: []byte(`[]`)},
		"CheckRuntimeCall":             &CheckRuntimeCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, OutputsJSON: []byte(`{}`)},
		"CheckRuntimeResult":           &CheckRuntimeResult{HealthJSON: []byte(`[]`)},
		"PrepareExecCall":              &PrepareExecCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, OutputsJSON: []byte(`{}`), QueryJSON: []byte(`{}`)},
		"PrepareExecResult":            &PrepareExecResult{TargetJSON: []byte(`{}`)},
		"PrepareTunnelCall":            &PrepareTunnelCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, OutputsJSON: []byte(`{}`), QueryJSON: []byte(`{}`)},
		"PrepareTunnelResult":          &PrepareTunnelResult{TargetJSON: []byte(`{}`)},
		"CostInputsRequest":            &CostInputsRequest{ProtocolVersion: ProtocolV1, Envelope: envelope, TargetBlock: []byte("a: b"), Budget: true},
		"CostInputsResult":             &CostInputsResult{ReportJSON: []byte(`{}`)},
		"InventoryCall":                &InventoryCall{ProtocolVersion: ProtocolV1, RequestJSON: []byte(`{}`)},
		"InventoryResult":              &InventoryResult{ResourcesJSON: []byte(`[]`)},
		"DeleteCall":                   &DeleteCall{ProtocolVersion: ProtocolV1, Project: "p", Marker: "m", ResourceJSON: []byte(`{}`)},
		"DeleteResult":                 &DeleteResult{Deleted: true},
		"DestroyLeftoverBackupsCall":   &DestroyLeftoverBackupsCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}},
		"DestroyLeftoverBackupsResult": &DestroyLeftoverBackupsResult{Destroyed: []string{"b"}},
		"EdgePlanCall":                 &EdgePlanCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, RequestJSON: []byte(`{"a":1}`)},
		"EdgePlanResult":               &EdgePlanResult{PlanJSON: []byte(`{"a":1}`)},
		"EdgeExecuteCall":              &EdgeExecuteCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, RequestJSON: []byte(`{"a":1}`)},
		"EdgeExecuteResult":            &EdgeExecuteResult{ResultJSON: []byte(`{"a":1}`)},
		"ResiliencePlanCall":           &ResiliencePlanCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, RequestJSON: []byte(`{"a":1}`)},
		"ResiliencePlanResult":         &ResiliencePlanResult{PlanJSON: []byte(`{"a":1}`)},
		"ResilienceExecuteCall":        &ResilienceExecuteCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, RequestJSON: []byte(`{"a":1}`)},
		"ResilienceExecuteResult":      &ResilienceExecuteResult{ResultJSON: []byte(`{"a":1}`)},
		"DeployAppPhaseCall":           &DeployAppPhaseCall{ProtocolVersion: ProtocolV1, Envelope: envelope, Plan: StoredPlan{StackName: "s"}, Phase: DeployPhaseMigrate, ImageDigest: "img@sha256:abc", OutputsJSON: []byte(`{"k":"v"}`), StateJSON: []byte(`{"s":1}`)},
		"DeployAppPhaseResult":         &DeployAppPhaseResult{StateJSON: []byte(`{"s":2}`), Message: "migrated"},
	}
	for name, message := range messages {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(message); err != nil {
			t.Errorf("%s gob encode: %v", name, err)
			continue
		}
		fresh := reflect.New(reflect.TypeOf(message).Elem()).Interface()
		if err := gob.NewDecoder(&buf).Decode(fresh); err != nil {
			t.Errorf("%s gob decode: %v", name, err)
		}
	}
}

func TestPluginMethodsCoverAllOperations(t *testing.T) {
	t.Parallel()
	operations := []Operation{
		OpDescribe, OpValidateConfig, OpPlan, OpPreview, OpApply, OpOutputs, OpDestroy,
		OpBootstrapVerify, OpBootstrapEnsure,
		OpStateStatus, OpStateLock, OpStateUnlock, OpStateBackup, OpStateRestore,
		OpSecretList, OpSecretSet, OpSecretRemove, OpSecretRead,
		OpTailLogs, OpCheckRuntime, OpPrepareExec, OpPrepareTunnel,
		OpCostInputs, OpInventory, OpDelete, OpDestroyLeftoverBackups,
		OpEdgePlan, OpEdgeExecute, OpResiliencePlan, OpResilienceExecute,
		OpDeployAppPhase,
	}
	if len(PluginMethods) != len(operations) {
		t.Fatalf("PluginMethods has %d entries, want %d", len(PluginMethods), len(operations))
	}
	for _, operation := range operations {
		method, err := PluginMethod(operation)
		if err != nil {
			t.Errorf("PluginMethod(%q): %v", operation, err)
			continue
		}
		if !strings.HasPrefix(method, "Plugin.") || strings.ContainsAny(method, "- ") {
			t.Errorf("method %q for %q is not a valid rpc name", method, operation)
		}
	}
	if _, err := PluginMethod("nope"); err == nil {
		t.Error("PluginMethod(unknown) succeeded, want error")
	}
}
