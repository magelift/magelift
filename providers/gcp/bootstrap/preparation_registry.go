package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// ArtifactRegistryAPI gets or creates an owned Docker repository.
type ArtifactRegistryAPI interface {
	GetRepository(ctx context.Context, project, location, repositoryID string) (RepositoryMetadata, error)
	CreateRepository(ctx context.Context, project, location, repositoryID string, labels map[string]string) (RepositoryMetadata, error)
	Close() error
}

// RepositoryMetadata is the subset of Artifact Registry fields preparation owns.
type RepositoryMetadata struct {
	Name     string
	Format   string
	Location string
	Labels   map[string]string
}

// NewArtifactRegistryClient builds a live Artifact Registry REST client.
func NewArtifactRegistryClient(ctx context.Context, opts ...option.ClientOption) (ArtifactRegistryAPI, error) {
	scopes := append([]option.ClientOption{option.WithScopes("https://www.googleapis.com/auth/cloud-platform")}, opts...)
	client, err := artifactregistry.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create Artifact Registry client: %w", err)
	}
	return &sdkArtifactRegistry{client: client}, nil
}

type sdkArtifactRegistry struct {
	client *artifactregistry.Service
}

func (c *sdkArtifactRegistry) Close() error { return nil }

func repositoryResourceName(project, location, repositoryID string) string {
	return fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repositoryID)
}

func (c *sdkArtifactRegistry) GetRepository(ctx context.Context, project, location, repositoryID string) (RepositoryMetadata, error) {
	name := repositoryResourceName(project, location, repositoryID)
	repo, err := c.client.Projects.Locations.Repositories.Get(name).Context(ctx).Do()
	if err != nil {
		var apiErr *googleapi.Error
		if errors.As(err, &apiErr) && apiErr.Code == 404 {
			return RepositoryMetadata{}, ErrPreparationNotFound
		}
		return RepositoryMetadata{}, fmt.Errorf("get Artifact Registry repository %q: %w", repositoryID, err)
	}
	return repositoryMetadataFromAPI(repo, location), nil
}

func (c *sdkArtifactRegistry) CreateRepository(ctx context.Context, project, location, repositoryID string, labels map[string]string) (RepositoryMetadata, error) {
	parent := fmt.Sprintf("projects/%s/locations/%s", project, location)
	op, err := c.client.Projects.Locations.Repositories.Create(parent, &artifactregistry.Repository{
		Format: "DOCKER",
		Labels: copyStringMap(labels),
	}).RepositoryId(repositoryID).Context(ctx).Do()
	if err != nil {
		return RepositoryMetadata{}, fmt.Errorf("create Artifact Registry repository %q in %s: %w", repositoryID, location, err)
	}
	if err := waitArtifactRegistryOperation(ctx, c.client, op); err != nil {
		return RepositoryMetadata{}, err
	}
	return c.GetRepository(ctx, project, location, repositoryID)
}

func waitArtifactRegistryOperation(ctx context.Context, client *artifactregistry.Service, op *artifactregistry.Operation) error {
	if op == nil {
		return errors.New("Artifact Registry operation is required")
	}
	name := strings.TrimSpace(op.Name)
	if name == "" {
		if op.Done {
			return artifactRegistryOperationError(op)
		}
		return errors.New("Artifact Registry operation name is missing")
	}
	deadline := time.Now().Add(2 * time.Minute)
	current := op
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if current.Done {
			return artifactRegistryOperationError(current)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for Artifact Registry operation %q", name)
		}
		next, err := client.Projects.Locations.Operations.Get(name).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("poll Artifact Registry operation %q: %w", name, err)
		}
		current = next
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func artifactRegistryOperationError(op *artifactregistry.Operation) error {
	if op == nil || !op.Done {
		return nil
	}
	if op.Error != nil && strings.TrimSpace(op.Error.Message) != "" {
		return fmt.Errorf("Artifact Registry operation failed: %s", op.Error.Message)
	}
	return nil
}

func repositoryMetadataFromAPI(repo *artifactregistry.Repository, location string) RepositoryMetadata {
	if repo == nil {
		return RepositoryMetadata{}
	}
	gotLocation := strings.TrimSpace(location)
	if parts := strings.Split(strings.TrimSpace(repo.Name), "/"); len(parts) >= 4 {
		gotLocation = parts[3]
	}
	return RepositoryMetadata{
		Name:     repo.Name,
		Format:   strings.TrimSpace(repo.Format),
		Location: gotLocation,
		Labels:   copyStringMap(repo.Labels),
	}
}

// ErrPreparationNotFound reports a missing preparation resource.
var ErrPreparationNotFound = errors.New("preparation resource not found")

// EnsureRepository gets or creates the owned Docker repository for the plan.
func (p *AccountPreparer) EnsureRepository(ctx context.Context, plan PreparationPlan, spec Spec) (RepositoryResult, error) {
	if p == nil || p.registry == nil {
		return RepositoryResult{}, errors.New("account preparer is required")
	}
	if err := ctx.Err(); err != nil {
		return RepositoryResult{}, err
	}
	if err := validatePreparationPlanScope(plan, spec); err != nil {
		return RepositoryResult{}, err
	}
	project := strings.TrimSpace(plan.Base.GCPProject)
	location := strings.TrimSpace(plan.RegistryLocation)
	repositoryID := strings.TrimSpace(plan.RegistryID)
	if project == "" || location == "" || repositoryID == "" {
		return RepositoryResult{}, errors.New("preparation plan registry fields are incomplete")
	}
	resource := repositoryResourceName(project, location, repositoryID)
	meta, err := p.registry.GetRepository(ctx, project, location, repositoryID)
	if err != nil && !errors.Is(err, ErrPreparationNotFound) {
		return RepositoryResult{}, err
	}
	if err == nil {
		if err := validateRepositoryOwnership(plan, resource, meta); err != nil {
			return RepositoryResult{}, err
		}
	} else {
		meta, err = p.registry.CreateRepository(ctx, project, location, repositoryID, plan.RegistryLabels)
		if err != nil {
			return RepositoryResult{}, err
		}
	}
	host := location + "-docker.pkg.dev"
	return RepositoryResult{
		RepositoryID: repositoryID,
		Location:     location,
		RegistryHost: host,
		ImageBase:    plan.ImageBase,
	}, nil
}

func validateRepositoryOwnership(plan PreparationPlan, resource string, meta RepositoryMetadata) error {
	if !strings.EqualFold(strings.TrimSpace(meta.Format), "DOCKER") {
		return fmt.Errorf(
			"refusing to mutate Artifact Registry repository %q: format is %q, want DOCKER; %s",
			resource, strings.TrimSpace(meta.Format), preparationOwnershipRefusalHint,
		)
	}
	if !strings.EqualFold(strings.TrimSpace(meta.Location), strings.TrimSpace(plan.RegistryLocation)) {
		return fmt.Errorf(
			"refusing to mutate Artifact Registry repository %q: location is %q, want %q; %s",
			resource, strings.TrimSpace(meta.Location), plan.RegistryLocation, preparationOwnershipRefusalHint,
		)
	}
	return validatePreparationOwnership(resource, plan.RegistryLabels, meta.Labels)
}

func copyStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
