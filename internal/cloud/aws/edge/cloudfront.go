package edge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	cloudfronttypes "github.com/aws/aws-sdk-go-v2/service/cloudfront/types"
	"github.com/magelift/magelift/sdk"
)

const (
	cloudFrontEdgeAdapterID        = "aws.edge.native"
	cloudFrontDistribution         = "aws.cloudfront.distribution:"
	cloudFrontOperation            = "aws.cloudfront"
	cloudFrontCommentPrefix        = "magelift ownership="
	cloudFrontStatusDeployed       = "Deployed"
	cloudFrontStatusCompleted      = "Completed"
	cloudFrontStatusInProgress     = "InProgress"
	managedCachingDisabledPolicyID = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
)

// CloudFrontAPI is the small provider-owned subset of the official AWS SDK
// used by the lifecycle translator. AWS request and response types do not
// cross this package boundary.
type CloudFrontAPI interface {
	ListDistributions(context.Context, *cloudfront.ListDistributionsInput) (*cloudfront.ListDistributionsOutput, error)
	CreateDistribution(context.Context, *cloudfront.CreateDistributionInput) (*cloudfront.CreateDistributionOutput, error)
	GetDistribution(context.Context, *cloudfront.GetDistributionInput) (*cloudfront.GetDistributionOutput, error)
	GetDistributionConfig(context.Context, *cloudfront.GetDistributionConfigInput) (*cloudfront.GetDistributionConfigOutput, error)
	UpdateDistribution(context.Context, *cloudfront.UpdateDistributionInput) (*cloudfront.UpdateDistributionOutput, error)
	DeleteDistribution(context.Context, *cloudfront.DeleteDistributionInput) (*cloudfront.DeleteDistributionOutput, error)
	CreateInvalidation(context.Context, *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error)
	GetInvalidation(context.Context, *cloudfront.GetInvalidationInput) (*cloudfront.GetInvalidationOutput, error)
}

type sdkCloudFrontAPI struct{ client *cloudfront.Client }

func (api sdkCloudFrontAPI) ListDistributions(ctx context.Context, input *cloudfront.ListDistributionsInput) (*cloudfront.ListDistributionsOutput, error) {
	return api.client.ListDistributions(ctx, input)
}

func (api sdkCloudFrontAPI) CreateDistribution(ctx context.Context, input *cloudfront.CreateDistributionInput) (*cloudfront.CreateDistributionOutput, error) {
	return api.client.CreateDistribution(ctx, input)
}

func (api sdkCloudFrontAPI) GetDistribution(ctx context.Context, input *cloudfront.GetDistributionInput) (*cloudfront.GetDistributionOutput, error) {
	return api.client.GetDistribution(ctx, input)
}

func (api sdkCloudFrontAPI) GetDistributionConfig(ctx context.Context, input *cloudfront.GetDistributionConfigInput) (*cloudfront.GetDistributionConfigOutput, error) {
	return api.client.GetDistributionConfig(ctx, input)
}

func (api sdkCloudFrontAPI) UpdateDistribution(ctx context.Context, input *cloudfront.UpdateDistributionInput) (*cloudfront.UpdateDistributionOutput, error) {
	return api.client.UpdateDistribution(ctx, input)
}

func (api sdkCloudFrontAPI) DeleteDistribution(ctx context.Context, input *cloudfront.DeleteDistributionInput) (*cloudfront.DeleteDistributionOutput, error) {
	return api.client.DeleteDistribution(ctx, input)
}

func (api sdkCloudFrontAPI) CreateInvalidation(ctx context.Context, input *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
	return api.client.CreateInvalidation(ctx, input)
}

func (api sdkCloudFrontAPI) GetInvalidation(ctx context.Context, input *cloudfront.GetInvalidationInput) (*cloudfront.GetInvalidationOutput, error) {
	return api.client.GetInvalidation(ctx, input)
}

// OriginHealthProbe is injected by the runtime that owns application health.
// A non-empty origin reference is configuration, not proof that traffic is
// safe; apply and verify call this probe before touching CloudFront.
type OriginHealthProbe interface {
	VerifyOrigin(context.Context, string) error
}

// AlwaysHealthy is an origin probe for purge-only and test adapters where
// Magento origin health is not part of the requested action.
type AlwaysHealthy struct{}

func (AlwaysHealthy) VerifyOrigin(context.Context, string) error { return nil }

// NativeAPI translates the provider-neutral edge lifecycle to CloudFront.
// Provider credentials, ACM/WAF/OAC identities, and CloudFront API models
// remain inside this package.
type NativeAPI struct {
	api    CloudFrontAPI
	health OriginHealthProbe
}

// NewNativeAPI constructs a CloudFront translator around an injected AWS SDK
// client. The injection point keeps fake-client tests deterministic and makes
// community implementations possible without changing the core SDK.
func NewNativeAPI(api CloudFrontAPI, health OriginHealthProbe) (*NativeAPI, error) {
	if api == nil {
		return nil, errors.New("CloudFront API is required")
	}
	if health == nil {
		return nil, errors.New("CloudFront origin health probe is required")
	}
	return &NativeAPI{api: api, health: health}, nil
}

// NewCloudFrontSDKClient loads the AWS SDK configuration and constructs the
// provider translator. CloudFront is a global service; us-east-1 is used when
// no region was selected because ACM certificates for aliases must also live
// in that region.
func NewCloudFrontSDKClient(ctx context.Context, health OriginHealthProbe, opts ...func(*awsconfig.LoadOptions) error) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("CloudFront context is required")
	}
	config, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS SDK configuration: %w", err)
	}
	if strings.TrimSpace(config.Region) == "" {
		config.Region = "us-east-1"
	}
	return NewNativeAPI(sdkCloudFrontAPI{client: cloudfront.NewFromConfig(config)}, health)
}

type cloudFrontPlan struct {
	config           *cloudfronttypes.DistributionConfig
	originHealthRefs []string
	purgeOnDeploy    bool
	purgePaths       []string
	ownershipMarker  string
}

type cloudFrontOrigin struct {
	kind                  string
	domainName            string
	originID              string
	originAccessControlID string
	originPath            string
}

type cloudFrontOriginSet struct {
	origins             []cloudFrontOrigin
	groupID             string
	failoverStatusCodes []int32
	healthRefs          []string
}

func (origins cloudFrontOriginSet) hasFailover() bool {
	return origins.groupID != ""
}

func (api *NativeAPI) Plan(ctx context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if api == nil || api.api == nil {
		return sdk.EdgePlan{}, errors.New("CloudFront API is required")
	}
	if ctx == nil {
		return sdk.EdgePlan{}, errors.New("CloudFront planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.TargetProvider != "aws" {
		return sdk.EdgePlan{}, fmt.Errorf("CloudFront target provider must be aws, got %q", request.TargetProvider)
	}
	if err := sdk.ValidateEdgePlanRequest(request); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.Intent.Mode == "none" {
		return sdk.EdgePlan{AdapterID: cloudFrontEdgeAdapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime}, nil
	}
	if request.Intent.Mode != "native" && request.Intent.Mode != "both" {
		return sdk.EdgePlan{}, fmt.Errorf("CloudFront adapter cannot handle edge mode %q", request.Intent.Mode)
	}
	if request.Intent.NativeProvider != "cloudfront" && request.Intent.NativeProvider != "cloudfront-waf" {
		return sdk.EdgePlan{}, fmt.Errorf("CloudFront adapter requires native provider cloudfront or cloudfront-waf, got %q", request.Intent.NativeProvider)
	}
	if len(request.Intent.Domains) == 0 {
		return sdk.EdgePlan{}, errors.New("CloudFront requires at least one alternate domain")
	}
	if len(request.Intent.Domains) > 100 {
		return sdk.EdgePlan{}, errors.New("CloudFront supports at most 100 alternate domains per distribution plan")
	}
	if !request.Intent.TLS {
		return sdk.EdgePlan{}, errors.New("CloudFront alternate domains require TLS")
	}
	marker := strings.TrimSpace(request.Intent.OwnershipMarker)
	comment := cloudFrontCommentPrefix + marker
	if len(comment) > 128 {
		return sdk.EdgePlan{}, errors.New("CloudFront ownership comment exceeds the 128-byte service limit")
	}
	origins, err := parseCloudFrontOriginSet(request)
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("parse CloudFront origins: %w", err)
	}
	config, err := buildDistributionConfig(request, origins, comment)
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("build CloudFront distribution configuration: %w", err)
	}
	purgePaths, err := parsePurgePaths(request.Configuration, request.Intent.PurgeOnDeploy)
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("parse CloudFront purge policy: %w", err)
	}
	plan := sdk.EdgePlan{
		AdapterID: cloudFrontEdgeAdapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime,
		OwnershipMarker: marker,
		Outputs:         []sdk.AdapterOutput{{Key: "distributionId"}, {Key: "distributionDomainName"}, {Key: "distributionStatus"}},
		Opaque:          cloudFrontPlan{config: config, originHealthRefs: origins.healthRefs, purgeOnDeploy: request.Intent.PurgeOnDeploy, purgePaths: purgePaths, ownershipMarker: marker},
	}
	if request.Intent.WAFPolicyRef != "" {
		plan.Outputs = append(plan.Outputs, sdk.AdapterOutput{Key: "webACLId"})
	}
	if request.Intent.PurgeOnDeploy {
		plan.Outputs = append(plan.Outputs, sdk.AdapterOutput{Key: "invalidationId"})
	}
	return plan, nil
}

