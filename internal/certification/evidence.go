// Package certification defines the machine-readable acceptance contract.
//
// Markdown matrices remain useful for people, but they are not a release gate.
// The JSONL records in this package carry the exact service choices, artifact
// provenance, and cleanup proof needed to make a certification claim.
package certification

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	EvidenceVersion   = "v1"
	EvidenceGenerator = "magelift-acceptance/v1"
)

type RecordType string

const (
	RecordCell    RecordType = "cell"
	RecordCleanup RecordType = "cleanup"
)

type Status string

const (
	StatusPass    Status = "PASS"
	StatusFail    Status = "FAIL"
	StatusSkip    Status = "SKIP"
	StatusBlocked Status = "BLOCKED"
	StatusPending Status = "PENDING"
	StatusNotRun  Status = "NOT_RUN"
)

var (
	imageDigestPattern  = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	cellPartPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._:-]*$`)
	runIDPattern        = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	versionPattern      = regexp.MustCompile(`^\d+\.\d+(?:\.\d+)?$`)
	phpExtensionPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	fingerprintPattern  = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// Dimensions identify one exact acceptance topology. The values are
// deliberately strings because providers use different service names for
// equivalent roles.
type Dimensions struct {
	Provider       string `json:"provider" yaml:"provider"`
	Runtime        string `json:"runtime" yaml:"runtime"`
	ComputeMode    string `json:"computeMode" yaml:"computeMode"`
	KubernetesMode string `json:"kubernetesMode" yaml:"kubernetesMode"`
	Release        string `json:"release" yaml:"release"`
	Edition        string `json:"edition" yaml:"edition"`
	Preset         string `json:"preset" yaml:"preset"`
	Database       string `json:"database" yaml:"database"`
	Search         string `json:"search" yaml:"search"`
	Queue          string `json:"queue" yaml:"queue"`
	Cache          string `json:"cache" yaml:"cache"`
	WebCache       string `json:"webCache" yaml:"webCache"`
	Edge           string `json:"edge" yaml:"edge"`
	Scenario       string `json:"scenario" yaml:"scenario"`
}

// ID returns the stable cell identifier used by checkpoints and evidence.
// Every position is retained so an empty optional choice cannot shift fields.
func (d Dimensions) ID() string {
	parts := []string{
		"rc1", valueOrNone(d.Provider), valueOrNone(d.Runtime), valueOrNone(d.ComputeMode), valueOrNone(d.KubernetesMode), valueOrNone(d.Release),
		valueOrNone(d.Edition), valueOrNone(d.Preset), valueOrNone(d.Database),
		valueOrNone(d.Search), valueOrNone(d.Queue), valueOrNone(d.Cache),
		valueOrNone(d.WebCache), valueOrNone(d.Edge), valueOrNone(d.Scenario),
	}
	return strings.Join(parts, "/")
}

type ArtifactProof struct {
	ImageDigest                   string   `json:"imageDigest,omitempty" yaml:"imageDigest,omitempty"`
	PHPVersion                    string   `json:"phpVersion" yaml:"phpVersion"`
	PHPExtensions                 []string `json:"phpExtensions" yaml:"phpExtensions"`
	ComposerVersion               string   `json:"composerVersion" yaml:"composerVersion"`
	ComposerCredentialsConfigured bool     `json:"composerCredentialsConfigured" yaml:"composerCredentialsConfigured"`
	ComposerCredentialScheme      string   `json:"composerCredentialScheme,omitempty" yaml:"composerCredentialScheme,omitempty"`
}

type CleanupProof struct {
	Status          Status   `json:"status" yaml:"status"`
	Prefix          string   `json:"prefix,omitempty" yaml:"prefix,omitempty"`
	OwnershipMarker string   `json:"ownershipMarker,omitempty" yaml:"ownershipMarker,omitempty"`
	Remaining       []string `json:"remaining,omitempty" yaml:"remaining,omitempty"`
	Live            []string `json:"live,omitempty" yaml:"live,omitempty"`
	Delayed         []string `json:"delayed,omitempty" yaml:"delayed,omitempty"`
	Protected       []string `json:"protected,omitempty" yaml:"protected,omitempty"`
	Metadata        []string `json:"metadata,omitempty" yaml:"metadata,omitempty"`
	CheckedAt       string   `json:"checkedAt,omitempty" yaml:"checkedAt,omitempty"`
	LatencySeconds  int64    `json:"latencySeconds,omitempty" yaml:"latencySeconds,omitempty"`
}

type Provenance struct {
	GeneratedBy string `json:"generatedBy" yaml:"generatedBy"`
	GeneratedAt string `json:"generatedAt" yaml:"generatedAt"`
	Source      string `json:"source" yaml:"source"`
	RunID       string `json:"runId" yaml:"runId"`
	AccountRef  string `json:"accountRef,omitempty" yaml:"accountRef,omitempty"`
	Certificate string `json:"certificate,omitempty" yaml:"certificate,omitempty"`
}

type CostProof struct {
	DurationSeconds         int64  `json:"durationSeconds,omitempty" yaml:"durationSeconds,omitempty"`
	CloudOperationSeconds   int64  `json:"cloudOperationSeconds,omitempty" yaml:"cloudOperationSeconds,omitempty"`
	ResourceLifetimeSeconds int64  `json:"resourceLifetimeSeconds,omitempty" yaml:"resourceLifetimeSeconds,omitempty"`
	EstimatedCents          int64  `json:"estimatedCents,omitempty" yaml:"estimatedCents,omitempty"`
	Currency                string `json:"currency,omitempty" yaml:"currency,omitempty"`
}

// SessionProof records whether the cell used the first stack state for this
// fingerprint or a compatible retained stack. A PASS without this proof is
// not a reproducible warm-session certification claim.
type SessionProof struct {
	Mode                 string `json:"mode" yaml:"mode"`
	StackID              string `json:"stackId" yaml:"stackId"`
	Fingerprint          string `json:"fingerprint" yaml:"fingerprint"`
	MigrationOwner       bool   `json:"migrationOwner" yaml:"migrationOwner"`
	ArtifactDigest       string `json:"artifactDigest,omitempty" yaml:"artifactDigest,omitempty"`
	FixtureID            string `json:"fixtureId,omitempty" yaml:"fixtureId,omitempty"`
	BackupSet            string `json:"backupSet,omitempty" yaml:"backupSet,omitempty"`
	ObservabilitySetup   string `json:"observabilitySetup,omitempty" yaml:"observabilitySetup,omitempty"`
	EdgeSetup            string `json:"edgeSetup,omitempty" yaml:"edgeSetup,omitempty"`
	SchemaFingerprint    string `json:"schemaFingerprint,omitempty" yaml:"schemaFingerprint,omitempty"`
	MigrationFingerprint string `json:"migrationFingerprint,omitempty" yaml:"migrationFingerprint,omitempty"`
	StateBackend         string `json:"stateBackend,omitempty" yaml:"stateBackend,omitempty"`
}

// ArchitectureProof binds a cell to the exact semantic profile that was
// exercised. A provider/runtime match without this fingerprint is not enough
// to reuse a warm session or make a cross-architecture claim.
type ArchitectureProof struct {
	ProfileID           string `json:"profileId" yaml:"profileId"`
	Fingerprint         string `json:"fingerprint" yaml:"fingerprint"`
	Provider            string `json:"provider" yaml:"provider"`
	Runtime             string `json:"runtime" yaml:"runtime"`
	Region              string `json:"region" yaml:"region"`
	ComputeMode         string `json:"computeMode" yaml:"computeMode"`
	ResilienceProfileID string `json:"resilienceProfileId" yaml:"resilienceProfileId"`
	ArtifactDigest      string `json:"artifactDigest" yaml:"artifactDigest"`
}

// ResilienceProof is one independently verifiable backup, restore, integrity,
// HA, DR, failover, or fencing observation. It intentionally keeps operation
// identifiers and measured recovery values separate from the final cell result.
type ResilienceProof struct {
	DataClass                string `json:"dataClass" yaml:"dataClass"`
	Action                   string `json:"action" yaml:"action"`
	Status                   Status `json:"status" yaml:"status"`
	Destination              string `json:"destination,omitempty" yaml:"destination,omitempty"`
	OperationID              string `json:"operationId,omitempty" yaml:"operationId,omitempty"`
	BackupID                 string `json:"backupId,omitempty" yaml:"backupId,omitempty"`
	RestoreID                string `json:"restoreId,omitempty" yaml:"restoreId,omitempty"`
	FixtureID                string `json:"fixtureId,omitempty" yaml:"fixtureId,omitempty"`
	IntegrityDigest          string `json:"integrityDigest,omitempty" yaml:"integrityDigest,omitempty"`
	RetentionDays            int    `json:"retentionDays,omitempty" yaml:"retentionDays,omitempty"`
	EncryptionVerified       bool   `json:"encryptionVerified" yaml:"encryptionVerified"`
	ProtectionVerified       bool   `json:"protectionVerified" yaml:"protectionVerified"`
	OwnershipVerified        bool   `json:"ownershipVerified" yaml:"ownershipVerified"`
	ManifestVerified         bool   `json:"manifestVerified" yaml:"manifestVerified"`
	CountsVerified           bool   `json:"countsVerified" yaml:"countsVerified"`
	ApplicationReadsVerified bool   `json:"applicationReadsVerified" yaml:"applicationReadsVerified"`
	PermissionsVerified      bool   `json:"permissionsVerified" yaml:"permissionsVerified"`
	SecretReferencesVerified bool   `json:"secretReferencesVerified" yaml:"secretReferencesVerified"`
	ServiceHealthVerified    bool   `json:"serviceHealthVerified" yaml:"serviceHealthVerified"`
	WriterIdentityRef        string `json:"writerIdentityRef,omitempty" yaml:"writerIdentityRef,omitempty"`
	WriterEpoch              string `json:"writerEpoch,omitempty" yaml:"writerEpoch,omitempty"`
	SingleWriterVerified     bool   `json:"singleWriterVerified" yaml:"singleWriterVerified"`
	SplitBrainAbsent         bool   `json:"splitBrainAbsent" yaml:"splitBrainAbsent"`
	StaleOriginRejected      bool   `json:"staleOriginRejected" yaml:"staleOriginRejected"`
	ReconciliationVerified   bool   `json:"reconciliationVerified" yaml:"reconciliationVerified"`
	ApprovalVerified         bool   `json:"approvalVerified" yaml:"approvalVerified"`
	RestoreDurationSeconds   int64  `json:"restoreDurationSeconds,omitempty" yaml:"restoreDurationSeconds,omitempty"`
	MeasuredRPOSeconds       int64  `json:"measuredRpoSeconds,omitempty" yaml:"measuredRpoSeconds,omitempty"`
	MeasuredRTOSeconds       int64  `json:"measuredRtoSeconds,omitempty" yaml:"measuredRtoSeconds,omitempty"`
	OperatorAction           string `json:"operatorAction,omitempty" yaml:"operatorAction,omitempty"`
	Reason                   string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ObservabilityProof records signal delivery and its safety checks without
// placing a vendor object schema in the core evidence format.
type ObservabilityProof struct {
	Provider          string `json:"provider" yaml:"provider"`
	ObjectType        string `json:"objectType,omitempty" yaml:"objectType,omitempty"`
	ObjectID          string `json:"objectId,omitempty" yaml:"objectId,omitempty"`
	Signal            string `json:"signal" yaml:"signal"`
	Status            Status `json:"status" yaml:"status"`
	Identity          string `json:"identity,omitempty" yaml:"identity,omitempty"`
	RetentionDays     int    `json:"retentionDays,omitempty" yaml:"retentionDays,omitempty"`
	LabelsVerified    bool   `json:"labelsVerified" yaml:"labelsVerified"`
	RedactionVerified bool   `json:"redactionVerified" yaml:"redactionVerified"`
	AlertVerified     bool   `json:"alertVerified" yaml:"alertVerified"`
	Reason            string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// EdgeProof records health-gated routing and edge lifecycle behavior. Native
// and Fastly implementations use the same proof shape while owning their API
// details in the adapter.
type EdgeProof struct {
	Provider             string `json:"provider" yaml:"provider"`
	Feature              string `json:"feature" yaml:"feature"`
	Status               Status `json:"status" yaml:"status"`
	Identity             string `json:"identity,omitempty" yaml:"identity,omitempty"`
	OriginHealthVerified bool   `json:"originHealthVerified" yaml:"originHealthVerified"`
	TLSVerified          bool   `json:"tlsVerified" yaml:"tlsVerified"`
	PurgeVerified        bool   `json:"purgeVerified" yaml:"purgeVerified"`
	RouteVerified        bool   `json:"routeVerified" yaml:"routeVerified"`
	DNSOwnershipVerified bool   `json:"dnsOwnershipVerified" yaml:"dnsOwnershipVerified"`
	CachePolicyVerified  bool   `json:"cachePolicyVerified" yaml:"cachePolicyVerified"`
	WAFVerified          bool   `json:"wafVerified" yaml:"wafVerified"`
	FailoverVerified     bool   `json:"failoverVerified" yaml:"failoverVerified"`
	RollbackVerified     bool   `json:"rollbackVerified" yaml:"rollbackVerified"`
	Reason               string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type SourceProof struct {
	Claim       string `json:"claim" yaml:"claim"`
	URL         string `json:"url" yaml:"url"`
	RetrievedAt string `json:"retrievedAt" yaml:"retrievedAt"`
	Status      Status `json:"status" yaml:"status"`
}

// Record is one append-only acceptance observation. A cell record is followed
// by a cleanup record for the same run after teardown has been checked.
type Record struct {
	Version       string                 `json:"version" yaml:"version"`
	Type          RecordType             `json:"type" yaml:"type"`
	RunID         string                 `json:"runId" yaml:"runId"`
	CellID        string                 `json:"cellId,omitempty" yaml:"cellId,omitempty"`
	Dimensions    Dimensions             `json:"dimensions,omitempty" yaml:"dimensions,omitempty"`
	Status        Status                 `json:"status" yaml:"status"`
	Required      bool                   `json:"required,omitempty" yaml:"required,omitempty"`
	Artifact      ArtifactProof          `json:"artifact,omitempty" yaml:"artifact,omitempty"`
	Cleanup       CleanupProof           `json:"cleanup,omitempty" yaml:"cleanup,omitempty"`
	Provenance    Provenance             `json:"provenance" yaml:"provenance"`
	Cost          CostProof              `json:"cost,omitempty" yaml:"cost,omitempty"`
	Session       SessionProof           `json:"session,omitempty" yaml:"session,omitempty"`
	Architecture  ArchitectureProof      `json:"architecture,omitempty" yaml:"architecture,omitempty"`
	Resilience    []ResilienceProof      `json:"resilience,omitempty" yaml:"resilience,omitempty"`
	Failures      []FailureScenarioProof `json:"failures,omitempty" yaml:"failures,omitempty"`
	Observability []ObservabilityProof   `json:"observability,omitempty" yaml:"observability,omitempty"`
	Edge          []EdgeProof            `json:"edge,omitempty" yaml:"edge,omitempty"`
	Sources       []SourceProof          `json:"sources,omitempty" yaml:"sources,omitempty"`
	Reason        string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
	RecordDigest  string                 `json:"recordDigest,omitempty" yaml:"recordDigest,omitempty"`
}

// Seal validates a generated record and adds its content digest. The digest
// catches hand edits and malformed append operations. It is not a substitute
// for a signed attestation when evidence is published outside the workspace.
func (r *Record) Seal() error {
	if r.Version == "" {
		r.Version = EvidenceVersion
	}
	if r.RecordDigest != "" {
		return errors.New("record is already sealed")
	}
	if err := validateRecord(*r, false); err != nil {
		return err
	}
	digest, err := recordDigest(*r)
	if err != nil {
		return err
	}
	r.RecordDigest = digest
	return nil
}

// Validate verifies a record's shape, provenance marker, and content digest.
func (r Record) Validate() error {
	return validateRecord(r, true)
}

// Write appends one sealed record as a single JSONL line.
func Write(w io.Writer, r Record) error {
	if err := r.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("encode evidence record: %w", err)
	}
	if _, err := fmt.Fprintln(w, string(data)); err != nil {
		return fmt.Errorf("write evidence record: %w", err)
	}
	return nil
}

// SealJSONL converts an append-only stream of unsealed evidence candidates
// into the release input format. The conversion belongs in the certification
// core so shell harnesses and community-maintained provider runners cannot
// drift in validation or digest semantics.
//
// A candidate with an existing digest is rejected deliberately. Re-sealing a
// record would hide whether an operator edited an already sealed observation;
// callers that need to copy sealed evidence should use Read and Write.
func SealJSONL(input io.Reader, output io.Writer) error {
	if input == nil || output == nil {
		return errors.New("evidence input and output are required")
	}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	line := 0
	for scanner.Scan() {
		line++
		data := strings.TrimSpace(scanner.Text())
		if data == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			return fmt.Errorf("evidence candidate line %d: %w", line, err)
		}
		if record.RecordDigest != "" {
			return fmt.Errorf("evidence candidate line %d is already sealed", line)
		}
		if err := record.Seal(); err != nil {
			return fmt.Errorf("seal evidence candidate line %d: %w", line, err)
		}
		if err := Write(output, record); err != nil {
			return fmt.Errorf("write sealed evidence line %d: %w", line, err)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read evidence candidates: %w", err)
	}
	return nil
}

// Read reads JSONL evidence and rejects blank, malformed, or unsealed rows.
func Read(r io.Reader) ([]Record, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	records := make([]Record, 0)
	line := 0
	for scanner.Scan() {
		line++
		data := strings.TrimSpace(scanner.Text())
		if data == "" {
			continue
		}
		var record Record
		if err := json.Unmarshal([]byte(data), &record); err != nil {
			return nil, fmt.Errorf("evidence line %d: %w", line, err)
		}
		if err := record.Validate(); err != nil {
			return nil, fmt.Errorf("evidence line %d: %w", line, err)
		}
		records = append(records, record)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read evidence: %w", err)
	}
	return records, nil
}

// EvidenceBundleFile identifies one validated JSONL evidence file. The path
// is relative to the filesystem and directory passed to LoadEvidenceBundle.
// Keeping file identity separate from records lets generated reports show
// exactly which inputs were consumed without copying provider credentials or
// native object payloads into the report.
type EvidenceBundleFile struct {
	Path        string `json:"path" yaml:"path"`
	RecordCount int    `json:"recordCount" yaml:"recordCount"`
}

// EvidenceBundle is the validated, sealed evidence input for generated
// certification documentation. An empty bundle is valid: it means no live
// certification run has been supplied, not that a capability is certified.
type EvidenceBundle struct {
	Directory string               `json:"directory" yaml:"directory"`
	Files     []EvidenceBundleFile `json:"files,omitempty" yaml:"files,omitempty"`
	Records   []Record             `json:"records,omitempty" yaml:"records,omitempty"`
}

// LoadEvidenceBundle reads every JSONL file below directory in deterministic
// path order. Each row is validated by Read, including its content digest and
// secret-safety checks. Missing directories and directories with no JSONL
// files return an empty bundle so offline documentation generation remains
// useful before the first live run.
func LoadEvidenceBundle(fsys fs.FS, directory string) (EvidenceBundle, error) {
	if fsys == nil {
		return EvidenceBundle{}, errors.New("evidence bundle filesystem is required")
	}
	directory = path.Clean(strings.TrimSpace(directory))
	if directory == "" || directory == "." {
		directory = "."
	}
	if path.IsAbs(directory) || directory == ".." || strings.HasPrefix(directory, "../") {
		return EvidenceBundle{}, fmt.Errorf("evidence bundle directory must be a relative filesystem path: %q", directory)
	}
	info, err := fs.Stat(fsys, directory)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return EvidenceBundle{Directory: directory}, nil
		}
		return EvidenceBundle{}, fmt.Errorf("stat evidence bundle directory %q: %w", directory, err)
	}
	if !info.IsDir() {
		return EvidenceBundle{}, fmt.Errorf("evidence bundle path %q is not a directory", directory)
	}

	bundle := EvidenceBundle{Directory: directory}
	err = fs.WalkDir(fsys, directory, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("evidence bundle cannot contain symlink %q", filePath)
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			return nil
		}
		file, err := fsys.Open(filePath)
		if err != nil {
			return fmt.Errorf("open evidence file %q: %w", filePath, err)
		}
		records, readErr := Read(file)
		closeErr := file.Close()
		if readErr != nil {
			return fmt.Errorf("read evidence file %q: %w", filePath, readErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close evidence file %q: %w", filePath, closeErr)
		}
		bundle.Files = append(bundle.Files, EvidenceBundleFile{Path: filePath, RecordCount: len(records)})
		bundle.Records = append(bundle.Records, records...)
		return nil
	})
	if err != nil {
		return EvidenceBundle{}, fmt.Errorf("walk evidence bundle %q: %w", directory, err)
	}
	sort.Slice(bundle.Files, func(i, j int) bool { return bundle.Files[i].Path < bundle.Files[j].Path })
	return bundle, nil
}

type RunReport struct {
	RunID           string   `json:"runId" yaml:"runId"`
	CellCount       int      `json:"cellCount" yaml:"cellCount"`
	PassedCells     int      `json:"passedCells" yaml:"passedCells"`
	CleanupVerified bool     `json:"cleanupVerified" yaml:"cleanupVerified"`
	MissingCells    []string `json:"missingCells,omitempty" yaml:"missingCells,omitempty"`
	MissingProofs   []string `json:"missingProofs,omitempty" yaml:"missingProofs,omitempty"`
}

// RunRequirements are the evidence classes a release claim needs in addition
// to the legacy runtime and cleanup checks. Every PASS cell must satisfy each
// declared requirement; neighboring architecture evidence cannot satisfy it.
type RunRequirements struct {
	RequireArchitecture             bool     `json:"requireArchitecture" yaml:"requireArchitecture"`
	RequiredRecoveryFixtureID       string   `json:"requiredRecoveryFixtureId,omitempty" yaml:"requiredRecoveryFixtureId,omitempty"`
	RequiredResilienceDataClasses   []string `json:"requiredResilienceDataClasses,omitempty" yaml:"requiredResilienceDataClasses,omitempty"`
	RequiredResilienceActions       []string `json:"requiredResilienceActions,omitempty" yaml:"requiredResilienceActions,omitempty"`
	RequiredObservabilitySignals    []string `json:"requiredObservabilitySignals,omitempty" yaml:"requiredObservabilitySignals,omitempty"`
	RequiredObservabilityAlerts     []string `json:"requiredObservabilityAlerts,omitempty" yaml:"requiredObservabilityAlerts,omitempty"`
	RequiredObservabilityDashboards []string `json:"requiredObservabilityDashboards,omitempty" yaml:"requiredObservabilityDashboards,omitempty"`
	RequiredObservabilitySLOs       []string `json:"requiredObservabilitySlos,omitempty" yaml:"requiredObservabilitySlos,omitempty"`
	RequiredEdgeFeatures            []string `json:"requiredEdgeFeatures,omitempty" yaml:"requiredEdgeFeatures,omitempty"`
	RequireSourceProof              bool     `json:"requireSourceProof" yaml:"requireSourceProof"`
	RequireMeasuredRecovery         bool     `json:"requireMeasuredRecovery" yaml:"requireMeasuredRecovery"`
	MaxMeasuredRPOSeconds           int64    `json:"maxMeasuredRpoSeconds,omitempty" yaml:"maxMeasuredRpoSeconds,omitempty"`
	MaxMeasuredRTOSeconds           int64    `json:"maxMeasuredRtoSeconds,omitempty" yaml:"maxMeasuredRtoSeconds,omitempty"`
	MaxEvidenceAgeSeconds           int64    `json:"maxEvidenceAgeSeconds,omitempty" yaml:"maxEvidenceAgeSeconds,omitempty"`
}

// VerifyRun applies the release gate to one run. requiredIDs may be empty for
// an exploratory run, but a cleanup proof is still required for any PASS cell.
func VerifyRun(records []Record, requiredIDs []string) (RunReport, error) {
	if len(records) == 0 {
		return RunReport{}, errors.New("evidence run is empty")
	}
	runID := records[0].RunID
	latestCells := make(map[string]Record)
	latestCleanup := Record{}
	for _, record := range records {
		if err := record.Validate(); err != nil {
			return RunReport{}, err
		}
		if record.RunID != runID {
			return RunReport{}, fmt.Errorf("evidence contains multiple run IDs: %q and %q", runID, record.RunID)
		}
		switch record.Type {
		case RecordCell:
			latestCells[record.CellID] = record
		case RecordCleanup:
			latestCleanup = record
		}
	}
	missing := make([]string, 0)
	for _, requiredID := range requiredIDs {
		record, found := latestCells[requiredID]
		if !found || record.Status != StatusPass {
			missing = append(missing, requiredID)
		}
	}
	sort.Strings(missing)
	cleanupVerified := latestCleanup.Type == RecordCleanup && latestCleanup.Status == StatusPass && latestCleanup.Cleanup.Status == StatusPass && cleanupHasNoLiveResources(latestCleanup.Cleanup)
	report := RunReport{
		RunID: runID, CellCount: len(latestCells), CleanupVerified: cleanupVerified,
		MissingCells: missing,
	}
	for _, record := range latestCells {
		if record.Status == StatusPass {
			report.PassedCells++
		}
	}
	if len(missing) > 0 {
		return report, fmt.Errorf("required evidence cells are missing or not PASS: %s", strings.Join(missing, ", "))
	}
	if report.PassedCells > 0 && !cleanupVerified {
		return report, errors.New("PASS evidence requires a final cleanup record with no remaining resources")
	}
	return report, nil
}

// VerifyRunWithRequirements adds architecture, resilience, telemetry, edge,
// and source gates without changing the v1 compatibility behavior of
// VerifyRun. It is the release-facing entry point for new certifications.
func VerifyRunWithRequirements(records []Record, requiredIDs []string, requirements RunRequirements) (RunReport, error) {
	if err := requirements.validate(); err != nil {
		return RunReport{}, err
	}
	report, err := VerifyRun(records, requiredIDs)
	if err != nil {
		return report, err
	}
	if !requirements.hasRequirements() {
		return report, nil
	}
	latestCells := latestCellRecords(records)
	missing := make([]string, 0)
	for cellID, record := range latestCells {
		if record.Status != StatusPass {
			continue
		}
		missing = append(missing, missingProofsForCell(cellID, record, requirements)...)
		if requirements.MaxEvidenceAgeSeconds > 0 && evidenceIsStale(record.Provenance.GeneratedAt, requirements.MaxEvidenceAgeSeconds, time.Now().UTC()) {
			missing = append(missing, cellID+":stale-evidence")
		}
	}
	sort.Strings(missing)
	report.MissingProofs = missing
	if len(missing) > 0 {
		return report, fmt.Errorf("required evidence proofs are missing or not PASS: %s", strings.Join(missing, ", "))
	}
	return report, nil
}

func (r RunRequirements) hasRequirements() bool {
	return r.RequireArchitecture || r.RequiredRecoveryFixtureID != "" || r.RequireSourceProof || r.RequireMeasuredRecovery || r.MaxMeasuredRPOSeconds > 0 || r.MaxMeasuredRTOSeconds > 0 || r.MaxEvidenceAgeSeconds > 0 || len(r.RequiredResilienceDataClasses) > 0 || len(r.RequiredResilienceActions) > 0 || len(r.RequiredObservabilitySignals) > 0 || len(r.RequiredObservabilityAlerts) > 0 || len(r.RequiredObservabilityDashboards) > 0 || len(r.RequiredObservabilitySLOs) > 0 || len(r.RequiredEdgeFeatures) > 0
}

func (r RunRequirements) validate() error {
	if strings.ContainsAny(r.RequiredRecoveryFixtureID, "\r\n") {
		return errors.New("required recovery fixture ID must not contain line breaks")
	}
	for name, value := range map[string]int64{
		"maximum measured RPO": r.MaxMeasuredRPOSeconds,
		"maximum measured RTO": r.MaxMeasuredRTOSeconds,
		"maximum evidence age": r.MaxEvidenceAgeSeconds,
	} {
		if value < 0 {
			return fmt.Errorf("%s cannot be negative", name)
		}
	}
	return nil
}

func latestCellRecords(records []Record) map[string]Record {
	latest := make(map[string]Record)
	for _, record := range records {
		if record.Type == RecordCell {
			latest[record.CellID] = record
		}
	}
	return latest
}

func missingProofsForCell(cellID string, record Record, requirements RunRequirements) []string {
	missing := make([]string, 0)
	if requirements.RequireArchitecture && !architectureProofMatchesRecord(record) {
		missing = append(missing, cellID+":architecture")
	}
	if requirements.RequiredRecoveryFixtureID != "" && !hasPassingResilience(record.Resilience, func(proof ResilienceProof) bool {
		return proof.Action == "restore" && proof.FixtureID == requirements.RequiredRecoveryFixtureID
	}) {
		missing = append(missing, cellID+":resilience-fixture:"+requirements.RequiredRecoveryFixtureID)
	}
	for _, dataClass := range uniqueSorted(requirements.RequiredResilienceDataClasses) {
		if !hasPassingResilience(record.Resilience, func(proof ResilienceProof) bool { return proof.DataClass == dataClass }) {
			missing = append(missing, cellID+":resilience:"+dataClass)
		}
	}
	for _, action := range uniqueSorted(requirements.RequiredResilienceActions) {
		if !hasPassingResilience(record.Resilience, func(proof ResilienceProof) bool { return proof.Action == action }) {
			missing = append(missing, cellID+":resilience-action:"+action)
		}
	}
	if requirements.RequireMeasuredRecovery && !hasMeasuredRecoveryWithin(record.Resilience, requirements.MaxMeasuredRPOSeconds, requirements.MaxMeasuredRTOSeconds) {
		missing = append(missing, cellID+":resilience:measured-rto")
	}
	for _, signal := range uniqueSorted(requirements.RequiredObservabilitySignals) {
		if !hasPassingObservabilitySignal(record.Observability, signal) {
			missing = append(missing, cellID+":observability:"+signal)
		}
	}
	for _, alert := range uniqueSorted(requirements.RequiredObservabilityAlerts) {
		if !hasPassingObservabilityObject(record.Observability, "alert", alert) {
			missing = append(missing, cellID+":observability-alert:"+alert)
		}
	}
	for _, dashboard := range uniqueSorted(requirements.RequiredObservabilityDashboards) {
		if !hasPassingObservabilityObject(record.Observability, "dashboard", dashboard) {
			missing = append(missing, cellID+":observability-dashboard:"+dashboard)
		}
	}
	for _, slo := range uniqueSorted(requirements.RequiredObservabilitySLOs) {
		if !hasPassingObservabilityObject(record.Observability, "slo", slo) {
			missing = append(missing, cellID+":observability-slo:"+slo)
		}
	}
	for _, feature := range uniqueSorted(requirements.RequiredEdgeFeatures) {
		if !hasPassingEdge(record.Edge, feature) {
			missing = append(missing, cellID+":edge:"+feature)
		}
	}
	if requirements.RequireSourceProof && !hasPassingSource(record.Sources, requirements.MaxEvidenceAgeSeconds, time.Now().UTC()) {
		missing = append(missing, cellID+":source")
	}
	return missing
}

func architectureProofMatchesRecord(record Record) bool {
	proof := record.Architecture
	return !isZeroArchitectureProof(proof) &&
		proof.Provider == record.Dimensions.Provider &&
		proof.Runtime == record.Dimensions.Runtime &&
		proof.ArtifactDigest == record.Artifact.ImageDigest &&
		proof.Fingerprint == record.Session.Fingerprint
}

func hasMeasuredRecoveryWithin(proofs []ResilienceProof, maxRPO, maxRTO int64) bool {
	for _, proof := range proofs {
		if proof.Status != StatusPass || proof.MeasuredRPOSeconds <= 0 || proof.MeasuredRTOSeconds <= 0 {
			continue
		}
		if maxRPO > 0 && proof.MeasuredRPOSeconds > maxRPO {
			continue
		}
		if maxRTO > 0 && proof.MeasuredRTOSeconds > maxRTO {
			continue
		}
		return true
	}
	return false
}

func evidenceIsStale(generatedAt string, maxAgeSeconds int64, now time.Time) bool {
	parsed, err := time.Parse(time.RFC3339, generatedAt)
	if err != nil {
		return true
	}
	return now.Sub(parsed) > time.Duration(maxAgeSeconds)*time.Second
}

func hasPassingResilience(proofs []ResilienceProof, matches func(ResilienceProof) bool) bool {
	for _, proof := range proofs {
		if proof.Status == StatusPass && matches(proof) {
			return true
		}
	}
	return false
}

func hasPassingObservabilitySignal(proofs []ObservabilityProof, signal string) bool {
	for _, proof := range proofs {
		if proof.Status == StatusPass && (proof.ObjectType == "" || proof.ObjectType == "signal") && proof.Signal == signal {
			return true
		}
	}
	return false
}

func hasPassingObservabilityObject(proofs []ObservabilityProof, objectType, objectID string) bool {
	for _, proof := range proofs {
		if proof.Status == StatusPass && proof.ObjectType == objectType && proof.ObjectID == objectID {
			return true
		}
	}
	return false
}

func hasPassingEdge(proofs []EdgeProof, feature string) bool {
	for _, proof := range proofs {
		if proof.Status == StatusPass && proof.Feature == feature && edgeFeatureVerified(proof, feature) {
			return true
		}
	}
	return false
}

func edgeFeatureVerified(proof EdgeProof, feature string) bool {
	switch strings.ToLower(strings.TrimSpace(feature)) {
	case "origin", "origin-health", "origin-health-check":
		return proof.OriginHealthVerified
	case "tls", "certificate", "certificate-rotation":
		return proof.TLSVerified
	case "purge", "cache-purge":
		return proof.PurgeVerified
	case "route", "routing", "traffic-routing":
		return proof.RouteVerified
	case "dns", "dns-ownership":
		return proof.DNSOwnershipVerified
	case "cache", "cache-policy":
		return proof.CachePolicyVerified
	case "waf", "security-policy", "security-headers":
		return proof.WAFVerified
	case "failover", "edge-failover":
		return proof.FailoverVerified
	case "rollback", "edge-rollback":
		return proof.RollbackVerified
	default:
		// Adapter-specific features remain identity-backed. The portable
		// contract only knows the four universal safety flags above.
		return true
	}
}

func hasPassingSource(proofs []SourceProof, maxAgeSeconds int64, now time.Time) bool {
	for _, proof := range proofs {
		if proof.Status != StatusPass {
			continue
		}
		if maxAgeSeconds <= 0 {
			return true
		}
		retrieved, err := parseCatalogDate(proof.RetrievedAt)
		if err == nil && now.Sub(retrieved) <= time.Duration(maxAgeSeconds)*time.Second {
			return true
		}
	}
	return false
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func validateRecord(r Record, requireDigest bool) error {
	if r.Version != EvidenceVersion {
		return fmt.Errorf("unsupported evidence version %q", r.Version)
	}
	if r.Type != RecordCell && r.Type != RecordCleanup {
		return fmt.Errorf("unsupported evidence record type %q", r.Type)
	}
	if !runIDPattern.MatchString(r.RunID) {
		return errors.New("evidence runId is invalid")
	}
	if r.Status != StatusPass && r.Status != StatusFail && r.Status != StatusSkip && r.Status != StatusBlocked && r.Status != StatusPending && r.Status != StatusNotRun {
		return fmt.Errorf("unsupported evidence status %q", r.Status)
	}
	if r.Provenance.GeneratedBy != EvidenceGenerator {
		return errors.New("evidence must be generated by the MageLift acceptance writer")
	}
	if strings.TrimSpace(r.Provenance.GeneratedAt) == "" || strings.TrimSpace(r.Provenance.Source) == "" || r.Provenance.RunID != r.RunID {
		return errors.New("evidence provenance is incomplete")
	}
	if _, err := time.Parse(time.RFC3339, r.Provenance.GeneratedAt); err != nil {
		return fmt.Errorf("evidence provenance generatedAt: %w", err)
	}
	if err := validateEvidenceSecretSafety(r); err != nil {
		return err
	}
	if r.Type == RecordCell {
		if err := validateCell(r); err != nil {
			return err
		}
	} else if err := validateCleanup(r); err != nil {
		return err
	}
	if requireDigest {
		if r.RecordDigest == "" {
			return errors.New("evidence record is not sealed")
		}
		want, err := recordDigest(r)
		if err != nil {
			return err
		}
		if r.RecordDigest != want {
			return errors.New("evidence record digest does not match its contents")
		}
	}
	return nil
}

func validateEvidenceSecretSafety(record Record) error {
	if err := ValidateSecretSafeValue(record); err != nil {
		if errors.Is(err, ErrSecretMaterial) {
			return errors.New("evidence contains a secret-like assignment")
		}
		return fmt.Errorf("evidence secret-safety check: %w", err)
	}
	return nil
}

func validateCell(r Record) error {
	if r.CellID == "" || r.CellID != r.Dimensions.ID() {
		return errors.New("cell evidence has an invalid stable cell ID")
	}
	if err := validateDimensions(r.Dimensions); err != nil {
		return err
	}
	if r.Status == StatusPass {
		if !IsImmutableImageDigest(r.Artifact.ImageDigest) {
			return errors.New("PASS evidence requires an immutable image digest")
		}
		if err := validateRuntimeContract(r.Artifact); err != nil {
			return err
		}
		if err := validateSessionProof(r.Session); err != nil {
			return err
		}
		if r.Dimensions.Edition == "commerce" && (!r.Artifact.ComposerCredentialsConfigured || r.Artifact.ComposerCredentialScheme == "") {
			return errors.New("PASS Adobe Commerce evidence requires Composer credentials configured through a secret reference")
		}
	}
	if r.Dimensions.Edition == "commerce" && r.Artifact.ComposerCredentialsConfigured && !validCredentialScheme(r.Artifact.ComposerCredentialScheme) {
		return fmt.Errorf("unsupported Composer credential scheme %q", r.Artifact.ComposerCredentialScheme)
	}
	if err := validateOptionalProofs(r); err != nil {
		return err
	}
	if r.Status != StatusPass && strings.TrimSpace(r.Reason) == "" {
		return errors.New("non-PASS evidence requires a reason")
	}
	return nil
}

func validateOptionalProofs(r Record) error {
	if r.Cost.DurationSeconds < 0 || r.Cost.CloudOperationSeconds < 0 || r.Cost.ResourceLifetimeSeconds < 0 || r.Cost.EstimatedCents < 0 {
		return errors.New("evidence cost proof contains a negative value")
	}
	if r.Cleanup.LatencySeconds < 0 {
		return errors.New("cleanup proof latency cannot be negative")
	}
	if !isZeroArchitectureProof(r.Architecture) {
		if strings.TrimSpace(r.Architecture.ProfileID) == "" || !fingerprintPattern.MatchString(r.Architecture.Fingerprint) || strings.TrimSpace(r.Architecture.Provider) == "" || strings.TrimSpace(r.Architecture.Runtime) == "" || strings.TrimSpace(r.Architecture.Region) == "" || strings.TrimSpace(r.Architecture.ComputeMode) == "" || strings.TrimSpace(r.Architecture.ResilienceProfileID) == "" {
			return errors.New("architecture proof is incomplete")
		}
		if !IsImmutableImageDigest(r.Architecture.ArtifactDigest) {
			return errors.New("architecture proof requires an immutable artifact digest")
		}
	}
	for _, proof := range r.Resilience {
		if strings.TrimSpace(proof.DataClass) == "" || strings.TrimSpace(proof.Action) == "" || !validEvidenceStatus(proof.Status) {
			return errors.New("resilience proof is incomplete")
		}
		if !validResilienceAction(proof.Action) {
			return fmt.Errorf("unsupported resilience proof action %q", proof.Action)
		}
		if proof.Status == StatusPass && strings.TrimSpace(proof.OperationID) == "" && strings.TrimSpace(proof.BackupID) == "" && strings.TrimSpace(proof.RestoreID) == "" && strings.TrimSpace(proof.IntegrityDigest) == "" {
			return fmt.Errorf("PASS resilience proof for %s requires an operation or integrity identity", proof.DataClass)
		}
		if proof.MeasuredRPOSeconds < 0 || proof.MeasuredRTOSeconds < 0 {
			return fmt.Errorf("resilience proof for %s contains a negative measured duration", proof.DataClass)
		}
		if proof.RetentionDays < 0 || proof.RestoreDurationSeconds < 0 {
			return fmt.Errorf("resilience proof for %s contains a negative retention or restore duration", proof.DataClass)
		}
		if proof.Status == StatusPass && (proof.Action == "fence" || proof.Action == "failover" || proof.Action == "failback") {
			if strings.TrimSpace(proof.WriterIdentityRef) == "" || strings.TrimSpace(proof.WriterEpoch) == "" || !proof.SingleWriterVerified || !proof.SplitBrainAbsent {
				return fmt.Errorf("PASS %s proof for %s requires writer identity, epoch, single-writer, and split-brain verification", proof.Action, proof.DataClass)
			}
			if (proof.Action == "failover" || proof.Action == "failback") && !proof.StaleOriginRejected {
				return fmt.Errorf("PASS %s proof for %s requires stale-origin rejection", proof.Action, proof.DataClass)
			}
			if !proof.ApprovalVerified {
				return fmt.Errorf("PASS %s proof for %s requires approved-transition verification", proof.Action, proof.DataClass)
			}
			if proof.Action == "failback" && !proof.ReconciliationVerified {
				return fmt.Errorf("PASS failback proof for %s requires reconciliation verification", proof.DataClass)
			}
		}
		if proof.Status == StatusPass && proof.Action == "backup" {
			if proof.RetentionDays <= 0 || !proof.EncryptionVerified || !proof.ProtectionVerified || !proof.OwnershipVerified {
				return fmt.Errorf("PASS backup proof for %s requires retention, encryption, protection, and ownership verification", proof.DataClass)
			}
		}
		if proof.Status == StatusPass && proof.Action == "restore" {
			if strings.TrimSpace(proof.FixtureID) == "" || proof.RestoreDurationSeconds <= 0 || !proof.ManifestVerified || !proof.CountsVerified || !proof.ApplicationReadsVerified || !proof.PermissionsVerified || !proof.SecretReferencesVerified || !proof.ServiceHealthVerified {
				return fmt.Errorf("PASS restore proof for %s requires fixture, manifest, counts, application-read, permission, secret-reference, health, and duration verification", proof.DataClass)
			}
		}
		if proof.Status != StatusPass && strings.TrimSpace(proof.Reason) == "" {
			return fmt.Errorf("non-PASS resilience proof for %s requires a reason", proof.DataClass)
		}
	}
	for _, proof := range r.Failures {
		if strings.TrimSpace(proof.ScenarioID) == "" || !validEvidenceStatus(proof.Status) {
			return errors.New("failure scenario proof is incomplete")
		}
		if (proof.Status == StatusPass || proof.Status == StatusFail) && !proof.InjectionVerified {
			return fmt.Errorf("%s failure scenario proof for %s requires injection verification", proof.Status, proof.ScenarioID)
		}
		if proof.MeasuredRPOSeconds < 0 || proof.MeasuredRTOSeconds < 0 {
			return fmt.Errorf("failure scenario proof for %s contains a negative measured duration", proof.ScenarioID)
		}
		if proof.Status != StatusPass && strings.TrimSpace(proof.Reason) == "" {
			return fmt.Errorf("non-PASS failure scenario proof for %s requires a reason", proof.ScenarioID)
		}
	}
	for _, proof := range r.Observability {
		if strings.TrimSpace(proof.Provider) == "" || !validEvidenceStatus(proof.Status) {
			return errors.New("observability proof is incomplete")
		}
		if proof.ObjectType != "" && proof.ObjectType != "signal" && proof.ObjectType != "alert" && proof.ObjectType != "dashboard" && proof.ObjectType != "slo" {
			return fmt.Errorf("unsupported observability proof object type %q", proof.ObjectType)
		}
		if proof.ObjectType == "" || proof.ObjectType == "signal" {
			if strings.TrimSpace(proof.Signal) == "" {
				return errors.New("signal observability proof requires a signal")
			}
		} else if strings.TrimSpace(proof.ObjectID) == "" {
			return fmt.Errorf("%s observability proof requires an object ID", proof.ObjectType)
		}
		if proof.Status == StatusPass && strings.TrimSpace(proof.Identity) == "" {
			name := proof.Signal
			if proof.ObjectType != "" && proof.ObjectType != "signal" {
				name = proof.ObjectType + "/" + proof.ObjectID
			}
			return fmt.Errorf("PASS observability proof for %s requires an identity", name)
		}
		if proof.RetentionDays < 0 {
			return fmt.Errorf("observability proof for %s has negative retention", proof.Signal)
		}
		if proof.Status != StatusPass && strings.TrimSpace(proof.Reason) == "" {
			return fmt.Errorf("non-PASS observability proof for %s requires a reason", proof.Signal)
		}
	}
	for _, proof := range r.Edge {
		if strings.TrimSpace(proof.Provider) == "" || strings.TrimSpace(proof.Feature) == "" || !validEvidenceStatus(proof.Status) {
			return errors.New("edge proof is incomplete")
		}
		if proof.Status == StatusPass && strings.TrimSpace(proof.Identity) == "" {
			return fmt.Errorf("PASS edge proof for %s requires an identity", proof.Feature)
		}
		if proof.Status == StatusPass && !edgeFeatureVerified(proof, proof.Feature) {
			return fmt.Errorf("PASS edge proof for %s is missing its verification flag", proof.Feature)
		}
		if proof.Status != StatusPass && strings.TrimSpace(proof.Reason) == "" {
			return fmt.Errorf("non-PASS edge proof for %s requires a reason", proof.Feature)
		}
	}
	for _, proof := range r.Sources {
		if strings.TrimSpace(proof.Claim) == "" || strings.TrimSpace(proof.URL) == "" || strings.TrimSpace(proof.RetrievedAt) == "" || !validEvidenceStatus(proof.Status) {
			return errors.New("source proof is incomplete")
		}
		if !strings.HasPrefix(proof.URL, "https://") {
			return fmt.Errorf("source proof URL must use HTTPS: %q", proof.URL)
		}
		if _, err := parseCatalogDate(proof.RetrievedAt); err != nil {
			return fmt.Errorf("source proof retrievedAt: %w", err)
		}
	}
	return nil
}

func isZeroArchitectureProof(proof ArchitectureProof) bool {
	return proof == (ArchitectureProof{})
}

func validEvidenceStatus(status Status) bool {
	switch status {
	case StatusPass, StatusFail, StatusSkip, StatusBlocked, StatusPending, StatusNotRun:
		return true
	default:
		return false
	}
}

func validResilienceAction(action string) bool {
	switch action {
	case "backup", "restore", "integrity-check", "failover", "failback", "fence", "cleanup":
		return true
	default:
		return false
	}
}

func validateRuntimeContract(artifact ArtifactProof) error {
	if !versionPattern.MatchString(artifact.PHPVersion) {
		return errors.New("PASS evidence requires a resolved PHP version")
	}
	if !versionPattern.MatchString(artifact.ComposerVersion) {
		return errors.New("PASS evidence requires a resolved Composer version")
	}
	if artifact.PHPExtensions == nil {
		return errors.New("PASS evidence requires the resolved PHP extension set")
	}
	seen := make(map[string]struct{}, len(artifact.PHPExtensions))
	for _, extension := range artifact.PHPExtensions {
		if !phpExtensionPattern.MatchString(extension) {
			return fmt.Errorf("PASS evidence contains an invalid PHP extension %q", extension)
		}
		if _, exists := seen[extension]; exists {
			return fmt.Errorf("PASS evidence contains duplicate PHP extension %q", extension)
		}
		seen[extension] = struct{}{}
	}
	return nil
}

func validateSessionProof(session SessionProof) error {
	if session.Mode != "baseline" && session.Mode != "reused" {
		return errors.New("PASS evidence requires a baseline or reused session mode")
	}
	if strings.TrimSpace(session.StackID) == "" {
		return errors.New("PASS evidence requires a warm-session stack ID")
	}
	if !fingerprintPattern.MatchString(session.Fingerprint) {
		return errors.New("PASS evidence requires a SHA-256 warm-session fingerprint")
	}
	boundary := map[string]string{
		"artifact digest":       session.ArtifactDigest,
		"fixture ID":            session.FixtureID,
		"backup set":            session.BackupSet,
		"observability setup":   session.ObservabilitySetup,
		"edge setup":            session.EdgeSetup,
		"schema fingerprint":    session.SchemaFingerprint,
		"migration fingerprint": session.MigrationFingerprint,
		"state backend":         session.StateBackend,
	}
	anyBoundary := false
	for _, value := range boundary {
		if strings.TrimSpace(value) != "" {
			anyBoundary = true
			break
		}
	}
	if !anyBoundary {
		return nil
	}
	if !IsImmutableImageDigest(session.ArtifactDigest) {
		return errors.New("PASS evidence boundary proof requires an immutable artifact digest")
	}
	for name, value := range boundary {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("PASS evidence boundary proof requires a single-line %s", name)
		}
	}
	return nil
}

func validateCleanup(r Record) error {
	if r.Cleanup.LatencySeconds < 0 {
		return errors.New("cleanup proof latency cannot be negative")
	}
	if r.CellID != "" {
		return errors.New("cleanup evidence must not have a cell ID")
	}
	if r.Cleanup.Status != StatusPass && r.Cleanup.Status != StatusFail && r.Cleanup.Status != StatusSkip && r.Cleanup.Status != StatusPending && r.Cleanup.Status != StatusNotRun {
		return fmt.Errorf("unsupported cleanup status %q", r.Cleanup.Status)
	}
	if strings.TrimSpace(r.Cleanup.Prefix) == "" || strings.TrimSpace(r.Cleanup.OwnershipMarker) == "" {
		return errors.New("cleanup evidence requires a prefix and ownership marker")
	}
	if r.Cleanup.Status == StatusPass && !cleanupHasNoLiveResources(r.Cleanup) {
		return errors.New("PASS cleanup evidence cannot list remaining resources")
	}
	if r.Cleanup.Status != StatusPass && strings.TrimSpace(r.Reason) == "" {
		return errors.New("non-PASS cleanup evidence requires a reason")
	}
	return nil
}

func cleanupHasNoLiveResources(cleanup CleanupProof) bool {
	return len(cleanup.Remaining) == 0 && len(cleanup.Live) == 0 && len(cleanup.Delayed) == 0
}

func validateDimensions(d Dimensions) error {
	values := map[string]string{
		"provider": d.Provider, "runtime": d.Runtime, "computeMode": d.ComputeMode, "kubernetesMode": d.KubernetesMode, "release": d.Release,
		"edition": d.Edition, "preset": d.Preset, "database": d.Database,
		"search": d.Search, "queue": d.Queue, "cache": d.Cache,
		"webCache": d.WebCache, "edge": d.Edge, "scenario": d.Scenario,
	}
	for name, value := range values {
		if value == "" || !cellPartPattern.MatchString(value) {
			return fmt.Errorf("evidence dimension %s is invalid", name)
		}
	}
	if d.Edition != "open-source" && d.Edition != "commerce" {
		return fmt.Errorf("unsupported evidence edition %q", d.Edition)
	}
	return nil
}

func recordDigest(r Record) (string, error) {
	canonical := map[string]any{
		"version": r.Version,
		"type":    r.Type,
		"runId":   r.RunID,
		"status":  r.Status,
		"provenance": map[string]any{
			"generatedBy": r.Provenance.GeneratedBy,
			"generatedAt": r.Provenance.GeneratedAt,
			"source":      r.Provenance.Source,
			"runId":       r.Provenance.RunID,
		},
	}
	if r.Type == RecordCell {
		canonical["cellId"] = r.CellID
		canonical["dimensions"] = map[string]any{
			"provider": r.Dimensions.Provider, "runtime": r.Dimensions.Runtime,
			"computeMode": r.Dimensions.ComputeMode, "kubernetesMode": r.Dimensions.KubernetesMode,
			"release": r.Dimensions.Release, "edition": r.Dimensions.Edition,
			"preset": r.Dimensions.Preset, "database": r.Dimensions.Database,
			"search": r.Dimensions.Search, "queue": r.Dimensions.Queue,
			"cache": r.Dimensions.Cache, "webCache": r.Dimensions.WebCache,
			"edge": r.Dimensions.Edge, "scenario": r.Dimensions.Scenario,
		}
		canonical["artifact"] = map[string]any{
			"composerCredentialsConfigured": r.Artifact.ComposerCredentialsConfigured,
			"phpVersion":                    r.Artifact.PHPVersion,
			"phpExtensions":                 sortedStrings(r.Artifact.PHPExtensions),
			"composerVersion":               r.Artifact.ComposerVersion,
		}
		if r.Artifact.ImageDigest != "" {
			canonical["artifact"].(map[string]any)["imageDigest"] = r.Artifact.ImageDigest
		}
		if r.Artifact.ComposerCredentialScheme != "" {
			canonical["artifact"].(map[string]any)["composerCredentialScheme"] = r.Artifact.ComposerCredentialScheme
		}
		session := map[string]any{
			"mode":           r.Session.Mode,
			"stackId":        r.Session.StackID,
			"fingerprint":    r.Session.Fingerprint,
			"migrationOwner": r.Session.MigrationOwner,
		}
		for name, value := range map[string]string{
			"artifactDigest":       r.Session.ArtifactDigest,
			"fixtureId":            r.Session.FixtureID,
			"backupSet":            r.Session.BackupSet,
			"observabilitySetup":   r.Session.ObservabilitySetup,
			"edgeSetup":            r.Session.EdgeSetup,
			"schemaFingerprint":    r.Session.SchemaFingerprint,
			"migrationFingerprint": r.Session.MigrationFingerprint,
			"stateBackend":         r.Session.StateBackend,
		} {
			if value != "" {
				session[name] = value
			}
		}
		canonical["session"] = session
		if !isZeroArchitectureProof(r.Architecture) {
			canonical["architecture"] = map[string]any{
				"profileId": r.Architecture.ProfileID, "fingerprint": r.Architecture.Fingerprint,
				"provider": r.Architecture.Provider, "runtime": r.Architecture.Runtime,
				"region": r.Architecture.Region, "computeMode": r.Architecture.ComputeMode,
				"resilienceProfileId": r.Architecture.ResilienceProfileID,
				"artifactDigest":      r.Architecture.ArtifactDigest,
			}
		}
		if len(r.Resilience) > 0 {
			canonical["resilience"] = canonicalResilienceProofs(r.Resilience)
		}
		if len(r.Failures) > 0 {
			canonical["failures"] = canonicalFailureScenarioProofs(r.Failures)
		}
		if len(r.Observability) > 0 {
			canonical["observability"] = canonicalObservabilityProofs(r.Observability)
		}
		if len(r.Edge) > 0 {
			canonical["edge"] = canonicalEdgeProofs(r.Edge)
		}
		if len(r.Sources) > 0 {
			canonical["sources"] = canonicalSourceProofs(r.Sources)
		}
		if r.Required {
			canonical["required"] = true
		}
	} else {
		canonical["cleanup"] = map[string]any{"status": r.Cleanup.Status, "prefix": r.Cleanup.Prefix, "ownershipMarker": r.Cleanup.OwnershipMarker}
		if len(r.Cleanup.Remaining) > 0 {
			canonical["cleanup"].(map[string]any)["remaining"] = r.Cleanup.Remaining
		}
		for name, values := range map[string][]string{"live": r.Cleanup.Live, "delayed": r.Cleanup.Delayed, "protected": r.Cleanup.Protected, "metadata": r.Cleanup.Metadata} {
			if len(values) > 0 {
				canonical["cleanup"].(map[string]any)[name] = values
			}
		}
		if r.Cleanup.CheckedAt != "" {
			canonical["cleanup"].(map[string]any)["checkedAt"] = r.Cleanup.CheckedAt
		}
		if r.Cleanup.LatencySeconds != 0 {
			canonical["cleanup"].(map[string]any)["latencySeconds"] = r.Cleanup.LatencySeconds
		}
	}
	if r.Provenance.AccountRef != "" {
		canonical["provenance"].(map[string]any)["accountRef"] = r.Provenance.AccountRef
	}
	if r.Provenance.Certificate != "" {
		canonical["provenance"].(map[string]any)["certificate"] = r.Provenance.Certificate
	}
	if r.Cost.DurationSeconds != 0 || r.Cost.CloudOperationSeconds != 0 || r.Cost.ResourceLifetimeSeconds != 0 || r.Cost.EstimatedCents != 0 || r.Cost.Currency != "" {
		cost := map[string]any{}
		if r.Cost.DurationSeconds != 0 {
			cost["durationSeconds"] = r.Cost.DurationSeconds
		}
		if r.Cost.CloudOperationSeconds != 0 {
			cost["cloudOperationSeconds"] = r.Cost.CloudOperationSeconds
		}
		if r.Cost.ResourceLifetimeSeconds != 0 {
			cost["resourceLifetimeSeconds"] = r.Cost.ResourceLifetimeSeconds
		}
		if r.Cost.EstimatedCents != 0 {
			cost["estimatedCents"] = r.Cost.EstimatedCents
		}
		if r.Cost.Currency != "" {
			cost["currency"] = r.Cost.Currency
		}
		canonical["cost"] = cost
	}
	if r.Reason != "" {
		canonical["reason"] = r.Reason
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("canonicalize evidence record: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func canonicalResilienceProofs(proofs []ResilienceProof) []map[string]any {
	copyProofs := append([]ResilienceProof(nil), proofs...)
	sort.Slice(copyProofs, func(i, j int) bool {
		if copyProofs[i].DataClass != copyProofs[j].DataClass {
			return copyProofs[i].DataClass < copyProofs[j].DataClass
		}
		return copyProofs[i].Action < copyProofs[j].Action
	})
	result := make([]map[string]any, 0, len(copyProofs))
	for _, proof := range copyProofs {
		result = append(result, map[string]any{
			"dataClass": proof.DataClass, "action": proof.Action, "status": proof.Status,
			"destination": proof.Destination, "operationId": proof.OperationID,
			"backupId": proof.BackupID, "restoreId": proof.RestoreID, "fixtureId": proof.FixtureID,
			"integrityDigest": proof.IntegrityDigest, "retentionDays": proof.RetentionDays,
			"encryptionVerified": proof.EncryptionVerified, "protectionVerified": proof.ProtectionVerified,
			"ownershipVerified": proof.OwnershipVerified, "manifestVerified": proof.ManifestVerified,
			"countsVerified": proof.CountsVerified, "applicationReadsVerified": proof.ApplicationReadsVerified,
			"permissionsVerified": proof.PermissionsVerified, "secretReferencesVerified": proof.SecretReferencesVerified,
			"serviceHealthVerified": proof.ServiceHealthVerified, "restoreDurationSeconds": proof.RestoreDurationSeconds,
			"writerIdentityRef": proof.WriterIdentityRef, "writerEpoch": proof.WriterEpoch,
			"singleWriterVerified": proof.SingleWriterVerified, "splitBrainAbsent": proof.SplitBrainAbsent,
			"staleOriginRejected": proof.StaleOriginRejected, "reconciliationVerified": proof.ReconciliationVerified,
			"approvalVerified":   proof.ApprovalVerified,
			"measuredRpoSeconds": proof.MeasuredRPOSeconds, "measuredRtoSeconds": proof.MeasuredRTOSeconds,
			"operatorAction": proof.OperatorAction, "reason": proof.Reason,
		})
	}
	return result
}

func canonicalFailureScenarioProofs(proofs []FailureScenarioProof) []map[string]any {
	copyProofs := append([]FailureScenarioProof(nil), proofs...)
	sort.Slice(copyProofs, func(i, j int) bool { return copyProofs[i].ScenarioID < copyProofs[j].ScenarioID })
	result := make([]map[string]any, 0, len(copyProofs))
	for _, proof := range copyProofs {
		result = append(result, map[string]any{
			"scenarioId": proof.ScenarioID, "status": proof.Status,
			"injectionVerified": proof.InjectionVerified, "trafficHealthVerified": proof.TrafficHealthVerified,
			"queueHealthVerified": proof.QueueHealthVerified, "databaseHealthVerified": proof.DatabaseHealthVerified,
			"cacheLossClassified": proof.CacheLossClassified, "integrityVerified": proof.IntegrityVerified,
			"fencingVerified": proof.FencingVerified, "measuredRpoSeconds": proof.MeasuredRPOSeconds,
			"measuredRtoSeconds": proof.MeasuredRTOSeconds, "operatorAction": proof.OperatorAction, "reason": proof.Reason,
		})
	}
	return result
}

func canonicalObservabilityProofs(proofs []ObservabilityProof) []map[string]any {
	copyProofs := append([]ObservabilityProof(nil), proofs...)
	sort.Slice(copyProofs, func(i, j int) bool {
		if copyProofs[i].Provider != copyProofs[j].Provider {
			return copyProofs[i].Provider < copyProofs[j].Provider
		}
		if copyProofs[i].ObjectType != copyProofs[j].ObjectType {
			return copyProofs[i].ObjectType < copyProofs[j].ObjectType
		}
		if copyProofs[i].ObjectID != copyProofs[j].ObjectID {
			return copyProofs[i].ObjectID < copyProofs[j].ObjectID
		}
		return copyProofs[i].Signal < copyProofs[j].Signal
	})
	result := make([]map[string]any, 0, len(copyProofs))
	for _, proof := range copyProofs {
		result = append(result, map[string]any{
			"provider": proof.Provider, "objectType": proof.ObjectType, "objectId": proof.ObjectID,
			"signal": proof.Signal, "status": proof.Status,
			"identity": proof.Identity, "retentionDays": proof.RetentionDays,
			"labelsVerified": proof.LabelsVerified, "redactionVerified": proof.RedactionVerified,
			"alertVerified": proof.AlertVerified, "reason": proof.Reason,
		})
	}
	return result
}

func canonicalEdgeProofs(proofs []EdgeProof) []map[string]any {
	copyProofs := append([]EdgeProof(nil), proofs...)
	sort.Slice(copyProofs, func(i, j int) bool {
		if copyProofs[i].Provider != copyProofs[j].Provider {
			return copyProofs[i].Provider < copyProofs[j].Provider
		}
		return copyProofs[i].Feature < copyProofs[j].Feature
	})
	result := make([]map[string]any, 0, len(copyProofs))
	for _, proof := range copyProofs {
		result = append(result, map[string]any{
			"provider": proof.Provider, "feature": proof.Feature, "status": proof.Status,
			"identity": proof.Identity, "originHealthVerified": proof.OriginHealthVerified,
			"tlsVerified": proof.TLSVerified, "purgeVerified": proof.PurgeVerified,
			"routeVerified": proof.RouteVerified, "dnsOwnershipVerified": proof.DNSOwnershipVerified,
			"cachePolicyVerified": proof.CachePolicyVerified, "wafVerified": proof.WAFVerified,
			"failoverVerified": proof.FailoverVerified, "rollbackVerified": proof.RollbackVerified,
			"reason": proof.Reason,
		})
	}
	return result
}

func canonicalSourceProofs(proofs []SourceProof) []map[string]any {
	copyProofs := append([]SourceProof(nil), proofs...)
	sort.Slice(copyProofs, func(i, j int) bool { return copyProofs[i].Claim < copyProofs[j].Claim })
	result := make([]map[string]any, 0, len(copyProofs))
	for _, proof := range copyProofs {
		result = append(result, map[string]any{
			"claim": proof.Claim, "url": proof.URL, "retrievedAt": proof.RetrievedAt, "status": proof.Status,
		})
	}
	return result
}

func IsImmutableImageDigest(value string) bool {
	return imageDigestPattern.MatchString(value)
}

func validCredentialScheme(value string) bool {
	switch value {
	case "aws-secrets-manager", "ssm", "gcp-secret-manager":
		return true
	default:
		return false
	}
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func sortedStrings(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}
