package platform

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/certification"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type certificationModule struct {
	fakeModule
	adapter   sdk.CertificationAdapter
	admission sdk.CertificationAdmissionAdapter
}

func (m certificationModule) Certification() sdk.CertificationAdapter { return m.adapter }
func (m certificationModule) CertificationAdmission() sdk.CertificationAdmissionAdapter {
	return m.admission
}

type certificationAdapterStub struct {
	descriptor     sdk.CertificationAdapterDescriptor
	request        sdk.CertificationExecutionRequest
	wrongOwnership bool
	executeErr     error
}

type certificationAdmissionStub struct {
	descriptor sdk.CertificationAdmissionAdapterDescriptor
	request    sdk.CertificationAdmissionRequest
	result     sdk.CertificationAdmissionResult
}

func (a *certificationAdmissionStub) CertificationAdmissionDescriptor() sdk.CertificationAdmissionAdapterDescriptor {
	return a.descriptor
}

func (a *certificationAdmissionStub) AdmitCertification(_ context.Context, request sdk.CertificationAdmissionRequest) (sdk.CertificationAdmissionResult, error) {
	a.request = request
	return a.result, nil
}

func (a *certificationAdapterStub) CertificationDescriptor() sdk.CertificationAdapterDescriptor {
	return a.descriptor
}

func (a *certificationAdapterStub) ExecuteCertification(_ context.Context, request sdk.CertificationExecutionRequest) (sdk.CertificationExecutionResult, error) {
	a.request = request
	ownership := request.Unit.OwnershipMarker
	if a.wrongOwnership {
		ownership = "magelift/other"
	}
	result := sdk.CertificationExecutionResult{
		StackID:         "stack-1",
		OperationIDs:    []string{"operation-1"},
		ResourceRefs:    []string{"resource-1"},
		OwnershipMarker: ownership,
		ReuseMode:       "cold",
		CleanupState:    "complete",
	}
	return result, a.executeErr
}

func TestCertificationAdapterIsRegisteredAndBridgedToCoreScheduler(t *testing.T) {
	adapter := &certificationAdapterStub{descriptor: sdk.CertificationAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion,
		ID:         "aws.ecs-certification",
		Provider:   "aws",
		Runtime:    "ecs-fargate",
		Version:    "1.0.0",
		Actions:    []sdk.CertificationAction{sdk.CertificationExecute},
	}}
	module := certificationModule{fakeModule: fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierExperimental,
		keys: RequiredOutputKeys(),
	}, adapter: adapter}
	registry := NewModuleRegistry()
	if err := registry.RegisterModule(module); err != nil {
		t.Fatal(err)
	}
	planned := fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"}
	executor, err := CertificationExecutorFor(context.Background(), &module, planned)
	if err != nil {
		t.Fatal(err)
	}
	if executor == nil {
		t.Fatal("certification executor is nil")
	}
	unit := certification.ExecutionUnit{
		ID: "unit-1", CellIDs: []string{"cell-1"}, BaselineCellID: "cell-1", Fingerprint: "fingerprint-1",
		WarmBoundary: "provider/aws", OwnershipMarker: "magelift/test/unit-1", Cold: true,
		MutationKeys: []string{"provider/aws"},
	}
	result, err := executor(context.Background(), unit)
	if err != nil {
		t.Fatal(err)
	}
	if result.StackID != "stack-1" || result.OwnershipMarker != unit.OwnershipMarker || result.ReuseMode != "cold" || result.CleanupState != "complete" {
		t.Fatalf("scheduler result = %#v", result)
	}
	if adapter.request.Unit.ID != unit.ID || adapter.request.Unit.Fingerprint != unit.Fingerprint || adapter.request.IdempotencyKey != unit.ID+":"+unit.Fingerprint {
		t.Fatalf("provider request = %#v", adapter.request)
	}
}

func TestCertificationAdapterRegistrationRejectsTargetMismatch(t *testing.T) {
	module := certificationModule{fakeModule: fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierExperimental,
		keys: RequiredOutputKeys(),
	}, adapter: &certificationAdapterStub{descriptor: sdk.CertificationAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.gke-certification", Provider: "gcp", Runtime: "gke", Version: "1.0.0",
		Actions: []sdk.CertificationAction{sdk.CertificationExecute},
	}}}
	err := NewModuleRegistry().RegisterModule(module)
	if err == nil || !strings.Contains(err.Error(), "certification adapter target") {
		t.Fatalf("target mismatch error = %v", err)
	}
}

func TestCertificationExecutorRejectsProviderScopeReturnedByAdapter(t *testing.T) {
	adapter := &certificationAdapterStub{descriptor: sdk.CertificationAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "aws.ecs-certification", Provider: "aws", Runtime: "ecs-fargate", Version: "1.0.0",
		Actions: []sdk.CertificationAction{sdk.CertificationExecute},
	}, wrongOwnership: true}
	module := certificationModule{fakeModule: fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierExperimental, keys: RequiredOutputKeys(),
	}, adapter: adapter}
	executor, err := CertificationExecutorFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"})
	if err != nil {
		t.Fatal(err)
	}
	unit := certification.ExecutionUnit{
		ID: "unit-1", CellIDs: []string{"cell-1"}, BaselineCellID: "cell-1", Fingerprint: "fingerprint-1",
		WarmBoundary: "provider/aws", OwnershipMarker: "magelift/test/unit-1", Cold: true,
	}
	if _, err := executor(context.Background(), unit); err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("provider ownership mismatch error = %v", err)
	}
}

