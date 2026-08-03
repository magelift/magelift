package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
)

var (
	ErrEnsureOIDCProvider = errors.New("ensure GitHub OIDC provider failed")
	ErrEnsureRoleBoundary = errors.New("ensure deployment role boundary failed")
	ErrEnsureRole         = errors.New("ensure deployment role failed")
	ErrWriteMetadata      = errors.New("write bootstrap metadata failed")
)

var kmsARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)

const githubOIDCURL = "https://token.actions.githubusercontent.com"

type IdentitySpec struct {
	Project     string
	Environment string
	AccountID   string
	Region      string
	GitHubOwner string
	GitHubRepo  string
	StateBucket string
	KMSKeyARN   string
}

type IdentityPlan struct {
	OIDCProviderARN string `json:"oidcProviderArn" yaml:"oidcProviderArn"`

	// StateRole* is the recovery identity. Its policy is intentionally limited
	// to the Pulumi state bucket, its KMS key, and bootstrap metadata.
	StateRoleName          string `json:"stateRoleName" yaml:"stateRoleName"`
	StateRoleARN           string `json:"stateRoleArn" yaml:"stateRoleArn"`
	StateBoundaryName      string `json:"stateBoundaryName" yaml:"stateBoundaryName"`
	StateBoundaryARN       string `json:"stateBoundaryArn" yaml:"stateBoundaryArn"`
	StatePermissionsPolicy string `json:"statePermissionsPolicy" yaml:"statePermissionsPolicy"`

	// CIRole* is assumed by generated GitHub Actions workflows. It can manage
	// only the services and MageLift-owned resource families required by v1.
	CIRoleName          string `json:"ciRoleName" yaml:"ciRoleName"`
	CIRoleARN           string `json:"ciRoleArn" yaml:"ciRoleArn"`
	CIBoundaryName      string `json:"ciBoundaryName" yaml:"ciBoundaryName"`
	CIBoundaryARN       string `json:"ciBoundaryArn" yaml:"ciBoundaryArn"`
	CITrustPolicy       string `json:"ciTrustPolicy" yaml:"ciTrustPolicy"`
	CIPermissionsPolicy string `json:"ciPermissionsPolicy" yaml:"ciPermissionsPolicy"`

	// BuildRole* is assumed only by the immutable image build job. It can read
	// MageLift-scoped Composer and build secrets, but cannot mutate infrastructure.
	BuildRoleName          string `json:"buildRoleName" yaml:"buildRoleName"`
	BuildRoleARN           string `json:"buildRoleArn" yaml:"buildRoleArn"`
	BuildBoundaryName      string `json:"buildBoundaryName" yaml:"buildBoundaryName"`
	BuildBoundaryARN       string `json:"buildBoundaryArn" yaml:"buildBoundaryArn"`
	BuildTrustPolicy       string `json:"buildTrustPolicy" yaml:"buildTrustPolicy"`
	BuildPermissionsPolicy string `json:"buildPermissionsPolicy" yaml:"buildPermissionsPolicy"`

	MetadataParameter string            `json:"metadataParameter" yaml:"metadataParameter"`
	Tags              map[string]string `json:"tags" yaml:"tags"`
	Region            string            `json:"region" yaml:"region"`
	StateBucket       string            `json:"stateBucket" yaml:"stateBucket"`
	KMSKeyARN         string            `json:"kmsKeyArn" yaml:"kmsKeyArn"`

	// Deprecated aliases remain in the output for consumers of the pre-split
	// bootstrap contract. They point to the recovery identity, never the CI role.
	RoleName          string `json:"roleName" yaml:"roleName"`
	RoleARN           string `json:"roleArn" yaml:"roleArn"`
	BoundaryName      string `json:"boundaryName" yaml:"boundaryName"`
	BoundaryARN       string `json:"boundaryArn" yaml:"boundaryArn"`
	TrustPolicy       string `json:"trustPolicy" yaml:"trustPolicy"`
	PermissionsPolicy string `json:"permissionsPolicy" yaml:"permissionsPolicy"`
}

