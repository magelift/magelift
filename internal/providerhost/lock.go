package providerhost

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
)

const (
	SchemaVersion = 1
	SDKAPIVersion = "v1"
	// ProtocolV1Marker is the required per-entry protocol marker. Only
	// typed net/rpc providers load; anything else is refused.
	ProtocolV1Marker = "magelift-v1"
)

var digestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

var (
	ErrUnsigned            = errors.New("provider artifact is unsigned")
	ErrDigestRequired      = errors.New("provider artifact digest is required")
	ErrChecksumMismatch    = errors.New("provider artifact checksum mismatch")
	ErrUnsupportedSchema   = errors.New("unsupported magelift.providers.lock schema")
	ErrUnsupportedAPI      = errors.New("unsupported provider SDK API version")
	ErrUnknownProvider     = errors.New("provider is not in magelift.providers.lock")
	ErrUnsupportedProtocol = errors.New("unsupported provider protocol")
	ErrNotDownloadable     = errors.New("provider artifact has no download URL")
	ErrCacheMiss           = errors.New("provider artifact is not in the cache")
)

// Lockfile is magelift.providers.lock. Digests are sha256 of the subprocess
// binary. Cosign identity and issuer are required; unsigned entries are refused.
type Lockfile struct {
	SchemaVersion int                 `json:"schemaVersion"`
	SDKAPIVersion string              `json:"sdkAPIVersion"`
	Providers     map[string]Artifact `json:"providers"`
}

type Artifact struct {
	Name     string      `json:"name"`
	Version  string      `json:"version"`
	Protocol string      `json:"protocol"`
	Digest   string      `json:"digest"`
	URL      string      `json:"url,omitempty"`
	Cosign   CosignTrust `json:"cosign"`
}

type CosignTrust struct {
	Identity string `json:"identity"`
	Issuer   string `json:"issuer"`
	Bundle   string `json:"bundle,omitempty"`
}

func ParseLock(r io.Reader) (Lockfile, error) {
	if r == nil {
		return Lockfile{}, errors.New("lockfile reader is required")
	}
	var lock Lockfile
	dec := json.NewDecoder(io.LimitReader(r, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&lock); err != nil {
		return Lockfile{}, fmt.Errorf("decode magelift.providers.lock: %w", err)
	}
	if lock.SchemaVersion != SchemaVersion {
		return Lockfile{}, fmt.Errorf("%w: got %d, want %d (regenerate the lockfile with protocol %q entries)", ErrUnsupportedSchema, lock.SchemaVersion, SchemaVersion, ProtocolV1Marker)
	}
	if strings.TrimSpace(lock.SDKAPIVersion) != SDKAPIVersion {
		return Lockfile{}, fmt.Errorf("%w: got %q", ErrUnsupportedAPI, lock.SDKAPIVersion)
	}
	if len(lock.Providers) == 0 {
		return Lockfile{}, errors.New("magelift.providers.lock has no providers")
	}
	for id, artifact := range lock.Providers {
		if err := artifact.validate(); err != nil {
			return Lockfile{}, fmt.Errorf("provider %q: %w", id, err)
		}
	}
	return lock, nil
}

func (l Lockfile) Artifact(id string) (Artifact, error) {
	artifact, ok := l.Providers[strings.TrimSpace(id)]
	if !ok {
		return Artifact{}, fmt.Errorf("%w: %s", ErrUnknownProvider, id)
	}
	return artifact, nil
}

func (a Artifact) validate() error {
	if strings.TrimSpace(a.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(a.Version) == "" {
		return errors.New("version is required")
	}
	if strings.TrimSpace(a.Protocol) != ProtocolV1Marker {
		return fmt.Errorf("%w: got %q, want %q", ErrUnsupportedProtocol, a.Protocol, ProtocolV1Marker)
	}
	if !digestPattern.MatchString(strings.TrimSpace(a.Digest)) {
		if strings.TrimSpace(a.Digest) == "" {
			return ErrDigestRequired
		}
		return fmt.Errorf("%w: %s", ErrDigestRequired, a.Digest)
	}
	if strings.TrimSpace(a.Cosign.Identity) == "" || strings.TrimSpace(a.Cosign.Issuer) == "" {
		return ErrUnsigned
	}
	return nil
}
