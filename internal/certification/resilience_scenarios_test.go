package certification

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestBuildResilienceScenarioMatrixCoversRuntimeAndDataFailureBoundaries(t *testing.T) {
	intent := scenarioArchitecture("gke")
	intent.Resilience.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoveryAlternateRegion}
	scenarios, err := BuildResilienceScenarioMatrix(intent)
	if err != nil {
		t.Fatalf("BuildResilienceScenarioMatrix() error = %v", err)
	}
	seen := make(map[ResilienceScenarioKind]ResilienceScenario)
	for _, scenario := range scenarios {
		seen[scenario.Kind] = scenario
	}
	for _, kind := range []ResilienceScenarioKind{ScenarioPodLoss, ScenarioNodeLoss, ScenarioZoneLoss, ScenarioRegionLoss, ScenarioAlternateProviderLoss, ScenarioCorruptedData, ScenarioFailedDeployment, ScenarioLostCredentials, ScenarioProviderOutage, ScenarioPartialRestore, ScenarioInterruptedTeardown} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("scenario %q missing from matrix", kind)
		}
	}
	if seen[ScenarioRegionLoss].Mode == ScenarioUnsupported {
		t.Fatalf("alternate-region profile unexpectedly unsupported: %#v", seen[ScenarioRegionLoss])
	}
	if !seen[ScenarioRegionLoss].FencingRequired || !seen[ScenarioRegionLoss].TrafficHealthRequired || !seen[ScenarioRegionLoss].IntegrityRequired {
		t.Fatalf("region-loss scenario lacks safety gates: %#v", seen[ScenarioRegionLoss])
	}
	if seen[ScenarioAlternateProviderLoss].Mode != ScenarioUnsupported || !strings.Contains(seen[ScenarioAlternateProviderLoss].Reason, "alternate-provider") {
		t.Fatalf("alternate-provider scenario must remain explicitly unsupported without a cross-provider adapter: %#v", seen[ScenarioAlternateProviderLoss])
	}
}

func TestBuildResilienceScenarioMatrixClassifiesMissingFailureDomains(t *testing.T) {
	intent := scenarioArchitecture("ecs-fargate")
	intent.Zones = []string{"zone-a"}
	intent.Resilience.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoverySameRegionIsolated}
	scenarios, err := BuildResilienceScenarioMatrix(intent)
	if err != nil {
		t.Fatalf("BuildResilienceScenarioMatrix() error = %v", err)
	}
	for _, scenario := range scenarios {
		if scenario.Kind == ScenarioZoneLoss && (scenario.Mode != ScenarioUnsupported || !strings.Contains(scenario.Reason, "two declared zones")) {
			t.Fatalf("zone-loss scenario = %#v", scenario)
		}
		if scenario.Kind == ScenarioRegionLoss && (scenario.Mode != ScenarioUnsupported || !strings.Contains(scenario.Reason, "alternate-region")) {
			t.Fatalf("region-loss scenario = %#v", scenario)
		}
	}
}

func TestValidateFailureScenarioProofEnforcesDeclaredGates(t *testing.T) {
	scenario := ResilienceScenario{ID: "scenario-zone_loss", Kind: ScenarioZoneLoss, Mode: ScenarioAutomated, TrafficHealthRequired: true, QueueHealthRequired: true, DatabaseHealthRequired: true, CacheLossRequired: true, IntegrityRequired: true, FencingRequired: true}
	proof := FailureScenarioProof{ScenarioID: scenario.ID, Status: StatusPass, InjectionVerified: true, TrafficHealthVerified: true, QueueHealthVerified: true, DatabaseHealthVerified: true, CacheLossClassified: true, IntegrityVerified: true, FencingVerified: true}
	if err := ValidateFailureScenarioProof(scenario, proof); err != nil {
		t.Fatalf("valid failure proof rejected: %v", err)
	}
	proof.IntegrityVerified = false
	if err := ValidateFailureScenarioProof(scenario, proof); err == nil || !strings.Contains(err.Error(), "integrity verification") {
		t.Fatalf("missing integrity proof error = %v", err)
	}
	proof = FailureScenarioProof{ScenarioID: scenario.ID, Status: StatusFail, Reason: "fault observation failed"}
	if err := ValidateFailureScenarioProof(scenario, proof); err == nil || !strings.Contains(err.Error(), "injection verification") {
		t.Fatalf("missing injection proof error = %v", err)
	}
	unsupported := ResilienceScenario{ID: "scenario-alternate_provider_loss", Kind: ScenarioAlternateProviderLoss, Mode: ScenarioUnsupported}
	if err := ValidateFailureScenarioProof(unsupported, FailureScenarioProof{ScenarioID: unsupported.ID, Status: StatusSkip, Reason: "no cross-provider adapter"}); err != nil {
		t.Fatalf("typed unsupported proof rejected: %v", err)
	}
}

