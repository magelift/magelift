package edge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/magelift/magelift/sdk"
)

type fakeCloudFrontAPI struct {
	distributions        map[string]*cloudfronttypes.Distribution
	etags                map[string]string
	invalidations        map[string]*cloudfronttypes.Invalidation
	createCalls          int
	updateCalls          int
	deleteCalls          int
	invalidationsCreated int
}

func newFakeCloudFrontAPI() *fakeCloudFrontAPI {
	return &fakeCloudFrontAPI{distributions: make(map[string]*cloudfronttypes.Distribution), etags: make(map[string]string), invalidations: make(map[string]*cloudfronttypes.Invalidation)}
}

func (api *fakeCloudFrontAPI) ListDistributions(_ context.Context, input *cloudfront.ListDistributionsInput) (*cloudfront.ListDistributionsOutput, error) {
	if input != nil && input.Marker != nil && aws.ToString(input.Marker) != "" {
		return &cloudfront.ListDistributionsOutput{DistributionList: &cloudfronttypes.DistributionList{IsTruncated: aws.Bool(false), Items: nil}}, nil
	}
	items := make([]cloudfronttypes.DistributionSummary, 0, len(api.distributions))
	for _, distribution := range api.distributions {
		items = append(items, cloudfronttypes.DistributionSummary{Id: distribution.Id, Comment: distribution.DistributionConfig.Comment, DomainName: distribution.DomainName, Status: distribution.Status, Enabled: distribution.DistributionConfig.Enabled})
	}
	return &cloudfront.ListDistributionsOutput{DistributionList: &cloudfronttypes.DistributionList{IsTruncated: aws.Bool(false), Items: items}}, nil
}

func (api *fakeCloudFrontAPI) CreateDistribution(_ context.Context, input *cloudfront.CreateDistributionInput) (*cloudfront.CreateDistributionOutput, error) {
	api.createCalls++
	if input == nil || input.DistributionConfig == nil {
		return nil, errors.New("missing distribution configuration")
	}
	for _, existing := range api.distributions {
		if aws.ToString(existing.DistributionConfig.CallerReference) == aws.ToString(input.DistributionConfig.CallerReference) {
			return nil, errors.New("caller reference already exists")
		}
	}
	id := "E1"
	api.etags[id] = "etag-1"
	distribution := &cloudfronttypes.Distribution{Id: aws.String(id), DomainName: aws.String("d111111abcdef8.cloudfront.net"), Status: aws.String("InProgress"), DistributionConfig: cloneDistributionConfig(input.DistributionConfig)}
	api.distributions[id] = distribution
	return &cloudfront.CreateDistributionOutput{Distribution: distribution, ETag: aws.String(api.etags[id])}, nil
}

func (api *fakeCloudFrontAPI) GetDistribution(_ context.Context, input *cloudfront.GetDistributionInput) (*cloudfront.GetDistributionOutput, error) {
	id := aws.ToString(input.Id)
	distribution, ok := api.distributions[id]
	if !ok {
		return nil, &cloudfronttypes.NoSuchDistribution{}
	}
	distribution.Status = aws.String(cloudFrontStatusDeployed)
	return &cloudfront.GetDistributionOutput{Distribution: distribution, ETag: aws.String(api.etags[id])}, nil
}

func (api *fakeCloudFrontAPI) GetDistributionConfig(_ context.Context, input *cloudfront.GetDistributionConfigInput) (*cloudfront.GetDistributionConfigOutput, error) {
	id := aws.ToString(input.Id)
	distribution, ok := api.distributions[id]
	if !ok {
		return nil, &cloudfronttypes.NoSuchDistribution{}
	}
	return &cloudfront.GetDistributionConfigOutput{DistributionConfig: distribution.DistributionConfig, ETag: aws.String(api.etags[id])}, nil
}

