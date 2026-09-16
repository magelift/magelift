package ops

import (
	"context"
	"errors"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
)

func TestStateLockFailsClosedOnGCSClientError(t *testing.T) {
	state := State{NewManager: func(context.Context, string, string, string) (*gcpstate.Manager, error) {
		return nil, errors.New("authentication failed")
	}}
	if err := state.Lock(context.Background(), gcpTestSpec(), "test-owner"); err == nil {
		t.Fatal("expected GCS client construction to fail closed")
	} else if !strings.Contains(err.Error(), "authentication failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestStateLockMissingBucketIsBootstrapError(t *testing.T) {
	state := State{NewManager: func(_ context.Context, bucket, project, environment string) (*gcpstate.Manager, error) {
		return gcpstate.NewManager(&missingBucketObjects{}, bucket, project, environment)
	}}
	err := state.Lock(context.Background(), gcpTestSpec(), "test-owner")
	if !errors.Is(err, ErrStateBucketMissing) {
		t.Fatalf("error = %v", err)
	}
	if !strings.Contains(err.Error(), "magelift bootstrap") {
		t.Fatalf("bootstrap guidance missing: %v", err)
	}
}

func TestStateLockRequiresOwner(t *testing.T) {
	if err := (State{}).Lock(context.Background(), gcpTestSpec(), ""); err == nil {
		t.Fatal("empty lock owner was accepted")
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
