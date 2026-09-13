package registry

import (
	awseksops "github.com/magelift/magelift/internal/cloud/aws/eksops"
	awsops "github.com/magelift/magelift/internal/cloud/aws/ops"
	gcpops "github.com/magelift/magelift/internal/cloud/gcp/ops"
	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	ovhstack "github.com/magelift/magelift/internal/cloud/ovh/stack"
	scwstack "github.com/magelift/magelift/internal/cloud/scaleway/stack"
	"github.com/magelift/magelift/internal/platform"
)

// NewDefault returns the first-party module set used by the released CLI.
func NewDefault() (*platform.ModuleRegistry, error) {
	modules := platform.NewModuleRegistry()
	for _, module := range []platform.StackModule{
		awsops.Module{},
		awseksops.Module{},
		gcpops.Module{},
		gcpops.Module{RuntimeID: gcptarget.RuntimeStandardID},
		ovhstack.Module{},
		scwstack.Module{},
	} {
		if err := modules.RegisterModule(module); err != nil {
			return nil, err
		}
	}
	return modules, nil
}
