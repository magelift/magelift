// Package statearchive contains the provider-neutral snapshot protocol used
// by DIY object-storage state backends. Provider packages only translate their
// SDK calls into Store; backup completeness, restore safety, and object
// filtering live here once.
package statearchive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const CompleteMarker = ".magelift-complete"
const ManifestObject = ".magelift-manifest.json"

var backupIDPattern = regexp.MustCompile(`^[0-9]{8}T[0-9]{6}\.[0-9]{9}Z$`)

// Store is the smallest object-storage boundary required by the archive
// protocol. It deliberately contains no AWS, GCP, S3, or GCS types.
type Store interface {
	List(context.Context, string) ([]string, error)
	Read(context.Context, string) ([]byte, error)
	Copy(context.Context, string, string) error
	Put(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

// BatchDeleter is an optional optimization for providers that can remove a
// bounded batch in one control-plane request. The archive remains correct
// with the single-object Delete fallback; certification runs use this seam to
// keep cleanup latency proportional to API batches rather than object count.
type BatchDeleter interface {
	DeleteMany(context.Context, []string) error
}

type Archive struct {
	store        Store
	backupRoot   string
	locationRoot string
	now          func() time.Time
}

type BackupResult struct {
	ID             string `json:"id" yaml:"id"`
	Prefix         string `json:"prefix" yaml:"prefix"`
	Objects        int    `json:"objects" yaml:"objects"`
	Bytes          int64  `json:"bytes" yaml:"bytes"`
	ManifestDigest string `json:"manifestDigest" yaml:"manifestDigest"`
}

type RestoreResult struct {
	ID             string `json:"id" yaml:"id"`
	Prefix         string `json:"prefix" yaml:"prefix"`
	Objects        int    `json:"objects" yaml:"objects"`
	Bytes          int64  `json:"bytes" yaml:"bytes"`
	ManifestDigest string `json:"manifestDigest" yaml:"manifestDigest"`
}

type manifest struct {
	Version int              `json:"version"`
	Objects []manifestObject `json:"objects"`
	Digest  string           `json:"digest"`
}

type manifestObject struct {
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

func New(store Store, backupRoot, locationRoot string) (*Archive, error) {
	if store == nil {
		return nil, errors.New("state archive store is required")
	}
	if strings.TrimSpace(backupRoot) == "" || !strings.HasSuffix(backupRoot, "/") || strings.ContainsAny(backupRoot, "\r\n\x00") {
		return nil, errors.New("state archive backup root must be a non-empty single-line prefix")
	}
	if strings.ContainsAny(locationRoot, "\r\n\x00") {
		return nil, errors.New("state archive location root must be a single-line reference")
	}
	return &Archive{store: store, backupRoot: backupRoot, locationRoot: locationRoot, now: time.Now}, nil
}

// SetClock is intended for deterministic provider adapter tests. Production
// callers should use the default UTC-independent wall clock.
func (a *Archive) SetClock(now func() time.Time) {
	if a != nil && now != nil {
		a.now = now
	}
}

func (a *Archive) Backup(ctx context.Context) (BackupResult, error) {
	if a == nil || a.store == nil {
		return BackupResult{}, errors.New("state archive is not configured")
	}
	if ctx == nil {
		return BackupResult{}, errors.New("state archive backup context is required")
	}
	id := a.now().UTC().Format("20060102T150405.000000000Z")
	prefix := a.backupRoot + id + "/"
	existing, err := a.store.List(ctx, prefix)
	if err != nil {
		return BackupResult{}, fmt.Errorf("check state backup identity: %w", err)
	}
	if len(existing) != 0 {
		return BackupResult{}, fmt.Errorf("state backup identity %q already exists", id)
	}
	keys, err := a.store.List(ctx, "")
	if err != nil {
		return BackupResult{}, fmt.Errorf("list state objects: %w", err)
	}
	sort.Strings(keys)
	count := 0
	var totalBytes int64
	objects := make([]manifestObject, 0, len(keys))
	for _, key := range keys {
		if a.reservedKey(key) {
			continue
		}
		body, err := a.store.Read(ctx, key)
		if err != nil {
			return BackupResult{}, fmt.Errorf("read state object %q: %w", key, err)
		}
		if err := a.store.Copy(ctx, key, prefix+key); err != nil {
			return BackupResult{}, fmt.Errorf("backup state object %q: %w", key, err)
		}
		copied, err := a.store.Read(ctx, prefix+key)
		if err != nil {
			return BackupResult{}, fmt.Errorf("verify backed-up state object %q: %w", key, err)
		}
		if int64(len(copied)) != int64(len(body)) || digest(copied) != digest(body) {
			return BackupResult{}, fmt.Errorf("backed-up state object %q changed during copy", key)
		}
		count++
		totalBytes += int64(len(body))
		objects = append(objects, manifestObject{Key: key, SHA256: digest(body), Bytes: int64(len(body))})
	}
	if count == 0 {
		return BackupResult{}, errors.New("state bucket contains no durable state objects")
	}
	manifestBody, manifestDigest, err := sealManifest(manifest{Version: 1, Objects: objects})
	if err != nil {
		return BackupResult{}, fmt.Errorf("encode state backup manifest: %w", err)
	}
	if err := a.store.Put(ctx, prefix+ManifestObject, manifestBody); err != nil {
		return BackupResult{}, fmt.Errorf("write state backup manifest: %w", err)
	}
	if err := a.store.Put(ctx, prefix+CompleteMarker, []byte("complete\n")); err != nil {
		return BackupResult{}, fmt.Errorf("complete state backup: %w", err)
	}
	return BackupResult{ID: id, Prefix: a.locationRoot + prefix, Objects: count, Bytes: totalBytes, ManifestDigest: manifestDigest}, nil
}

func (a *Archive) Restore(ctx context.Context, id string) (RestoreResult, error) {
	if a == nil || a.store == nil {
		return RestoreResult{}, errors.New("state archive is not configured")
	}
	if ctx == nil {
		return RestoreResult{}, errors.New("state archive restore context is required")
	}
	if !backupIDPattern.MatchString(id) {
		return RestoreResult{}, errors.New("state backup ID is invalid")
	}
	prefix := a.backupRoot + id + "/"
	backupKeys, err := a.store.List(ctx, prefix)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("list state backup: %w", err)
	}
	if len(backupKeys) == 0 {
		return RestoreResult{}, errors.New("state backup was not found")
	}
	sort.Strings(backupKeys)
	completeKey := prefix + CompleteMarker
	manifestKey := prefix + ManifestObject
	complete := false
	manifestFound := false
	backupObjects := make(map[string]manifestObject, len(backupKeys))
	for _, key := range backupKeys {
		if key == completeKey {
			complete = true
			continue
		}
		if key == manifestKey {
			manifestFound = true
			continue
		}
		target := strings.TrimPrefix(key, prefix)
		if target == key || target == "" || a.reservedKey(target) {
			return RestoreResult{}, errors.New("state backup contains an invalid object")
		}
		if _, exists := backupObjects[target]; exists {
			return RestoreResult{}, fmt.Errorf("state backup contains duplicate object %q", target)
		}
		backupObjects[target] = manifestObject{Key: target}
	}
	if !complete {
		return RestoreResult{}, errors.New("state backup is incomplete")
	}
	if !manifestFound {
		return RestoreResult{}, errors.New("state backup manifest is missing")
	}
	marker, err := a.store.Read(ctx, completeKey)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("read state backup completion marker: %w", err)
	}
	if string(marker) != "complete\n" {
		return RestoreResult{}, errors.New("state backup completion marker is invalid")
	}
	manifestBody, err := a.store.Read(ctx, manifestKey)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("read state backup manifest: %w", err)
	}
	verifiedManifest, manifestDigest, err := openManifest(manifestBody)
	if err != nil {
		return RestoreResult{}, fmt.Errorf("verify state backup manifest: %w", err)
	}
	if len(verifiedManifest.Objects) == 0 {
		return RestoreResult{}, errors.New("state backup contains no durable state objects")
	}
	if len(verifiedManifest.Objects) != len(backupObjects) {
		return RestoreResult{}, errors.New("state backup manifest does not match its object listing")
	}
	manifestObjects := make(map[string]struct{}, len(verifiedManifest.Objects))
	var totalBytes int64
	for _, object := range verifiedManifest.Objects {
		if object.Key == "" || a.reservedKey(object.Key) {
			return RestoreResult{}, errors.New("state backup manifest contains an invalid object")
		}
		if _, exists := manifestObjects[object.Key]; exists {
			return RestoreResult{}, fmt.Errorf("state backup manifest contains duplicate object %q", object.Key)
		}
		manifestObjects[object.Key] = struct{}{}
		listed, exists := backupObjects[object.Key]
		if !exists || listed.Key != object.Key {
			return RestoreResult{}, fmt.Errorf("state backup manifest references missing object %q", object.Key)
		}
		body, err := a.store.Read(ctx, prefix+object.Key)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("read state backup object %q: %w", object.Key, err)
		}
		if int64(len(body)) != object.Bytes || digest(body) != object.SHA256 {
			return RestoreResult{}, fmt.Errorf("state backup object %q failed checksum verification", object.Key)
		}
		totalBytes += int64(len(body))
	}

	currentKeys, err := a.store.List(ctx, "")
	if err != nil {
		return RestoreResult{}, fmt.Errorf("list current state: %w", err)
	}
	var stale []string
	for _, key := range currentKeys {
		if a.reservedKey(key) {
			continue
		}
		if _, exists := backupObjects[key]; !exists {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)

	// Copy the complete snapshot before deleting stale current state. If a
	// restore copy fails, the original state remains available for recovery.
	for _, object := range verifiedManifest.Objects {
		target := object.Key
		if err := a.store.Copy(ctx, prefix+target, target); err != nil {
			return RestoreResult{}, fmt.Errorf("restore state object %q: %w", target, err)
		}
		body, err := a.store.Read(ctx, target)
		if err != nil {
			return RestoreResult{}, fmt.Errorf("verify restored state object %q: %w", target, err)
		}
		if int64(len(body)) != object.Bytes || digest(body) != object.SHA256 {
			return RestoreResult{}, fmt.Errorf("restored state object %q failed checksum verification", target)
		}
	}
	if len(stale) > 0 {
		if deleter, ok := a.store.(BatchDeleter); ok {
			if err := deleter.DeleteMany(ctx, stale); err != nil {
				return RestoreResult{}, fmt.Errorf("delete stale state objects: %w", err)
			}
		} else {
			for _, key := range stale {
				if err := a.store.Delete(ctx, key); err != nil {
					return RestoreResult{}, fmt.Errorf("delete stale state object %q: %w", key, err)
				}
			}
		}
	}
	return RestoreResult{ID: id, Prefix: a.locationRoot + prefix, Objects: len(backupObjects), Bytes: totalBytes, ManifestDigest: manifestDigest}, nil
}

func (a *Archive) reservedKey(key string) bool {
	return key == "locks" || strings.HasPrefix(key, "locks/") || key == "backups" || strings.HasPrefix(key, "backups/") || key == CompleteMarker || key == ManifestObject || strings.HasPrefix(key, a.backupRoot)
}

func digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func sealManifest(value manifest) ([]byte, string, error) {
	value.Digest = ""
	unsigned, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	value.Digest = digest(unsigned)
	signed, err := json.Marshal(value)
	if err != nil {
		return nil, "", err
	}
	return signed, value.Digest, nil
}

func openManifest(body []byte) (manifest, string, error) {
	var value manifest
	if err := json.Unmarshal(body, &value); err != nil {
		return manifest{}, "", err
	}
	if value.Version != 1 || value.Digest == "" {
		return manifest{}, "", errors.New("unsupported or unsigned state backup manifest")
	}
	expected := value.Digest
	value.Digest = ""
	unsigned, err := json.Marshal(value)
	if err != nil {
		return manifest{}, "", err
	}
	if digest(unsigned) != expected {
		return manifest{}, "", errors.New("state backup manifest checksum mismatch")
	}
	return value, expected, nil
}
