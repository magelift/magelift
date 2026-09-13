package certification

import (
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestCurrentArchitectureFamiliesCoverRequestedProviderAxes(t *testing.T) {
	families := CurrentArchitectureFamilies()
	if len(families) != 5 {
		t.Fatalf("architecture families = %d, want AWS ECS/EKS, GKE, Kapsule, and MKS", len(families))
	}
	wantModes := map[string][]string{
		"aws.ecs":          {"ec2-asg", "fargate", "fargate-spot", "managed-instances"},
		"aws.eks":          {"auto-mode", "fargate", "managed-node-groups", "self-managed"},
		"gcp.gke":          {"autopilot", "standard"},
		"ovh.mks":          {"managed-node-pools", "self-managed-workloads"},
		"scaleway.kapsule": {"managed-node-pools", "self-managed-workloads"},
	}
	for _, family := range families {
		if err := family.Validate(CurrentCapabilityCatalog()); err != nil {
			t.Fatalf("validate %s: %v", family.ID, err)
		}
		for _, mode := range wantModes[family.ID] {
			if !containsString(family.ComputeModes, mode) {
				t.Errorf("family %s omitted compute mode %s", family.ID, mode)
			}
		}
	}
}

func TestArchitectureFamilyProfilesValidateAcrossEveryFirstPartyFamily(t *testing.T) {
	catalog := CurrentCapabilityCatalog()
	digest := "image@sha256:" + strings.Repeat("a", 64)
	for _, family := range CurrentArchitectureFamilies() {
		resilienceProfile, found := FindResilienceProfile(family.ID)
		if !found {
			t.Fatalf("resilience profile %s missing", family.ID)
		}
		dataClasses := make([]sdk.DataClassIntent, 0, len(resilienceProfile.DataClasses))
		for _, capability := range resilienceProfile.DataClasses {
			if capability.Status == CapabilityUnavailable || capability.Status == CapabilityUnsupported || capability.Status == CapabilityBlocked {
				continue
			}
			// Queue recovery destinations differ by provider: AWS SQS is
			// isolated-only while GCP Pub/Sub seeks the existing
			// subscription. This test exercises the common durable profile;
			// the provider-specific queue ceiling is tested separately.
			if capability.Name == "queue" {
				continue
			}
			dataClasses = append(dataClasses, sdk.DataClassIntent{
				Name: capability.Name, SourceOfTruth: capability.Name, FailureDomains: []string{"zone"},
				BackupMethod: capability.Strategy, RestoreMethod: capability.Strategy, IntegrityMethod: "fixture-check",
				LossSemantics: capability.Strategy, RetentionDays: 1, OwnershipMarker: "magelift/test/" + family.ID,
			})
		}
		input := ArchitectureProfileInput{
			AccountOrProjectRef: "account/" + family.Provider, Region: "region-1", Zones: []string{"zone-a", "zone-b"},
			ComputeMode: family.ComputeModes[0], NetworkMode: family.NetworkModes[0], IngressMode: family.IngressModes[0],
			ArtifactDigest: digest, SchemaFingerprint: strings.Repeat("b", 64), MigrationFingerprint: strings.Repeat("c", 64),
			OwnershipMarker: "magelift/test/" + family.ID, Edge: sdk.EdgeIntent{Mode: "none"},
			Resilience: sdk.ResilienceIntent{
				ProfileID: "test", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
				RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual",
				RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion}, DataClasses: dataClasses,
			},
		}
		if len(family.KubernetesTopologies) > 0 {
			input.KubernetesTopology = family.KubernetesTopologies[0]
		}
		profile, err := family.Profile(input, catalog)
		if err != nil {
			t.Fatalf("profile %s: %v", family.ID, err)
		}
		if _, err := profile.Fingerprint(); err != nil {
			t.Fatalf("fingerprint %s: %v", family.ID, err)
		}
	}
}