func TestCertificationExecutorPreservesSafePartialResultOnProviderError(t *testing.T) {
	adapter := &certificationAdapterStub{descriptor: sdk.CertificationAdapterDescriptor{
		APIVersion: sdk.ExtensionAPIVersion, ID: "aws.ecs-certification", Provider: "aws", Runtime: "ecs-fargate", Version: "1.0.0",
		Actions: []sdk.CertificationAction{sdk.CertificationExecute},
	}, executeErr: errors.New("provider timeout")}
	module := certificationModule{fakeModule: fakeModule{
		id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierExperimental, keys: RequiredOutputKeys(),
	}, adapter: adapter}
	executor, err := CertificationExecutorFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"})
	if err != nil {
		t.Fatal(err)
	}
	unit := certification.ExecutionUnit{
		ID: "unit-1", CellIDs: []string{"cell-1"}, BaselineCellID: "cell-1", Fingerprint: "fingerprint-1",
		WarmBoundary: "provider/aws", OwnershipMarker: "magelift/test/unit-1", Cold: true,
	}
	result, err := executor(context.Background(), unit)
	if err == nil || !strings.Contains(err.Error(), "provider timeout") {
		t.Fatalf("provider error = %v", err)
	}
	if result.StackID != "stack-1" || result.OperationIDs[0] != "operation-1" || result.OwnershipMarker != unit.OwnershipMarker {
		t.Fatalf("partial result = %#v", result)
	}
}

func TestCertificationAdmissionIsRegisteredAndBridgedToCore(t *testing.T) {
	adapter := &certificationAdmissionStub{
		descriptor: sdk.CertificationAdmissionAdapterDescriptor{
			APIVersion: sdk.ExtensionAPIVersion, ID: "aws.ecs-admission", Provider: "aws", Runtime: "ecs-fargate", Version: "1.0.0",
		},
		result: sdk.CertificationAdmissionResult{
			CredentialsReady: true, AccountOrProjectReady: true, NetworkReady: true,
			StateBackendReady: true, FixtureReady: true, OwnershipReady: true,
			AvailableQuota: map[string]int{"vpc": 2},
		},
	}
	module := certificationModule{
		fakeModule: fakeModule{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierExperimental, keys: RequiredOutputKeys()},
		admission:  adapter,
	}
	if err := NewModuleRegistry().RegisterModule(module); err != nil {
		t.Fatal(err)
	}
	probe, err := CertificationAdmissionFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"})
	if err != nil {
		t.Fatal(err)
	}
	if probe == nil {
		t.Fatal("certification admission probe is nil")
	}
	input := certification.AdmissionInput{
		Provider: "aws", AccountOrProjectRef: "account-1", Region: "eu-west-3", NetworkRef: "aws-vpc://vpc-1",
		StateBackendRef: "s3://state/runs", FixtureID: "fixture-1", OwnershipMarker: "magelift/test/admission-1",
		CredentialRefs: []string{"aws-secrets://acceptance/provider"}, RequireCredentials: true,
		RequiredQuota: map[string]int{"vpc": 1}, MutationKeys: []string{"aws/eu-west-3"},
	}
	result, err := probe.Probe(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CredentialsReady || !result.AccountOrProjectReady || result.AvailableQuota["vpc"] != 2 {
		t.Fatalf("admission result = %#v", result)
	}
	if adapter.request.Provider != "aws" || adapter.request.StateBackendRef != input.StateBackendRef || adapter.request.CredentialRefs[0] != input.CredentialRefs[0] {
		t.Fatalf("provider admission request = %#v", adapter.request)
	}
}

func TestCertificationAdmissionRejectsProviderScopeAndSecretLikeFacts(t *testing.T) {
	wrong := &certificationAdmissionStub{
		descriptor: sdk.CertificationAdmissionAdapterDescriptor{
			APIVersion: sdk.ExtensionAPIVersion, ID: "gcp.gke-admission", Provider: "gcp", Runtime: "gke", Version: "1.0.0",
		},
	}
	module := certificationModule{
		fakeModule: fakeModule{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierExperimental, keys: RequiredOutputKeys()},
		admission:  wrong,
	}
	if _, err := CertificationAdmissionFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"}); err == nil || !strings.Contains(err.Error(), "provider") {
		t.Fatalf("provider mismatch error = %v", err)
	}

	secret := &certificationAdmissionStub{
		descriptor: sdk.CertificationAdmissionAdapterDescriptor{
			APIVersion: sdk.ExtensionAPIVersion, ID: "aws.ecs-admission", Provider: "aws", Runtime: "ecs-fargate", Version: "1.0.0",
		},
		result: sdk.CertificationAdmissionResult{Reasons: map[string]string{"provider": "token: plaintext-secret-value"}},
	}
	module.admission = secret
	probe, err := CertificationAdmissionFor(context.Background(), &module, fakePlannedStack{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate"})
	if err != nil {
		t.Fatal(err)
	}
	input := certification.AdmissionInput{
		Provider: "aws", AccountOrProjectRef: "account-1", Region: "eu-west-3", NetworkRef: "aws-vpc://vpc-1",
		StateBackendRef: "s3://state/runs", FixtureID: "fixture-1", OwnershipMarker: "magelift/test/admission-1", MutationKeys: []string{"aws/eu-west-3"},
	}
	if _, err := probe.Probe(context.Background(), input); err == nil || !strings.Contains(err.Error(), "secret-like") {
		t.Fatalf("secret-like result error = %v", err)
	}
}
