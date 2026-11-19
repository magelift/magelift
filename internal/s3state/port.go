package s3state

import (
	"context"
	"errors"
	"strings"

	"github.com/magelift/magelift/internal/awsendpoint"
	"github.com/magelift/magelift/internal/platform"
)

// Target holds the S3 connection and encryption settings selected by an adapter.
type Target struct {
	Region, Bucket, Endpoint string
	Encryption               ObjectEncryption
}

type Port func(platform.PlannedStack) (Target, error)

func (port Port) target(p platform.PlannedStack) (Target, error) {
	t, err := port(p)
	if err != nil {
		return Target{}, err
	}
	if strings.TrimSpace(t.Endpoint) == "" {
		t.Endpoint, err = awsendpoint.FromEnv()
	} else {
		t.Endpoint, err = awsendpoint.Parse(t.Endpoint)
	}
	return t, err
}

func (port Port) manager(ctx context.Context, p platform.PlannedStack) (*Manager, string, error) {
	t, err := port.target(p)
	if err != nil {
		return nil, "", err
	}
	manager, err := NewAWSWithEndpoint(ctx, t.Region, t.Bucket, p.Project(), p.Environment(), t.Encryption, t.Endpoint)
	return manager, t.Bucket, err
}
func (port Port) archive(ctx context.Context, p platform.PlannedStack) (*Archive, error) {
	t, err := port.target(p)
	if err != nil {
		return nil, err
	}
	return NewAWSArchiveWithEndpoint(ctx, t.Region, t.Bucket, t.Encryption, t.Endpoint)
}
func lockInfo(info Info) *platform.LockInfo {
	return &platform.LockInfo{Project: info.Project, Environment: info.Environment, Owner: info.Owner, AcquiredAt: info.AcquiredAt}
}

func (port Port) Status(ctx context.Context, p platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := port.manager(ctx, p)
	if err != nil {
		return false, nil, "", err
	}
	info, err := manager.Status(ctx)
	if errors.Is(err, ErrNotLocked) {
		return false, nil, bucket, nil
	}
	if err != nil {
		return false, nil, bucket, err
	}
	return true, lockInfo(info), bucket, nil
}
func (port Port) Lock(ctx context.Context, p platform.PlannedStack, owner string) (func(context.Context) error, error) {
	manager, _, err := port.manager(ctx, p)
	if err != nil {
		return nil, err
	}
	handle, err := manager.Acquire(ctx, p.Project(), p.Environment(), owner)
	if err != nil {
		return nil, err
	}
	return handle.Release, nil
}
func (port Port) Unlock(ctx context.Context, p platform.PlannedStack) (*platform.LockInfo, error) {
	manager, _, err := port.manager(ctx, p)
	if err != nil {
		return nil, err
	}
	info, err := manager.Unlock(ctx)
	if errors.Is(err, ErrNotLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return lockInfo(info), nil
}
func (port Port) Backup(ctx context.Context, p platform.PlannedStack) (platform.BackupResult, error) {
	a, err := port.archive(ctx, p)
	if err != nil {
		return platform.BackupResult{}, err
	}
	r, err := a.Backup(ctx)
	if err != nil {
		return platform.BackupResult{}, err
	}
	return platform.BackupResult{ID: r.ID, Location: r.Prefix, Objects: r.Objects, Bytes: r.Bytes, ManifestDigest: r.ManifestDigest}, nil
}
func (port Port) Restore(ctx context.Context, p platform.PlannedStack, id string) (platform.RestoreResult, error) {
	a, err := port.archive(ctx, p)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	r, err := a.Restore(ctx, id)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	return platform.RestoreResult{ID: r.ID, Location: r.Prefix, Objects: r.Objects, Bytes: r.Bytes, ManifestDigest: r.ManifestDigest}, nil
}
