package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	awsbootstrap "github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
	awsstate "github.com/acourtiol/magelift/internal/cloud/aws/state"
	"github.com/spf13/cobra"
)

type stateManager interface {
	Status(context.Context) (awsstate.Info, error)
}

type stateUnlocker interface {
	Unlock(context.Context) (awsstate.Info, error)
}

type stateLocker interface {
	Lock(context.Context, string, string, string) (func() error, error)
}

type stateArchive interface {
	Backup(context.Context) (awsstate.BackupResult, error)
	Restore(context.Context, string) (awsstate.RestoreResult, error)
}

type stateStatusResult struct {
	Environment string         `json:"environment" yaml:"environment"`
	Bucket      string         `json:"bucket" yaml:"bucket"`
	Locked      bool           `json:"locked" yaml:"locked"`
	Lock        *awsstate.Info `json:"lock,omitempty" yaml:"lock,omitempty"`
}

type stateScope struct {
	Manager     stateManager
	Project     string
	Environment string
	Region      string
	KMSARN      string
	Plan        awsbootstrap.Plan
}

func stateStatusCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Show the deployment lock status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		manager, environment, plan, err := o.stateManager(cmd.Context())
		if err != nil {
			return invalid(err)
		}
		info, err := manager.Status(cmd.Context())
		if errors.Is(err, awsstate.ErrNotLocked) {
			return o.write(stateStatusResult{Environment: environment, Bucket: plan.StateBucket})
		}
		if err != nil {
			return fmt.Errorf("inspect deployment lock: %w", err)
		}
		return o.write(stateStatusResult{Environment: environment, Bucket: plan.StateBucket, Locked: true, Lock: &info})
	}}
}

func (o *options) stateManager(ctx context.Context) (stateManager, string, awsbootstrap.Plan, error) {
	scope, err := o.resolveStateScope(ctx)
	if err != nil {
		return nil, "", awsbootstrap.Plan{}, err
	}
	return scope.Manager, scope.Environment, scope.Plan, nil
}

func (o *options) resolveStateScope(ctx context.Context) (stateScope, error) {
	effective, environment, err := o.resolveWithEnvironment()
	if err != nil {
		return stateScope{}, err
	}
	aws := effective.Config.Target.AWS
	if aws == nil {
		return stateScope{}, errors.New("target.aws is required for state operations")
	}
	if strings.TrimSpace(effective.Config.Account) == "" || strings.TrimSpace(effective.Config.Defaults.Region) == "" || strings.TrimSpace(aws.KMSKeyARN) == "" {
		return stateScope{}, errors.New("account, region, and target.aws.kmsKeyArn are required for state operations")
	}
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: effective.Config.Project.Name, Environment: environment,
		AccountID: effective.Config.Account, Region: effective.Config.Defaults.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return stateScope{}, err
	}
	if o.newState == nil {
		return stateScope{}, errors.New("state manager factory is required")
	}
	manager, err := o.newState(ctx, effective.Config.Defaults.Region, plan.StateBucket, effective.Config.Project.Name, environment, aws.KMSKeyARN)
	if err != nil {
		return stateScope{}, fmt.Errorf("initialize state manager: %w", err)
	}
	return stateScope{Manager: manager, Project: effective.Config.Project.Name, Environment: environment, Region: effective.Config.Defaults.Region, KMSARN: aws.KMSKeyARN, Plan: plan}, nil
}

func stateUnlockCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "unlock", Short: "Remove a stale deployment lock", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		if !o.yes {
			return invalid(errors.New("unlocking deployment state requires --yes"))
		}
		manager, environment, plan, err := o.stateManager(cmd.Context())
		if err != nil {
			return invalid(err)
		}
		unlocker, ok := manager.(stateUnlocker)
		if !ok {
			return errors.New("state manager does not support unlocking")
		}
		info, err := unlocker.Unlock(cmd.Context())
		if errors.Is(err, awsstate.ErrNotLocked) {
			return o.write(stateStatusResult{Environment: environment, Bucket: plan.StateBucket})
		}
		if err != nil {
			return fmt.Errorf("unlock deployment state: %w", err)
		}
		return o.write(stateStatusResult{Environment: environment, Bucket: plan.StateBucket, Locked: false, Lock: &info})
	}}
}

func stateBackupCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "backup", Short: "Create a versioned Pulumi state backup", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		manager, archive, project, environment, plan, err := o.stateArchive(cmd.Context())
		if err != nil {
			return invalid(err)
		}
		release, err := acquireStateLock(cmd.Context(), manager, project, environment)
		if err != nil {
			return fmt.Errorf("acquire state backup lock: %w", err)
		}
		result, err := archive.Backup(cmd.Context())
		releaseErr := release()
		if err != nil {
			if releaseErr != nil {
				return fmt.Errorf("backup state: %w; release lock: %v", err, releaseErr)
			}
			return fmt.Errorf("backup state: %w", err)
		}
		if releaseErr != nil {
			return fmt.Errorf("release state backup lock: %w", releaseErr)
		}
		return o.write(map[string]any{"environment": environment, "bucket": plan.StateBucket, "backup": result})
	}}
}

func stateRestoreCommand(o *options) *cobra.Command {
	return &cobra.Command{Use: "restore <backup-id>", Short: "Restore Pulumi state from a versioned backup", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !o.yes {
			return invalid(errors.New("restoring deployment state requires --yes"))
		}
		manager, archive, project, environment, plan, err := o.stateArchive(cmd.Context())
		if err != nil {
			return invalid(err)
		}
		release, err := acquireStateLock(cmd.Context(), manager, project, environment)
		if err != nil {
			return fmt.Errorf("acquire state restore lock: %w", err)
		}
		result, err := archive.Restore(cmd.Context(), args[0])
		releaseErr := release()
		if err != nil {
			if releaseErr != nil {
				return fmt.Errorf("restore state: %w; release lock: %v", err, releaseErr)
			}
			return fmt.Errorf("restore state: %w", err)
		}
		if releaseErr != nil {
			return fmt.Errorf("release state restore lock: %w", releaseErr)
		}
		return o.write(map[string]any{"environment": environment, "bucket": plan.StateBucket, "restore": result})
	}}
}

func (o *options) stateArchive(ctx context.Context) (stateManager, stateArchive, string, string, awsbootstrap.Plan, error) {
	scope, err := o.resolveStateScope(ctx)
	if err != nil {
		return nil, nil, "", "", awsbootstrap.Plan{}, err
	}
	if o.newArchive == nil {
		return nil, nil, "", "", awsbootstrap.Plan{}, errors.New("state archive factory is required")
	}
	archive, err := o.newArchive(ctx, scope.Region, scope.Plan.StateBucket, scope.KMSARN)
	if err != nil {
		return nil, nil, "", "", awsbootstrap.Plan{}, fmt.Errorf("initialize state archive: %w", err)
	}
	return scope.Manager, archive, scope.Project, scope.Environment, scope.Plan, nil
}

func acquireStateLock(ctx context.Context, manager stateManager, project, environment string) (func() error, error) {
	locker, ok := manager.(stateLocker)
	if !ok {
		return nil, errors.New("state manager does not support locking")
	}
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-state-%s-%d", host, os.Getpid())
	release, err := locker.Lock(ctx, project, environment, owner)
	if err != nil {
		return nil, err
	}
	return release, nil
}