func BuildIdentityPlan(spec IdentitySpec) (IdentityPlan, error) {
	if !componentPattern.MatchString(spec.Project) || !componentPattern.MatchString(spec.Environment) {
		return IdentityPlan{}, errors.New("identity bootstrap project and environment must be stable names")
	}
	if !accountPattern.MatchString(spec.AccountID) || !regionPattern.MatchString(spec.Region) {
		return IdentityPlan{}, errors.New("identity bootstrap account or region is invalid")
	}
	if !componentPattern.MatchString(spec.GitHubOwner) || !componentPattern.MatchString(spec.GitHubRepo) {
		return IdentityPlan{}, errors.New("GitHub owner and repository must be lowercase stable names")
	}
	if !bucketPattern.MatchString(spec.StateBucket) || !kmsARNPattern.MatchString(spec.KMSKeyARN) || !strings.HasPrefix(spec.KMSKeyARN, "arn:"+awsPartition(spec.Region)+":kms:"+spec.Region+":"+spec.AccountID+":key/") {
		return IdentityPlan{}, errors.New("identity bootstrap state resources are invalid")
	}
	stateRoleName := "magelift-" + spec.Project + "-" + spec.Environment + "-deploy"
	stateBoundaryName := stateRoleName + "-boundary"
	ciRoleName := "magelift-" + spec.Project + "-" + spec.Environment + "-ci"
	ciBoundaryName := ciRoleName + "-boundary"
	buildRoleName := "magelift-" + spec.Project + "-" + spec.Environment + "-build"
	buildBoundaryName := buildRoleName + "-boundary"
	if len(stateRoleName) > 64 || len(stateBoundaryName) > 128 || len(ciRoleName) > 64 || len(ciBoundaryName) > 128 || len(buildRoleName) > 64 || len(buildBoundaryName) > 128 {
		return IdentityPlan{}, errors.New("generated IAM name exceeds the AWS limit")
	}
	partition := awsPartition(spec.Region)
	providerARN := "arn:" + partition + ":iam::" + spec.AccountID + ":oidc-provider/token.actions.githubusercontent.com"
	stateRoleARN := "arn:" + partition + ":iam::" + spec.AccountID + ":role/" + stateRoleName
	stateBoundaryARN := "arn:" + partition + ":iam::" + spec.AccountID + ":policy/" + stateBoundaryName
	ciRoleARN := "arn:" + partition + ":iam::" + spec.AccountID + ":role/" + ciRoleName
	ciBoundaryARN := "arn:" + partition + ":iam::" + spec.AccountID + ":policy/" + ciBoundaryName
	buildRoleARN := "arn:" + partition + ":iam::" + spec.AccountID + ":role/" + buildRoleName
	buildBoundaryARN := "arn:" + partition + ":iam::" + spec.AccountID + ":policy/" + buildBoundaryName
	parameter := "/magelift/bootstrap/" + spec.Project + "/" + spec.Environment
	trust, err := githubTrustPolicy(providerARN, spec.GitHubOwner, spec.GitHubRepo, spec.Environment)
	if err != nil {
		return IdentityPlan{}, err
	}
	metadataARN := "arn:" + partition + ":ssm:" + spec.Region + ":" + spec.AccountID + ":parameter" + parameter
	stateBucketARN := "arn:" + partition + ":s3:::" + spec.StateBucket
	statePolicy, err := statePermissionsPolicy(stateBucketARN, spec.KMSKeyARN, metadataARN)
	if err != nil {
		return IdentityPlan{}, err
	}
	ciPolicy, err := ciPermissionsPolicy(partition, spec.AccountID, spec.Region, providerARN, spec.StateBucket, spec.KMSKeyARN, metadataARN)
	if err != nil {
		return IdentityPlan{}, err
	}
	buildTrust, err := githubTrustPolicy(providerARN, spec.GitHubOwner, spec.GitHubRepo, spec.Environment)
	if err != nil {
		return IdentityPlan{}, err
	}
	buildPolicy, err := buildPermissionsPolicy(partition, spec.AccountID, spec.Region)
	if err != nil {
		return IdentityPlan{}, err
	}
	tags := map[string]string{"magelift:environment": spec.Environment, "magelift:managed-by": "magelift", "magelift:project": spec.Project, "magelift:purpose": "deployment-identity"}
	return IdentityPlan{
		OIDCProviderARN: providerARN,
		StateRoleName:   stateRoleName, StateRoleARN: stateRoleARN, StateBoundaryName: stateBoundaryName, StateBoundaryARN: stateBoundaryARN, StatePermissionsPolicy: statePolicy,
		CIRoleName: ciRoleName, CIRoleARN: ciRoleARN, CIBoundaryName: ciBoundaryName, CIBoundaryARN: ciBoundaryARN, CITrustPolicy: trust, CIPermissionsPolicy: ciPolicy,
		BuildRoleName: buildRoleName, BuildRoleARN: buildRoleARN, BuildBoundaryName: buildBoundaryName, BuildBoundaryARN: buildBoundaryARN, BuildTrustPolicy: buildTrust, BuildPermissionsPolicy: buildPolicy,
		MetadataParameter: parameter, Tags: tags, Region: spec.Region, StateBucket: spec.StateBucket, KMSKeyARN: spec.KMSKeyARN,
		RoleName: stateRoleName, RoleARN: stateRoleARN, BoundaryName: stateBoundaryName, BoundaryARN: stateBoundaryARN, TrustPolicy: trust, PermissionsPolicy: statePolicy,
	}, nil
}

