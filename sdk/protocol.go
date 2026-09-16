package sdk

import (
	"errors"
	"fmt"
	"strings"
)

// Plugin protocol v1: versioned typed operations between the core and an
// autonomous provider subprocess. Every message carries ProtocolVersion;
// every response carries a typed error or a result, never both. All types
// are gob-safe (exported fields, no interfaces, no freestanding any).
// Rich pre-existing payloads (stack outputs, platform reports, adapter
// plans) cross as JSON bytes inside typed envelopes — the Kubernetes
// RawExtension pattern — with the JSON schema named on each field, so no
// wire type is ever an untyped blob.
const (
	// ProtocolV1 is the only protocol version this SDK speaks.
	ProtocolV1 = "1.0"
	// ProtocolMajorV1 is the compatibility boundary: majors must match.
	ProtocolMajorV1 = "1"
)

// ProtocolCompatible reports whether a plugin speaking peer is compatible
// with this host. Majors must match exactly; malformed versions fail
// closed. Minor skew is accepted: providers must ignore unknown fields
// and the core must tolerate unknown operations as unimplemented.
func ProtocolCompatible(peer string) bool {
	return protocolMajor(peer) == ProtocolMajorV1
}

func protocolMajor(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	major, minor, hasMinor := strings.Cut(version, ".")
	if major == "" || !isDigits(major) {
		return ""
	}
	if hasMinor && (minor == "" || !isDigits(minor) || strings.Contains(minor, ".")) {
		return ""
	}
	return major
}

func isDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// OperationErrorCode names a typed plugin failure. Codes are stable within
// a protocol major; hosts switch on codes, never on message text.
type OperationErrorCode string

const (
	ErrCodeIntegrity     OperationErrorCode = "integrity"
	ErrCodeCompatibility OperationErrorCode = "compatibility"
	ErrCodeCredential    OperationErrorCode = "credential"
	ErrCodeNotFound      OperationErrorCode = "not-found"
	ErrCodeConflict      OperationErrorCode = "conflict"
	ErrCodeUpstream      OperationErrorCode = "upstream"
	ErrCodeInvalid       OperationErrorCode = "invalid"
	ErrCodeTimeout       OperationErrorCode = "timeout"
	ErrCodeInternal      OperationErrorCode = "internal"
)

// OperationError is a typed plugin failure with human detail and a
// retryability verdict. It carries no secret values.
type OperationError struct {
	Code      OperationErrorCode `json:"code"`
	Message   string             `json:"message"`
	Retryable bool               `json:"retryable"`
}

func (e *OperationError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Operation names the versioned plugin operations. Server and client must
// agree on names; unknown operations on either side are unimplemented.
type Operation string

const (
	// Lifecycle: plan/apply/outputs/destroy plus handshake and validation.
	OpDescribe       Operation = "describe"
	OpValidateConfig Operation = "validate-config"
	OpPlan           Operation = "plan"
	OpApply          Operation = "apply"
	OpOutputs        Operation = "outputs"
	OpDestroy        Operation = "destroy"
	// Bootstrap mirrors platform.Bootstrap.
	OpBootstrapVerify Operation = "bootstrap-verify"
	OpBootstrapEnsure Operation = "bootstrap-ensure"
	// State mirrors platform.State (Pulumi state, locks, state backup).
	OpStateStatus  Operation = "state-status"
	OpStateLock    Operation = "state-lock"
	OpStateUnlock  Operation = "state-unlock"
	OpStateBackup  Operation = "state-backup"
	OpStateRestore Operation = "state-restore"
	// Secrets mirrors platform.Secrets plus the secretref value read.
	OpSecretList   Operation = "secret-list"
	OpSecretSet    Operation = "secret-set"
	OpSecretRemove Operation = "secret-remove"
	OpSecretRead   Operation = "secret-read"
	// Observe mirrors platform.RuntimeObserve.
	OpTailLogs     Operation = "tail-logs"
	OpCheckRuntime Operation = "check-runtime"
	OpPrepareExec  Operation = "prepare-exec"
	// Tunnel mirrors platform.RuntimeTunnel.
	OpPrepareTunnel Operation = "prepare-tunnel"
	// Cost mirrors platform.CostEstimator.
	OpCostInputs Operation = "cost-inputs"
	// Cleanup mirrors cli.CleanupProvider (ledger replay has no plan).
	OpInventory Operation = "inventory"
	OpDelete    Operation = "delete"
	// Adapter proxies forward sdk adapter calls (purge, recovery, cert).
	OpEdgePlan          Operation = "edge-plan"
	OpEdgeExecute       Operation = "edge-execute"
	OpResiliencePlan    Operation = "resilience-plan"
	OpResilienceExecute Operation = "resilience-execute"
)

// OperationVersion advertises one operation name and version.
type OperationVersion struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// DescribeRequest asks the plugin to identify itself.
type DescribeRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
}

