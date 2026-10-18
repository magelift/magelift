package certification

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

const testImageDigest = "registry.example.invalid/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestRecordSealRejectsMutableArtifactsAndManualPassRows(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Artifact.ImageDigest = "registry.example.invalid/magento:latest"
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "immutable image digest") {
		t.Fatalf("mutable image tag error = %v", err)
	}

	record = testCellRecord(StatusPass)
	record.Provenance.GeneratedBy = "operator"
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "generated") {
		t.Fatalf("manual PASS error = %v", err)
	}
}

func TestRecordSealRejectsSecretLikeAssignments(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Record)
	}{
		{name: "reason", mutate: func(record *Record) { record.Reason = "token=abcdefghijkl" }},
		{name: "cleanup inventory", mutate: func(record *Record) { record.Cleanup.Live = []string{"secret=abcdefghijkl"} }},
		{name: "resilience operation", mutate: func(record *Record) {
			record.Resilience = []ResilienceProof{{DataClass: "database", Action: "backup", Status: StatusPass, OperationID: "password=abcdefghijkl"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := testCellRecord(StatusPass)
			test.mutate(&record)
			if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "secret-like assignment") {
				t.Fatalf("secret-like evidence error = %v", err)
			}
		})
	}
}

func TestRecordSealAllowsSecretReferences(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Dimensions.Edition = "commerce"
	record.CellID = record.Dimensions.ID()
	record.Artifact.ComposerCredentialsConfigured = true
	record.Artifact.ComposerCredentialScheme = "gcp-secret-manager"
	if err := record.Seal(); err != nil {
		t.Fatalf("secret reference was rejected: %v", err)
	}
}

func TestRecordSealCarriesCompleteReuseBoundaryProof(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Session.ArtifactDigest = testImageDigest
	record.Session.FixtureID = "fixture-2026-08-09"
	record.Session.BackupSet = "backup-set-1"
	record.Session.ObservabilitySetup = "cloudwatch+newrelic"
	record.Session.EdgeSetup = "cloudfront"
	record.Session.SchemaFingerprint = "schema-1"
	record.Session.MigrationFingerprint = "migration-1"
	record.Session.StateBackend = "state://certification"
	if err := record.Seal(); err != nil {
		t.Fatalf("complete reuse boundary proof was rejected: %v", err)
	}

	record = testCellRecord(StatusPass)
	record.Session.FixtureID = "fixture-only"
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "boundary proof") {
		t.Fatalf("partial reuse boundary proof was accepted: %v", err)
	}
}

func TestRecordDigestIncludesReuseBoundaryProof(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Session.ArtifactDigest = testImageDigest
	record.Session.FixtureID = "fixture-2026-08-12"
	record.Session.BackupSet = "backup-set-1"
	record.Session.ObservabilitySetup = "cloudwatch+newrelic"
	record.Session.EdgeSetup = "cloudfront"
	record.Session.SchemaFingerprint = "schema-1"
	record.Session.MigrationFingerprint = "migration-1"
	record.Session.StateBackend = "state://certification"
	if err := record.Seal(); err != nil {
		t.Fatalf("complete reuse boundary proof was rejected: %v", err)
	}
	record.Session.FixtureID = "edited-after-seal"
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("reuse boundary edit was not detected: %v", err)
	}
}

func TestSealJSONLSealsCandidatesWithCoreValidation(t *testing.T) {
	cell := testCellRecord(StatusFail)
	cell.Reason = "provider operation failed before runtime readiness"
	cleanup := Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: cell.RunID, Status: StatusPass,
		Cleanup:    CleanupProof{Status: StatusPass, Prefix: "ml-rc1", OwnershipMarker: "magelift-run=run-1", CheckedAt: "2026-08-12T12:00:00Z"},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-12T12:00:00Z", Source: "acceptance-harness", RunID: cell.RunID},
	}
	cell.RecordDigest = ""
	cleanup.RecordDigest = ""
	cellData, err := json.Marshal(cell)
	if err != nil {
		t.Fatal(err)
	}
	cleanupData, err := json.Marshal(cleanup)
	if err != nil {
		t.Fatal(err)
	}
	var sealed bytes.Buffer
	input := bytes.NewBufferString("\n" + string(cellData) + "\n" + string(cleanupData) + "\n")
	if err := SealJSONL(input, &sealed); err != nil {
		t.Fatalf("SealJSONL() error = %v", err)
	}
	records, err := Read(&sealed)
	if err != nil {
		t.Fatalf("Read(sealed) error = %v", err)
	}
	if len(records) != 2 || records[0].RecordDigest == "" || records[1].RecordDigest == "" {
		t.Fatalf("sealed records = %#v", records)
	}
}

