package v1

import (
	"context"
	"strings"
	"testing"
)

func TestCertificationAdapterContractsValidateSafeUnitAndResult(t *testing.T) {
	unit := CertificationExecutionUnit{
		ID:              "unit-1",
		CellIDs:         []string{"cell-1"},
		BaselineCellID:  "cell-1",
		Fingerprint:     "fingerprint-1",
		WarmBoundary:    "provider/aws",
		OwnershipMarker: "magelift/test/unit-1",
		Cold:            true,
		MutationKeys:    []string{"provider/aws"},
	}
	if err := ValidateCertificationExecutionUnit(unit); err != nil {
		t.Fatal(err)
	}
	request := CertificationExecutionRequest{Unit: unit, IdempotencyKey: "unit-1:fingerprint-1"}
	if err := ValidateCertificationExecutionRequest(request); err != nil {
		t.Fatal(err)
	}
	result := CertificationExecutionResult{
		StackID:         "stack-1",
		OperationIDs:    []string{"operation-1"},
		ResourceRefs:    []string{"resource-1"},
		OwnershipMarker: unit.OwnershipMarker,
		ReuseMode:       "cold",
		CleanupState:    "complete",
	}
	if err := ValidateCertificationExecutionResult(result, unit); err != nil {
		t.Fatal(err)
	}
}

func TestCertificationAdapterContractsRejectScopeAndReferenceDrift(t *testing.T) {
	unit := CertificationExecutionUnit{
		ID: "unit-1", CellIDs: []string{"cell-1"}, BaselineCellID: "cell-1", Fingerprint: "fingerprint-1",
		WarmBoundary: "provider/aws", OwnershipMarker: "magelift/test/unit-1", Cold: true,
	}
	result := CertificationExecutionResult{OwnershipMarker: "magelift/other", ReuseMode: "cold"}
	if err := ValidateCertificationExecutionResult(result, unit); err == nil || !strings.Contains(err.Error(), "ownership marker") {
		t.Fatalf("ownership drift error = %v", err)
	}
	unit.CellIDs = []string{"cell-1", "cell-1"}
	if err := ValidateCertificationExecutionUnit(unit); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate cell error = %v", err)
	}

	descriptor := CertificationAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "aws.ecs-certification", Provider: "aws", Runtime: "ecs-fargate", Version: "1.0.0",
		Actions: []CertificationAction{CertificationExecute},
	}
	if err := ValidateCertificationAdapterDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
	descriptor.Actions = []CertificationAction{"unknown"}
	if err := ValidateCertificationAdapterDescriptor(descriptor); err == nil || !strings.Contains(err.Error(), "invalid certification adapter action") {
		t.Fatalf("invalid action error = %v", err)
	}
}

func TestCertificationAdmissionContractsValidateOpaqueRequestAndFacts(t *testing.T) {
	request := CertificationAdmissionRequest{
		Provider:            "aws",
		AccountOrProjectRef: "account-1",
		Region:              "eu-west-3",
		NetworkRef:          "aws-vpc://vpc-1",
		StateBackendRef:     "s3://magelift-state/runs",
		FixtureID:           "fixture-1",
		OwnershipMarker:     "magelift/test/admission-1",
		CredentialRefs:      []string{"aws-secrets://acceptance/provider"},
		RequireCredentials:  true,
		RequiredQuota:       map[string]int{"vpc": 1},
		MutationKeys:        []string{"aws/eu-west-3"},
	}
	if err := ValidateCertificationAdmissionRequest(request); err != nil {
		t.Fatal(err)
	}
	result := CertificationAdmissionResult{
		CredentialsReady: true, AccountOrProjectReady: true, NetworkReady: true,
		StateBackendReady: true, FixtureReady: true, OwnershipReady: true,
		AvailableQuota: map[string]int{"vpc": 2}, Reasons: map[string]string{"provider.quota.vpc": "available"},
		AvailableLocalCPUMilli: 1000, AvailableLocalMemoryMB: 1024,
	}
	if err := ValidateCertificationAdmissionResult(result); err != nil {
		t.Fatal(err)
	}
	result.AvailableLocalCPUMilli = -1
	if err := ValidateCertificationAdmissionResult(result); err == nil || !strings.Contains(err.Error(), "local CPU") {
		t.Fatalf("negative local CPU result error = %v", err)
	}
	result.AvailableLocalCPUMilli = 1000
	request.MutationKeys = []string{"aws/eu-west-3", "aws/eu-west-3"}
	if err := ValidateCertificationAdmissionRequest(request); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate mutation key error = %v", err)
	}

	descriptor := CertificationAdmissionAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "aws.ecs-admission", Provider: "aws", Runtime: "ecs-fargate", Version: "1.0.0",
	}
	if err := ValidateCertificationAdmissionAdapterDescriptor(descriptor); err != nil {
		t.Fatal(err)
	}
}

type certificationAdapterContractStub struct{}

func (certificationAdapterContractStub) CertificationDescriptor() CertificationAdapterDescriptor {
	return CertificationAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "vendor.certification", Provider: "vendor", Runtime: "example", Version: "1.0.0",
		Actions: []CertificationAction{CertificationExecute},
	}
}

func (certificationAdapterContractStub) ExecuteCertification(_ context.Context, request CertificationExecutionRequest) (CertificationExecutionResult, error) {
	return CertificationExecutionResult{OwnershipMarker: request.Unit.OwnershipMarker, ReuseMode: "cold"}, nil
}

var _ CertificationAdapter = certificationAdapterContractStub{}
