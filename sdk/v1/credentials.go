package v1

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// CredentialLifecycleAction is the provider-neutral secret lifecycle surface
// used by observability and edge extensions. The core validates references and
// ownership scope; the provider adapter owns the vendor API and secret value.
type CredentialLifecycleAction string

const (
	CredentialValidate CredentialLifecycleAction = "validate"
	CredentialRotate   CredentialLifecycleAction = "rotate"
	CredentialRevoke   CredentialLifecycleAction = "revoke"
)

type CredentialLifecycleDescriptor struct {
	APIVersion string                      `json:"apiVersion" yaml:"apiVersion"`
	ID         string                      `json:"id" yaml:"id"`
	Provider   ProviderID                  `json:"provider" yaml:"provider"`
	Version    string                      `json:"version" yaml:"version"`
	Actions    []CredentialLifecycleAction `json:"actions" yaml:"actions"`
}

type CredentialLifecycleRequest struct {
	Action                  CredentialLifecycleAction `json:"action" yaml:"action"`
	Provider                ProviderID                `json:"provider" yaml:"provider"`
	CredentialRef           string                    `json:"credentialRef" yaml:"credentialRef"`
	ManagementCredentialRef string                    `json:"managementCredentialRef,omitempty" yaml:"managementCredentialRef,omitempty"`
	CredentialVersion       string                    `json:"credentialVersion,omitempty" yaml:"credentialVersion,omitempty"`
	OwnershipMarker         string                    `json:"ownershipMarker" yaml:"ownershipMarker"`
	IdempotencyKey          string                    `json:"idempotencyKey" yaml:"idempotencyKey"`
}

