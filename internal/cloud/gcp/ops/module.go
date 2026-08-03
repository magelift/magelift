// Package ops implements platform Magento ports for the experimental GCP
// StackModule without creating an import cycle between stack and deployment.
package ops

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	gcpbootstrap "github.com/acourtiol/magelift/internal/cloud/gcp/bootstrap"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	gcpstate "github.com/acourtiol/magelift/internal/cloud/gcp/state"
	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/config"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Module is the GCP StackModule with Magento deploy Ops and day-2 ports.
type Module struct{}

func (Module) Descriptor() sdk.TargetDescriptor { return gcpstack.Module{}.Descriptor() }
func (Module) CertificationTier() platform.CertificationTier {
	return gcpstack.Module{}.CertificationTier()
}
func (Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	return gcpstack.Module{}.Plan(cfg, environment, opts)
}
func (Module) Program(planned platform.PlannedStack) (pulumi.RunFunc, error) {
	return gcpstack.Module{}.Program(planned)
}
func (Module) OutputKeys() []string { return gcpstack.Module{}.OutputKeys() }
func (Module) Ops() platform.Ops    { return Ops{} }

// Ops implements platform.Ops for GCP GKE Autopilot.
type Ops struct {
	NewCandidate  func(context.Context, kube.Backend) (kube.CandidateRunner, error)
	NewRuntime    func(context.Context, kube.Backend) (kube.RuntimeChecker, error)
	RecordRelease func(context.Context, deployflow.Request, deployflow.Result) error
}

// WithRecordRelease implements platform.HasRecordRelease.
func (o Ops) WithRecordRelease(fn func(context.Context, deployflow.Request, deployflow.Result) error) platform.Ops {
	o.RecordRelease = fn
	return o
}

func (o Ops) AcquireLock(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP ops received unexpected planned type %T", planned)
	}
	return acquireDeploymentLock(ctx, gcpPlanned.GCPSpec())
}

func (o Ops) NewDeploySteps(ctx context.Context, backend any, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP ops received unexpected planned type %T", planned)
	}
	typed, ok := backend.(kube.Backend)
	if !ok {
		return nil, fmt.Errorf("GCP deploy steps require an infrastructure backend with outputs, got %T", backend)
	}
	spec := gcpPlanned.GCPSpec()
	deploySpec := kube.DeploySpec{
		ImageDigest:     spec.Artifact.ImageDigest,
		DatabaseName:    spec.Dependencies.DatabaseName,
		ApplicationMode: spec.Application.Mode,
		WebRuntime:      spec.Application.WebRuntime,
		CPURequest:      spec.Catalog.AutopilotCPURequest,
		MemoryRequest:   spec.Catalog.AutopilotMemoryRequest,
		CloudProject:    spec.Identity.GCPProject,
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

func acquireDeploymentLock(ctx context.Context, spec gcpstack.Spec) (func(context.Context) error, error) {
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, err
	}
	manager, err := gcpstate.NewGCS(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
	if err != nil {
		// Experimental: when DIY state is not bootstrapped yet, allow deploy without lock.
		return func(context.Context) error { return nil }, nil
	}
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	handle, err := manager.Acquire(ctx, spec.Identity.Project, spec.Identity.Environment, owner)
	if err != nil {
		// Bucket missing (404) means bootstrap was never run — same experimental skip.
		if isGCSNotFound(err) {
			return func(context.Context) error { return nil }, nil
		}
		return nil, err
	}
	return func(context.Context) error { return handle.Release() }, nil
}

func isGCSNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") || strings.Contains(msg, "does not exist")
}
