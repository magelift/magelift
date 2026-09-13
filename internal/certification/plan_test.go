package certification

import (
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestPlanForTargetMakesWarmReuseAndDeferredCellsExplicit(t *testing.T) {
	plan, err := PlanForTarget("aws/ecs-fargate", "2.4.9", "open-source", "preview")
	if err != nil {
		t.Fatal(err)
	}
	if plan.CellCount == 0 || len(plan.WarmGroups) == 0 {
		t.Fatalf("plan did not expand cells: %#v", plan)
	}
	if plan.StatusCounts[CapabilityCompatible] == 0 {
		t.Fatalf("plan has no compatible cells: %#v", plan.StatusCounts)
	}
	if plan.EstimatedCost.Status != "not-calculated" || !strings.Contains(plan.EstimatedCost.Note, "read-only") {
		t.Fatalf("cost disclosure = %#v", plan.EstimatedCost)
	}
	if len(plan.CredentialRequirements) != 1 || !strings.Contains(plan.CredentialRequirements[0], "aws") {
		t.Fatalf("credentials = %#v", plan.CredentialRequirements)
	}
	for _, group := range plan.WarmGroups {
		if group.BaselineCellID == "" || len(group.CellIDs) == 0 {
			t.Fatalf("invalid warm group = %#v", group)
		}
	}
}

func TestPlanForCommerceRequiresComposerCredentialDisclosure(t *testing.T) {
	plan, err := PlanForTarget("gcp/gke-standard", "2.4.9", "commerce", "preview")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, requirement := range plan.CredentialRequirements {
		if strings.Contains(requirement, "Composer credentials") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Commerce Composer credential requirement missing: %#v", plan.CredentialRequirements)
	}
}

func TestPlanForTargetWithBoundaryCarriesCompleteReuseIdentity(t *testing.T) {
	compatibilityFingerprint := strings.Repeat("a", 64)
	boundary := ReuseBoundary{
		CompatibilityFingerprint: compatibilityFingerprint,
		Artifact: sdk.ImmutableArtifactContract{
			ImageDigest:         "ghcr.io/magelift/test@sha256:" + strings.Repeat("b", 64),
			ManifestDigest:      "sha256:" + strings.Repeat("c", 64),
			InputFingerprint:    compatibilityFingerprint,
			ProvenanceReference: "oci://ghcr.io/magelift/test:provenance",
			SignatureReference:  "oci://ghcr.io/magelift/test:signature",
		},
		Fixture:              "fixture-2026-08-09",
		BackupSet:            "backup-set-1",
		Observability:        "cloudwatch+newrelic",
		Edge:                 "cloudfront",
		SchemaFingerprint:    "schema-1",
		MigrationFingerprint: "migration-1",
		StateBackend:         "state://certification",
	}
	plan, err := PlanForTargetWithBoundary("aws/ecs-fargate", "2.4.9", "open-source", "preview", boundary)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Artifact == nil || plan.Artifact.ImageDigest != boundary.Artifact.ImageDigest {
		t.Fatalf("plan artifact = %#v", plan.Artifact)
	}
	for _, unit := range plan.Execution.Units {
		if unit.ReuseFingerprint == nil {
			t.Fatalf("unit %q has no complete reuse fingerprint", unit.ID)
		}
		if unit.Fingerprint == unit.ReuseFingerprint.Architecture {
			t.Fatalf("unit %q retained semantic placeholder fingerprint", unit.ID)
		}
		digest, digestErr := unit.ReuseFingerprint.Digest()
		if digestErr != nil {
			t.Fatal(digestErr)
		}
		if digest != unit.Fingerprint {
			t.Fatalf("unit %q fingerprint = %q, reuse digest = %q", unit.ID, unit.Fingerprint, digest)
		}
	}
}

func TestReuseBoundaryRejectsArtifactFromAnotherCompatibilityContract(t *testing.T) {
	boundary := ReuseBoundary{
		CompatibilityFingerprint: strings.Repeat("a", 64),
		Artifact: sdk.ImmutableArtifactContract{
			ImageDigest:         "ghcr.io/magelift/test@sha256:" + strings.Repeat("b", 64),
			ManifestDigest:      "sha256:" + strings.Repeat("c", 64),
			InputFingerprint:    strings.Repeat("d", 64),
			ProvenanceReference: "oci://ghcr.io/magelift/test:provenance",
			SignatureReference:  "oci://ghcr.io/magelift/test:signature",
		},
		Fixture: "fixture", BackupSet: "backup", Observability: "native", Edge: "none",
		SchemaFingerprint: "schema", MigrationFingerprint: "migration", StateBackend: "state",
	}
	if err := boundary.Validate(); err == nil {
		t.Fatal("expected compatibility/artifact mismatch")
	}
}

func TestReuseBoundaryRejectsSecretMaterial(t *testing.T) {
	boundary := ReuseBoundary{
		CompatibilityFingerprint: strings.Repeat("a", 64),
		Artifact: sdk.ImmutableArtifactContract{
			ImageDigest:         "ghcr.io/magelift/test@sha256:" + strings.Repeat("b", 64),
			ManifestDigest:      "sha256:" + strings.Repeat("c", 64),
			InputFingerprint:    strings.Repeat("a", 64),
			ProvenanceReference: "oci://ghcr.io/magelift/test:provenance",
			SignatureReference:  "oci://ghcr.io/magelift/test:signature",
		},
		Fixture:              "fixture",
		BackupSet:            "backup",
		Observability:        "cloudwatch password=not-a-reference",
		Edge:                 "none",
		SchemaFingerprint:    "schema",
		MigrationFingerprint: "migration",
		StateBackend:         "state",
	}
	if err := boundary.Validate(); err == nil {
		t.Fatal("expected secret-material rejection")
	}
}