func (api *fakeCloudFrontAPI) UpdateDistribution(_ context.Context, input *cloudfront.UpdateDistributionInput) (*cloudfront.UpdateDistributionOutput, error) {
	id := aws.ToString(input.Id)
	distribution, ok := api.distributions[id]
	if !ok {
		return nil, &cloudfronttypes.NoSuchDistribution{}
	}
	api.updateCalls++
	distribution.DistributionConfig = cloneDistributionConfig(input.DistributionConfig)
	api.etags[id] = api.etags[id] + "-next"
	distribution.Status = aws.String(cloudFrontStatusDeployed)
	return &cloudfront.UpdateDistributionOutput{Distribution: distribution, ETag: aws.String(api.etags[id])}, nil
}

func (api *fakeCloudFrontAPI) DeleteDistribution(_ context.Context, input *cloudfront.DeleteDistributionInput) (*cloudfront.DeleteDistributionOutput, error) {
	id := aws.ToString(input.Id)
	distribution, ok := api.distributions[id]
	if !ok {
		return nil, &cloudfronttypes.NoSuchDistribution{}
	}
	if aws.ToBool(distribution.DistributionConfig.Enabled) {
		return nil, &cloudfronttypes.DistributionNotDisabled{}
	}
	api.deleteCalls++
	delete(api.distributions, id)
	return &cloudfront.DeleteDistributionOutput{}, nil
}

func (api *fakeCloudFrontAPI) CreateInvalidation(_ context.Context, input *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
	api.invalidationsCreated++
	batch := input.InvalidationBatch
	callerReference := aws.ToString(batch.CallerReference)
	if existing, ok := api.invalidations[callerReference]; ok {
		return &cloudfront.CreateInvalidationOutput{Invalidation: existing}, nil
	}
	id := "I1"
	api.invalidations[callerReference] = &cloudfronttypes.Invalidation{Id: aws.String(id), Status: aws.String(cloudFrontStatusCompleted), InvalidationBatch: batch}
	return &cloudfront.CreateInvalidationOutput{Invalidation: api.invalidations[callerReference]}, nil
}

func (api *fakeCloudFrontAPI) GetInvalidation(_ context.Context, input *cloudfront.GetInvalidationInput) (*cloudfront.GetInvalidationOutput, error) {
	for _, invalidation := range api.invalidations {
		if aws.ToString(invalidation.Id) == aws.ToString(input.Id) {
			return &cloudfront.GetInvalidationOutput{Invalidation: invalidation}, nil
		}
	}
	return nil, errors.New("invalidation not found")
}

type fakeCloudFrontHealth struct {
	calls         int
	refs          []string
	failReference string
	err           error
}

func (probe *fakeCloudFrontHealth) VerifyOrigin(_ context.Context, reference string) error {
	probe.calls++
	probe.refs = append(probe.refs, reference)
	if strings.TrimSpace(reference) == "" {
		return errors.New("empty health reference")
	}
	if reference == probe.failReference {
		if probe.err != nil {
			return probe.err
		}
		return errors.New("origin is unhealthy")
	}
	return probe.err
}

func cloudFrontTestRequest() sdk.EdgePlanRequest {
	return sdk.EdgePlanRequest{
		TargetProvider: "aws",
		TargetRuntime:  "ecs-fargate",
		Intent:         sdk.EdgeIntent{Mode: "native", NativeProvider: "cloudfront-waf", OriginHealthRef: "health/origin", OwnershipMarker: "magelift/test/cloudfront", Domains: []string{"shop.example.com"}, TLS: true, TLSMode: "managed", DNSMode: "external", CachePolicyRef: "cache/dynamic", PurgeOnDeploy: true, WAFPolicyRef: "waf/managed"},
		Configuration: map[string]any{
			"origin":        map[string]any{"kind": "load-balancer", "domainName": "alb.eu-west-1.elb.amazonaws.com", "id": "alb"},
			"cachePolicyID": "cache-policy-1", "certificateARN": "arn:aws:acm:us-east-1:123456789012:certificate/cert-1", "webACLID": "arn:aws:wafv2:us-east-1:123456789012:global/webacl/owned/acl-1",
		},
	}
}

func cloudFrontTestPolicy() sdk.EdgeOperationPolicy {
	return sdk.EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 4}
}

