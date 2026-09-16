package sdk

import (
	"strings"
	"testing"
)

func TestCoverageBoundaryFingerprintIsOrderIndependentAndTransitionsAreExplicit(t *testing.T) {
	base := testCoverageBoundary()
	reordered := base
	reordered.Regions = []string{"eu-west-1b", "eu-west-1a"}
	reordered.ServiceMajors = map[string]string{"cache": "9", "database": "8.4"}
	first, err := base.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	second, err := reordered.Fingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("fingerprint changed with unordered inputs: %s != %s", first, second)
	}

	serviceUpgrade := base
	serviceUpgrade.ServiceMajors = map[string]string{"database": "8.4", "cache": "10"}
	if err := ValidateCoverageTransition(base, serviceUpgrade, CoverageTransitionServiceMajor); err != nil {
		t.Fatalf("service-major transition rejected: %v", err)
	}

	edgeChange := base
	edgeChange.EdgePath = "native+external"
	if err := ValidateCoverageTransition(base, edgeChange, CoverageTransitionEdge); err != nil {
		t.Fatalf("edge transition rejected: %v", err)
	}
	if err := ValidateCoverageTransition(base, base, CoverageTransitionNone); err != nil {
		t.Fatalf("identical boundary should be a valid no-op: %v", err)
	}
	restoreChange := base
	restoreChange.RecoveryDestination = "same-region-isolated"
	if err := ValidateCoverageTransition(base, restoreChange, CoverageTransitionRestore); err != nil {
		t.Fatalf("restore transition rejected: %v", err)
	}
	artifactChange := base
	artifactChange.ArtifactDigest = "registry.example.invalid/app@sha256:" + strings.Repeat("b", 64)
	if err := ValidateCoverageTransition(base, artifactChange, CoverageTransitionServiceMajor); err == nil || !strings.Contains(err.Error(), "cold boundary") {
		t.Fatalf("artifact change was accepted as a warm transition: %v", err)
	}
	networkChange := base
	networkChange.NetworkProfile = "private-aws-fck-nat-multi-az-auto-scaling"
	if err := ValidateCoverageTransition(base, networkChange, CoverageTransitionNone); err == nil || !strings.Contains(err.Error(), "cold boundary") {
		t.Fatalf("network profile change was accepted as a warm transition: %v", err)
	}
	configurationChange := base
	configurationChange.ConfigurationFingerprint = "sha256:" + strings.Repeat("a", 64)
	if err := ValidateCoverageTransition(base, configurationChange, CoverageTransitionNone); err == nil || !strings.Contains(err.Error(), "cold boundary") {
		t.Fatalf("configuration fingerprint change was accepted as a warm transition: %v", err)
	}
}

func TestCoverageBoundaryFromArchitectureRequiresFixtureAndCapturesDestinations(t *testing.T) {
	intent := testCoverageArchitectureIntent()
	boundary, err := CoverageBoundaryFromArchitecture(intent, "fixture-v1")
	if err != nil {
		t.Fatal(err)
	}
	if boundary.ObservabilityDestination != "cloudwatch+newrelic" || boundary.EdgePath != "both" || boundary.ManagedSelfHostedBoundary != "managed" || boundary.NetworkProfile != "private-aws-fck-nat-multi-az-auto-scaling" || boundary.ConfigurationFingerprint == "none" {
		t.Fatalf("boundary = %#v", boundary)
	}
	if _, err := CoverageBoundaryFromArchitecture(intent, ""); err == nil || !strings.Contains(err.Error(), "recovery fixture") {
		t.Fatalf("missing recovery fixture was accepted: %v", err)
	}
}

func TestImmutableArtifactContractRequiresProvenanceAndSignature(t *testing.T) {
	contract := ImmutableArtifactContract{
		ImageDigest:         "registry.example.invalid/app@sha256:" + strings.Repeat("a", 64),
		ManifestDigest:      "sha256:" + strings.Repeat("b", 64),
		InputFingerprint:    strings.Repeat("c", 64),
		ProvenanceReference: "https://example.invalid/provenance/commit-a",
		SignatureReference:  "oci://example.invalid/app:sha256-" + strings.Repeat("a", 64),
	}
	first, err := contract.ReuseKey()
	if err != nil {
		t.Fatal(err)
	}
	contract.SignatureReference = ""
	if _, err := contract.ReuseKey(); err == nil {
		t.Fatal("artifact without signature was accepted")
	}
	if first == "" {
		t.Fatal("artifact reuse key is empty")
	}
}

