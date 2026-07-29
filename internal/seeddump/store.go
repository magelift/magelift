package seeddump

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var environmentPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)

const (
	StatusRecorded  = "recorded"
	StatusImporting = "importing"
	StatusImported  = "imported"
	StatusFailed    = "failed"
)

// Record is the single-document seed-dump status journal for one environment.
type Record struct {
	Status    string    `json:"status"`
	DumpPath  string    `json:"dumpPath,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Store reads and writes .magelift/seed-dumps/<env>.json.
type Store struct {
	path string
	now  func() time.Time
}

// New returns a store for environment under projectRoot.
func New(projectRoot, environment string) (*Store, error) {
	if !environmentPattern.MatchString(environment) {
		return nil, errors.New("seed dump environment is invalid")
	}
	return &Store{
		path: filepath.Join(projectRoot, ".magelift", "seed-dumps", environment+".json"),
		now:  time.Now,
	}, nil
}

// Path returns the journal file path.
func (s *Store) Path() string {
	return s.path
}

// Read loads the journal record, or (nil, nil) when the file is absent.
func (s *Store) Read(ctx context.Context) (*Record, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect seed dump journal: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("seed dump journal must be a regular file")
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("read seed dump journal: %w", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("decode seed dump journal: %w", err)
	}
	if record.Status == "" {
		return nil, errors.New("seed dump journal status is missing")
	}
	return &record, nil
}

// InitRecorded writes status=recorded (idempotent overwrite of recorded→recorded).
func InitRecorded(ctx context.Context, projectRoot, environment, dumpPath string) (*Record, error) {
	store, err := New(projectRoot, environment)
	if err != nil {
		return nil, err
	}
	return store.write(ctx, Record{
		Status:   StatusRecorded,
		DumpPath: dumpPath,
	})
}

func (s *Store) write(ctx context.Context, record Record) (*Record, error) {
	release, err := acquireLock(ctx, s.path+".lock")
	if err != nil {
		return nil, err
	}
	defer release()
	record.UpdatedAt = s.now().UTC()
	if err := writeAtomic(s.path, record); err != nil {
		return nil, err
	}
	return &record, nil
}

func acquireLock(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create seed dump journal directory: %w", err)
	}
	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire seed dump journal lock: %w", err)
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, context.Cause(ctx)
		case <-timer.C:
		}
	}
}

func writeAtomic(path string, record Record) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create seed dump journal directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".seed-dump-*.json")
	if err != nil {
		return fmt.Errorf("create seed dump journal: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()
	if err := temporary.Chmod(0o600); err != nil {
		return err
	}
	encoder := json.NewEncoder(temporary)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(record); err != nil {
		return fmt.Errorf("encode seed dump journal: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace seed dump journal: %w", err)
	}
	return nil
}
