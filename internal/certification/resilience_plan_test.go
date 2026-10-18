package certification

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestBuildResiliencePlanSeparatesDurableAndRebuildableState(t *testing.T) {
	intent := testResilienceArchitectureIntent([]sdk.DataClassIntent{
		{Name: "database", SourceOfTruth: "managed", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "durable", RetentionDays: 14, OwnershipMarker: "marker"},
		{Name: "search-index", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "rebuild", RestoreMethod: "rebuild", IntegrityMethod: "query", LossSemantics: "rebuildable", RetentionDays: 1, OwnershipMarker: "marker"},
		{Name: "cache", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "recreate", RestoreMethod: "recreate", IntegrityMethod: "health", LossSemantics: "reconstructible", RetentionDays: 1, OwnershipMarker: "marker"},
	})
	plan, err := BuildResiliencePlan(intent)
	if err != nil {
		t.Fatal(err)
	}
	if err := sdk.ValidateResiliencePlan(plan, intent.Resilience); err != nil {
		t.Fatal(err)
	}
	if !hasStage(plan, "backup-database", sdk.ResilienceBackup) || hasStage(plan, "backup-search-index", sdk.ResilienceBackup) || hasStage(plan, "backup-cache", sdk.ResilienceBackup) {
		t.Fatalf("durable/rebuild stages = %#v", plan.Stages)
	}
	if !containsString(plan.RequiredProofs, "backup:database") || !containsString(plan.RequiredProofs, "restore:search-index") || !containsString(plan.RequiredProofs, "cleanup") {
		t.Fatalf("required proofs = %#v", plan.RequiredProofs)
	}
}

func TestBuildResiliencePlanAddsFencingBeforeRegionalFailover(t *testing.T) {
	intent := testResilienceArchitectureIntent([]sdk.DataClassIntent{{
		Name: "database", SourceOfTruth: "managed", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "durable", RetentionDays: 14, OwnershipMarker: "marker",
	}})
	intent.Resilience.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoveryAlternateRegion}
	plan, err := BuildResiliencePlan(intent)
	if err != nil {
		t.Fatal(err)
	}
	fence, failover := stageByID(plan, "fence-runtime"), stageByID(plan, "failover-runtime")
	restore := stageByID(plan, "restore-database")
	integrity := stageByID(plan, "integrity-database")
	if fence.ID == "" || failover.ID == "" || restore.ID == "" || integrity.ID == "" || !containsString(restore.DependsOn, fence.ID) || !containsString(failover.DependsOn, integrity.ID) || !fence.RequiresApproval || !failover.RequiresApproval {
		t.Fatalf("failover graph = %#v", plan.Stages)
	}
}

func TestBuildResiliencePlanRequiresApprovalForInPlaceRestore(t *testing.T) {
	intent := testResilienceArchitectureIntent([]sdk.DataClassIntent{{
		Name: "database", SourceOfTruth: "managed", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "in-place", IntegrityMethod: "checksum", LossSemantics: "durable", RetentionDays: 14, OwnershipMarker: "marker",
	}})
	plan, err := BuildResiliencePlan(intent)
	if err != nil {
		t.Fatal(err)
	}
	fence, restore := stageByID(plan, "fence-runtime"), stageByID(plan, "restore-database")
	if !fence.RequiresApproval || !restore.RequiresApproval || !containsString(restore.DependsOn, fence.ID) {
		t.Fatalf("in-place recovery stages = %#v", plan.Stages)
	}
}

func TestBuildResiliencePlanCanUseIsolatedSameRegionWithoutFencing(t *testing.T) {
	intent := testResilienceArchitectureIntent([]sdk.DataClassIntent{{
		Name: "database", SourceOfTruth: "managed", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "durable", RetentionDays: 14, OwnershipMarker: "marker",
	}})
	intent.Resilience.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoverySameRegionIsolated}
	plan, err := BuildResiliencePlan(intent)
	if err != nil {
		t.Fatal(err)
	}
	if stageByID(plan, "fence-runtime").ID != "" || stageByID(plan, "failover-runtime").ID != "" {
		t.Fatalf("isolated recovery unexpectedly fences or fails over: %#v", plan.Stages)
	}
	restore := stageByID(plan, "restore-database")
	if restore.Destination != sdk.RecoverySameRegionIsolated || restore.RequiresApproval {
		t.Fatalf("isolated restore = %#v", restore)
	}
}