func TestResilienceProfileRejectsUnsupportedQueueDestinationsBeforeMutation(t *testing.T) {
	tests := []struct {
		name            string
		profileID       string
		accepted        sdk.RecoveryDestination
		rejected        []sdk.RecoveryDestination
		wantUnsupported string
	}{
		{name: "aws.ecs", profileID: "aws.ecs", accepted: sdk.RecoverySameRegionIsolated, rejected: []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoveryAlternateRegion}, wantUnsupported: "queue"},
		{name: "gcp.gke same-region", profileID: "gcp.gke", accepted: sdk.RecoverySameRegion, rejected: []sdk.RecoveryDestination{sdk.RecoveryAlternateRegion}, wantUnsupported: "queue"},
		{name: "gcp.gke isolated", profileID: "gcp.gke", accepted: sdk.RecoverySameRegionIsolated, rejected: []sdk.RecoveryDestination{sdk.RecoveryAlternateRegion}, wantUnsupported: "queue"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			profile, found := FindResilienceProfile(test.profileID)
			if !found {
				t.Fatalf("resilience profile %s not found", test.profileID)
			}
			capability, found := resilienceDataClass(profile.DataClasses, test.wantUnsupported)
			if !found {
				t.Fatalf("resilience data class %s not found in %s", test.wantUnsupported, test.profileID)
			}
			intent := sdk.ResilienceIntent{
				ProfileID: "test", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
				RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual",
				DataClasses: []sdk.DataClassIntent{{
					Name: capability.Name, SourceOfTruth: "provider", FailureDomains: []string{"zone"}, BackupMethod: "provider", RestoreMethod: "provider",
					IntegrityMethod: "fixture-check", LossSemantics: "recover", RetentionDays: 1, OwnershipMarker: "magelift/test/queue",
				}},
			}

			intent.RecoveryDestinations = []sdk.RecoveryDestination{test.accepted}
			if err := profile.ValidateIntent(intent); err != nil {
				t.Fatalf("accepted destination %s rejected: %v", test.accepted, err)
			}
			for _, destination := range test.rejected {
				intent.RecoveryDestinations = []sdk.RecoveryDestination{destination}
				err := profile.ValidateIntent(intent)
				if err == nil || !strings.Contains(err.Error(), "does not support recovery destination") {
					t.Fatalf("destination %s error = %v", destination, err)
				}
			}
		})
	}
}

func TestArchitectureFamilyRejectsUnavailableProviderDataClasses(t *testing.T) {
	for _, familyID := range []string{"scaleway.kapsule", "ovh.mks"} {
		profile, found := FindResilienceProfile(familyID)
		if !found {
			t.Fatalf("resilience profile %s missing", familyID)
		}
		for _, dataClass := range []string{"queue", "search-index"} {
			capability, found := resilienceDataClass(profile.DataClasses, dataClass)
			if !found || capability.Status != CapabilityUnavailable {
				t.Fatalf("%s data class %s = %#v, want unavailable", familyID, dataClass, capability)
			}
			intent := sdk.ResilienceIntent{
				ProfileID: "test", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
				RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual",
				RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion},
				DataClasses: []sdk.DataClassIntent{{
					Name: dataClass, SourceOfTruth: "fixture", FailureDomains: []string{"zone"}, BackupMethod: "fixture", RestoreMethod: "fixture", IntegrityMethod: "checksum", LossSemantics: "recover", RetentionDays: 1, OwnershipMarker: "magelift/test/unsupported",
				}},
			}
			if err := profile.ValidateIntent(intent); err == nil || !strings.Contains(err.Error(), "is unavailable") {
				t.Fatalf("%s data class %s was accepted: %v", familyID, dataClass, err)
			}
		}
	}
}

func resilienceDataClass(classes []ResilienceDataClassCapability, name string) (ResilienceDataClassCapability, bool) {
	for _, class := range classes {
		if class.Name == name {
			return class, true
		}
	}
	return ResilienceDataClassCapability{}, false
}