func TestSealJSONLRejectsAlreadySealedCandidates(t *testing.T) {
	record := testCellRecord(StatusFail)
	record.Reason = "provider operation failed"
	if err := record.Seal(); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := SealJSONL(bytes.NewReader(data), &output); err == nil || !strings.Contains(err.Error(), "already sealed") {
		t.Fatalf("already sealed candidate error = %v", err)
	}
}

func TestRecordSealRequiresRecoverySafetyEvidence(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Resilience = []ResilienceProof{{DataClass: "database", Action: "backup", Status: StatusPass, OperationID: "backup-1"}}
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "retention") {
		t.Fatalf("incomplete backup proof was accepted: %v", err)
	}

	record = testCellRecord(StatusPass)
	record.Resilience = []ResilienceProof{{DataClass: "database", Action: "restore", Status: StatusPass, RestoreID: "restore-1"}}
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "fixture") {
		t.Fatalf("incomplete restore proof was accepted: %v", err)
	}

	record.Resilience = []ResilienceProof{{
		DataClass: "database", Action: "restore", Status: StatusPass, RestoreID: "restore-1", FixtureID: "fixture-1", RestoreDurationSeconds: 12,
		ManifestVerified: true, CountsVerified: true, ApplicationReadsVerified: true, PermissionsVerified: true,
		SecretReferencesVerified: true, ServiceHealthVerified: true,
	}}
	if err := record.Seal(); err != nil {
		t.Fatalf("complete restore proof was rejected: %v", err)
	}
}

func TestRecordSealRequiresSingleWriterAndFailbackEvidence(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Resilience = []ResilienceProof{{DataClass: "database", Action: "failover", Status: StatusPass, OperationID: "failover-1"}}
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "writer identity") {
		t.Fatalf("incomplete failover safety proof was accepted: %v", err)
	}

	record.Resilience = []ResilienceProof{{
		DataClass: "database", Action: "failback", Status: StatusPass, OperationID: "failback-1",
		WriterIdentityRef: "writer/secondary", WriterEpoch: "epoch/8", SingleWriterVerified: true,
		SplitBrainAbsent: true, StaleOriginRejected: true, ApprovalVerified: true,
	}}
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "reconciliation") {
		t.Fatalf("failback without reconciliation was accepted: %v", err)
	}
	record.Resilience[0].ReconciliationVerified = true
	if err := record.Seal(); err != nil {
		t.Fatalf("complete failback safety proof was rejected: %v", err)
	}
}

func TestRecordSealRequiresFailureInjectionProof(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Failures = []FailureScenarioProof{{ScenarioID: "scenario-zone_loss", Status: StatusPass}}
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "injection verification") {
		t.Fatalf("unverified failure scenario was accepted: %v", err)
	}
	record.Failures[0] = FailureScenarioProof{ScenarioID: "scenario-zone_loss", Status: StatusFail, Reason: "observation failed"}
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "injection verification") {
		t.Fatalf("unverified failed scenario was accepted: %v", err)
	}
	record.Failures[0] = FailureScenarioProof{ScenarioID: "scenario-zone_loss", Status: StatusPass, InjectionVerified: true, TrafficHealthVerified: true, IntegrityVerified: true, FencingVerified: true, MeasuredRTOSeconds: 42}
	if err := record.Seal(); err != nil {
		t.Fatalf("verified failure scenario was rejected: %v", err)
	}
}

