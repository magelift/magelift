// Package ops implements platform Magento ports for the experimental GCP
// StackModule without creating an import cycle between stack and deployment.
package ops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"cloud.google.com/go/storage"
	gcpbootstrap "github.com/magelift/magelift/internal/cloud/gcp/bootstrap"
	gcpedge "github.com/magelift/magelift/internal/cloud/gcp/edge"
	gcpstack "github.com/magelift/magelift/internal/cloud/gcp/stack"
	gcpstate "github.com/magelift/magelift/internal/cloud/gcp/state"
	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"google.golang.org/api/googleapi"
)

// Module is the GCP StackModule with Magento deploy Ops and day-2 ports.
type Module struct {
	RuntimeID sdk.RuntimeID
	platform.LifecycleFactories
}

func (m Module) stackModule() gcpstack.Module {
	return gcpstack.Module{RuntimeID: m.runtime(), LifecycleFactories: m.LifecycleFactories}
}

func (m Module) runtime() sdk.RuntimeID {
	if m.RuntimeID == "" {
		return gcptarget.RuntimeAutopilotID
	}
	return m.RuntimeID
}
func (m Module) Descriptor() sdk.TargetDescriptor { return m.stackModule().Descriptor() }
func (m Module) CertificationTier() platform.CertificationTier {
	return m.stackModule().CertificationTier()
}
func (m Module) PlanAdmission() platform.PlanAdmission { return m.stackModule().PlanAdmission() }
func (m Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	return m.stackModule().Plan(cfg, environment, opts)
}
func (m Module) Program(planned platform.PlannedStack) (pulumi.RunFunc, error) {
	return m.stackModule().Program(planned)
}
func (m Module) OutputKeys() []string { return m.stackModule().OutputKeys() }
func (m Module) Ops() platform.Ops    { return Ops{} }

func (m Module) NewEdge(ctx context.Context, planned platform.PlannedStack) (sdk.EdgeAdapter, error) {
	if m.Edge != nil {
		return m.Edge(ctx, planned)
	}
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP edge adapter received unexpected planned type %T", planned)
	}
	return gcpedge.NewNativeSDKLifecycleAdapter(ctx, gcpPlanned.GCPSpec().Identity.GCPProject, gcpedge.AlwaysHealthy{}, sdk.DefaultEdgeOperationPolicy())
}

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
		ImageDigest:        spec.Artifact.ImageDigest,
		DatabaseName:       spec.Dependencies.DatabaseName,
		ApplicationMode:    spec.Application.Mode,
		ApplicationVersion: spec.Application.Version,
		WebRuntime:         spec.Application.WebRuntime,
		Magento:            spec.Application.Magento,
		CPURequest:         spec.Catalog.AutopilotCPURequest,
		MemoryRequest:      spec.Catalog.AutopilotMemoryRequest,
		CloudProject:       spec.Identity.GCPProject,
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

// ErrStateBucketMissing is returned when certified deploy cannot lock because
// the DIY GCS state bucket was never bootstrapped.
var ErrStateBucketMissing = errors.New("deployment state bucket is missing")

var newGCSState = gcpstate.NewGCS

func acquireDeploymentLock(ctx context.Context, spec gcpstack.Spec) (func(context.Context) error, error) {
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, err
	}
	manager, err := newGCSState(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
	if err != nil {
		return nil, fmt.Errorf("create GCS state lock: %w", err)
	}
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	handle, err := manager.Acquire(ctx, spec.Identity.Project, spec.Identity.Environment, owner)
	if err != nil {
		if isGCSNotFound(err) {
			return nil, fmt.Errorf("%w: run magelift bootstrap --env %s: %w", ErrStateBucketMissing, spec.Identity.Environment, err)
		}
		return nil, err
	}
	return handle.Release, nil
}

func isGCSNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, storage.ErrBucketNotExist) || errors.Is(err, storage.ErrObjectNotExist) {
		return true
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == 404 {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "notfound") || strings.Contains(msg, "does not exist") || strings.Contains(msg, "doesn't exist")
}
