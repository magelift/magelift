package stack

import (
	"context"
	"io"

	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
)

// Ops implements platform.Ops for experimental GCP. Magento candidate deploy is
// not available yet; stack preview/update remains the supported path.
type Ops struct{}

func (Module) Ops() platform.Ops { return Ops{} }

func (Ops) AcquireLock(context.Context, platform.PlannedStack) (func(context.Context) error, error) {
	return func(context.Context) error { return nil }, nil
}

func (Ops) NewDeploySteps(context.Context, any, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
	return nil, platform.ErrNotSupported
}