func TestRecordSealRequiresUniversalEdgeVerificationFlags(t *testing.T) {
	checks := []struct {
		name   string
		proof  EdgeProof
		needle string
	}{
		{name: "origin health", proof: EdgeProof{Feature: "origin-health", Identity: "origin-1"}, needle: "origin-health"},
		{name: "TLS", proof: EdgeProof{Feature: "tls", Identity: "certificate-1"}, needle: "tls"},
		{name: "purge", proof: EdgeProof{Feature: "purge", Identity: "purge-1"}, needle: "purge"},
		{name: "routing", proof: EdgeProof{Feature: "routing", Identity: "route-1"}, needle: "routing"},
		{name: "DNS ownership", proof: EdgeProof{Feature: "dns-ownership", Identity: "dns-1"}, needle: "dns-ownership"},
		{name: "cache policy", proof: EdgeProof{Feature: "cache-policy", Identity: "cache-1"}, needle: "cache-policy"},
		{name: "WAF", proof: EdgeProof{Feature: "waf", Identity: "waf-1"}, needle: "waf"},
		{name: "failover", proof: EdgeProof{Feature: "failover", Identity: "failover-1"}, needle: "failover"},
		{name: "rollback", proof: EdgeProof{Feature: "rollback", Identity: "rollback-1"}, needle: "rollback"},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			record := testCellRecord(StatusPass)
			check.proof.Provider = "edge"
			check.proof.Status = StatusPass
			record.Edge = []EdgeProof{check.proof}
			if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "verification flag") {
				t.Fatalf("unverified edge proof was accepted: %v", err)
			}
		})
	}
}

func TestRecordSealRequiresResolvedRuntimeContractForPass(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*ArtifactProof)
		message string
	}{
		{name: "PHP version", mutate: func(artifact *ArtifactProof) { artifact.PHPVersion = "" }, message: "PHP version"},
		{name: "extensions", mutate: func(artifact *ArtifactProof) { artifact.PHPExtensions = nil }, message: "extension set"},
		{name: "Composer version", mutate: func(artifact *ArtifactProof) { artifact.ComposerVersion = "" }, message: "Composer version"},
		{name: "duplicate extension", mutate: func(artifact *ArtifactProof) { artifact.PHPExtensions = []string{"intl", "intl"} }, message: "duplicate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := testCellRecord(StatusPass)
			test.mutate(&record.Artifact)
			if err := record.Seal(); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("runtime contract error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestRecordSealRequiresWarmSessionProofForPass(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*SessionProof)
		message string
	}{
		{name: "mode", mutate: func(session *SessionProof) { session.Mode = "cold" }, message: "session mode"},
		{name: "stack", mutate: func(session *SessionProof) { session.StackID = "" }, message: "stack ID"},
		{name: "fingerprint", mutate: func(session *SessionProof) { session.Fingerprint = "not-a-digest" }, message: "fingerprint"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			record := testCellRecord(StatusPass)
			test.mutate(&record.Session)
			if err := record.Seal(); err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("session proof error = %v, want %q", err, test.message)
			}
		})
	}
}

func TestRecordSealDetectsEditsAndEditionCredentialGaps(t *testing.T) {
	record := testCellRecord(StatusPass)
	record.Dimensions.Edition = "commerce"
	record.CellID = record.Dimensions.ID()
	if err := record.Seal(); err == nil || !strings.Contains(err.Error(), "Composer credentials") {
		t.Fatalf("missing Commerce credential error = %v", err)
	}

	record.Artifact.ComposerCredentialsConfigured = true
	record.Artifact.ComposerCredentialScheme = "gcp-secret-manager"
	if err := record.Seal(); err != nil {
		t.Fatal(err)
	}
	record.Status = StatusFail
	record.Reason = "edited after sealing"
	if err := record.Validate(); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("edited record error = %v", err)
	}
}

func TestVerifyRunRequiresCleanupProof(t *testing.T) {
	cell := testCellRecord(StatusPass)
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRun([]Record{cell}, []string{cell.CellID}); err == nil || !strings.Contains(err.Error(), "cleanup") {
		t.Fatalf("missing cleanup error = %v", err)
	}

	cleanup := Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: cell.RunID, Status: StatusPass,
		Cleanup:    CleanupProof{Status: StatusPass, Prefix: "ml-rc1-preview", OwnershipMarker: "magelift-run=run-1", CheckedAt: "2026-08-04T12:00:00Z"},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-04T12:00:00Z", Source: "scripts/aws-acceptance-local.sh", RunID: cell.RunID},
	}
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyRun([]Record{cell, cleanup}, []string{cell.CellID})
	if err != nil {
		t.Fatal(err)
	}
	if !report.CleanupVerified || report.PassedCells != 1 {
		t.Fatalf("report = %#v", report)
	}
}