func (api *NativeAPI) Start(ctx context.Context, operation sdk.EdgeOperationRequest) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront execution context is required")
	}
	if operation.Provider != "aws" {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("CloudFront operation provider must be aws, got %q", operation.Provider)
	}
	plan, ok := operation.Request.Plan.Opaque.(cloudFrontPlan)
	if !ok {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront operation plan is missing provider configuration")
	}
	if plan.ownershipMarker != operation.Request.OwnershipMarker {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront operation ownership marker does not match plan")
	}
	if operation.Action == sdk.EdgeApply || operation.Action == sdk.EdgeVerify || operation.Action == sdk.EdgeFailover || operation.Action == sdk.EdgeRollback {
		for _, healthRef := range plan.originHealthRefs {
			if err := api.health.VerifyOrigin(ctx, healthRef); err != nil {
				return sdk.EdgeOperationObservation{}, fmt.Errorf("verify CloudFront origin health before mutation: %w", err)
			}
		}
	}
	switch operation.Action {
	case sdk.EdgeApply:
		return api.startApply(ctx, operation, plan)
	case sdk.EdgeVerify:
		id, err := distributionIDFromReferences(operation.Request.ResourceReferences)
		if err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		return pendingObservation(sdk.EdgeVerify, distributionOperationID(sdk.EdgeVerify, id, false, "", nil), operation.Request.OwnershipMarker), nil
	case sdk.EdgeDestroy:
		return api.startDestroy(ctx, operation)
	case sdk.EdgeFailover:
		return api.startOriginTransition(ctx, operation, plan, true)
	case sdk.EdgeRollback:
		return api.startOriginTransition(ctx, operation, plan, false)
	case sdk.EdgePurge:
		return api.startPurge(ctx, operation, plan)
	default:
		return sdk.EdgeOperationObservation{}, fmt.Errorf("unsupported CloudFront edge action %q", operation.Action)
	}
}

func (api *NativeAPI) startPurge(_ context.Context, operation sdk.EdgeOperationRequest, plan cloudFrontPlan) (sdk.EdgeOperationObservation, error) {
	id, err := distributionIDFromReferences(operation.Request.ResourceReferences)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	paths := plan.purgePaths
	if len(paths) == 0 {
		paths = []string{"/*"}
	}
	return pendingObservation(sdk.EdgePurge, distributionOperationID(sdk.EdgePurge, id, true, operation.Request.IdempotencyKey, paths), operation.Request.OwnershipMarker), nil
}

func (api *NativeAPI) startOriginTransition(ctx context.Context, operation sdk.EdgeOperationRequest, plan cloudFrontPlan, failover bool) (sdk.EdgeOperationObservation, error) {
	if plan.config == nil || plan.config.OriginGroups == nil || len(plan.config.OriginGroups.Items) != 1 {
		return sdk.EdgeOperationObservation{}, sdk.EdgeCapabilityError{AdapterID: cloudFrontEdgeAdapterID, Action: operation.Action, Status: sdk.EdgeCapabilityUnsupported, Reason: "CloudFront failover and rollback require a two-origin group"}
	}
	id, err := distributionIDFromReferences(operation.Request.ResourceReferences)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	distribution, err := api.api.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(id)})
	if err != nil {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("read CloudFront distribution for %s: %w", operation.Action, err)
	}
	if distribution == nil || distribution.Distribution == nil || !ownedDistributionConfig(distribution.Distribution.DistributionConfig, operation.Request.OwnershipMarker) {
		return sdk.EdgeOperationObservation{}, errors.New("refusing to transition an unowned CloudFront distribution")
	}
	configOutput, err := api.api.GetDistributionConfig(ctx, &cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
	if err != nil {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("read CloudFront configuration for %s: %w", operation.Action, err)
	}
	if configOutput == nil || configOutput.DistributionConfig == nil || strings.TrimSpace(aws.ToString(configOutput.ETag)) == "" {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront transition requires the current configuration and ETag")
	}
	if !originGroupTopologyMatch(configOutput.DistributionConfig.OriginGroups, plan.config.OriginGroups) {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront origin group no longer matches the planned failover boundary")
	}
	next, err := transitionOriginGroup(configOutput.DistributionConfig, plan.config.OriginGroups, failover)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	if !originGroupsMatch(configOutput.DistributionConfig.OriginGroups, next.OriginGroups) || aws.ToString(configOutput.DistributionConfig.DefaultCacheBehavior.TargetOriginId) != aws.ToString(next.DefaultCacheBehavior.TargetOriginId) {
		if _, err := api.api.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{Id: aws.String(id), IfMatch: configOutput.ETag, DistributionConfig: next}); err != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("update CloudFront origin group for %s: %w", operation.Action, err)
		}
	}
	operationID, err := transitionOperationID(operation.Action, id, next.OriginGroups)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	return pendingObservation(operation.Action, operationID, operation.Request.OwnershipMarker), nil
}

func (api *NativeAPI) startApply(ctx context.Context, operation sdk.EdgeOperationRequest, plan cloudFrontPlan) (sdk.EdgeOperationObservation, error) {
	distribution, err := api.distributionForRequest(ctx, operation.Request.ResourceReferences, plan.ownershipMarker)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	if distribution == nil {
		created, createErr := api.api.CreateDistribution(ctx, &cloudfront.CreateDistributionInput{DistributionConfig: plan.config})
		if createErr != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("create CloudFront distribution: %w", createErr)
		}
		distribution = createdDistribution(created)
		if distribution == nil || aws.ToString(distribution.Id) == "" {
			return sdk.EdgeOperationObservation{}, errors.New("CloudFront create response did not contain a distribution identity")
		}
		return pendingObservation(sdk.EdgeApply, distributionOperationID(sdk.EdgeApply, aws.ToString(distribution.Id), plan.purgeOnDeploy, operation.Request.IdempotencyKey, plan.purgePaths), operation.Request.OwnershipMarker), nil
	}
	id := aws.ToString(distribution.Id)
	if id == "" {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront distribution identity is empty")
	}
	if !distributionMatchesPlan(distribution, plan.config) {
		configOutput, configErr := api.api.GetDistributionConfig(ctx, &cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
		if configErr != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("read CloudFront distribution configuration: %w", configErr)
		}
		if configOutput == nil || configOutput.DistributionConfig == nil || strings.TrimSpace(aws.ToString(configOutput.ETag)) == "" {
			return sdk.EdgeOperationObservation{}, errors.New("CloudFront distribution configuration or ETag is missing")
		}
		if !ownedDistributionConfig(configOutput.DistributionConfig, plan.ownershipMarker) {
			return sdk.EdgeOperationObservation{}, errors.New("CloudFront distribution ownership marker does not match")
		}
		updated, updateErr := api.api.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{Id: aws.String(id), IfMatch: configOutput.ETag, DistributionConfig: plan.config})
		if updateErr != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("update CloudFront distribution: %w", updateErr)
		}
		if updated == nil || updated.Distribution == nil {
			return sdk.EdgeOperationObservation{}, errors.New("CloudFront update response did not contain a distribution")
		}
	}
	return pendingObservation(sdk.EdgeApply, distributionOperationID(sdk.EdgeApply, id, plan.purgeOnDeploy, operation.Request.IdempotencyKey, plan.purgePaths), operation.Request.OwnershipMarker), nil
}