func TestBuildResilienceScenarioMatrixMarksGCPRegionLossUnsupportedWithoutAlternateRegion(t *testing.T) {
	intent := scenarioArchitecture("gke")
	intent.Resilience.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated}
	scenarios, err := BuildResilienceScenarioMatrix(intent)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range scenarios {
		if scenario.Kind != ScenarioRegionLoss {
			continue
		}
		if scenario.Mode != ScenarioUnsupported || !strings.Contains(scenario.Reason, "alternate-region") {
			t.Fatalf("GCP region-loss without alternate-region = %#v", scenario)
		}
		return
	}
	t.Fatal("region-loss scenario missing")
}

func TestBuildResilienceScenarioMatrixMarksGCPFencingScenariosUnsupported(t *testing.T) {
	gke, err := BuildResilienceScenarioMatrix(scenarioArchitecture("gke"))
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[ResilienceScenarioKind]ResilienceScenario)
	for _, scenario := range gke {
		seen[scenario.Kind] = scenario
	}
	zone := seen[ScenarioZoneLoss]
	if zone.Mode != ScenarioAutomated || !strings.Contains(zone.Reason, "physical zone outage") || !strings.Contains(zone.Reason, "known-content") {
		t.Fatalf("GCP zone-loss should stay automated with a physical-outage and known-content ceiling: %#v", zone)
	}
	pod := seen[ScenarioPodLoss]
	if pod.Mode != ScenarioAutomated || !strings.Contains(pod.Reason, "known-content") {
		t.Fatalf("GCP pod-loss should stay automated with a Magento known-content ceiling: %#v", pod)
	}
	node := seen[ScenarioNodeLoss]
	if node.Mode != ScenarioOperatorAssisted || !strings.Contains(node.Reason, "known-content") {
		t.Fatalf("GCP node-loss should stay operator-assisted with a Magento known-content ceiling: %#v", node)
	}
	failedDeploy := seen[ScenarioFailedDeployment]
	if failedDeploy.Mode != ScenarioOperatorAssisted || !strings.Contains(failedDeploy.Reason, "no failed-deployment injector") {
		t.Fatalf("GCP failed-deployment should be operator-assisted until an injector exists: %#v", failedDeploy)
	}
	partial := seen[ScenarioPartialRestore]
	if partial.Mode != ScenarioOperatorAssisted || !strings.Contains(partial.Reason, "no partial-restore injector") {
		t.Fatalf("GCP partial-restore should be operator-assisted until a multi-class injector exists: %#v", partial)
	}
	for _, kind := range []ResilienceScenarioKind{ScenarioLostCredentials, ScenarioProviderOutage} {
		scenario := seen[kind]
		if scenario.Mode != ScenarioUnsupported || !strings.Contains(scenario.Reason, "writer fencing") {
			t.Fatalf("GCP %s should be typed unsupported until a fence translator exists: %#v", kind, scenario)
		}
	}
	teardown := seen[ScenarioInterruptedTeardown]
	if teardown.Mode != ScenarioAutomated {
		t.Fatalf("GCP interrupted-teardown should be automated once a Cloud SQL ledger adapter exists: %#v", teardown)
	}

	ecs, err := BuildResilienceScenarioMatrix(scenarioArchitecture("ecs-fargate"))
	if err != nil {
		t.Fatal(err)
	}
	ecsSeen := make(map[ResilienceScenarioKind]ResilienceScenario)
	for _, scenario := range ecs {
		ecsSeen[scenario.Kind] = scenario
	}
	if ecsSeen[ScenarioLostCredentials].Mode != ScenarioOperatorAssisted {
		t.Fatalf("non-GKE lost-credentials overlay leaked: %#v", ecsSeen[ScenarioLostCredentials])
	}
	if ecsSeen[ScenarioFailedDeployment].Mode != ScenarioOperatorAssisted || !strings.Contains(ecsSeen[ScenarioFailedDeployment].Reason, "no failed-deployment injector") {
		t.Fatalf("GCP failed-deployment overlay should apply off GKE: %#v", ecsSeen[ScenarioFailedDeployment])
	}
	if ecsSeen[ScenarioPartialRestore].Mode != ScenarioOperatorAssisted || !strings.Contains(ecsSeen[ScenarioPartialRestore].Reason, "no partial-restore injector") {
		t.Fatalf("GCP partial-restore overlay should apply off GKE: %#v", ecsSeen[ScenarioPartialRestore])
	}

	awsIntent := scenarioArchitecture("ecs-fargate")
	awsIntent.Provider = "aws"
	awsScenarios, err := BuildResilienceScenarioMatrix(awsIntent)
	if err != nil {
		t.Fatal(err)
	}
	awsSeen := make(map[ResilienceScenarioKind]ResilienceScenario)
	for _, scenario := range awsScenarios {
		awsSeen[scenario.Kind] = scenario
	}
	if awsSeen[ScenarioFailedDeployment].Mode != ScenarioAutomated || awsSeen[ScenarioFailedDeployment].Reason != "" {
		t.Fatalf("AWS failed-deployment overlay leaked: %#v", awsSeen[ScenarioFailedDeployment])
	}
	if awsSeen[ScenarioPartialRestore].Mode != ScenarioAutomated || awsSeen[ScenarioPartialRestore].Reason != "" {
		t.Fatalf("AWS partial-restore overlay leaked: %#v", awsSeen[ScenarioPartialRestore])
	}
}