func awsPartition(region string) string {
	if strings.HasPrefix(region, "us-gov-") {
		return "aws-us-gov"
	}
	if strings.HasPrefix(region, "cn-") {
		return "aws-cn"
	}
	return "aws"
}

func canonicalJSON(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode bootstrap policy: %w", err)
	}
	return string(encoded), nil
}

type IAMAPI interface {
	GetOpenIDConnectProvider(context.Context, *iam.GetOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error)
	CreateOpenIDConnectProvider(context.Context, *iam.CreateOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error)
	AddClientIDToOpenIDConnectProvider(context.Context, *iam.AddClientIDToOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error)
	TagOpenIDConnectProvider(context.Context, *iam.TagOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.TagOpenIDConnectProviderOutput, error)
	GetPolicy(context.Context, *iam.GetPolicyInput, ...func(*iam.Options)) (*iam.GetPolicyOutput, error)
	GetPolicyVersion(context.Context, *iam.GetPolicyVersionInput, ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error)
	CreatePolicy(context.Context, *iam.CreatePolicyInput, ...func(*iam.Options)) (*iam.CreatePolicyOutput, error)
	ListPolicyVersions(context.Context, *iam.ListPolicyVersionsInput, ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error)
	CreatePolicyVersion(context.Context, *iam.CreatePolicyVersionInput, ...func(*iam.Options)) (*iam.CreatePolicyVersionOutput, error)
	DeletePolicyVersion(context.Context, *iam.DeletePolicyVersionInput, ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error)
	TagPolicy(context.Context, *iam.TagPolicyInput, ...func(*iam.Options)) (*iam.TagPolicyOutput, error)
	GetRole(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	CreateRole(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	UpdateAssumeRolePolicy(context.Context, *iam.UpdateAssumeRolePolicyInput, ...func(*iam.Options)) (*iam.UpdateAssumeRolePolicyOutput, error)
	PutRolePermissionsBoundary(context.Context, *iam.PutRolePermissionsBoundaryInput, ...func(*iam.Options)) (*iam.PutRolePermissionsBoundaryOutput, error)
	PutRolePolicy(context.Context, *iam.PutRolePolicyInput, ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error)
	TagRole(context.Context, *iam.TagRoleInput, ...func(*iam.Options)) (*iam.TagRoleOutput, error)
}

type SSMMetadataAPI interface {
	PutParameter(context.Context, *ssm.PutParameterInput, ...func(*ssm.Options)) (*ssm.PutParameterOutput, error)
}

type IdentityBootstrapper struct {
	iam IAMAPI
	ssm SSMMetadataAPI
}

func NewIdentity(iamClient IAMAPI, ssmClient SSMMetadataAPI) *IdentityBootstrapper {
	return &IdentityBootstrapper{iam: iamClient, ssm: ssmClient}
}

func (b *IdentityBootstrapper) Ensure(ctx context.Context, plan IdentityPlan) error {
	if b == nil || b.iam == nil || b.ssm == nil {
		return errors.New("identity bootstrap clients are required")
	}
	if err := b.ensureProvider(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureOIDCProvider, err)
	}
	if err := b.ensureBoundary(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureRoleBoundary, err)
	}
	if err := b.ensureRole(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureRole, err)
	}
	if err := b.ensureCIBoundary(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureRoleBoundary, err)
	}
	if err := b.ensureCIRole(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureRole, err)
	}
	if err := b.ensureBuildBoundary(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureRoleBoundary, err)
	}
	if err := b.ensureBuildRole(ctx, plan); err != nil {
		return redactAWS(ctx, ErrEnsureRole, err)
	}
	if err := b.writeMetadata(ctx, plan); err != nil {
		return redactAWS(ctx, ErrWriteMetadata, err)
	}
	return nil
}

