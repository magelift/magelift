// Package bootstrap prepares the GCS DIY Pulumi backend and GitHub WIF identity
// for a Magento environment on GCP.
package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

var (
	componentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
	projectPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	regionPattern    = regexp.MustCompile(`^[a-z]+-[a-z]+[0-9]+$`)
)

const highAvailabilityEnvironment = "high-availability"

const stateSoftDeleteRetention = 7 * 24 * time.Hour

type Spec struct {
	Project     string // Magento project
	Environment string
	GCPProject  string
	Region      string
}

type Plan struct {
	StateBucket string            `json:"stateBucket" yaml:"stateBucket"`
	GCPProject  string            `json:"gcpProject" yaml:"gcpProject"`
	Region      string            `json:"region" yaml:"region"`
	Labels      map[string]string `json:"labels" yaml:"labels"`
}

type Result struct {
	Plan Plan `json:"plan" yaml:"plan"`
}

func BuildPlan(spec Spec) (Plan, error) {
	if !componentPattern.MatchString(spec.Project) {
		return Plan{}, errors.New("bootstrap project must be a lowercase stable name")
	}
	if !componentPattern.MatchString(spec.Environment) {
		return Plan{}, errors.New("bootstrap environment must be a lowercase stable name")
	}
	if !projectPattern.MatchString(spec.GCPProject) {
		return Plan{}, errors.New("bootstrap GCP project ID is invalid")
	}
	if !regionPattern.MatchString(spec.Region) {
		return Plan{}, errors.New("bootstrap region is invalid")
	}
	stateEnvironment := stateBucketEnvironment(spec.Environment)
	bucket := strings.Join([]string{
		"magelift",
		spec.GCPProject,
		spec.Region,
		spec.Project,
		stateEnvironment,
		"state",
	}, "-")
	return Plan{
		StateBucket: compactStateBucketName(bucket, stateEnvironment),
		GCPProject:  spec.GCPProject,
		Region:      spec.Region,
		Labels: map[string]string{
			"magelift-managed-by":  "magelift",
			"magelift-project":     spec.Project,
			"magelift-environment": spec.Environment,
			"magelift-purpose":     "pulumi-state",
		},
	}, nil
}

const maxStateBucketNameLength = 63

func compactStateBucketName(bucket, stateEnvironment string) string {
	if len(bucket) <= maxStateBucketNameLength {
		return bucket
	}

	digest := sha256.Sum256([]byte(bucket))
	suffix := hex.EncodeToString(digest[:])[:10]
	tail := stateEnvironment + "-state"
	prefixLength := maxStateBucketNameLength - len(suffix) - len(tail) - 2
	prefix := strings.TrimRight(bucket[:prefixLength], "-")
	return prefix + "-" + suffix + "-" + tail
}

func stateBucketEnvironment(environment string) string {
	if environment == highAvailabilityEnvironment {
		// GCS bucket names are capped at 63 characters. Keep the public
		// environment label intact while shortening only this physical name.
		return "ha"
	}
	return environment
}

// BucketMetadata records the GCS fields bootstrap uses to check state-bucket ownership.
type BucketMetadata struct {
	Location string
	Labels   map[string]string
}

type BucketAPI interface {
	BucketExists(ctx context.Context, name string) (bool, BucketMetadata, error)
	CreateBucket(ctx context.Context, name, project, location string, labels map[string]string) error
	EnsureVersioning(ctx context.Context, name string) error
	EnsureSoftDelete(ctx context.Context, name string, retention time.Duration) error
}

var stateBucketOwnershipLabelKeys = []string{
	"magelift-managed-by",
	"magelift-project",
	"magelift-environment",
	"magelift-purpose",
}

const stateBucketOwnershipRefusalHint = "select a fresh MageLift project/environment name, or have the bucket owner review its recorded ownership and region"

func validateExistingStateBucket(plan Plan, meta BucketMetadata) error {
	for _, key := range stateBucketOwnershipLabelKeys {
		want := plan.Labels[key]
		got := strings.TrimSpace(meta.Labels[key])
		if got == "" {
			return fmt.Errorf(
				"refusing to mutate GCS state bucket %q: missing ownership label %q; %s",
				plan.StateBucket, key, stateBucketOwnershipRefusalHint,
			)
		}
		if got != want {
			return fmt.Errorf(
				"refusing to mutate GCS state bucket %q: label %q is %q, want %q; %s",
				plan.StateBucket, key, got, want, stateBucketOwnershipRefusalHint,
			)
		}
	}
	if !stateBucketLocationsMatch(plan.Region, meta.Location) {
		return fmt.Errorf(
			"refusing to mutate GCS state bucket %q: location is %q, want %q; %s",
			plan.StateBucket, strings.TrimSpace(meta.Location), plan.Region, stateBucketOwnershipRefusalHint,
		)
	}
	return nil
}