// DescribeResponse carries the plugin identity and operation versions.
type DescribeResponse struct {
	ProtocolVersion string             `json:"protocolVersion"`
	ProviderID      string             `json:"providerId"`
	ProviderVersion string             `json:"providerVersion"`
	Operations      []OperationVersion `json:"operations"`
	Runtimes        []string           `json:"runtimes"`
}

// Envelope carries core-resolved deployment coordinates. Every plan-scoped
// operation takes it so the server never imports core config.
type Envelope struct {
	Project            string `json:"project"`
	Environment        string `json:"environment"`
	Region             string `json:"region"`
	EnvironmentClass   string `json:"environmentClass"`
	StackName          string `json:"stackName"`
	StateBackendURL    string `json:"stateBackendUrl"`
	SecretsProvider    string `json:"secretsProvider"`
	Preset             string `json:"preset,omitempty"`
	MonthlyBudgetCents int64  `json:"monthlyBudgetCents,omitempty"`
	AppVersion         string `json:"appVersion,omitempty"`
	ExpiresAt          string `json:"expiresAt,omitempty"`
	Protected          bool   `json:"protected,omitempty"`
	Domain             string `json:"domain,omitempty"`
}

// StoredPlan is a provider plan: stable identity fields the core may read
// plus opaque bytes the core stores uninspected and returns verbatim.
type StoredPlan struct {
	StackName       string `json:"stackName"`
	Provider        string `json:"provider"`
	Runtime         string `json:"runtime"`
	ImageDigest     string `json:"imageDigest"`
	StateBackendURL string `json:"stateBackendUrl,omitempty"`
	// DeployInputsJSON carries the JSON-encoded provider deploy inputs
	// (for GCP, a kube.DeploySpec) so the core can construct deploy
	// steps without reading the opaque plan.
	DeployInputsJSON []byte `json:"deployInputsJson,omitempty"`
	Opaque           []byte `json:"opaque,omitempty"`
}

// PlanRequest asks the provider to plan from core intents plus raw target
// bytes. The target block stays provider-owned: the core never parses it.
type PlanRequest struct {
	ProtocolVersion     string              `json:"protocolVersion"`
	Envelope            Envelope            `json:"envelope"`
	Application         Application         `json:"application"`
	Edge                EdgeIntent          `json:"edge"`
	Observability       ObservabilityIntent `json:"observability"`
	TargetBlock         []byte              `json:"targetBlock"`
	Runtime             string              `json:"runtime,omitempty"`
	LiveQueueReplicas   int                 `json:"liveQueueReplicas,omitempty"`
	ValkeyRequirement   string              `json:"valkeyRequirement,omitempty"`
	AllowUnsupported    bool                `json:"allowUnsupported,omitempty"`
	AllowExpiredPreview bool                `json:"allowExpiredPreview,omitempty"`
}

// PlanResult carries the stored plan or a typed validation failure.
type PlanResult struct {
	Plan  StoredPlan      `json:"plan,omitempty"`
	Error *OperationError `json:"error,omitempty"`
}

