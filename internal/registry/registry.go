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

var awsOperations = []sdk.Operation{
	sdk.OpDescribe, sdk.OpValidateConfig, sdk.OpPlan, sdk.OpPreview, sdk.OpApply,
	sdk.OpOutputs, sdk.OpDestroy, sdk.OpBootstrapVerify, sdk.OpBootstrapEnsure,
	sdk.OpStateStatus, sdk.OpStateLock, sdk.OpStateUnlock, sdk.OpStateBackup,
	sdk.OpStateRestore, sdk.OpSecretList, sdk.OpSecretSet, sdk.OpSecretRemove,
	sdk.OpSecretRead, sdk.OpTailLogs, sdk.OpCheckRuntime, sdk.OpPrepareExec,
	sdk.OpPrepareTunnel, sdk.OpCostInputs, sdk.OpDeployAppPhase,
	sdk.OpMediaExport, sdk.OpMediaImport,
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
	awsDialer := newProviderDialer("aws", awsOperations)
	gcpDialer := newProviderDialer("gcp", nil)
	azureDialer := newProviderDialer("azure", []sdk.Operation{sdk.OpDescribe, sdk.OpValidateConfig, sdk.OpPlan})
	modules.RegisterCloser(awsDialer.Close)
	modules.RegisterCloser(gcpDialer.Close)
	modules.RegisterCloser(azureDialer.Close)

	if experimental {
		for _, module := range []platform.StackModule{ovhstack.Module{}, scwstack.Module{}} {
			if err := modules.RegisterModule(module); err != nil {
				return nil, internalcli.Hooks{}, err
			}
		}
	}
	if err := registerAWSShims(modules, awsDialer, experimental); err != nil {
		return nil, internalcli.Hooks{}, err
	}
	if err := registerGCPShims(modules, gcpDialer); err != nil {
		return nil, internalcli.Hooks{}, err
	}
	if err := registerAzureShim(modules, azureDialer); err != nil {
		return nil, internalcli.Hooks{}, err
	}
	hooks := registerHooks(gcpDialer, awsDialer)
	hooks.Close = nil
	return modules, hooks, nil
}

func registerAWSShims(modules *platform.ModuleRegistry, dialer *providerhost.CachedDialer, experimental bool) error {
	for _, target := range []struct {
		runtime sdk.RuntimeID
		tier    platform.CertificationTier
	}{
		{runtime: "ecs-fargate", tier: platform.TierBuiltIn},
		{runtime: "eks", tier: platform.TierExperimental},
	} {
		if target.runtime == "eks" && !experimental {
			continue
		}
		shim, err := providerhost.NewLazyProviderShimModule("aws", target.runtime, target.tier, platform.RequiredOutputKeys(), dialer.Do)
		if err != nil {
			return err
		}
		if err := modules.RegisterModule(shim); err != nil {
			return fmt.Errorf("register AWS %s shim: %w", target.runtime, err)
		}
	}
	return nil
}

func registerAzureShim(modules *platform.ModuleRegistry, dialer *providerhost.CachedDialer) error {
	shim, err := providerhost.NewLazyAzureShimModule(dialer.Do)
	if err != nil {
		return err
	}
	if err := modules.RegisterModule(shim); err != nil {
		return fmt.Errorf("register Azure AKS shim: %w", err)
	}
	return nil
}

func registerGCPShims(modules *platform.ModuleRegistry, dialer *providerhost.CachedDialer) error {
	for _, runtime := range []sdk.RuntimeID{"gke-autopilot", "gke-standard"} {
		shim, err := providerhost.NewLazyShimModule(runtime, dialer.Do)
		if err != nil {
			return err
		}
		if err := modules.RegisterModule(shim); err != nil {
			return fmt.Errorf("register GCP %s shim: %w", runtime, err)
		}
	}
	return nil
}

func newProviderDialer(provider string, required []sdk.Operation) *providerhost.CachedDialer {
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
			Required:         required,
		})
	}}
}
