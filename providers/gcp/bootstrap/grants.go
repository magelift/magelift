package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	artifactregistry "google.golang.org/api/artifactregistry/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/iam/v1"
	secretmanager "google.golang.org/api/secretmanager/v1"
	storagev1 "google.golang.org/api/storage/v1"
)

// CIGrantPreparer applies bounded CI IAM grants. It does not run on package import.
type CIGrantPreparer struct {
	registry ArtifactRegistryAPI
	secrets  StateSecretAPI
	buckets  BucketAPI
	wif      WIFAPI

	repoIAM    RepositoryIAMAPI
	secretIAM  SecretIAMAPI
	bucketIAM  BucketIAMAPI
	projectIAM ProjectIAMAPI
}

// NewCIGrantPreparer wires injectable clients for tests and custom endpoints.
func NewCIGrantPreparer(
	registry ArtifactRegistryAPI,
	secrets StateSecretAPI,
	buckets BucketAPI,
	wif WIFAPI,
	repoIAM RepositoryIAMAPI,
	secretIAM SecretIAMAPI,
	bucketIAM BucketIAMAPI,
	projectIAM ProjectIAMAPI,
) (*CIGrantPreparer, error) {
	if registry == nil || secrets == nil || buckets == nil || wif == nil ||
		repoIAM == nil || secretIAM == nil || bucketIAM == nil || projectIAM == nil {
		return nil, errors.New("CI grant preparer requires registry, secrets, bucket, WIF, and IAM clients")
	}
	return &CIGrantPreparer{
		registry:   registry,
		secrets:    secrets,
		buckets:    buckets,
		wif:        wif,
		repoIAM:    repoIAM,
		secretIAM:  secretIAM,
		bucketIAM:  bucketIAM,
		projectIAM: projectIAM,
	}, nil
}

// NewCIGrantPreparerFromContext builds live native clients.
func NewCIGrantPreparerFromContext(ctx context.Context) (_ *CIGrantPreparer, err error) {
	p := &CIGrantPreparer{}
	success := false
	defer func() {
		if !success {
			_ = p.Close()
		}
	}()
	p.registry, err = NewArtifactRegistryClient(ctx)
	if err != nil {
		return nil, err
	}
	p.secrets, err = NewStateSecretClient(ctx)
	if err != nil {
		return nil, err
	}
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	p.buckets = gcsBuckets{client: client}
	p.wif, err = NewWIFClient(ctx)
	if err != nil {
		return nil, err
	}
	p.repoIAM, err = NewRepositoryIAMClient(ctx)
	if err != nil {
		return nil, err
	}
	p.secretIAM, err = NewSecretIAMClient(ctx)
	if err != nil {
		return nil, err
	}
	p.bucketIAM, err = NewBucketIAMClient(ctx)
	if err != nil {
		return nil, err
	}
	p.projectIAM, err = NewProjectIAMClient(ctx)
	if err != nil {
		return nil, err
	}
	success = true
	return p, nil
}

// Close releases owned native clients when supported.
func (p *CIGrantPreparer) Close() error {
	if p == nil {
		return nil
	}
	var first error
	closeIf := func(err error) {
		if err != nil && first == nil {
			first = err
		}
	}
	if p.registry != nil {
		closeIf(p.registry.Close())
	}
	if p.secrets != nil {
		closeIf(p.secrets.Close())
	}
	if p.buckets != nil {
		closeIf(closeGCSBucketClient(p.buckets))
	}
	if p.repoIAM != nil {
		closeIf(p.repoIAM.Close())
	}
	if p.secretIAM != nil {
		closeIf(p.secretIAM.Close())
	}
	if p.bucketIAM != nil {
		closeIf(p.bucketIAM.Close())
	}
	if p.projectIAM != nil {
		closeIf(p.projectIAM.Close())
	}
	return first
}

// Plan returns the deterministic grant recipe for operator review.
func (p *CIGrantPreparer) Plan(inputs CIGrantInputs) (CIGrantPlan, error) {
	return BuildCIGrantPlan(inputs)
}