func (api *NativeAPI) startDestroy(ctx context.Context, operation sdk.EdgeOperationRequest) (sdk.EdgeOperationObservation, error) {
	id, err := distributionIDFromReferences(operation.Request.ResourceReferences)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	distribution, getErr := api.api.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(id)})
	if getErr != nil {
		if isNoSuchDistribution(getErr) {
			return succeededObservation(sdk.EdgeDestroy, distributionOperationID(sdk.EdgeDestroy, id, false, "", nil), operation.Request.OwnershipMarker, []string{cloudFrontDistribution + id}, []string{"aws.cloudfront.owning-service-inventory-empty"}), nil
		}
		return sdk.EdgeOperationObservation{}, fmt.Errorf("read CloudFront distribution for destroy: %w", getErr)
	}
	if distribution == nil || distribution.Distribution == nil || !ownedDistributionConfig(distribution.Distribution.DistributionConfig, operation.Request.OwnershipMarker) {
		return sdk.EdgeOperationObservation{}, errors.New("refusing to destroy an unowned CloudFront distribution")
	}
	if err := api.disableIfNeeded(ctx, id, distribution.Distribution); err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	return pendingObservation(sdk.EdgeDestroy, distributionOperationID(sdk.EdgeDestroy, id, false, "", nil), operation.Request.OwnershipMarker), nil
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("CloudFront polling context is required")
	}
	kind, action, id, purge, callerReference, paths, transitionState, err := parseDistributionOperationID(operationID)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	switch kind {
	case "distribution":
		distributionOutput, getErr := api.api.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(id)})
		if getErr != nil {
			if action == sdk.EdgeDestroy && isNoSuchDistribution(getErr) {
				return succeededObservation(action, operationID, "", []string{cloudFrontDistribution + id}, []string{"aws.cloudfront.owning-service-inventory-empty"}), nil
			}
			return sdk.EdgeOperationObservation{}, getErr
		}
		if distributionOutput == nil || distributionOutput.Distribution == nil {
			return sdk.EdgeOperationObservation{}, errors.New("CloudFront poll response did not contain a distribution")
		}
		distribution := distributionOutput.Distribution
		marker, markerErr := markerFromDistribution(distribution)
		if markerErr != nil {
			return failedObservation(action, operationID, markerErr.Error()), nil
		}
		if action == sdk.EdgeDestroy {
			if distribution.DistributionConfig != nil && aws.ToBool(distribution.DistributionConfig.Enabled) {
				return failedObservation(action, operationID, "CloudFront distribution remains enabled during destroy"), nil
			}
			if aws.ToString(distribution.Status) != cloudFrontStatusDeployed {
				return pendingObservation(action, operationID, marker), nil
			}
			if err := api.deleteDistribution(ctx, distribution); err != nil {
				return sdk.EdgeOperationObservation{}, err
			}
			return succeededObservation(action, operationID, marker, []string{cloudFrontDistribution + id}, []string{"aws.cloudfront.disabled-before-delete"}), nil
		}
		if aws.ToString(distribution.Status) != cloudFrontStatusDeployed {
			return pendingObservation(action, operationID, marker), nil
		}
		if action == sdk.EdgeFailover || action == sdk.EdgeRollback {
			if err := transitionConverged(distribution.DistributionConfig, transitionState); err != nil {
				return failedObservation(action, operationID, err.Error()), nil
			}
		}
		refs := []string{cloudFrontDistribution + id}
		outputs := []sdk.AdapterOutput{{Key: "distributionId", Value: id}, {Key: "distributionDomainName", Value: aws.ToString(distribution.DomainName)}, {Key: "distributionStatus", Value: aws.ToString(distribution.Status)}}
		if purge {
			invalidation, invalidationErr := api.api.CreateInvalidation(ctx, &cloudfront.CreateInvalidationInput{DistributionId: aws.String(id), InvalidationBatch: invalidationBatch(callerReference, paths)})
			if invalidationErr != nil {
				return sdk.EdgeOperationObservation{}, fmt.Errorf("create CloudFront invalidation: %w", invalidationErr)
			}
			if invalidation == nil || invalidation.Invalidation == nil || aws.ToString(invalidation.Invalidation.Id) == "" {
				return sdk.EdgeOperationObservation{}, errors.New("CloudFront invalidation response did not contain an identity")
			}
			invalidationID := aws.ToString(invalidation.Invalidation.Id)
			refs = append(refs, "aws.cloudfront.invalidation:"+id+":"+invalidationID)
			outputs = append(outputs, sdk.AdapterOutput{Key: "invalidationId", Value: invalidationID})
			if aws.ToString(invalidation.Invalidation.Status) != cloudFrontStatusCompleted {
				return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationPending, Action: action, OperationID: operationID, ResourceRefs: refs, Outputs: outputs, OwnershipMarker: marker, OwnershipVerified: true, IdempotencyVerified: true, ProofRefs: []string{"aws.cloudfront.origin-health", "aws.cloudfront.cache-policy", "aws.cloudfront.purge-requested"}}, nil
			}
		}
		return succeededObservationWithOutputs(action, operationID, marker, refs, outputs, cloudFrontProofs(distribution, action, purge)), nil
	case "invalidation":
		invalidationOutput, getErr := api.api.GetInvalidation(ctx, &cloudfront.GetInvalidationInput{DistributionId: aws.String(id), Id: aws.String(callerReference)})
		if getErr != nil {
			return sdk.EdgeOperationObservation{}, getErr
		}
		if invalidationOutput == nil || invalidationOutput.Invalidation == nil {
			return sdk.EdgeOperationObservation{}, errors.New("CloudFront invalidation poll response is empty")
		}
		if aws.ToString(invalidationOutput.Invalidation.Status) != cloudFrontStatusCompleted {
			return pendingObservation(action, operationID, ""), nil
		}
		return succeededObservation(action, operationID, "", []string{"aws.cloudfront.invalidation:" + id + ":" + callerReference}, []string{"aws.cloudfront.purge-complete"}), nil
	default:
		return sdk.EdgeOperationObservation{}, fmt.Errorf("unsupported CloudFront operation kind %q", kind)
	}
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]sdk.EdgeInventoryResource, error) {
	if api == nil || api.api == nil {
		return nil, errors.New("CloudFront API is required")
	}
	if ctx == nil {
		return nil, errors.New("CloudFront inventory context is required")
	}
	marker = strings.TrimSpace(marker)
	if marker == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("CloudFront ownership marker is required and must be single-line")
	}
	var resources []sdk.EdgeInventoryResource
	var nextMarker *string
	for {
		output, err := api.api.ListDistributions(ctx, &cloudfront.ListDistributionsInput{Marker: nextMarker})
		if err != nil {
			return nil, fmt.Errorf("list CloudFront distributions: %w", err)
		}
		if output == nil || output.DistributionList == nil {
			break
		}
		for _, distribution := range output.DistributionList.Items {
			if aws.ToString(distribution.Comment) != cloudFrontCommentPrefix+marker || aws.ToString(distribution.Id) == "" {
				continue
			}
			resources = append(resources, sdk.EdgeInventoryResource{Identity: cloudFrontDistribution + aws.ToString(distribution.Id), OwnershipMarker: marker, Owned: true, Live: true})
		}
		if !aws.ToBool(output.DistributionList.IsTruncated) || strings.TrimSpace(aws.ToString(output.DistributionList.NextMarker)) == "" {
			break
		}
		nextMarker = output.DistributionList.NextMarker
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) distributionForRequest(ctx context.Context, references []string, marker string) (*cloudfronttypes.Distribution, error) {
	id, found, err := distributionReference(references)
	if err != nil {
		return nil, err
	}
	if found {
		output, getErr := api.api.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(id)})
		if getErr != nil {
			if isNoSuchDistribution(getErr) {
				return nil, nil
			}
			return nil, fmt.Errorf("read referenced CloudFront distribution: %w", getErr)
		}
		if output == nil || output.Distribution == nil {
			return nil, errors.New("referenced CloudFront distribution response is empty")
		}
		if !ownedDistributionConfig(output.Distribution.DistributionConfig, marker) {
			return nil, errors.New("refusing to adopt an unowned referenced CloudFront distribution")
		}
		return output.Distribution, nil
	}
	owned, err := api.findOwnedDistribution(ctx, marker)
	if err != nil {
		return nil, err
	}
	return owned, nil
}

