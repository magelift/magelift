package eksops

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/magelift/magelift/internal/cloud/kube"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/s3state"
	"github.com/magelift/magelift/internal/usererr"
	awsbootstrap "github.com/magelift/magelift/providers/aws/bootstrap"
	awscost "github.com/magelift/magelift/providers/aws/cost"
	awssecrets "github.com/magelift/magelift/providers/aws/secrets"
)

func (Module) Bootstrap() platform.Bootstrap { return Bootstrap{} }
func (Module) State() platform.State         { return s3state.Port(stateTarget) }
func (Module) Secrets() platform.Secrets     { return Secrets{} }
func (Module) RuntimeObserve() platform.RuntimeObserve {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) RuntimeTunnel() platform.RuntimeTunnel {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) CostEstimator() platform.CostEstimator { return awscost.Estimator{} }
func (Module) Ops() platform.Ops                     { return Ops{} }

// Bootstrap provisions the state bucket and the optional GitHub deployment role.
type Bootstrap struct{}

func (Bootstrap) VerifyAccount(ctx context.Context, planned platform.PlannedStack) error {
	eksPlanned, ok := AsEKSPlanned(planned)
	if !ok {
		return fmt.Errorf("EKS bootstrap received unexpected planned type %T", planned)
	}
	return awsbootstrap.VerifyAccount(ctx, planned.Region(), eksPlanned.Spec.Identity.AccountID)
}

func (Bootstrap) Ensure(ctx context.Context, planned platform.PlannedStack, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	if strings.TrimSpace(req.AccessLogBucket) == "" {
		return platform.BootstrapResult{}, usererr.New(
			"AWS bootstrap needs an existing log bucket",
			"magelift bootstrap --env "+planned.Environment()+" --access-log-bucket <existing-bucket>",
			"docs/getting-started.md",
		)
	}
	owner, repo, wantGitHub, err := req.GitHubIdentity()
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	eksPlanned, ok := AsEKSPlanned(planned)
	if !ok {
		return platform.BootstrapResult{}, fmt.Errorf("EKS bootstrap received unexpected planned type %T", planned)
	}
	spec := eksPlanned.Spec
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.BootstrapEnvironment(),
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: req.AccessLogBucket,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	ensurer, err := awsbootstrap.NewAWS(ctx, spec.Identity.Region)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	result, err := ensurer.Ensure(ctx, plan, spec.Identity.Region)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	details := map[string]any{"state": result}
	if wantGitHub {
		identityPlan, err := awsbootstrap.BuildIdentityPlan(awsbootstrap.IdentitySpec{
			Project: spec.Identity.Project, Environment: spec.Identity.BootstrapEnvironment(),
			AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
			GitHubOwner: owner, GitHubRepo: repo,
			StateBucket: result.Plan.StateBucket, KMSKeyARN: result.KeyARN,
		})
		if err != nil {
			return platform.BootstrapResult{}, err
		}
		identity, err := awsbootstrap.NewAWSIdentity(ctx, spec.Identity.Region)
		if err != nil {
			return platform.BootstrapResult{}, err
		}
		if err := identity.Ensure(ctx, identityPlan); err != nil {
			return platform.BootstrapResult{}, err
		}
		details["identity"] = identityPlan
	}
	return platform.BootstrapResult{
		BackendURL: "s3://" + result.Plan.StateBucket,
		KeyRef:     result.KeyARN,
		Details:    details,
	}, nil
}

type Secrets struct{}

func (Secrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	store, err := awssecrets.NewStore(ctx, planned.Region())
	if err != nil {
		return nil, err
	}
	listed, err := store.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]platform.SecretMeta, 0, len(listed))
	for _, item := range listed {
		out = append(out, platform.SecretMeta{Name: item.Name})
	}
	return out, nil
}

func (Secrets) Set(ctx context.Context, planned platform.PlannedStack, name string, value []byte) error {
	store, err := awssecrets.NewStore(ctx, planned.Region())
	if err != nil {
		return err
	}
	return store.Set(ctx, name, value)
}

func (Secrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	store, err := awssecrets.NewStore(ctx, planned.Region())
	if err != nil {
		return err
	}
	return store.Remove(ctx, name)
}

type Ops struct {
	NewCandidate  func(context.Context, kube.Backend) (kube.CandidateRunner, error)
	NewRuntime    func(context.Context, kube.Backend) (kube.RuntimeChecker, error)
	RecordRelease func(context.Context, deployflow.Request, deployflow.Result) error
}

func (Ops) AcquireLock(ctx context.Context, planned platform.PlannedStack) (func(context.Context) error, error) {
	host, _ := os.Hostname()
	owner := fmt.Sprintf("magelift-cli-%s-%d", host, os.Getpid())
	return s3state.Port(stateTarget).Lock(ctx, planned, owner)
}

func (o Ops) NewDeploySteps(ctx context.Context, backend any, planned platform.PlannedStack, diagnostics io.Writer) (deployflow.Steps, error) {
	eksPlanned, ok := AsEKSPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("EKS ops received unexpected planned type %T", planned)
	}
	typed, ok := backend.(kube.Backend)
	if !ok {
		return nil, fmt.Errorf("EKS deploy steps require an infrastructure backend with outputs, got %T", backend)
	}
	spec := eksPlanned.Spec
	deploySpec := kube.DeploySpec{
		ImageDigest:        spec.Artifact.ImageDigest,
		DatabaseName:       spec.Dependencies.DatabaseName,
		ApplicationMode:    spec.Application.Mode,
		ApplicationVersion: spec.Application.Version,
		WebRuntime:         spec.Application.WebRuntime,
		Magento:            spec.Application.Magento,
		CPURequest:         spec.Catalog.CPURequest,
		MemoryRequest:      spec.Catalog.MemoryRequest,
		CloudProject:       spec.Identity.AccountID,
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

func stateTarget(planned platform.PlannedStack) (s3state.Target, error) {
	eksPlanned, ok := AsEKSPlanned(planned)
	if !ok {
		return s3state.Target{}, fmt.Errorf("EKS state received unexpected planned type %T", planned)
	}
	spec := eksPlanned.Spec
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.BootstrapEnvironment(),
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return s3state.Target{}, err
	}
	return s3state.Target{Region: spec.Identity.Region, Bucket: plan.StateBucket, Encryption: s3state.ObjectEncryption{Mode: s3state.EncryptionKMS, KMSKeyARN: spec.Dependencies.KMSKeyARN}}, nil
}
