package certification

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestArchitectureProfileFingerprintIgnoresUnorderedCollections(t *testing.T) {
	intent := sdk.ArchitectureIntent{
		ProfileID: "gcp-gke-ha", Provider: "gcp", Runtime: "gke-standard", AccountOrProjectRef: "project-ref",
		Region: "europe-west1", Zones: []string{"europe-west1-c", "europe-west1-b"}, ComputeMode: "standard",
		NetworkMode: "private", IngressMode: "load-balancer", ArtifactDigest: "image@sha256:" + strings.Repeat("a", 64),
		SchemaFingerprint: strings.Repeat("b", 64), MigrationFingerprint: strings.Repeat("c", 64), OwnershipMarker: "marker",
		Boundaries: []sdk.ServiceBoundaryIntent{{Role: "cache", Family: "valkey", Major: "9", Ownership: sdk.ServiceManaged, BackupProfile: "rebuild", RecoveryProfile: "same-region", CapabilityID: "gcp.memorystore.valkey-9.0"}},
		Resilience: sdk.ResilienceIntent{
			ProfileID: "regional-ha", AvailabilityTarget: "99.9", RPOSeconds: 300, RTOSeconds: 900, RetentionDays: 14,
			RecoveryScope: "runtime", FailoverOwner: "platform", FencingPolicy: "lease",
			RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoveryAlternateRegion, sdk.RecoverySameRegion},
			DataClasses:          []sdk.DataClassIntent{{Name: "cache", SourceOfTruth: "rebuild", FailureDomains: []string{"zone"}, BackupMethod: "rebuild", RestoreMethod: "recreate", IntegrityMethod: "health", LossSemantics: "reconstructible", RetentionDays: 1, OwnershipMarker: "marker"}},
		},
	}
	profile := ArchitectureProfile{Version: ArchitectureProfileVersion, Intent: intent, CapabilityCatalog: CurrentCapabilityCatalog().Version, DeclaredStatus: CapabilityExperimental}
	first, err := profile.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	intent.Zones = []string{"europe-west1-b", "europe-west1-c"}
	intent.Resilience.RecoveryDestinations = []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoveryAlternateRegion}
	profile.Intent = intent
	second, err := profile.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("fingerprint changed with ordering: %s != %s", first, second)
	}
}

func TestArchitectureProfileRejectsUnavailableCapability(t *testing.T) {
	intent := sdk.ArchitectureIntent{
		ProfileID: "scaleway-kapsule", Provider: "scaleway", Runtime: "kapsule", AccountOrProjectRef: "project-ref",
		Region: "fr-par", ComputeMode: "kapsule", NetworkMode: "private", IngressMode: "load-balancer",
		ArtifactDigest: "digest", SchemaFingerprint: "schema", MigrationFingerprint: "migration", OwnershipMarker: "marker",
		Boundaries: []sdk.ServiceBoundaryIntent{{Role: "queue", Family: "queue", Major: "none", Ownership: sdk.ServiceSelfHosted, BackupProfile: "none", RecoveryProfile: "none", CapabilityID: "scaleway.queue"}},
		Resilience: sdk.ResilienceIntent{
			ProfileID: "preview", AvailabilityTarget: "best-effort", RPOSeconds: 3600, RTOSeconds: 3600, RetentionDays: 1,
			RecoveryScope: "runtime", FailoverOwner: "operator", FencingPolicy: "manual",
			RecoveryDestinations: []sdk.RecoveryDestination{sdk.RecoverySameRegion},
			DataClasses:          []sdk.DataClassIntent{{Name: "queue", SourceOfTruth: "database", FailureDomains: []string{"zone"}, BackupMethod: "rebuild", RestoreMethod: "rebuild", IntegrityMethod: "health", LossSemantics: "rebuildable", RetentionDays: 1, OwnershipMarker: "marker"}},
		},
	}
	profile := ArchitectureProfile{Version: ArchitectureProfileVersion, Intent: intent, CapabilityCatalog: CurrentCapabilityCatalog().Version, DeclaredStatus: CapabilityExperimental}
	if err := profile.Validate(CurrentCapabilityCatalog()); err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unavailable capability was accepted: %v", err)
	}
}

func TestProfileAllowsCommunityMaintainedExternalCapability(t *testing.T) {
	record := CapabilityRecord{Provider: "community-edge", ServiceMajor: "current", Role: "edge"}
	boundary := sdk.ServiceBoundaryIntent{Ownership: sdk.ServiceExternal, Role: "edge"}
	if !profileCapabilityProviderAllowed(record, boundary, "aws") {
		t.Fatal("external capability from a community provider was rejected")
	}
	boundary.Ownership = sdk.ServiceManaged
	if profileCapabilityProviderAllowed(record, boundary, "aws") {
		t.Fatal("managed boundary accepted a capability owned by another provider")
	}
}
