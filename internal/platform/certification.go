package platform

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/certification"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// CertificationExecutorFor resolves the provider-owned certification port and
// adapts it to the core scheduler. The adapter receives only a defensive,
// provider-neutral projection; scheduler invariants are checked again after
// the callback returns so a provider cannot widen its ownership scope.
func CertificationExecutorFor(ctx context.Context, module StackModule, planned PlannedStack) (certification.ScheduleExecutor, error) {
	adapter, err := ModuleCertificationFor(ctx, module, planned)
	if err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, nil
	}
	return func(executionCtx context.Context, unit certification.ExecutionUnit) (certification.ScheduleExecutionResult, error) {
		publicUnit := certificationExecutionUnit(unit)
		request := sdk.CertificationExecutionRequest{
			Unit:           publicUnit,
			IdempotencyKey: unit.ID + ":" + unit.Fingerprint,
		}
		if err := sdk.ValidateCertificationExecutionRequest(request); err != nil {
			return certification.ScheduleExecutionResult{}, fmt.Errorf("validate certification unit %q: %w", unit.ID, err)
		}
		result, executeErr := adapter.ExecuteCertification(executionCtx, request)
		if executeErr != nil {
			// Preserve only provider-safe identities returned alongside an
			// ambiguous error. The core writes them to the failure checkpoint
			// so a retry can reconcile the same operation instead of creating
			// a duplicate resource. An invalid partial result is discarded and
			// turns into a hard failure rather than entering the checkpoint.
			if safetyErr := certification.ValidateSecretSafeValue(result); safetyErr != nil {
				return certification.ScheduleExecutionResult{}, errors.Join(executeErr, fmt.Errorf("provider returned secret-like certification state: %w", safetyErr))
			}
			if result.OwnershipMarker == "" {
				result.OwnershipMarker = publicUnit.OwnershipMarker
			}
			if result.ReuseMode == "" {
				result.ReuseMode = defaultCertificationReuseMode(unit)
			}
			if validationErr := sdk.ValidateCertificationExecutionResult(result, publicUnit); validationErr != nil {
				return certification.ScheduleExecutionResult{}, errors.Join(executeErr, fmt.Errorf("provider returned invalid partial certification state: %w", validationErr))
			}
			return certificationExecutionResult(result), executeErr
		}
		if result.ReuseMode == "" {
			result.ReuseMode = defaultCertificationReuseMode(unit)
		}
		if err := sdk.ValidateCertificationExecutionResult(result, publicUnit); err != nil {
			return certification.ScheduleExecutionResult{}, fmt.Errorf("validate certification result for unit %q: %w", unit.ID, err)
		}
		return certificationExecutionResult(result), nil
	}, nil
}

// CertificationAdmissionFor resolves the public provider admission port and
// adapts it to the core's run-scoped admission snapshot. Provider SDK request
// and response models stop at this boundary; the core retains all ordering,
// blocked-state, caching, and secret-safety rules.
func CertificationAdmissionFor(ctx context.Context, module StackModule, planned PlannedStack) (certification.AdmissionProbe, error) {
	adapter, err := ModuleCertificationAdmissionFor(ctx, module, planned)
	if err != nil {
		return nil, err
	}
	if adapter == nil {
		return nil, nil
	}
	return certificationAdmissionProbeFunc(func(probeCtx context.Context, input certification.AdmissionInput) (certification.AdmissionProbeResult, error) {
		request := sdk.CertificationAdmissionRequest{
			Provider:              sdk.ProviderID(input.Provider),
			AccountOrProjectRef:   input.AccountOrProjectRef,
			Region:                input.Region,
			NetworkRef:            input.NetworkRef,
			StateBackendRef:       input.StateBackendRef,
			FixtureID:             input.FixtureID,
			OwnershipMarker:       input.OwnershipMarker,
			CredentialRefs:        append([]string(nil), input.CredentialRefs...),
			RequireCredentials:    input.RequireCredentials,
			RequiredQuota:         cloneIntMap(input.RequiredQuota),
			RequiredLocalCPUMilli: input.RequiredLocalCPUMilli,
			RequiredLocalMemoryMB: input.RequiredLocalMemoryMB,
			MutationKeys:          append([]string(nil), input.MutationKeys...),
		}
		if err := sdk.ValidateCertificationAdmissionRequest(request); err != nil {
			return certification.AdmissionProbeResult{}, fmt.Errorf("validate certification admission request: %w", err)
		}
		if probeCtx == nil {
			return certification.AdmissionProbeResult{}, errors.New("certification admission context is required")
		}
		result, admitErr := adapter.AdmitCertification(probeCtx, request)
		if safetyErr := certification.ValidateSecretSafeValue(result); safetyErr != nil {
			return certification.AdmissionProbeResult{}, errors.New("provider returned secret-like certification admission state")
		}
		if validationErr := sdk.ValidateCertificationAdmissionResult(result); validationErr != nil {
			return certification.AdmissionProbeResult{}, fmt.Errorf("validate certification admission result: %w", validationErr)
		}
		if admitErr != nil {
			if errors.Is(admitErr, context.Canceled) || errors.Is(admitErr, context.DeadlineExceeded) {
				return certification.AdmissionProbeResult{}, admitErr
			}
			return certification.AdmissionProbeResult{}, errors.New("provider certification admission is unavailable")
		}
		return certification.AdmissionProbeResult{
			CredentialsReady:       result.CredentialsReady,
			AccountOrProjectReady:  result.AccountOrProjectReady,
			NetworkReady:           result.NetworkReady,
			StateBackendReady:      result.StateBackendReady,
			FixtureReady:           result.FixtureReady,
			OwnershipReady:         result.OwnershipReady,
			AvailableQuota:         cloneIntMap(result.AvailableQuota),
			AvailableLocalCPUMilli: result.AvailableLocalCPUMilli,
			AvailableLocalMemoryMB: result.AvailableLocalMemoryMB,
			Reasons:                cloneStringMap(result.Reasons),
		}, nil
	}), nil
}

