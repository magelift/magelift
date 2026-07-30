package stack

import (
	"context"
	"io"
	"os"

	"github.com/acourtiol/magelift/internal/config"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
)

// diyLockWarnOut is the sink for AcquireLock honesty warnings; tests redirect it.
var diyLockWarnOut io.Writer = os.Stderr

// unsupported implements optional day-2 ports for experimental OVH.
type unsupported struct{}

func (Module) Bootstrap() platform.Bootstrap           { return unsupported{} }
func (Module) State() platform.State                   { return State{} }
func (Module) Secrets() platform.Secrets               { return unsupported{} }
func (Module) RuntimeObserve() platform.RuntimeObserve { return unsupported{} }
func (Module) CostEstimator() platform.CostEstimator   { return unsupported{} }

func (unsupported) VerifyAccount(context.Context, platform.PlannedStack) error {
	return platform.ErrNotSupported
}
func (unsupported) Ensure(context.Context, platform.PlannedStack, platform.BootstrapRequest) (platform.BootstrapResult, error) {
	return platform.BootstrapResult{}, platform.ErrNotSupported
}
func (unsupported) Status(context.Context, platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	return false, nil, "", platform.ErrNotSupported
}
func (unsupported) Lock(context.Context, platform.PlannedStack, string) (func(context.Context) error, error) {
	return nil, platform.ErrNotSupported
}
func (unsupported) Unlock(context.Context, platform.PlannedStack) (*platform.LockInfo, error) {
	return nil, platform.ErrNotSupported
}
func (unsupported) Backup(context.Context, platform.PlannedStack) (platform.BackupResult, error) {
	return platform.BackupResult{}, platform.ErrNotSupported
}
func (unsupported) Restore(context.Context, platform.PlannedStack, string) (platform.RestoreResult, error) {
	return platform.RestoreResult{}, platform.ErrNotSupported
}
func (unsupported) List(context.Context, platform.PlannedStack) ([]platform.SecretMeta, error) {
	return nil, platform.ErrNotSupported
}
func (unsupported) Set(context.Context, platform.PlannedStack, string, []byte) error {
	return platform.ErrNotSupported
}
func (unsupported) Remove(context.Context, platform.PlannedStack, string) error {
	return platform.ErrNotSupported
}
func (unsupported) TailLogs(context.Context, platform.PlannedStack, platform.LogQuery) ([]platform.LogEvent, error) {
	return nil, platform.ErrNotSupported
}
func (unsupported) CheckRuntime(context.Context, platform.PlannedStack, map[string]any) ([]platform.RuntimeHealth, error) {
	return nil, platform.ErrNotSupported
}
func (unsupported) PrepareExec(context.Context, platform.PlannedStack, map[string]any, platform.ExecQuery) (platform.ExecTarget, error) {
	return platform.ExecTarget{}, platform.ErrNotSupported
}
func (unsupported) Estimate(context.Context, platform.PlannedStack, config.Config, platform.CostOptions) (platform.CostReport, error) {
	return platform.CostReport{}, platform.ErrNotSupported
}

type Ops struct{}

func (Module) Ops() platform.Ops { return Ops{} }

func (Ops) AcquireLock(_ context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	platform.WarnNoDIYLock(diyLockWarnOut, planned)
	return func(context.Context) error { return nil }, nil
}

func (Ops) NewDeploySteps(context.Context, any, platform.PlannedStack, io.Writer) (deployflow.Steps, error) {
	return nil, platform.ErrNotSupported
}
