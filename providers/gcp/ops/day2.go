package ops

import (
	"context"
	"errors"

	"github.com/magelift/magelift/internal/platform"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpsecrets "github.com/magelift/magelift/providers/gcp/secrets"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
)

// Bootstrap implements platform.Bootstrap for GCS DIY state + GitHub WIF identity.
type Bootstrap struct{}

func (Bootstrap) VerifyAccount(ctx context.Context, spec gcpstack.Spec) error {
	return gcpbootstrap.VerifyAccount(ctx, spec.Identity.GCPProject)
}

func (Bootstrap) Ensure(ctx context.Context, spec gcpstack.Spec, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	owner, repo, wantGitHub, err := req.GitHubIdentity()
	if err != nil {
		return platform.BootstrapResult{}, err
	}
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
	details := map[string]any{"state": result}
	if wantGitHub {
		identityPlan, err := gcpbootstrap.BuildIdentityPlan(gcpbootstrap.IdentitySpec{
			Project: spec.Identity.Project, Environment: spec.Identity.Environment,
			GCPProject:  spec.Identity.GCPProject,
			GitHubOwner: owner, GitHubRepo: repo,
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
		details["wif"] = map[string]any{
			"pool":           wif.PoolResource,
			"provider":       wif.ProviderResource,
			"serviceAccount": wif.ServiceAccountEmail,
		}
	}
	return platform.BootstrapResult{
		BackendURL: gcpbootstrap.BackendURL(result.Plan),
		Details:    details,
	}, nil
}

// State implements platform.State for GCS DIY locks.
type State struct{}

func (State) Status(ctx context.Context, spec gcpstack.Spec) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := stateManager(ctx, spec)
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

func (State) Lock(ctx context.Context, spec gcpstack.Spec, owner string) error {
	_, err := acquireDeploymentLock(ctx, spec, owner)
	return err
}

func (State) Unlock(ctx context.Context, spec gcpstack.Spec) (*platform.LockInfo, error) {
	manager, _, err := stateManager(ctx, spec)
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

func (State) Backup(ctx context.Context, spec gcpstack.Spec) (platform.BackupResult, error) {
	archive, err := stateArchive(ctx, spec)
	if err != nil {
		return platform.BackupResult{}, err
	}
	result, err := archive.Backup(ctx)
	if err != nil {
		return platform.BackupResult{}, err
	}
	return platform.BackupResult{ID: result.ID, Location: result.Prefix, Objects: result.Objects, Bytes: result.Bytes, ManifestDigest: result.ManifestDigest}, nil
}

func (State) Restore(ctx context.Context, spec gcpstack.Spec, location string) (platform.RestoreResult, error) {
	archive, err := stateArchive(ctx, spec)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	result, err := archive.Restore(ctx, location)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	return platform.RestoreResult{ID: result.ID, Location: result.Prefix, Objects: result.Objects, Bytes: result.Bytes, ManifestDigest: result.ManifestDigest}, nil
}

// Secrets implements platform.Secrets for Secret Manager.
type Secrets struct{}

func (Secrets) List(ctx context.Context, spec gcpstack.Spec) ([]platform.SecretMeta, error) {
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := store.List(ctx, spec.Identity.GCPProject)
	if err != nil {
		return nil, err
	}
	out := make([]platform.SecretMeta, 0, len(listed))
	for _, item := range listed {
		out = append(out, platform.SecretMeta{Name: item.Name})
	}
	return out, nil
}

func (Secrets) Set(ctx context.Context, spec gcpstack.Spec, name string, value []byte) error {
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return err
	}
	return store.Set(ctx, spec.Identity.GCPProject, name, value)
}

func (Secrets) Remove(ctx context.Context, spec gcpstack.Spec, name string) error {
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return err
	}
	return store.Remove(ctx, spec.Identity.GCPProject, name)
}

func stateManager(ctx context.Context, spec gcpstack.Spec) (*gcpstate.Manager, string, error) {
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

func stateArchive(ctx context.Context, spec gcpstack.Spec) (*gcpstate.Archive, error) {
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
