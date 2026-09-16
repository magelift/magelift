package certification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/magelift/magelift/sdk"
)

var artifactCompatibilityFingerprintPattern = regexp.MustCompile(`^[a-f0-9]{64}$`)

// ArtifactBuildRequest is the provider-neutral input to an immutable artifact
// builder. The builder may resolve its own build credentials, but those values
// never cross this boundary and are not part of the request.
type ArtifactBuildRequest struct {
	CompatibilityFingerprint string
	OwnershipMarker          string
}

func (request ArtifactBuildRequest) Validate() error {
	if !artifactCompatibilityFingerprintPattern.MatchString(request.CompatibilityFingerprint) {
		return errors.New("artifact compatibility fingerprint must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(request.OwnershipMarker) == "" || strings.ContainsAny(request.OwnershipMarker, "\r\n\x00") {
		return errors.New("artifact ownership marker is required and must be single-line")
	}
	if err := ValidateSecretSafeText(request.OwnershipMarker); err != nil {
		return errors.New("artifact ownership marker contains secret material")
	}
	return nil
}

// ImmutableArtifactRecord binds one published artifact to the compatibility
// boundary it was built for. Provider groups reuse this record by digest; they
// never rebuild or retag the image merely because their infrastructure differs.
type ImmutableArtifactRecord struct {
	CompatibilityFingerprint string                        `json:"compatibilityFingerprint" yaml:"compatibilityFingerprint"`
	Artifact                 sdk.ImmutableArtifactContract `json:"artifact" yaml:"artifact"`
}

func (record ImmutableArtifactRecord) Validate() error {
	if !artifactCompatibilityFingerprintPattern.MatchString(record.CompatibilityFingerprint) {
		return errors.New("artifact record compatibility fingerprint must be a lowercase SHA-256 digest")
	}
	if err := ValidateSecretSafeValue(record); err != nil {
		return errors.New("artifact record contains secret material")
	}
	if err := record.Artifact.Validate(); err != nil {
		return fmt.Errorf("artifact record contract: %w", err)
	}
	if record.Artifact.InputFingerprint != record.CompatibilityFingerprint {
		return errors.New("artifact record input fingerprint does not match its compatibility boundary")
	}
	return nil
}

type ArtifactBuildFunc func(context.Context, ArtifactBuildRequest) (sdk.ImmutableArtifactContract, error)

type ArtifactReuseDecision string

const (
	ArtifactReuseReady   ArtifactReuseDecision = "ready"
	ArtifactReuseMiss    ArtifactReuseDecision = "miss"
	ArtifactReuseBlocked ArtifactReuseDecision = "blocked"
)

type ArtifactReuseLookup struct {
	Decision ArtifactReuseDecision
	Record   ImmutableArtifactRecord
	Reason   string
}

// ImmutableArtifactRegistry is the durable coordination boundary for
// build-once certification. A production implementation should back these
// operations with a conditional write or transaction; the in-memory version
// below is intentionally strict for contract tests.
type ImmutableArtifactRegistry interface {
	Lookup(context.Context, string) (ImmutableArtifactRecord, bool, error)
	Claim(context.Context, string, string) (ArtifactReuseLookup, error)
	ReleaseClaim(context.Context, string, string) error
	Publish(context.Context, string, string, ImmutableArtifactRecord) error
}

// EnsureImmutableArtifact claims a compatibility boundary before building it.
// A concurrent caller receives a typed blocked result instead of starting a
// second build. A failed build releases the claim so a later run can retry.
func EnsureImmutableArtifact(ctx context.Context, registry ImmutableArtifactRegistry, request ArtifactBuildRequest, build ArtifactBuildFunc) (ImmutableArtifactRecord, bool, error) {
	if ctx == nil {
		return ImmutableArtifactRecord{}, false, errors.New("artifact registry context is required")
	}
	if registry == nil {
		return ImmutableArtifactRecord{}, false, errors.New("artifact registry is required")
	}
	if err := request.Validate(); err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	if build == nil {
		return ImmutableArtifactRecord{}, false, errors.New("immutable artifact builder is required")
	}
	if err := ctx.Err(); err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	lookup, found, err := registry.Lookup(ctx, request.CompatibilityFingerprint)
	if err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	if found {
		if err := lookup.Validate(); err != nil {
			return ImmutableArtifactRecord{}, false, errors.New("artifact registry returned an invalid published record")
		}
		return lookup, true, nil
	}
	claimOwner := request.OwnershipMarker
	claim, err := registry.Claim(ctx, request.CompatibilityFingerprint, claimOwner)
	if err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	switch claim.Decision {
	case ArtifactReuseReady:
		if err := claim.Record.Validate(); err != nil {
			return ImmutableArtifactRecord{}, false, errors.New("artifact registry returned an invalid claimed record")
		}
		return claim.Record, true, nil
	case ArtifactReuseBlocked:
		return ImmutableArtifactRecord{}, false, fmt.Errorf("artifact build is blocked: %s", claim.Reason)
	case ArtifactReuseMiss:
		// Continue to the single builder owner below.
	default:
		return ImmutableArtifactRecord{}, false, fmt.Errorf("artifact registry returned unknown reuse decision %q", claim.Decision)
	}
	contract, buildErr := build(ctx, request)
	if buildErr != nil {
		return ImmutableArtifactRecord{}, false, errors.Join(buildErr, releaseArtifactClaim(ctx, registry, request.CompatibilityFingerprint, claimOwner))
	}
	record := ImmutableArtifactRecord{CompatibilityFingerprint: request.CompatibilityFingerprint, Artifact: contract}
	if err := record.Validate(); err != nil {
		return ImmutableArtifactRecord{}, false, errors.Join(err, releaseArtifactClaim(ctx, registry, request.CompatibilityFingerprint, claimOwner))
	}
	if err := registry.Publish(ctx, request.CompatibilityFingerprint, claimOwner, record); err != nil {
		return ImmutableArtifactRecord{}, false, errors.Join(err, releaseArtifactClaim(ctx, registry, request.CompatibilityFingerprint, claimOwner))
	}
	return record, false, nil
}

func releaseArtifactClaim(ctx context.Context, registry ImmutableArtifactRegistry, fingerprint, owner string) error {
	releaseContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()
	return registry.ReleaseClaim(releaseContext, fingerprint, owner)
}

// MemoryImmutableArtifactRegistry is a deterministic local registry. It is
// safe for concurrent provider groups and mirrors the conditional ownership
// checks a durable implementation must provide.
type MemoryImmutableArtifactRegistry struct {
	mu      sync.RWMutex
	claims  map[string]string
	records map[string]ImmutableArtifactRecord
}

// FileImmutableArtifactRegistry is a durable single-host registry for local
// and CI certification runners. It keeps only immutable artifact identities
// and claim owners, writes state through a same-directory atomic rename, and
// serializes processes with an exact lock directory. A distributed runner
// should inject an object-store or database implementation of
// ImmutableArtifactRegistry instead; the certification core does not depend
// on this storage choice.
type FileImmutableArtifactRegistry struct {
	path           string
	lockPath       string
	lockPoll       time.Duration
	staleLockAfter time.Duration
}

const fileArtifactRegistryVersion = 1

type fileArtifactRegistryState struct {
	Version int                                `json:"version"`
	Claims  map[string]string                  `json:"claims"`
	Records map[string]ImmutableArtifactRecord `json:"records"`
}

// NewFileImmutableArtifactRegistry opens or creates a durable registry file.
// The file and its parent directory are private to the current user because
// the state is a coordination record even though it never contains secrets.
func NewFileImmutableArtifactRegistry(path string) (*FileImmutableArtifactRegistry, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return nil, errors.New("file artifact registry path is required and must be single-line")
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve file artifact registry path: %w", err)
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create file artifact registry directory: %w", err)
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return nil, errors.New("file artifact registry path must be a file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect file artifact registry: %w", err)
	}
	return &FileImmutableArtifactRegistry{
		path:           path,
		lockPath:       path + ".lock",
		lockPoll:       defaultFileRegistryLockPoll,
		staleLockAfter: defaultFileRegistryStaleLockAfter,
	}, nil
}

