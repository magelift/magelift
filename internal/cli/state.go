package cli

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/magelift/magelift/internal/platform"
	"github.com/spf13/cobra"
)

type stateStatusResult struct {
	Environment string             `json:"environment" yaml:"environment"`
	Backend     string             `json:"backend" yaml:"backend"`
	Locked      bool               `json:"locked" yaml:"locked"`
	Lock        *platform.LockInfo `json:"lock,omitempty" yaml:"lock,omitempty"`
}

func stateStatusCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show the deployment lock status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, planned, err := o.planStack(false)
		if err != nil {
			return invalid(err)
		}
		state, err := o.statePort()
		if err != nil {
			return err
		}
		locked, info, backend, err := state.Status(cmd.Context(), planned)
		if err != nil {
			if mapped := notSupported(err, planned, "state"); mapped != err {
				return mapped
			}
			return fmt.Errorf("inspect deployment lock: %w", err)
		}
		return o.write(stateStatusResult{Environment: planned.Environment(), Backend: backend, Locked: locked, Lock: info})
	}}
}

func stateUnlockCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "unlock", Short: "Remove a stale deployment lock", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !o.yes {
			return invalid(errors.New("unlocking deployment state requires --yes"))
		}
		_, planned, err := o.planStack(false)
		if err != nil {
			return invalid(err)
		}
		state, err := o.statePort()
		if err != nil {
			return err
		}
		info, err := state.Unlock(cmd.Context(), planned)
		if err != nil {
			if mapped := notSupported(err, planned, "state unlock"); mapped != err {
				return mapped
			}
			return fmt.Errorf("unlock deployment state: %w", err)
		}
		return o.write(stateStatusResult{Environment: planned.Environment(), Locked: false, Lock: info})
	}}
}

func stateBackupCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "backup", Short: "Create a versioned Pulumi state backup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		_, planned, err := o.planStack(false)
		if err != nil {
			return invalid(err)
		}
		state, err := o.statePort()
		if err != nil {
			return err
		}
		release, err := acquirePlatformStateLock(cmd.Context(), state, planned)
		if err != nil {
			return fmt.Errorf("acquire state backup lock: %w", err)
		}
		result, err := state.Backup(cmd.Context(), planned)
		releaseErr := release(cmd.Context())
		if err != nil {
			if mapped := notSupported(err, planned, "state backup"); mapped != err {
				_ = releaseErr
				return mapped
			}
			if releaseErr != nil {
				return fmt.Errorf("backup state: %w; release lock: %v", err, releaseErr)
			}
			return fmt.Errorf("backup state: %w", err)
		}
		if releaseErr != nil {
			return fmt.Errorf("release state backup lock: %w", releaseErr)
		}
		return o.write(map[string]any{"environment": planned.Environment(), "backup": result})
	}}
}

func stateRestoreCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "restore <backup-id>", Short: "Restore Pulumi state from a versioned backup", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !o.yes {
			return invalid(errors.New("restoring deployment state requires --yes"))
		}
		_, planned, err := o.planStack(false)
		if err != nil {
			return invalid(err)
		}
		state, err := o.statePort()
		if err != nil {
			return err
		}
		release, err := acquirePlatformStateLock(cmd.Context(), state, planned)
		if err != nil {
			return fmt.Errorf("acquire state restore lock: %w", err)
		}
		result, err := state.Restore(cmd.Context(), planned, args[0])
		releaseErr := release(cmd.Context())
		if err != nil {
			if mapped := notSupported(err, planned, "state restore"); mapped != err {
				_ = releaseErr
				return mapped
			}
			if releaseErr != nil {
				return fmt.Errorf("restore state: %w; release lock: %v", err, releaseErr)
			}
			return fmt.Errorf("restore state: %w", err)
		}
		if releaseErr != nil {
			return fmt.Errorf("release state restore lock: %w", releaseErr)
		}
		return o.write(map[string]any{"environment": planned.Environment(), "restore": result})
	}}
}

func acquirePlatformStateLock(ctx context.Context, state platform.State, planned platform.PlannedStack) (func(context.Context) error, error) {
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-state-%s-%d", host, os.Getpid())
	return state.Lock(ctx, planned, owner)
}
