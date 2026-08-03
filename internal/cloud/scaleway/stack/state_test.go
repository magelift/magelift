package stack

import (
	"context"
	"errors"
	"testing"

	awsstate "github.com/magelift/magelift/internal/cloud/aws/state"
	"github.com/magelift/magelift/internal/platform"
)

func TestModuleStateReturnsConcreteAdapter(t *testing.T) {
	m := Module{}
	got := m.State()
	if got == nil {
		t.Fatal("State() returned nil")
	}
	if _, ok := got.(State); !ok {
		t.Fatalf("State() type = %T, want stack.State", got)
	}
}

func TestDIYStateUsesAES256Encryption(t *testing.T) {
	enc := diyObjectEncryption()
	if enc.Mode != awsstate.EncryptionAES256 {
		t.Fatalf("encryption mode = %v, want EncryptionAES256", enc.Mode)
	}
	if enc.KMSKeyARN != "" {
		t.Fatalf("AES256 must not set KMS ARN, got %q", enc.KMSKeyARN)
	}
}

func TestResolveStateEndpointUsesConfigOrEnvAndRejectsNonLoopback(t *testing.T) {
	t.Setenv("MAGELIFT_AWS_ENDPOINT_URL", "")
	ep, err := resolveStateEndpoint(Spec{Dependencies: Dependencies{StateEndpoint: "http://127.0.0.1:4566"}})
	if err != nil {
		t.Fatal(err)
	}
	if ep != "http://127.0.0.1:4566" {
		t.Fatalf("endpoint = %q", ep)
	}
	if _, err := resolveStateEndpoint(Spec{Dependencies: Dependencies{StateEndpoint: "https://evil.example"}}); err == nil {
		t.Fatal("non-loopback endpoint accepted")
	}
	t.Setenv("MAGELIFT_AWS_ENDPOINT_URL", "http://localhost:4566")
	ep, err = resolveStateEndpoint(Spec{})
	if err != nil {
		t.Fatal(err)
	}
	if ep != "http://localhost:4566" {
		t.Fatalf("FromEnv endpoint = %q", ep)
	}
}

func TestResolveStateBucketRequiresConfiguredBucket(t *testing.T) {
	if _, err := resolveStateBucket(Spec{}); err == nil {
		t.Fatal("empty state bucket accepted")
	}
	bucket, err := resolveStateBucket(Spec{Dependencies: Dependencies{StateBucket: "magelift-shop-staging-state"}})
	if err != nil {
		t.Fatal(err)
	}
	if bucket != "magelift-shop-staging-state" {
		t.Fatalf("bucket = %q", bucket)
	}
}

func TestStateLockRequiresConfiguredBucket(t *testing.T) {
	var s State
	_, err := s.Lock(context.Background(), Planned{Spec: Spec{
		Identity: Identity{Project: "shop", Environment: "staging", Region: "GRA9"},
	}}, "owner")
	if err == nil {
		t.Fatal("Lock succeeded without state bucket")
	}
	if errors.Is(err, platform.ErrNotSupported) {
		t.Fatal("Lock must not return ErrNotSupported once State is wired")
	}
}