// CredentialLifecycleResult contains only provider identities. Secret values
// and access tokens must never cross this SDK boundary.
type CredentialLifecycleResult struct {
	Action            CredentialLifecycleAction `json:"action" yaml:"action"`
	OperationID       string                    `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	CredentialVersion string                    `json:"credentialVersion,omitempty" yaml:"credentialVersion,omitempty"`
	ScopeVerified     bool                      `json:"scopeVerified" yaml:"scopeVerified"`
	RedactionVerified bool                      `json:"redactionVerified" yaml:"redactionVerified"`
}

// CredentialLifecycleAdapter is implemented by a native or community
// provider extension. It is intentionally independent from the portable
// observability schema so new secret stores do not change the core API.
type CredentialLifecycleAdapter interface {
	CredentialLifecycleDescriptor() CredentialLifecycleDescriptor
	ExecuteCredentialLifecycle(context.Context, CredentialLifecycleRequest) (CredentialLifecycleResult, error)
}

// RunCredentialLifecycle is the shared execution gate for provider and
// community secret adapters. It validates the descriptor, provider identity,
// requested action, and redaction/scope proof exactly once before returning a
// result to an observability or edge integration.
func RunCredentialLifecycle(ctx context.Context, adapter CredentialLifecycleAdapter, request CredentialLifecycleRequest) (CredentialLifecycleResult, error) {
	if ctx == nil {
		return CredentialLifecycleResult{}, errors.New("credential lifecycle context is required")
	}
	if adapter == nil {
		return CredentialLifecycleResult{}, errors.New("credential lifecycle adapter is required")
	}
	descriptor := adapter.CredentialLifecycleDescriptor()
	if err := ValidateCredentialLifecycleDescriptor(descriptor); err != nil {
		return CredentialLifecycleResult{}, fmt.Errorf("validate credential lifecycle descriptor: %w", err)
	}
	if request.Provider != descriptor.Provider {
		return CredentialLifecycleResult{}, fmt.Errorf("credential lifecycle provider %q does not match adapter provider %q", request.Provider, descriptor.Provider)
	}
	if !containsCredentialLifecycleAction(descriptor.Actions, request.Action) {
		return CredentialLifecycleResult{}, fmt.Errorf("credential lifecycle adapter %q does not support action %q", descriptor.ID, request.Action)
	}
	if err := ValidateCredentialLifecycleRequest(request); err != nil {
		return CredentialLifecycleResult{}, err
	}
	result, err := adapter.ExecuteCredentialLifecycle(ctx, request)
	if err != nil {
		return CredentialLifecycleResult{}, fmt.Errorf("execute credential lifecycle %q: %w", request.Action, err)
	}
	if err := ValidateCredentialLifecycleResult(request, result); err != nil {
		return CredentialLifecycleResult{}, err
	}
	return result, nil
}

func ValidateCredentialLifecycleDescriptor(descriptor CredentialLifecycleDescriptor) error {
	var problems []error
	if descriptor.APIVersion != ExtensionAPIVersion {
		problems = append(problems, fmt.Errorf("credential lifecycle API version %q is not supported", descriptor.APIVersion))
	}
	problems = append(problems,
		validateID("credential lifecycle adapter ID", descriptor.ID),
		validateID("credential lifecycle provider ID", string(descriptor.Provider)),
		validateExtensionVersion(descriptor.Version),
	)
	if len(descriptor.Actions) == 0 {
		problems = append(problems, errors.New("credential lifecycle adapter must declare at least one action"))
	}
	seen := make(map[CredentialLifecycleAction]struct{}, len(descriptor.Actions))
	for _, action := range descriptor.Actions {
		if !validCredentialLifecycleAction(action) {
			problems = append(problems, fmt.Errorf("invalid credential lifecycle action %q", action))
		}
		if _, exists := seen[action]; exists {
			problems = append(problems, fmt.Errorf("duplicate credential lifecycle action %q", action))
		}
		seen[action] = struct{}{}
	}
	return errors.Join(problems...)
}

func ValidateCredentialLifecycleRequest(request CredentialLifecycleRequest) error {
	var problems []error
	if !validCredentialLifecycleAction(request.Action) {
		problems = append(problems, fmt.Errorf("invalid credential lifecycle action %q", request.Action))
	}
	problems = append(problems,
		validateID("credential lifecycle provider ID", string(request.Provider)),
	)
	if err := ValidateCredentialReference(request.CredentialRef); err != nil {
		problems = append(problems, fmt.Errorf("credential lifecycle reference: %w", err))
	}
	if request.ManagementCredentialRef != "" {
		if err := ValidateCredentialReference(request.ManagementCredentialRef); err != nil {
			problems = append(problems, fmt.Errorf("credential lifecycle management reference: %w", err))
		}
	}
	if strings.TrimSpace(request.CredentialVersion) != "" && strings.ContainsAny(request.CredentialVersion, "\r\n\x00") {
		problems = append(problems, errors.New("credential lifecycle version must be single-line"))
	}
	if err := validateOwnershipMarker("credential lifecycle ownership marker", request.OwnershipMarker); err != nil {
		problems = append(problems, err)
	}
	if strings.TrimSpace(request.IdempotencyKey) == "" || strings.ContainsAny(request.IdempotencyKey, "\r\n") {
		problems = append(problems, errors.New("credential lifecycle idempotency key is required and must not contain line breaks"))
	}
	return errors.Join(problems...)
}

func ValidateCredentialLifecycleResult(request CredentialLifecycleRequest, result CredentialLifecycleResult) error {
	if err := ValidateCredentialLifecycleRequest(request); err != nil {
		return fmt.Errorf("validate credential lifecycle request: %w", err)
	}
	if result.Action != request.Action {
		return fmt.Errorf("credential lifecycle result action %q does not match request %q", result.Action, request.Action)
	}
	if strings.TrimSpace(result.OperationID) == "" && strings.TrimSpace(result.CredentialVersion) == "" {
		return errors.New("credential lifecycle result requires an operation or credential version identity")
	}
	if !result.ScopeVerified {
		return errors.New("credential lifecycle result does not verify ownership scope")
	}
	if !result.RedactionVerified {
		return errors.New("credential lifecycle result does not verify secret redaction")
	}
	return nil
}

func validCredentialLifecycleAction(action CredentialLifecycleAction) bool {
	switch action {
	case CredentialValidate, CredentialRotate, CredentialRevoke:
		return true
	default:
		return false
	}
}

func containsCredentialLifecycleAction(actions []CredentialLifecycleAction, wanted CredentialLifecycleAction) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}
