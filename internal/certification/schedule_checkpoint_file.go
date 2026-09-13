package certification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// FileScheduleCheckpointStore is a durable local/CI checkpoint sink for one
// scheduler run. Save merges the unit-sized writes emitted by ExecuteSchedule
// and atomically replaces the state file, so a process interruption cannot
// leave a partially written checkpoint batch. Distributed runners should
// inject a transactional checkpoint backend instead.
type FileScheduleCheckpointStore struct {
	path           string
	lockPath       string
	lockPoll       time.Duration
	staleLockAfter time.Duration
}

const fileScheduleCheckpointVersion = 1

type fileScheduleCheckpointState struct {
	Version     int                            `json:"version"`
	Checkpoints map[string]SchedulerCheckpoint `json:"checkpoints"`
}

// NewFileScheduleCheckpointStore opens or creates a durable checkpoint file.
func NewFileScheduleCheckpointStore(path string) (*FileScheduleCheckpointStore, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.ContainsAny(path, "\r\n\x00") {
		return nil, errors.New("file schedule checkpoint path is required and must be single-line")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve file schedule checkpoint path: %w", err)
	}
	parent := filepath.Dir(absPath)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return nil, fmt.Errorf("create file schedule checkpoint directory: %w", err)
	}
	if info, err := os.Stat(absPath); err == nil && info.IsDir() {
		return nil, errors.New("file schedule checkpoint path must be a file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect file schedule checkpoint: %w", err)
	}
	return &FileScheduleCheckpointStore{
		path:           absPath,
		lockPath:       absPath + ".lock",
		lockPoll:       defaultFileRegistryLockPoll,
		staleLockAfter: defaultFileRegistryStaleLockAfter,
	}, nil
}

// Load returns a defensive, deterministic snapshot suitable for
// SchedulerOptions.Checkpoints.
func (store *FileScheduleCheckpointStore) Load(ctx context.Context) ([]SchedulerCheckpoint, error) {
	var checkpoints []SchedulerCheckpoint
	if err := store.withState(ctx, false, func(state *fileScheduleCheckpointState) error {
		checkpoints = cloneScheduleCheckpoints(state.Checkpoints)
		return nil
	}); err != nil {
		return nil, err
	}
	return checkpoints, nil
}

// Save merges one or more unit checkpoint writes. A later write may update a
// cell's status and operation references, but it may not change the cell's
// fingerprint or ownership scope.
func (store *FileScheduleCheckpointStore) Save(ctx context.Context, checkpoints []SchedulerCheckpoint) error {
	if len(checkpoints) == 0 {
		return errors.New("file schedule checkpoint save requires at least one checkpoint")
	}
	for _, checkpoint := range checkpoints {
		if err := validateFileScheduleCheckpoint(checkpoint); err != nil {
			return err
		}
	}
	return store.withState(ctx, true, func(state *fileScheduleCheckpointState) error {
		for _, checkpoint := range checkpoints {
			existing, found := state.Checkpoints[checkpoint.CellID]
			if found && (existing.Fingerprint != checkpoint.Fingerprint || existing.OwnershipMarker != checkpoint.OwnershipMarker) {
				return fmt.Errorf("file schedule checkpoint rejects stale identity for cell %q", checkpoint.CellID)
			}
			state.Checkpoints[checkpoint.CellID] = cloneSchedulerCheckpoint(checkpoint)
		}
		return nil
	})
}

// Writer adapts Save to the scheduler's checkpoint callback shape.
func (store *FileScheduleCheckpointStore) Writer() ScheduleCheckpointWriter {
	if store == nil {
		return nil
	}
	return store.Save
}

func (store *FileScheduleCheckpointStore) withState(ctx context.Context, write bool, operation func(*fileScheduleCheckpointState) error) error {
	if ctx == nil {
		return errors.New("file schedule checkpoint context is required")
	}
	if store == nil || store.path == "" || store.lockPath == "" {
		return errors.New("file schedule checkpoint store is required")
	}
	if operation == nil {
		return errors.New("file schedule checkpoint operation is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return withFileRegistryLock(ctx, store.lockPath, store.lockPoll, store.staleLockAfter, func() error {
		state, err := store.readState()
		if err != nil {
			return err
		}
		if err := operation(&state); err != nil {
			return err
		}
		if !write {
			return nil
		}
		return store.writeState(state)
	})
}

