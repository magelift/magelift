package registry

import (
	"context"
	"fmt"
	"os"

	internalcli "github.com/magelift/magelift/internal/cli"
	awseksops "github.com/magelift/magelift/internal/cloud/aws/eksops"
	awsops "github.com/magelift/magelift/internal/cloud/aws/ops"
	ovhstack "github.com/magelift/magelift/internal/cloud/ovh/stack"
	scwstack "github.com/magelift/magelift/internal/cloud/scaleway/stack"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

// NewDefault returns the alpha module set: AWS ECS Fargate (certified,
// in-process pending parity extraction) plus lazy GCP plugin shims.
// Registration spawns no processes and needs no credentials. The first
// fallible GCP use dials exactly once per CLI invocation (one plugin
// serves both runtimes); dial failure fails the GCP command with a clear
// error, never other providers' commands. Provider hook constructors
// live in hooks.go (RegisterHooks).
func NewDefault() (*platform.ModuleRegistry, error) {
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterModule(awsops.Module{}); err != nil {
		return nil, err
	}
	return registerGCPShims(modules)
}

// NewDefaultWithExperimental adds the deferred providers (EKS, OVH,
// Scaleway) to the alpha set. The CLI selects it only when
// MAGELIFT_EXPERIMENTAL_PROVIDERS is set; harnesses and tests call it
// explicitly. Cleanup/recovery providers stay available regardless:
// recovery must read old ledgers.
func NewDefaultWithExperimental() (*platform.ModuleRegistry, error) {
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
	return registerGCPShims(modules)
}

func registerGCPShims(modules *platform.ModuleRegistry) (*platform.ModuleRegistry, error) {
	dialer := &providerhost.CachedDialer{Dial: dialGCP}
	modules.RegisterCloser(dialer.Close)
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

// dialGCP acquires, verifies and dials the first-party GCP plugin lazily.
// The shared resolver preserves project pins. A failed acquisition or trust
// check is returned without selecting another implementation.
func dialGCP(ctx context.Context) (*providerhost.Client, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate CLI executable for provider discovery: %w", err)
	}
	loaded, err := providerhost.Acquire(ctx, "gcp", internalcli.Version, providerhost.AcquireOptions{
		Resolve: providerhost.ResolveOptions{ExecutablePath: executable}, Progress: os.Stderr,
	})
	if err != nil {
		return nil, err
	}
	return providerhost.Dial(ctx, loaded.Binary, providerhost.DialOptions{})
}
