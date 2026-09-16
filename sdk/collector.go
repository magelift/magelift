package sdk

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// CollectorDeploymentAction is the lifecycle surface for an injected
// OpenTelemetry or New Relic collector deployment. The deployment is owned by
// the target provider; this contract carries only semantic intent and opaque
// references.
type CollectorDeploymentAction string

const (
	CollectorApply    CollectorDeploymentAction = "apply"
	CollectorVerify   CollectorDeploymentAction = "verify"
	CollectorRollback CollectorDeploymentAction = "rollback"
	CollectorDestroy  CollectorDeploymentAction = "destroy"
)

// CollectorDeploymentPlanRequest is provider-neutral collector intent. A
// provider adapter translates it to an ECS task definition, Helm release, or
// another officially supported workload boundary.
type CollectorDeploymentPlanRequest struct {
	TargetProvider  ProviderID `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime   RuntimeID  `json:"targetRuntime" yaml:"targetRuntime"`
	Workload        string     `json:"workload" yaml:"workload"`
	Distribution    string     `json:"distribution" yaml:"distribution"`
	CredentialRef   string     `json:"credentialRef,omitempty" yaml:"credentialRef,omitempty"`
	Endpoint        string     `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	NativeReference string     `json:"nativeReference,omitempty" yaml:"nativeReference,omitempty"`
	OwnershipMarker string     `json:"ownershipMarker" yaml:"ownershipMarker"`
	Signals         []string   `json:"signals" yaml:"signals"`
}

