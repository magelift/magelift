package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"google.golang.org/api/googleapi"

	"github.com/magelift/magelift/providers/gcp/naming"
)

// NodeIdentityPlan is the deterministic owned GKE node service account recipe.
type NodeIdentityPlan struct {
	GCPProject           string `json:"gcpProject" yaml:"gcpProject"`
	ServiceAccountID     string `json:"serviceAccountId" yaml:"serviceAccountId"`
	ServiceAccountEmail  string `json:"serviceAccountEmail" yaml:"serviceAccountEmail"`
	ServiceAccountName   string `json:"serviceAccountName" yaml:"serviceAccountName"`
	ServiceAccountMarker string `json:"serviceAccountMarker" yaml:"serviceAccountMarker"`
}

// NodeIdentityResult is the Ensure output for runtime node identity preparation.
type NodeIdentityResult struct {
	ServiceAccountName  string `json:"serviceAccountName" yaml:"serviceAccountName"`
	ServiceAccountEmail string `json:"serviceAccountEmail" yaml:"serviceAccountEmail"`
}

// NodeIdentityPreparer ensures the owned GKE node service account and its bounded IAM grants.
type NodeIdentityPreparer struct {
	wif        WIFAPI
	registry   ArtifactRegistryAPI
	repoIAM    RepositoryIAMAPI
	projectIAM ProjectIAMAPI
}

// NewNodeIdentityPreparer wires injectable clients for tests and custom endpoints.
func NewNodeIdentityPreparer(
	wif WIFAPI,
	registry ArtifactRegistryAPI,
	repoIAM RepositoryIAMAPI,
	projectIAM ProjectIAMAPI,
) (*NodeIdentityPreparer, error) {
	if wif == nil || registry == nil || repoIAM == nil || projectIAM == nil {
		return nil, errors.New("node identity preparer requires WIF, Artifact Registry, and IAM clients")
	}
	return &NodeIdentityPreparer{
		wif:        wif,
		registry:   registry,
		repoIAM:    repoIAM,
		projectIAM: projectIAM,
	}, nil
}