func (api *NativeAPI) findOwnedDistribution(ctx context.Context, marker string) (*cloudfronttypes.Distribution, error) {
	resources, err := api.Inventory(ctx, marker)
	if err != nil {
		return nil, err
	}
	if len(resources) > 1 {
		return nil, errors.New("multiple CloudFront distributions share the ownership marker")
	}
	if len(resources) == 0 {
		return nil, nil
	}
	id := strings.TrimPrefix(resources[0].Identity, cloudFrontDistribution)
	output, err := api.api.GetDistribution(ctx, &cloudfront.GetDistributionInput{Id: aws.String(id)})
	if err != nil {
		return nil, fmt.Errorf("read owned CloudFront distribution: %w", err)
	}
	if output == nil || output.Distribution == nil {
		return nil, errors.New("owned CloudFront distribution response is empty")
	}
	return output.Distribution, nil
}

func (api *NativeAPI) disableIfNeeded(ctx context.Context, id string, distribution *cloudfronttypes.Distribution) error {
	if distribution == nil || distribution.DistributionConfig == nil || !aws.ToBool(distribution.DistributionConfig.Enabled) {
		return nil
	}
	configOutput, err := api.api.GetDistributionConfig(ctx, &cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
	if err != nil {
		return fmt.Errorf("read CloudFront configuration before disable: %w", err)
	}
	if configOutput == nil || configOutput.DistributionConfig == nil || strings.TrimSpace(aws.ToString(configOutput.ETag)) == "" {
		return errors.New("CloudFront disable requires the current configuration and ETag")
	}
	configOutput.DistributionConfig.Enabled = aws.Bool(false)
	if _, err := api.api.UpdateDistribution(ctx, &cloudfront.UpdateDistributionInput{Id: aws.String(id), IfMatch: configOutput.ETag, DistributionConfig: configOutput.DistributionConfig}); err != nil {
		return fmt.Errorf("disable CloudFront distribution: %w", err)
	}
	return nil
}

func (api *NativeAPI) deleteDistribution(ctx context.Context, distribution *cloudfronttypes.Distribution) error {
	id := aws.ToString(distribution.Id)
	configOutput, err := api.api.GetDistributionConfig(ctx, &cloudfront.GetDistributionConfigInput{Id: aws.String(id)})
	if err != nil {
		return fmt.Errorf("read CloudFront configuration before delete: %w", err)
	}
	if configOutput == nil || configOutput.DistributionConfig == nil || strings.TrimSpace(aws.ToString(configOutput.ETag)) == "" {
		return errors.New("CloudFront delete requires the current configuration and ETag")
	}
	if aws.ToBool(configOutput.DistributionConfig.Enabled) {
		return errors.New("CloudFront distribution must be disabled before delete")
	}
	if _, err := api.api.DeleteDistribution(ctx, &cloudfront.DeleteDistributionInput{Id: aws.String(id), IfMatch: configOutput.ETag}); err != nil {
		return fmt.Errorf("delete CloudFront distribution: %w", err)
	}
	return nil
}

func parseCloudFrontOrigin(configuration map[string]any) (cloudFrontOrigin, error) {
	return parseCloudFrontOriginValue(configuration["origin"], "origin")
}

func parseCloudFrontOriginSet(request sdk.EdgePlanRequest) (cloudFrontOriginSet, error) {
	groupValue, groupConfigured := request.Configuration["originGroup"]
	if !groupConfigured {
		if strings.TrimSpace(request.Intent.FailoverPolicyRef) != "" {
			return cloudFrontOriginSet{}, sdk.EdgeCapabilityError{AdapterID: cloudFrontEdgeAdapterID, Action: sdk.EdgeFailover, Status: sdk.EdgeCapabilityUnsupported, Reason: "CloudFront failover requires a provider-owned originGroup with primary and secondary origins"}
		}
		origin, err := parseCloudFrontOrigin(request.Configuration)
		if err != nil {
			return cloudFrontOriginSet{}, err
		}
		return cloudFrontOriginSet{origins: []cloudFrontOrigin{origin}, healthRefs: []string{request.Intent.OriginHealthRef}}, nil
	}
	if strings.TrimSpace(request.Intent.FailoverPolicyRef) == "" {
		return cloudFrontOriginSet{}, errors.New("provider configuration originGroup requires FailoverPolicyRef")
	}
	group, ok := groupValue.(map[string]any)
	if !ok || group == nil {
		return cloudFrontOriginSet{}, errors.New("originGroup must be an object")
	}
	primary, err := parseCloudFrontOriginValue(group["primary"], "originGroup.primary")
	if err != nil {
		return cloudFrontOriginSet{}, err
	}
	secondary, err := parseCloudFrontOriginValue(group["secondary"], "originGroup.secondary")
	if err != nil {
		return cloudFrontOriginSet{}, err
	}
	if primary.originID == secondary.originID {
		return cloudFrontOriginSet{}, errors.New("originGroup primary and secondary origins require different IDs")
	}
	groupID := strings.TrimSpace(stringOption(group, "id"))
	if groupID == "" {
		groupID = "magelift-origin-group"
	}
	if strings.ContainsAny(groupID, "\r\n\x00 /\\") {
		return cloudFrontOriginSet{}, errors.New("originGroup.id must be a single-line CloudFront identifier")
	}
	statusCodes, err := parseFailoverStatusCodes(group["statusCodes"])
	if err != nil {
		return cloudFrontOriginSet{}, err
	}
	primaryHealthRef := strings.TrimSpace(stringOption(group, "primaryHealthRef"))
	secondaryHealthRef := strings.TrimSpace(stringOption(group, "secondaryHealthRef"))
	if primaryHealthRef == "" || secondaryHealthRef == "" {
		return cloudFrontOriginSet{}, errors.New("originGroup.primaryHealthRef and originGroup.secondaryHealthRef are required")
	}
	if primaryHealthRef != request.Intent.OriginHealthRef {
		return cloudFrontOriginSet{}, errors.New("originGroup.primaryHealthRef must match the portable origin health reference")
	}
	return cloudFrontOriginSet{origins: []cloudFrontOrigin{primary, secondary}, groupID: groupID, failoverStatusCodes: statusCodes, healthRefs: []string{primaryHealthRef, secondaryHealthRef}}, nil
}

func parseCloudFrontOriginValue(value any, label string) (cloudFrontOrigin, error) {
	originMap, ok := value.(map[string]any)
	if !ok || originMap == nil {
		return cloudFrontOrigin{}, fmt.Errorf("%s object is required", label)
	}
	origin := cloudFrontOrigin{kind: stringOption(originMap, "kind"), domainName: strings.TrimSpace(stringOption(originMap, "domainName")), originID: strings.TrimSpace(stringOption(originMap, "id")), originAccessControlID: strings.TrimSpace(stringOption(originMap, "originAccessControlID")), originPath: stringOption(originMap, "path")}
	if origin.kind == "" {
		origin.kind = "custom"
	}
	if origin.domainName == "" || strings.ContainsAny(origin.domainName, "\r\n\x00 /\\") {
		return cloudFrontOrigin{}, fmt.Errorf("%s.domainName is required and must be a host-like value", label)
	}
	if origin.originID == "" {
		origin.originID = "magelift-origin"
	}
	if strings.ContainsAny(origin.originID+origin.originAccessControlID+origin.originPath, "\r\n\x00") {
		return cloudFrontOrigin{}, fmt.Errorf("%s identifiers must be single-line", label)
	}
	switch origin.kind {
	case "custom", "load-balancer", "alb", "service":
	case "s3":
		if origin.originAccessControlID == "" {
			return cloudFrontOrigin{}, fmt.Errorf("%s S3 origins require originAccessControlID", label)
		}
	default:
		return cloudFrontOrigin{}, fmt.Errorf("unsupported CloudFront origin kind %q for %s", origin.kind, label)
	}
	return origin, nil
}

func parseFailoverStatusCodes(value any) ([]int32, error) {
	if value == nil {
		return []int32{500, 502, 503, 504}, nil
	}
	var values []int32
	switch items := value.(type) {
	case []int:
		for _, item := range items {
			values = append(values, int32(item))
		}
	case []int32:
		values = append(values, items...)
	case []any:
		for _, item := range items {
			number, ok := item.(float64)
			if !ok || number != float64(int32(number)) {
				return nil, errors.New("originGroup.statusCodes must contain only integers")
			}
			values = append(values, int32(number))
		}
	default:
		return nil, errors.New("originGroup.statusCodes must be an integer array")
	}
	if len(values) == 0 {
		return nil, errors.New("originGroup.statusCodes must contain at least one status code")
	}
	seen := make(map[int32]struct{}, len(values))
	for _, value := range values {
		if value < 400 || value > 599 {
			return nil, fmt.Errorf("originGroup.statusCodes value %d must be between 400 and 599", value)
		}
		if _, exists := seen[value]; exists {
			return nil, fmt.Errorf("originGroup.statusCodes contains duplicate value %d", value)
		}
		seen[value] = struct{}{}
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	return values, nil
}

func buildDistributionConfig(request sdk.EdgePlanRequest, originSet cloudFrontOriginSet, comment string) (*cloudfronttypes.DistributionConfig, error) {
	cachePolicyID := strings.TrimSpace(stringOption(request.Configuration, "cachePolicyID"))
	if cachePolicyID == "" {
		if request.Intent.CachePolicyRef != "" {
			return nil, errors.New("edge cachePolicyRef requires provider configuration cachePolicyID")
		}
		cachePolicyID = managedCachingDisabledPolicyID
	}
	if strings.ContainsAny(cachePolicyID, "\r\n\x00") {
		return nil, errors.New("cachePolicyID must be single-line")
	}
	originProtocol := cloudfronttypes.OriginProtocolPolicyHttpsOnly
	if value := stringOption(request.Configuration, "originProtocolPolicy"); value != "" {
		originProtocol = cloudfronttypes.OriginProtocolPolicy(value)
	}
	if originProtocol != cloudfronttypes.OriginProtocolPolicyHttpsOnly {
		return nil, errors.New("CloudFront originProtocolPolicy must be https-only")
	}
	if len(originSet.origins) == 0 {
		return nil, errors.New("at least one CloudFront origin is required")
	}
	originValues := make([]cloudfronttypes.Origin, 0, len(originSet.origins))
	for _, origin := range originSet.origins {
		originValue := cloudfronttypes.Origin{DomainName: aws.String(origin.domainName), Id: aws.String(origin.originID), OriginPath: optionalString(origin.originPath), OriginAccessControlId: optionalString(origin.originAccessControlID)}
		if origin.kind == "s3" {
			originValue.S3OriginConfig = &cloudfronttypes.S3OriginConfig{OriginAccessIdentity: aws.String("")}
		} else {
			originValue.CustomOriginConfig = &cloudfronttypes.CustomOriginConfig{HTTPPort: aws.Int32(80), HTTPSPort: aws.Int32(443), OriginProtocolPolicy: originProtocol, OriginSslProtocols: &cloudfronttypes.OriginSslProtocols{Quantity: aws.Int32(1), Items: []cloudfronttypes.SslProtocol{cloudfronttypes.SslProtocolTLSv12}}}
		}
		originValues = append(originValues, originValue)
	}
	domains := append([]string(nil), request.Intent.Domains...)
	sort.Strings(domains)
	allowed := []cloudfronttypes.Method{cloudfronttypes.MethodDelete, cloudfronttypes.MethodGet, cloudfronttypes.MethodHead, cloudfronttypes.MethodOptions, cloudfronttypes.MethodPatch, cloudfronttypes.MethodPost, cloudfronttypes.MethodPut}
	if originSet.hasFailover() {
		allowed = []cloudfronttypes.Method{cloudfronttypes.MethodGet, cloudfronttypes.MethodHead, cloudfronttypes.MethodOptions}
	}
	cached := []cloudfronttypes.Method{cloudfronttypes.MethodGet, cloudfronttypes.MethodHead}
	viewerPolicy := cloudfronttypes.ViewerProtocolPolicyAllowAll
	viewerCertificate := &cloudfronttypes.ViewerCertificate{CloudFrontDefaultCertificate: aws.Bool(true)}
	if request.Intent.TLS {
		viewerPolicy = cloudfronttypes.ViewerProtocolPolicyRedirectToHttps
		certificateARN := strings.TrimSpace(stringOption(request.Configuration, "certificateARN"))
		if certificateARN == "" {
			return nil, errors.New("TLS-enabled CloudFront aliases require provider configuration certificateARN")
		}
		if !strings.HasPrefix(certificateARN, "arn:aws:acm:us-east-1:") {
			return nil, errors.New("CloudFront certificateARN must reference an ACM certificate in us-east-1")
		}
		viewerCertificate = &cloudfronttypes.ViewerCertificate{ACMCertificateArn: aws.String(certificateARN), MinimumProtocolVersion: cloudfronttypes.MinimumProtocolVersionTLSv122021, SSLSupportMethod: cloudfronttypes.SSLSupportMethodSniOnly}
	}
	webACLID := strings.TrimSpace(stringOption(request.Configuration, "webACLID"))
	if request.Intent.WAFPolicyRef != "" && webACLID == "" {
		return nil, errors.New("WAFPolicyRef requires provider configuration webACLID")
	}
	if strings.ContainsAny(webACLID, "\r\n\x00") {
		return nil, errors.New("webACLID must be single-line")
	}
	targetOriginID := originSet.origins[0].originID
	var originGroups *cloudfronttypes.OriginGroups
	if originSet.hasFailover() {
		targetOriginID = originSet.groupID
		originGroups = &cloudfronttypes.OriginGroups{Quantity: aws.Int32(1), Items: []cloudfronttypes.OriginGroup{{Id: aws.String(originSet.groupID), SelectionCriteria: cloudfronttypes.OriginGroupSelectionCriteriaDefault, Members: &cloudfronttypes.OriginGroupMembers{Quantity: aws.Int32(2), Items: []cloudfronttypes.OriginGroupMember{{OriginId: aws.String(originSet.origins[0].originID)}, {OriginId: aws.String(originSet.origins[1].originID)}}}, FailoverCriteria: &cloudfronttypes.OriginGroupFailoverCriteria{StatusCodes: &cloudfronttypes.StatusCodes{Quantity: aws.Int32(int32(len(originSet.failoverStatusCodes))), Items: append([]int32(nil), originSet.failoverStatusCodes...)}}}}}
	}
	return &cloudfronttypes.DistributionConfig{
		CallerReference: aws.String(distributionCallerReference(comment, originSet, domains, cachePolicyID)), Comment: aws.String(comment), Enabled: aws.Bool(true),
		Aliases: &cloudfronttypes.Aliases{Quantity: aws.Int32(int32(len(domains))), Items: domains},
		Origins: &cloudfronttypes.Origins{Quantity: aws.Int32(int32(len(originValues))), Items: originValues}, OriginGroups: originGroups,
		DefaultCacheBehavior: &cloudfronttypes.DefaultCacheBehavior{TargetOriginId: aws.String(targetOriginID), ViewerProtocolPolicy: viewerPolicy, AllowedMethods: &cloudfronttypes.AllowedMethods{Quantity: aws.Int32(int32(len(allowed))), Items: allowed, CachedMethods: &cloudfronttypes.CachedMethods{Quantity: aws.Int32(int32(len(cached))), Items: cached}}, CachePolicyId: aws.String(cachePolicyID), Compress: aws.Bool(true), OriginRequestPolicyId: optionalString(stringOption(request.Configuration, "originRequestPolicyID")), ResponseHeadersPolicyId: optionalString(stringOption(request.Configuration, "responseHeadersPolicyID"))},
		ViewerCertificate:    viewerCertificate, WebACLId: optionalString(webACLID), IsIPV6Enabled: aws.Bool(true),
		Restrictions: &cloudfronttypes.Restrictions{GeoRestriction: &cloudfronttypes.GeoRestriction{RestrictionType: cloudfronttypes.GeoRestrictionTypeNone, Quantity: aws.Int32(0)}},
	}, nil
}

func parsePurgePaths(configuration map[string]any, enabled bool) ([]string, error) {
	if !enabled {
		return nil, nil
	}
	value, found := configuration["purgePaths"]
	if !found {
		return []string{"/*"}, nil
	}
	var paths []string
	switch values := value.(type) {
	case []string:
		paths = append(paths, values...)
	case []any:
		for _, value := range values {
			path, ok := value.(string)
			if !ok {
				return nil, errors.New("purgePaths must contain only strings")
			}
			paths = append(paths, path)
		}
	default:
		return nil, errors.New("purgePaths must be a string array")
	}
	if len(paths) == 0 || len(paths) > 3000 {
		return nil, errors.New("purgePaths must contain between 1 and 3000 paths")
	}
	seen := make(map[string]struct{}, len(paths))
	for index, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n\x00") {
			return nil, fmt.Errorf("purge path %d is invalid", index)
		}
		if _, exists := seen[path]; exists {
			return nil, fmt.Errorf("purge path %q is duplicated", path)
		}
		seen[path] = struct{}{}
		paths[index] = path
	}
	sort.Strings(paths)
	return paths, nil
}