func TestBuildResiliencePlanRejectsRebuildForDurableData(t *testing.T) {
	intent := testResilienceArchitectureIntent([]sdk.DataClassIntent{{
		Name: "database", SourceOfTruth: "managed", FailureDomains: []string{"zone"}, BackupMethod: "rebuild", RestoreMethod: "rebuild", IntegrityMethod: "checksum", LossSemantics: "rebuildable", RetentionDays: 14, OwnershipMarker: "marker",
	}})
	if _, err := BuildResiliencePlan(intent); err == nil || !strings.Contains(err.Error(), "durable data class") {
		t.Fatalf("durable rebuild was accepted: %v", err)
	}
}

func TestGCPResilienceProfileDocumentsFencingTypedGap(t *testing.T) {
	profile, found := FindResilienceProfile("gcp.gke")
	if !found {
		t.Fatal("gcp.gke resilience profile missing")
	}
	if !strings.Contains(profile.Reason, "typed unsupported") || !strings.Contains(profile.Reason, "regional DR") || !strings.Contains(profile.Reason, "Physical zone outage") || !strings.Contains(profile.Reason, "Failed-deployment") || !strings.Contains(profile.Reason, "Partial-restore") || !strings.Contains(profile.Reason, "known-content HA integrity") {
		t.Fatalf("gcp.gke reason does not document the fencing/DR/zone/failed-deploy/partial-restore/HA-integrity typed gap: %q", profile.Reason)
	}
	for _, destination := range profile.AllowedDestinations {
		if destination == sdk.RecoveryAlternateRegion {
			t.Fatal("gcp.gke must not advertise alternate-region while fencing/failover are typed unsupported")
		}
	}
	for _, dataClass := range profile.DataClasses {
		for _, destination := range dataClass.Destinations {
			if destination == string(sdk.RecoveryAlternateRegion) {
				t.Fatalf("gcp.gke data class %q still advertises alternate-region", dataClass.Name)
			}
		}
	}
}

func TestGCPResilienceProfileRejectsAlternateRegionBeforeMutation(t *testing.T) {
	profile, found := FindResilienceProfile("gcp.gke")
	if !found {
		t.Fatal("gcp.gke resilience profile missing")
	}
	intent := sdk.ResilienceIntent{
		ProfileID: "test", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
		RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual",
		RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoveryAlternateRegion},
		DataClasses: []sdk.DataClassIntent{{
			Name: "database", SourceOfTruth: "cloud-sql", FailureDomains: []string{"zone"}, BackupMethod: "on-demand", RestoreMethod: "restore",
			IntegrityMethod: "checksum", LossSemantics: "recover", RetentionDays: 1, OwnershipMarker: "magelift/test/dr",
		}},
	}
	err := profile.ValidateIntent(intent)
	if err == nil || !strings.Contains(err.Error(), "not allowed by profile") {
		t.Fatalf("alternate-region error = %v", err)
	}
	intent.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoverySameRegionIsolated}
	if err := profile.ValidateIntent(intent); err != nil {
		t.Fatalf("isolated same-region rejected: %v", err)
	}
}

func testResilienceArchitectureIntent(dataClasses []sdk.DataClassIntent) sdk.ArchitectureIntent {
	return sdk.ArchitectureIntent{
		ProfileID: "aws-ecs-preview", Provider: "aws", Runtime: "ecs-fargate", AccountOrProjectRef: "account", Region: "eu-north-1", ComputeMode: "fargate", NetworkMode: "private", IngressMode: "load-balancer",
		ArtifactDigest: "image@sha256:" + strings.Repeat("a", 64), SchemaFingerprint: strings.Repeat("b", 64), MigrationFingerprint: strings.Repeat("c", 64), OwnershipMarker: "marker",
		Boundaries: []sdk.ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: sdk.ServiceManaged, BackupProfile: "snapshot", RecoveryProfile: "restore"}},
		Edge:       sdk.EdgeIntent{Mode: "none"}, Observability: sdk.ObservabilityIntent{},
		Resilience: sdk.ResilienceIntent{ProfileID: "preview", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 14, RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual", RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion}, DataClasses: dataClasses},
	}
}

func hasStage(plan sdk.ResiliencePlan, id string, action sdk.ResilienceAction) bool {
	stage := stageByID(plan, id)
	return stage.ID == id && stage.Action == action
}

func stageByID(plan sdk.ResiliencePlan, id string) sdk.ResilienceStage {
	for _, stage := range plan.Stages {
		if stage.ID == id {
			return stage
		}
	}
	return sdk.ResilienceStage{}
}
