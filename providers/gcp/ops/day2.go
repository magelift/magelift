package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/magelift/magelift/internal/platform"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpsecrets "github.com/magelift/magelift/providers/gcp/secrets"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"google.golang.org/api/googleapi"
)

// Bootstrap implements platform.Bootstrap for GCS DIY state + GitHub WIF identity.
// Nil factory fields select production GCP clients.
type Bootstrap struct {
	NewEnsurer      func(context.Context) (*gcpbootstrap.Bootstrapper, error)
	NewIdentity     func(context.Context) (*gcpbootstrap.IdentityBootstrapper, error)
	VerifyAccountFn func(context.Context, string) error
}

func (b Bootstrap) VerifyAccount(ctx context.Context, spec gcpstack.Spec) error {
	verify := b.VerifyAccountFn
	if verify == nil {
		verify = gcpbootstrap.VerifyAccount
	}
	return verify(ctx, spec.Identity.GCPProject)
}

func (b Bootstrap) Ensure(ctx context.Context, spec gcpstack.Spec, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
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
	newEnsurer := b.NewEnsurer
	if newEnsurer == nil {
		newEnsurer = gcpbootstrap.New
	}
	ensurer, err := newEnsurer(ctx)
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
		newIdentity := b.NewIdentity
		if newIdentity == nil {
			newIdentity = gcpbootstrap.NewIdentity
		}
		identity, err := newIdentity(ctx)
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

// ErrStateBucketMissing is returned when locking cannot proceed because
// the DIY GCS state bucket was never bootstrapped.
var ErrStateBucketMissing = errors.New("deployment state bucket is missing")

// State implements platform.State for GCS DIY locks.
// Nil factory fields select production GCP clients.
type State struct {
	NewManager func(context.Context, string, string, string) (*gcpstate.Manager, error)
	NewArchive func(context.Context, string, string, string) (*gcpstate.Archive, error)
}

func (s State) Status(ctx context.Context, spec gcpstack.Spec) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := s.stateManager(ctx, spec)
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

func (s State) Lock(ctx context.Context, spec gcpstack.Spec, owner string) error {
	if strings.TrimSpace(owner) == "" {
		return errors.New("lock owner is required")
	}
	manager, _, err := s.stateManager(ctx, spec)
	if err != nil {
		return err
	}
	_, err = manager.Acquire(ctx, spec.Identity.Project, spec.Identity.Environment, owner)
	if err != nil {
		if isGCSNotFound(err) {
			return fmt.Errorf("%w: run magelift bootstrap --env %s: %w", ErrStateBucketMissing, spec.Identity.Environment, err)
		}
		return err
	}
	return nil
}

func (s State) Unlock(ctx context.Context, spec gcpstack.Spec) (*platform.LockInfo, error) {
	manager, _, err := s.stateManager(ctx, spec)
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

func (s State) Backup(ctx context.Context, spec gcpstack.Spec) (platform.BackupResult, error) {
	archive, err := s.stateArchive(ctx, spec)
	if err != nil {
		return platform.BackupResult{}, err
	}
	result, err := archive.Backup(ctx)
	if err != nil {
		return platform.BackupResult{}, err
	}
	return platform.BackupResult{ID: result.ID, Location: result.Prefix, Objects: result.Objects, Bytes: result.Bytes, ManifestDigest: result.ManifestDigest}, nil
}

func (s State) Restore(ctx context.Context, spec gcpstack.Spec, location string) (platform.RestoreResult, error) {
	archive, err := s.stateArchive(ctx, spec)
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
// A nil NewStore selects the production GCP client.
type Secrets struct {
	NewStore func(context.Context) (*gcpsecrets.Store, error)
}

func (s Secrets) newStore(ctx context.Context) (*gcpsecrets.Store, error) {
	if s.NewStore != nil {
		return s.NewStore(ctx)
	}
	return gcpsecrets.NewStore(ctx)
}

// ReadValue resolves one full version resource name for secretref and
// Composer credential flows.
func (s Secrets) ReadValue(ctx context.Context, name string) ([]byte, error) {
	store, err := s.newStore(ctx)
	if err != nil {
		return nil, err
	}
	return store.GetSecretValue(ctx, name)
}

func (s Secrets) List(ctx context.Context, spec gcpstack.Spec) ([]platform.SecretMeta, error) {
	store, err := s.newStore(ctx)
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

func (s Secrets) Set(ctx context.Context, spec gcpstack.Spec, name string, value []byte) error {
	store, err := s.newStore(ctx)
	if err != nil {
		return err
	}
	return store.Set(ctx, spec.Identity.GCPProject, name, value)
}

func (s Secrets) Remove(ctx context.Context, spec gcpstack.Spec, name string) error {
	store, err := s.newStore(ctx)
	if err != nil {
		return err
	}
	return store.Remove(ctx, spec.Identity.GCPProject, name)
}

func (s State) stateManager(ctx context.Context, spec gcpstack.Spec) (*gcpstate.Manager, string, error) {
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, "", err
	}
	newManager := s.NewManager
	if newManager == nil {
		newManager = gcpstate.NewGCS
	}
	manager, err := newManager(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
	if err != nil {
		return nil, "", err
	}
	return manager, plan.StateBucket, nil
}

func (s State) stateArchive(ctx context.Context, spec gcpstack.Spec) (*gcpstate.Archive, error) {
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, err
	}
	newArchive := s.NewArchive
	if newArchive == nil {
		newArchive = gcpstate.NewArchiveGCS
	}
	return newArchive(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
}

func toLockInfo(info gcpstate.Info) platform.LockInfo {
	return platform.LockInfo{
		Project: info.Project, Environment: info.Environment,
		Owner: info.Owner, AcquiredAt: info.AcquiredAt,
	}
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