func cloudFrontOriginGroupRequest() sdk.EdgePlanRequest {
	request := cloudFrontTestRequest()
	request.Intent.PurgeOnDeploy = false
	request.Intent.FailoverPolicyRef = "policy/cloudfront-origin-failover"
	request.Intent.OriginHealthRef = "health/primary"
	request.Configuration["originGroup"] = map[string]any{
		"id": "magelift-origin-group",
		"primary": map[string]any{
			"kind": "load-balancer", "domainName": "primary.eu-west-1.elb.amazonaws.com", "id": "primary-origin",
		},
		"secondary": map[string]any{
			"kind": "load-balancer", "domainName": "secondary.eu-west-1.elb.amazonaws.com", "id": "secondary-origin",
		},
		"primaryHealthRef":   "health/primary",
		"secondaryHealthRef": "health/secondary",
		"statusCodes":        []any{float64(500), float64(502), float64(503), float64(504)},
	}
	delete(request.Configuration, "origin")
	return request
}

func TestCloudFrontPlanIsProviderTypedAndDoesNotMutate(t *testing.T) {
	api := newFakeCloudFrontAPI()
	health := &fakeCloudFrontHealth{}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		t.Fatal(err)
	}
	request := cloudFrontTestRequest()
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID != cloudFrontEdgeAdapterID || plan.OwnershipMarker != request.Intent.OwnershipMarker || api.createCalls != 0 || health.calls != 0 {
		t.Fatalf("plan = %#v, API = %#v, health calls = %d", plan, api, health.calls)
	}
	request.Intent.CachePolicyRef = "cache/ref"
	delete(request.Configuration, "cachePolicyID")
	if _, err := native.Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "cachePolicyRef") {
		t.Fatalf("missing cache policy error = %v", err)
	}
}

func TestCloudFrontPlanCoversCacheBypassTLSDNSWAFAndPurgePolicy(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*sdk.EdgePlanRequest)
		wantCache  string
		wantOrigin string
		wantPurge  []string
	}{
		{
			name: "explicit cache policy and purge paths",
			mutate: func(request *sdk.EdgePlanRequest) {
				request.Configuration["cachePolicyID"] = "cache-policy-explicit"
				request.Configuration["originRequestPolicyID"] = "origin-request-all"
				request.Configuration["purgePaths"] = []any{"/products/*", "/"}
			},
			wantCache: "cache-policy-explicit", wantOrigin: "origin-request-all", wantPurge: []string{"/", "/products/*"},
		},
		{
			name: "cache bypass uses managed disabled policy",
			mutate: func(request *sdk.EdgePlanRequest) {
				request.Intent.CachePolicyRef = ""
				request.Intent.PurgeOnDeploy = false
				delete(request.Configuration, "cachePolicyID")
			},
			wantCache: managedCachingDisabledPolicyID,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := cloudFrontTestRequest()
			test.mutate(&request)
			native, err := NewNativeAPI(newFakeCloudFrontAPI(), &fakeCloudFrontHealth{})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := native.Plan(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			config := plan.Opaque.(cloudFrontPlan).config
			if got := aws.ToString(config.DefaultCacheBehavior.CachePolicyId); got != test.wantCache {
				t.Errorf("cache policy = %q, want %q", got, test.wantCache)
			}
			if got := aws.ToString(config.DefaultCacheBehavior.OriginRequestPolicyId); got != test.wantOrigin {
				t.Errorf("origin request policy = %q, want %q", got, test.wantOrigin)
			}
			if got := plan.Opaque.(cloudFrontPlan).purgePaths; !reflect.DeepEqual(got, test.wantPurge) {
				t.Errorf("purge paths = %v, want %v", got, test.wantPurge)
			}
			if got := config.ViewerCertificate; got == nil || aws.ToString(got.ACMCertificateArn) != "arn:aws:acm:us-east-1:123456789012:certificate/cert-1" {
				t.Errorf("TLS certificate = %#v", got)
			}
			if got := config.WebACLId; aws.ToString(got) != "arn:aws:wafv2:us-east-1:123456789012:global/webacl/owned/acl-1" {
				t.Errorf("WAF ID = %q", aws.ToString(got))
			}
			if got := config.Aliases.Items; !reflect.DeepEqual(got, []string{"shop.example.com"}) {
				t.Errorf("DNS aliases = %v", got)
			}
			if got := config.Origins.Items[0].CustomOriginConfig.OriginProtocolPolicy; got != cloudfronttypes.OriginProtocolPolicyHttpsOnly {
				t.Errorf("origin protocol policy = %q", got)
			}
		})
	}
}

