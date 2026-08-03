package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	awsendpoint "github.com/acourtiol/magelift/internal/cloud/aws/endpoint"
	awsstate "github.com/acourtiol/magelift/internal/cloud/aws/state"
	"github.com/acourtiol/magelift/internal/platform"
)

// State implements platform.State for Scaleway via the shared S3-compatible manager.
type State struct{}

func diyObjectEncryption() awsstate.ObjectEncryption {
	return awsstate.ObjectEncryption{Mode: awsstate.EncryptionAES256}
}

func resolveStateEndpoint(spec Spec) (string, error) {
	if ep := strings.TrimSpace(spec.Dependencies.StateEndpoint); ep != "" {
		return awsendpoint.Parse(ep)
	}
	return awsendpoint.FromEnv()
}

func resolveStateBucket(spec Spec) (string, error) {
	bucket := strings.TrimSpace(spec.Dependencies.StateBucket)
	if bucket == "" {
		return "", errors.New("Scaleway DIY state requires target.scaleway.stateBucket")
	}
	return bucket, nil
}

func resolveStateRegion(spec Spec) string {
	if region := strings.TrimSpace(spec.Dependencies.StateRegion); region != "" {
		return region
	}
	if region := strings.TrimSpace(spec.Identity.Region); region != "" {
		return region
	}
	return "fr-par"
}

func stateClients(ctx context.Context, planned platform.PlannedStack) (*awsstate.Manager, *awsstate.Archive, string, error) {
	scwPlanned, ok := planned.(Planned)
	if !ok {
		return nil, nil, "", fmt.Errorf("Scaleway state received unexpected planned type %T", planned)
	}
	spec := scwPlanned.Spec
	bucket, err := resolveStateBucket(spec)
	if err != nil {
		return nil, nil, "", err
	}
	endpoint, err := resolveStateEndpoint(spec)
	if err != nil {
		return nil, nil, "", err
	}
	encryption := diyObjectEncryption()
	region := resolveStateRegion(spec)
	manager, err := awsstate.NewAWSWithEndpoint(ctx, region, bucket, spec.Identity.Project, spec.Identity.Environment, encryption, endpoint)
	if err != nil {
		return nil, nil, "", err
	}
	archive, err := awsstate.NewAWSArchiveWithEndpoint(ctx, region, bucket, encryption, endpoint)
	if err != nil {
		return nil, nil, "", err
	}
	return manager, archive, bucket, nil
}

func (State) Status(ctx context.Context, planned platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	manager, _, bucket, err := stateClients(ctx, planned)
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
	meta := toLockInfo(info)
	return true, &meta, bucket, nil
}

func (State) Lock(ctx context.Context, planned platform.PlannedStack, owner string) (func(context.Context) error, error) {
	manager, _, _, err := stateClients(ctx, planned)
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
	manager, _, _, err := stateClients(ctx, planned)
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
	meta := toLockInfo(info)
	return &meta, nil
}

func (State) Backup(ctx context.Context, planned platform.PlannedStack) (platform.BackupResult, error) {
	_, archive, _, err := stateClients(ctx, planned)
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
	_, archive, _, err := stateClients(ctx, planned)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	result, err := archive.Restore(ctx, location)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	return platform.RestoreResult{ID: result.ID, Location: result.Prefix}, nil
}

func toLockInfo(info awsstate.Info) platform.LockInfo {
	return platform.LockInfo{
		Project: info.Project, Environment: info.Environment,
		Owner: info.Owner, AcquiredAt: info.AcquiredAt,
	}
}
