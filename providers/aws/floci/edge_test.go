//go:build floci

package floci_test

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

func TestAWSPluginPurgesOnlyItsCloudFrontDistribution(t *testing.T) {
	endpoint := flociEndpoint(t)
	ctx, cancel := context.WithTimeout(t.Context(), 75*time.Second)
	defer cancel()
	plugin := awsFlociPlugin(t, endpoint, sdk.OpPlan, sdk.OpEdgePlan, sdk.OpEdgeExecute)
	module, err := providerhost.NewLazyProviderShimModule("aws", "ecs-fargate", platform.TierBuiltIn, nil, plugin.Do)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		Project: config.Project{Name: "purgetest"}, Account: "000000000000", Class: "staging",
		Defaults: config.Defaults{Preset: "standard", Region: "us-east-1"},
		Target: config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &config.AWSTarget{
			KMSKeyARN: "arn:aws:kms:us-east-1:000000000000:key/00000000-0000-0000-0000-000000000000",
		}},
		Edge: config.EdgeConfig{
			Mode: "native", NativeProvider: "cloudfront", Domains: []string{"shop.example.test"},
			TLS: true, TLSMode: "managed", DNSMode: "external", OriginHealthRef: "health/shop",
		},
	}
	stack, err := module.Plan(cfg, "staging", platform.PlanOptions{Context: ctx, AccountOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := platform.ModuleEdgeFor(ctx, module, stack)
	if err != nil || adapter == nil {
		t.Fatalf("open native edge adapter: %v", err)
	}
	intent, err := platform.EdgeIntentFromConfig(cfg, "staging")
	if err != nil {
		t.Fatal(err)
	}
	cloud := cloudfront.NewFromConfig(flociConfig(t, ctx, endpoint))
	other := createFlociDistribution(t, cloud, "other-shop", "magelift/edge/other-shop")
	ours := createFlociDistribution(t, cloud, "this-shop", intent.OwnershipMarker)
	planPurge := func(id string) sdk.EdgeExecutionRequest {
		t.Helper()
		plan, err := adapter.PlanEdge(ctx, sdk.EdgePlanRequest{
			TargetProvider: stack.Provider(), TargetRuntime: stack.Runtime(), Intent: intent,
			DeployedOutputs: map[string]string{"edgeDistributionId": id},
		})
		if err != nil {
			t.Fatal(err)
		}
		return sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgePurge, IdempotencyKey: "this-purge", OwnershipMarker: intent.OwnershipMarker}
	}
	request := planPurge(other)
	if _, err := adapter.ExecuteEdge(ctx, request); err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("expected to reject the other shop's distribution, got %v", err)
	}
	list, err := cloud.ListInvalidations(ctx, &cloudfront.ListInvalidationsInput{DistributionId: aws.String(other)})
	if err != nil {
		t.Fatal(err)
	}
	if list.InvalidationList == nil || len(list.InvalidationList.Items) != 0 {
		t.Fatalf("unexpected invalidations on the other distribution: %v", list.InvalidationList)
	}

	request = planPurge(ours)
	request.ResourceReferences = []string{"aws.cloudfront.distribution:" + other}
	if _, err := adapter.ExecuteEdge(ctx, request); err == nil || !strings.Contains(err.Error(), "differs from the planned") {
		t.Fatalf("a different distribution was substituted after planning: %v", err)
	}
	request.ResourceReferences = nil
	result, err := adapter.ExecuteEdge(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OwnershipVerified || result.OwnershipMarker != intent.OwnershipMarker || !slices.Contains(result.ProofRefs, "aws.cloudfront.purge-complete") {
		t.Fatalf("incomplete purge result: %+v", result)
	}
	list, err = cloud.ListInvalidations(ctx, &cloudfront.ListInvalidationsInput{DistributionId: aws.String(ours)})
	if err != nil {
		t.Fatal(err)
	}
	if list.InvalidationList == nil || len(list.InvalidationList.Items) != 1 {
		t.Fatalf("expected one CloudFront invalidation, got %v", list.InvalidationList)
	}
	id := list.InvalidationList.Items[0].Id
	get, err := cloud.GetInvalidation(ctx, &cloudfront.GetInvalidationInput{DistributionId: aws.String(ours), Id: id})
	if err != nil {
		t.Fatal(err)
	}
	invalidation := get.Invalidation
	if invalidation == nil || aws.ToString(invalidation.Status) != "Completed" || invalidation.InvalidationBatch == nil ||
		invalidation.InvalidationBatch.Paths == nil || !slices.Equal(invalidation.InvalidationBatch.Paths.Items, []string{"/*"}) {
		t.Fatalf("unexpected CloudFront invalidation: %+v", invalidation)
	}
}

func createFlociDistribution(t *testing.T, cloud *cloudfront.Client, caller, marker string) string {
	t.Helper()
	origin := aws.String("shop-origin")
	resp, err := cloud.CreateDistribution(t.Context(), &cloudfront.CreateDistributionInput{DistributionConfig: &types.DistributionConfig{
		CallerReference: aws.String(caller), Comment: aws.String("magelift ownership=" + marker), Enabled: aws.Bool(true),
		Origins: &types.Origins{Quantity: aws.Int32(1), Items: []types.Origin{{
			Id: origin, DomainName: aws.String("origin.example.test"), CustomOriginConfig: &types.CustomOriginConfig{
				HTTPPort: aws.Int32(80), HTTPSPort: aws.Int32(443), OriginProtocolPolicy: types.OriginProtocolPolicyHttpsOnly,
			},
		}}},
		DefaultCacheBehavior: &types.DefaultCacheBehavior{TargetOriginId: origin, ViewerProtocolPolicy: types.ViewerProtocolPolicyRedirectToHttps,
			CachePolicyId: aws.String("4135ea2d-6df8-44a3-9df3-4b5a84be39ad")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Distribution == nil || aws.ToString(resp.Distribution.Id) == "" {
		t.Fatal("Floci returned a distribution without an ID")
	}
	return *resp.Distribution.Id
}