func TestBuildResilienceScenarioMatrixAddsDeclaredStateHealthGates(t *testing.T) {
	intent := scenarioArchitecture("gke")
	intent.Resilience.DataClasses = append(intent.Resilience.DataClasses,
		sdk.DataClassIntent{Name: "queue", SourceOfTruth: "queue", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "restore", IntegrityMethod: "checksum", LossSemantics: "replay", RetentionDays: 30, OwnershipMarker: "magelift/test/scenarios/queue"},
		sdk.DataClassIntent{Name: "cache", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "none", RestoreMethod: "rebuild", IntegrityMethod: "read-check", LossSemantics: "reconstructible", RetentionDays: 1, OwnershipMarker: "magelift/test/scenarios/cache"},
	)
	scenarios, err := BuildResilienceScenarioMatrix(intent)
	if err != nil {
		t.Fatalf("BuildResilienceScenarioMatrix() error = %v", err)
	}
	for _, scenario := range scenarios {
		switch scenario.Kind {
		case ScenarioPodLoss, ScenarioZoneLoss, ScenarioRegionLoss, ScenarioPartialRestore:
			if !scenario.QueueHealthRequired || !scenario.DatabaseHealthRequired || !scenario.CacheLossRequired {
				t.Errorf("scenario %q lacks declared state gates: %#v", scenario.Kind, scenario)
			}
		}
	}
}

func scenarioArchitecture(runtime sdk.RuntimeID) sdk.ArchitectureIntent {
	owner := "magelift/test/scenarios"
	return sdk.ArchitectureIntent{
		ProfileID: "scenario-test", Provider: "gcp", Runtime: runtime, AccountOrProjectRef: "opaque/project", Region: "region-1", Regions: []string{"region-1"}, Zones: []string{"zone-a", "zone-b"},
		ComputeMode: "managed", KubernetesMode: "standard", NetworkMode: "private", IngressMode: "load-balancer",
		Boundaries: []sdk.ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: sdk.ServiceManaged, BackupProfile: "snapshot", RecoveryProfile: "restore"}},
		Edge:       sdk.EdgeIntent{Mode: "none"}, Observability: sdk.ObservabilityIntent{NativeProvider: "none", ExternalProvider: "none", OwnershipMarker: owner},
		Resilience:     sdk.ResilienceIntent{ProfileID: "scenario-test", AvailabilityTarget: "99.9", RPOSeconds: 60, RTOSeconds: 900, RetentionDays: 30, RecoveryScope: "application", FailoverOwner: "operator", FencingPolicy: "single-writer", RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegionIsolated}, DataClasses: []sdk.DataClassIntent{{Name: "database", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "restore", IntegrityMethod: "checksum", LossSemantics: "rpo-bound", RetentionDays: 30, OwnershipMarker: owner + "/database"}}},
		ArtifactDigest: "registry.example.invalid/magelift@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: owner,
	}
}