// CheckOwnedResources verifies repository, secret, bucket, and service account ownership before IAM writes.
func (p *CIGrantPreparer) CheckOwnedResources(ctx context.Context, inputs CIGrantInputs) error {
	if p == nil {
		return errors.New("CI grant preparer is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	plan, err := BuildCIGrantPlan(inputs)
	if err != nil {
		return err
	}
	actualNumber, err := p.wif.ProjectNumber(ctx, plan.GCPProject)
	if err != nil {
		return err
	}
	if actualNumber != plan.ProjectNumber {
		return errors.New("CI grant project number differs from the selected GCP project")
	}
	prep := inputs.Preparation
	project := plan.GCPProject
	location := strings.TrimSpace(prep.RegistryLocation)
	repositoryID := strings.TrimSpace(prep.RegistryID)
	resource := repositoryResourceName(project, location, repositoryID)
	meta, err := p.registry.GetRepository(ctx, project, location, repositoryID)
	if err != nil {
		return fmt.Errorf("CI grants require an existing owned Artifact Registry repository: %w", err)
	}
	if err := validateRepositoryOwnership(prep, resource, meta); err != nil {
		return err
	}
	secretMeta, err := p.secrets.GetSecret(ctx, project, prep.StateSecretID)
	if err != nil {
		return fmt.Errorf("CI grants require an existing owned state passphrase secret: %w", err)
	}
	if err := validatePreparationOwnership(preparationSecretResourceName(project, prep.StateSecretID), prep.StateSecretLabels, secretMeta.Labels); err != nil {
		return err
	}
	exists, bucketMeta, err := p.buckets.BucketExists(ctx, prep.Base.StateBucket)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("CI grants require an existing owned GCS state bucket %q", prep.Base.StateBucket)
	}
	if err := validateExistingStateBucket(prep.Base, bucketMeta); err != nil {
		return err
	}
	if err := p.checkServiceAccountOwnership(ctx, inputs.Deploy); err != nil {
		return err
	}
	return p.checkServiceAccountOwnership(ctx, inputs.Build)
}

func (p *CIGrantPreparer) checkServiceAccountOwnership(ctx context.Context, plan IdentityPlan) error {
	sa, err := p.wif.GetServiceAccount(ctx, plan.ServiceAccountName)
	if err != nil {
		return fmt.Errorf("CI grants require an existing %s service account: %w", plan.Role, err)
	}
	return validateServiceAccountOwnership(plan.ServiceAccountMarker, sa.Description)
}

// Ensure applies the bounded grant plan after ownership checks. Callers must show Plan to the operator first.
func (p *CIGrantPreparer) Ensure(ctx context.Context, inputs CIGrantInputs) error {
	if err := p.CheckOwnedResources(ctx, inputs); err != nil {
		return err
	}
	grantPlan, err := BuildCIGrantPlan(inputs)
	if err != nil {
		return err
	}
	return p.ensurePlan(ctx, grantPlan)
}

func (p *CIGrantPreparer) ensurePlan(ctx context.Context, plan CIGrantPlan) error {
	repoBindings := map[string][]CIGrantBinding{}
	secretBindings := map[string][]CIGrantBinding{}
	bucketBindings := map[string][]CIGrantBinding{}
	projectBindings := map[string][]CIGrantBinding{}
	saBindings := map[string][]CIGrantBinding{}

	for _, binding := range plan.Bindings {
		switch binding.Scope {
		case grantScopeResource:
			switch {
			case strings.Contains(binding.Resource, "/serviceAccounts/"):
				saBindings[binding.Resource] = append(saBindings[binding.Resource], binding)
			case strings.Contains(binding.Resource, "/repositories/"):
				repoBindings[binding.Resource] = append(repoBindings[binding.Resource], binding)
			case strings.Contains(binding.Resource, "/secrets/"):
				secretBindings[binding.Resource] = append(secretBindings[binding.Resource], binding)
			default:
				bucketBindings[binding.Resource] = append(bucketBindings[binding.Resource], binding)
			}
		case grantScopeProject:
			projectBindings[binding.Resource] = append(projectBindings[binding.Resource], binding)
		default:
			return fmt.Errorf("unsupported grant scope %q", binding.Scope)
		}
	}
	for resource, bindings := range saBindings {
		if err := p.ensureNodeImpersonation(ctx, resource, bindings); err != nil {
			return err
		}
	}
	for resource, bindings := range repoBindings {
		if err := p.ensureRepositoryIAM(ctx, resource, bindings); err != nil {
			return err
		}
	}
	for resource, bindings := range secretBindings {
		if err := p.ensureSecretIAM(ctx, resource, bindings); err != nil {
			return err
		}
	}
	for bucket, bindings := range bucketBindings {
		if err := p.ensureBucketIAM(ctx, bucket, bindings); err != nil {
			return err
		}
	}
	for resource, bindings := range projectBindings {
		if err := p.ensureProjectIAM(ctx, strings.TrimPrefix(resource, "projects/"), bindings); err != nil {
			return err
		}
	}
	return nil
}

func (p *CIGrantPreparer) ensureRepositoryIAM(ctx context.Context, resource string, bindings []CIGrantBinding) error {
	policy, err := p.repoIAM.GetRepositoryIAMPolicy(ctx, resource)
	if err != nil {
		return err
	}
	updated := cloneARPolicy(policy)
	changed := false
	for _, binding := range bindings {
		next, delta := mergeUnconditionalARMember(updated, binding.Role, binding.Member)
		updated = next
		changed = changed || delta
	}
	if !changed {
		return nil
	}
	if policy == nil || strings.TrimSpace(policy.Etag) == "" {
		return errors.New("refusing IAM update without a policy etag")
	}
	updated.Etag = policy.Etag
	return p.setRepositoryIAMWithCAS(ctx, resource, updated, bindings)
}

func (p *CIGrantPreparer) setRepositoryIAMWithCAS(ctx context.Context, resource string, policy *artifactregistry.Policy, want []CIGrantBinding) error {
	if err := p.repoIAM.SetRepositoryIAMPolicy(ctx, resource, policy); err != nil {
		if !isIAMPolicyConflict(err) {
			return err
		}
		fresh, err := p.repoIAM.GetRepositoryIAMPolicy(ctx, resource)
		if err != nil {
			return err
		}
		updated := cloneARPolicy(fresh)
		changed := false
		for _, binding := range want {
			next, delta := mergeUnconditionalARMember(updated, binding.Role, binding.Member)
			updated = next
			changed = changed || delta
		}
		if !changed {
			return nil
		}
		if fresh == nil || strings.TrimSpace(fresh.Etag) == "" {
			return errors.New("refusing IAM retry without a fresh policy etag")
		}
		updated.Etag = fresh.Etag
		return p.repoIAM.SetRepositoryIAMPolicy(ctx, resource, updated)
	}
	return nil
}

func closeGCSBucketClient(buckets BucketAPI) error {
	switch api := buckets.(type) {
	case gcsBuckets:
		if api.client != nil {
			return api.client.Close()
		}
	case *gcsBuckets:
		if api != nil && api.client != nil {
			return api.client.Close()
		}
	}
	return nil
}

func (p *CIGrantPreparer) ensureSecretIAM(ctx context.Context, resource string, bindings []CIGrantBinding) error {
	policy, err := p.secretIAM.GetSecretIAMPolicy(ctx, resource)
	if err != nil {
		return err
	}
	updated := cloneSecretPolicy(policy)
	changed := false
	for _, binding := range bindings {
		next, delta := mergeUnconditionalSecretMember(updated, binding.Role, binding.Member)
		updated = next
		changed = changed || delta
	}
	if !changed {
		return nil
	}
	if policy == nil || strings.TrimSpace(policy.Etag) == "" {
		return errors.New("refusing IAM update without a policy etag")
	}
	updated.Etag = policy.Etag
	return p.setSecretIAMWithCAS(ctx, resource, updated, bindings)
}

func (p *CIGrantPreparer) setSecretIAMWithCAS(ctx context.Context, resource string, policy *secretmanager.Policy, want []CIGrantBinding) error {
	if err := p.secretIAM.SetSecretIAMPolicy(ctx, resource, policy); err != nil {
		if !isIAMPolicyConflict(err) {
			return err
		}
		fresh, err := p.secretIAM.GetSecretIAMPolicy(ctx, resource)
		if err != nil {
			return err
		}
		updated := cloneSecretPolicy(fresh)
		changed := false
		for _, binding := range want {
			next, delta := mergeUnconditionalSecretMember(updated, binding.Role, binding.Member)
			updated = next
			changed = changed || delta
		}
		if !changed {
			return nil
		}
		if fresh == nil || strings.TrimSpace(fresh.Etag) == "" {
			return errors.New("refusing IAM retry without a fresh policy etag")
		}
		updated.Etag = fresh.Etag
		return p.secretIAM.SetSecretIAMPolicy(ctx, resource, updated)
	}
	return nil
}

func (p *CIGrantPreparer) ensureBucketIAM(ctx context.Context, bucket string, bindings []CIGrantBinding) error {
	policy, err := p.bucketIAM.GetBucketIAMPolicy(ctx, bucket)
	if err != nil {
		return err
	}
	updated := cloneStoragePolicy(policy)
	changed := false
	for _, binding := range bindings {
		next, delta := mergeUnconditionalStorageMember(updated, binding.Role, binding.Member)
		updated = next
		changed = changed || delta
	}
	if !changed {
		return nil
	}
	if policy == nil || strings.TrimSpace(policy.Etag) == "" {
		return errors.New("refusing IAM update without a policy etag")
	}
	updated.Etag = policy.Etag
	return p.setBucketIAMWithCAS(ctx, bucket, updated, bindings)
}

func (p *CIGrantPreparer) setBucketIAMWithCAS(ctx context.Context, bucket string, policy *storagev1.Policy, want []CIGrantBinding) error {
	if err := p.bucketIAM.SetBucketIAMPolicy(ctx, bucket, policy); err != nil {
		if !isIAMPolicyConflict(err) {
			return err
		}
		fresh, err := p.bucketIAM.GetBucketIAMPolicy(ctx, bucket)
		if err != nil {
			return err
		}
		updated := cloneStoragePolicy(fresh)
		changed := false
		for _, binding := range want {
			next, delta := mergeUnconditionalStorageMember(updated, binding.Role, binding.Member)
			updated = next
			changed = changed || delta
		}
		if !changed {
			return nil
		}
		if fresh == nil || strings.TrimSpace(fresh.Etag) == "" {
			return errors.New("refusing IAM retry without a fresh policy etag")
		}
		updated.Etag = fresh.Etag
		return p.bucketIAM.SetBucketIAMPolicy(ctx, bucket, updated)
	}
	return nil
}

func (p *CIGrantPreparer) ensureProjectIAM(ctx context.Context, projectID string, bindings []CIGrantBinding) error {
	policy, err := p.projectIAM.GetProjectIAMPolicy(ctx, projectID)
	if err != nil {
		return err
	}
	updated := cloneCRMPolicy(policy)
	changed := false
	for _, binding := range bindings {
		next, delta := mergeUnconditionalCRMMember(updated, binding.Role, binding.Member)
		updated = next
		changed = changed || delta
	}
	if !changed {
		return nil
	}
	if policy == nil || strings.TrimSpace(policy.Etag) == "" {
		return errors.New("refusing IAM update without a policy etag")
	}
	updated.Etag = policy.Etag
	return p.setProjectIAMWithCAS(ctx, projectID, updated, bindings)
}

func (p *CIGrantPreparer) setProjectIAMWithCAS(ctx context.Context, projectID string, policy *cloudresourcemanager.Policy, want []CIGrantBinding) error {
	if err := p.projectIAM.SetProjectIAMPolicy(ctx, projectID, policy); err != nil {
		if !isIAMPolicyConflict(err) {
			return err
		}
		fresh, err := p.projectIAM.GetProjectIAMPolicy(ctx, projectID)
		if err != nil {
			return err
		}
		updated := cloneCRMPolicy(fresh)
		changed := false
		for _, binding := range want {
			next, delta := mergeUnconditionalCRMMember(updated, binding.Role, binding.Member)
			updated = next
			changed = changed || delta
		}
		if !changed {
			return nil
		}
		if fresh == nil || strings.TrimSpace(fresh.Etag) == "" {
			return errors.New("refusing IAM retry without a fresh policy etag")
		}
		updated.Etag = fresh.Etag
		return p.projectIAM.SetProjectIAMPolicy(ctx, projectID, updated)
	}
	return nil
}

func (p *CIGrantPreparer) ensureNodeImpersonation(ctx context.Context, resource string, bindings []CIGrantBinding) error {
	for attempt := 0; attempt < 2; attempt++ {
		policy, err := p.wif.GetServiceAccountIAMPolicy(ctx, resource)
		if err != nil {
			return err
		}
		updated := cloneIAMPolicy(policy)
		changed := false
		for _, want := range bindings {
			found := false
			for _, binding := range updated.Bindings {
				if binding != nil && binding.Condition == nil && binding.Role == want.Role {
					found = true
					if !containsString(binding.Members, want.Member) {
						binding.Members = append(binding.Members, want.Member)
						changed = true
					}
					break
				}
			}
			if !found {
				updated.Bindings = append(updated.Bindings, &iam.Binding{Role: want.Role, Members: []string{want.Member}})
				changed = true
			}
		}
		if !changed {
			return nil
		}
		if updated.Etag == "" {
			return errors.New("refusing node impersonation IAM update without a policy etag")
		}
		err = p.wif.SetServiceAccountIAMPolicy(ctx, resource, updated)
		if err == nil {
			return nil
		}
		if attempt == 1 || !isIAMPolicyConflict(err) {
			return err
		}
	}
	return errors.New("node impersonation IAM retry exhausted")
}
