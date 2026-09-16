// Package recovery contains provider-neutral algorithms shared by durable
// recovery adapters. Provider packages supply object-store calls and keep
// cloud SDK request/response models at the edge.
package recovery

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

// ObjectArchiveManifest is the portable description of an ownership-scoped
// object backup. It contains keys, sizes, and provider-returned opaque
// validators only; object contents and secret values never enter the proof.
type ObjectArchiveManifest struct {
	Version         int                  `json:"version"`
	DataClass       string               `json:"dataClass"`
	FixtureID       string               `json:"fixtureId"`
	OwnershipMarker string               `json:"ownershipMarker"`
	SourceBucket    string               `json:"sourceBucket"`
	SourcePrefix    string               `json:"sourcePrefix"`
	ArchiveBucket   string               `json:"archiveBucket"`
	ArchivePrefix   string               `json:"archivePrefix"`
	Entries         []ObjectArchiveEntry `json:"entries"`
	Digest          string               `json:"digest"`
}

type ObjectArchiveEntry struct {
	SourceKey  string `json:"sourceKey"`
	ArchiveKey string `json:"archiveKey"`
	Size       int64  `json:"size"`
	ETag       string `json:"etag,omitempty"`
}

// SealObjectManifest adds a deterministic digest over the unsigned manifest.
func SealObjectManifest(manifest ObjectArchiveManifest) ([]byte, string, error) {
	manifest.Digest = ""
	unsigned, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	manifest.Digest = Digest(unsigned)
	signed, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	return signed, manifest.Digest, nil
}

// OpenObjectManifest verifies and decodes a sealed object manifest.
func OpenObjectManifest(body []byte) (ObjectArchiveManifest, string, error) {
	var manifest ObjectArchiveManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return ObjectArchiveManifest{}, "", err
	}
	if manifest.Version <= 0 || strings.TrimSpace(manifest.Digest) == "" || len(manifest.Entries) == 0 {
		return ObjectArchiveManifest{}, "", errors.New("object archive manifest is unsigned or empty")
	}
	expected := manifest.Digest
	manifest.Digest = ""
	unsigned, err := json.Marshal(manifest)
	if err != nil {
		return ObjectArchiveManifest{}, "", err
	}
	if Digest(unsigned) != expected {
		return ObjectArchiveManifest{}, "", errors.New("object archive manifest digest mismatch")
	}
	manifest.Digest = expected
	return manifest, expected, nil
}

func Digest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

// RelativeObjectKey keeps the source prefix out of an archive object's
// relative path while preserving nested object names.
func RelativeObjectKey(key, prefix string) string {
	key = strings.TrimPrefix(key, "/")
	prefix = strings.Trim(strings.TrimSuffix(prefix, "/"), "/")
	if prefix == "" {
		return key
	}
	if key == prefix {
		return ""
	}
	if strings.HasPrefix(key, prefix+"/") {
		return strings.TrimPrefix(key, prefix+"/")
	}
	return key
}

// SafeArchiveKey rejects traversal-like object names before a provider copies
// them into an ownership-scoped archive or restore prefix.
func SafeArchiveKey(prefix, key string) string {
	key = strings.TrimPrefix(key, "/")
	key = path.Clean(key)
	if key == "." || key == ".." || strings.HasPrefix(key, "../") {
		return ""
	}
	result := strings.Trim(prefix, "/") + "/" + key
	if strings.TrimSpace(result) == "" || strings.ContainsAny(result, "\r\n\x00") {
		return ""
	}
	return result
}

func ValidateObjectManifest(manifest ObjectArchiveManifest, dataClass, fixtureID, ownershipMarker string) error {
	if manifest.Version <= 0 || manifest.DataClass != dataClass || manifest.FixtureID != fixtureID || manifest.OwnershipMarker != ownershipMarker {
		return errors.New("object archive manifest does not match the operation scope")
	}
	if strings.TrimSpace(manifest.ArchiveBucket) == "" || strings.TrimSpace(manifest.ArchivePrefix) == "" || len(manifest.Entries) == 0 {
		return errors.New("object archive manifest is incomplete")
	}
	seen := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		if strings.TrimSpace(entry.SourceKey) == "" || strings.TrimSpace(entry.ArchiveKey) == "" || entry.Size < 0 {
			return fmt.Errorf("object archive manifest contains an invalid entry %q", entry.ArchiveKey)
		}
		if _, ok := seen[entry.ArchiveKey]; ok {
			return fmt.Errorf("object archive manifest contains duplicate entry %q", entry.ArchiveKey)
		}
		seen[entry.ArchiveKey] = struct{}{}
	}
	return nil
}
