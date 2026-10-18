package ops

import (
	"context"

	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
)

// NodeIdentityPreparation prepares the runtime identity independently of CI.
type NodeIdentityPreparation interface {
	Ensure(context.Context, gcpbootstrap.PreparationPlan, gcpbootstrap.Spec) (gcpbootstrap.NodeIdentityResult, error)
	Close() error
}