func TestCloudFrontS3OriginRequiresAndCarriesOriginAccessControl(t *testing.T) {
	request := cloudFrontTestRequest()
	request.Configuration["origin"] = map[string]any{
		"kind": "s3", "domainName": "shop-bucket.s3.eu-west-3.amazonaws.com", "id": "shop-bucket",
		"originAccessControlID": "oac-1",
	}
	native, err := NewNativeAPI(newFakeCloudFrontAPI(), &fakeCloudFrontHealth{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	origin := plan.Opaque.(cloudFrontPlan).config.Origins.Items[0]
	if aws.ToString(origin.OriginAccessControlId) != "oac-1" || origin.S3OriginConfig == nil {
		t.Fatalf("S3 origin authentication = %#v", origin)
	}

	delete(request.Configuration["origin"].(map[string]any), "originAccessControlID")
	if _, err := native.Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "originAccessControlID") {
		t.Fatalf("missing origin access control error = %v", err)
	}
}

func TestCloudFrontHealthGateBlocksEveryTrafficBearingActionBeforeMutation(t *testing.T) {
	api := newFakeCloudFrontAPI()
	health := &fakeCloudFrontHealth{err: errors.New("origin is unhealthy")}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), cloudFrontTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []sdk.EdgeAction{sdk.EdgeApply, sdk.EdgeVerify, sdk.EdgeFailover, sdk.EdgeRollback} {
		t.Run(string(action), func(t *testing.T) {
			_, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: action, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: action, IdempotencyKey: "health-gate-" + string(action), OwnershipMarker: plan.OwnershipMarker}})
			if err == nil || !strings.Contains(err.Error(), "origin health") {
				t.Fatalf("action error = %v, want origin-health gate", err)
			}
		})
	}
	if api.createCalls != 0 || api.updateCalls != 0 || api.invalidationsCreated != 0 {
		t.Fatalf("provider mutated before health gate: %#v", api)
	}
}

func TestCloudFrontFailoverHealthGateChecksBothOriginsBeforeMutation(t *testing.T) {
	api := newFakeCloudFrontAPI()
	health := &fakeCloudFrontHealth{failReference: "health/secondary"}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), cloudFrontOriginGroupRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeFailover, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "failover-unhealthy-secondary", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: []string{cloudFrontDistribution + "E1"}}})
	if err == nil || !strings.Contains(err.Error(), "origin health") {
		t.Fatalf("failover health error = %v, want health gate", err)
	}
	if !reflect.DeepEqual(health.refs, []string{"health/primary", "health/secondary"}) {
		t.Fatalf("health references = %v, want both planned origins", health.refs)
	}
	if api.createCalls != 0 || api.updateCalls != 0 {
		t.Fatalf("CloudFront mutated before both-origin health gate: %#v", api)
	}
}

func TestCloudFrontApplyPurgeAndDestroyUseSharedLifecycle(t *testing.T) {
	api := newFakeCloudFrontAPI()
	health := &fakeCloudFrontHealth{}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeLifecycleAdapter(native, cloudFrontTestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := cloudFrontTestRequest()
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-1", OwnershipMarker: request.Intent.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if api.createCalls != 1 || api.invalidationsCreated != 1 || health.calls != 1 || len(result.ResourceRefs) != 2 {
		t.Fatalf("apply result = %#v, API = %#v, health = %d", result, api, health.calls)
	}
	if !containsString(result.ProofRefs, "aws.cloudfront.purge-complete") || !containsString(result.ProofRefs, "aws.cloudfront.waf") {
		t.Fatalf("apply proof refs = %#v", result.ProofRefs)
	}
	destroy, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy-1", OwnershipMarker: request.Intent.OwnershipMarker, ResourceReferences: result.ResourceRefs})
	if err != nil {
		t.Fatal(err)
	}
	if destroy.Action != sdk.EdgeDestroy || api.updateCalls != 1 || api.deleteCalls != 1 {
		t.Fatalf("destroy result = %#v, API = %#v", destroy, api)
	}
	resources, err := native.Inventory(context.Background(), request.Intent.OwnershipMarker)
	if err != nil || len(resources) != 0 {
		t.Fatalf("post-destroy inventory = %#v, err = %v", resources, err)
	}
}

