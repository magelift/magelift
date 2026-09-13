package certification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

// ReuseFingerprint is the complete boundary identity required before a
// provisioned stack, fixture, backup set, telemetry setup, or edge setup can
// be reused by another certification cell. The digest is derived from all
// fields; callers cannot opt out of one boundary by using a partial key.
type ReuseFingerprint struct {
	Architecture         string `json:"architecture" yaml:"architecture"`
	Fixture              string `json:"fixture" yaml:"fixture"`
	BackupSet            string `json:"backupSet" yaml:"backupSet"`
	Observability        string `json:"observability" yaml:"observability"`
	Edge                 string `json:"edge" yaml:"edge"`
	ArtifactDigest       string `json:"artifactDigest" yaml:"artifactDigest"`
	SchemaFingerprint    string `json:"schemaFingerprint" yaml:"schemaFingerprint"`
	MigrationFingerprint string `json:"migrationFingerprint" yaml:"migrationFingerprint"`
	OwnershipMarker      string `json:"ownershipMarker" yaml:"ownershipMarker"`
	StateBackend         string `json:"stateBackend" yaml:"stateBackend"`
}

func (fingerprint ReuseFingerprint) Validate() error {
	if err := ValidateSecretSafeValue(fingerprint); err != nil {
		return errors.New("reuse fingerprint contains secret-like material")
	}
	for name, value := range map[string]string{
		"architecture":          fingerprint.Architecture,
		"fixture":               fingerprint.Fixture,
		"backup set":            fingerprint.BackupSet,
		"observability":         fingerprint.Observability,
		"edge":                  fingerprint.Edge,
		"artifact digest":       fingerprint.ArtifactDigest,
		"schema fingerprint":    fingerprint.SchemaFingerprint,
		"migration fingerprint": fingerprint.MigrationFingerprint,
		"ownership marker":      fingerprint.OwnershipMarker,
		"state backend":         fingerprint.StateBackend,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("reuse fingerprint %s is required and must be single-line", name)
		}
	}
	return nil
}

