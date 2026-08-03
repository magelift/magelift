package stack

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
)

// unsupported covers remaining experimental gaps: Bootstrap, Secrets, Cost only.
// Observe/Steps/State live on shared kube / concrete State adapters (D-05).
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

func (Ops) AcquireLock(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	return (State{}).Lock(ctx, planned, owner)
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
