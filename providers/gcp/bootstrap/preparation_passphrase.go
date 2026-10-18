package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const preparationLockReleaseTimeout = 90 * time.Second

// StateSecretAPI manages the immutable version-1 state passphrase secret.
type StateSecretAPI interface {
	GetSecret(ctx context.Context, project, secretID string) (SecretMetadata, error)
	CreateSecret(ctx context.Context, project, secretID string, labels map[string]string) error
	GetVersion(ctx context.Context, project, secretID string, version string) (SecretVersionMetadata, error)
	ListVersions(ctx context.Context, project, secretID string) ([]SecretVersionMetadata, error)
	AddVersion(ctx context.Context, project, secretID string, payload []byte) error
	Close() error
}

// SecretMetadata records Secret Manager ownership labels.
type SecretMetadata struct {
	Name   string
	Labels map[string]string
}

// SecretVersionMetadata records version state without secret bytes.
type SecretVersionMetadata struct {
	Name  string
	State string
}

// StateCheckpointAPI reports whether the state bucket already holds Pulumi data.
type StateCheckpointAPI interface {
	HasPriorCheckpointData(ctx context.Context, bucket, project, environment string) (bool, error)
	Close() error
}

// StateLockAPI acquires the deployment lock used for passphrase initialization.
type StateLockAPI interface {
	Acquire(ctx context.Context, bucket, project, environment, owner string) (func(context.Context) error, error)
	Close() error
}

// NewStateSecretClient builds a live Secret Manager client for preparation.
func NewStateSecretClient(ctx context.Context, opts ...option.ClientOption) (StateSecretAPI, error) {
	client, err := secretmanager.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("create Secret Manager client: %w", err)
	}
	return &sdkStateSecret{client: client}, nil
}

type sdkStateSecret struct {
	client *secretmanager.Client
}

func (c *sdkStateSecret) Close() error {
	if c == nil || c.client == nil {
		return nil
	}
	return c.client.Close()
}

func (c *sdkStateSecret) GetSecret(ctx context.Context, project, secretID string) (SecretMetadata, error) {
	name := preparationSecretResourceName(project, secretID)
	secret, err := c.client.GetSecret(ctx, &secretmanagerpb.GetSecretRequest{Name: name})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return SecretMetadata{}, ErrPreparationNotFound
		}
		return SecretMetadata{}, fmt.Errorf("get secret %q: %w", secretID, err)
	}
	return SecretMetadata{Name: secret.Name, Labels: copyStringMap(secret.Labels)}, nil
}