func TestVerifyRunWithRequirementsGatesEachPassCell(t *testing.T) {
	cell := testCellRecord(StatusPass)
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	cleanup := Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: cell.RunID, Status: StatusPass,
		Cleanup:    CleanupProof{Status: StatusPass, Prefix: "ml-rc1", OwnershipMarker: "magelift-run=run-1"},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-04T12:00:00Z", Source: "cleanup", RunID: cell.RunID},
	}
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	requirements := RunRequirements{
		RequireArchitecture: true, RequiredResilienceDataClasses: []string{"database"},
		RequiredResilienceActions: []string{"backup"}, RequiredObservabilitySignals: []string{"metrics"},
		RequiredEdgeFeatures: []string{"routing"}, RequireSourceProof: true, RequireMeasuredRecovery: true,
		MaxMeasuredRPOSeconds: 60, MaxMeasuredRTOSeconds: 60, MaxEvidenceAgeSeconds: 7 * 24 * 60 * 60,
	}
	if _, err := VerifyRunWithRequirements([]Record{cell, cleanup}, []string{cell.CellID}, requirements); err == nil || !strings.Contains(err.Error(), "architecture") {
		t.Fatalf("missing proof error = %v", err)
	}

	cell = testCellRecord(StatusPass)
	cell.Architecture = ArchitectureProof{
		ProfileID: "aws.ecs.fargate", Fingerprint: strings.Repeat("b", 64), Provider: "aws", Runtime: "ecs-fargate",
		Region: "eu-west-1", ComputeMode: "fargate", ResilienceProfileID: "aws.ecs", ArtifactDigest: testImageDigest,
	}
	cell.Resilience = []ResilienceProof{{DataClass: "database", Action: "backup", Status: StatusPass, OperationID: "backup-1", RetentionDays: 30, EncryptionVerified: true, ProtectionVerified: true, OwnershipVerified: true, MeasuredRPOSeconds: 10, MeasuredRTOSeconds: 30}}
	cell.Observability = []ObservabilityProof{{Provider: "cloudwatch", Signal: "metrics", Status: StatusPass, Identity: "metric-1"}}
	cell.Edge = []EdgeProof{{Provider: "cloudfront", Feature: "routing", Status: StatusPass, Identity: "distribution-1", OriginHealthVerified: true, RouteVerified: true}}
	cell.Sources = []SourceProof{{Claim: "capability", URL: "https://docs.example.invalid/capability", RetrievedAt: time.Now().UTC().Format(time.RFC3339), Status: StatusPass}}
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	report, err := VerifyRunWithRequirements([]Record{cell, cleanup}, []string{cell.CellID}, requirements)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.MissingProofs) != 0 {
		t.Fatalf("missing proofs = %#v", report.MissingProofs)
	}

	cell.Observability[0].Identity = "edited-after-seal"
	if err := cell.Validate(); err == nil || !strings.Contains(err.Error(), "digest") {
		t.Fatalf("optional proof edit was not detected: %v", err)
	}
}

func TestVerifyRunWithRequirementsGatesObservabilityObjects(t *testing.T) {
	cell := testCellRecord(StatusPass)
	cell.Observability = []ObservabilityProof{
		{Provider: "cloudwatch", ObjectType: "signal", Signal: "metrics", Status: StatusPass, Identity: "metric-1"},
		{Provider: "cloudwatch", ObjectType: "alert", ObjectID: "availability", Status: StatusPass, Identity: "alarm-1"},
		{Provider: "cloudwatch", ObjectType: "dashboard", ObjectID: "operations", Status: StatusPass, Identity: "dashboard-1"},
		{Provider: "cloudwatch", ObjectType: "slo", ObjectID: "availability-slo", Status: StatusPass, Identity: "slo-1"},
	}
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	cleanup := Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: cell.RunID, Status: StatusPass,
		Cleanup:    CleanupProof{Status: StatusPass, Prefix: "ml-rc1", OwnershipMarker: "magelift-run=run-1"},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-04T12:00:00Z", Source: "cleanup", RunID: cell.RunID},
	}
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	requirements := RunRequirements{
		RequiredObservabilitySignals:    []string{"metrics"},
		RequiredObservabilityAlerts:     []string{"availability"},
		RequiredObservabilityDashboards: []string{"operations"},
		RequiredObservabilitySLOs:       []string{"availability-slo"},
	}
	if _, err := VerifyRunWithRequirements([]Record{cell, cleanup}, []string{cell.CellID}, requirements); err != nil {
		t.Fatalf("complete observability proof was rejected: %v", err)
	}

	cell = testCellRecord(StatusPass)
	cell.Observability = []ObservabilityProof{{Provider: "cloudwatch", ObjectType: "dashboard", ObjectID: "operations", Status: StatusPass, Identity: "dashboard-1"}}
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	cleanup.RunID = cell.RunID
	cleanup.Provenance.RunID = cell.RunID
	cleanup.RecordDigest = ""
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRunWithRequirements([]Record{cell, cleanup}, []string{cell.CellID}, requirements); err == nil || !strings.Contains(err.Error(), "observability") {
		t.Fatalf("incomplete observability objects were accepted: %v", err)
	}
}