func (fingerprint ReuseFingerprint) Digest() (string, error) {
	if err := fingerprint.Validate(); err != nil {
		return "", err
	}
	data, err := json.Marshal(fingerprint)
	if err != nil {
		return "", fmt.Errorf("encode reuse fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

type SessionState string

const (
	SessionReady           SessionState = "ready"
	SessionPendingDelete   SessionState = "pending-delete"
	SessionCleanupVerified SessionState = "cleanup-verified"
	SessionQuarantined     SessionState = "quarantined"
)

// SessionRecord is the only data a certification run needs to reuse a
// provisioned scope. All fields are identities or digests; secret values and
// provider SDK objects are intentionally impossible to store here.
type SessionRecord struct {
	SessionID       string           `json:"sessionId" yaml:"sessionId"`
	Fingerprint     ReuseFingerprint `json:"fingerprint" yaml:"fingerprint"`
	FingerprintHash string           `json:"fingerprintHash" yaml:"fingerprintHash"`
	StackID         string           `json:"stackId" yaml:"stackId"`
	WriterID        string           `json:"writerId" yaml:"writerId"`
	ClaimOwner      string           `json:"claimOwner,omitempty" yaml:"claimOwner,omitempty"`
	State           SessionState     `json:"state" yaml:"state"`
	Live            bool             `json:"live" yaml:"live"`
	CleanupComplete bool             `json:"cleanupComplete" yaml:"cleanupComplete"`
	OperationIDs    []string         `json:"operationIds,omitempty" yaml:"operationIds,omitempty"`
	ResourceRefs    []string         `json:"resourceRefs,omitempty" yaml:"resourceRefs,omitempty"`
	FixtureRefs     []string         `json:"fixtureRefs,omitempty" yaml:"fixtureRefs,omitempty"`
	MigrationRefs   []string         `json:"migrationRefs,omitempty" yaml:"migrationRefs,omitempty"`
	BackupRefs      []string         `json:"backupRefs,omitempty" yaml:"backupRefs,omitempty"`
	TelemetryRefs   []string         `json:"telemetryRefs,omitempty" yaml:"telemetryRefs,omitempty"`
	EdgeRefs        []string         `json:"edgeRefs,omitempty" yaml:"edgeRefs,omitempty"`
	PendingRefs     []string         `json:"pendingRefs,omitempty" yaml:"pendingRefs,omitempty"`
}

func (record SessionRecord) Validate() error {
	if err := ValidateSecretSafeValue(record); err != nil {
		return errors.New("session registry record contains secret-like material")
	}
	for name, value := range map[string]string{
		"session ID":  record.SessionID,
		"stack ID":    record.StackID,
		"writer ID":   record.WriterID,
		"claim owner": record.ClaimOwner,
	} {
		if value == "" {
			if name == "claim owner" {
				continue
			}
			return fmt.Errorf("session registry %s is required and must be single-line", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("session registry %s is required and must be single-line", name)
		}
	}
	if err := record.Fingerprint.Validate(); err != nil {
		return err
	}
	digest, err := record.Fingerprint.Digest()
	if err != nil {
		return err
	}
	if record.FingerprintHash != digest {
		return errors.New("session registry fingerprint hash does not match fingerprint")
	}
	switch record.State {
	case SessionReady:
		if !record.Live || record.CleanupComplete || len(record.PendingRefs) > 0 {
			return errors.New("ready session registry record must be live and not pending cleanup")
		}
	case SessionPendingDelete:
		if !record.Live || record.CleanupComplete || len(record.PendingRefs) == 0 {
			return errors.New("pending-delete session registry record requires live pending resources")
		}
	case SessionCleanupVerified:
		if record.Live || !record.CleanupComplete || len(record.PendingRefs) > 0 {
			return errors.New("cleanup-verified session registry record must be non-live and complete")
		}
	case SessionQuarantined:
		if record.CleanupComplete && record.Live {
			return errors.New("quarantined session registry record cannot be live and cleanup-complete")
		}
	default:
		return fmt.Errorf("invalid session registry state %q", record.State)
	}
	for name, values := range map[string][]string{
		"operation ID":        record.OperationIDs,
		"resource reference":  record.ResourceRefs,
		"fixture reference":   record.FixtureRefs,
		"migration reference": record.MigrationRefs,
		"backup reference":    record.BackupRefs,
		"telemetry reference": record.TelemetryRefs,
		"edge reference":      record.EdgeRefs,
		"pending reference":   record.PendingRefs,
	} {
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
				return fmt.Errorf("session registry %s must be non-empty and single-line", name)
			}
			if _, exists := seen[value]; exists {
				return fmt.Errorf("session registry %s %q is duplicated", name, value)
			}
			seen[value] = struct{}{}
		}
	}
	if record.State == SessionReady || record.State == SessionPendingDelete {
		for name, values := range map[string][]string{
			"resource reference":  record.ResourceRefs,
			"fixture reference":   record.FixtureRefs,
			"migration reference": record.MigrationRefs,
			"backup reference":    record.BackupRefs,
		} {
			if len(values) == 0 {
				return fmt.Errorf("live session registry record requires at least one %s", name)
			}
		}
		if record.Fingerprint.Observability != "none" && len(record.TelemetryRefs) == 0 {
			return errors.New("live session registry record requires telemetry references for an enabled observability destination")
		}
		if record.Fingerprint.Edge != "none" && len(record.EdgeRefs) == 0 {
			return errors.New("live session registry record requires edge references for an enabled edge destination")
		}
	}
	return nil
}

// SessionRegistry is implemented by a durable state-backed store in a live
// certification environment. The in-memory implementation below is only for
// deterministic local tests.
type SessionRegistry interface {
	Lookup(context.Context, ReuseFingerprint) (SessionRecord, bool, error)
	// Claim atomically reserves a ready session for one certification owner.
	// A second owner must receive ReuseBlocked rather than racing a later Put.
	Claim(context.Context, ReuseFingerprint, string) (ReuseLookup, error)
	ReleaseClaim(context.Context, ReuseFingerprint, string) error
	Put(context.Context, SessionRecord) error
	MarkPendingDeletion(context.Context, ReuseFingerprint, []string) error
	MarkCleanupVerified(context.Context, ReuseFingerprint, string) error
}

type ReuseDecision string

const (
	ReuseReady   ReuseDecision = "ready"
	ReuseMiss    ReuseDecision = "miss"
	ReuseBlocked ReuseDecision = "blocked"
)

type ReuseLookup struct {
	Decision ReuseDecision
	Record   SessionRecord
	Reason   string
}

// FindReusableSession returns a ready session only when the complete
// fingerprint matches. A pending or quarantined record is a hard block so the
// caller cannot create a duplicate writer while deletion or investigation is
// unfinished.
func FindReusableSession(ctx context.Context, registry SessionRegistry, fingerprint ReuseFingerprint) (ReuseLookup, error) {
	if ctx == nil {
		return ReuseLookup{}, errors.New("session registry context is required")
	}
	if registry == nil || isNilSessionRegistry(registry) {
		return ReuseLookup{}, errors.New("session registry is required")
	}
	if err := fingerprint.Validate(); err != nil {
		return ReuseLookup{}, err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return ReuseLookup{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReuseLookup{}, err
	}
	record, found, err := registry.Lookup(ctx, fingerprint)
	if err != nil {
		return ReuseLookup{}, err
	}
	if !found {
		return ReuseLookup{Decision: ReuseMiss, Reason: "no compatible session is registered"}, nil
	}
	if err := record.Validate(); err != nil {
		return ReuseLookup{}, errors.New("session registry returned an invalid record")
	}
	if recordDigest, err := record.Fingerprint.Digest(); err != nil || recordDigest != digest {
		return ReuseLookup{}, errors.New("session registry returned an incompatible record")
	}
	switch record.State {
	case SessionReady:
		return ReuseLookup{Decision: ReuseReady, Record: record}, nil
	case SessionPendingDelete:
		return ReuseLookup{Decision: ReuseBlocked, Record: record, Reason: "compatible session has pending owning-service deletion"}, nil
	case SessionQuarantined:
		return ReuseLookup{Decision: ReuseBlocked, Record: record, Reason: "compatible session is quarantined"}, nil
	case SessionCleanupVerified:
		return ReuseLookup{Decision: ReuseMiss, Record: record, Reason: "compatible session was already cleaned up"}, nil
	default:
		return ReuseLookup{Decision: ReuseBlocked, Record: record, Reason: "compatible session has an unknown state"}, nil
	}
}

// ClaimReusableSession is the mutation-safe form of FindReusableSession. A
// scheduler must claim a reusable session before it emits a plan that can
// attach a writer to that session. The claim owner is an opaque run identity;
// it is not a credential and is never resolved by the provider.
func ClaimReusableSession(ctx context.Context, registry SessionRegistry, fingerprint ReuseFingerprint, owner string) (ReuseLookup, error) {
	if ctx == nil {
		return ReuseLookup{}, errors.New("session registry context is required")
	}
	if registry == nil || isNilSessionRegistry(registry) {
		return ReuseLookup{}, errors.New("session registry is required")
	}
	if strings.TrimSpace(owner) == "" || strings.ContainsAny(owner, "\r\n\x00") {
		return ReuseLookup{}, errors.New("session reuse claim owner is required and must be single-line")
	}
	if err := fingerprint.Validate(); err != nil {
		return ReuseLookup{}, err
	}
	if err := ctx.Err(); err != nil {
		return ReuseLookup{}, err
	}
	lookup, err := registry.Claim(ctx, fingerprint, owner)
	if err != nil {
		return ReuseLookup{}, err
	}
	switch lookup.Decision {
	case ReuseReady:
		if err := validateClaimedSessionRecord(lookup.Record, fingerprint, owner); err != nil {
			return ReuseLookup{}, err
		}
	case ReuseBlocked:
		if lookup.Record.SessionID != "" {
			if err := validateSessionRecordFingerprint(lookup.Record, fingerprint); err != nil {
				return ReuseLookup{}, err
			}
		}
	case ReuseMiss:
		// A miss is allowed to omit a record. A non-empty record is still
		// checked so a broken registry cannot smuggle an unrelated session
		// through a miss result.
		if lookup.Record.SessionID != "" {
			if err := validateSessionRecordFingerprint(lookup.Record, fingerprint); err != nil {
				return ReuseLookup{}, err
			}
		}
	default:
		return ReuseLookup{}, errors.New("session registry returned an unknown reuse decision")
	}
	return lookup, nil
}

func isNilSessionRegistry(registry SessionRegistry) bool {
	if registry == nil {
		return true
	}
	value := reflect.ValueOf(registry)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validateSessionRecordFingerprint(record SessionRecord, fingerprint ReuseFingerprint) error {
	if err := record.Validate(); err != nil {
		return errors.New("session registry returned an invalid record")
	}
	want, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	got, err := record.Fingerprint.Digest()
	if err != nil || got != want {
		return errors.New("session registry returned an incompatible record")
	}
	return nil
}

func validateClaimedSessionRecord(record SessionRecord, fingerprint ReuseFingerprint, owner string) error {
	if err := validateSessionRecordFingerprint(record, fingerprint); err != nil {
		return err
	}
	if record.ClaimOwner != owner {
		return errors.New("session registry returned an unclaimed reusable session")
	}
	return nil
}

// MemorySessionRegistry is concurrency-safe and deliberately strict so local
// tests exercise the same duplicate-writer and delayed-cleanup rules as a
// durable implementation.
type MemorySessionRegistry struct {
	mu      sync.RWMutex
	records map[string]SessionRecord
}

func NewMemorySessionRegistry() *MemorySessionRegistry {
	return &MemorySessionRegistry{records: make(map[string]SessionRecord)}
}

func (registry *MemorySessionRegistry) Lookup(ctx context.Context, fingerprint ReuseFingerprint) (SessionRecord, bool, error) {
	if ctx == nil {
		return SessionRecord{}, false, errors.New("session registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return SessionRecord{}, false, err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return SessionRecord{}, false, err
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	record, found := registry.records[digest]
	if !found {
		return SessionRecord{}, false, nil
	}
	return cloneSessionRecord(record), true, nil
}

func (registry *MemorySessionRegistry) Claim(ctx context.Context, fingerprint ReuseFingerprint, owner string) (ReuseLookup, error) {
	if ctx == nil {
		return ReuseLookup{}, errors.New("session registry context is required")
	}
	if strings.TrimSpace(owner) == "" || strings.ContainsAny(owner, "\r\n\x00") {
		return ReuseLookup{}, errors.New("session reuse claim owner is required and must be single-line")
	}
	if err := ctx.Err(); err != nil {
		return ReuseLookup{}, err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return ReuseLookup{}, err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	record, found := registry.records[digest]
	if !found {
		return ReuseLookup{Decision: ReuseMiss, Reason: "no compatible session is registered"}, nil
	}
	if err := record.Validate(); err != nil {
		return ReuseLookup{}, errors.New("session registry returned an invalid record")
	}
	switch record.State {
	case SessionReady:
		if record.ClaimOwner != "" && record.ClaimOwner != owner {
			return ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session is claimed by another certification run"}, nil
		}
		record.ClaimOwner = owner
		if err := registry.putLocked(record); err != nil {
			return ReuseLookup{}, err
		}
		return ReuseLookup{Decision: ReuseReady, Record: cloneSessionRecord(record)}, nil
	case SessionPendingDelete:
		return ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session has pending owning-service deletion"}, nil
	case SessionQuarantined:
		return ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session is quarantined"}, nil
	case SessionCleanupVerified:
		return ReuseLookup{Decision: ReuseMiss, Record: cloneSessionRecord(record), Reason: "compatible session was already cleaned up"}, nil
	default:
		return ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session has an unknown state"}, nil
	}
}

func (registry *MemorySessionRegistry) ReleaseClaim(ctx context.Context, fingerprint ReuseFingerprint, owner string) error {
	if ctx == nil {
		return errors.New("session registry context is required")
	}
	if strings.TrimSpace(owner) == "" || strings.ContainsAny(owner, "\r\n\x00") {
		return errors.New("session reuse claim owner is required and must be single-line")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	record, found := registry.records[digest]
	if !found {
		return errors.New("session registry claim identity was not found")
	}
	if record.ClaimOwner == "" {
		return nil
	}
	if record.ClaimOwner != owner {
		return errors.New("session registry claim owner does not match")
	}
	record.ClaimOwner = ""
	return registry.putLocked(record)
}

func (registry *MemorySessionRegistry) Put(ctx context.Context, record SessionRecord) error {
	if ctx == nil {
		return errors.New("session registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := record.Validate(); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.records == nil {
		registry.records = make(map[string]SessionRecord)
	}
	existing, found := registry.records[record.FingerprintHash]
	if found && existing.StackID != record.StackID && existing.Live {
		return errors.New("session registry rejects a duplicate live writer for the fingerprint")
	}
	registry.records[record.FingerprintHash] = cloneSessionRecord(record)
	return nil
}

func (registry *MemorySessionRegistry) MarkPendingDeletion(ctx context.Context, fingerprint ReuseFingerprint, pending []string) error {
	if ctx == nil {
		return errors.New("session registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(pending) == 0 {
		return errors.New("pending deletion requires owning-service resource references")
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	record, found := registry.records[digest]
	if !found {
		return errors.New("session registry record was not found")
	}
	record.State = SessionPendingDelete
	record.Live = true
	record.CleanupComplete = false
	record.ClaimOwner = ""
	record.PendingRefs = append([]string(nil), pending...)
	return registry.putLocked(record)
}

func (registry *MemorySessionRegistry) MarkCleanupVerified(ctx context.Context, fingerprint ReuseFingerprint, stackID string) error {
	if ctx == nil {
		return errors.New("session registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	record, found := registry.records[digest]
	if !found || record.StackID != stackID {
		return errors.New("session registry cleanup identity was not found")
	}
	if record.State != SessionPendingDelete {
		return errors.New("session registry cleanup verification requires pending deletion")
	}
	record.State = SessionCleanupVerified
	record.Live = false
	record.CleanupComplete = true
	record.ClaimOwner = ""
	record.PendingRefs = nil
	return registry.putLocked(record)
}

func (registry *MemorySessionRegistry) putLocked(record SessionRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	registry.records[record.FingerprintHash] = cloneSessionRecord(record)
	return nil
}

func cloneSessionRecord(record SessionRecord) SessionRecord {
	record.OperationIDs = append([]string(nil), record.OperationIDs...)
	record.ResourceRefs = append([]string(nil), record.ResourceRefs...)
	record.FixtureRefs = append([]string(nil), record.FixtureRefs...)
	record.MigrationRefs = append([]string(nil), record.MigrationRefs...)
	record.BackupRefs = append([]string(nil), record.BackupRefs...)
	record.TelemetryRefs = append([]string(nil), record.TelemetryRefs...)
	record.EdgeRefs = append([]string(nil), record.EdgeRefs...)
	record.PendingRefs = append([]string(nil), record.PendingRefs...)
	return record
}