func (c *sdkStateSecret) CreateSecret(ctx context.Context, project, secretID string, labels map[string]string) error {
	_, err := c.client.CreateSecret(ctx, &secretmanagerpb.CreateSecretRequest{
		Parent:   "projects/" + project,
		SecretId: secretID,
		Secret: &secretmanagerpb.Secret{
			Labels: copyStringMap(labels),
			Replication: &secretmanagerpb.Replication{
				Replication: &secretmanagerpb.Replication_Automatic_{
					Automatic: &secretmanagerpb.Replication_Automatic{},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("create secret %q: %w", secretID, err)
	}
	return nil
}

func (c *sdkStateSecret) GetVersion(ctx context.Context, project, secretID, version string) (SecretVersionMetadata, error) {
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/%s", project, secretID, version)
	resp, err := c.client.GetSecretVersion(ctx, &secretmanagerpb.GetSecretVersionRequest{Name: name})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return SecretVersionMetadata{}, ErrPreparationNotFound
		}
		return SecretVersionMetadata{}, fmt.Errorf("get secret version %q: %w", version, err)
	}
	return SecretVersionMetadata{Name: resp.Name, State: resp.State.String()}, nil
}

func (c *sdkStateSecret) ListVersions(ctx context.Context, project, secretID string) ([]SecretVersionMetadata, error) {
	parent := preparationSecretResourceName(project, secretID)
	it := c.client.ListSecretVersions(ctx, &secretmanagerpb.ListSecretVersionsRequest{Parent: parent})
	out := make([]SecretVersionMetadata, 0)
	for {
		version, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("list secret versions for %q: %w", secretID, err)
		}
		out = append(out, SecretVersionMetadata{Name: version.Name, State: version.State.String()})
	}
	return out, nil
}

func (c *sdkStateSecret) AddVersion(ctx context.Context, project, secretID string, payload []byte) error {
	_, err := c.client.AddSecretVersion(ctx, &secretmanagerpb.AddSecretVersionRequest{
		Parent:  preparationSecretResourceName(project, secretID),
		Payload: &secretmanagerpb.SecretPayload{Data: payload},
	})
	if err != nil {
		return errAddSecretVersionStage
	}
	return nil
}

var errAddSecretVersionStage = errors.New("state passphrase secret version create failed at add-version stage")

// NewGCSCheckpointReader lists live GCS objects to detect prior Pulumi state.
func NewGCSCheckpointReader(ctx context.Context, opts ...option.ClientOption) (StateCheckpointAPI, error) {
	return newGCSCheckpointReader(ctx, opts...)
}

// NewGCSStateLockFactory returns a lock factory backed by gcpstate.Manager.
func NewGCSStateLockFactory(ctx context.Context, opts ...option.ClientOption) (StateLockAPI, error) {
	return newGCSStateLockFactory(ctx, opts...)
}

// EnsureStatePassphrase creates or reuses the immutable version-1 passphrase reference.
func (p *AccountPreparer) EnsureStatePassphrase(ctx context.Context, plan PreparationPlan, spec Spec, lockOwner string) (ref StatePassphraseRef, err error) {
	if p == nil || p.secrets == nil || p.checkpoints == nil || p.locks == nil {
		return StatePassphraseRef{}, errors.New("account preparer is required")
	}
	if err := ctx.Err(); err != nil {
		return StatePassphraseRef{}, err
	}
	if err := validatePreparationPlanScope(plan, spec); err != nil {
		return StatePassphraseRef{}, err
	}
	lockOwner = strings.TrimSpace(lockOwner)
	if lockOwner == "" {
		return StatePassphraseRef{}, errors.New("state passphrase lock owner is required")
	}
	project := strings.TrimSpace(plan.Base.GCPProject)
	secretID := strings.TrimSpace(plan.StateSecretID)
	bucket := strings.TrimSpace(plan.Base.StateBucket)
	if project == "" || secretID == "" || bucket == "" {
		return StatePassphraseRef{}, errors.New("preparation plan state secret fields are incomplete")
	}
	if !componentPattern.MatchString(spec.Project) || !componentPattern.MatchString(spec.Environment) {
		return StatePassphraseRef{}, errors.New("preparation spec project and environment are required")
	}
	release, err := p.locks.Acquire(ctx, bucket, spec.Project, spec.Environment, lockOwner)
	if err != nil {
		return StatePassphraseRef{}, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), preparationLockReleaseTimeout)
		defer cancel()
		if releaseErr := release(releaseCtx); releaseErr != nil {
			err = errors.Join(err, fmt.Errorf("release state passphrase lock: %w", releaseErr))
		}
	}()

	resource := preparationSecretResourceName(project, secretID)
	versionName := preparationSecretVersionName(project, secretID)
	meta, secretErr := p.secrets.GetSecret(ctx, project, secretID)
	switch {
	case secretErr == nil:
		if err := validatePreparationOwnership(resource, plan.StateSecretLabels, meta.Labels); err != nil {
			return StatePassphraseRef{}, err
		}
		version, err := p.secrets.GetVersion(ctx, project, secretID, "1")
		if err == nil {
			if !strings.EqualFold(strings.TrimSpace(version.State), "ENABLED") {
				return StatePassphraseRef{}, fmt.Errorf(
					"refusing to use state passphrase secret %q: version 1 is %s; restore or rotate outside MageLift",
					secretID, strings.TrimSpace(version.State),
				)
			}
			return StatePassphraseRef{SecretID: secretID, VersionName: versionName}, nil
		}
		if !errors.Is(err, ErrPreparationNotFound) {
			return StatePassphraseRef{}, err
		}
		versions, err := p.secrets.ListVersions(ctx, project, secretID)
		if err != nil {
			return StatePassphraseRef{}, err
		}
		if len(versions) > 0 {
			return StatePassphraseRef{}, fmt.Errorf(
				"refusing to initialize state passphrase secret %q: versions exist but version 1 is missing or unavailable",
				secretID,
			)
		}
		prior, err := p.checkpoints.HasPriorCheckpointData(ctx, bucket, spec.Project, spec.Environment)
		if err != nil {
			return StatePassphraseRef{}, err
		}
		if prior {
			return StatePassphraseRef{}, fmt.Errorf(
				"refusing to create state passphrase secret %q: encrypted Pulumi state already exists in %q; restore the original state passphrase or import the existing secret",
				secretID, bucket,
			)
		}
		if err := initializeStatePassphraseVersion(ctx, p.secrets, project, secretID); err != nil {
			return StatePassphraseRef{}, err
		}
		return StatePassphraseRef{SecretID: secretID, VersionName: versionName}, nil
	case errors.Is(secretErr, ErrPreparationNotFound):
		prior, err := p.checkpoints.HasPriorCheckpointData(ctx, bucket, spec.Project, spec.Environment)
		if err != nil {
			return StatePassphraseRef{}, err
		}
		if prior {
			return StatePassphraseRef{}, fmt.Errorf(
				"refusing to create state passphrase secret %q: encrypted Pulumi state already exists in %q; restore the original state passphrase or import the existing secret",
				secretID, bucket,
			)
		}
		if err := p.secrets.CreateSecret(ctx, project, secretID, plan.StateSecretLabels); err != nil {
			return StatePassphraseRef{}, err
		}
		if err := initializeStatePassphraseVersion(ctx, p.secrets, project, secretID); err != nil {
			// Interrupted create with no versions can resume on rerun after inspecting version 1.
			version, inspectErr := p.secrets.GetVersion(ctx, project, secretID, "1")
			if inspectErr == nil && strings.EqualFold(strings.TrimSpace(version.State), "ENABLED") {
				return StatePassphraseRef{SecretID: secretID, VersionName: versionName}, nil
			}
			return StatePassphraseRef{}, err
		}
		return StatePassphraseRef{SecretID: secretID, VersionName: versionName}, nil
	default:
		return StatePassphraseRef{}, secretErr
	}
}

func initializeStatePassphraseVersion(ctx context.Context, secrets StateSecretAPI, project, secretID string) error {
	version, err := secrets.GetVersion(ctx, project, secretID, "1")
	if err == nil {
		if strings.EqualFold(strings.TrimSpace(version.State), "ENABLED") {
			return nil
		}
		return fmt.Errorf(
			"refusing to initialize state passphrase secret %q: version 1 is %s",
			secretID, strings.TrimSpace(version.State),
		)
	}
	if !errors.Is(err, ErrPreparationNotFound) {
		return err
	}
	payload, err := randomPassphrasePayload()
	if err != nil {
		return err
	}
	if err := secrets.AddVersion(ctx, project, secretID, payload); err != nil {
		version, inspectErr := secrets.GetVersion(ctx, project, secretID, "1")
		if inspectErr == nil && strings.EqualFold(strings.TrimSpace(version.State), "ENABLED") {
			return nil
		}
		return err
	}
	return nil
}

func randomPassphrasePayload() ([]byte, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return nil, fmt.Errorf("generate state passphrase: %w", err)
	}
	return []byte(hex.EncodeToString(raw[:])), nil
}
