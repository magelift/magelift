package certification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileSessionRegistry is a durable single-host registry for local and CI
// certification runners. It persists only provider-neutral session identities,
// fingerprints, opaque operation/resource references, and cleanup state.
// Distributed runners must inject a conditional object-store or database
// implementation of SessionRegistry; this file lock is not a cross-host
// coordination protocol.
type FileSessionRegistry struct {
	path           string
	lockPath       string
	lockPoll       time.Duration
	staleLockAfter time.Duration
}

const fileSessionRegistryVersion = 1

type fileSessionRegistryState struct {
	Version int                      `json:"version"`
	Records map[string]SessionRecord `json:"records"`
}

// NewFileSessionRegistry opens or creates a durable session registry file.
// The parent directory and state file are private to the current user because
// the registry is a coordination record even though it never contains
// credential values.
func NewFileSessionRegistry(path string) (*FileSessionRegistry, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return nil, errors.New("file session registry path is required and must be single-line")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve file session registry path: %w", err)
	}
	parent := filepath.Dir(absPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create file session registry directory: %w", err)
	}
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return nil, errors.New("file session registry path must be a file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect file session registry: %w", err)
	}
	return &FileSessionRegistry{
		path:           absPath,
		lockPath:       absPath + ".lock",
		lockPoll:       defaultFileRegistryLockPoll,
		staleLockAfter: defaultFileRegistryStaleLockAfter,
	}, nil
}

var _ SessionRegistry = (*FileSessionRegistry)(nil)

func (registry *FileSessionRegistry) Lookup(ctx context.Context, fingerprint ReuseFingerprint) (record SessionRecord, found bool, err error) {
	digest, err := fingerprint.Digest()
	if err != nil {
		return SessionRecord{}, false, err
	}
	err = registry.withState(ctx, false, func(state *fileSessionRegistryState) error {
		record, found = state.Records[digest]
		if found {
			record = cloneSessionRecord(record)
		}
		return nil
	})
	return record, found, err
}

func (registry *FileSessionRegistry) Claim(ctx context.Context, fingerprint ReuseFingerprint, owner string) (lookup ReuseLookup, err error) {
	if err := validateSessionClaimOwner(owner); err != nil {
		return ReuseLookup{}, err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return ReuseLookup{}, err
	}
	err = registry.withState(ctx, true, func(state *fileSessionRegistryState) error {
		record, found := state.Records[digest]
		if !found {
			lookup = ReuseLookup{Decision: ReuseMiss, Reason: "no compatible session is registered"}
			return nil
		}
		if err := record.Validate(); err != nil {
			return errors.New("session registry returned an invalid record")
		}
		switch record.State {
		case SessionReady:
			if record.ClaimOwner != "" && record.ClaimOwner != owner {
				lookup = ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session is claimed by another certification run"}
				return nil
			}
			record.ClaimOwner = owner
			if err := record.Validate(); err != nil {
				return err
			}
			state.Records[digest] = cloneSessionRecord(record)
			lookup = ReuseLookup{Decision: ReuseReady, Record: cloneSessionRecord(record)}
			return nil
		case SessionPendingDelete:
			lookup = ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session has pending owning-service deletion"}
		case SessionQuarantined:
			lookup = ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session is quarantined"}
		case SessionCleanupVerified:
			lookup = ReuseLookup{Decision: ReuseMiss, Record: cloneSessionRecord(record), Reason: "compatible session was already cleaned up"}
		default:
			lookup = ReuseLookup{Decision: ReuseBlocked, Record: cloneSessionRecord(record), Reason: "compatible session has an unknown state"}
		}
		return nil
	})
	return lookup, err
}

