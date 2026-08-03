// Package ops implements platform.Ops for the certified AWS StackModule without
// creating an import cycle between stack and deployment.
package ops

import (
	"context"
	"fmt"
	"io"
	"os"

	awsbootstrap "github.com/magelift/magelift/internal/cloud/aws/bootstrap"
	awsdeployment "github.com/magelift/magelift/internal/cloud/aws/deployment"
	awsoperations "github.com/magelift/magelift/internal/cloud/aws/operations"
	awsstack "github.com/magelift/magelift/internal/cloud/aws/stack"
	awsstate "github.com/magelift/magelift/internal/cloud/aws/state"
	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Module is the AWS StackModule with Magento deploy Ops attached.
type Module struct{}

func (Module) Descriptor() sdk.TargetDescriptor { return awsstack.Module{}.Descriptor() }
func (Module) CertificationTier() platform.CertificationTier {
	return awsstack.Module{}.CertificationTier()
}
func (Module) Plan(cfg config.Config, environment string, opts platform.PlanOptions) (platform.PlannedStack, error) {
	return awsstack.Module{}.Plan(cfg, environment, opts)
}
func (Module) Program(planned platform.PlannedStack) (pulumi.RunFunc, error) {
	return awsstack.Module{}.Program(planned)
}
func (Module) OutputKeys() []string { return awsstack.Module{}.OutputKeys() }
func (Module) Ops() platform.Ops    { return Ops{} }

// Ops implements platform.Ops for AWS.
type Ops struct {
	NewDeployment func(context.Context, string) (awsdeployment.CandidateRunner, error)
	NewRuntime    func(context.Context, string) (awsdeployment.RuntimeChecker, error)
	RecordRelease func(context.Context, deployflow.Request, deployflow.Result) error
}

// WithRecordRelease implements platform.HasRecordRelease.
func (o Ops) WithRecordRelease(fn func(context.Context, deployflow.Request, deployflow.Result) error) platform.Ops {
	o.RecordRelease = fn
	return o
}

func (o Ops) AcquireLock(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	awsPlanned, ok := awsstack.AsAWSPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("AWS ops received unexpected planned type %T", planned)
	}
	return acquireDeploymentLock(ctx, awsPlanned.AWSSpec())
}

func (o Ops) NewDeploySteps(ctx context.Context, backend any, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
	awsPlanned, ok := awsstack.AsAWSPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("AWS ops received unexpected planned type %T", planned)
	}
	typed, ok := backend.(awsdeployment.Backend)
	if !ok {
		return nil, fmt.Errorf("AWS deploy steps require an infrastructure backend with outputs, got %T", backend)
	}
	spec := awsPlanned.AWSSpec()
	newDeployment := o.NewDeployment
	if newDeployment == nil {
		newDeployment = func(ctx context.Context, region string) (awsdeployment.CandidateRunner, error) {
			return awsoperations.NewDeployment(ctx, region)
		}
	}
	newRuntime := o.NewRuntime
	if newRuntime == nil {
		newRuntime = func(ctx context.Context, region string) (awsdeployment.RuntimeChecker, error) {
			return awsoperations.NewRuntime(ctx, region)
		}
	}
	candidate, err := newDeployment(ctx, spec.Identity.Region)
	if err != nil {
		return nil, err
	}
	runtime, err := newRuntime(ctx, spec.Identity.Region)
	if err != nil {
		return nil, err
	}
	return awsdeployment.New(typed, spec, candidate, runtime, diagnostics, o.RecordRelease)
}

func acquireDeploymentLock(ctx context.Context, spec awsstack.Spec) (func(context.Context) error, error) {
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return nil, err
	}
	manager, err := awsstate.NewAWS(ctx, spec.Identity.Region, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment, awsstate.ObjectEncryption{Mode: awsstate.EncryptionKMS, KMSKeyARN: spec.Dependencies.KMSKeyARN})
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	handle, err := manager.Acquire(ctx, spec.Identity.Project, spec.Identity.Environment, owner)
	if err != nil {
		return nil, err
	}
	return handle.Release, nil
}