// NewNodeIdentityPreparerFromContext builds live native clients.
func NewNodeIdentityPreparerFromContext(ctx context.Context) (_ *NodeIdentityPreparer, err error) {
	p := &NodeIdentityPreparer{}
	success := false
	defer func() {
		if !success {
			_ = p.Close()
		}
	}()
	p.wif, err = NewWIFClient(ctx)
	if err != nil {
		return nil, err
	}
	p.registry, err = NewArtifactRegistryClient(ctx)
	if err != nil {
		return nil, err
	}
	p.repoIAM, err = NewRepositoryIAMClient(ctx)
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
func (p *NodeIdentityPreparer) Close() error {
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
	if p.repoIAM != nil {
		closeIf(p.repoIAM.Close())
	}
	if p.projectIAM != nil {
		closeIf(p.projectIAM.Close())
	}
	return first
}

// NodeServiceAccountOwnershipMarker is the exact service account description MageLift owns.
func NodeServiceAccountOwnershipMarker(project, environment string) string {
	return fmt.Sprintf("%s role=%s", wifSharedOwnershipMarker(project, environment), identityRoleGKENode)
}

// BuildNodeIdentityPlan validates spec inputs and returns the owned node service account recipe.
func BuildNodeIdentityPlan(spec Spec) (NodeIdentityPlan, error) {
	if !componentPattern.MatchString(spec.Project) || !componentPattern.MatchString(spec.Environment) {
		return NodeIdentityPlan{}, errors.New("node identity project and environment must be stable names")
	}
	if !projectPattern.MatchString(spec.GCPProject) {
		return NodeIdentityPlan{}, errors.New("node identity GCP project ID is invalid")
	}
	saID := naming.NodeServiceAccountID(spec.Project, spec.Environment)
	email := naming.NodeServiceAccountEmail(spec.GCPProject, spec.Project, spec.Environment)
	saName := "projects/" + spec.GCPProject + "/serviceAccounts/" + email
	return NodeIdentityPlan{
		GCPProject:           spec.GCPProject,
		ServiceAccountID:     saID,
		ServiceAccountEmail:  email,
		ServiceAccountName:   saName,
		ServiceAccountMarker: NodeServiceAccountOwnershipMarker(spec.Project, spec.Environment),
	}, nil
}

// Ensure creates or verifies the owned node service account and applies bounded IAM grants.
func (p *NodeIdentityPreparer) Ensure(ctx context.Context, prep PreparationPlan, spec Spec) (NodeIdentityResult, error) {
	if p == nil {
		return NodeIdentityResult{}, errors.New("node identity preparer is required")
	}
	if err := ctx.Err(); err != nil {
		return NodeIdentityResult{}, err
	}
	if err := validatePreparationPlanScope(prep, spec); err != nil {
		return NodeIdentityResult{}, err
	}
	plan, err := BuildNodeIdentityPlan(spec)
	if err != nil {
		return NodeIdentityResult{}, err
	}
	project := strings.TrimSpace(spec.GCPProject)
	location := strings.TrimSpace(prep.RegistryLocation)
	repositoryID := strings.TrimSpace(prep.RegistryID)
	resource := repositoryResourceName(project, location, repositoryID)
	meta, err := p.registry.GetRepository(ctx, project, location, repositoryID)
	if err != nil {
		return NodeIdentityResult{}, fmt.Errorf("node identity requires an existing owned Artifact Registry repository: %w", err)
	}
	if err := validateRepositoryOwnership(prep, resource, meta); err != nil {
		return NodeIdentityResult{}, err
	}
	sa, err := p.ensureOwnedNodeServiceAccount(ctx, plan)
	if err != nil {
		return NodeIdentityResult{}, err
	}
	if err := p.ensureNodeIAMGrants(ctx, project, resource, sa.Email); err != nil {
		return NodeIdentityResult{}, err
	}
	return NodeIdentityResult{
		ServiceAccountName:  sa.Name,
		ServiceAccountEmail: sa.Email,
	}, nil
}

func (p *NodeIdentityPreparer) ensureOwnedNodeServiceAccount(ctx context.Context, plan NodeIdentityPlan) (WIFServiceAccount, error) {
	sa, err := p.wif.GetServiceAccount(ctx, plan.ServiceAccountName)
	if err == nil {
		if err := validateOwnedNodeServiceAccount(plan, sa); err != nil {
			return WIFServiceAccount{}, err
		}
		return sa, nil
	}
	if !errors.Is(err, ErrWIFNotFound) {
		return WIFServiceAccount{}, err
	}
	display := truncateDisplay(plan.ServiceAccountMarker)
	created, err := p.wif.CreateServiceAccount(ctx, plan.GCPProject, plan.ServiceAccountID, display, plan.ServiceAccountMarker)
	if err == nil {
		if err := validateOwnedNodeServiceAccount(plan, created); err != nil {
			return WIFServiceAccount{}, err
		}
		return created, nil
	}
	if !isServiceAccountCreateConflict(err) {
		return WIFServiceAccount{}, err
	}
	refetched, err := p.wif.GetServiceAccount(ctx, plan.ServiceAccountName)
	if err != nil {
		return WIFServiceAccount{}, err
	}
	if err := validateOwnedNodeServiceAccount(plan, refetched); err != nil {
		return WIFServiceAccount{}, err
	}
	return refetched, nil
}

func validateOwnedNodeServiceAccount(plan NodeIdentityPlan, sa WIFServiceAccount) error {
	if strings.TrimSpace(sa.Name) != plan.ServiceAccountName {
		return errors.New("refusing node service account outside the owned resource name")
	}
	if strings.TrimSpace(sa.Email) != plan.ServiceAccountEmail {
		return fmt.Errorf("refusing to adopt foreign node service account: email %q does not match owned recipe %q", sa.Email, plan.ServiceAccountEmail)
	}
	if strings.Contains(sa.Email, "-compute@developer.gserviceaccount.com") {
		return errors.New("refusing to adopt the default compute service account as the GKE node identity")
	}
	return validateServiceAccountOwnership(plan.ServiceAccountMarker, sa.Description)
}

func isServiceAccountCreateConflict(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.Code == http.StatusConflict
}

func (p *NodeIdentityPreparer) ensureNodeIAMGrants(ctx context.Context, project, repoResource, nodeEmail string) error {
	nodeMember := "serviceAccount:" + nodeEmail
	helper := &CIGrantPreparer{repoIAM: p.repoIAM, projectIAM: p.projectIAM}
	repoBindings := []CIGrantBinding{{
		Resource:     repoResource,
		Member:       nodeMember,
		Role:         roleArtifactRegistryReader,
		Scope:        grantScopeResource,
		Purpose:      grantPurposeNodePullImage,
		IdentityRole: identityRoleGKENode,
	}}
	if err := helper.ensureRepositoryIAM(ctx, repoResource, repoBindings); err != nil {
		return nodeIAMPreparationError(err, "prepare GKE node registry read access", "artifactregistry.repositories.getIamPolicy and artifactregistry.repositories.setIamPolicy on the owned repository")
	}
	projectResource := "projects/" + project
	projectBindings := []CIGrantBinding{{
		Resource:     projectResource,
		Member:       nodeMember,
		Role:         roleContainerDefaultNodeSA,
		Scope:        grantScopeProject,
		Purpose:      grantPurposeNodeGKEMinimum,
		ScopeNote:    grantProjectScopeNote,
		IdentityRole: identityRoleGKENode,
	}}
	if err := helper.ensureProjectIAM(ctx, project, projectBindings); err != nil {
		return nodeIAMPreparationError(err, "prepare the GKE node minimum runtime role", "resourcemanager.projects.getIamPolicy and resourcemanager.projects.setIamPolicy")
	}
	return nil
}

func nodeIAMPreparationError(err error, stage, permission string) error {
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusForbidden {
		return fmt.Errorf("%s: ask the project administrator to permit %s for bootstrap, then rerun magelift bootstrap: %w", stage, permission, err)
	}
	return fmt.Errorf("%s: %w", stage, err)
}
