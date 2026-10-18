package plugin

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpsecrets "github.com/magelift/magelift/providers/gcp/secrets"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
)

const syntheticPassphraseCanary = "synthetic-canary-passphrase-bytes"

func expectedStatePassphraseVersionRef(spec gcpstack.Spec) string {
	plan, err := gcpbootstrap.BuildPreparationPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("projects/%s/secrets/%s/versions/1", plan.Base.GCPProject, plan.StateSecretID)
}

func TestPulumiStatePassphraseVersionRefUsesImmutableVersionOne(t *testing.T) {
	t.Parallel()
	spec := testSpec()
	ref, err := pulumiStatePassphraseVersionRef(spec)
	if err != nil {
		t.Fatal(err)
	}
	if ref != expectedStatePassphraseVersionRef(spec) || !strings.HasSuffix(ref, "/versions/1") {
		t.Fatalf("ref=%q", ref)
	}
}

func TestPulumiPassphraseEnvForStackExplicitOverrideSkipsManagedRead(t *testing.T) {
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "legacy-passphrase")
	env, err := pulumiPassphraseEnvForStack(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	if env != nil {
		t.Fatalf("env=%#v, want nil", env)
	}
}

func TestManagedPulumiPassphraseEnvFromReader(t *testing.T) {
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE_FILE", "")
	spec := testSpec()
	versionRef := expectedStatePassphraseVersionRef(spec)
	env, err := managedPulumiPassphraseEnv(context.Background(), spec, func(_ context.Context, name string) ([]byte, error) {
		if name != versionRef {
			t.Fatalf("name=%q", name)
		}
		return []byte("managed-key"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if env["PULUMI_CONFIG_PASSPHRASE"] != "managed-key" {
		t.Fatalf("env=%#v", env)
	}
}

func TestManagedPulumiPassphraseEnvRejectsEmptyPayload(t *testing.T) {
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE_FILE", "")
	_, err := managedPulumiPassphraseEnv(context.Background(), testSpec(), func(context.Context, string) ([]byte, error) {
		return []byte("   "), nil
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), syntheticPassphraseCanary) {
		t.Fatalf("error leaks canary: %v", err)
	}
}

func TestManagedPulumiPassphraseEnvSanitizesAPIFailure(t *testing.T) {
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE_FILE", "")
	spec := testSpec()
	versionRef := expectedStatePassphraseVersionRef(spec)
	_, err := managedPulumiPassphraseEnv(context.Background(), spec, func(context.Context, string) ([]byte, error) {
		return nil, fmt.Errorf("access denied: %s", syntheticPassphraseCanary)
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), syntheticPassphraseCanary) || strings.Contains(err.Error(), "access denied") {
		t.Fatalf("error=%v", err)
	}
	if !strings.Contains(err.Error(), versionRef) {
		t.Fatalf("error=%v", err)
	}
}

func TestManagedPulumiPassphraseEnvPropagatesCanceledContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := managedPulumiPassphraseEnv(ctx, testSpec(), func(callCtx context.Context, _ string) ([]byte, error) {
		return nil, callCtx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
}

func TestPulumiPassphraseEnvForStackHonorsPassphraseFile(t *testing.T) {
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE_FILE", "/tmp/passphrase")
	env, err := pulumiPassphraseEnvForStack(context.Background(), testSpec())
	if err != nil {
		t.Fatal(err)
	}
	if env != nil {
		t.Fatalf("env=%#v", env)
	}
}

func TestStoreCloseInvokesNativeCloser(t *testing.T) {
	t.Parallel()
	closed := false
	store := gcpsecrets.NewStoreFromClient(&closeTrackingAPI{closed: &closed})
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if !closed {
		t.Fatal("expected close")
	}
}

type closeTrackingAPI struct {
	closed *bool
}

func (c *closeTrackingAPI) List(context.Context, string) ([]gcpsecrets.Meta, error) { return nil, nil }
func (c *closeTrackingAPI) Set(context.Context, string, string, []byte) error       { return nil }
func (c *closeTrackingAPI) Remove(context.Context, string, string) error            { return nil }
func (c *closeTrackingAPI) GetSecretValue(context.Context, string) ([]byte, error) {
	return nil, errors.New("unused")
}
func (c *closeTrackingAPI) Close() error {
	*c.closed = true
	return nil
}

func TestManagedPassphraseCancellationDoesNotExposeJoinedAPIError(t *testing.T) {
	t.Setenv("PULUMI_CONFIG_PASSPHRASE", "")
	t.Setenv("PULUMI_CONFIG_PASSPHRASE_FILE", "")
	_, err := managedPulumiPassphraseEnv(context.Background(), testSpec(), func(context.Context, string) ([]byte, error) {
		return nil, errors.Join(context.Canceled, errors.New(syntheticPassphraseCanary))
	})
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), syntheticPassphraseCanary) {
		t.Fatal("cancellation diagnostic exposed the API error")
	}
}