func redactAWS(ctx context.Context, fallback, cause error) error {
	if ctxCause := context.Cause(ctx); ctxCause != nil {
		return ctxCause
	}
	if cause == nil {
		return fallback
	}
	var apiErr smithy.APIError
	if errors.As(cause, &apiErr) {
		code := strings.TrimSpace(apiErr.ErrorCode())
		if code != "" {
			return fmt.Errorf("%w: %s", fallback, code)
		}
	}
	return fallback
}

func (b *IdentityBootstrapper) ensureProvider(ctx context.Context, plan IdentityPlan) error {
	provider, err := b.iam.GetOpenIDConnectProvider(ctx, &iam.GetOpenIDConnectProviderInput{OpenIDConnectProviderArn: awssdk.String(plan.OIDCProviderARN)})
	if err != nil && !isIAMNotFound(err) {
		return err
	}
	if isIAMNotFound(err) {
		_, err = b.iam.CreateOpenIDConnectProvider(ctx, &iam.CreateOpenIDConnectProviderInput{Url: awssdk.String(githubOIDCURL), ClientIDList: []string{"sts.amazonaws.com"}, Tags: iamTags(plan.Tags)})
		if err != nil {
			return err
		}
	} else if !containsString(provider.ClientIDList, "sts.amazonaws.com") {
		if _, err = b.iam.AddClientIDToOpenIDConnectProvider(ctx, &iam.AddClientIDToOpenIDConnectProviderInput{OpenIDConnectProviderArn: awssdk.String(plan.OIDCProviderARN), ClientID: awssdk.String("sts.amazonaws.com")}); err != nil {
			return err
		}
	}
	_, err = b.iam.TagOpenIDConnectProvider(ctx, &iam.TagOpenIDConnectProviderInput{OpenIDConnectProviderArn: awssdk.String(plan.OIDCProviderARN), Tags: iamTags(plan.Tags)})
	return err
}

func (b *IdentityBootstrapper) ensureBoundary(ctx context.Context, plan IdentityPlan) error {
	return b.ensureManagedPolicy(ctx, plan.BoundaryName, plan.BoundaryARN, plan.PermissionsPolicy, plan.Tags)
}

func (b *IdentityBootstrapper) ensureRole(ctx context.Context, plan IdentityPlan) error {
	return b.ensureRoleSpec(ctx, plan.StateRoleName, plan.TrustPolicy, plan.StateBoundaryARN, "magelift-state", plan.StatePermissionsPolicy, plan.Tags)
}

func (b *IdentityBootstrapper) ensureCIBoundary(ctx context.Context, plan IdentityPlan) error {
	boundary, err := ciPermissionsBoundaryPolicy()
	if err != nil {
		return err
	}
	return b.ensureBoundarySpec(ctx, plan.CIBoundaryName, plan.CIBoundaryARN, boundary, plan.Tags)
}

