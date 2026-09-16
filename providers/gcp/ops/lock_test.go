package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
)

func TestAcquireDeploymentLockFailsClosedOnGCSClientError(t *testing.T) {
	t.Cleanup(func() { newGCSState = gcpstate.NewGCS })
	newGCSState = func(context.Context, string, string, string) (*gcpstate.Manager, error) {
		return nil, errors.New("authentication failed")
	}

	release, err := acquireDeploymentLock(context.Background(), gcpTestSpec(), "test-owner")
	if err == nil {
		t.Fatal("expected GCS client construction to fail closed")
	}
	if release != nil {
		t.Fatal("GCS construct failure must not return a no-op unlock")
	}
	if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestAcquireDeploymentLockMissingBucketIsBootstrapError(t *testing.T) {
	t.Cleanup(func() { newGCSState = gcpstate.NewGCS })
	newGCSState = func(_ context.Context, bucket, project, environment string) (*gcpstate.Manager, error) {
		return gcpstate.NewManager(&missingBucketObjects{}, bucket, project, environment)
	}

	release, err := acquireDeploymentLock(context.Background(), gcpTestSpec(), "test-owner")
	if release != nil {
		t.Fatal("missing bootstrap bucket must not return a no-op unlock")
	}
	if !errors.Is(err, ErrStateBucketMissing) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "magelift bootstrap") {
		t.Fatalf("bootstrap guidance missing: %v", err)
	}
}

type missingBucketObjects struct{}

func (missingBucketObjects) Get(context.Context, string, string) ([]byte, string, error) {
	return nil, "", storage.ErrBucketNotExist
}
func (missingBucketObjects) PutIfAbsent(context.Context, string, string, []byte) (string, error) {
	return "", storage.ErrBucketNotExist
}
func (missingBucketObjects) Put(context.Context, string, string, []byte) (string, error) {
	return "", storage.ErrBucketNotExist
}
func (missingBucketObjects) Delete(context.Context, string, string) error {
	return storage.ErrBucketNotExist
}
func (missingBucketObjects) DeleteGeneration(context.Context, string, string, string) error {
	return storage.ErrBucketNotExist
}
func (missingBucketObjects) Copy(context.Context, string, string, string, string) error {
	return storage.ErrBucketNotExist
}

func TestAcquireDeploymentLockRequiresOwner(t *testing.T) {
	if _, err := acquireDeploymentLock(context.Background(), gcpTestSpec(), ""); err == nil {
		t.Fatal("empty lock owner was accepted")
	}
}
