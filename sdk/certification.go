package sdk

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CertificationAction identifies the provider-side work a certification
// adapter can execute. The scheduler remains in the core; this port only
// carries one already-admitted execution unit into a provider implementation.
type CertificationAction string

const CertificationExecute CertificationAction = "execute"

// CertificationAdapterDescriptor identifies a provider implementation without
// importing a provider SDK into the core or public contract.
type CertificationAdapterDescriptor struct {
	APIVersion string                `json:"apiVersion" yaml:"apiVersion"`
	ID         string                `json:"id" yaml:"id"`
	Provider   ProviderID            `json:"provider" yaml:"provider"`
	Runtime    RuntimeID             `json:"runtime" yaml:"runtime"`
	Version    string                `json:"version" yaml:"version"`
	Actions    []CertificationAction `json:"actions" yaml:"actions"`
}

// CertificationExecutionUnit is the semantic projection of one core
// scheduler unit. Every field is an identity, digest, or provider-safe
// reference. Provider adapters must not mutate the unit or infer a missing
// boundary from native defaults.
type CertificationExecutionUnit struct {
	ID                     string                 `json:"id" yaml:"id"`
	CellIDs                []string               `json:"cellIds" yaml:"cellIds"`
	BaselineCellID         string                 `json:"baselineCellId" yaml:"baselineCellId"`
	Fingerprint            string                 `json:"fingerprint" yaml:"fingerprint"`
	WarmBoundary           string                 `json:"warmBoundary" yaml:"warmBoundary"`
	WarmTransition         CoverageTransitionKind `json:"warmTransition,omitempty" yaml:"warmTransition,omitempty"`
	Coverage               *CoverageBoundary      `json:"coverage,omitempty" yaml:"coverage,omitempty"`
	OwnershipMarker        string                 `json:"ownershipMarker" yaml:"ownershipMarker"`
	Cold                   bool                   `json:"cold" yaml:"cold"`
	MigrationOwner         bool                   `json:"migrationOwner" yaml:"migrationOwner"`
	DependsOn              []string               `json:"dependsOn,omitempty" yaml:"dependsOn,omitempty"`
	MutationKeys           []string               `json:"mutationKeys" yaml:"mutationKeys"`
	Required               bool                   `json:"required" yaml:"required"`
	ReuseSessionID         string                 `json:"reuseSessionId,omitempty" yaml:"reuseSessionId,omitempty"`
	ReuseFingerprintDigest string                 `json:"reuseFingerprintDigest,omitempty" yaml:"reuseFingerprintDigest,omitempty"`
}

// CertificationExecutionRequest is the single provider mutation boundary.
// The idempotency key is supplied by the core and must be used by the
// provider adapter for ambiguous retries.
type CertificationExecutionRequest struct {
	Unit           CertificationExecutionUnit `json:"unit" yaml:"unit"`
	IdempotencyKey string                     `json:"idempotencyKey" yaml:"idempotencyKey"`
}

