package registry

import (
	"context"
	"fmt"
	"os"

	internalcli "github.com/magelift/magelift/internal/cli"
	ovhstack "github.com/magelift/magelift/internal/cloud/ovh/stack"
	scwstack "github.com/magelift/magelift/internal/cloud/scaleway/stack"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

// A first-party deployment must be able to release its lock and complete a
// Magento rollout before the host starts any infrastructure change.
var deploymentOperations = []sdk.Operation{
	sdk.OpDescribe, sdk.OpValidateConfig, sdk.OpPlan, sdk.OpPreview, sdk.OpApply,
	sdk.OpOutputs, sdk.OpDestroy, sdk.OpStateLock, sdk.OpStateUnlock, sdk.OpDeployAppPhase,
}

// NewDefault returns the default first-party module set.
func NewDefault() (*platform.ModuleRegistry, error) {
	modules, _, err := NewDefaultSet()
	return modules, err
}

// NewDefaultSet returns the default modules and hooks with shared provider
// sessions.
func NewDefaultSet() (*platform.ModuleRegistry, internalcli.Hooks, error) {
	return newDefaultSet(false)
}

// NewDefaultWithExperimental returns modules with EKS, OVH, and Scaleway.
func NewDefaultWithExperimental() (*platform.ModuleRegistry, error) {
	modules, _, err := NewDefaultExperimentalSet()
	return modules, err
}

// NewDefaultExperimentalSet returns experimental modules and hooks with
// shared provider sessions.
func NewDefaultExperimentalSet() (*platform.ModuleRegistry, internalcli.Hooks, error) {
	return newDefaultSet(true)
}

func newDefaultSet(experimental bool) (*platform.ModuleRegistry, internalcli.Hooks, error) {
	modules := platform.NewModuleRegistry()
	dialers := make(map[sdk.ProviderID]*providerhost.CachedDialer)
	for _, provider := range providerhost.StandaloneProviderIDs() {
		dialer := newProviderDialer(provider)
		dialers[sdk.ProviderID(provider)] = dialer
		modules.RegisterCloser(dialer.Close)
	}

	if experimental {
		for _, module := range []platform.StackModule{ovhstack.Module{}, scwstack.Module{}} {
			if err := modules.RegisterModule(module); err != nil {
				return nil, internalcli.Hooks{}, err
			}
		}
	}
	for _, descriptor := range providerhost.StandaloneRuntimes() {
		if descriptor.OptIn && !experimental {
			continue
		}
		dialer := dialers[descriptor.Provider]
		shim, err := providerhost.NewLazyProviderShimModule(
			descriptor.Provider, descriptor.Runtime, descriptor.Tier, descriptor.OutputKeys, dialer.Do,
		)
		if err != nil {
			return nil, internalcli.Hooks{}, err
		}
		if err := modules.RegisterModule(shim); err != nil {
			return nil, internalcli.Hooks{}, fmt.Errorf("register %s shim: %w", descriptor.Runtime, err)
		}
	}
	hooks := registerHooks(dialers[providerhost.StandaloneProviderGCP], dialers[providerhost.StandaloneProviderAWS])
	hooks.Close = nil
	return modules, hooks, nil
}

func newProviderDialer(provider string) *providerhost.CachedDialer {
	return &providerhost.CachedDialer{Dial: func(ctx context.Context) (*providerhost.Client, error) {
		executable, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("locate CLI executable for provider discovery: %w", err)
		}
		loaded, err := providerhost.Acquire(ctx, provider, internalcli.Version, providerhost.AcquireOptions{
			Resolve: providerhost.ResolveOptions{ExecutablePath: executable}, Progress: os.Stderr,
		})
		if err != nil {
			return nil, err
		}
		return providerhost.Dial(ctx, loaded.Binary, providerhost.DialOptions{
			ExpectedProvider: sdk.ProviderID(provider),
			Required:         deploymentOperations,
		})
	}}
}
