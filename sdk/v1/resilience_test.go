package v1

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestResilienceAdapterContractsValidatePortablePlan(t *testing.T) {
	intent := validArchitectureIntentForResilienceTest()
	request := ResiliencePlanRequest{Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker}
	if err := ValidateResiliencePlanRequest(request); err != nil {
		t.Fatalf("ValidateResiliencePlanRequest() error = %v", err)
	}
	plan := ResiliencePlan{
		AdapterID: "aws-resilience", FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker,
		Stages: []ResilienceStage{
			{ID: "backup-database", Action: ResilienceBackup, DataClasses: []string{"database"}, Idempotent: true},
			{ID: "restore-database", Action: ResilienceRestore, DataClasses: []string{"database"}, Destination: RecoveryAlternateRegion, DependsOn: []string{"backup-database"}, Idempotent: true},
			{ID: "verify-database", Action: ResilienceIntegrityCheck, DataClasses: []string{"database"}, DependsOn: []string{"restore-database"}, Idempotent: true},
		},
		RequiredProofs: []string{"backup-id", "restore-id", "integrity-digest"},
	}
	if err := ValidateResiliencePlan(plan, intent.Resilience); err != nil {
		t.Fatalf("ValidateResiliencePlan() error = %v", err)
	}
	if err := ValidateResilienceExecutionRequest(ResilienceExecutionRequest{Plan: plan, StageID: "restore-database", FixtureID: "fixture-1", IdempotencyKey: "run-1/restore-database", OwnershipMarker: intent.OwnershipMarker}); err != nil {
		t.Fatalf("ValidateResilienceExecutionRequest() error = %v", err)
	}
}