// ValidateConfigRequest asks the provider to validate a raw target block.
type ValidateConfigRequest struct {
	ProtocolVersion string `json:"protocolVersion"`
	TargetBlock     []byte `json:"targetBlock"`
}

// ValidateConfigResult carries the validation verdict.
type ValidateConfigResult struct {
	Valid    bool            `json:"valid"`
	Problems []string        `json:"problems,omitempty"`
	Error    *OperationError `json:"error,omitempty"`
}

// StackCall invokes a plan-scoped lifecycle operation (apply, outputs,
// destroy). The server unmarshals the opaque plan it stored.
type StackCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
}

// ChangeSummary counts lifecycle mutations for human output.
type ChangeSummary struct {
	Create int `json:"create"`
	Update int `json:"update"`
	Delete int `json:"delete"`
	Same   int `json:"same"`
}

// LifecycleResult carries apply/destroy outcomes.
type LifecycleResult struct {
	Summary     ChangeSummary   `json:"summary"`
	Diagnostics []string        `json:"diagnostics,omitempty"`
	Error       *OperationError `json:"error,omitempty"`
}

// OutputsResult carries stack outputs as a JSON object. The schema is the
// provider's exported output map (string keys, JSON values).
type OutputsResult struct {
	ValuesJSON []byte          `json:"valuesJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// BootstrapVerifyCall mirrors Bootstrap.VerifyAccount.
type BootstrapVerifyCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
}

// BootstrapVerifyResult carries the verification verdict.
type BootstrapVerifyResult struct {
	Verified bool            `json:"verified"`
	Error    *OperationError `json:"error,omitempty"`
}

// BootstrapEnsureCall mirrors Bootstrap.Ensure. RequestJSON encodes a
// platform.BootstrapRequest.
type BootstrapEnsureCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	RequestJSON     []byte     `json:"requestJson"`
}

// BootstrapEnsureResult carries the JSON-encoded platform.BootstrapResult.
type BootstrapEnsureResult struct {
	ResultJSON []byte          `json:"resultJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// StateStatusCall mirrors State.Status.
type StateStatusCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
}

// StateStatusResult carries the lock state. InfoJSON encodes a
// platform.LockInfo when a lock is present.
type StateStatusResult struct {
	Locked     bool            `json:"locked"`
	InfoJSON   []byte          `json:"infoJson,omitempty"`
	BackendURL string          `json:"backendUrl,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// StateLockCall mirrors State.Lock. Unlock is a separate operation, so no
// release closure crosses the wire; the shim's release calls state-unlock.
type StateLockCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	Owner           string     `json:"owner"`
}

// StateLockResult reports the lock acquisition.
type StateLockResult struct {
	Locked bool            `json:"locked"`
	Error  *OperationError `json:"error,omitempty"`
}

// StateUnlockCall mirrors State.Unlock.
type StateUnlockCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
}

// StateUnlockResult carries the released lock, if any, as platform.LockInfo.
type StateUnlockResult struct {
	InfoJSON []byte          `json:"infoJson,omitempty"`
	Error    *OperationError `json:"error,omitempty"`
}

// StateBackupCall mirrors State.Backup (Pulumi state snapshot).
type StateBackupCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
}

// StateBackupResult carries the JSON-encoded platform.BackupResult.
type StateBackupResult struct {
	ResultJSON []byte          `json:"resultJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// StateRestoreCall mirrors State.Restore (Pulumi state restore).
type StateRestoreCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	Location        string     `json:"location"`
}