func testCoverageBoundary() CoverageBoundary {
	return CoverageBoundary{
		Provider:                  "aws",
		Regions:                   []string{"eu-west-1a", "eu-west-1b"},
		ComputeMode:               "fargate",
		KubernetesTopology:        "none",
		NetworkProfile:            "private",
		ManagedSelfHostedBoundary: "managed",
		ServiceMajors:             map[string]string{"database": "8.4", "cache": "9"},
		ResilienceProfileID:       "aws.ecs",
		ObservabilityDestination:  "cloudwatch",
		EdgePath:                  "none",
		ArtifactDigest:            "registry.example.invalid/app@sha256:" + strings.Repeat("a", 64),
		SchemaFingerprint:         strings.Repeat("d", 64),
		MigrationFingerprint:      strings.Repeat("e", 64),
		RecoveryFixtureID:         "fixture-v1",
		RecoveryDestination:       "none",
		OwnershipMarker:           "magelift/test/coverage",
		ConfigurationFingerprint:  "sha256:" + strings.Repeat("f", 64),
	}
}

func testCoverageArchitectureIntent() ArchitectureIntent {
	return ArchitectureIntent{
		ProfileID:           "aws.ecs",
		Provider:            "aws",
		Runtime:             "ecs-fargate",
		AccountOrProjectRef: "account-1",
		Region:              "eu-west-1",
		Regions:             []string{"eu-west-1"},
		Zones:               []string{"eu-west-1a", "eu-west-1b"},
		ComputeMode:         "fargate",
		NetworkMode:         "private",
		NetworkProfile:      "private-aws-fck-nat-multi-az-auto-scaling",
		IngressMode:         "load-balancer",
		Boundaries: []ServiceBoundaryIntent{{
			Role: "database", Family: "mysql", Major: "8.4", Ownership: ServiceManaged,
			BackupProfile: "provider", RecoveryProfile: "same-region",
		}},
		Edge:          EdgeIntent{Mode: "both", NativeProvider: "cloudfront", ExternalProvider: "fastly", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental, CredentialRefs: []string{"aws-secrets-manager://magelift/test/fastly"}, OriginHealthRef: "health", OwnershipMarker: "magelift/test/coverage"},
		Observability: ObservabilityIntent{NativeProvider: "cloudwatch", ExternalProvider: "newrelic", Lifecycle: ExternalLifecycleExtension, Certification: ExternalExperimental, CredentialRefs: []string{"aws-secrets-manager://magelift/test/newrelic"}, OwnershipMarker: "magelift/test/coverage", Signals: []string{"logs"}},
		Resilience: ResilienceIntent{
			ProfileID: "aws.ecs", AvailabilityTarget: "99.9", RPOSeconds: 300, RTOSeconds: 600, RetentionDays: 30,
			RecoveryScope: "application", FailoverOwner: "operator", FencingPolicy: "single-writer",
			RecoveryDestinations: []RecoveryDestination{RecoverySameRegion},
			DataClasses:          []DataClassIntent{{Name: "database", SourceOfTruth: "rds", FailureDomains: []string{"zone"}, BackupMethod: "snapshot", RestoreMethod: "restore", IntegrityMethod: "checksum", LossSemantics: "bounded", RetentionDays: 30, Encrypted: true, Immutable: true, DeletionProtection: true, OwnershipMarker: "magelift/test/coverage/database"}},
		},
		ArtifactDigest:           "registry.example.invalid/app@sha256:" + strings.Repeat("a", 64),
		ConfigurationFingerprint: "sha256:" + strings.Repeat("f", 64),
		SchemaFingerprint:        strings.Repeat("d", 64),
		MigrationFingerprint:     strings.Repeat("e", 64),
		OwnershipMarker:          "magelift/test/coverage",
	}
}
