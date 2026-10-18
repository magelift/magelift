package bootstrap

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/magelift/magelift/providers/gcp/naming"
)

const (
	roleArtifactRegistryWriter    = "roles/artifactregistry.writer"
	roleArtifactRegistryReader    = "roles/artifactregistry.reader"
	roleSecretAccessor            = "roles/secretmanager.secretAccessor"
	roleStorageObjectAdmin        = "roles/storage.objectAdmin"
	roleStorageLegacyBucketReader = "roles/storage.legacyBucketReader"
	roleContainerDefaultNodeSA    = "roles/container.defaultNodeServiceAccount"
	roleNodeServiceAccountUser    = "roles/iam.serviceAccountUser"

	grantPurposeBuildPushImage        = "CI build identity pushes application images to the owned Artifact Registry repository"
	grantPurposeDeployPullImage       = "CI deploy identity pulls and verifies application images from the owned Artifact Registry repository"
	grantPurposeNodePullImage         = "GKE node service account pulls application images from the owned Artifact Registry repository"
	grantPurposeDeployStateSecret     = "CI deploy identity reads the owned Pulumi state passphrase secret"
	grantPurposeDeployStateBucket     = "CI deploy identity manages encrypted Pulumi state objects in the owned GCS bucket"
	grantPurposeDeployStateBucketMeta = "CI deploy identity reads owned state bucket metadata"
	grantPurposeDeployProjectGraph    = "CI deploy identity provisions the Magento environment (project-wide IAM; administrator review required)"
	grantPurposeNodeGKEMinimum        = "GKE node service account minimum role for node workloads (project-wide IAM)"

	identityRoleGKENode = "gke-node"

	grantScopeResource = "resource"
	grantScopeProject  = "project"

	grantProjectScopeNote = "IAM role applies to the entire GCP project; MageLift does not restrict project IAM bindings to resource name prefixes."
)

var projectNumberPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

// deployProjectGraphRoles lists project-level roles required for the deployment Pulumi graph.
// These are intentionally broad within a dedicated test project and require human review before apply.
var deployProjectGraphRoles = []string{
	"roles/browser",
	"roles/serviceusage.serviceUsageConsumer",
	"roles/compute.networkAdmin",
	"roles/compute.securityAdmin",
	"roles/container.admin",
	"roles/cloudsql.admin",
	"roles/memorystore.admin",
	"roles/networkconnectivity.consumerNetworkAdmin",
	"roles/servicenetworking.networksAdmin",
	"roles/iam.serviceAccountAdmin",
	"roles/storage.admin",
	"roles/storage.hmacKeyAdmin",
	"roles/secretmanager.admin",
	"roles/logging.configWriter",
	"roles/logging.viewer",
	"roles/monitoring.editor",
}

// CIGrantInputs binds deterministic preparation and identity plans to a verified project number.
type CIGrantInputs struct {
	Preparation   PreparationPlan
	Deploy        IdentityPlan
	Build         IdentityPlan
	ProjectNumber string
}

// CIGrantBinding is one intended IAM grant with explanatory metadata for operator review.
type CIGrantBinding struct {
	Resource     string `json:"resource" yaml:"resource"`
	Member       string `json:"member" yaml:"member"`
	Role         string `json:"role" yaml:"role"`
	Scope        string `json:"scope" yaml:"scope"`
	Purpose      string `json:"purpose" yaml:"purpose"`
	ScopeNote    string `json:"scopeNote,omitempty" yaml:"scopeNote,omitempty"`
	IdentityRole string `json:"identityRole,omitempty" yaml:"identityRole,omitempty"`
}

// CIGrantPlan is the full bounded grant recipe. Applying it requires an explicit Ensure call.
type CIGrantPlan struct {
	GCPProject         string           `json:"gcpProject" yaml:"gcpProject"`
	ProjectNumber      string           `json:"projectNumber" yaml:"projectNumber"`
	NodeServiceAccount string           `json:"nodeServiceAccount" yaml:"nodeServiceAccount"`
	Bindings           []CIGrantBinding `json:"bindings" yaml:"bindings"`
}