func TestCloudFrontOriginGroupSupportsFailoverAndRollback(t *testing.T) {
	api := newFakeCloudFrontAPI()
	native, err := NewNativeAPI(api, &fakeCloudFrontHealth{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeLifecycleAdapter(native, cloudFrontTestPolicy())
	if err != nil {
		t.Fatal(err)
	}
	request := cloudFrontOriginGroupRequest()
	plan, err := adapter.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	groupConfig := plan.Opaque.(cloudFrontPlan).config
	if got := groupConfig.DefaultCacheBehavior.AllowedMethods.Items; len(got) != 3 || got[0] != cloudfronttypes.MethodGet || got[1] != cloudfronttypes.MethodHead || got[2] != cloudfronttypes.MethodOptions {
		t.Fatalf("origin-group allowed methods = %v", got)
	}
	if descriptor := adapter.EdgeDescriptor(); !containsString(stringActions(descriptor.Capabilities), string(sdk.EdgeFailover)) || !containsString(stringActions(descriptor.Capabilities), string(sdk.EdgeRollback)) {
		t.Fatalf("descriptor does not expose origin-group actions: %#v", descriptor)
	}
	apply, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "group-apply", OwnershipMarker: request.Intent.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if len(apply.ResourceRefs) != 1 {
		t.Fatalf("origin-group apply refs = %#v", apply.ResourceRefs)
	}
	distribution := api.distributions["E1"]
	if distribution == nil || distribution.DistributionConfig == nil || distribution.DistributionConfig.OriginGroups == nil {
		t.Fatalf("origin group was not configured: %#v", distribution)
	}
	group := distribution.DistributionConfig.OriginGroups.Items[0]
	if got := aws.ToString(group.Members.Items[0].OriginId); got != "primary-origin" {
		t.Fatalf("initial primary origin = %q", got)
	}
	failover, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "group-failover", OwnershipMarker: request.Intent.OwnershipMarker, ResourceReferences: apply.ResourceRefs})
	if err != nil {
		t.Fatal(err)
	}
	group = api.distributions["E1"].DistributionConfig.OriginGroups.Items[0]
	if got := aws.ToString(group.Members.Items[0].OriginId); got != "secondary-origin" {
		t.Fatalf("failover primary origin = %q", got)
	}
	if !containsString(failover.ProofRefs, "aws.cloudfront.failover-control-plane-converged") {
		t.Fatalf("failover proof refs = %#v", failover.ProofRefs)
	}
	rollback, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeRollback, IdempotencyKey: "group-rollback", OwnershipMarker: request.Intent.OwnershipMarker, ResourceReferences: apply.ResourceRefs})
	if err != nil {
		t.Fatal(err)
	}
	group = api.distributions["E1"].DistributionConfig.OriginGroups.Items[0]
	if got := aws.ToString(group.Members.Items[0].OriginId); got != "primary-origin" {
		t.Fatalf("rollback primary origin = %q", got)
	}
	if !containsString(rollback.ProofRefs, "aws.cloudfront.rollback-control-plane-converged") {
		t.Fatalf("rollback proof refs = %#v", rollback.ProofRefs)
	}
}