func (registry *FileSessionRegistry) ReleaseClaim(ctx context.Context, fingerprint ReuseFingerprint, owner string) error {
	if err := validateSessionClaimOwner(owner); err != nil {
		return err
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	return registry.withState(ctx, true, func(state *fileSessionRegistryState) error {
		record, found := state.Records[digest]
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
		if err := record.Validate(); err != nil {
			return err
		}
		state.Records[digest] = cloneSessionRecord(record)
		return nil
	})
}

func (registry *FileSessionRegistry) Put(ctx context.Context, record SessionRecord) error {
	if err := record.Validate(); err != nil {
		return err
	}
	return registry.withState(ctx, true, func(state *fileSessionRegistryState) error {
		existing, found := state.Records[record.FingerprintHash]
		if found && existing.StackID != record.StackID && existing.Live {
			return errors.New("session registry rejects a duplicate live writer for the fingerprint")
		}
		state.Records[record.FingerprintHash] = cloneSessionRecord(record)
		return nil
	})
}

func (registry *FileSessionRegistry) MarkPendingDeletion(ctx context.Context, fingerprint ReuseFingerprint, pending []string) error {
	if len(pending) == 0 {
		return errors.New("pending deletion requires owning-service resource references")
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	return registry.withState(ctx, true, func(state *fileSessionRegistryState) error {
		record, found := state.Records[digest]
		if !found {
			return errors.New("session registry record was not found")
		}
		record.State = SessionPendingDelete
		record.Live = true
		record.CleanupComplete = false
		record.ClaimOwner = ""
		record.PendingRefs = append([]string(nil), pending...)
		if err := record.Validate(); err != nil {
			return err
		}
		state.Records[digest] = cloneSessionRecord(record)
		return nil
	})
}

func (registry *FileSessionRegistry) MarkCleanupVerified(ctx context.Context, fingerprint ReuseFingerprint, stackID string) error {
	if strings.TrimSpace(stackID) == "" || strings.ContainsAny(stackID, "\r\n\x00") {
		return errors.New("session registry cleanup stack ID is required and must be single-line")
	}
	digest, err := fingerprint.Digest()
	if err != nil {
		return err
	}
	return registry.withState(ctx, true, func(state *fileSessionRegistryState) error {
		record, found := state.Records[digest]
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
		if err := record.Validate(); err != nil {
			return err
		}
		state.Records[digest] = cloneSessionRecord(record)
		return nil
	})
}

func (registry *FileSessionRegistry) withState(ctx context.Context, write bool, operation func(*fileSessionRegistryState) error) error {
	if ctx == nil {
		return errors.New("file session registry context is required")
	}
	if registry == nil || registry.path == "" || registry.lockPath == "" {
		return errors.New("file session registry is required")
	}
	if operation == nil {
		return errors.New("file session registry operation is required")
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

func (registry *FileSessionRegistry) readState() (fileSessionRegistryState, error) {
	state := fileSessionRegistryState{Version: fileSessionRegistryVersion, Records: make(map[string]SessionRecord)}
	file, err := os.Open(registry.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return fileSessionRegistryState{}, fmt.Errorf("open file session registry: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return fileSessionRegistryState{}, fmt.Errorf("decode file session registry: %w", err)
	}
	if state.Version != fileSessionRegistryVersion {
		return fileSessionRegistryState{}, fmt.Errorf("unsupported file session registry version %d", state.Version)
	}
	if state.Records == nil {
		state.Records = make(map[string]SessionRecord)
	}
	for fingerprintHash, record := range state.Records {
		if _, err := parseSessionFingerprintHash(fingerprintHash); err != nil {
			return fileSessionRegistryState{}, fmt.Errorf("file session registry record: %w", err)
		}
		if err := record.Validate(); err != nil {
			return fileSessionRegistryState{}, fmt.Errorf("file session registry record: %w", err)
		}
		if record.FingerprintHash != fingerprintHash {
			return fileSessionRegistryState{}, errors.New("file session registry record key does not match its fingerprint hash")
		}
	}
	return state, nil
}

func (registry *FileSessionRegistry) writeState(state fileSessionRegistryState) error {
	if err := os.MkdirAll(filepath.Dir(registry.path), 0o700); err != nil {
		return fmt.Errorf("create file session registry directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(registry.path), ".session-registry-*")
	if err != nil {
		return fmt.Errorf("create file session registry temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect file session registry temporary file: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode file session registry: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync file session registry: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close file session registry temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, registry.path); err != nil {
		return fmt.Errorf("publish file session registry: %w", err)
	}
	return os.Chmod(registry.path, 0o600)
}

func validateSessionClaimOwner(owner string) error {
	if strings.TrimSpace(owner) == "" || strings.ContainsAny(owner, "\r\n\x00") {
		return errors.New("session reuse claim owner is required and must be single-line")
	}
	return ValidateSecretSafeText(owner)
}

func parseSessionFingerprintHash(hash string) (string, error) {
	if !fingerprintPattern.MatchString(hash) {
		return "", errors.New("session registry fingerprint hash must be a lowercase SHA-256 digest")
	}
	return hash, nil
}
