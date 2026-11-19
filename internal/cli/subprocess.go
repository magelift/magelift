package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

// defaultLoadProvider acquires the target's signed release when first activated.
func (o *options) defaultLoadProvider(ctx context.Context, provider string) (providerhost.Loaded, error) {
	if provider != "gcp" && provider != "azure" {
		return o.loadInstalledProvider(ctx, provider)
	}
	return providerhost.Acquire(ctx, provider, Version, providerhost.AcquireOptions{
		Resolve:   o.providerLocation(),
		FetchLock: o.fetchProviderLock, EnsureVerifier: o.ensureProviderVerifier,
		Verifier: o.providerVerifier, Progress: o.stderr,
	})
}

func (o *options) providerLocation() providerhost.ResolveOptions {
	executable := ""
	if o.executable != nil {
		if path, err := o.executable(); err == nil {
			executable = path
		}
	}
	return providerhost.ResolveOptions{ExecutablePath: executable}
}

func (o *options) loadInstalledProvider(ctx context.Context, provider string) (providerhost.Loaded, error) {
	resolved, err := providerhost.ResolveInstalled(provider, Version, o.providerLocation())
	if err != nil {
		return providerhost.Loaded{}, err
	}
	verifier := o.providerVerifier
	if verifier == nil {
		verifier = providerhost.NewPreferredVerifier("")
	}
	return providerhost.Load(ctx, providerhost.Options{
		Mode:       providerhost.ModeSubprocess,
		Provider:   provider,
		LockPath:   resolved.LockPath,
		BinaryPath: resolved.Binary,
		BundlePath: resolved.Bundle,
		Verifier:   verifier,
	})
}

// defaultDialProvider starts the verified provider subprocess and tracks the
// session for closeProviderSessions. Callers defer closeProviderSessions
// after newBackend returns.
func (o *options) defaultDialProvider(ctx context.Context, binary string) (*providerhost.Client, error) {
	client, err := providerhost.Dial(ctx, binary, providerhost.DialOptions{Required: []sdk.Operation{sdk.OpDescribe}})
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

// Plugin-backed plans execute in the verified provider process.
func (o *options) defaultNewBackend(ctx context.Context, planned platform.PlannedStack, backendURL string) (infrastructureBackend, error) {
	if _, ok := providerhost.AsShimPlanned(planned); ok {
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
		return nil, fmt.Errorf("provider backend requires a plugin-backed plan, got %T", planned)
	}
	if o.loadProvider == nil || o.dialProvider == nil {
		return nil, fmt.Errorf("provider loading is not configured")
	}
	loaded, err := o.loadProvider(ctx, string(planned.Provider()))
	if err != nil {
		return nil, err
	}
	client, err := o.dialProvider(ctx, loaded.Binary)
	if err != nil {
		return nil, err
	}
	if id := client.ProviderID(); id != "" && id != string(planned.Provider()) {
		client.Close()
		return nil, fmt.Errorf("provider advertised %q, expected %q", id, planned.Provider())
	}
	return providerhost.NewPluginBackend(client, shim.Envelope(), shim.StoredPlan())
}

// providerProvenance describes the local artifact verified by extensions list.
type providerProvenance struct {
	Name    string `json:"name" yaml:"name"`
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
	Digest  string `json:"digest,omitempty" yaml:"digest,omitempty"`
	Mode    string `json:"mode" yaml:"mode"`
}

func (o *options) subprocessProvenance(ctx context.Context) []providerProvenance {
	var result []providerProvenance
	for _, id := range []string{"gcp", "azure"} {
		entry := providerProvenance{Name: providerhost.ExtractName(id), Mode: "not-installed"}
		if o.inspectProvider != nil {
			loaded, err := o.inspectProvider(ctx, id)
			switch {
			case err == nil:
				entry.Version = loaded.Artifact.Version
				entry.Digest = loaded.Artifact.Digest
				entry.Mode = "subprocess"
			case errors.Is(err, providerhost.ErrCacheCorrupt):
				entry.Mode = "unverified"
			case errors.Is(err, providerhost.ErrNoLockfile), errors.Is(err, providerhost.ErrUnknownProvider), errors.Is(err, providerhost.ErrCacheMiss):
				// The provider has no installed artifact for this project.
			default:
				entry.Mode = "unverified"
			}
		}
		result = append(result, entry)
	}
	return result
}
