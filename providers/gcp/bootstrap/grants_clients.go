package bootstrap

import (
	"context"
	"fmt"

	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/option"
	secretmanager "google.golang.org/api/secretmanager/v1"
	storagev1 "google.golang.org/api/storage/v1"
)

// RepositoryIAMAPI reads and writes Artifact Registry repository IAM policies.
type RepositoryIAMAPI interface {
	GetRepositoryIAMPolicy(ctx context.Context, resource string) (*artifactregistry.Policy, error)
	SetRepositoryIAMPolicy(ctx context.Context, resource string, policy *artifactregistry.Policy) error
	Close() error
}

// SecretIAMAPI reads and writes Secret Manager secret IAM policies.
type SecretIAMAPI interface {
	GetSecretIAMPolicy(ctx context.Context, resource string) (*secretmanager.Policy, error)
	SetSecretIAMPolicy(ctx context.Context, resource string, policy *secretmanager.Policy) error
	Close() error
}

// BucketIAMAPI reads and writes GCS bucket IAM policies.
type BucketIAMAPI interface {
	GetBucketIAMPolicy(ctx context.Context, bucket string) (*storagev1.Policy, error)
	SetBucketIAMPolicy(ctx context.Context, bucket string, policy *storagev1.Policy) error
	Close() error
}

// ProjectIAMAPI reads and writes project IAM policies.
type ProjectIAMAPI interface {
	GetProjectIAMPolicy(ctx context.Context, projectID string) (*cloudresourcemanager.Policy, error)
	SetProjectIAMPolicy(ctx context.Context, projectID string, policy *cloudresourcemanager.Policy) error
	Close() error
}

type sdkRepositoryIAM struct {
	client *artifactregistry.Service
}

func NewRepositoryIAMClient(ctx context.Context, opts ...option.ClientOption) (RepositoryIAMAPI, error) {
	scopes := append([]option.ClientOption{option.WithScopes("https://www.googleapis.com/auth/cloud-platform")}, opts...)
	client, err := artifactregistry.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create Artifact Registry IAM client: %w", err)
	}
	return &sdkRepositoryIAM{client: client}, nil
}

func (c *sdkRepositoryIAM) Close() error { return nil }

func (c *sdkRepositoryIAM) GetRepositoryIAMPolicy(ctx context.Context, resource string) (*artifactregistry.Policy, error) {
	policy, err := c.client.Projects.Locations.Repositories.GetIamPolicy(resource).
		OptionsRequestedPolicyVersion(iamPolicyVersion3).
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get Artifact Registry IAM policy: %w", err)
	}
	return cloneARPolicy(policy), nil
}

func (c *sdkRepositoryIAM) SetRepositoryIAMPolicy(ctx context.Context, resource string, policy *artifactregistry.Policy) error {
	_, err := c.client.Projects.Locations.Repositories.SetIamPolicy(resource, &artifactregistry.SetIamPolicyRequest{
		Policy: cloneARPolicy(policy),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set Artifact Registry IAM policy: %w", err)
	}
	return nil
}

type sdkSecretIAM struct {
	client *secretmanager.Service
}

func NewSecretIAMClient(ctx context.Context, opts ...option.ClientOption) (SecretIAMAPI, error) {
	scopes := append([]option.ClientOption{option.WithScopes("https://www.googleapis.com/auth/cloud-platform")}, opts...)
	client, err := secretmanager.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create Secret Manager IAM client: %w", err)
	}
	return &sdkSecretIAM{client: client}, nil
}

func (c *sdkSecretIAM) Close() error { return nil }

func (c *sdkSecretIAM) GetSecretIAMPolicy(ctx context.Context, resource string) (*secretmanager.Policy, error) {
	policy, err := c.client.Projects.Secrets.GetIamPolicy(resource).
		OptionsRequestedPolicyVersion(iamPolicyVersion3).
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get Secret Manager IAM policy: %w", err)
	}
	return cloneSecretPolicy(policy), nil
}

func (c *sdkSecretIAM) SetSecretIAMPolicy(ctx context.Context, resource string, policy *secretmanager.Policy) error {
	_, err := c.client.Projects.Secrets.SetIamPolicy(resource, &secretmanager.SetIamPolicyRequest{
		Policy: cloneSecretPolicy(policy),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set Secret Manager IAM policy: %w", err)
	}
	return nil
}

type sdkBucketIAM struct {
	client *storagev1.Service
}

func NewBucketIAMClient(ctx context.Context, opts ...option.ClientOption) (BucketIAMAPI, error) {
	scopes := append([]option.ClientOption{option.WithScopes(storagev1.DevstorageFullControlScope)}, opts...)
	client, err := storagev1.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create GCS IAM client: %w", err)
	}
	return &sdkBucketIAM{client: client}, nil
}

func (c *sdkBucketIAM) Close() error { return nil }

func (c *sdkBucketIAM) GetBucketIAMPolicy(ctx context.Context, bucket string) (*storagev1.Policy, error) {
	policy, err := c.client.Buckets.GetIamPolicy(bucket).
		OptionsRequestedPolicyVersion(iamPolicyVersion3).
		Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get GCS bucket IAM policy: %w", err)
	}
	return cloneStoragePolicy(policy), nil
}

func (c *sdkBucketIAM) SetBucketIAMPolicy(ctx context.Context, bucket string, policy *storagev1.Policy) error {
	_, err := c.client.Buckets.SetIamPolicy(bucket, cloneStoragePolicy(policy)).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set GCS bucket IAM policy: %w", err)
	}
	return nil
}

type sdkProjectIAM struct {
	client *cloudresourcemanager.Service
}

func NewProjectIAMClient(ctx context.Context, opts ...option.ClientOption) (ProjectIAMAPI, error) {
	scopes := append([]option.ClientOption{option.WithScopes(cloudresourcemanager.CloudPlatformScope)}, opts...)
	client, err := cloudresourcemanager.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create Resource Manager IAM client: %w", err)
	}
	return &sdkProjectIAM{client: client}, nil
}

func (c *sdkProjectIAM) Close() error { return nil }

func (c *sdkProjectIAM) GetProjectIAMPolicy(ctx context.Context, projectID string) (*cloudresourcemanager.Policy, error) {
	// Resource Manager v1 adds projects/ in its generated request path.
	policy, err := c.client.Projects.GetIamPolicy(projectID, &cloudresourcemanager.GetIamPolicyRequest{
		Options: &cloudresourcemanager.GetPolicyOptions{RequestedPolicyVersion: iamPolicyVersion3},
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("get project IAM policy: %w", err)
	}
	return cloneCRMPolicy(policy), nil
}

func (c *sdkProjectIAM) SetProjectIAMPolicy(ctx context.Context, projectID string, policy *cloudresourcemanager.Policy) error {
	_, err := c.client.Projects.SetIamPolicy(projectID, &cloudresourcemanager.SetIamPolicyRequest{
		Policy: cloneCRMPolicy(policy),
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("set project IAM policy: %w", err)
	}
	return nil
}