func TestVerifyRunWithRequirementsRejectsNeighboringAndStaleProof(t *testing.T) {
	cell := testCellRecord(StatusPass)
	cell.Architecture = ArchitectureProof{
		ProfileID: "gcp.gke.autopilot", Fingerprint: strings.Repeat("b", 64), Provider: "gcp", Runtime: "gke",
		Region: "europe-west1", ComputeMode: "autopilot", ResilienceProfileID: "gcp.gke", ArtifactDigest: testImageDigest,
	}
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	cleanup := Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: cell.RunID, Status: StatusPass,
		Cleanup:    CleanupProof{Status: StatusPass, Prefix: "ml-rc1", OwnershipMarker: "magelift-run=run-1"},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-04T12:00:00Z", Source: "cleanup", RunID: cell.RunID},
	}
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRunWithRequirements([]Record{cell, cleanup}, []string{cell.CellID}, RunRequirements{RequireArchitecture: true}); err == nil || !strings.Contains(err.Error(), "architecture") {
		t.Fatalf("neighboring architecture was accepted: %v", err)
	}

	cell = testCellRecord(StatusPass)
	cell.Provenance.GeneratedAt = "2020-01-01T00:00:00Z"
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	cleanup.RunID = cell.RunID
	cleanup.Provenance.RunID = cell.RunID
	cleanup.Provenance.GeneratedAt = cell.Provenance.GeneratedAt
	cleanup.RecordDigest = ""
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyRunWithRequirements([]Record{cell, cleanup}, []string{cell.CellID}, RunRequirements{MaxEvidenceAgeSeconds: 1}); err == nil || !strings.Contains(err.Error(), "stale-evidence") {
		t.Fatalf("stale evidence was accepted: %v", err)
	}
}

func TestCleanupProofSeparatesDelayedAndProtectedResources(t *testing.T) {
	cleanup := Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: "run-cleanup", Status: StatusPass,
		Cleanup: CleanupProof{
			Status: StatusPass, Prefix: "ml-rc1", OwnershipMarker: "magelift-run=run-cleanup",
			Protected: []string{"gcp secret/magelift-composer-auth"}, Metadata: []string{"ovh project/8728"},
		},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-07T12:00:00Z", Source: "cleanup", RunID: "run-cleanup"},
	}
	if err := cleanup.Seal(); err != nil {
		t.Fatal(err)
	}
	if len(cleanup.Cleanup.Protected) != 1 || len(cleanup.Cleanup.Metadata) != 1 {
		t.Fatalf("cleanup classification = %#v", cleanup.Cleanup)
	}
	cleanup = Record{
		Version: EvidenceVersion, Type: RecordCleanup, RunID: "run-cleanup", Status: StatusPass,
		Cleanup:    CleanupProof{Status: StatusPass, Prefix: "ml-rc1", OwnershipMarker: "magelift-run=run-cleanup", Delayed: []string{"gcp service networking"}},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: "2026-08-07T12:00:00Z", Source: "cleanup", RunID: "run-cleanup"},
	}
	if err := cleanup.Seal(); err == nil || !strings.Contains(err.Error(), "remaining") {
		t.Fatalf("delayed cleanup was accepted: %v", err)
	}
}

func TestReadRejectsUnsealedJSONL(t *testing.T) {
	if _, err := Read(bytes.NewBufferString(`{"version":"v1","type":"cell"}
`)); err == nil {
		t.Fatal("unsealed row was accepted")
	}
}