func TestResiliencePlanRejectsNonIdempotentCycleAndUnknownClass(t *testing.T) {
	intent := validArchitectureIntentForResilienceTest()
	plan := ResiliencePlan{
		AdapterID: "community-resilience", FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker,
		Stages: []ResilienceStage{
			{ID: "a", Action: ResilienceBackup, DataClasses: []string{"unknown"}, Idempotent: false, DependsOn: []string{"b"}},
			{ID: "b", Action: ResilienceRestore, DataClasses: []string{"database"}, Idempotent: true, DependsOn: []string{"a"}},
		},
		RequiredProofs: []string{"proof"},
	}
	err := ValidateResiliencePlan(plan, intent.Resilience)
	for _, want := range []string{"must be idempotent", "unknown data class", "dependency cycle"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestResilienceAdapterDescriptorAndExecutionValidation(t *testing.T) {
	descriptor := ResilienceAdapterDescriptor{
		APIVersion:   ExtensionAPIVersion,
		ID:           "community-resilience",
		Provider:     "community",
		Version:      "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup, ResilienceRestore},
		DataClasses: []ResilienceDataClassCapability{{
			Name:                "database",
			Status:              ResilienceCapabilityCertified,
			Actions:             []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck},
			Destinations:        []RecoveryDestination{RecoverySameRegionIsolated, RecoveryAlternateRegion},
			PollingRequired:     true,
			RetentionRequired:   true,
			EncryptionRequired:  true,
			ProtectionRequired:  true,
			RetentionPolicy:     "retention-policy",
			EncryptionBoundary:  "encryption-boundary",
			ProtectionMechanism: "protection-mechanism",
		}},
	}
	if err := ValidateResilienceAdapterDescriptor(descriptor); err != nil {
		t.Fatalf("ValidateResilienceAdapterDescriptor() error = %v", err)
	}
	plan := ResiliencePlan{AdapterID: descriptor.ID, FixtureID: "fixture-1", OwnershipMarker: "marker", Stages: []ResilienceStage{{ID: "backup", Action: ResilienceBackup, DataClasses: []string{"database"}, Idempotent: true}}, RequiredProofs: []string{"backup"}}
	if err := ValidateResilienceExecutionRequest(ResilienceExecutionRequest{Plan: plan, StageID: "missing", FixtureID: "fixture-1", IdempotencyKey: "idempotent", OwnershipMarker: "marker"}); err == nil {
		t.Fatal("expected unknown stage error")
	}

	var adapter ResilienceAdapter = testResilienceAdapter{descriptor: descriptor}
	if got := adapter.ResilienceDescriptor().ID; got != descriptor.ID {
		t.Fatalf("ResilienceDescriptor().ID = %q, want %q", got, descriptor.ID)
	}
	if _, err := adapter.PlanResilience(context.Background(), ResiliencePlanRequest{}); err == nil {
		t.Fatal("expected test adapter to reject invalid plan request")
	}
}

func TestValidateResilienceExecutionRequestRequiresReconciliationForFailback(t *testing.T) {
	plan := ResiliencePlan{
		AdapterID: "community-resilience", FixtureID: "fixture-1", OwnershipMarker: "marker",
		Stages: []ResilienceStage{{
			ID: "failback-runtime", Action: ResilienceFailback, DataClasses: []string{"database"},
			Destination: RecoveryAlternateRegion, Idempotent: true, RequiresApproval: true,
			RequiresSingleWriter: true, RequiresSplitBrainCheck: true, RequiresStaleOriginCheck: true,
			RequiresReconciliation: true, ApprovalReference: "approval/1", ReconciliationReference: "reconciliation/1",
		}},
		RequiredProofs: []string{"failback"},
	}
	request := ResilienceExecutionRequest{Plan: plan, StageID: "failback-runtime", FixtureID: "fixture-1", IdempotencyKey: "run/failback", OwnershipMarker: "marker", ApprovalReference: "approval/1"}
	if err := ValidateResilienceExecutionRequest(request); err == nil || !strings.Contains(err.Error(), "reconciliation reference") {
		t.Fatalf("missing reconciliation reference error = %v", err)
	}
	request.ReconciliationReference = "reconciliation/1"
	if err := ValidateResilienceExecutionRequest(request); err != nil {
		t.Fatalf("complete failback request rejected: %v", err)
	}
}

func TestResilienceAdapterDescriptorRejectsAmbiguousDataClassCapability(t *testing.T) {
	base := ResilienceAdapterDescriptor{
		APIVersion:   ExtensionAPIVersion,
		ID:           "community-resilience",
		Provider:     "community",
		Version:      "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup},
		DataClasses: []ResilienceDataClassCapability{{
			Name:            "database",
			Status:          ResilienceCapabilitySupported,
			Actions:         []ResilienceAction{ResilienceBackup},
			PollingRequired: true,
		}},
	}
	tests := []struct {
		name   string
		mutate func(*ResilienceAdapterDescriptor)
		want   string
	}{
		{
			name: "unavailable requires reason",
			mutate: func(descriptor *ResilienceAdapterDescriptor) {
				descriptor.DataClasses[0].Status = ResilienceCapabilityUnavailable
			},
			want: "requires a reason",
		},
		{
			name: "invalid action",
			mutate: func(descriptor *ResilienceAdapterDescriptor) {
				descriptor.DataClasses[0].Actions = []ResilienceAction{"provider-delete-all"}
			},
			want: "invalid resilience adapter data-class action",
		},
		{
			name: "invalid destination",
			mutate: func(descriptor *ResilienceAdapterDescriptor) {
				descriptor.DataClasses[0].Destinations = []RecoveryDestination{"somewhere"}
			},
			want: "invalid resilience adapter data-class destination",
		},
		{
			name: "restore requires destination",
			mutate: func(descriptor *ResilienceAdapterDescriptor) {
				descriptor.DataClasses[0].Actions = []ResilienceAction{ResilienceRestore}
			},
			want: "must declare restore destinations",
		},
		{
			name: "unavailable cannot expose operations",
			mutate: func(descriptor *ResilienceAdapterDescriptor) {
				descriptor.DataClasses[0].Status = ResilienceCapabilityUnavailable
				descriptor.DataClasses[0].Reason = "no native adapter"
				descriptor.DataClasses[0].Actions = []ResilienceAction{ResilienceBackup}
			},
			want: "must not declare executable operations",
		},
		{
			name: "duplicate data class",
			mutate: func(descriptor *ResilienceAdapterDescriptor) {
				descriptor.DataClasses = append(descriptor.DataClasses, descriptor.DataClasses[0])
			},
			want: "duplicate resilience adapter data-class capability",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := base
			descriptor.DataClasses = append([]ResilienceDataClassCapability(nil), base.DataClasses...)
			test.mutate(&descriptor)
			err := ValidateResilienceAdapterDescriptor(descriptor)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateResilienceAdapterDescriptor() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestResilienceAdapterDescriptorRequiresDurableProtectionMetadata(t *testing.T) {
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "community-resilience", Provider: "community", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported, PollingRequired: true,
			Actions: []ResilienceAction{ResilienceBackup},
		}},
	}
	descriptor.DataClasses[0].RetentionRequired = true
	if err := ValidateResilienceAdapterDescriptor(descriptor); err == nil || !strings.Contains(err.Error(), "retention policy") {
		t.Fatalf("missing retention metadata error = %v", err)
	}
	descriptor.DataClasses[0].RetentionPolicy = "retention"
	descriptor.DataClasses[0].EncryptionRequired = true
	if err := ValidateResilienceAdapterDescriptor(descriptor); err == nil || !strings.Contains(err.Error(), "encryption boundary") {
		t.Fatalf("missing encryption metadata error = %v", err)
	}
	descriptor.DataClasses[0].EncryptionBoundary = "encryption"
	descriptor.DataClasses[0].ProtectionRequired = true
	if err := ValidateResilienceAdapterDescriptor(descriptor); err == nil || !strings.Contains(err.Error(), "protection mechanism") {
		t.Fatalf("missing protection metadata error = %v", err)
	}
}

func TestResilienceCapabilityErrorIsTypedAndActionable(t *testing.T) {
	original := ResilienceCapabilityError{
		AdapterID: "scaleway.kapsule",
		Operation: ResilienceRestore,
		DataClass: "search-index",
		Status:    ResilienceCapabilityUnavailable,
		Reason:    "the selected region has no supported restore adapter",
	}
	err := errors.Join(original)
	var got ResilienceCapabilityError
	if !errors.As(err, &got) {
		t.Fatal("errors.As() did not preserve ResilienceCapabilityError")
	}
	if got.AdapterID != original.AdapterID || got.Operation != original.Operation || got.DataClass != original.DataClass {
		t.Fatalf("errors.As() = %#v, want %#v", got, original)
	}
	for _, want := range []string{"unavailable", "adapter=scaleway.kapsule", "operation=restore", "dataClass=search-index", "no supported restore adapter"} {
		if !strings.Contains(original.Error(), want) {
			t.Fatalf("ResilienceCapabilityError.Error() = %q, want substring %q", original.Error(), want)
		}
	}
}

func TestCompileResiliencePlanOwnsProviderNeutralOrdering(t *testing.T) {
	intent := validArchitectureIntentForResilienceTest()
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "aws.resilience", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck, ResilienceFence, ResilienceFailover, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported,
			Actions:         []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck},
			Destinations:    []RecoveryDestination{RecoveryAlternateRegion},
			PollingRequired: true,
		}},
	}
	plan, err := CompileResiliencePlan(descriptor, ResiliencePlanRequest{
		Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID != descriptor.ID || len(plan.Stages) != 6 {
		t.Fatalf("compiled plan = %#v", plan)
	}
	fence, restore, failover := resilienceStage(plan, "fence-runtime"), resilienceStage(plan, "restore-database"), resilienceStage(plan, "failover-runtime")
	if !fence.RequiresApproval || !containsResilienceString(restore.DependsOn, fence.ID) || !containsResilienceString(failover.DependsOn, "integrity-database") {
		t.Fatalf("compiled recovery ordering = %#v", plan.Stages)
	}
	if err := ValidateResiliencePlan(plan, intent.Resilience); err != nil {
		t.Fatal(err)
	}
}

func TestCompileResiliencePlanRequiresExplicitSafeFailback(t *testing.T) {
	intent := validArchitectureIntentForResilienceTest()
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "aws.resilience", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck, ResilienceFence, ResilienceFailover, ResilienceFailback, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported,
			Actions:      []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck},
			Destinations: []RecoveryDestination{RecoveryAlternateRegion}, PollingRequired: true,
		}},
	}

	withoutFailback, err := CompileResiliencePlan(descriptor, ResiliencePlanRequest{
		Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stage := resilienceStage(withoutFailback, "failback-runtime"); stage.ID != "" {
		t.Fatalf("failback was scheduled without an explicit request: %#v", withoutFailback.Stages)
	}

	withFailback, err := CompileResiliencePlan(descriptor, ResiliencePlanRequest{
		Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker,
		ApprovalReferences:       map[string]string{"failback-runtime": "approval/failback-1"},
		ReconciliationReferences: map[string]string{"failback-runtime": "reconciliation/1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	failback := resilienceStage(withFailback, "failback-runtime")
	if failback.Action != ResilienceFailback || !failback.RequiresApproval || !failback.RequiresSingleWriter || !failback.RequiresSplitBrainCheck || !failback.RequiresStaleOriginCheck || !failback.RequiresReconciliation {
		t.Fatalf("failback safety contract = %#v", failback)
	}
	if failback.ApprovalReference != "approval/failback-1" || failback.ReconciliationReference != "reconciliation/1" {
		t.Fatalf("failback admission references = %#v", failback)
	}
	if !containsResilienceString(failback.DependsOn, "failover-runtime") {
		t.Fatalf("failback does not depend on successful failover: %#v", failback.DependsOn)
	}
}

func TestResilienceExecutionAdmissionRejectsIdentityAndReferenceSubstitution(t *testing.T) {
	plan := ResiliencePlan{
		AdapterID: "aws.resilience", FixtureID: "fixture-1", OwnershipMarker: "owner/primary",
		Stages: []ResilienceStage{{
			ID: "failback-runtime", Action: ResilienceFailback, DataClasses: []string{"database"},
			Destination: RecoveryAlternateRegion, Idempotent: true, RequiresApproval: true,
			ApprovalReference: "approval/failback-1", RequiresSingleWriter: true,
			RequiresSplitBrainCheck: true, RequiresStaleOriginCheck: true,
			RequiresReconciliation: true, ReconciliationReference: "reconciliation/1",
		}},
		RequiredProofs: []string{"failback"},
	}
	base := ResilienceExecutionRequest{
		Plan: plan, StageID: "failback-runtime", FixtureID: "fixture-1", IdempotencyKey: "run/failback",
		OwnershipMarker: "owner/primary", ApprovalReference: "approval/failback-1", ReconciliationReference: "reconciliation/1",
	}
	tests := []struct {
		name      string
		mutate    func(*ResilienceExecutionRequest)
		wantField string
	}{
		{name: "fixture substitution", mutate: func(request *ResilienceExecutionRequest) { request.FixtureID = "fixture-2" }, wantField: "fixture"},
		{name: "ownership substitution", mutate: func(request *ResilienceExecutionRequest) { request.OwnershipMarker = "owner/secondary" }, wantField: "ownership"},
		{name: "approval substitution", mutate: func(request *ResilienceExecutionRequest) { request.ApprovalReference = "approval/other" }, wantField: "approval"},
		{name: "reconciliation substitution", mutate: func(request *ResilienceExecutionRequest) { request.ReconciliationReference = "reconciliation/other" }, wantField: "reconciliation"},
		{name: "matching admission", wantField: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			if test.mutate != nil {
				test.mutate(&request)
			}
			err := ValidateResilienceExecutionRequest(request)
			if test.wantField == "" {
				if err != nil {
					t.Fatalf("matching admission rejected: %v", err)
				}
				return
			}
			var admissionErr ResilienceAdmissionError
			if !errors.As(err, &admissionErr) || admissionErr.Field != test.wantField {
				t.Fatalf("admission error = %v, typed = %#v, want field %q", err, admissionErr, test.wantField)
			}
		})
	}
}

func TestCompileResiliencePlanRejectsFailbackWithoutAdapterOptIn(t *testing.T) {
	intent := validArchitectureIntentForResilienceTest()
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "community.resilience", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck, ResilienceFence, ResilienceFailover, ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilitySupported,
			Actions:      []ResilienceAction{ResilienceBackup, ResilienceRestore, ResilienceIntegrityCheck},
			Destinations: []RecoveryDestination{RecoveryAlternateRegion}, PollingRequired: true,
		}},
	}
	_, err := CompileResiliencePlan(descriptor, ResiliencePlanRequest{
		Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker,
		ApprovalReferences:       map[string]string{"failback-runtime": "approval/failback-1"},
		ReconciliationReferences: map[string]string{"failback-runtime": "reconciliation/1"},
	})
	var capabilityErr ResilienceCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Operation != ResilienceFailback || capabilityErr.Status != ResilienceCapabilityUnavailable {
		t.Fatalf("failback opt-in error = %v, typed = %#v", err, capabilityErr)
	}
}