// CertificationExecutionResult contains only identities and cleanup state.
// Secret values, rendered configuration, and provider SDK objects cannot
// cross this boundary.
type CertificationExecutionResult struct {
	StackID         string   `json:"stackId,omitempty" yaml:"stackId,omitempty"`
	OperationIDs    []string `json:"operationIds,omitempty" yaml:"operationIds,omitempty"`
	ResourceRefs    []string `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	BackupRefs      []string `json:"backupRefs,omitempty" yaml:"backupRefs,omitempty"`
	RestoreRefs     []string `json:"restoreRefs,omitempty" yaml:"restoreRefs,omitempty"`
	TelemetryRefs   []string `json:"telemetryRefs,omitempty" yaml:"telemetryRefs,omitempty"`
	EdgeRefs        []string `json:"edgeRefs,omitempty" yaml:"edgeRefs,omitempty"`
	ReuseMode       string   `json:"reuseMode,omitempty" yaml:"reuseMode,omitempty"`
	CleanupState    string   `json:"cleanupState,omitempty" yaml:"cleanupState,omitempty"`
	OwnershipMarker string   `json:"ownershipMarker" yaml:"ownershipMarker"`
}

// CertificationAdapter is the optional provider execution surface. Planning,
// admission, scheduling, checkpointing, and evidence promotion remain owned
// by the core; an adapter owns only native provisioning, polling, verification,
// and cleanup for the unit it receives.
type CertificationAdapter interface {
	CertificationDescriptor() CertificationAdapterDescriptor
	ExecuteCertification(context.Context, CertificationExecutionRequest) (CertificationExecutionResult, error)
}

// CertificationAdapterFactory constructs a provider client from a validated
// module plan. Construction must not mutate provider state.
type CertificationAdapterFactory interface {
	NewCertification(context.Context, ModulePlan) (CertificationAdapter, error)
}

// CertificationAdmissionAdapterDescriptor identifies a provider's read-only
// certification admission implementation. It is separate from
// CertificationAdapterDescriptor because a provider may expose admission
// before it is ready to execute a certification unit.
type CertificationAdmissionAdapterDescriptor struct {
	APIVersion string     `json:"apiVersion" yaml:"apiVersion"`
	ID         string     `json:"id" yaml:"id"`
	Provider   ProviderID `json:"provider" yaml:"provider"`
	Runtime    RuntimeID  `json:"runtime" yaml:"runtime"`
	Version    string     `json:"version" yaml:"version"`
}

// CertificationAdmissionRequest is the provider-neutral read-only gate for
// one certification scope. Credential references and resource identities are
// opaque; the provider adapter resolves them without returning their values.
type CertificationAdmissionRequest struct {
	Provider              ProviderID     `json:"provider" yaml:"provider"`
	AccountOrProjectRef   string         `json:"accountOrProjectRef" yaml:"accountOrProjectRef"`
	Region                string         `json:"region" yaml:"region"`
	NetworkRef            string         `json:"networkRef" yaml:"networkRef"`
	StateBackendRef       string         `json:"stateBackendRef" yaml:"stateBackendRef"`
	FixtureID             string         `json:"fixtureId" yaml:"fixtureId"`
	OwnershipMarker       string         `json:"ownershipMarker" yaml:"ownershipMarker"`
	CredentialRefs        []string       `json:"credentialRefs,omitempty" yaml:"credentialRefs,omitempty"`
	RequireCredentials    bool           `json:"requireCredentials" yaml:"requireCredentials"`
	RequiredQuota         map[string]int `json:"requiredQuota,omitempty" yaml:"requiredQuota,omitempty"`
	RequiredLocalCPUMilli int64          `json:"requiredLocalCpuMilli" yaml:"requiredLocalCpuMilli"`
	RequiredLocalMemoryMB int64          `json:"requiredLocalMemoryMb" yaml:"requiredLocalMemoryMb"`
	MutationKeys          []string       `json:"mutationKeys" yaml:"mutationKeys"`
}

// CertificationAdmissionResult contains only redacted readiness facts. A
// missing or false fact blocks a paid mutation; it is never interpreted as
// an unknown-but-ready value by the core.
type CertificationAdmissionResult struct {
	CredentialsReady       bool              `json:"credentialsReady" yaml:"credentialsReady"`
	AccountOrProjectReady  bool              `json:"accountOrProjectReady" yaml:"accountOrProjectReady"`
	NetworkReady           bool              `json:"networkReady" yaml:"networkReady"`
	StateBackendReady      bool              `json:"stateBackendReady" yaml:"stateBackendReady"`
	FixtureReady           bool              `json:"fixtureReady" yaml:"fixtureReady"`
	OwnershipReady         bool              `json:"ownershipReady" yaml:"ownershipReady"`
	AvailableQuota         map[string]int    `json:"availableQuota,omitempty" yaml:"availableQuota,omitempty"`
	AvailableLocalCPUMilli int64             `json:"availableLocalCpuMilli" yaml:"availableLocalCpuMilli"`
	AvailableLocalMemoryMB int64             `json:"availableLocalMemoryMb" yaml:"availableLocalMemoryMb"`
	Reasons                map[string]string `json:"reasons,omitempty" yaml:"reasons,omitempty"`
}

// CertificationAdmissionAdapter is the optional public provider port for
// live, read-only certification admission. The core owns the schema checks,
// snapshotting, ordering, and blocked-state semantics; the provider owns only
// SDK calls needed to establish these facts.
type CertificationAdmissionAdapter interface {
	CertificationAdmissionDescriptor() CertificationAdmissionAdapterDescriptor
	AdmitCertification(context.Context, CertificationAdmissionRequest) (CertificationAdmissionResult, error)
}

// CertificationAdmissionAdapterFactory constructs a read-only admission
// client after a module plan has been resolved. Construction must not mutate
// provider state.
type CertificationAdmissionAdapterFactory interface {
	NewCertificationAdmission(context.Context, ModulePlan) (CertificationAdmissionAdapter, error)
}

// ValidateCertificationAdmissionAdapterDescriptor validates stable metadata
// before an admission adapter is registered or used for a planned target.
func ValidateCertificationAdmissionAdapterDescriptor(descriptor CertificationAdmissionAdapterDescriptor) error {
	return errors.Join(
		func() error {
			if descriptor.APIVersion != ExtensionAPIVersion {
				return fmt.Errorf("certification admission adapter API version %q is not supported", descriptor.APIVersion)
			}
			return nil
		}(),
		validateID("certification admission adapter ID", descriptor.ID),
		validateID("certification admission adapter provider ID", string(descriptor.Provider)),
		validateID("certification admission adapter runtime ID", string(descriptor.Runtime)),
		validateExtensionVersion(descriptor.Version),
	)
}

// ValidateCertificationAdmissionRequest validates the public admission port
// without importing the internal scheduler's AdmissionInput type.
func ValidateCertificationAdmissionRequest(request CertificationAdmissionRequest) error {
	var problems []error
	problems = append(problems,
		validateID("certification admission provider ID", string(request.Provider)),
		validateAdmissionLine("certification admission account/project reference", request.AccountOrProjectRef),
		validateAdmissionLine("certification admission region", request.Region),
		validateAdmissionLine("certification admission network reference", request.NetworkRef),
		validateAdmissionLine("certification admission state backend reference", request.StateBackendRef),
		validateAdmissionLine("certification admission fixture ID", request.FixtureID),
		validateAdmissionLine("certification admission ownership marker", request.OwnershipMarker),
	)
	if len(request.CredentialRefs) == 0 && request.RequireCredentials {
		problems = append(problems, errors.New("certification admission requires credential references"))
	}
	for _, reference := range request.CredentialRefs {
		if err := ValidateCredentialReference(reference); err != nil {
			problems = append(problems, fmt.Errorf("certification admission credential reference: %w", err))
		}
	}
	problems = append(problems, validateUniqueStringLines("certification admission credential reference", request.CredentialRefs))
	problems = append(problems, validateUniqueStringLines("certification admission mutation key", request.MutationKeys))
	if len(request.MutationKeys) == 0 {
		problems = append(problems, errors.New("certification admission requires at least one mutation key"))
	}
	for name, values := range map[string]map[string]int{"required quota": request.RequiredQuota} {
		for key, value := range values {
			if strings.TrimSpace(key) == "" || value < 0 || strings.ContainsAny(key, "\r\n\x00") {
				problems = append(problems, fmt.Errorf("certification admission %s contains an invalid entry", name))
			}
		}
	}
	for name, value := range map[string]int64{
		"certification admission required local CPU":    request.RequiredLocalCPUMilli,
		"certification admission required local memory": request.RequiredLocalMemoryMB,
	} {
		if value < 0 {
			problems = append(problems, fmt.Errorf("%s cannot be negative", name))
		}
	}
	return errors.Join(problems...)
}

// ValidateCertificationAdmissionResult validates redacted provider facts
// before they enter the core snapshot or a checkpoint.
func ValidateCertificationAdmissionResult(result CertificationAdmissionResult) error {
	if result.AvailableLocalCPUMilli < 0 {
		return errors.New("certification admission result available local CPU cannot be negative")
	}
	if result.AvailableLocalMemoryMB < 0 {
		return errors.New("certification admission result available local memory cannot be negative")
	}
	for key, value := range result.AvailableQuota {
		if strings.TrimSpace(key) == "" || value < 0 || strings.ContainsAny(key, "\r\n\x00") {
			return errors.New("certification admission result contains an invalid quota")
		}
	}
	for key, reason := range result.Reasons {
		if strings.TrimSpace(key) == "" || strings.ContainsAny(key+reason, "\r\n\x00") {
			return errors.New("certification admission result contains an invalid reason")
		}
	}
	return nil
}

func validateAdmissionLine(name, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s is required and must be single-line", name)
	}
	return nil
}

func validateUniqueStringLines(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s values must be non-empty and single-line", name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

// ValidateCertificationAdapterDescriptor validates the stable adapter
// metadata before it is registered or used for a planned target.
func ValidateCertificationAdapterDescriptor(descriptor CertificationAdapterDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("certification adapter API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("certification adapter ID", descriptor.ID),
		validateID("certification adapter provider ID", string(descriptor.Provider)),
		validateID("certification adapter runtime ID", string(descriptor.Runtime)),
		validateExtensionVersion(descriptor.Version),
	)
	if len(descriptor.Actions) == 0 {
		problems = append(problems, errors.New("certification adapter must declare at least one action"))
	}
	seen := make(map[CertificationAction]struct{}, len(descriptor.Actions))
	for _, action := range descriptor.Actions {
		if action != CertificationExecute {
			problems = append(problems, fmt.Errorf("invalid certification adapter action %q", action))
		}
		if _, exists := seen[action]; exists {
			problems = append(problems, fmt.Errorf("duplicate certification adapter action %q", action))
		}
		seen[action] = struct{}{}
	}
	return errors.Join(problems...)
}

// ValidateCertificationExecutionUnit rejects incomplete or ambiguous unit
// identities before provider code is called.
func ValidateCertificationExecutionUnit(unit CertificationExecutionUnit) error {
	var problems []error
	for name, value := range map[string]string{
		"certification unit ID":               unit.ID,
		"certification unit fingerprint":      unit.Fingerprint,
		"certification unit warm boundary":    unit.WarmBoundary,
		"certification unit ownership marker": unit.OwnershipMarker,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("%s is required and must be single-line", name))
		}
	}
	if len(unit.CellIDs) == 0 {
		problems = append(problems, errors.New("certification unit requires at least one cell ID"))
	}
	problems = append(problems, validateUniqueLines("certification unit cell ID", unit.CellIDs))
	if unit.BaselineCellID != "" && !containsString(unit.CellIDs, unit.BaselineCellID) {
		problems = append(problems, errors.New("certification unit baseline cell ID must be one of its cell IDs"))
	}
	problems = append(problems,
		validateUniqueLines("certification unit dependency", unit.DependsOn),
		validateUniqueLines("certification unit mutation key", unit.MutationKeys),
	)
	if unit.Coverage != nil {
		problems = append(problems, unit.Coverage.Validate())
	}
	for name, value := range map[string]string{
		"certification unit reuse session ID":         unit.ReuseSessionID,
		"certification unit reuse fingerprint digest": unit.ReuseFingerprintDigest,
	} {
		if value != "" && strings.ContainsAny(value, "\r\n\x00") {
			problems = append(problems, fmt.Errorf("%s must be single-line and NUL-free", name))
		}
	}
	return errors.Join(problems...)
}

// ValidateCertificationExecutionRequest validates the provider mutation
// boundary, including the core-owned idempotency identity.
func ValidateCertificationExecutionRequest(request CertificationExecutionRequest) error {
	if err := ValidateCertificationExecutionUnit(request.Unit); err != nil {
		return err
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.ContainsAny(request.IdempotencyKey, "\r\n\x00") {
		return errors.New("certification execution idempotency key is required and must be single-line")
	}
	return nil
}

// ValidateCertificationExecutionResult verifies that a provider returned the
// same ownership scope it was given and only provider-safe references.
func ValidateCertificationExecutionResult(result CertificationExecutionResult, unit CertificationExecutionUnit) error {
	if err := ValidateCertificationExecutionUnit(unit); err != nil {
		return err
	}
	if result.OwnershipMarker != unit.OwnershipMarker {
		return errors.New("certification execution result ownership marker does not match the unit")
	}
	for name, value := range map[string]string{"stack ID": result.StackID, "ownership marker": result.OwnershipMarker, "reuse mode": result.ReuseMode, "cleanup state": result.CleanupState} {
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("certification execution result %s contains a control character", name)
		}
	}
	if result.ReuseMode != "cold" && result.ReuseMode != "warm-transition" && result.ReuseMode != "reused-session" {
		return fmt.Errorf("certification execution result has invalid reuse mode %q", result.ReuseMode)
	}
	if result.CleanupState != "" && result.CleanupState != "pending" && result.CleanupState != "complete" && result.CleanupState != "failed" && result.CleanupState != "protected" {
		return fmt.Errorf("certification execution result has invalid cleanup state %q", result.CleanupState)
	}
	for name, values := range map[string][]string{
		"operation ID": result.OperationIDs, "resource reference": result.ResourceRefs, "backup reference": result.BackupRefs,
		"restore reference": result.RestoreRefs, "telemetry reference": result.TelemetryRefs, "edge reference": result.EdgeRefs,
	} {
		if err := validateUniqueLines("certification execution "+name, values); err != nil {
			return err
		}
	}
	return nil
}

func validateUniqueLines(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("%s values must be non-empty and single-line", name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("duplicate %s %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func containsString(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
