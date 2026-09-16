package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"google.golang.org/api/googleapi"
)

// ErrStateBucketMissing is returned when locking cannot proceed because
// the DIY GCS state bucket was never bootstrapped.
var ErrStateBucketMissing = errors.New("deployment state bucket is missing")

var newGCSState = gcpstate.NewGCS

// acquireDeploymentLock locks the environment's GCS state bucket. The owner
// is supplied by the caller (the CLI builds the magelift-cli-host-pid form).
func acquireDeploymentLock(ctx context.Context, spec gcpstack.Spec, owner string) (func(context.Context) error, error) {
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
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("lock owner is required")
	}
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
