package providerhost

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Mode int

const (
	ModeInProcess Mode = iota
	ModeSubprocess
)

type Options struct {
	Mode       Mode
	Provider   string
	LockPath   string
	BinaryPath string
	BundlePath string
	Verifier   BlobVerifier
}

type Loaded struct {
	Mode     Mode
	Provider string
	Artifact Artifact
	Binary   string
}

// Load returns an in-process handle for tests and Floci, or a verified
// subprocess identity for the published CLI. It never calls plugin.Open.
// ModeSubprocess verifies digest and Cosign; Dial starts the go-plugin
// process. Magento cells stay on the in-process path until a GCP module
// extract exists.
func Load(ctx context.Context, opts Options) (Loaded, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	provider := strings.TrimSpace(opts.Provider)
	if provider == "" {
		return Loaded{}, errors.New("provider is required")
	}
	if opts.Mode == ModeInProcess {
		return Loaded{Mode: ModeInProcess, Provider: provider}, nil
	}
	if opts.Mode != ModeSubprocess {
		return Loaded{}, fmt.Errorf("unsupported provider host mode %d", opts.Mode)
	}
	file, err := os.Open(opts.LockPath)
	if err != nil {
		return Loaded{}, fmt.Errorf("open magelift.providers.lock: %w", err)
	}
	defer file.Close()
	lock, err := ParseLock(file)
	if err != nil {
		return Loaded{}, err
	}
	artifact, err := lock.Artifact(provider)
	if err != nil {
		return Loaded{}, err
	}
	bundlePath := opts.BundlePath
	if strings.TrimSpace(bundlePath) == "" {
		if strings.TrimSpace(artifact.Cosign.Bundle) == "" {
			return Loaded{}, fmt.Errorf("%w: %s", ErrUnsigned, provider)
		}
		bundlePath = filepath.Join(filepath.Dir(opts.LockPath), artifact.Cosign.Bundle)
	}
	if err := VerifyLocal(ctx, artifact, opts.BinaryPath, bundlePath, opts.Verifier); err != nil {
		return Loaded{}, err
	}
	return Loaded{
		Mode:     ModeSubprocess,
		Provider: provider,
		Artifact: artifact,
		Binary:   opts.BinaryPath,
	}, nil
}

// ExtractName is the release asset basename for a first-party provider.
func ExtractName(provider string) string {
	return "magelift-provider-" + strings.TrimSpace(provider)
}
