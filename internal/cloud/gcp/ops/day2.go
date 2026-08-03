package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gcpbootstrap "github.com/acourtiol/magelift/internal/cloud/gcp/bootstrap"
	gcpcost "github.com/acourtiol/magelift/internal/cloud/gcp/cost"
	gcpsecrets "github.com/acourtiol/magelift/internal/cloud/gcp/secrets"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	gcpstate "github.com/acourtiol/magelift/internal/cloud/gcp/state"
	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/platform"
)

func (Module) Bootstrap() platform.Bootstrap           { return Bootstrap{} }
func (Module) State() platform.State                   { return State{} }
func (Module) Secrets() platform.Secrets               { return Secrets{} }
func (Module) RuntimeObserve() platform.RuntimeObserve {
	return kube.NewObserveWithFactory(kube.ClientFromOutputs)
}
func (Module) CostEstimator() platform.CostEstimator { return gcpcost.Estimator{} }

// Bootstrap implements platform.Bootstrap for GCS DIY state + GitHub WIF identity.
type Bootstrap struct{}

func (Bootstrap) VerifyAccount(ctx context.Context, planned platform.PlannedStack) error {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return fmt.Errorf("GCP bootstrap received unexpected planned type %T", planned)
	}
	return gcpbootstrap.VerifyAccount(ctx, gcpPlanned.GCPSpec().Identity.GCPProject)
}

func (Bootstrap) Ensure(ctx context.Context, planned platform.PlannedStack, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	if strings.TrimSpace(req.GitHubOwner) == "" || strings.TrimSpace(req.GitHubRepo) == "" {
		return platform.BootstrapResult{}, fmt.Errorf("--github-owner and --github-repo are required")
	}
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return platform.BootstrapResult{}, fmt.Errorf("GCP bootstrap received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	ensurer, err := gcpbootstrap.New(ctx)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	result, err := ensurer.Ensure(ctx, plan)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	identityPlan, err := gcpbootstrap.BuildIdentityPlan(gcpbootstrap.IdentitySpec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject,
		GitHubOwner: req.GitHubOwner, GitHubRepo: req.GitHubRepo,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	identity, err := gcpbootstrap.NewIdentity(ctx)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	wif, err := identity.Ensure(ctx, identityPlan)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	return platform.BootstrapResult{
		BackendURL: gcpbootstrap.BackendURL(result.Plan),
		Details: map[string]any{
			"state": result,
			"wif": map[string]any{
				"pool":           wif.PoolResource,
				"provider":       wif.ProviderResource,
				"serviceAccount": wif.ServiceAccountEmail,
			},
		},
	}, nil
}

// State implements platform.State for GCS DIY locks.
type State struct{}

func (State) Status(ctx context.Context, planned platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := stateManager(ctx, planned)
	if err != nil {
		return false, nil, "", err
	}
	info, err := manager.Status(ctx)
	if errors.Is(err, gcpstate.ErrNotLocked) {
		return false, nil, bucket, nil
	}
	if err != nil {
		return false, nil, bucket, err
	}
	meta := toLockInfo(info)
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
	if errors.Is(err, gcpstate.ErrNotLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	meta := toLockInfo(info)
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
	return platform.BackupResult{ID: result.ID, Location: result.Prefix}, nil
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
	return platform.RestoreResult{ID: result.ID, Location: result.Prefix}, nil
}

// Secrets implements platform.Secrets for Secret Manager.
type Secrets struct{}

func (Secrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP secrets received unexpected planned type %T", planned)
	}
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := store.List(ctx, gcpPlanned.GCPSpec().Identity.GCPProject)
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
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return fmt.Errorf("GCP secrets received unexpected planned type %T", planned)
	}
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return err
	}
	return store.Set(ctx, gcpPlanned.GCPSpec().Identity.GCPProject, name, value)
}

func (Secrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return fmt.Errorf("GCP secrets received unexpected planned type %T", planned)
	}
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return err
	}
	return store.Remove(ctx, gcpPlanned.GCPSpec().Identity.GCPProject, name)
}

func stateManager(ctx context.Context, planned platform.PlannedStack) (*gcpstate.Manager, string, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, "", fmt.Errorf("GCP state received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, "", err
	}
	manager, err := gcpstate.NewGCS(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
	if err != nil {
		return nil, "", err
	}
	return manager, plan.StateBucket, nil
}

func stateArchive(ctx context.Context, planned platform.PlannedStack) (*gcpstate.Archive, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP state received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, err
	}
	return gcpstate.NewArchiveGCS(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
}

func toLockInfo(info gcpstate.Info) platform.LockInfo {
	return platform.LockInfo{
		Project: info.Project, Environment: info.Environment,
		Owner: info.Owner, AcquiredAt: info.AcquiredAt,
	}
}
