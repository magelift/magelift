package cli

import (
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func (o *options) resolveModule() (platform.StackModule, error) {
	if o.modules == nil {
		return nil, errors.New("stack module registry is required")
	}
	effective, _, err := o.resolveWithEnvironment()
	if err != nil {
		return nil, invalid(err)
	}
	module, found := o.modules.Module(sdk.ProviderID(effective.Config.Target.Provider), sdk.RuntimeID(effective.Config.Target.Runtime))
	if !found {
		return nil, guided(
			fmt.Sprintf("no stack module for %s/%s", effective.Config.Target.Provider, effective.Config.Target.Runtime),
			"check target.provider and target.runtime in magelift.yaml, or build a custom CLI that RegisterModules your provider",
			"docs/capability-matrix.md",
		)
	}
	return module, nil
}

func (o *options) bootstrapPort() (platform.Bootstrap, error) {
	if o.testBootstrap != nil {
		return o.testBootstrap, nil
	}
	module, err := o.resolveModule()
	if err != nil {
		return nil, err
	}
	port := platform.ModuleBootstrap(module)
	if port == nil {
		return nil, invalid(fmt.Errorf("%s", notSupportedForModule(module, "bootstrap")))
	}
	return port, nil
}

func (o *options) statePort() (platform.State, error) {
	if o.testState != nil {
		return o.testState, nil
	}
	module, err := o.resolveModule()
	if err != nil {
		return nil, err
	}
	port := platform.ModuleState(module)
	if port == nil {
		return nil, invalid(fmt.Errorf("%s", notSupportedForModule(module, "state operations")))
	}
	return port, nil
}

func (o *options) secretsPort() (platform.Secrets, error) {
	if o.testSecrets != nil {
		return o.testSecrets, nil
	}
	module, err := o.resolveModule()
	if err != nil {
		return nil, err
	}
	port := platform.ModuleSecrets(module)
	if port == nil {
		return nil, invalid(fmt.Errorf("%s", notSupportedForModule(module, "secrets")))
	}
	return port, nil
}

// notSupportedMessage is the single unsupported day-2 shape: surface, provider/runtime,
// then the certification tier after the preserved trailing "yet" (plan 01-03 prefix).
func notSupportedMessage(provider, runtime string, tier platform.CertificationTier, surface string) string {
	return fmt.Sprintf("%s is not supported for target %s/%s yet (%s)", surface, provider, runtime, tier)
}

func notSupportedForPlanned(planned platform.PlannedStack, surface string) string {
	return notSupportedMessage(string(planned.Provider()), string(planned.Runtime()), planned.CertificationTier(), surface)
}

func notSupportedForModule(module platform.StackModule, surface string) string {
	d := module.Descriptor()
	return notSupportedMessage(string(d.Provider), string(d.Runtime), module.CertificationTier(), surface)
}

func notSupported(err error, planned platform.PlannedStack, surface string) error {
	if errors.Is(err, platform.ErrNotSupported) {
		return invalid(fmt.Errorf("%s", notSupportedForPlanned(planned, surface)))
	}
	return err
}
