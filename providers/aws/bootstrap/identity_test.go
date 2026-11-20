package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
)

func testIdentityPlan(t *testing.T) IdentityPlan {
	t.Helper()
	plan, err := BuildIdentityPlan(IdentitySpec{Project: "shop", Environment: "production", AccountID: "123456789012", Region: "eu-west-3", GitHubOwner: "magelift", GitHubRepo: "magelift", StateBucket: "magelift-123456789012-eu-west-3-shop-production-state", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/key-1"})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

type fakeIAM struct {
	mu                       sync.Mutex
	providerExists           bool
	policyExists             bool
	roleExists               bool
	policies                 map[string]bool
	roles                    map[string]bool
	createProviderCalls      int
	createPolicyCalls        int
	createRoleCalls          int
	updateTrustCalls         int
	putBoundaryCalls         int
	putRolePolicyCalls       int
	listPolicyVersionCalls   int
	createPolicyVersionCalls int
	deletePolicyVersionCalls int
	policyDocuments          map[string]string
	failRolePolicyOnce       bool
	providerError            error
}

func (f *fakeIAM) GetOpenIDConnectProvider(context.Context, *iam.GetOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.GetOpenIDConnectProviderOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.providerError != nil {
		return nil, f.providerError
	}
	if !f.providerExists {
		return nil, &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "missing"}
	}
	return &iam.GetOpenIDConnectProviderOutput{ClientIDList: []string{"sts.amazonaws.com"}, Url: awssdk.String(githubOIDCURL)}, nil
}

func (f *fakeIAM) CreateOpenIDConnectProvider(context.Context, *iam.CreateOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.CreateOpenIDConnectProviderOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.providerExists = true
	f.createProviderCalls++
	return &iam.CreateOpenIDConnectProviderOutput{}, nil
}

func (f *fakeIAM) AddClientIDToOpenIDConnectProvider(context.Context, *iam.AddClientIDToOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.AddClientIDToOpenIDConnectProviderOutput, error) {
	return &iam.AddClientIDToOpenIDConnectProviderOutput{}, nil
}

func (f *fakeIAM) TagOpenIDConnectProvider(context.Context, *iam.TagOpenIDConnectProviderInput, ...func(*iam.Options)) (*iam.TagOpenIDConnectProviderOutput, error) {
	return &iam.TagOpenIDConnectProviderOutput{}, nil
}

func (f *fakeIAM) GetPolicy(_ context.Context, input *iam.GetPolicyInput, _ ...func(*iam.Options)) (*iam.GetPolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.policies != nil {
		found := false
		for name := range f.policies {
			if strings.HasSuffix(awssdk.ToString(input.PolicyArn), "/"+name) {
				found = true
				break
			}
		}
		if !found {
			return nil, &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "missing"}
		}
	} else if !f.policyExists {
		return nil, &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "missing"}
	}
	return &iam.GetPolicyOutput{Policy: &iamtypes.Policy{DefaultVersionId: awssdk.String("v1")}}, nil
}

func (f *fakeIAM) CreatePolicy(_ context.Context, input *iam.CreatePolicyInput, _ ...func(*iam.Options)) (*iam.CreatePolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.policies == nil {
		f.policies = map[string]bool{}
	}
	if f.policyDocuments == nil {
		f.policyDocuments = map[string]string{}
	}
	f.policies[awssdk.ToString(input.PolicyName)] = true
	f.policyDocuments[awssdk.ToString(input.PolicyName)] = awssdk.ToString(input.PolicyDocument)
	f.policyExists = true
	f.createPolicyCalls++
	return &iam.CreatePolicyOutput{}, nil
}

func (f *fakeIAM) GetPolicyVersion(_ context.Context, input *iam.GetPolicyVersionInput, _ ...func(*iam.Options)) (*iam.GetPolicyVersionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := strings.TrimPrefix(awssdk.ToString(input.PolicyArn), "arn:aws:iam::123456789012:policy/")
	document := ""
	if f.policyDocuments != nil {
		document = f.policyDocuments[name]
	}
	return &iam.GetPolicyVersionOutput{PolicyVersion: &iamtypes.PolicyVersion{Document: awssdk.String(document), VersionId: awssdk.String("v1"), IsDefaultVersion: true}}, nil
}