func TestLoadEvidenceBundleReadsOnlyValidatedJSONLInStableOrder(t *testing.T) {
	cell := testCellRecord(StatusFail)
	cell.RunID = "run-first"
	cell.Provenance.RunID = cell.RunID
	cell.Reason = "provider credentials are unavailable"
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	var first bytes.Buffer
	if err := Write(&first, cell); err != nil {
		t.Fatal(err)
	}
	cell.RunID = "run-second"
	cell.Provenance.RunID = cell.RunID
	cell.CellID = cell.Dimensions.ID()
	cell.RecordDigest = ""
	if err := cell.Seal(); err != nil {
		t.Fatal(err)
	}
	var second bytes.Buffer
	if err := Write(&second, cell); err != nil {
		t.Fatal(err)
	}

	bundle, err := LoadEvidenceBundle(fstest.MapFS{
		"runs/z.jsonl":   &fstest.MapFile{Data: second.Bytes()},
		"runs/a.jsonl":   &fstest.MapFile{Data: first.Bytes()},
		"runs/README.md": &fstest.MapFile{Data: []byte("not evidence")},
	}, "runs")
	if err != nil {
		t.Fatalf("LoadEvidenceBundle() error = %v", err)
	}
	if len(bundle.Files) != 2 || bundle.Files[0].Path != "runs/a.jsonl" || bundle.Files[1].Path != "runs/z.jsonl" {
		t.Fatalf("bundle files = %#v", bundle.Files)
	}
	if len(bundle.Records) != 2 || bundle.Records[0].RunID != "run-first" || bundle.Records[1].RunID != "run-second" {
		t.Fatalf("bundle records = %#v", bundle.Records)
	}
}

func TestLoadEvidenceBundleTreatsMissingDirectoryAsNoLiveEvidence(t *testing.T) {
	bundle, err := LoadEvidenceBundle(fstest.MapFS{}, "missing/runs")
	if err != nil {
		t.Fatalf("LoadEvidenceBundle() error = %v", err)
	}
	if bundle.Directory != "missing/runs" || len(bundle.Files) != 0 || len(bundle.Records) != 0 {
		t.Fatalf("empty bundle = %#v", bundle)
	}
}

func TestCellsForTargetIsExplicitAboutExperimentalOptions(t *testing.T) {
	cells, err := CellsForTarget("aws/ecs-fargate", "2.4.9", "open-source", "preview")
	if err != nil {
		t.Fatal(err)
	}
	if len(cells) < 20 {
		t.Fatalf("cells = %d, want expanded service combinations", len(cells))
	}
	var foundArtemis bool
	var foundMariaDB bool
	for _, cell := range cells {
		if cell.Dimensions.Queue == "ecs-artemis" {
			foundArtemis = true
			if cell.Status != CapabilityExperimental || cell.Required {
				t.Fatalf("Artemis cell = %#v", cell)
			}
		}
		if cell.Dimensions.Database == "rds-mariadb" {
			foundMariaDB = true
			if cell.Status != CapabilityExperimental || cell.Required {
				t.Fatalf("MariaDB cell = %#v", cell)
			}
		}
	}
	if !foundArtemis {
		t.Fatal("expanded matrix omitted Artemis")
	}
	if !foundMariaDB {
		t.Fatal("expanded matrix omitted RDS MariaDB")
	}
}

func TestCellsForTargetIncludesEveryComputeAndKubernetesBoundary(t *testing.T) {
	cells, err := CellsForTarget("aws/eks", "2.4.9", "open-source", "preview")
	if err != nil {
		t.Fatal(err)
	}
	wantCompute := map[string]bool{"auto-mode": false, "fargate": false, "managed-node-groups": false, "self-managed": false}
	wantKubernetes := map[string]bool{"managed-control-plane": false, "multi-zone": false}
	warmBoundaries := make(map[string]struct{})
	cellIDs := make(map[string]struct{}, len(cells))
	for _, cell := range cells {
		if _, exists := cellIDs[cell.ID]; exists {
			t.Fatalf("duplicate cell ID %q", cell.ID)
		}
		cellIDs[cell.ID] = struct{}{}
		if _, exists := wantCompute[cell.Dimensions.ComputeMode]; exists {
			wantCompute[cell.Dimensions.ComputeMode] = true
		}
		if _, exists := wantKubernetes[cell.Dimensions.KubernetesMode]; exists {
			wantKubernetes[cell.Dimensions.KubernetesMode] = true
		}
		warmBoundaries[cell.WarmBoundary] = struct{}{}
	}
	for mode, found := range wantCompute {
		if !found {
			t.Errorf("matrix omitted EKS compute mode %q", mode)
		}
	}
	for mode, found := range wantKubernetes {
		if !found {
			t.Errorf("matrix omitted EKS Kubernetes mode %q", mode)
		}
	}
	if len(warmBoundaries) < len(wantCompute)*len(wantKubernetes) {
		t.Fatalf("architecture modes collapsed into warm boundaries: %d", len(warmBoundaries))
	}
}