func stateBucketLocationsMatch(wantRegion, bucketLocation string) bool {
	want := strings.TrimSpace(wantRegion)
	got := strings.TrimSpace(bucketLocation)
	if want == "" || got == "" {
		return false
	}
	return strings.EqualFold(want, got)
}

type Bootstrapper struct {
	buckets BucketAPI
}

func NewFromClient(buckets BucketAPI) (*Bootstrapper, error) {
	if buckets == nil {
		return nil, errors.New("GCS bucket client is required")
	}
	return &Bootstrapper{buckets: buckets}, nil
}

func New(ctx context.Context) (*Bootstrapper, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return NewFromClient(gcsBuckets{client: client})
}

func VerifyAccount(ctx context.Context, gcpProject string) error {
	if !projectPattern.MatchString(gcpProject) {
		return errors.New("GCP project ID is invalid")
	}
	client, err := NewWIFClient(ctx)
	if err != nil {
		return fmt.Errorf("verify GCP account: %w", err)
	}
	_, err = client.ProjectNumber(ctx, gcpProject)
	if err != nil {
		return fmt.Errorf("verify GCP project %q access: %w", gcpProject, err)
	}
	return nil
}

func (b *Bootstrapper) Ensure(ctx context.Context, plan Plan) (Result, error) {
	if b == nil || b.buckets == nil {
		return Result{}, errors.New("GCP bootstrapper is required")
	}
	exists, meta, err := b.buckets.BucketExists(ctx, plan.StateBucket)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		if err := b.buckets.CreateBucket(ctx, plan.StateBucket, plan.GCPProject, plan.Region, plan.Labels); err != nil {
			return Result{}, err
		}
	} else if err := validateExistingStateBucket(plan, meta); err != nil {
		return Result{}, err
	}
	if err := b.buckets.EnsureVersioning(ctx, plan.StateBucket); err != nil {
		return Result{}, err
	}
	if err := b.buckets.EnsureSoftDelete(ctx, plan.StateBucket, stateSoftDeleteRetention); err != nil {
		return Result{}, err
	}
	return Result{Plan: plan}, nil
}

type gcsBuckets struct {
	client *storage.Client
}

func (g gcsBuckets) BucketExists(ctx context.Context, name string) (bool, BucketMetadata, error) {
	attrs, err := g.client.Bucket(name).Attrs(ctx)
	if err == nil {
		return true, bucketMetadataFromAttrs(attrs), nil
	}
	if errors.Is(err, storage.ErrBucketNotExist) {
		return false, BucketMetadata{}, nil
	}
	return false, BucketMetadata{}, fmt.Errorf("head GCS bucket: %w", err)
}

func bucketMetadataFromAttrs(attrs *storage.BucketAttrs) BucketMetadata {
	if attrs == nil {
		return BucketMetadata{}
	}
	labels := make(map[string]string, len(attrs.Labels))
	for key, value := range attrs.Labels {
		labels[key] = value
	}
	return BucketMetadata{
		Location: attrs.Location,
		Labels:   labels,
	}
}

func (g gcsBuckets) CreateBucket(ctx context.Context, name, project, location string, labels map[string]string) error {
	attrs := &storage.BucketAttrs{
		Location:                 location,
		Labels:                   labels,
		UniformBucketLevelAccess: storage.UniformBucketLevelAccess{Enabled: true},
		PublicAccessPrevention:   storage.PublicAccessPreventionEnforced,
	}
	if err := g.client.Bucket(name).Create(ctx, project, attrs); err != nil {
		return fmt.Errorf("create GCS state bucket: %w", err)
	}
	return nil
}

func (g gcsBuckets) EnsureVersioning(ctx context.Context, name string) error {
	_, err := g.client.Bucket(name).Update(ctx, storage.BucketAttrsToUpdate{
		VersioningEnabled: true,
	})
	if err != nil {
		return fmt.Errorf("enable GCS versioning: %w", err)
	}
	return nil
}

func (g gcsBuckets) EnsureSoftDelete(ctx context.Context, name string, retention time.Duration) error {
	if retention <= 0 {
		return errors.New("GCS state soft-delete retention must be positive")
	}
	_, err := g.client.Bucket(name).Update(ctx, storage.BucketAttrsToUpdate{
		SoftDeletePolicy: &storage.SoftDeletePolicy{RetentionDuration: retention},
	})
	if err != nil {
		return fmt.Errorf("enable GCS soft delete: %w", err)
	}
	return nil
}

// BackendURL returns the Pulumi DIY gs:// URL for a plan.
func BackendURL(plan Plan) string {
	return "gs://" + plan.StateBucket
}

func StateBackendURL(spec Spec) (string, error) {
	plan, err := BuildPlan(spec)
	if err != nil {
		return "", err
	}
	return BackendURL(plan), nil
}
