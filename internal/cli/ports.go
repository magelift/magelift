package cli

import (
	"errors"
	"fmt"

	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
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
		return nil, invalid(fmt.Errorf("no stack module for %s/%s", effective.Config.Target.Provider, effective.Config.Target.Runtime))
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
		return nil, invalid(fmt.Errorf("bootstrap is not supported for this target yet"))
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
		return nil, invalid(fmt.Errorf("state operations are not supported for this target yet"))
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
		return nil, invalid(fmt.Errorf("secrets are not supported for this target yet"))
	}
	return port, nil
}

func notSupported(err error, planned platform.PlannedStack, surface string) error {
	if errors.Is(err, platform.ErrNotSupported) {
		return invalid(fmt.Errorf("%s is not supported for target %s/%s yet", surface, planned.Provider(), planned.Runtime()))
	}
	return err
}