func TestCloudFrontRefusesUnownedFailoverBeforeMutation(t *testing.T) {
	api := newFakeCloudFrontAPI()
	native, err := NewNativeAPI(api, &fakeCloudFrontHealth{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), cloudFrontOriginGroupRequest())
	if err != nil {
		t.Fatal(err)
	}
	planned := plan.Opaque.(cloudFrontPlan).config
	foreignConfig := *planned
	foreignConfig.Comment = aws.String("customer-owned")
	api.distributions["user-1"] = &cloudfronttypes.Distribution{Id: aws.String("user-1"), Status: aws.String(cloudFrontStatusDeployed), DistributionConfig: &foreignConfig}
	api.etags["user-1"] = "etag-user"
	_, err = native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeFailover, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "failover-user", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: []string{cloudFrontDistribution + "user-1"}}})
	if err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("unowned failover error = %v", err)
	}
	if api.updateCalls != 0 {
		t.Fatalf("unowned distribution was mutated: %#v", api)
	}
}

func TestCloudFrontRefusesOriginGroupDriftBeforeRollbackMutation(t *testing.T) {
	api := newFakeCloudFrontAPI()
	native, err := NewNativeAPI(api, &fakeCloudFrontHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := cloudFrontOriginGroupRequest()
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	apply, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "group-apply-drift", OwnershipMarker: plan.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}
	if apply.Status != sdk.EdgeOperationPending {
		t.Fatalf("apply observation = %#v", apply)
	}
	if _, err := native.Poll(context.Background(), apply.OperationID); err != nil {
		t.Fatal(err)
	}
	api.distributions["E1"].DistributionConfig.OriginGroups.Items[0].FailoverCriteria.StatusCodes.Items[0] = 501
	_, err = native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeRollback, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeRollback, IdempotencyKey: "rollback-drift", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: []string{cloudFrontDistribution + "E1"}}})
	if err == nil || !strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("drifted rollback error = %v", err)
	}
	if api.updateCalls != 0 {
		t.Fatalf("drifted origin group was mutated: %#v", api)
	}
}

func TestCloudFrontTransitionPollRejectsStaleOriginOrder(t *testing.T) {
	api := newFakeCloudFrontAPI()
	native, err := NewNativeAPI(api, &fakeCloudFrontHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := cloudFrontOriginGroupRequest()
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	apply, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "group-apply-stale", OwnershipMarker: plan.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.Poll(context.Background(), apply.OperationID); err != nil {
		t.Fatal(err)
	}
	applyRefs := []string{cloudFrontDistribution + "E1"}
	transition, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeFailover, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "failover-stale", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: applyRefs}})
	if err != nil {
		t.Fatal(err)
	}
	group := api.distributions["E1"].DistributionConfig.OriginGroups.Items[0]
	group.Members.Items[0].OriginId = aws.String("primary-origin")
	group.Members.Items[1].OriginId = aws.String("secondary-origin")
	group.Members.Quantity = aws.Int32(2)
	api.distributions["E1"].DistributionConfig.DefaultCacheBehavior.TargetOriginId = aws.String("magelift-origin-group")
	observation, err := native.Poll(context.Background(), transition.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Status != sdk.EdgeOperationFailed || !strings.Contains(observation.Detail, "requested origin order") {
		t.Fatalf("stale transition observation = %#v", observation)
	}
}

func TestCloudFrontRefusesUnownedDestroy(t *testing.T) {
	api := newFakeCloudFrontAPI()
	api.distributions["user-1"] = &cloudfronttypes.Distribution{Id: aws.String("user-1"), Status: aws.String(cloudFrontStatusDeployed), DistributionConfig: &cloudfronttypes.DistributionConfig{Comment: aws.String("user-owned"), Enabled: aws.Bool(true)}}
	api.etags["user-1"] = "etag-user"
	native, err := NewNativeAPI(api, &fakeCloudFrontHealth{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), cloudFrontTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "aws", Action: sdk.EdgeDestroy, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy-user", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: []string{cloudFrontDistribution + "user-1"}}})
	if err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("unowned destroy error = %v", err)
	}
	if api.updateCalls != 0 || api.deleteCalls != 0 {
		t.Fatalf("unowned resource was mutated: %#v", api)
	}
}

