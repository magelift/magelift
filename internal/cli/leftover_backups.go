package cli

import (
	"context"
	"fmt"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
)

// LeftoverBackupDestroyer removes provider backups left after infrastructure
// teardown.
type LeftoverBackupDestroyer interface {
	Destroy(context.Context) ([]string, error)
}

type leftoverBackupDestroyer = LeftoverBackupDestroyer

func destroyBackupsImplemented(mechanism string) bool {
	return mechanism == "cloud-sql-final-backup"
}

func (o *options) leftoverBackupDestroyerFor(ctx context.Context, planned platform.PlannedStack) (leftoverBackupDestroyer, error) {
	if o.newLeftoverBackupDestroyer != nil {
		return o.newLeftoverBackupDestroyer(ctx, planned)
	}
	return nil, fmt.Errorf("leftover backup destroyer is not configured for provider %q", planned.Provider())
}

// destroyLeftoverProviderBackups deletes GCP Cloud SQL leftovers for the
// Magelift instance after Pulumi destroy. A disposable backup policy still
// runs the destroyer: --destroy-backups means delete leftovers, not only
// when retention would have kept them. Other providers no-op when nothing
// is retained and refuse when retention still applies.
func (o *options) destroyLeftoverProviderBackups(ctx context.Context, planned platform.PlannedStack, retained []config.RetainedBackup) ([]string, error) {
	if planned.Provider() != "gcp" {
		if len(retained) == 0 {
			return nil, nil
		}
		return nil, fmt.Errorf("leftover backup destroy is not implemented for provider %q", planned.Provider())
	}
	destroyer, err := o.leftoverBackupDestroyerFor(ctx, planned)
	if err != nil {
		return nil, err
	}
	return destroyer.Destroy(ctx)
}