func TestArchitectureFamilyProfileIsProviderNeutralAndValidatedBeforeMutation(t *testing.T) {
	family := CurrentArchitectureFamilies()[0]
	input := ArchitectureProfileInput{
		AccountOrProjectRef: "account-ref", Region: "eu-west-3", Zones: []string{"eu-west-3a", "eu-west-3b"},
		ComputeMode: "fargate", NetworkMode: "private", IngressMode: "load-balancer",
		ArtifactDigest: "image@sha256:" + strings.Repeat("a", 64), SchemaFingerprint: "schema-v1", MigrationFingerprint: "migration-v1", OwnershipMarker: "magelift/test/run-1",
		Boundaries: []sdk.ServiceBoundaryIntent{{Role: "database", Family: "mysql", Major: "8.4", Ownership: sdk.ServiceManaged, BackupProfile: "provider", RecoveryProfile: "same-region", CapabilityID: "aws.rds.mysql"}},
		Resilience: sdk.ResilienceIntent{
			ProfileID: "preview", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
			RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual", RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion},
			DataClasses: []sdk.DataClassIntent{{Name: "database", SourceOfTruth: "managed-database", FailureDomains: []string{"zone"}, BackupMethod: "provider", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "recover", RetentionDays: 1, OwnershipMarker: "magelift/test/run-1"}},
		},
	}
	profile, err := family.Profile(input, CurrentCapabilityCatalog())
	if err != nil {
		t.Fatal(err)
	}
	if profile.Intent.Provider != "aws" || profile.Intent.ComputeMode != "fargate" || profile.Intent.Boundaries[0].CapabilityID != "aws.rds.mysql" {
		t.Fatalf("profile = %#v", profile.Intent)
	}
	if _, err := profile.Fingerprint(); err != nil {
		t.Fatal(err)
	}
	input.ComputeMode = "does-not-exist"
	if _, err := family.Profile(input, CurrentCapabilityCatalog()); err == nil || !strings.Contains(err.Error(), "does not support compute mode") {
		t.Fatalf("unsupported mode was accepted: %v", err)
	}
	input.ComputeMode = "fargate"
	input.Resilience.RPOSeconds = 7200
	if _, err := family.Profile(input, CurrentCapabilityCatalog()); err == nil || !strings.Contains(err.Error(), "exceeds profile ceiling") {
		t.Fatalf("resilience policy violation was accepted: %v", err)
	}
}

func TestArchitectureFamilyRejectsNativeIntegrationOutsideFamily(t *testing.T) {
	var family ArchitectureFamily
	for _, candidate := range CurrentArchitectureFamilies() {
		if candidate.ID == "gcp.gke" {
			family = candidate
			break
		}
	}
	if family.ID == "" {
		t.Fatal("GKE family missing")
	}
	input := ArchitectureProfileInput{
		AccountOrProjectRef: "project", Region: "europe-west1", ComputeMode: "autopilot", NetworkMode: "private", IngressMode: "load-balancer",
		ArtifactDigest: "image@sha256:" + strings.Repeat("a", 64), SchemaFingerprint: strings.Repeat("b", 64), MigrationFingerprint: strings.Repeat("c", 64), OwnershipMarker: "magelift/test/run-2",
		Edge: sdk.EdgeIntent{Mode: "none"}, Observability: sdk.ObservabilityIntent{NativeProvider: "cloudwatch", OwnershipMarker: "magelift/architecture/test", Signals: []string{"logs"}},
		Resilience: sdk.ResilienceIntent{
			ProfileID: "preview", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
			RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual", RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion},
			DataClasses: []sdk.DataClassIntent{{Name: "database", SourceOfTruth: "managed-database", FailureDomains: []string{"zone"}, BackupMethod: "provider", RestoreMethod: "isolated", IntegrityMethod: "checksum", LossSemantics: "recover", RetentionDays: 1, OwnershipMarker: "magelift/test/run-2"}},
		},
	}
	if _, err := family.Profile(input, CurrentCapabilityCatalog()); err == nil || !strings.Contains(err.Error(), "native observability provider") {
		t.Fatalf("unsupported native observability provider was accepted: %v", err)
	}
}

func TestArchitectureCoverageIncludesSourcesAndReasons(t *testing.T) {
	report, err := ArchitectureCoverage(CurrentCapabilityCatalog(), catalogTime(CurrentCapabilityCatalog()))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != len(CurrentArchitectureFamilies()) {
		t.Fatalf("coverage rows = %d, want %d", len(report.Rows), len(CurrentArchitectureFamilies()))
	}
	for _, row := range report.Rows {
		if row.Status == CapabilityCertified {
			continue
		}
		if len(row.Sources) == 0 || len(row.Reasons) == 0 {
			t.Fatalf("non-certified row lacks provenance: %#v", row)
		}
	}
	if report.Counts[CapabilityExperimental] == 0 {
		t.Fatalf("coverage counts = %#v", report.Counts)
	}
}