func TestDistributionMatchesPlanDetectsAliasAndCertificateDrift(t *testing.T) {
	api := newFakeCloudFrontAPI()
	health := &fakeCloudFrontHealth{}
	native, err := NewNativeAPI(api, health)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), cloudFrontTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	desired := plan.Opaque.(cloudFrontPlan).config
	current := cloneDistributionConfig(desired)
	current.Comment = aws.String("magelift ownership=" + plan.OwnershipMarker)
	if !distributionMatchesPlan(&cloudfronttypes.Distribution{DistributionConfig: current}, desired) {
		t.Fatal("identical CloudFront configuration was treated as drift")
	}

	current.Aliases.Items[0] = "other.example.com"
	if distributionMatchesPlan(&cloudfronttypes.Distribution{DistributionConfig: current}, desired) {
		t.Fatal("alias drift was not detected")
	}

	current = cloneDistributionConfig(desired)
	current.Comment = aws.String("magelift ownership=" + plan.OwnershipMarker)
	current.ViewerCertificate.ACMCertificateArn = aws.String("arn:aws:acm:us-east-1:123456789012:certificate/other")
	if distributionMatchesPlan(&cloudfronttypes.Distribution{DistributionConfig: current}, desired) {
		t.Fatal("viewer certificate drift was not detected")
	}
}

func cloneDistributionConfig(config *cloudfronttypes.DistributionConfig) *cloudfronttypes.DistributionConfig {
	if config == nil {
		return nil
	}
	clone := *config
	if config.Aliases != nil {
		aliases := *config.Aliases
		aliases.Items = append([]string(nil), config.Aliases.Items...)
		clone.Aliases = &aliases
	}
	if config.ViewerCertificate != nil {
		certificate := *config.ViewerCertificate
		clone.ViewerCertificate = &certificate
	}
	if config.Origins != nil {
		origins := *config.Origins
		origins.Items = append([]cloudfronttypes.Origin(nil), config.Origins.Items...)
		clone.Origins = &origins
	}
	if config.DefaultCacheBehavior != nil {
		behavior := *config.DefaultCacheBehavior
		if config.DefaultCacheBehavior.AllowedMethods != nil {
			allowed := *config.DefaultCacheBehavior.AllowedMethods
			if allowed.CachedMethods != nil {
				cached := *allowed.CachedMethods
				cached.Items = append([]cloudfronttypes.Method(nil), allowed.CachedMethods.Items...)
				allowed.CachedMethods = &cached
			}
			allowed.Items = append([]cloudfronttypes.Method(nil), config.DefaultCacheBehavior.AllowedMethods.Items...)
			behavior.AllowedMethods = &allowed
		}
		clone.DefaultCacheBehavior = &behavior
	}
	if config.OriginGroups != nil {
		groups := *config.OriginGroups
		groups.Items = append([]cloudfronttypes.OriginGroup(nil), config.OriginGroups.Items...)
		for index, group := range groups.Items {
			if group.Members != nil {
				members := *group.Members
				members.Items = append([]cloudfronttypes.OriginGroupMember(nil), group.Members.Items...)
				groups.Items[index].Members = &members
			}
			if group.FailoverCriteria != nil {
				criteria := *group.FailoverCriteria
				if group.FailoverCriteria.StatusCodes != nil {
					statusCodes := *group.FailoverCriteria.StatusCodes
					statusCodes.Items = append([]int32(nil), group.FailoverCriteria.StatusCodes.Items...)
					criteria.StatusCodes = &statusCodes
				}
				groups.Items[index].FailoverCriteria = &criteria
			}
		}
		clone.OriginGroups = &groups
	}
	if config.Restrictions != nil && config.Restrictions.GeoRestriction != nil {
		restrictions := *config.Restrictions
		geo := *config.Restrictions.GeoRestriction
		geo.Items = append([]string(nil), config.Restrictions.GeoRestriction.Items...)
		restrictions.GeoRestriction = &geo
		clone.Restrictions = &restrictions
	}
	return &clone
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringActions(values []sdk.EdgeAction) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, string(value))
	}
	return result
}
