package cleanup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// DefaultDir is the restartable cleanup ledger directory relative to a
// project root. Process traps are last-mile; this directory survives them.
const DefaultDir = ".magelift/cleanup"

// Load reads one ledger document. The file is the authority for interrupted
// cleanup, not a cache of a still-running process.
func Load(path string) (sdk.CleanupLedger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return sdk.CleanupLedger{}, errors.New("cleanup ledger path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sdk.CleanupLedger{}, fmt.Errorf("read cleanup ledger: %w", err)
	}
	var ledger sdk.CleanupLedger
	if err := json.Unmarshal(data, &ledger); err != nil {
		return sdk.CleanupLedger{}, fmt.Errorf("decode cleanup ledger: %w", err)
	}
	if err := sdk.ValidateCleanupLedger(ledger); err != nil {
		return sdk.CleanupLedger{}, err
	}
	return ledger, nil
}

// Save writes one ledger document atomically. Callers must persist before
// provider mutation and after every successful delete.
func Save(path string, ledger sdk.CleanupLedger) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("cleanup ledger path is required")
	}
	if err := sdk.ValidateCleanupLedger(ledger); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create cleanup ledger directory: %w", err)
	}
	data, err := json.MarshalIndent(ledger, "", "  ")
	if err != nil {
		return fmt.Errorf("encode cleanup ledger: %w", err)
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp.")
	if err != nil {
		return fmt.Errorf("create cleanup ledger tempfile: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write cleanup ledger tempfile: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod cleanup ledger tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close cleanup ledger tempfile: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace cleanup ledger: %w", err)
	}
	return nil
}

// ListDir returns every valid ledger in dir. Invalid files fail closed so an
// operator can see a corrupt document instead of silently skipping a leak.
func ListDir(dir string) ([]string, []sdk.CleanupLedger, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, nil, errors.New("cleanup ledger directory is required")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("read cleanup ledger directory: %w", err)
	}
	paths := make([]string, 0, len(entries))
	ledgers := make([]sdk.CleanupLedger, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		ledger, err := Load(path)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", path, err)
		}
		paths = append(paths, path)
		ledgers = append(ledgers, ledger)
	}
	return paths, ledgers, nil
}

// Claim inserts or updates one intended resource and persists the document
// before the caller is allowed to mutate the provider.
func Claim(path string, ledger sdk.CleanupLedger, resource sdk.CleanupResource) (sdk.CleanupLedger, error) {
	if resource.Status == "" {
		resource.Status = sdk.CleanupStatusIntended
	}
	if resource.Rank == 0 {
		resource.Rank = sdk.DefaultCleanupRank(resource.Kind, resource.Role)
	}
	replaced := false
	for i, existing := range ledger.Resources {
		if sameCleanupClaim(existing, resource) {
			ledger.Resources[i] = mergeCleanupClaim(existing, resource)
			replaced = true
			break
		}
	}
	if !replaced {
		ledger.Resources = append(ledger.Resources, resource)
	}
	if err := Save(path, ledger); err != nil {
		return sdk.CleanupLedger{}, err
	}
	return ledger, nil
}

// RecordIdentity binds a provider identity onto an intended or claimed
// resource after create succeeds.
func RecordIdentity(path string, ledger sdk.CleanupLedger, kind, name, identity string) (sdk.CleanupLedger, error) {
	found := false
	for i, existing := range ledger.Resources {
		if existing.Kind != kind {
			continue
		}
		if existing.Name != name && existing.Identity != identity {
			continue
		}
		existing.Identity = identity
		if existing.Name == "" {
			existing.Name = name
		}
		if existing.Status == sdk.CleanupStatusIntended {
			existing.Status = sdk.CleanupStatusClaimed
		}
		ledger.Resources[i] = existing
		found = true
		break
	}
	if !found {
		return sdk.CleanupLedger{}, fmt.Errorf("cleanup ledger has no %s named %q to record", kind, name)
	}
	if err := Save(path, ledger); err != nil {
		return sdk.CleanupLedger{}, err
	}
	return ledger, nil
}

func sameCleanupClaim(existing, resource sdk.CleanupResource) bool {
	if existing.Identity != "" && resource.Identity != "" && existing.Identity == resource.Identity {
		return true
	}
	return existing.Kind == resource.Kind && existing.Name != "" && existing.Name == resource.Name
}

func mergeCleanupClaim(existing, resource sdk.CleanupResource) sdk.CleanupResource {
	if resource.Identity != "" {
		existing.Identity = resource.Identity
	}
	if resource.Name != "" {
		existing.Name = resource.Name
	}
	if resource.Role != "" {
		existing.Role = resource.Role
	}
	if resource.Rank != 0 {
		existing.Rank = resource.Rank
	}
	if resource.Status != "" {
		existing.Status = resource.Status
	}
	if resource.Detail != "" {
		existing.Detail = resource.Detail
	}
	if existing.Status == sdk.CleanupStatusIntended && existing.Identity != "" {
		existing.Status = sdk.CleanupStatusClaimed
	}
	return existing
}