// StateRestoreResult carries the JSON-encoded platform.RestoreResult.
type StateRestoreResult struct {
	ResultJSON []byte          `json:"resultJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// SecretListCall mirrors Secrets.List.
type SecretListCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
}

// SecretListResult carries the JSON-encoded []platform.SecretMeta.
type SecretListResult struct {
	SecretsJSON []byte          `json:"secretsJson,omitempty"`
	Error       *OperationError `json:"error,omitempty"`
}

// SecretSetCall mirrors Secrets.Set.
type SecretSetCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	Name            string     `json:"name"`
	Value           []byte     `json:"value"`
}

// SecretSetResult reports the write.
type SecretSetResult struct {
	Written bool            `json:"written"`
	Error   *OperationError `json:"error,omitempty"`
}

// SecretRemoveCall mirrors Secrets.Remove.
type SecretRemoveCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	Name            string     `json:"name"`
}

// SecretRemoveResult reports the removal.
type SecretRemoveResult struct {
	Removed bool            `json:"removed"`
	Error   *OperationError `json:"error,omitempty"`
}

// SecretReadRequest serves secretref value resolution. Name is a full
// version resource name; JSON-field extraction stays core-side.
type SecretReadRequest struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Envelope        Envelope `json:"envelope"`
	Name            string   `json:"name"`
}

// SecretReadResult carries the raw secret bytes.
type SecretReadResult struct {
	Value []byte          `json:"value,omitempty"`
	Error *OperationError `json:"error,omitempty"`
}

// TailLogsCall mirrors RuntimeObserve.TailLogs. QueryJSON encodes a
// platform.LogQuery; outputs are not needed (the plan locates the cluster).
type TailLogsCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	QueryJSON       []byte     `json:"queryJson"`
}

// TailLogsResult carries the JSON-encoded []platform.LogEvent.
type TailLogsResult struct {
	EventsJSON []byte          `json:"eventsJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// CheckRuntimeCall mirrors RuntimeObserve.CheckRuntime. OutputsJSON encodes
// the stack outputs map the health check reads.
type CheckRuntimeCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	OutputsJSON     []byte     `json:"outputsJson,omitempty"`
}

// CheckRuntimeResult carries the JSON-encoded []platform.RuntimeHealth.
type CheckRuntimeResult struct {
	HealthJSON []byte          `json:"healthJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// PrepareExecCall mirrors RuntimeObserve.PrepareExec. QueryJSON encodes a
// platform.ExecQuery; the target launcher runs client-side.
type PrepareExecCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	OutputsJSON     []byte     `json:"outputsJson,omitempty"`
	QueryJSON       []byte     `json:"queryJson"`
}

// PrepareExecResult carries the JSON-encoded platform.ExecTarget.
type PrepareExecResult struct {
	TargetJSON []byte          `json:"targetJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// PrepareTunnelCall mirrors RuntimeTunnel.PrepareTunnel. QueryJSON encodes
// a platform.TunnelQuery; the tunnel launcher runs client-side.
type PrepareTunnelCall struct {
	ProtocolVersion string     `json:"protocolVersion"`
	Envelope        Envelope   `json:"envelope"`
	Plan            StoredPlan `json:"plan"`
	OutputsJSON     []byte     `json:"outputsJson,omitempty"`
	QueryJSON       []byte     `json:"queryJson"`
}

// PrepareTunnelResult carries the JSON-encoded platform.ExecTarget.
type PrepareTunnelResult struct {
	TargetJSON []byte          `json:"targetJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// CostInputsRequest mirrors CostEstimator.Estimate without requiring a
// stored plan: cost works from resolved inputs plus the raw target block.
type CostInputsRequest struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Envelope        Envelope `json:"envelope"`
	TargetBlock     []byte   `json:"targetBlock"`
	Live            bool     `json:"live,omitempty"`
	Budget          bool     `json:"budget,omitempty"`
}

// CostInputsResult carries the JSON-encoded platform.CostReport.
type CostInputsResult struct {
	ReportJSON []byte          `json:"reportJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// InventoryCall mirrors CleanupProvider.Inventory. The ledger carries its
// own identity (provider, project, region, marker), so no envelope or plan
// is needed. RequestJSON encodes an sdk.CleanupInventoryRequest.
type InventoryCall struct {
	ProtocolVersion string `json:"protocolVersion"`
	RequestJSON     []byte `json:"requestJson"`
}

// InventoryResult carries the JSON-encoded []sdk.CleanupInventoryResource.
type InventoryResult struct {
	ResourcesJSON []byte          `json:"resourcesJson,omitempty"`
	Error         *OperationError `json:"error,omitempty"`
}

// DeleteCall mirrors CleanupProvider.Delete. ResourceJSON encodes an
// sdk.CleanupResource.
type DeleteCall struct {
	ProtocolVersion string `json:"protocolVersion"`
	ResourceJSON    []byte `json:"resourceJson"`
}

// DeleteResult reports the deletion.
type DeleteResult struct {
	Deleted bool            `json:"deleted"`
	Error   *OperationError `json:"error,omitempty"`
}

// Adapter proxy calls carry their adapter-defined payloads as JSON bytes
// inside typed envelopes (the Kubernetes RawExtension pattern): adapter
// types contain dynamic maps that gob cannot encode, so JSON- not gob—
// framing applies below this layer. Malformed payloads fail closed with a
// typed Invalid error; the operation names, versions, and error taxonomy
// stay fully typed.

// EdgePlanCall forwards an edge adapter Plan call; RequestJSON encodes an
// EdgePlanRequest.
type EdgePlanCall struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Envelope        Envelope `json:"envelope"`
	RequestJSON     []byte   `json:"requestJson"`
}

// EdgePlanResult carries the JSON-encoded edge plan.
type EdgePlanResult struct {
	PlanJSON []byte          `json:"planJson,omitempty"`
	Error    *OperationError `json:"error,omitempty"`
}

// EdgeExecuteCall forwards an edge adapter execution; RequestJSON encodes
// an EdgeExecutionRequest.
type EdgeExecuteCall struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Envelope        Envelope `json:"envelope"`
	RequestJSON     []byte   `json:"requestJson"`
}

