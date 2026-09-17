package cli

import (
	"context"
	"fmt"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
)

// gcpProviderID selects the plugin-backed target. GCP has no in-process
// implementation left: every GCP command dials the verified provider
// plugin, and dial failure fails the command (no fallback).
const gcpProviderID = "gcp"

// defaultLoadProvider verifies the installed provider artifact through the
// shared resolver: project locks control the version run (cache), the
// beside-CLI lock loads the beside-CLI binary. Verification failures stay
// loud; only a missing binary names the install command.
func (o *options) defaultLoadProvider(ctx context.Context, provider string) (providerhost.Loaded, error) {
	executable := ""
	if o.executable != nil {
		if path, err := o.executable(); err == nil {
			executable = path
		}
	}
	resolved, err := providerhost.Resolve(provider, providerhost.ResolveOptions{ExecutablePath: executable})
	if err != nil {
		return providerhost.Loaded{}, err
	}
	return providerhost.Load(ctx, providerhost.Options{
		Mode:       providerhost.ModeSubprocess,
		Provider:   provider,
		LockPath:   resolved.LockPath,
		BinaryPath: resolved.Binary,
		BundlePath: resolved.Bundle,
		Verifier:   providerhost.NewCosignVerifier(),
	})
}

// defaultDialProvider starts the verified provider subprocess and tracks the
// session for closeProviderSessions. Callers defer closeProviderSessions
// after newBackend returns.
func (o *options) defaultDialProvider(ctx context.Context, binary string) (*providerhost.Client, error) {
	client, err := providerhost.Dial(ctx, binary, providerhost.DialOptions{})
	if err != nil {
		return nil, err
	}
	o.providerSessions = append(o.providerSessions, client)
	return client, nil
}

func (o *options) closeProviderSessions() {
	for _, session := range o.providerSessions {
		if session != nil {
			session.Close()
		}
	}
	o.providerSessions = nil
}

// defaultNewBackend serves GCP from the verified provider plugin and every
// other target from the existing in-process backend.
func (o *options) defaultNewBackend(ctx context.Context, planned platform.PlannedStack, backendURL string) (infrastructureBackend, error) {
	if planned.Provider() == gcpProviderID {
		return o.pluginBackend(ctx, planned)
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

func (o *options) pluginBackend(ctx context.Context, planned platform.PlannedStack) (infrastructureBackend, error) {
	shim, ok := providerhost.AsShimPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP backend requires a plugin-backed plan, got %T", planned)
	}
	if o.loadProvider == nil || o.dialProvider == nil {
		return nil, fmt.Errorf("GCP provider loading is not configured")
	}
	loaded, err := o.loadProvider(ctx, string(planned.Provider()))
	if err != nil {
		return nil, err
	}
	client, err := o.dialProvider(ctx, loaded.Binary)
	if err != nil {
		return nil, err
	}
	return providerhost.NewPluginBackend(client, shim.Envelope(), shim.StoredPlan())
}

// providerProvenance describes one plugin provider for `extensions list`.
// Mode is "subprocess" when a verified artifact is installed beside the CLI
// and "not-installed" otherwise (GCP has no in-process implementation, so
// there is nothing to fall back to); no subprocess starts to answer the
// listing.
type providerProvenance struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	Digest  string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Mode    string `json:"mode" yaml:"mode"`
}

func (o *options) subprocessProvenance(ctx context.Context) []providerProvenance {
	name := providerhost.ExtractName(gcpProviderID)
	provenance := providerProvenance{Name: name, Mode: "not-installed"}
	if o.loadProvider == nil {
		return []providerProvenance{provenance}
	}
	loaded, err := o.loadProvider(ctx, gcpProviderID)
	if err != nil {
		return []providerProvenance{provenance}
	}
	provenance.Version = loaded.Artifact.Version
	provenance.Digest = loaded.Artifact.Digest
	provenance.Mode = "subprocess"
	return []providerProvenance{provenance}
}
