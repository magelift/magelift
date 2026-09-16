package registry

import (
	"context"
	"fmt"
	"os"

	awseksops "github.com/magelift/magelift/internal/cloud/aws/eksops"
	awsops "github.com/magelift/magelift/internal/cloud/aws/ops"
	ovhstack "github.com/magelift/magelift/internal/cloud/ovh/stack"
	scwstack "github.com/magelift/magelift/internal/cloud/scaleway/stack"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

// NewDefault returns the first-party module set used by the released CLI.
// GCP entries are lazy plugin shims: registration spawns no processes and
// needs no credentials. The first fallible GCP use dials exactly once per
// CLI invocation (one plugin serves both runtimes); dial failure fails the
// GCP command with a clear error, never other providers' commands.
// Provider hook constructors live in hooks.go (RegisterHooks).
func NewDefault() (*platform.ModuleRegistry, error) {
	modules := platform.NewModuleRegistry()
	for _, module := range []platform.StackModule{
		awsops.Module{},
		awseksops.Module{},
		ovhstack.Module{},
		scwstack.Module{},
	} {
		if err := modules.RegisterModule(module); err != nil {
			return nil, err
		}
	}
	dialer := &providerhost.CachedDialer{Dial: dialGCP}
	for _, runtime := range []sdk.RuntimeID{"gke-autopilot", "gke-standard"} {
		shim, err := providerhost.NewLazyShimModule(runtime, dialer.Do)
		if err != nil {
			return nil, err
		}
		if err := modules.RegisterModule(shim); err != nil {
			return nil, fmt.Errorf("register GCP %s shim: %w", runtime, err)
		}
	}
	return modules, nil
}

// dialGCP loads, verifies, and dials the installed GCP provider plugin.
// There is no fallback: with no embedded GCP implementation left, a
// missing or tampered plugin is a hard error naming the failed check.
func dialGCP(ctx context.Context) (*providerhost.Client, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate CLI executable for provider discovery: %w", err)
	}
	paths := providerhost.DiscoverArtifactPaths(executable, "gcp")
	loaded, err := providerhost.Load(ctx, providerhost.Options{
		Mode:       providerhost.ModeSubprocess,
		Provider:   "gcp",
		LockPath:   paths.Lock,
		BinaryPath: paths.Binary,
		Verifier:   providerhost.NewCosignVerifier(),
	})
	if err != nil {
		return nil, err
	}
	return providerhost.DialV2(ctx, loaded.Binary, providerhost.DialOptions{})
}
