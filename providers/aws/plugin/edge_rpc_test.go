package plugin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/providerhost"
	awsedge "github.com/magelift/magelift/providers/aws/edge"
	"github.com/magelift/magelift/sdk"
)

type awsEdgeRPCCaller struct{ server *Server }

func (c awsEdgeRPCCaller) Call(method string, args any, reply any) error {
	switch method {
	case "Plugin.Plan":
		return c.server.Plan(args.(*sdk.PlanRequest), reply.(*sdk.PlanResult))
	case "Plugin.EdgePlan":
		return c.server.EdgePlan(args.(*sdk.EdgePlanCall), reply.(*sdk.EdgePlanResult))
	case "Plugin.EdgeExecute":
		return c.server.EdgeExecute(args.(*sdk.EdgeExecuteCall), reply.(*sdk.EdgeExecuteResult))
	default:
		return errors.New("unexpected AWS RPC method " + method)
	}
}

type fakeRPCCloudFront struct {
	distribution         *cloudfronttypes.Distribution
	invalidationsCreated int
}

func (fakeRPCCloudFront) ListDistributions(context.Context, *cloudfront.ListDistributionsInput) (*cloudfront.ListDistributionsOutput, error) {
	return nil, errors.New("unexpected ListDistributions")
}
func (fakeRPCCloudFront) CreateDistribution(context.Context, *cloudfront.CreateDistributionInput) (*cloudfront.CreateDistributionOutput, error) {
	return nil, errors.New("unexpected CreateDistribution")
}
func (api *fakeRPCCloudFront) GetDistribution(context.Context, *cloudfront.GetDistributionInput) (*cloudfront.GetDistributionOutput, error) {
	return &cloudfront.GetDistributionOutput{Distribution: api.distribution}, nil
}
func (fakeRPCCloudFront) GetDistributionConfig(context.Context, *cloudfront.GetDistributionConfigInput) (*cloudfront.GetDistributionConfigOutput, error) {
	return nil, errors.New("unexpected GetDistributionConfig")
}
func (fakeRPCCloudFront) UpdateDistribution(context.Context, *cloudfront.UpdateDistributionInput) (*cloudfront.UpdateDistributionOutput, error) {
	return nil, errors.New("unexpected UpdateDistribution")
}
func (fakeRPCCloudFront) DeleteDistribution(context.Context, *cloudfront.DeleteDistributionInput) (*cloudfront.DeleteDistributionOutput, error) {
	return nil, errors.New("unexpected DeleteDistribution")
}
func (api *fakeRPCCloudFront) CreateInvalidation(_ context.Context, request *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
	api.invalidationsCreated++
	if request == nil || aws.ToString(request.DistributionId) != aws.ToString(api.distribution.Id) || request.InvalidationBatch == nil || request.InvalidationBatch.Paths == nil || len(request.InvalidationBatch.Paths.Items) != 1 || request.InvalidationBatch.Paths.Items[0] != "/*" {
		return nil, errors.New("invalidation request did not target the existing Magento distribution")
	}
	return &cloudfront.CreateInvalidationOutput{Invalidation: &cloudfronttypes.Invalidation{Id: aws.String("I123"), Status: aws.String("Completed")}}, nil
}
func (fakeRPCCloudFront) GetInvalidation(context.Context, *cloudfront.GetInvalidationInput) (*cloudfront.GetInvalidationOutput, error) {
	return nil, errors.New("unexpected GetInvalidation")
}

func TestAWSEdgePurgeOpaquePlanRoundTripsThroughProviderRPC(t *testing.T) {
	request := shopRequest(t, "ecs-fargate")
	target, err := parseTarget(request.TargetBlock)
	if err != nil {
		t.Fatal(err)
	}
	cfg := projectConfig(request, target)
	marker := platform.EdgeOwnershipMarker(cfg, request.Envelope.Environment)
	cloudFrontAPI := &fakeRPCCloudFront{distribution: &cloudfronttypes.Distribution{
		Id: aws.String("E123"), DomainName: aws.String("d123.cloudfront.net"), Status: aws.String("Deployed"),
		DistributionConfig: &cloudfronttypes.DistributionConfig{Comment: aws.String("magelift ownership=" + marker), Enabled: aws.Bool(true)},
	}}
	native, err := awsedge.NewNativeAPI(cloudFrontAPI, awsedge.AlwaysHealthy{})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := awsedge.NewNativeLifecycleAdapter(native, sdk.EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 4})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{NewEdgeAdapter: func(context.Context, platform.StackModule, platform.PlannedStack) (sdk.EdgeAdapter, error) {
		return adapter, nil
	}}
	descriptor := awsedge.Descriptor()
	describe := &sdk.DescribeResponse{
		ProtocolVersion: sdk.ProtocolV1, ProviderID: "aws", ProviderVersion: "test", Edge: &descriptor,
		Operations: []sdk.OperationVersion{{Name: string(sdk.OpPlan), Version: sdk.ProtocolV1}, {Name: string(sdk.OpEdgePlan), Version: sdk.ProtocolV1}, {Name: string(sdk.OpEdgeExecute), Version: sdk.ProtocolV1}},
	}
	client := providerhost.NewTestClient(awsEdgeRPCCaller{server: server}, describe)
	shim, err := providerhost.NewShimModule("aws", "ecs-fargate", client)
	if err != nil {
		t.Fatal(err)
	}
	planned, err := shim.Plan(cfg, request.Envelope.Environment, platform.PlanOptions{Context: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	edgeAdapter, err := platform.ModuleEdgeFor(t.Context(), shim, planned)
	if err != nil {
		t.Fatal(err)
	}
	intent, err := platform.EdgeIntentFromConfig(cfg, request.Envelope.Environment)
	if err != nil {
		t.Fatal(err)
	}
	_, result, err := sdk.RunEdgeLifecycle(t.Context(), edgeAdapter,
		sdk.EdgePlanRequest{TargetProvider: "aws", TargetRuntime: "ecs-fargate", Intent: intent, DeployedOutputs: map[string]string{"edgeDistributionId": "E123"}},
		sdk.EdgePurge, "purge/staging/unique", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID == "" || cloudFrontAPI.invalidationsCreated != 1 {
		t.Fatalf("purge result=%#v invalidations=%d", result, cloudFrontAPI.invalidationsCreated)
	}
}