func (b *IdentityBootstrapper) ensureCIRole(ctx context.Context, plan IdentityPlan) error {
	return b.ensureRoleSpec(ctx, plan.CIRoleName, plan.CITrustPolicy, plan.CIBoundaryARN, "magelift-ci", plan.CIPermissionsPolicy, plan.Tags)
}

func (b *IdentityBootstrapper) ensureBuildBoundary(ctx context.Context, plan IdentityPlan) error {
	return b.ensureBoundarySpec(ctx, plan.BuildBoundaryName, plan.BuildBoundaryARN, plan.BuildPermissionsPolicy, plan.Tags)
}

func (b *IdentityBootstrapper) ensureBuildRole(ctx context.Context, plan IdentityPlan) error {
	return b.ensureRoleSpec(ctx, plan.BuildRoleName, plan.BuildTrustPolicy, plan.BuildBoundaryARN, "magelift-build", plan.BuildPermissionsPolicy, plan.Tags)
}

func (b *IdentityBootstrapper) ensureBoundarySpec(ctx context.Context, name, arn, policy string, tags map[string]string) error {
	return b.ensureManagedPolicy(ctx, name, arn, policy, tags)
}

func (b *IdentityBootstrapper) ensureManagedPolicy(ctx context.Context, name, arn, policy string, tags map[string]string) error {
	existing, err := b.iam.GetPolicy(ctx, &iam.GetPolicyInput{PolicyArn: awssdk.String(arn)})
	if err != nil && !isIAMNotFound(err) {
		return err
	}
	if isIAMNotFound(err) {
		if _, err = b.iam.CreatePolicy(ctx, &iam.CreatePolicyInput{PolicyName: awssdk.String(name), Description: awssdk.String("MageLift deployment permissions boundary"), PolicyDocument: awssdk.String(policy), Tags: iamTags(tags)}); err != nil {
			return err
		}
	} else {
		if existing != nil && existing.Policy != nil && existing.Policy.DefaultVersionId != nil {
			equal, compareErr := b.managedPolicyVersionMatches(ctx, arn, awssdk.ToString(existing.Policy.DefaultVersionId), policy)
			if compareErr != nil {
				return compareErr
			}
			if equal {
				_, err = b.iam.TagPolicy(ctx, &iam.TagPolicyInput{PolicyArn: awssdk.String(arn), Tags: iamTags(tags)})
				return err
			}
		}
		if err := b.replaceManagedPolicyVersion(ctx, arn, policy); err != nil {
			return err
		}
	}
	_, err = b.iam.TagPolicy(ctx, &iam.TagPolicyInput{PolicyArn: awssdk.String(arn), Tags: iamTags(tags)})
	return err
}

func (b *IdentityBootstrapper) managedPolicyVersionMatches(ctx context.Context, arn, versionID, desired string) (bool, error) {
	version, err := b.iam.GetPolicyVersion(ctx, &iam.GetPolicyVersionInput{PolicyArn: awssdk.String(arn), VersionId: awssdk.String(versionID)})
	if err != nil {
		return false, err
	}
	if version == nil || version.PolicyVersion == nil || version.PolicyVersion.Document == nil {
		return false, nil
	}
	actual, err := canonicalPolicyJSON(awssdk.ToString(version.PolicyVersion.Document))
	if err != nil {
		return false, nil
	}
	want, err := canonicalPolicyJSON(desired)
	if err != nil {
		return false, err
	}
	return actual == want, nil
}

func canonicalPolicyJSON(document string) (string, error) {
	decoded, err := url.QueryUnescape(document)
	if err != nil {
		return "", err
	}
	var value any
	if err := json.Unmarshal([]byte(decoded), &value); err != nil {
		return "", err
	}
	return canonicalJSON(value)
}

func (b *IdentityBootstrapper) replaceManagedPolicyVersion(ctx context.Context, arn, policy string) error {
	// IAM keeps at most five versions. Remove every non-default version before
	// creating the replacement, then remove the old default after the swap.
	if err := b.deleteNonDefaultPolicyVersions(ctx, arn); err != nil {
		return err
	}
	if _, err := b.iam.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn: awssdk.String(arn), PolicyDocument: awssdk.String(policy), SetAsDefault: true,
	}); err != nil {
		return err
	}
	return b.deleteNonDefaultPolicyVersions(ctx, arn)
}