func (registry *FileImmutableArtifactRegistry) Lookup(ctx context.Context, fingerprint string) (record ImmutableArtifactRecord, found bool, err error) {
	if err := validateArtifactFingerprint(fingerprint); err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	err = registry.withState(ctx, false, func(state *fileArtifactRegistryState) error {
		record, found = state.Records[fingerprint]
		return nil
	})
	return record, found, err
}

func (registry *FileImmutableArtifactRegistry) Claim(ctx context.Context, fingerprint, owner string) (lookup ArtifactReuseLookup, err error) {
	if err := validateArtifactFingerprint(fingerprint); err != nil {
		return ArtifactReuseLookup{}, err
	}
	if err := validateArtifactClaimOwner(owner); err != nil {
		return ArtifactReuseLookup{}, err
	}
	err = registry.withState(ctx, true, func(state *fileArtifactRegistryState) error {
		if record, found := state.Records[fingerprint]; found {
			lookup = ArtifactReuseLookup{Decision: ArtifactReuseReady, Record: record}
			return nil
		}
		if current, found := state.Claims[fingerprint]; found {
			if current == owner {
				lookup = ArtifactReuseLookup{Decision: ArtifactReuseBlocked, Reason: "compatibility boundary is already claimed"}
			} else {
				lookup = ArtifactReuseLookup{Decision: ArtifactReuseBlocked, Reason: "compatibility boundary is claimed by another build"}
			}
			return nil
		}
		state.Claims[fingerprint] = owner
		lookup = ArtifactReuseLookup{Decision: ArtifactReuseMiss}
		return nil
	})
	return lookup, err
}