func (f *fakeIAM) ListPolicyVersions(context.Context, *iam.ListPolicyVersionsInput, ...func(*iam.Options)) (*iam.ListPolicyVersionsOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listPolicyVersionCalls++
	return &iam.ListPolicyVersionsOutput{Versions: []iamtypes.PolicyVersion{{VersionId: awssdk.String("v1"), IsDefaultVersion: true}}}, nil
}

func (f *fakeIAM) CreatePolicyVersion(_ context.Context, input *iam.CreatePolicyVersionInput, _ ...func(*iam.Options)) (*iam.CreatePolicyVersionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.createPolicyVersionCalls++
	if f.policyDocuments == nil {
		f.policyDocuments = map[string]string{}
	}
	name := strings.TrimPrefix(awssdk.ToString(input.PolicyArn), "arn:aws:iam::123456789012:policy/")
	f.policyDocuments[name] = awssdk.ToString(input.PolicyDocument)
	return &iam.CreatePolicyVersionOutput{}, nil
}

func (f *fakeIAM) DeletePolicyVersion(context.Context, *iam.DeletePolicyVersionInput, ...func(*iam.Options)) (*iam.DeletePolicyVersionOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deletePolicyVersionCalls++
	return &iam.DeletePolicyVersionOutput{}, nil
}

func (f *fakeIAM) TagPolicy(context.Context, *iam.TagPolicyInput, ...func(*iam.Options)) (*iam.TagPolicyOutput, error) {
	return &iam.TagPolicyOutput{}, nil
}

func (f *fakeIAM) GetRole(_ context.Context, input *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.roles != nil {
		if !f.roles[awssdk.ToString(input.RoleName)] {
			return nil, &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "missing"}
		}
	} else if !f.roleExists {
		return nil, &smithy.GenericAPIError{Code: "NoSuchEntity", Message: "missing"}
	}
	return &iam.GetRoleOutput{}, nil
}

func (f *fakeIAM) CreateRole(_ context.Context, input *iam.CreateRoleInput, _ ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.roles == nil {
		f.roles = map[string]bool{}
	}
	f.roles[awssdk.ToString(input.RoleName)] = true
	f.roleExists = true
	f.createRoleCalls++
	return &iam.CreateRoleOutput{}, nil
}

func (f *fakeIAM) UpdateAssumeRolePolicy(context.Context, *iam.UpdateAssumeRolePolicyInput, ...func(*iam.Options)) (*iam.UpdateAssumeRolePolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateTrustCalls++
	return &iam.UpdateAssumeRolePolicyOutput{}, nil
}

func (f *fakeIAM) PutRolePermissionsBoundary(context.Context, *iam.PutRolePermissionsBoundaryInput, ...func(*iam.Options)) (*iam.PutRolePermissionsBoundaryOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putBoundaryCalls++
	return &iam.PutRolePermissionsBoundaryOutput{}, nil
}

func (f *fakeIAM) PutRolePolicy(context.Context, *iam.PutRolePolicyInput, ...func(*iam.Options)) (*iam.PutRolePolicyOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.putRolePolicyCalls++
	if f.failRolePolicyOnce {
		f.failRolePolicyOnce = false
		return nil, errors.New("injected role policy failure")
	}
	return &iam.PutRolePolicyOutput{}, nil
}

func (f *fakeIAM) TagRole(context.Context, *iam.TagRoleInput, ...func(*iam.Options)) (*iam.TagRoleOutput, error) {
	return &iam.TagRoleOutput{}, nil
}

type fakeMetadataSSM struct {
	mu     sync.Mutex
	inputs []*ssm.PutParameterInput
	err    error
}