// CollectorDeploymentPlan is safe to persist in a plan or checkpoint. Opaque
// is adapter-owned runtime state and is never serialized by the SDK.
type CollectorDeploymentPlan struct {
	AdapterID       string     `json:"adapterId" yaml:"adapterId"`
	TargetProvider  ProviderID `json:"targetProvider" yaml:"targetProvider"`
	TargetRuntime   RuntimeID  `json:"targetRuntime" yaml:"targetRuntime"`
	Workload        string     `json:"workload" yaml:"workload"`
	Distribution    string     `json:"distribution" yaml:"distribution"`
	CredentialRef   string     `json:"credentialRef,omitempty" yaml:"credentialRef,omitempty"`
	Endpoint        string     `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	NativeReference string     `json:"nativeReference,omitempty" yaml:"nativeReference,omitempty"`
	OwnershipMarker string     `json:"ownershipMarker" yaml:"ownershipMarker"`
	Signals         []string   `json:"signals" yaml:"signals"`
	Opaque          any        `json:"-" yaml:"-"`
}

// CollectorDeploymentExecutionRequest is the normalized mutation request.
// ResourceReferences are provider-owned identities returned by a previous
// operation; callers must not synthesize them from names.
type CollectorDeploymentExecutionRequest struct {
	Plan               CollectorDeploymentPlan   `json:"plan" yaml:"plan"`
	Action             CollectorDeploymentAction `json:"action" yaml:"action"`
	IdempotencyKey     string                    `json:"idempotencyKey" yaml:"idempotencyKey"`
	OwnershipMarker    string                    `json:"ownershipMarker" yaml:"ownershipMarker"`
	ApprovalReference  string                    `json:"approvalReference,omitempty" yaml:"approvalReference,omitempty"`
	ResourceReferences []string                  `json:"resourceReferences,omitempty" yaml:"resourceReferences,omitempty"`
}

// CollectorDeploymentExecutionResult contains only provider-safe identities
// and proof flags. It never contains an API key, rendered secret, Helm values,
// task environment, or private certificate.
type CollectorDeploymentExecutionResult struct {
	Action              CollectorDeploymentAction `json:"action" yaml:"action"`
	OperationID         string                    `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	ResourceRefs        []string                  `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	ProofRefs           []string                  `json:"proofRefs,omitempty" yaml:"proofRefs,omitempty"`
	OwnershipMarker     string                    `json:"ownershipMarker" yaml:"ownershipMarker"`
	OwnershipVerified   bool                      `json:"ownershipVerified" yaml:"ownershipVerified"`
	IdempotencyVerified bool                      `json:"idempotencyVerified" yaml:"idempotencyVerified"`
	ReadyVerified       bool                      `json:"readyVerified" yaml:"readyVerified"`
	HealthVerified      bool                      `json:"healthVerified" yaml:"healthVerified"`
	CleanupVerified     bool                      `json:"cleanupVerified" yaml:"cleanupVerified"`
	RollbackVerified    bool                      `json:"rollbackVerified" yaml:"rollbackVerified"`
}

// CollectorCapabilityError is returned before mutation when a target cannot
// safely host the selected collector path.
type CollectorCapabilityError struct {
	AdapterID string `json:"adapterId" yaml:"adapterId"`
	Action    string `json:"action" yaml:"action"`
	Reason    string `json:"reason" yaml:"reason"`
}

func (e CollectorCapabilityError) Error() string {
	if e.AdapterID == "" && e.Action == "" && e.Reason == "" {
		return "collector deployment capability is unavailable"
	}
	return fmt.Sprintf("collector deployment capability is unavailable for adapter %q and action %q: %s", e.AdapterID, e.Action, e.Reason)
}

// CollectorDeploymentAdapter is optional. It is separate from the signal
// exporter because a collector creates workload resources with a different
// ownership, readiness, rollback, and cleanup lifecycle.
type CollectorDeploymentAdapter interface {
	PlanCollector(context.Context, CollectorDeploymentPlanRequest) (CollectorDeploymentPlan, error)
	ExecuteCollector(context.Context, CollectorDeploymentExecutionRequest) (CollectorDeploymentExecutionResult, error)
}

func ValidateCollectorDeploymentPlanRequest(request CollectorDeploymentPlanRequest) error {
	var problems []error
	problems = append(problems,
		validateID("collector target provider", string(request.TargetProvider)),
		validateID("collector target runtime", string(request.TargetRuntime)),
		validateID("collector workload", request.Workload),
	)
	if strings.TrimSpace(request.Distribution) == "" || strings.ContainsAny(request.Distribution, "\r\n\x00") || strings.IndexFunc(request.Distribution, func(r rune) bool { return r == ' ' || r == '\t' }) >= 0 {
		problems = append(problems, errors.New("collector distribution is required and must be single-line without whitespace"))
	}
	if strings.TrimSpace(request.OwnershipMarker) == "" || strings.ContainsAny(request.OwnershipMarker, "\r\n\x00") {
		problems = append(problems, errors.New("collector ownership marker is required and must be single-line"))
	}
	if request.CredentialRef != "" {
		if err := ValidateCredentialReference(request.CredentialRef); err != nil {
			problems = append(problems, fmt.Errorf("collector credential reference: %w", err))
		}
	}
	if request.Endpoint != "" {
		if err := validateCollectorEndpoint(request.Endpoint); err != nil {
			problems = append(problems, err)
		}
	}
	if strings.TrimSpace(request.NativeReference) == "" || strings.ContainsAny(request.NativeReference, "\r\n\x00") {
		problems = append(problems, errors.New("collector native reference is required and must be single-line"))
	}
	problems = append(problems, validateCollectorSignals(request.Signals))
	return errors.Join(problems...)
}

func ValidateCollectorDeploymentPlan(plan CollectorDeploymentPlan, request CollectorDeploymentPlanRequest) error {
	var problems []error
	if strings.TrimSpace(plan.AdapterID) == "" {
		problems = append(problems, errors.New("collector plan adapter ID is required"))
	}
	if plan.TargetProvider != request.TargetProvider || plan.TargetRuntime != request.TargetRuntime || plan.Workload != request.Workload || plan.Distribution != request.Distribution {
		problems = append(problems, errors.New("collector plan target or distribution does not match the request"))
	}
	if plan.CredentialRef != request.CredentialRef || plan.Endpoint != request.Endpoint || plan.NativeReference != request.NativeReference || plan.OwnershipMarker != request.OwnershipMarker {
		problems = append(problems, errors.New("collector plan references do not match the request"))
	}
	if !sameStrings(plan.Signals, request.Signals) {
		problems = append(problems, errors.New("collector plan must preserve requested signals"))
	}
	return errors.Join(problems...)
}

func ValidateCollectorDeploymentExecutionRequest(request CollectorDeploymentExecutionRequest) error {
	if request.Action != CollectorApply && request.Action != CollectorVerify && request.Action != CollectorRollback && request.Action != CollectorDestroy {
		return fmt.Errorf("invalid collector deployment action %q", request.Action)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.ContainsAny(request.IdempotencyKey, "\r\n\x00") {
		return errors.New("collector deployment idempotency key is required and must be single-line")
	}
	if request.OwnershipMarker != request.Plan.OwnershipMarker {
		return errors.New("collector deployment ownership marker does not match the plan")
	}
	if strings.ContainsAny(request.ApprovalReference, "\r\n\x00") {
		return errors.New("collector deployment approval reference must be single-line")
	}
	seen := make(map[string]struct{}, len(request.ResourceReferences))
	for _, reference := range request.ResourceReferences {
		if strings.TrimSpace(reference) == "" || strings.ContainsAny(reference, "\r\n\x00") {
			return errors.New("collector deployment resource references must be non-empty and single-line")
		}
		if _, exists := seen[reference]; exists {
			return fmt.Errorf("duplicate collector deployment resource reference %q", reference)
		}
		seen[reference] = struct{}{}
	}
	return nil
}

func validateCollectorEndpoint(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return errors.New("collector endpoint must be an HTTP(S) URL without user information or fragment")
	}
	return nil
}

func validateCollectorSignals(signals []string) error {
	if len(signals) == 0 {
		return errors.New("collector must declare at least one signal")
	}
	seen := make(map[string]struct{}, len(signals))
	for _, signal := range signals {
		if err := validateID("collector signal", signal); err != nil {
			return err
		}
		if _, exists := seen[signal]; exists {
			return fmt.Errorf("duplicate collector signal %q", signal)
		}
		seen[signal] = struct{}{}
	}
	return nil
}