func TestCellsForTargetAppliesAdobeReleaseCompatibility(t *testing.T) {
	tests := []struct {
		name         string
		targetID     string
		release      string
		dimensions   func(Dimensions) bool
		wantStatus   CapabilityStatus
		wantRequired bool
		wantNote     string
	}{
		{
			name:     "mysql is rejected on 2.4.6 latest patch",
			targetID: "aws/ecs-fargate",
			release:  "2.4.6-p15",
			dimensions: func(dimensions Dimensions) bool {
				return dimensions.Database == "rds-mysql" && dimensions.Search == "disabled" && dimensions.Queue == "db" && dimensions.Cache == "valkey" && dimensions.WebCache == "none" && dimensions.Edge == "none"
			},
			wantStatus:   CapabilityUnsupported,
			wantRequired: false,
			wantNote:     "Adobe 2.4.6-p15 marks database/mysql unsupported",
		},
		{
			name:     "mysql is accepted on 2.4.8 latest patch",
			targetID: "aws/ecs-fargate",
			release:  "2.4.8-p5",
			dimensions: func(dimensions Dimensions) bool {
				return dimensions.ComputeMode == "fargate" && dimensions.Database == "rds-mysql" && dimensions.Search == "disabled" && dimensions.Queue == "db" && dimensions.Cache == "valkey" && dimensions.WebCache == "none" && dimensions.Edge == "none"
			},
			wantStatus:   CapabilityCompatible,
			wantRequired: true,
		},
		{
			name:     "redis is rejected on 2.4.9 latest patch",
			targetID: "scaleway/kapsule",
			release:  "2.4.9",
			dimensions: func(dimensions Dimensions) bool {
				return dimensions.Database == "mysql" && dimensions.Search == "disabled" && dimensions.Queue == "database" && dimensions.Cache == "redis" && dimensions.WebCache == "none" && dimensions.Edge == "none"
			},
			wantStatus:   CapabilityUnsupported,
			wantRequired: false,
			wantNote:     "Adobe 2.4.9 marks cache/redis unsupported",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cells, err := CellsForTarget(test.targetID, test.release, "open-source", "preview")
			if err != nil {
				t.Fatal(err)
			}
			var matches []ArchitectureCell
			for _, cell := range cells {
				if test.dimensions(cell.Dimensions) {
					matches = append(matches, cell)
				}
			}
			if len(matches) == 0 {
				t.Fatalf("matching cells = %d, want at least 1", len(matches))
			}
			for _, cell := range matches {
				if cell.Status != test.wantStatus {
					t.Fatalf("cell status = %s, want %s: %#v", cell.Status, test.wantStatus, cell)
				}
				if cell.Required != test.wantRequired {
					t.Fatalf("cell required = %t, want %t: %#v", cell.Required, test.wantRequired, cell)
				}
				if test.wantNote != "" && !strings.Contains(cell.Notes, test.wantNote) {
					t.Fatalf("cell notes = %q, want %q", cell.Notes, test.wantNote)
				}
			}
		})
	}
}

func testCellRecord(status Status) Record {
	dimensions := Dimensions{
		Provider: "aws", Runtime: "ecs-fargate", Release: "2.4.9", Edition: "open-source",
		ComputeMode: "fargate", KubernetesMode: "none",
		Preset: "preview", Database: "rds-mysql", Search: "disabled", Queue: "db",
		Cache: "valkey", WebCache: "varnish", Edge: "none", Scenario: "architecture",
	}
	return Record{
		Version: EvidenceVersion, Type: RecordCell, RunID: "run-1", CellID: dimensions.ID(), Dimensions: dimensions,
		Status: status, Artifact: ArtifactProof{
			ImageDigest: testImageDigest, PHPVersion: "8.5.4", PHPExtensions: []string{"intl", "pdo_mysql"}, ComposerVersion: "2.10.2",
		},
		Session:    SessionProof{Mode: "baseline", StackID: "magelift/stack-1", Fingerprint: strings.Repeat("b", 64), MigrationOwner: true},
		Provenance: Provenance{GeneratedBy: EvidenceGenerator, GeneratedAt: time.Now().UTC().Format(time.RFC3339), Source: "scripts/aws-acceptance-local.sh", RunID: "run-1"},
		Cost:       CostProof{DurationSeconds: 1},
	}
}