func distributionCallerReference(comment string, originSet cloudFrontOriginSet, domains []string, cachePolicyID string) string {
	parts := []string{comment, originSet.groupID, strings.Join(domains, ","), cachePolicyID}
	for _, origin := range originSet.origins {
		parts = append(parts, origin.kind, origin.domainName, origin.originID, origin.originAccessControlID, origin.originPath)
	}
	for _, statusCode := range originSet.failoverStatusCodes {
		parts = append(parts, strconv.FormatInt(int64(statusCode), 10))
	}
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "magelift-" + hex.EncodeToString(hash[:])
}

func distributionOperationID(action sdk.EdgeAction, id string, purge bool, callerReference string, paths []string) string {
	operation := cloudFrontOperation + ":distribution:" + string(action) + ":" + id
	if !purge {
		return operation
	}
	pathBytes := []byte(strings.Join(paths, "\x00"))
	return operation + ":purge:" + base64.RawURLEncoding.EncodeToString([]byte(callerReference)) + ":" + base64.RawURLEncoding.EncodeToString(pathBytes)
}

func transitionOperationID(action sdk.EdgeAction, id string, groups *cloudfronttypes.OriginGroups) (string, error) {
	if action != sdk.EdgeFailover && action != sdk.EdgeRollback {
		return "", fmt.Errorf("invalid CloudFront transition action %q", action)
	}
	if strings.TrimSpace(id) == "" || groups == nil || len(groups.Items) != 1 {
		return "", errors.New("CloudFront transition requires one origin group")
	}
	group := groups.Items[0]
	if strings.TrimSpace(aws.ToString(group.Id)) == "" || group.Members == nil || len(group.Members.Items) != 2 {
		return "", errors.New("CloudFront transition requires two origin-group members")
	}
	state := []string{aws.ToString(group.Id), aws.ToString(group.Members.Items[0].OriginId), aws.ToString(group.Members.Items[1].OriginId), originGroupStatusCodeIdentity(group)}
	for _, value := range state {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return "", errors.New("CloudFront transition state contains an invalid value")
		}
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(strings.Join(state, "\x00")))
	return cloudFrontOperation + ":distribution:" + string(action) + ":" + id + ":transition:" + payload, nil
}