func (store *FileScheduleCheckpointStore) readState() (fileScheduleCheckpointState, error) {
	state := fileScheduleCheckpointState{Version: fileScheduleCheckpointVersion, Checkpoints: make(map[string]SchedulerCheckpoint)}
	file, err := os.Open(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return fileScheduleCheckpointState{}, fmt.Errorf("open file schedule checkpoint: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return fileScheduleCheckpointState{}, fmt.Errorf("decode file schedule checkpoint: %w", err)
	}
	if state.Version != fileScheduleCheckpointVersion {
		return fileScheduleCheckpointState{}, fmt.Errorf("unsupported file schedule checkpoint version %d", state.Version)
	}
	if state.Checkpoints == nil {
		state.Checkpoints = make(map[string]SchedulerCheckpoint)
	}
	for cellID, checkpoint := range state.Checkpoints {
		if checkpoint.CellID != cellID {
			return fileScheduleCheckpointState{}, errors.New("file schedule checkpoint key does not match cell ID")
		}
		if err := validateFileScheduleCheckpoint(checkpoint); err != nil {
			return fileScheduleCheckpointState{}, fmt.Errorf("file schedule checkpoint: %w", err)
		}
	}
	return state, nil
}

func (store *FileScheduleCheckpointStore) writeState(state fileScheduleCheckpointState) error {
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return fmt.Errorf("create file schedule checkpoint directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".schedule-checkpoint-*")
	if err != nil {
		return fmt.Errorf("create file schedule checkpoint temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return fmt.Errorf("protect file schedule checkpoint temporary file: %w", err)
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(state); err != nil {
		return fmt.Errorf("encode file schedule checkpoint: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return fmt.Errorf("sync file schedule checkpoint: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close file schedule checkpoint temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, store.path); err != nil {
		return fmt.Errorf("publish file schedule checkpoint: %w", err)
	}
	return os.Chmod(store.path, 0o600)
}

func validateFileScheduleCheckpoint(checkpoint SchedulerCheckpoint) error {
	if err := ValidateSecretSafeValue(checkpoint); err != nil {
		return errors.New("schedule checkpoint contains secret-like material")
	}
	if strings.TrimSpace(checkpoint.CellID) == "" || strings.ContainsAny(checkpoint.CellID, "\r\n\x00") {
		return errors.New("schedule checkpoint cell ID is required and must be single-line")
	}
	if !fingerprintPattern.MatchString(checkpoint.Fingerprint) {
		return fmt.Errorf("schedule checkpoint for cell %q requires a lowercase SHA-256 fingerprint", checkpoint.CellID)
	}
	if strings.TrimSpace(checkpoint.OwnershipMarker) == "" || strings.ContainsAny(checkpoint.OwnershipMarker, "\r\n\x00") {
		return fmt.Errorf("schedule checkpoint for cell %q requires a single-line ownership marker", checkpoint.CellID)
	}
	switch checkpoint.Status {
	case "PASS", "FAIL", "PENDING", "IN_PROGRESS":
	default:
		return fmt.Errorf("schedule checkpoint for cell %q has invalid status %q", checkpoint.CellID, checkpoint.Status)
	}
	if checkpoint.ReuseFingerprint != nil {
		if err := checkpoint.ReuseFingerprint.Validate(); err != nil {
			return fmt.Errorf("schedule checkpoint for cell %q reuse fingerprint: %w", checkpoint.CellID, err)
		}
		digest, err := checkpoint.ReuseFingerprint.Digest()
		if err != nil {
			return fmt.Errorf("schedule checkpoint for cell %q reuse fingerprint: %w", checkpoint.CellID, err)
		}
		if digest != checkpoint.Fingerprint || checkpoint.ReuseFingerprint.OwnershipMarker != checkpoint.OwnershipMarker {
			return fmt.Errorf("schedule checkpoint for cell %q reuse fingerprint does not match its identity", checkpoint.CellID)
		}
	}
	if checkpoint.UpdatedAt != "" {
		if _, err := time.Parse(time.RFC3339, checkpoint.UpdatedAt); err != nil {
			return fmt.Errorf("schedule checkpoint for cell %q has invalid updatedAt", checkpoint.CellID)
		}
	}
	return nil
}

func cloneScheduleCheckpoints(values map[string]SchedulerCheckpoint) []SchedulerCheckpoint {
	result := make([]SchedulerCheckpoint, 0, len(values))
	for _, checkpoint := range values {
		result = append(result, cloneSchedulerCheckpoint(checkpoint))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CellID < result[j].CellID })
	return result
}

func cloneSchedulerCheckpoint(checkpoint SchedulerCheckpoint) SchedulerCheckpoint {
	checkpoint.OperationIDs = append([]string(nil), checkpoint.OperationIDs...)
	checkpoint.ResourceRefs = append([]string(nil), checkpoint.ResourceRefs...)
	checkpoint.BackupRefs = append([]string(nil), checkpoint.BackupRefs...)
	checkpoint.RestoreRefs = append([]string(nil), checkpoint.RestoreRefs...)
	checkpoint.TelemetryRefs = append([]string(nil), checkpoint.TelemetryRefs...)
	checkpoint.EdgeRefs = append([]string(nil), checkpoint.EdgeRefs...)
	if checkpoint.ReuseFingerprint != nil {
		fingerprint := *checkpoint.ReuseFingerprint
		checkpoint.ReuseFingerprint = &fingerprint
	}
	return checkpoint
}
