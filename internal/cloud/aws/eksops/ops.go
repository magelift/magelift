package eksops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	awsbootstrap "github.com/magelift/magelift/internal/cloud/aws/bootstrap"
	awscost "github.com/magelift/magelift/internal/cloud/aws/cost"
	awssecrets "github.com/magelift/magelift/internal/cloud/aws/secrets"
	awsstate "github.com/magelift/magelift/internal/cloud/aws/state"
	"github.com/magelift/magelift/internal/cloud/kube"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/usererr"
)

func (Module) Bootstrap() platform.Bootstrap { return Bootstrap{} }
func (Module) State() platform.State         { return State{} }
func (Module) Secrets() platform.Secrets     { return Secrets{} }
func (Module) RuntimeObserve() platform.RuntimeObserve {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) RuntimeTunnel() platform.RuntimeTunnel {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) CostEstimator() platform.CostEstimator { return awscost.Estimator{} }
func (Module) Ops() platform.Ops                     { return Ops{} }

// Bootstrap reuses the certified AWS account DIY bootstrap packages.
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
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
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
			Project: spec.Identity.Project, Environment: spec.Identity.Environment,
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

type State struct{}

func (State) Status(ctx context.Context, planned platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := stateManager(ctx, planned)
	if err != nil {
		return false, nil, "", err
	}
	info, err := manager.Status(ctx)
	if errors.Is(err, awsstate.ErrNotLocked) {
		return false, nil, bucket, nil
	}
	if err != nil {
		return false, nil, bucket, err
	}
	meta := platform.LockInfo{Project: info.Project, Environment: info.Environment, Owner: info.Owner, AcquiredAt: info.AcquiredAt}
	return true, &meta, bucket, nil
}

func (State) Lock(ctx context.Context, planned platform.PlannedStack, owner string) (func(context.Context) error, error) {
	manager, _, err := stateManager(ctx, planned)
	if err != nil {
		return nil, err
	}
	release, err := manager.Lock(ctx, planned.Project(), planned.Environment(), owner)
	if err != nil {
		return nil, err
	}
	return func(context.Context) error { return release() }, nil
}

func (State) Unlock(ctx context.Context, planned platform.PlannedStack) (*platform.LockInfo, error) {
	manager, _, err := stateManager(ctx, planned)
	if err != nil {
		return nil, err
	}
	info, err := manager.Unlock(ctx)
	if errors.Is(err, awsstate.ErrNotLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	meta := platform.LockInfo{Project: info.Project, Environment: info.Environment, Owner: info.Owner, AcquiredAt: info.AcquiredAt}
	return &meta, nil
}

func (State) Backup(ctx context.Context, planned platform.PlannedStack) (platform.BackupResult, error) {
	archive, err := stateArchive(ctx, planned)
	if err != nil {
		return platform.BackupResult{}, err
	}
	result, err := archive.Backup(ctx)
	if err != nil {
		return platform.BackupResult{}, err
	}
	return platform.BackupResult{ID: result.ID, Location: result.Prefix, Objects: result.Objects, Bytes: result.Bytes, ManifestDigest: result.ManifestDigest}, nil
}

func (State) Restore(ctx context.Context, planned platform.PlannedStack, location string) (platform.RestoreResult, error) {
	archive, err := stateArchive(ctx, planned)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	result, err := archive.Restore(ctx, location)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	return platform.RestoreResult{ID: result.ID, Location: result.Prefix, Objects: result.Objects, Bytes: result.Bytes, ManifestDigest: result.ManifestDigest}, nil
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
	return (State{}).Lock(ctx, planned, owner)
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

func stateManager(ctx context.Context, planned platform.PlannedStack) (*awsstate.Manager, string, error) {
	eksPlanned, ok := AsEKSPlanned(planned)
	if !ok {
		return nil, "", fmt.Errorf("EKS state received unexpected planned type %T", planned)
	}
	spec := eksPlanned.Spec
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return nil, "", err
	}
	manager, err := awsstate.NewAWS(ctx, spec.Identity.Region, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment, awsstate.ObjectEncryption{Mode: awsstate.EncryptionKMS, KMSKeyARN: spec.Dependencies.KMSKeyARN})
	if err != nil {
		return nil, "", err
	}
	return manager, plan.StateBucket, nil
}

func stateArchive(ctx context.Context, planned platform.PlannedStack) (*awsstate.Archive, error) {
	eksPlanned, ok := AsEKSPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("EKS state archive received unexpected planned type %T", planned)
	}
	spec := eksPlanned.Spec
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return nil, err
	}
	return awsstate.NewAWSArchive(ctx, spec.Identity.Region, plan.StateBucket, awsstate.ObjectEncryption{Mode: awsstate.EncryptionKMS, KMSKeyARN: spec.Dependencies.KMSKeyARN})
}
