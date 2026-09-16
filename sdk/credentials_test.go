package sdk

import (
	"context"
	"strings"
	"testing"
)

type credentialLifecycleStub struct{}

func (credentialLifecycleStub) CredentialLifecycleDescriptor() CredentialLifecycleDescriptor {
	return CredentialLifecycleDescriptor{APIVersion: ExtensionAPIVersion, ID: "test.credentials", Provider: "newrelic", Version: "1.0.0", Actions: []CredentialLifecycleAction{CredentialValidate, CredentialRotate, CredentialRevoke}}
}

func (credentialLifecycleStub) ExecuteCredentialLifecycle(_ context.Context, request CredentialLifecycleRequest) (CredentialLifecycleResult, error) {
	return CredentialLifecycleResult{Action: request.Action, OperationID: "operation-1", ScopeVerified: true, RedactionVerified: true}, nil
}

func TestRunCredentialLifecycleCentralizesProviderAndRedactionGates(t *testing.T) {
	request := CredentialLifecycleRequest{Action: CredentialRotate, Provider: "newrelic", CredentialRef: "secret://observability/newrelic", OwnershipMarker: "magelift/test", IdempotencyKey: "rotate-1"}
	result, err := RunCredentialLifecycle(context.Background(), credentialLifecycleStub{}, request)
	if err != nil {
		t.Fatalf("RunCredentialLifecycle() error = %v", err)
	}
	if result.Action != CredentialRotate || result.OperationID == "" {
		t.Fatalf("result = %#v", result)
	}
	request.Provider = "aws"
	if _, err := RunCredentialLifecycle(context.Background(), credentialLifecycleStub{}, request); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("provider mismatch error = %v", err)
	}
}

func TestCredentialLifecycleContractRequiresScopeAndRedaction(t *testing.T) {
	descriptor := CredentialLifecycleDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "community.secret-store", Provider: "community", Version: "1.0.0",
		Actions: []CredentialLifecycleAction{CredentialValidate, CredentialRotate, CredentialRevoke},
	}
	if err := ValidateCredentialLifecycleDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	request := CredentialLifecycleRequest{
		Action: CredentialRotate, Provider: "community", CredentialRef: "secret://observability/newrelic",
		OwnershipMarker: "magelift/architecture/test", IdempotencyKey: strings.Repeat("a", 32),
	}
	if err := ValidateCredentialLifecycleRequest(request); err != nil {
		t.Fatal(err)
	}
	if err := ValidateCredentialLifecycleResult(request, CredentialLifecycleResult{
		Action: CredentialRotate, OperationID: "operation-1", ScopeVerified: true, RedactionVerified: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCredentialLifecycleContractRejectsPlaintextAndUnscopedResults(t *testing.T) {
	request := CredentialLifecycleRequest{
		Action: CredentialRotate, Provider: "community", CredentialRef: "plaintext-token",
		OwnershipMarker: "magelift/architecture/test", IdempotencyKey: "operation-1",
	}
	if err := ValidateCredentialLifecycleRequest(request); err == nil || !strings.Contains(err.Error(), "URI-shaped") {
		t.Fatalf("plaintext credential was accepted: %v", err)
	}
	request.CredentialRef = "secret://observability/newrelic"
	if err := ValidateCredentialLifecycleResult(request, CredentialLifecycleResult{Action: CredentialRotate, OperationID: "operation-1", RedactionVerified: true}); err == nil || !strings.Contains(err.Error(), "ownership scope") {
		t.Fatalf("unscoped credential result was accepted: %v", err)
	}
}
