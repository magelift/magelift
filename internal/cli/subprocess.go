package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	// subprocessProofProvider and subprocessProofRuntime select the one
	// adapter proven over a subprocess in v1. Every other cell stays
	// in-process.
	subprocessProofProvider = "gcp"
	subprocessProofRuntime  = "gke-autopilot"
	providerLockFilename    = "magelift.providers.lock"
)

// cosignBlobVerifier adapts cosign.Client to the providerhost blob
// verifier. Both sides take bundle-first order, so this is a pure
// pass-through; the parameter names pin that order.
type cosignBlobVerifier struct {
	client *cosign.Client
}

func (v cosignBlobVerifier) VerifyBlob(ctx context.Context, bundlePath, binaryPath string, options cosign.VerifyOptions) error {
	if v.client == nil {
		return cosign.ErrRunnerRequired
	}
	return v.client.VerifyBlob(ctx, bundlePath, binaryPath, options)
}

// defaultLoadProvider verifies the installed provider artifact that ships
// beside the CLI executable: the lockfile plus the provider binary plus its
// Cosign bundle, all in the executable directory.
func (o *options) defaultLoadProvider(ctx context.Context, provider string) (providerhost.Loaded, error) {
	executable := ""
	if o.executable != nil {
		if path, err := o.executable(); err == nil {
			executable = path
		}
	}
	dir := "."
	if executable != "" {
		dir = filepath.Dir(executable)
	}
	binary := filepath.Join(dir, providerhost.ExtractName(provider))
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	lockPath := filepath.Join(dir, providerLockFilename)
	artifact, err := readLockArtifact(lockPath, provider)
	if err != nil {
		return providerhost.Loaded{}, err
	}
	warnProviderSkew(o.stderr, provider, artifact.Version)
	bundle := artifact.Cosign.Bundle
	if bundle == "" {
		return providerhost.Loaded{}, fmt.Errorf("%w: %s", providerhost.ErrUnsigned, provider)
	}
	return providerhost.Load(ctx, providerhost.Options{
		Mode:       providerhost.ModeSubprocess,
		Provider:   provider,
		LockPath:   lockPath,
		BinaryPath: binary,
		BundlePath: filepath.Join(dir, bundle),
		Verifier:   cosignBlobVerifier{client: cosign.New()},
	})
}

// warnProviderSkew names a version mismatch between the CLI and the
// installed provider lockfile. magelift upgrade replaces the CLI only,
// so a stale provider would otherwise run silently; v1's frozen RPCs
// keep the skew functional, and this warning keeps it visible. Dev
// builds and versionless locks stay quiet because their versions are
// meaningless.
func warnProviderSkew(stderr io.Writer, provider, lockVersion string) {
	if stderr == nil {
		return
	}
	if strings.TrimSpace(lockVersion) == "" || Version == "dev" {
		return
	}
	if lockVersion == Version || strings.TrimPrefix(lockVersion, "v") == strings.TrimPrefix(Version, "v") {
		return
	}
	fmt.Fprintf(stderr, "warning: provider %s version %s differs from CLI version %s; reinstall the provider artifacts beside the CLI to match\n", provider, lockVersion, Version)
}

func readLockArtifact(lockPath, provider string) (providerhost.Artifact, error) {
	file, err := os.Open(lockPath)
	if err != nil {
		return providerhost.Artifact{}, fmt.Errorf("open provider lockfile: %w", err)
	}
	defer file.Close()
	lock, err := providerhost.ParseLock(file)
	if err != nil {
		return providerhost.Artifact{}, err
	}
	return lock.Artifact(provider)
}

// defaultDialProvider starts the verified provider subprocess and tracks the
// session for closeProviderSessions. Callers defer closeProviderSessions
// after newBackend returns.
func (o *options) defaultDialProvider(ctx context.Context, binary string) (providerhost.API, error) {
	session, err := providerhost.Dial(ctx, binary)
	if err != nil {
		return nil, err
	}
	o.providerSessions = append(o.providerSessions, session)
	return session, nil
}