func (registry *FileImmutableArtifactRegistry) ReleaseClaim(ctx context.Context, fingerprint, owner string) error {
	if err := validateArtifactFingerprint(fingerprint); err != nil {
		return err
	}
	if err := validateArtifactClaimOwner(owner); err != nil {
		return err
	}
	return registry.withState(ctx, true, func(state *fileArtifactRegistryState) error {
		current, found := state.Claims[fingerprint]
		if !found {
			return nil
		}
		if current != owner {
			return errors.New("artifact claim owner does not match")
		}
		delete(state.Claims, fingerprint)
		return nil
	})
}

func (registry *FileImmutableArtifactRegistry) Publish(ctx context.Context, fingerprint, owner string, record ImmutableArtifactRecord) error {
	if err := validateArtifactFingerprint(fingerprint); err != nil {
		return err
	}
	if err := validateArtifactClaimOwner(owner); err != nil {
		return err
	}
	if record.CompatibilityFingerprint != fingerprint {
		return errors.New("artifact publication fingerprint does not match the registry key")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	return registry.withState(ctx, true, func(state *fileArtifactRegistryState) error {
		current, found := state.Claims[fingerprint]
		if !found || current != owner {
			return errors.New("artifact publication requires the matching build claim")
		}
		if existing, exists := state.Records[fingerprint]; exists && existing.Artifact != record.Artifact {
			return errors.New("artifact registry rejects a conflicting immutable publication")
		}
		state.Records[fingerprint] = record
		delete(state.Claims, fingerprint)
		return nil
	})
}

func (registry *FileImmutableArtifactRegistry) withState(ctx context.Context, write bool, operation func(*fileArtifactRegistryState) error) (err error) {
	if ctx == nil {
		return errors.New("artifact registry context is required")
	}
	if registry == nil || registry.path == "" || registry.lockPath == "" {
		return errors.New("file artifact registry is required")
	}
	if operation == nil {
		return errors.New("file artifact registry operation is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return withFileRegistryLock(ctx, registry.lockPath, registry.lockPoll, registry.staleLockAfter, func() error {
		state, err := registry.readState()
		if err != nil {
			return err
		}
		if err := operation(&state); err != nil {
			return err
		}
		if !write {
			return nil
		}
		return registry.writeState(state)
	})
}

func (registry *FileImmutableArtifactRegistry) readState() (fileArtifactRegistryState, error) {
	state := fileArtifactRegistryState{Version: fileArtifactRegistryVersion, Claims: make(map[string]string), Records: make(map[string]ImmutableArtifactRecord)}
	file, err := os.Open(registry.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return fileArtifactRegistryState{}, fmt.Errorf("open file artifact registry: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return fileArtifactRegistryState{}, fmt.Errorf("decode file artifact registry: %w", err)
	}
	if state.Version != fileArtifactRegistryVersion {
		return fileArtifactRegistryState{}, fmt.Errorf("unsupported file artifact registry version %d", state.Version)
	}
	if state.Claims == nil {
		state.Claims = make(map[string]string)
	}
	if state.Records == nil {
		state.Records = make(map[string]ImmutableArtifactRecord)
	}
	for fingerprint, owner := range state.Claims {
		if err := validateArtifactFingerprint(fingerprint); err != nil {
			return fileArtifactRegistryState{}, fmt.Errorf("file artifact registry claim: %w", err)
		}
		if err := validateArtifactClaimOwner(owner); err != nil {
			return fileArtifactRegistryState{}, fmt.Errorf("file artifact registry claim: %w", err)
		}
	}
	for fingerprint, record := range state.Records {
		if err := validateArtifactFingerprint(fingerprint); err != nil {
			return fileArtifactRegistryState{}, fmt.Errorf("file artifact registry record: %w", err)
		}
		if err := record.Validate(); err != nil {
			return fileArtifactRegistryState{}, fmt.Errorf("file artifact registry record: %w", err)
		}
		if record.CompatibilityFingerprint != fingerprint {
			return fileArtifactRegistryState{}, errors.New("file artifact registry record key does not match its compatibility fingerprint")
		}
	}
	return state, nil
}

func (registry *FileImmutableArtifactRegistry) writeState(state fileArtifactRegistryState) error {
	if err := os.MkdirAll(filepath.Dir(registry.path), 0o700); err != nil {
		return fmt.Errorf("create file artifact registry directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(registry.path), ".artifact-registry-*")
	if err != nil {
		return fmt.Errorf("create file artifact registry temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect file artifact registry temporary file: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode file artifact registry: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync file artifact registry: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close file artifact registry temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, registry.path); err != nil {
		return fmt.Errorf("publish file artifact registry: %w", err)
	}
	return os.Chmod(registry.path, 0o600)
}

func validateArtifactFingerprint(fingerprint string) error {
	if !artifactCompatibilityFingerprintPattern.MatchString(fingerprint) {
		return errors.New("artifact compatibility fingerprint must be a lowercase SHA-256 digest")
	}
	return nil
}

func validateArtifactClaimOwner(owner string) error {
	if strings.TrimSpace(owner) == "" || strings.ContainsAny(owner, "\r\n\x00") {
		return errors.New("artifact claim owner is required and must be single-line")
	}
	return nil
}

func NewMemoryImmutableArtifactRegistry() *MemoryImmutableArtifactRegistry {
	return &MemoryImmutableArtifactRegistry{claims: make(map[string]string), records: make(map[string]ImmutableArtifactRecord)}
}

func (registry *MemoryImmutableArtifactRegistry) Lookup(ctx context.Context, fingerprint string) (ImmutableArtifactRecord, bool, error) {
	if ctx == nil {
		return ImmutableArtifactRecord{}, false, errors.New("artifact registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return ImmutableArtifactRecord{}, false, err
	}
	if !artifactCompatibilityFingerprintPattern.MatchString(fingerprint) {
		return ImmutableArtifactRecord{}, false, errors.New("artifact compatibility fingerprint must be a lowercase SHA-256 digest")
	}
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	record, found := registry.records[fingerprint]
	return record, found, nil
}

func (registry *MemoryImmutableArtifactRegistry) Claim(ctx context.Context, fingerprint, owner string) (ArtifactReuseLookup, error) {
	if ctx == nil {
		return ArtifactReuseLookup{}, errors.New("artifact registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return ArtifactReuseLookup{}, err
	}
	if !artifactCompatibilityFingerprintPattern.MatchString(fingerprint) {
		return ArtifactReuseLookup{}, errors.New("artifact compatibility fingerprint must be a lowercase SHA-256 digest")
	}
	if strings.TrimSpace(owner) == "" || strings.ContainsAny(owner, "\r\n\x00") {
		return ArtifactReuseLookup{}, errors.New("artifact claim owner is required and must be single-line")
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if record, found := registry.records[fingerprint]; found {
		return ArtifactReuseLookup{Decision: ArtifactReuseReady, Record: record}, nil
	}
	if current, found := registry.claims[fingerprint]; found && current != owner {
		return ArtifactReuseLookup{Decision: ArtifactReuseBlocked, Reason: "compatibility boundary is claimed by another build"}, nil
	}
	if _, found := registry.claims[fingerprint]; found {
		return ArtifactReuseLookup{Decision: ArtifactReuseBlocked, Reason: "compatibility boundary is already claimed"}, nil
	}
	registry.claims[fingerprint] = owner
	return ArtifactReuseLookup{Decision: ArtifactReuseMiss}, nil
}

func (registry *MemoryImmutableArtifactRegistry) ReleaseClaim(ctx context.Context, fingerprint, owner string) error {
	if ctx == nil {
		return errors.New("artifact registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	current, found := registry.claims[fingerprint]
	if !found {
		return nil
	}
	if current != owner {
		return errors.New("artifact claim owner does not match")
	}
	delete(registry.claims, fingerprint)
	return nil
}

func (registry *MemoryImmutableArtifactRegistry) Publish(ctx context.Context, fingerprint, owner string, record ImmutableArtifactRecord) error {
	if ctx == nil {
		return errors.New("artifact registry context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if record.CompatibilityFingerprint != fingerprint {
		return errors.New("artifact publication fingerprint does not match the registry key")
	}
	if err := record.Validate(); err != nil {
		return err
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	current, found := registry.claims[fingerprint]
	if !found || current != owner {
		return errors.New("artifact publication requires the matching build claim")
	}
	if existing, exists := registry.records[fingerprint]; exists && existing.Artifact != record.Artifact {
		return errors.New("artifact registry rejects a conflicting immutable publication")
	}
	registry.records[fingerprint] = record
	delete(registry.claims, fingerprint)
	return nil
}
