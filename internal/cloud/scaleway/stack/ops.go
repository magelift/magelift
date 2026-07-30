package stack

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/config"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
)

// diyLockWarnOut is the sink for AcquireLock honesty warnings; tests redirect it.
var diyLockWarnOut io.Writer = os.Stderr

// unsupported implements optional day-2 ports for experimental Scaleway.
type unsupported struct{}

func (Module) Bootstrap() platform.Bootstrap { return unsupported{} }
func (Module) State() platform.State         { return State{} }
func (Module) Secrets() platform.Secrets     { return unsupported{} }
func (Module) RuntimeObserve() platform.RuntimeObserve {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) CostEstimator() platform.CostEstimator { return unsupported{} }

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
func (unsupported) Estimate(context.Context, platform.PlannedStack, config.Config, platform.CostOptions) (platform.CostReport, error) {
	return platform.CostReport{}, platform.ErrNotSupported
}

// Ops remains Magento deploy-only; day-2 ports are separate Has* interfaces.
type Ops struct {
	NewCandidate  func(context.Context, kube.Backend) (kube.CandidateRunner, error)
	NewRuntime    func(context.Context, kube.Backend) (kube.RuntimeChecker, error)
	RecordRelease func(context.Context, deployflow.Request, deployflow.Result) error
}

func (Module) Ops() platform.Ops { return Ops{} }

func (Ops) AcquireLock(_ context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	platform.WarnNoDIYLock(diyLockWarnOut, planned)
	return func(context.Context) error { return nil }, nil
}

func (o Ops) NewDeploySteps(ctx context.Context, backend any, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
	scwPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("Scaleway ops received unexpected planned type %T", planned)
	}
	typed, ok := backend.(kube.Backend)
	if !ok {
		return nil, fmt.Errorf("Scaleway deploy steps require an infrastructure backend with outputs, got %T", backend)
	}
	spec := scwPlanned.Spec
	deploySpec := kube.DeploySpec{
		ImageDigest:     spec.Artifact.ImageDigest,
		DatabaseName:    spec.Dependencies.DatabaseName,
		ApplicationMode: spec.Application.Mode,
		WebRuntime:      spec.Application.WebRuntime,
		CPURequest:      spec.Catalog.CPURequest,
		MemoryRequest:   spec.Catalog.MemoryRequest,
		Region:          spec.Identity.Region,
	}
	newCandidate := o.NewCandidate
	if newCandidate == nil {
		newCandidate = func(context.Context, kube.Backend) (kube.CandidateRunner, error) {
			return kube.NewCandidateFromFactory(kube.ClientFromOutputs), nil
		}
	}
	newRuntime := o.NewRuntime
	if newRuntime == nil {
		newRuntime = func(_ context.Context, b kube.Backend) (kube.RuntimeChecker, error) {
			return kube.NewRuntimeFromFactory(b, kube.ClientFromOutputs)
		}
	}
	candidate, err := newCandidate(ctx, typed)
	if err != nil {
		return nil, err
	}
	runtime, err := newRuntime(ctx, typed)
	if err != nil {
		return nil, err
	}
	return kube.New(typed, deploySpec, candidate, runtime, diagnostics, o.RecordRelease)
}