func (o *options) closeProviderSessions() {
	for _, session := range o.providerSessions {
		if session != nil {
			session.Close()
		}
	}
	o.providerSessions = nil
}

// defaultNewBackend serves the gcp/gke-autopilot proof cell from a verified
// subprocess when one is installed, and every other case from the existing
// in-process backend. A failed subprocess attempt never fails the command:
// it falls back with a stderr notice naming the artifact and the expected
// install path.
func (o *options) defaultNewBackend(ctx context.Context, planned platform.PlannedStack, backendURL string) (infrastructureBackend, error) {
	if backend, ok := o.subprocessBackend(ctx, planned, backendURL); ok {
		return backend, nil
	}
	module, found := o.modules.Module(planned.Provider(), planned.Runtime())
	if !found {
		return nil, fmt.Errorf("no stack module for %q/%q", planned.Provider(), planned.Runtime())
	}
	program, err := module.Program(planned)
	if err != nil {
		return nil, err
	}
	pulumiStack, err := automation.NewInlineStackWithBackend(ctx, planned.StackName(), program, backendURL)
	if err != nil {
		return nil, err
	}
	return automation.NewPulumiBackend(pulumiStack), nil
}

func (o *options) subprocessBackend(ctx context.Context, planned platform.PlannedStack, backendURL string) (infrastructureBackend, bool) {
	if planned.Provider() != subprocessProofProvider || planned.Runtime() != subprocessProofRuntime {
		return nil, false
	}
	withSpec, ok := planned.(providerhost.OpaqueSpecProvider)
	if !ok || withSpec.OpaquePlanSpec() == nil {
		return nil, false
	}
	notice := func(reason string) {
		fmt.Fprintf(o.stderr, "subprocess provider %s unavailable (%s); using in-process backend\n", providerhost.ExtractName(string(planned.Provider())), reason)
	}
	if o.loadProvider == nil || o.dialProvider == nil {
		notice("provider loading is not configured")
		return nil, false
	}
	loaded, err := o.loadProvider(ctx, string(planned.Provider()))
	if err != nil {
		notice(err.Error())
		return nil, false
	}
	api, err := o.dialProvider(ctx, loaded.Binary)
	if err != nil {
		notice(err.Error())
		return nil, false
	}
	plan := sdk.ModulePlan{
		StackName:        planned.StackName(),
		Provider:         planned.Provider(),
		Runtime:          planned.Runtime(),
		Project:          planned.Project(),
		Environment:      planned.Environment(),
		Region:           planned.Region(),
		EnvironmentClass: planned.EnvironmentClass(),
		Protected:        planned.Protected(),
		ImageDigest:      planned.ImageDigest(),
		Target:           planned.TargetDescriptor(),
		Opaque:           withSpec.OpaquePlanSpec(),
	}
	if planned.CertificationTier() == platform.TierCertified {
		plan.Tier = sdk.ExtensionTierCertified
	} else {
		plan.Tier = sdk.ExtensionTierExperimental
	}
	fmt.Fprintf(o.stderr, "using subprocess provider %s %s\n", loaded.Artifact.Name, loaded.Artifact.Version)
	return providerhost.NewSubprocessBackend(api, plan, backendURL), true
}

// providerProvenance describes one proof provider for `extensions list`.
// Mode is "subprocess" when a verified artifact is installed beside the CLI
// and "in-process" otherwise; no subprocess starts to answer the listing.
type providerProvenance struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	Digest  string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Mode    string `json:"mode" yaml:"mode"`
}

func (o *options) subprocessProvenance(ctx context.Context) []providerProvenance {
	name := providerhost.ExtractName(subprocessProofProvider)
	provenance := providerProvenance{Name: name, Mode: "in-process"}
	if o.loadProvider == nil {
		return []providerProvenance{provenance}
	}
	loaded, err := o.loadProvider(ctx, subprocessProofProvider)
	if err != nil {
		return []providerProvenance{provenance}
	}
	provenance.Version = loaded.Artifact.Version
	provenance.Digest = loaded.Artifact.Digest
	provenance.Mode = "subprocess"
	return []providerProvenance{provenance}
}
