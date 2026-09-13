package resilience

import (
	"errors"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestCompileResiliencePlanIsolatedSameRegionDoesNotRequireFence(t *testing.T) {
	plan, err := sdk.CompileResiliencePlan(Descriptor(), gcpDatabasePlanRequest(sdk.RecoverySameRegionIsolated))
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range plan.Stages {
		if stage.Action == sdk.ResilienceFence || stage.Action == sdk.ResilienceFailover || stage.Action == sdk.ResilienceFailback {
			t.Fatalf("isolated same-region plan scheduled %s: %#v", stage.Action, plan.Stages)
		}
	}
	if restore := gcpStage(plan, "restore-database"); restore.Destination != sdk.RecoverySameRegionIsolated || restore.RequiresApproval {
		t.Fatalf("isolated restore = %#v", restore)
	}
}

func TestCompileResiliencePlanSameRegionIsTypedFenceGap(t *testing.T) {
	_, err := sdk.CompileResiliencePlan(Descriptor(), gcpDatabasePlanRequest(sdk.RecoverySameRegion))
	assertTypedRuntimeGap(t, err, sdk.ResilienceFence)
}

func TestCompileResiliencePlanAlternateRegionIsTypedFenceGap(t *testing.T) {
	_, err := sdk.CompileResiliencePlan(Descriptor(), gcpDatabasePlanRequest(sdk.RecoveryAlternateRegion))
	assertTypedRuntimeGap(t, err, sdk.ResilienceFence)
}

func TestCompileResiliencePlanFailbackIsTypedGap(t *testing.T) {
	request := gcpDatabasePlanRequest(sdk.RecoverySameRegionIsolated)
	request.ApprovalReferences = map[string]string{"failback-runtime": "approval/failback-1"}
	request.ReconciliationReferences = map[string]string{"failback-runtime": "reconciliation/1"}
	_, err := sdk.CompileResiliencePlan(Descriptor(), request)
	assertTypedRuntimeGap(t, err, sdk.ResilienceFailback)
}

func assertTypedRuntimeGap(t *testing.T, err error, operation sdk.ResilienceAction) {
	t.Helper()
	var capability sdk.ResilienceCapabilityError
	if !errors.As(err, &capability) || capability.Operation != operation || capability.AdapterID != AdapterID {
		t.Fatalf("error = %v, typed = %#v, want operation %s", err, capability, operation)
	}
	if capability.Status != sdk.ResilienceCapabilityUnavailable && capability.Status != sdk.ResilienceCapabilityUnsupported {
		t.Fatalf("status = %q, want unavailable or unsupported", capability.Status)
	}
}

func gcpStage(plan sdk.ResiliencePlan, id string) sdk.ResilienceStage {
	for _, stage := range plan.Stages {
		if stage.ID == id {
			return stage
		}
	}
	return sdk.ResilienceStage{}
}

func gcpDatabasePlanRequest(destination sdk.RecoveryDestination) sdk.ResiliencePlanRequest {
	intent := sdk.ArchitectureIntent{
		ProfileID: "gcp.gke.autopilot", Provider: "gcp", Runtime: "gke-autopilot", AccountOrProjectRef: "gcp://project/demo", Region: "europe-west1",
		ComputeMode: "autopilot", NetworkMode: "private", IngressMode: "load-balancer",
		Boundaries: []sdk.ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: sdk.ServiceManaged, BackupProfile: "on-demand", RecoveryProfile: "isolated", CapabilityID: "gcp.cloud-sql.mysql"}},
		Edge:       sdk.EdgeIntent{Mode: "none"}, Observability: sdk.ObservabilityIntent{},
		Resilience: sdk.ResilienceIntent{
			ProfileID: "gcp.gke", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 7,
			RecoveryScope: "database", FailoverOwner: "operator", FencingPolicy: "manual",
			RecoveryDestinations: []sdk.RecoveryDestination{destination},
			DataClasses: []sdk.DataClassIntent{{
				Name: "database", SourceOfTruth: "cloud-sql", FailureDomains: []string{"zone"}, BackupMethod: "on-demand-backup", RestoreMethod: "restore-backup",
				IntegrityMethod: "checksum", LossSemantics: "rpo-bound", RetentionDays: 7, Encrypted: true, OwnershipMarker: "marker",
			}},
		},
		ArtifactDigest: "sha256:image", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "marker",
	}
	return sdk.ResiliencePlanRequest{Architecture: intent, FixtureID: "fixture-1", OwnershipMarker: intent.OwnershipMarker}
}

func TestDescriptorDoesNotAdvertiseFenceFailoverOrFailback(t *testing.T) {
	for _, action := range Descriptor().Capabilities {
		switch action {
		case sdk.ResilienceFence, sdk.ResilienceFailover, sdk.ResilienceFailback:
			t.Fatalf("GCP descriptor advertised %q without a native translator", action)
		}
	}
}