func (b *IdentityBootstrapper) deleteNonDefaultPolicyVersions(ctx context.Context, arn string) error {
	output, err := b.iam.ListPolicyVersions(ctx, &iam.ListPolicyVersionsInput{PolicyArn: awssdk.String(arn)})
	if err != nil {
		return err
	}
	for _, version := range output.Versions {
		if version.IsDefaultVersion || version.VersionId == nil || strings.TrimSpace(*version.VersionId) == "" {
			continue
		}
		if _, err := b.iam.DeletePolicyVersion(ctx, &iam.DeletePolicyVersionInput{PolicyArn: awssdk.String(arn), VersionId: version.VersionId}); err != nil {
			return err
		}
	}
	return nil
}

func (b *IdentityBootstrapper) ensureRoleSpec(ctx context.Context, name, trust, boundary, policyName, policy string, tags map[string]string) error {
	_, err := b.iam.GetRole(ctx, &iam.GetRoleInput{RoleName: awssdk.String(name)})
	if err != nil && !isIAMNotFound(err) {
		return err
	}
	if isIAMNotFound(err) {
		if _, err = b.iam.CreateRole(ctx, &iam.CreateRoleInput{RoleName: awssdk.String(name), AssumeRolePolicyDocument: awssdk.String(trust), PermissionsBoundary: awssdk.String(boundary), Tags: iamTags(tags)}); err != nil {
			return err
		}
	} else if _, err = b.iam.UpdateAssumeRolePolicy(ctx, &iam.UpdateAssumeRolePolicyInput{RoleName: awssdk.String(name), PolicyDocument: awssdk.String(trust)}); err != nil {
		return err
	}
	if _, err = b.iam.PutRolePermissionsBoundary(ctx, &iam.PutRolePermissionsBoundaryInput{RoleName: awssdk.String(name), PermissionsBoundary: awssdk.String(boundary)}); err != nil {
		return err
	}
	if _, err = b.iam.PutRolePolicy(ctx, &iam.PutRolePolicyInput{RoleName: awssdk.String(name), PolicyName: awssdk.String(policyName), PolicyDocument: awssdk.String(policy)}); err != nil {
		return err
	}
	_, err = b.iam.TagRole(ctx, &iam.TagRoleInput{RoleName: awssdk.String(name), Tags: iamTags(tags)})
	return err
}

func (b *IdentityBootstrapper) writeMetadata(ctx context.Context, plan IdentityPlan) error {
	metadata, err := canonicalJSON(map[string]string{"buildBoundaryArn": plan.BuildBoundaryARN, "buildRoleArn": plan.BuildRoleARN, "ciBoundaryArn": plan.CIBoundaryARN, "ciRoleArn": plan.CIRoleARN, "kmsKeyArn": plan.KMSKeyARN, "oidcProviderArn": plan.OIDCProviderARN, "region": plan.Region, "roleArn": plan.StateRoleARN, "stateBoundaryArn": plan.StateBoundaryARN, "stateRoleArn": plan.StateRoleARN, "stateBucket": plan.StateBucket})
	if err != nil {
		return err
	}
	_, err = b.ssm.PutParameter(ctx, &ssm.PutParameterInput{Name: awssdk.String(plan.MetadataParameter), Value: awssdk.String(metadata), Type: ssmtypes.ParameterTypeString, DataType: awssdk.String("text"), Overwrite: awssdk.Bool(true), Tier: ssmtypes.ParameterTierStandard})
	return err
}

func iamTags(tags map[string]string) []iamtypes.Tag {
	keys := sortedKeys(tags)
	result := make([]iamtypes.Tag, 0, len(keys))
	for _, key := range keys {
		result = append(result, iamtypes.Tag{Key: awssdk.String(key), Value: awssdk.String(tags[key])})
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func isIAMNotFound(err error) bool {
	var apiError smithy.APIError
	return errors.As(err, &apiError) && apiError.ErrorCode() == "NoSuchEntity"
}