func transitionConverged(config *cloudfronttypes.DistributionConfig, expected []string) error {
	if len(expected) != 4 {
		return errors.New("CloudFront transition is missing its expected origin-group state")
	}
	if config == nil || config.DefaultCacheBehavior == nil || config.OriginGroups == nil || len(config.OriginGroups.Items) != 1 {
		return errors.New("CloudFront transition did not converge to one origin group")
	}
	group := config.OriginGroups.Items[0]
	if aws.ToString(group.Id) != expected[0] || aws.ToString(config.DefaultCacheBehavior.TargetOriginId) != expected[0] || group.Members == nil || aws.ToInt32(group.Members.Quantity) != 2 || len(group.Members.Items) != 2 || aws.ToString(group.Members.Items[0].OriginId) != expected[1] || aws.ToString(group.Members.Items[1].OriginId) != expected[2] {
		return errors.New("CloudFront transition did not converge to the requested origin order")
	}
	if originGroupStatusCodeIdentity(group) != expected[3] {
		return errors.New("CloudFront transition changed the planned failover status codes")
	}
	return nil
}

func originGroupStatusCodeIdentity(group cloudfronttypes.OriginGroup) string {
	if group.FailoverCriteria == nil || group.FailoverCriteria.StatusCodes == nil || len(group.FailoverCriteria.StatusCodes.Items) == 0 {
		return ""
	}
	codes := make([]string, 0, len(group.FailoverCriteria.StatusCodes.Items))
	for _, code := range group.FailoverCriteria.StatusCodes.Items {
		codes = append(codes, strconv.FormatInt(int64(code), 10))
	}
	return strings.Join(codes, ",")
}

func parseDistributionOperationID(value string) (kind string, action sdk.EdgeAction, id string, purge bool, callerReference string, paths, transitionState []string, err error) {
	parts := strings.Split(value, ":")
	if len(parts) < 4 || parts[0] != "aws.cloudfront" || (parts[1] != "distribution" && parts[1] != "invalidation") {
		return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront operation ID %q", value)
	}
	kind, action, id = parts[1], sdk.EdgeAction(parts[2]), parts[3]
	if id == "" || (action != sdk.EdgeApply && action != sdk.EdgeVerify && action != sdk.EdgeFailover && action != sdk.EdgeRollback && action != sdk.EdgeDestroy) {
		return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront operation ID %q", value)
	}
	if kind == "invalidation" {
		if len(parts) != 5 || parts[4] == "" {
			return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront invalidation operation ID %q", value)
		}
		return kind, action, id, false, parts[4], nil, nil, nil
	}
	if len(parts) == 4 {
		return kind, action, id, false, "", nil, nil, nil
	}
	if len(parts) == 6 && parts[4] == "transition" {
		if action != sdk.EdgeFailover && action != sdk.EdgeRollback {
			return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront transition operation ID %q", value)
		}
		stateBytes, decodeErr := base64.RawURLEncoding.DecodeString(parts[5])
		if decodeErr != nil {
			return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront transition operation ID %q", value)
		}
		transitionState = strings.Split(string(stateBytes), "\x00")
		if len(transitionState) != 4 {
			return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront transition operation ID %q", value)
		}
		for _, value := range transitionState {
			if value == "" || strings.ContainsAny(value, "\r\n\x00") {
				return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront transition operation ID %q", value)
			}
		}
		return kind, action, id, false, "", nil, transitionState, nil
	}
	if len(parts) != 7 || parts[4] != "purge" {
		return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront distribution operation ID %q", value)
	}
	callerBytes, callerErr := base64.RawURLEncoding.DecodeString(parts[5])
	pathBytes, pathErr := base64.RawURLEncoding.DecodeString(parts[6])
	if callerErr != nil || pathErr != nil || len(callerBytes) == 0 {
		return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront purge operation ID %q", value)
	}
	paths = strings.Split(string(pathBytes), "\x00")
	if len(paths) == 0 || paths[0] == "" {
		return "", "", "", false, "", nil, nil, fmt.Errorf("invalid CloudFront purge operation ID %q", value)
	}
	return kind, action, id, true, string(callerBytes), paths, nil, nil
}

