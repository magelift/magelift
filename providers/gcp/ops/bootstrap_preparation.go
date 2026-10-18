package ops

import (
	"context"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
)

// AccountPreparation prepares provider-owned prerequisites without returning keys.
type AccountPreparation interface {
	EnsureAPIs(context.Context, gcpbootstrap.PreparationPlan, gcpbootstrap.Spec) error
	EnsureStatePassphrase(context.Context, gcpbootstrap.PreparationPlan, gcpbootstrap.Spec, string) (gcpbootstrap.StatePassphraseRef, error)
	EnsureRepository(context.Context, gcpbootstrap.PreparationPlan, gcpbootstrap.Spec) (gcpbootstrap.RepositoryResult, error)
	Close() error
}