func (f *fakeMetadataSSM) PutParameter(_ context.Context, input *ssm.PutParameterInput, _ ...func(*ssm.Options)) (*ssm.PutParameterOutput, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	copy := *input
	f.inputs = append(f.inputs, &copy)
	return &ssm.PutParameterOutput{}, f.err
}

func TestIdentityPlanIsRepoScopedAndLeastPrivilege(t *testing.T) {
	plan := testIdentityPlan(t)
	if !strings.Contains(plan.StateTrustPolicy, "repo:magelift/magelift:environment:production") || !strings.Contains(plan.StateTrustPolicy, "sts.amazonaws.com") {
		t.Fatalf("trust policy is not repository scoped: %s", plan.StateTrustPolicy)
	}
	if strings.Contains(plan.StatePermissionsPolicy, `"Resource":"*"`) || strings.Contains(plan.StatePermissionsPolicy, `"Action":"*"`) {
		t.Fatalf("permissions contain an unrestricted wildcard: %s", plan.StatePermissionsPolicy)
	}
	if !strings.Contains(plan.StatePermissionsPolicy, plan.StateBucket) || !strings.Contains(plan.StatePermissionsPolicy, plan.KMSKeyARN) {
		t.Fatal("state resources are absent from the permissions boundary")
	}
	if plan.CIRoleARN == plan.StateRoleARN || plan.BuildRoleARN == plan.CIRoleARN || !strings.Contains(plan.CIPermissionsPolicy, "ecs:RegisterTaskDefinition") || !strings.Contains(plan.CIPermissionsPolicy, "iam:PassRole") || !strings.Contains(plan.CIPermissionsPolicy, "role/magelift-*") || !strings.Contains(plan.BuildPermissionsPolicy, "secretsmanager:GetSecretValue") || strings.Contains(plan.BuildPermissionsPolicy, `"Action":"*"`) || strings.Contains(plan.CIPermissionsPolicy, `"Action":"*"`) {
		t.Fatalf("CI identity is not explicitly scoped: role=%q policy=%s", plan.CIRoleARN, plan.CIPermissionsPolicy)
	}
	for _, action := range []string{"cloudfront:GetDistribution", "cloudfront:GetDistributionConfig", "cloudfront:ListDistributions", "cloudfront:CreateInvalidation", "cloudfront:GetInvalidation"} {
		if !strings.Contains(plan.CIPermissionsPolicy, `"`+action+`"`) {
			t.Errorf("CI role lacks %s for native edge operations", action)
		}
	}
	if !strings.Contains(plan.CITrustPolicy, "repo:magelift/magelift:environment:production") {
		t.Fatal("CI trust policy is not repository scoped")
	}
}