func TestCompileResiliencePlanReturnsTypedUnavailableCapability(t *testing.T) {
	intent := validArchitectureIntentForResilienceTest()
	descriptor := ResilienceAdapterDescriptor{
		APIVersion: ExtensionAPIVersion, ID: "community.resilience", Provider: "aws", Version: "1.0.0",
		Capabilities: []ResilienceAction{ResilienceCleanup},
		DataClasses: []ResilienceDataClassCapability{{
			Name: "database", Status: ResilienceCapabilityUnavailable, Reason: "provider API is not enabled",
		}},
	}
	_, err := CompileResiliencePlan(descriptor, ResiliencePlanRequest{Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker})
	var capabilityErr ResilienceCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Status != ResilienceCapabilityUnavailable || capabilityErr.DataClass != "database" {
		t.Fatalf("unavailable capability error = %v, typed = %#v", err, capabilityErr)
	}
}

func resilienceStage(plan ResiliencePlan, id string) ResilienceStage {
	for _, stage := range plan.Stages {
		if stage.ID == id {
			return stage
		}
	}
	return ResilienceStage{}
}

func containsResilienceString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func validArchitectureIntentForResilienceTest() ArchitectureIntent {
	return ArchitectureIntent{
		ProfileID: "aws.ecs.fargate", Provider: "aws", Runtime: "ecs-fargate", AccountOrProjectRef: "aws://account/123", Region: "eu-west-1", ComputeMode: "fargate", NetworkMode: "private", IngressMode: "load-balancer",
		Boundaries: []ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: ServiceManaged, BackupProfile: "continuous", RecoveryProfile: "cross-region", CapabilityID: "aws.rds.mysql"}},
		Edge:       EdgeIntent{Mode: "none"}, Observability: ObservabilityIntent{}, Resilience: ResilienceIntent{ProfileID: "durable-cross-region", AvailabilityTarget: "multi-zone", RPOSeconds: 60, RTOSeconds: 900, RetentionDays: 30, RecoveryScope: "database-and-media", FailoverOwner: "magelift", FencingPolicy: "lease", RecoveryDestinations: []RecoveryDestination{RecoveryAlternateRegion}, DataClasses: []DataClassIntent{{Name: "database", SourceOfTruth: "rds", BackupMethod: "automated-snapshot", RestoreMethod: "point-in-time", IntegrityMethod: "checksum", LossSemantics: "rpo-bound", RetentionDays: 30, Encrypted: true, Immutable: true, DeletionProtection: true, OwnershipMarker: "marker"}}},
		ArtifactDigest: "sha256:image", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "marker",
	}
}

type testResilienceAdapter struct {
	descriptor ResilienceAdapterDescriptor
}

func (a testResilienceAdapter) ResilienceDescriptor() ResilienceAdapterDescriptor {
	return a.descriptor
}
func (a testResilienceAdapter) PlanResilience(_ context.Context, request ResiliencePlanRequest) (ResiliencePlan, error) {
	if err := ValidateResiliencePlanRequest(request); err != nil {
		return ResiliencePlan{}, err
	}
	return ResiliencePlan{}, nil
}
func (a testResilienceAdapter) ExecuteResilience(_ context.Context, request ResilienceExecutionRequest) (ResilienceExecutionResult, error) {
	if err := ValidateResilienceExecutionRequest(request); err != nil {
		return ResilienceExecutionResult{}, err
	}
	return ResilienceExecutionResult{}, nil
}