// EdgeExecuteResult carries the JSON-encoded edge execution result.
type EdgeExecuteResult struct {
	ResultJSON []byte          `json:"resultJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// ResiliencePlanCall forwards a resilience adapter Plan call; RequestJSON
// encodes a ResiliencePlanRequest.
type ResiliencePlanCall struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Envelope        Envelope `json:"envelope"`
	RequestJSON     []byte   `json:"requestJson"`
}

// ResiliencePlanResult carries the JSON-encoded resilience plan.
type ResiliencePlanResult struct {
	PlanJSON []byte          `json:"planJson,omitempty"`
	Error    *OperationError `json:"error,omitempty"`
}

// ResilienceExecuteCall forwards a resilience stage execution;
// RequestJSON encodes a ResilienceExecutionRequest.
type ResilienceExecuteCall struct {
	ProtocolVersion string   `json:"protocolVersion"`
	Envelope        Envelope `json:"envelope"`
	RequestJSON     []byte   `json:"requestJson"`
}

// ResilienceExecuteResult carries the JSON-encoded stage result.
type ResilienceExecuteResult struct {
	ResultJSON []byte          `json:"resultJson,omitempty"`
	Error      *OperationError `json:"error,omitempty"`
}

// ValidateProtocolVersion fails closed on missing or malformed versions.
func ValidateProtocolVersion(version string) error {
	if strings.TrimSpace(version) == "" {
		return errors.New("protocol version is required")
	}
	if protocolMajor(version) == "" {
		return fmt.Errorf("protocol version %q is malformed", version)
	}
	if !ProtocolCompatible(version) {
		return fmt.Errorf("protocol version %q is not compatible with %q", version, ProtocolV1)
	}
	return nil
}

// RequireOperation fails closed when the peer does not advertise an operation.
func RequireOperation(operations []OperationVersion, name Operation, minimum string) error {
	for _, operation := range operations {
		if Operation(operation.Name) != name {
			continue
		}
		if operation.Version == "" {
			return fmt.Errorf("operation %q advertises an empty version", name)
		}
		if protocolMajor(operation.Version) != protocolMajor(minimum) {
			return fmt.Errorf("operation %q version %q is not compatible with %q", name, operation.Version, minimum)
		}
		return nil
	}
	return fmt.Errorf("operation %q is not supported by this provider", name)
}
