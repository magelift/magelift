package releasejournal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var environmentPattern = regexp.MustCompile(`^[a-z][a-z0-9.-]*$`)

type Action string

const (
	ActionPromote  Action = "promote"
	ActionRollback Action = "rollback"
	ActionDeploy   Action = "deploy"
)

type Entry struct {
	Sequence                   int       `json:"sequence" yaml:"sequence"`
	RecordedAt                 time.Time `json:"recordedAt" yaml:"recordedAt"`
	Action                     Action    `json:"action" yaml:"action"`
	Environment                string    `json:"environment" yaml:"environment"`
	DigestReference            string    `json:"digestReference" yaml:"digestReference"`
	SourceEnvironment          string    `json:"sourceEnvironment,omitempty" yaml:"sourceEnvironment,omitempty"`
	SourceSequence             int       `json:"sourceSequence,omitempty" yaml:"sourceSequence,omitempty"`
	SignatureIdentity          string    `json:"signatureIdentity" yaml:"signatureIdentity"`
	SignatureIssuer            string    `json:"signatureIssuer" yaml:"signatureIssuer"`
	ForwardOnly                bool      `json:"forwardOnly" yaml:"forwardOnly"`
	DatabaseMigrationsReversed bool      `json:"databaseMigrationsReversed" yaml:"databaseMigrationsReversed"`
}

type Store struct {
	path string
	now  func() time.Time
}

func New(projectRoot, environment string) (*Store, error) {
	if !environmentPattern.MatchString(environment) {
		return nil, errors.New("release environment is invalid")
	}
	return &Store{path: filepath.Join(projectRoot, ".magelift", "releases", environment+".jsonl"), now: time.Now}, nil
}

func (s *Store) List(ctx context.Context) ([]Entry, error) {
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	info, err := os.Lstat(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect release journal: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("release journal must be a regular file")
	}
	file, err := os.Open(s.path)
	if err != nil {
		return nil, fmt.Errorf("open release journal: %w", err)
	}
	defer file.Close()
	var entries []Entry
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		var entry Entry
		if err := decoder.Decode(&entry); err != nil {
			return nil, fmt.Errorf("decode release journal entry: %w", err)
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return nil, errors.New("release journal entry contains trailing data")
		}
		if entry.Sequence != len(entries)+1 || entry.Environment == "" || entry.DigestReference == "" {
			return nil, errors.New("release journal sequence or entry is invalid")
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read release journal: %w", err)
	}
	return entries, nil
}

func (s *Store) Append(ctx context.Context, entry Entry) (Entry, error) {
	release, err := acquireLock(ctx, s.path+".lock")
	if err != nil {
		return Entry{}, err
	}
	defer release()
	entries, err := s.List(ctx)
	if err != nil {
		return Entry{}, err
	}
	entry.Sequence = len(entries) + 1
	entry.RecordedAt = s.now().UTC()
	entries = append(entries, entry)
	if err := writeAtomic(s.path, entries); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func acquireLock(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create release journal directory: %w", err)
	}
	for {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err == nil {
			_ = file.Close()
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("acquire release journal lock: %w", err)
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

func writeAtomic(path string, entries []Entry) (err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create release journal directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".release-*.jsonl")
	if err != nil {
		return fmt.Errorf("create release journal: %w", err)
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
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return fmt.Errorf("encode release journal: %w", err)
		}
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace release journal: %w", err)
	}
	return nil
}