func distributionIDFromReferences(references []string) (string, error) {
	var id string
	for _, reference := range references {
		if !strings.HasPrefix(reference, cloudFrontDistribution) {
			continue
		}
		candidate := strings.TrimPrefix(reference, cloudFrontDistribution)
		if candidate == "" || strings.ContainsAny(candidate, "\r\n\x00:") {
			return "", fmt.Errorf("invalid CloudFront distribution reference %q", reference)
		}
		if id != "" && id != candidate {
			return "", errors.New("multiple CloudFront distribution references were supplied")
		}
		id = candidate
	}
	if id == "" {
		return "", errors.New("CloudFront distribution reference is required")
	}
	return id, nil
}

func distributionReference(references []string) (string, bool, error) {
	for _, reference := range references {
		if strings.HasPrefix(reference, cloudFrontDistribution) {
			id, err := distributionIDFromReferences(references)
			return id, true, err
		}
	}
	return "", false, nil
}

func distributionMatchesPlan(distribution *cloudfronttypes.Distribution, desired *cloudfronttypes.DistributionConfig) bool {
	if distribution == nil || desired == nil || !ownedDistributionConfig(distribution.DistributionConfig, strings.TrimPrefix(aws.ToString(desired.Comment), cloudFrontCommentPrefix)) {
		return false
	}
	current := distribution.DistributionConfig
	if current == nil || aws.ToString(current.CallerReference) != aws.ToString(desired.CallerReference) || aws.ToBool(current.Enabled) != aws.ToBool(desired.Enabled) || aws.ToString(current.WebACLId) != aws.ToString(desired.WebACLId) {
		return false
	}
	return aliasesMatch(current.Aliases, desired.Aliases) && viewerCertificateMatch(current.ViewerCertificate, desired.ViewerCertificate) && originAndBehaviorMatch(current, desired) && restrictionsMatch(current.Restrictions, desired.Restrictions) && aws.ToBool(current.IsIPV6Enabled) == aws.ToBool(desired.IsIPV6Enabled)
}

func aliasesMatch(current, desired *cloudfronttypes.Aliases) bool {
	if current == nil || desired == nil || aws.ToInt32(current.Quantity) != aws.ToInt32(desired.Quantity) || len(current.Items) != len(desired.Items) {
		return false
	}
	for index := range desired.Items {
		if desired.Items[index] != current.Items[index] {
			return false
		}
	}
	return true
}

func viewerCertificateMatch(current, desired *cloudfronttypes.ViewerCertificate) bool {
	if current == nil || desired == nil || aws.ToBool(current.CloudFrontDefaultCertificate) != aws.ToBool(desired.CloudFrontDefaultCertificate) || aws.ToString(current.ACMCertificateArn) != aws.ToString(desired.ACMCertificateArn) || current.MinimumProtocolVersion != desired.MinimumProtocolVersion || current.SSLSupportMethod != desired.SSLSupportMethod {
		return false
	}
	return true
}

func originAndBehaviorMatch(current, desired *cloudfronttypes.DistributionConfig) bool {
	if current == nil || desired == nil || current.Origins == nil || desired.Origins == nil || len(current.Origins.Items) != len(desired.Origins.Items) || current.DefaultCacheBehavior == nil || desired.DefaultCacheBehavior == nil || !originGroupsMatch(current.OriginGroups, desired.OriginGroups) {
		return false
	}
	for index := range desired.Origins.Items {
		want, got := desired.Origins.Items[index], current.Origins.Items[index]
		if aws.ToString(want.DomainName) != aws.ToString(got.DomainName) || aws.ToString(want.Id) != aws.ToString(got.Id) || aws.ToString(want.OriginAccessControlId) != aws.ToString(got.OriginAccessControlId) || aws.ToString(want.OriginPath) != aws.ToString(got.OriginPath) || !reflect.DeepEqual(want.OriginShield, got.OriginShield) || !reflect.DeepEqual(want.CustomHeaders, got.CustomHeaders) || !reflect.DeepEqual(want.CustomOriginConfig, got.CustomOriginConfig) || !reflect.DeepEqual(want.S3OriginConfig, got.S3OriginConfig) {
			return false
		}
	}
	want, got := desired.DefaultCacheBehavior, current.DefaultCacheBehavior
	if aws.ToString(want.TargetOriginId) != aws.ToString(got.TargetOriginId) || want.ViewerProtocolPolicy != got.ViewerProtocolPolicy || aws.ToString(want.CachePolicyId) != aws.ToString(got.CachePolicyId) || aws.ToString(want.OriginRequestPolicyId) != aws.ToString(got.OriginRequestPolicyId) || aws.ToString(want.ResponseHeadersPolicyId) != aws.ToString(got.ResponseHeadersPolicyId) || aws.ToBool(want.Compress) != aws.ToBool(got.Compress) {
		return false
	}
	return allowedMethodsMatch(want.AllowedMethods, got.AllowedMethods) && cachedMethodsMatch(want.AllowedMethods, got.AllowedMethods)
}

func originGroupsMatch(current, desired *cloudfronttypes.OriginGroups) bool {
	if current == nil || desired == nil {
		return current == nil && desired == nil
	}
	if aws.ToInt32(current.Quantity) != aws.ToInt32(desired.Quantity) || len(current.Items) != len(desired.Items) {
		return false
	}
	for index := range desired.Items {
		want, got := desired.Items[index], current.Items[index]
		if aws.ToString(want.Id) != aws.ToString(got.Id) || want.SelectionCriteria != got.SelectionCriteria || !originGroupMembersMatch(got.Members, want.Members) || !statusCodesMatch(got.FailoverCriteria, want.FailoverCriteria) {
			return false
		}
	}
	return true
}

func originGroupTopologyMatch(current, desired *cloudfronttypes.OriginGroups) bool {
	if current == nil || desired == nil || len(current.Items) != 1 || len(desired.Items) != 1 {
		return false
	}
	currentGroup, desiredGroup := current.Items[0], desired.Items[0]
	if aws.ToString(currentGroup.Id) != aws.ToString(desiredGroup.Id) || !statusCodesMatch(currentGroup.FailoverCriteria, desiredGroup.FailoverCriteria) || currentGroup.Members == nil || desiredGroup.Members == nil || len(currentGroup.Members.Items) != len(desiredGroup.Members.Items) {
		return false
	}
	currentIDs := make([]string, 0, len(currentGroup.Members.Items))
	desiredIDs := make([]string, 0, len(desiredGroup.Members.Items))
	for _, member := range currentGroup.Members.Items {
		currentIDs = append(currentIDs, aws.ToString(member.OriginId))
	}
	for _, member := range desiredGroup.Members.Items {
		desiredIDs = append(desiredIDs, aws.ToString(member.OriginId))
	}
	sort.Strings(currentIDs)
	sort.Strings(desiredIDs)
	return strings.Join(currentIDs, "\x00") == strings.Join(desiredIDs, "\x00")
}