// BuildCIGrantPlan derives CI IAM grants from preparation and identity plans without cloud calls.
func BuildCIGrantPlan(inputs CIGrantInputs) (CIGrantPlan, error) {
	if err := validateCIGrantInputs(inputs); err != nil {
		return CIGrantPlan{}, err
	}
	project := strings.TrimSpace(inputs.Preparation.Base.GCPProject)
	projectNumber := strings.TrimSpace(inputs.ProjectNumber)
	repoResource := repositoryResourceName(project, inputs.Preparation.RegistryLocation, inputs.Preparation.RegistryID)
	secretResource := preparationSecretResourceName(project, inputs.Preparation.StateSecretID)
	bucketResource := inputs.Preparation.Base.StateBucket

	deployMember, err := serviceAccountMember(inputs.Deploy)
	if err != nil {
		return CIGrantPlan{}, err
	}
	buildMember, err := serviceAccountMember(inputs.Build)
	if err != nil {
		return CIGrantPlan{}, err
	}
	nodeEmail := naming.NodeServiceAccountEmail(project, inputs.Deploy.Project, inputs.Deploy.Environment)
	nodeMember := "serviceAccount:" + nodeEmail

	bindings := []CIGrantBinding{
		{Resource: "projects/" + project + "/serviceAccounts/" + nodeEmail, Member: deployMember, Role: roleNodeServiceAccountUser, Scope: grantScopeResource, Purpose: "CI deploy identity attaches the reference GKE node service account", IdentityRole: identityRoleDeploy},
		{
			Resource: repoResource, Member: buildMember, Role: roleArtifactRegistryWriter,
			Scope: grantScopeResource, Purpose: grantPurposeBuildPushImage, IdentityRole: identityRoleBuild,
		},
		{
			Resource: repoResource, Member: deployMember, Role: roleArtifactRegistryReader,
			Scope: grantScopeResource, Purpose: grantPurposeDeployPullImage, IdentityRole: identityRoleDeploy,
		},
		{
			Resource: repoResource, Member: nodeMember, Role: roleArtifactRegistryReader,
			Scope: grantScopeResource, Purpose: grantPurposeNodePullImage, IdentityRole: identityRoleGKENode,
		},
		{
			Resource: secretResource, Member: deployMember, Role: roleSecretAccessor,
			Scope: grantScopeResource, Purpose: grantPurposeDeployStateSecret, IdentityRole: identityRoleDeploy,
		},
		{
			Resource: bucketResource, Member: deployMember, Role: roleStorageObjectAdmin,
			Scope: grantScopeResource, Purpose: grantPurposeDeployStateBucket, IdentityRole: identityRoleDeploy,
		},
		{
			Resource: bucketResource, Member: deployMember, Role: roleStorageLegacyBucketReader,
			Scope: grantScopeResource, Purpose: grantPurposeDeployStateBucketMeta, IdentityRole: identityRoleDeploy,
		},
	}
	for _, role := range deployProjectGraphRoles {
		bindings = append(bindings, CIGrantBinding{
			Resource:     "projects/" + project,
			Member:       deployMember,
			Role:         role,
			Scope:        grantScopeProject,
			Purpose:      grantPurposeDeployProjectGraph,
			ScopeNote:    grantProjectScopeNote,
			IdentityRole: identityRoleDeploy,
		})
	}
	bindings = append(bindings, CIGrantBinding{
		Resource:     "projects/" + project,
		Member:       nodeMember,
		Role:         roleContainerDefaultNodeSA,
		Scope:        grantScopeProject,
		Purpose:      grantPurposeNodeGKEMinimum,
		ScopeNote:    grantProjectScopeNote,
		IdentityRole: identityRoleGKENode,
	})
	return CIGrantPlan{
		GCPProject:         project,
		ProjectNumber:      projectNumber,
		NodeServiceAccount: nodeEmail,
		Bindings:           bindings,
	}, nil
}