func TestBuildAndDeployTrustUseSeparateEnvironmentsAndMain(t *testing.T) {
	base := IdentitySpec{Project: "shop", Environment: "preview", AccountID: "123456789012", Region: "eu-west-3", GitHubOwner: "magelift", GitHubRepo: "magelift", StateBucket: "magelift-shop-state", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/1234"}
	for _, environment := range []string{"preview", "staging", "production"} {
		base.Environment = environment
		plan, err := BuildIdentityPlan(base)
		if err != nil {
			t.Fatal(err)
		}
		for _, policy := range []string{plan.CITrustPolicy, plan.BuildTrustPolicy} {
			if !strings.Contains(policy, `"token.actions.githubusercontent.com:ref":"refs/heads/main"`) {
				t.Fatalf("%s missing main restriction: %s", environment, policy)
			}
		}
		if !strings.Contains(plan.CITrustPolicy, ":environment:"+environment) || strings.Contains(plan.CITrustPolicy, ":environment:build") || strings.Contains(plan.CITrustPolicy, ":environment:preview-build") {
			t.Fatalf("deploy trust %s", plan.CITrustPolicy)
		}
		if !strings.Contains(plan.BuildTrustPolicy, ":environment:build") || strings.Contains(plan.BuildTrustPolicy, ":environment:"+environment+`"`) {
			t.Fatalf("build trust %s", plan.BuildTrustPolicy)
		}
		if strings.Contains(plan.BuildTrustPolicy, ":environment:preview-build") != (environment == "preview") {
			t.Fatalf("preview build trust %s", plan.BuildTrustPolicy)
		}
	}
}

func TestPreviewRoleTrustDeniesWrongRepositoryRefAndRole(t *testing.T) {
	plan, err := BuildIdentityPlan(IdentitySpec{Project: "shop", Environment: "preview", AccountID: "123456789012", Region: "eu-west-3", GitHubOwner: "magelift", GitHubRepo: "magelift", StateBucket: "magelift-shop-state", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/1234"})
	if err != nil {
		t.Fatal(err)
	}
	allowed := func(policy, subject, ref string) bool {
		var document struct {
			Statement []struct {
				Condition struct {
					StringEquals map[string]json.RawMessage `json:"StringEquals"`
				} `json:"Condition"`
			} `json:"Statement"`
		}
		if err := json.Unmarshal([]byte(policy), &document); err != nil {
			t.Fatal(err)
		}
		if len(document.Statement) != 1 {
			t.Fatal("unexpected trust statement count")
		}
		claims := document.Statement[0].Condition.StringEquals
		for key, value := range map[string]string{"token.actions.githubusercontent.com:aud": "sts.amazonaws.com", "token.actions.githubusercontent.com:ref": ref, "token.actions.githubusercontent.com:sub": subject} {
			var values []string
			if err := json.Unmarshal(claims[key], &values); err != nil {
				var one string
				if err := json.Unmarshal(claims[key], &one); err != nil {
					t.Fatal(err)
				}
				values = []string{one}
			}
			found := false
			for _, expected := range values {
				if value == expected {
					found = true
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	tests := []struct {
		role, subject, ref string
		want               bool
	}{
		{"build", "repo:magelift/magelift:environment:preview-build", "refs/heads/main", true},
		{"deploy", "repo:magelift/magelift:environment:preview", "refs/heads/main", true},
		{"build", "repo:fork/magelift:environment:preview-build", "refs/heads/main", false},
		{"build", "repo:magelift/magelift:environment:preview-build", "refs/heads/feature", false},
		{"deploy", "repo:magelift/magelift:environment:preview-build", "refs/heads/main", false},
		{"build", "repo:magelift/magelift:environment:preview", "refs/heads/main", false},
	}
	for _, test := range tests {
		policy := plan.BuildTrustPolicy
		if test.role == "deploy" {
			policy = plan.CITrustPolicy
		}
		if got := allowed(policy, test.subject, test.ref); got != test.want {
			t.Errorf("%+v: got %v", test, got)
		}
	}
}

// TestIdentityPolicyDocumentsStayUnderIAMQuotas asserts every rendered IAM
// document against the character quota of the API that submits it.
//
// Quotas (AWS IAM quotas; whitespace is excluded from AWS's count; we measure
// len() on canonicalJSON compact output, which is a sound upper bound):
// https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_iam-quotas.html
//
//   - Customer managed policy document: 6,144 characters (CreatePolicy /
//     CreatePolicyVersion); used for the three permissions boundaries.
//   - Inline role policy document: 10,240 characters (PutRolePolicy).
//   - Role trust policy (AssumeRolePolicyDocument): 2,048 characters by
//     default, raisable to 8,192; we guard the default so a raise is a
//     deliberate decision.
func TestIdentityPolicyDocumentsStayUnderIAMQuotas(t *testing.T) {
	plan := testIdentityPlan(t)
	ciBoundary, err := ciPermissionsBoundaryPolicy()
	if err != nil {
		t.Fatal(err)
	}

	const (
		managedPolicyQuota = 6144
		inlinePolicyQuota  = 10240
		trustPolicyQuota   = 2048 // default; AWS can raise to 8192
	)

	// Documents map to the API that submits them (ensureManagedPolicy → 6144,
	// ensureRoleSpec inline PutRolePolicy → 10240, AssumeRolePolicyDocument → 2048).
	cases := []struct {
		name   string
		doc    string
		quota  int
		remedy string
	}{
		{
			name:   "CI permissions boundary",
			doc:    ciBoundary,
			quota:  managedPolicyQuota,
			remedy: "move detail into the CI inline role policy (see commit 8c3a4c6)",
		},
		{
			name:   "state permissions boundary",
			doc:    plan.StatePermissionsPolicy,
			quota:  managedPolicyQuota,
			remedy: "move detail into the state inline role policy (see commit 8c3a4c6)",
		},
		{
			name:   "build permissions boundary",
			doc:    plan.BuildPermissionsPolicy,
			quota:  managedPolicyQuota,
			remedy: "move detail into the build inline role policy (see commit 8c3a4c6)",
		},
		{
			name:   "CI inline role policy",
			doc:    plan.CIPermissionsPolicy,
			quota:  inlinePolicyQuota,
			remedy: "split actions across additional scoped policies or shrink the allowlist",
		},
		{
			name:   "CI email inline role policy",
			doc:    plan.CIEmailPermissionsPolicy,
			quota:  inlinePolicyQuota,
			remedy: "split actions across additional scoped policies or shrink the allowlist",
		},
		{
			name:   "state inline role policy",
			doc:    plan.StatePermissionsPolicy,
			quota:  inlinePolicyQuota,
			remedy: "split actions across additional scoped policies or shrink the allowlist",
		},
		{
			name:   "build inline role policy",
			doc:    plan.BuildPermissionsPolicy,
			quota:  inlinePolicyQuota,
			remedy: "split actions across additional scoped policies or shrink the allowlist",
		},
		{
			name:   "CI role trust policy",
			doc:    plan.CITrustPolicy,
			quota:  trustPolicyQuota,
			remedy: "narrow conditions or request an AWS quota raise to 8192 before growing further",
		},
		{
			name:   "state role trust policy",
			doc:    plan.StateTrustPolicy,
			quota:  trustPolicyQuota,
			remedy: "narrow conditions or request an AWS quota raise to 8192 before growing further",
		},
		{
			name:   "build role trust policy",
			doc:    plan.BuildTrustPolicy,
			quota:  trustPolicyQuota,
			remedy: "narrow conditions or request an AWS quota raise to 8192 before growing further",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			size := len(tc.doc)
			warnAt := (tc.quota * 90) / 100
			if size > tc.quota {
				t.Fatalf("%s is %d characters, over the IAM quota of %d; %s", tc.name, size, tc.quota, tc.remedy)
			}
			if size > warnAt {
				t.Fatalf("%s is %d characters (%.0f%% of the %d-character IAM quota); %s before the next permission pushes it over", tc.name, size, 100*float64(size)/float64(tc.quota), tc.quota, tc.remedy)
			}
		})
	}
}

func TestIdentityEnsureIsIdempotent(t *testing.T) {
	iamClient, ssmClient := &fakeIAM{}, &fakeMetadataSSM{}
	bootstrapper := NewIdentity(iamClient, ssmClient)
	plan := testIdentityPlan(t)
	for range 2 {
		if err := bootstrapper.Ensure(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
	}
	if iamClient.createProviderCalls != 1 || iamClient.createPolicyCalls != 3 || iamClient.createRoleCalls != 3 {
		t.Fatalf("identity resources recreated: %#v", iamClient)
	}
	if iamClient.updateTrustCalls != 3 || iamClient.putBoundaryCalls != 6 || iamClient.putRolePolicyCalls != 8 || iamClient.listPolicyVersionCalls != 0 || iamClient.createPolicyVersionCalls != 0 || len(ssmClient.inputs) != 2 {
		t.Fatal("existing identity settings were not reconciled")
	}
	for _, input := range ssmClient.inputs {
		if !awssdk.ToBool(input.Overwrite) || awssdk.ToString(input.Name) != plan.MetadataParameter || strings.Contains(strings.ToLower(awssdk.ToString(input.Value)), "secret") {
			t.Fatalf("unsafe metadata input: %#v", input)
		}
	}
	var metadata map[string]string
	if err := json.Unmarshal([]byte(awssdk.ToString(ssmClient.inputs[1].Value)), &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["roleArn"] != plan.StateRoleARN || metadata["stateRoleArn"] != plan.StateRoleARN || metadata["ciRoleArn"] != plan.CIRoleARN || metadata["buildRoleArn"] != plan.BuildRoleARN {
		t.Fatalf("metadata role compatibility = %#v", metadata)
	}
}

func TestIdentityEnsureRecoversAfterPartialRoleFailure(t *testing.T) {
	iamClient, ssmClient := &fakeIAM{failRolePolicyOnce: true}, &fakeMetadataSSM{}
	bootstrapper := NewIdentity(iamClient, ssmClient)
	plan := testIdentityPlan(t)
	if err := bootstrapper.Ensure(context.Background(), plan); !errors.Is(err, ErrEnsureRole) {
		t.Fatalf("first error = %v", err)
	}
	if err := bootstrapper.Ensure(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	if iamClient.createRoleCalls != 3 || iamClient.putRolePolicyCalls != 5 || len(ssmClient.inputs) != 1 {
		t.Fatal("partial role failure was not resumed safely")
	}
}

func TestIdentityEnsureRedactsProviderErrors(t *testing.T) {
	sensitive := "provider response contains plaintext-token"
	err := NewIdentity(&fakeIAM{providerError: errors.New(sensitive)}, &fakeMetadataSSM{}).Ensure(context.Background(), testIdentityPlan(t))
	if !errors.Is(err, ErrEnsureOIDCProvider) || strings.Contains(err.Error(), sensitive) {
		t.Fatalf("error = %v", err)
	}
}

func TestIdentityEnsurePreservesCancellation(t *testing.T) {
	cause := errors.New("bootstrap canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	err := NewIdentity(&fakeIAM{providerError: context.Canceled}, &fakeMetadataSSM{}).Ensure(ctx, testIdentityPlan(t))
	if !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
}

func TestBuildIdentityPlanUsesAWSPartitionForGovCloud(t *testing.T) {
	plan, err := BuildIdentityPlan(IdentitySpec{
		Project: "shop", Environment: "production", AccountID: "123456789012", Region: "us-gov-west-1",
		GitHubOwner: "magelift", GitHubRepo: "magelift", StateBucket: "magelift-state",
		KMSKeyARN: "arn:aws-us-gov:kms:us-gov-west-1:123456789012:key/00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{plan.OIDCProviderARN, plan.StateRoleARN, plan.StateBoundaryARN, plan.MetadataParameter} {
		if strings.HasPrefix(value, "arn:aws:") {
			t.Fatalf("partition was not preserved in %q", value)
		}
	}
	if !strings.Contains(plan.StatePermissionsPolicy, `"Resource":"arn:aws-us-gov:s3:::magelift-state"`) {
		t.Fatalf("GovCloud S3 policy resource used the wrong partition: %s", plan.StatePermissionsPolicy)
	}
}

func TestBuildIdentityPlanUsesAWSChinaPartitionForStatePolicy(t *testing.T) {
	plan, err := BuildIdentityPlan(IdentitySpec{
		Project: "shop", Environment: "production", AccountID: "123456789012", Region: "cn-north-1",
		GitHubOwner: "magelift", GitHubRepo: "magelift", StateBucket: "magelift-state",
		KMSKeyARN: "arn:aws-cn:kms:cn-north-1:123456789012:key/00000000-0000-0000-0000-000000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.StatePermissionsPolicy, `"Resource":"arn:aws-cn:s3:::magelift-state"`) {
		t.Fatalf("China S3 policy resource used the wrong partition: %s", plan.StatePermissionsPolicy)
	}
}
