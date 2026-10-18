package stack

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/magelift/magelift/internal/cloud/kube"
	ovhcost "github.com/magelift/magelift/internal/cloud/ovh/cost"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
)

// unsupported covers remaining experimental gaps: Bootstrap and application
// Secrets when the module was not given a provider-local OKMS client factory.
// Observe/Steps/State live on shared kube / concrete State adapters (D-05).
type unsupported struct{}

func (Module) Bootstrap() platform.Bootstrap { return unsupported{} }
func (Module) State() platform.State         { return State{} }
func (m Module) Secrets() platform.Secrets {
	if m.NewApplicationSecretClient == nil {
		return unsupported{}
	}
	return NewSecrets(m.NewApplicationSecretClient)
}
func (Module) RuntimeObserve() platform.RuntimeObserve {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) RuntimeTunnel() platform.RuntimeTunnel {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) CostEstimator() platform.CostEstimator { return ovhcost.Estimator{} }

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
	ovhPlanned, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("OVH ops received unexpected planned type %T", planned)
	}
	typed, ok := backend.(kube.Backend)
	if !ok {
		return nil, fmt.Errorf("OVH deploy steps require an infrastructure backend with outputs, got %T", backend)
	}
	spec := ovhPlanned.Spec
	deploySpec := kube.DeploySpec{
		ImageDigest:        spec.Artifact.ImageDigest,
		DatabaseName:       spec.Dependencies.DatabaseName,
		ApplicationMode:    spec.Application.Mode,
		ApplicationVersion: spec.Application.Version,
		WebRuntime:         spec.Application.WebRuntime,
		Magento:            spec.Application.Magento,
		CPURequest:         spec.Catalog.CPURequest,
		MemoryRequest:      spec.Catalog.MemoryRequest,
		Region:             spec.Identity.Region,
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
