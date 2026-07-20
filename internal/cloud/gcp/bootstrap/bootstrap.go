// Package bootstrap prepares the GCS DIY Pulumi backend for a Magento environment.
// GitHub WIF identity is deferred (experimental); Ensure creates the state bucket only.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

var (
	componentPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,30}$`)
	projectPattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)
	regionPattern    = regexp.MustCompile(`^[a-z]+-[a-z]+[0-9]+$`)
)

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
	bucket := strings.Join([]string{"magelift", spec.GCPProject, spec.Region, spec.Project, spec.Environment, "state"}, "-")
	if len(bucket) > 63 {
		return Plan{}, errors.New("generated state bucket name exceeds 63 characters")
	}
	return Plan{
		StateBucket: bucket,
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

type BucketAPI interface {
	BucketExists(ctx context.Context, name string) (bool, error)
	CreateBucket(ctx context.Context, name, project, location string, labels map[string]string) error
	EnsureVersioning(ctx context.Context, name string) error
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
	client, err := storage.NewClient(ctx, option.WithScopes(storage.ScopeReadOnly))
	if err != nil {
		return fmt.Errorf("verify GCP account: %w", err)
	}
	defer client.Close()
	it := client.Buckets(ctx, gcpProject)
	_, err = it.Next()
	if err != nil && !errors.Is(err, iterator.Done) {
		return fmt.Errorf("verify GCP project %q access: %w", gcpProject, err)
	}
	return nil
}

func (b *Bootstrapper) Ensure(ctx context.Context, plan Plan) (Result, error) {
	if b == nil || b.buckets == nil {
		return Result{}, errors.New("GCP bootstrapper is required")
	}
	exists, err := b.buckets.BucketExists(ctx, plan.StateBucket)
	if err != nil {
		return Result{}, err
	}
	if !exists {
		if err := b.buckets.CreateBucket(ctx, plan.StateBucket, plan.GCPProject, plan.Region, plan.Labels); err != nil {
			return Result{}, err
		}
	}
	if err := b.buckets.EnsureVersioning(ctx, plan.StateBucket); err != nil {
		return Result{}, err
	}
	return Result{Plan: plan}, nil
}

type gcsBuckets struct {
	client *storage.Client
}

func (g gcsBuckets) BucketExists(ctx context.Context, name string) (bool, error) {
	_, err := g.client.Bucket(name).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, storage.ErrBucketNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("head GCS bucket: %w", err)
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

// BackendURL returns the Pulumi DIY gs:// URL for a plan.
func BackendURL(plan Plan) string {
	return "gs://" + plan.StateBucket
}
