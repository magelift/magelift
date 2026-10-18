package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	gcpbootstrap "github.com/magelift/magelift/providers/gcp/bootstrap"
	gcpsecrets "github.com/magelift/magelift/providers/gcp/secrets"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
)

type readManagedStatePassphraseFunc func(context.Context, string) ([]byte, error)

func explicitPulumiPassphraseConfigured() bool {
	if strings.TrimSpace(os.Getenv("PULUMI_CONFIG_PASSPHRASE")) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("PULUMI_CONFIG_PASSPHRASE_FILE")) != ""
}

func pulumiStatePassphraseVersionRef(spec gcpstack.Spec) (string, error) {
	plan, err := gcpbootstrap.BuildPreparationPlan(gcpbootstrap.Spec{
		Project:     spec.Identity.Project,
		Environment: spec.Identity.Environment,
		GCPProject:  spec.Identity.GCPProject,
		Region:      spec.Identity.Region,
	})
	if err != nil {
		return "", err
	}
	project := strings.TrimSpace(plan.Base.GCPProject)
	secretID := strings.TrimSpace(plan.StateSecretID)
	if project == "" || secretID == "" {
		return "", errors.New("preparation plan state secret fields are incomplete")
	}
	return fmt.Sprintf("projects/%s/secrets/%s/versions/1", project, secretID), nil
}

func pulumiPassphraseEnvForStack(ctx context.Context, spec gcpstack.Spec) (map[string]string, error) {
	return managedPulumiPassphraseEnv(ctx, spec, defaultReadManagedStatePassphrase)
}

func managedPulumiPassphraseEnv(
	ctx context.Context,
	spec gcpstack.Spec,
	read readManagedStatePassphraseFunc,
) (map[string]string, error) {
	if ctx == nil {
		return nil, errors.New("context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if explicitPulumiPassphraseConfigured() {
		return nil, nil
	}
	versionRef, err := pulumiStatePassphraseVersionRef(spec)
	if err != nil {
		return nil, err
	}
	value, err := read(ctx, versionRef)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errManagedPassphraseUnavailable(versionRef)
	}
	passphrase := strings.TrimSpace(string(value))
	if passphrase == "" {
		return nil, errManagedPassphraseUnavailable(versionRef)
	}
	return map[string]string{"PULUMI_CONFIG_PASSPHRASE": passphrase}, nil
}

func defaultReadManagedStatePassphrase(ctx context.Context, versionRef string) ([]byte, error) {
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return nil, errManagedPassphraseUnavailable(versionRef)
	}
	defer store.Close()
	value, err := store.GetSecretValue(ctx, versionRef)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.Canceled) {
			return nil, context.Canceled
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, context.DeadlineExceeded
		}
		return nil, errManagedPassphraseUnavailable(versionRef)
	}
	if len(bytes.TrimSpace(value)) == 0 {
		return nil, errManagedPassphraseUnavailable(versionRef)
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out, nil
}

func errManagedPassphraseUnavailable(versionRef string) error {
	return fmt.Errorf(
		"pulumi state passphrase is not available in Secret Manager at %s; run MageLift GCP account preparation or set PULUMI_CONFIG_PASSPHRASE / PULUMI_CONFIG_PASSPHRASE_FILE for recovery",
		versionRef,
	)
}