func validateCIGrantInputs(inputs CIGrantInputs) error {
	prep := inputs.Preparation
	deploy := inputs.Deploy
	build := inputs.Build
	projectNumber := strings.TrimSpace(inputs.ProjectNumber)
	if projectNumber == "" || !projectNumberPattern.MatchString(projectNumber) {
		return errors.New("CI grant preparation requires a numeric GCP project number")
	}
	if deploy.Role != identityRoleDeploy {
		return fmt.Errorf("deploy identity plan role is %q, want %q", deploy.Role, identityRoleDeploy)
	}
	if build.Role != identityRoleBuild {
		return fmt.Errorf("build identity plan role is %q, want %q", build.Role, identityRoleBuild)
	}
	gcpProject := strings.TrimSpace(prep.Base.GCPProject)
	if gcpProject == "" {
		return errors.New("preparation plan GCP project is required")
	}
	if strings.TrimSpace(deploy.GCPProject) != gcpProject || strings.TrimSpace(build.GCPProject) != gcpProject {
		return errors.New("identity plans must target the same GCP project as preparation")
	}
	if strings.TrimSpace(deploy.Project) != strings.TrimSpace(build.Project) {
		return errors.New("deploy and build identity plans must share the same MageLift project name")
	}
	if strings.TrimSpace(deploy.Environment) != strings.TrimSpace(build.Environment) {
		return errors.New("deploy and build identity plans must share the same MageLift environment name")
	}
	wantProject := strings.TrimSpace(prep.Base.Labels["magelift-project"])
	wantEnvironment := strings.TrimSpace(prep.Base.Labels["magelift-environment"])
	if wantProject == "" || wantEnvironment == "" {
		return errors.New("preparation plan ownership labels are incomplete")
	}
	if err := validatePreparationPlanScope(prep, Spec{Project: wantProject, Environment: wantEnvironment, GCPProject: gcpProject, Region: prep.Base.Region}); err != nil {
		return err
	}
	if deploy.Project != wantProject || build.Project != wantProject {
		return errors.New("identity plans do not match preparation MageLift project label")
	}
	if deploy.Environment != wantEnvironment || build.Environment != wantEnvironment {
		return errors.New("identity plans do not match preparation MageLift environment label")
	}
	if strings.TrimSpace(prep.RegistryID) == "" || strings.TrimSpace(prep.RegistryLocation) == "" {
		return errors.New("preparation plan registry fields are incomplete")
	}
	if strings.TrimSpace(prep.StateSecretID) == "" || strings.TrimSpace(prep.Base.StateBucket) == "" {
		return errors.New("preparation plan state secret and bucket fields are incomplete")
	}
	if err := validateIdentityPlanOwnership(deploy); err != nil {
		return err
	}
	if err := validateIdentityPlanOwnership(build); err != nil {
		return err
	}
	expectedNode := naming.NodeServiceAccountEmail(gcpProject, wantProject, wantEnvironment)
	if expectedNode == "" || !strings.Contains(expectedNode, "@") {
		return errors.New("owned node service account email is invalid")
	}
	return nil
}

func validateIdentityPlanOwnership(plan IdentityPlan) error {
	owner, repo, ok := strings.Cut(plan.Repository, "/")
	if !ok {
		return errors.New("identity repository is incomplete")
	}
	expected, err := BuildIdentityPlan(IdentitySpec{Project: plan.Project, Environment: plan.Environment, GCPProject: plan.GCPProject, GitHubOwner: owner, GitHubRepo: repo, GitHubRepositoryID: plan.GitHubRepositoryID, GitHubOwnerID: plan.GitHubOwnerID, Role: plan.Role})
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(plan, expected) {
		return errors.New("identity plan differs from its deterministic ownership and trust recipe")
	}
	if strings.TrimSpace(plan.ServiceAccountEmail) == "" || strings.TrimSpace(plan.ServiceAccountMarker) == "" {
		return errors.New("identity plan service account fields are incomplete")
	}
	if !strings.HasSuffix(plan.ServiceAccountEmail, "@"+plan.GCPProject+".iam.gserviceaccount.com") {
		return fmt.Errorf("identity plan service account email %q does not belong to GCP project %q", plan.ServiceAccountEmail, plan.GCPProject)
	}
	return nil
}

func serviceAccountMember(plan IdentityPlan) (string, error) {
	email := strings.TrimSpace(plan.ServiceAccountEmail)
	if email == "" {
		return "", errors.New("identity plan service account email is required")
	}
	wantEmail := strings.TrimSpace(plan.ServiceAccountID) + "@" + strings.TrimSpace(plan.GCPProject) + ".iam.gserviceaccount.com"
	if email != wantEmail {
		return "", fmt.Errorf("identity plan service account email %q does not match derived email %q", email, wantEmail)
	}
	return "serviceAccount:" + email, nil
}