type certificationAdmissionProbeFunc func(context.Context, certification.AdmissionInput) (certification.AdmissionProbeResult, error)

func (probe certificationAdmissionProbeFunc) Probe(ctx context.Context, input certification.AdmissionInput) (certification.AdmissionProbeResult, error) {
	return probe(ctx, input)
}

func certificationExecutionUnit(unit certification.ExecutionUnit) sdk.CertificationExecutionUnit {
	var reuseFingerprintDigest string
	if unit.ReuseFingerprint != nil {
		if digest, err := unit.ReuseFingerprint.Digest(); err == nil {
			reuseFingerprintDigest = digest
		}
	}
	return sdk.CertificationExecutionUnit{
		ID:                     unit.ID,
		CellIDs:                append([]string(nil), unit.CellIDs...),
		BaselineCellID:         unit.BaselineCellID,
		Fingerprint:            unit.Fingerprint,
		WarmBoundary:           unit.WarmBoundary,
		WarmTransition:         unit.WarmTransition,
		Coverage:               cloneCoverageBoundary(unit.Coverage),
		OwnershipMarker:        unit.OwnershipMarker,
		Cold:                   unit.Cold,
		MigrationOwner:         unit.MigrationOwner,
		DependsOn:              append([]string(nil), unit.DependsOn...),
		MutationKeys:           append([]string(nil), unit.MutationKeys...),
		Required:               unit.Required,
		ReuseSessionID:         unit.ReuseSessionID,
		ReuseFingerprintDigest: reuseFingerprintDigest,
	}
}

func certificationExecutionResult(result sdk.CertificationExecutionResult) certification.ScheduleExecutionResult {
	return certification.ScheduleExecutionResult{
		StackID:         result.StackID,
		OperationIDs:    append([]string(nil), result.OperationIDs...),
		ResourceRefs:    append([]string(nil), result.ResourceRefs...),
		BackupRefs:      append([]string(nil), result.BackupRefs...),
		RestoreRefs:     append([]string(nil), result.RestoreRefs...),
		TelemetryRefs:   append([]string(nil), result.TelemetryRefs...),
		EdgeRefs:        append([]string(nil), result.EdgeRefs...),
		ReuseMode:       result.ReuseMode,
		CleanupState:    result.CleanupState,
		OwnershipMarker: result.OwnershipMarker,
	}
}

func defaultCertificationReuseMode(unit certification.ExecutionUnit) string {
	switch {
	case unit.ReuseSessionID != "":
		return "reused-session"
	case unit.Cold:
		return "cold"
	default:
		return "warm-transition"
	}
}

func cloneCoverageBoundary(value *sdk.CoverageBoundary) *sdk.CoverageBoundary {
	if value == nil {
		return nil
	}
	clone := *value
	clone.Regions = append([]string(nil), value.Regions...)
	clone.ServiceMajors = make(map[string]string, len(value.ServiceMajors))
	for role, major := range value.ServiceMajors {
		clone.ServiceMajors[role] = major
	}
	return &clone
}

func cloneIntMap(values map[string]int) map[string]int {
	if values == nil {
		return nil
	}
	clone := make(map[string]int, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}