func originGroupMembersMatch(current, desired *cloudfronttypes.OriginGroupMembers) bool {
	if current == nil || desired == nil {
		return current == nil && desired == nil
	}
	if aws.ToInt32(current.Quantity) != aws.ToInt32(desired.Quantity) || len(current.Items) != len(desired.Items) {
		return false
	}
	for index := range desired.Items {
		if aws.ToString(current.Items[index].OriginId) != aws.ToString(desired.Items[index].OriginId) {
			return false
		}
	}
	return true
}

func statusCodesMatch(current, desired *cloudfronttypes.OriginGroupFailoverCriteria) bool {
	if current == nil || desired == nil {
		return current == nil && desired == nil
	}
	if current.StatusCodes == nil || desired.StatusCodes == nil || aws.ToInt32(current.StatusCodes.Quantity) != aws.ToInt32(desired.StatusCodes.Quantity) || len(current.StatusCodes.Items) != len(desired.StatusCodes.Items) {
		return false
	}
	for index := range desired.StatusCodes.Items {
		if current.StatusCodes.Items[index] != desired.StatusCodes.Items[index] {
			return false
		}
	}
	return true
}

func transitionOriginGroup(current *cloudfronttypes.DistributionConfig, desired *cloudfronttypes.OriginGroups, failover bool) (*cloudfronttypes.DistributionConfig, error) {
	if current == nil || current.DefaultCacheBehavior == nil || current.OriginGroups == nil || len(current.OriginGroups.Items) != 1 || desired == nil || len(desired.Items) != 1 || current.OriginGroups.Items[0].Members == nil || len(current.OriginGroups.Items[0].Members.Items) != 2 || desired.Items[0].Members == nil || len(desired.Items[0].Members.Items) != 2 {
		return nil, errors.New("CloudFront origin transition requires one origin group and a default cache behavior")
	}
	next := *current
	behavior := *current.DefaultCacheBehavior
	next.DefaultCacheBehavior = &behavior
	groups := *current.OriginGroups
	groups.Items = append([]cloudfronttypes.OriginGroup(nil), current.OriginGroups.Items...)
	group := groups.Items[0]
	desiredGroup := desired.Items[0]
	members := append([]cloudfronttypes.OriginGroupMember(nil), desiredGroup.Members.Items...)
	if failover {
		members[0], members[1] = members[1], members[0]
	}
	group.Members = &cloudfronttypes.OriginGroupMembers{Quantity: aws.Int32(int32(len(members))), Items: members}
	groups.Items[0] = group
	next.OriginGroups = &groups
	next.DefaultCacheBehavior.TargetOriginId = aws.String(aws.ToString(desiredGroup.Id))
	return &next, nil
}

func allowedMethodsMatch(current, desired *cloudfronttypes.AllowedMethods) bool {
	if current == nil || desired == nil || aws.ToInt32(current.Quantity) != aws.ToInt32(desired.Quantity) || len(current.Items) != len(desired.Items) {
		return false
	}
	for index := range desired.Items {
		if desired.Items[index] != current.Items[index] {
			return false
		}
	}
	return true
}

func cachedMethodsMatch(current, desired *cloudfronttypes.AllowedMethods) bool {
	if current == nil || desired == nil || current.CachedMethods == nil || desired.CachedMethods == nil || aws.ToInt32(current.CachedMethods.Quantity) != aws.ToInt32(desired.CachedMethods.Quantity) || len(current.CachedMethods.Items) != len(desired.CachedMethods.Items) {
		return false
	}
	for index := range desired.CachedMethods.Items {
		if desired.CachedMethods.Items[index] != current.CachedMethods.Items[index] {
			return false
		}
	}
	return true
}

func restrictionsMatch(current, desired *cloudfronttypes.Restrictions) bool {
	if current == nil || desired == nil || current.GeoRestriction == nil || desired.GeoRestriction == nil {
		return false
	}
	currentGeo, desiredGeo := current.GeoRestriction, desired.GeoRestriction
	if currentGeo.RestrictionType != desiredGeo.RestrictionType || aws.ToInt32(currentGeo.Quantity) != aws.ToInt32(desiredGeo.Quantity) || len(currentGeo.Items) != len(desiredGeo.Items) {
		return false
	}
	for index := range desiredGeo.Items {
		if desiredGeo.Items[index] != currentGeo.Items[index] {
			return false
		}
	}
	return true
}

func ownedDistributionConfig(config *cloudfronttypes.DistributionConfig, marker string) bool {
	return config != nil && aws.ToString(config.Comment) == cloudFrontCommentPrefix+strings.TrimSpace(marker)
}

func markerFromDistribution(distribution *cloudfronttypes.Distribution) (string, error) {
	if distribution == nil || distribution.DistributionConfig == nil {
		return "", errors.New("CloudFront distribution configuration is missing")
	}
	comment := aws.ToString(distribution.DistributionConfig.Comment)
	if !strings.HasPrefix(comment, cloudFrontCommentPrefix) {
		return "", errors.New("CloudFront distribution ownership comment is missing")
	}
	marker := strings.TrimPrefix(comment, cloudFrontCommentPrefix)
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return "", errors.New("CloudFront distribution ownership marker is invalid")
	}
	return marker, nil
}

func cloudFrontProofs(distribution *cloudfronttypes.Distribution, action sdk.EdgeAction, purge bool) []string {
	proofs := []string{"aws.cloudfront.ownership", "aws.cloudfront.origin-health", "aws.cloudfront.tls", "aws.cloudfront.cache-policy"}
	if distribution != nil && distribution.DistributionConfig != nil && strings.TrimSpace(aws.ToString(distribution.DistributionConfig.WebACLId)) != "" {
		proofs = append(proofs, "aws.cloudfront.waf")
	}
	if distribution != nil && distribution.DistributionConfig != nil && distribution.DistributionConfig.OriginGroups != nil {
		proofs = append(proofs, "aws.cloudfront.origin-failover-configured")
		if action == sdk.EdgeFailover || action == sdk.EdgeRollback {
			proofs = append(proofs, "aws.cloudfront."+string(action)+"-control-plane-converged")
		}
	}
	if purge {
		proofs = append(proofs, "aws.cloudfront.purge-complete")
	}
	sort.Strings(proofs)
	return proofs
}

func invalidationBatch(callerReference string, paths []string) *cloudfronttypes.InvalidationBatch {
	return &cloudfronttypes.InvalidationBatch{CallerReference: aws.String(callerReference), Paths: &cloudfronttypes.Paths{Quantity: aws.Int32(int32(len(paths))), Items: append([]string(nil), paths...)}}
}

func createdDistribution(output *cloudfront.CreateDistributionOutput) *cloudfronttypes.Distribution {
	if output == nil {
		return nil
	}
	return output.Distribution
}

func pendingObservation(action sdk.EdgeAction, operationID, marker string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationPending, Action: action, OperationID: operationID, OwnershipMarker: marker, OwnershipVerified: true, IdempotencyVerified: true}
}

func succeededObservation(action sdk.EdgeAction, operationID, marker string, refs, proofs []string) sdk.EdgeOperationObservation {
	return succeededObservationWithOutputs(action, operationID, marker, refs, nil, proofs)
}

func succeededObservationWithOutputs(action sdk.EdgeAction, operationID, marker string, refs []string, outputs []sdk.AdapterOutput, proofs []string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationSucceeded, Action: action, OperationID: operationID, ResourceRefs: append([]string(nil), refs...), ProofRefs: append([]string(nil), proofs...), Outputs: append([]sdk.AdapterOutput(nil), outputs...), OwnershipMarker: marker, OwnershipVerified: true, IdempotencyVerified: true}
}

func failedObservation(action sdk.EdgeAction, operationID, detail string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationFailed, Action: action, OperationID: operationID, Detail: detail}
}

func optionalString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return aws.String(value)
}

func stringOption(configuration map[string]any, key string) string {
	value, _ := configuration[key].(string)
	return strings.TrimSpace(value)
}

func isNoSuchDistribution(err error) bool {
	var target *cloudfronttypes.NoSuchDistribution
	return errors.As(err, &target)
}
