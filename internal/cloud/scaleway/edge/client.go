package edge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	edgeservices "github.com/scaleway/scaleway-sdk-go/api/edge_services/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"

	"github.com/magelift/magelift/internal/edge/waf"
	sdk "github.com/magelift/magelift/sdk/v1"
)

const (
	scalewayEdgeAdapterID = "scaleway.edge.native"
	ownedPipelinePrefix   = "magelift-"
	ownedDescription      = "magelift ownership="
	resourcePrefix        = "scaleway.edge.pipeline:"
	operationPrefix       = "scaleway.edge"
)

// EdgeServicesAPI is the provider-owned subset of the current Scaleway Edge
// Services SDK used by the lifecycle translator. No generated SDK type leaves
// this package.
type EdgeServicesAPI interface {
	ListPipelinesWithStages(context.Context, *edgeservices.ListPipelinesWithStagesRequest) (*edgeservices.ListPipelinesWithStagesResponse, error)
	CreatePipeline(context.Context, *edgeservices.CreatePipelineRequest) (*edgeservices.Pipeline, error)
	GetPipeline(context.Context, *edgeservices.GetPipelineRequest) (*edgeservices.Pipeline, error)
	DeletePipeline(context.Context, *edgeservices.DeletePipelineRequest) error
	CreateBackendStage(context.Context, *edgeservices.CreateBackendStageRequest) (*edgeservices.BackendStage, error)
	CreateCacheStage(context.Context, *edgeservices.CreateCacheStageRequest) (*edgeservices.CacheStage, error)
	CreateDNSStage(context.Context, *edgeservices.CreateDNSStageRequest) (*edgeservices.DNSStage, error)
	CreateTLSStage(context.Context, *edgeservices.CreateTLSStageRequest) (*edgeservices.TLSStage, error)
	CreateWAFStage(context.Context, *edgeservices.CreateWafStageRequest) (*edgeservices.WafStage, error)
	SetHeadStage(context.Context, *edgeservices.SetHeadStageRequest) (*edgeservices.HeadStageResponse, error)
	CreatePurgeRequest(context.Context, *edgeservices.CreatePurgeRequestRequest) (*edgeservices.PurgeRequest, error)
	GetPurgeRequest(context.Context, *edgeservices.GetPurgeRequestRequest) (*edgeservices.PurgeRequest, error)
}

type sdkEdgeServicesAPI struct {
	api *edgeservices.API
}

func (api sdkEdgeServicesAPI) ListPipelinesWithStages(ctx context.Context, request *edgeservices.ListPipelinesWithStagesRequest) (*edgeservices.ListPipelinesWithStagesResponse, error) {
	return api.api.ListPipelinesWithStages(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreatePipeline(ctx context.Context, request *edgeservices.CreatePipelineRequest) (*edgeservices.Pipeline, error) {
	return api.api.CreatePipeline(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) GetPipeline(ctx context.Context, request *edgeservices.GetPipelineRequest) (*edgeservices.Pipeline, error) {
	return api.api.GetPipeline(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) DeletePipeline(ctx context.Context, request *edgeservices.DeletePipelineRequest) error {
	return api.api.DeletePipeline(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreateBackendStage(ctx context.Context, request *edgeservices.CreateBackendStageRequest) (*edgeservices.BackendStage, error) {
	return api.api.CreateBackendStage(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreateCacheStage(ctx context.Context, request *edgeservices.CreateCacheStageRequest) (*edgeservices.CacheStage, error) {
	return api.api.CreateCacheStage(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreateDNSStage(ctx context.Context, request *edgeservices.CreateDNSStageRequest) (*edgeservices.DNSStage, error) {
	return api.api.CreateDNSStage(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreateTLSStage(ctx context.Context, request *edgeservices.CreateTLSStageRequest) (*edgeservices.TLSStage, error) {
	return api.api.CreateTLSStage(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreateWAFStage(ctx context.Context, request *edgeservices.CreateWafStageRequest) (*edgeservices.WafStage, error) {
	return api.api.CreateWafStage(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) SetHeadStage(ctx context.Context, request *edgeservices.SetHeadStageRequest) (*edgeservices.HeadStageResponse, error) {
	return api.api.SetHeadStage(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) CreatePurgeRequest(ctx context.Context, request *edgeservices.CreatePurgeRequestRequest) (*edgeservices.PurgeRequest, error) {
	return api.api.CreatePurgeRequest(request, scw.WithContext(ctx))
}

func (api sdkEdgeServicesAPI) GetPurgeRequest(ctx context.Context, request *edgeservices.GetPurgeRequestRequest) (*edgeservices.PurgeRequest, error) {
	return api.api.GetPurgeRequest(request, scw.WithContext(ctx))
}

// OriginHealthProbe is deliberately injected. The edge lifecycle cannot
// safely publish a DNS head stage based only on a user-provided string; the
// caller must connect this probe to the runtime health contract it owns.
type OriginHealthProbe interface {
	VerifyOrigin(context.Context, string) error
}

// ErrPurgeRequestInFlight means another certification attempt owns an
// unresolved purge reservation. The caller must reconcile that reservation
// through its durable session store before attempting another provider
// mutation; creating a second purge would violate idempotency.
var ErrPurgeRequestInFlight = errors.New("Scaleway purge request has an unresolved reservation")

// PurgeRequestClaim is the result of an atomic purge reservation. A non-empty
// PurgeID is an already-committed provider request. A non-empty
// ReservationID is a new reservation owned by the caller and must be passed
// to Commit after the provider returns a purge identity.
//
// The IDs are opaque provider/session identities. They must never contain
// credentials or be copied into user-facing configuration.
type PurgeRequestClaim struct {
	PurgeID       string
	ReservationID string
}

// PurgeRequestStore is the provider-side durable coordination seam for
// Scaleway purge requests. Edge Services does not expose a portable
// idempotency token on CreatePurgeRequest, so a caller that enables purge-on-
// deploy must provide a store that survives process restarts and checkpoints.
// Claim must be an atomic conditional write: it may return an existing
// committed purge, create exactly one unresolved reservation, or return
// ErrPurgeRequestInFlight. Commit must bind the provider identity to the
// reservation without allowing a conflicting writer to replace it.
type PurgeRequestStore interface {
	Claim(context.Context, string, string) (PurgeRequestClaim, error)
	Commit(context.Context, string, string, string, string) error
}

// MemoryPurgeRequestStore is a deterministic store for contract tests and
// local dry runs. Production certification must replace it with a durable,
// conditional-write implementation owned by the certification session store.
type MemoryPurgeRequestStore struct {
	mu           sync.Mutex
	purges       map[string]purgeRequestRecord
	nextSequence uint64
}

type purgeRequestRecord struct {
	reservationID string
	purgeID       string
}

func NewMemoryPurgeRequestStore() *MemoryPurgeRequestStore {
	return &MemoryPurgeRequestStore{purges: make(map[string]purgeRequestRecord)}
}

func (store *MemoryPurgeRequestStore) Claim(ctx context.Context, key, pipelineID string) (PurgeRequestClaim, error) {
	if err := validatePurgeStoreInput(ctx, key, pipelineID); err != nil {
		return PurgeRequestClaim{}, err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record, found := store.purges[purgeStoreKey(key, pipelineID)]
	if found {
		if record.purgeID != "" {
			return PurgeRequestClaim{PurgeID: record.purgeID}, nil
		}
		return PurgeRequestClaim{}, fmt.Errorf("%w for key %q", ErrPurgeRequestInFlight, key)
	}
	store.nextSequence++
	reservationID := fmt.Sprintf("memory-reservation-%d", store.nextSequence)
	store.purges[purgeStoreKey(key, pipelineID)] = purgeRequestRecord{reservationID: reservationID}
	return PurgeRequestClaim{ReservationID: reservationID}, nil
}

func (store *MemoryPurgeRequestStore) Commit(ctx context.Context, key, pipelineID, reservationID, purgeID string) error {
	if err := validatePurgeStoreInput(ctx, key, pipelineID); err != nil {
		return err
	}
	if strings.TrimSpace(reservationID) == "" || strings.ContainsAny(reservationID, "\r\n\x00:") {
		return errors.New("Scaleway purge store reservation ID is required and must be opaque")
	}
	if strings.TrimSpace(purgeID) == "" || strings.ContainsAny(purgeID, "\r\n\x00:") {
		return errors.New("Scaleway purge store purge ID is required and must be opaque")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	storeKey := purgeStoreKey(key, pipelineID)
	record, found := store.purges[storeKey]
	if !found || record.reservationID != reservationID {
		return errors.New("Scaleway purge store rejects an unknown or mismatched reservation")
	}
	if record.purgeID != "" && record.purgeID != purgeID {
		return errors.New("Scaleway purge store rejects a conflicting purge identity")
	}
	record.purgeID = purgeID
	store.purges[storeKey] = record
	return nil
}

// NativeAPI translates the portable edge lifecycle to Scaleway Edge Services.
// Project identity and origin health stay at this provider boundary; the
// public SDK receives only opaque pipeline and proof identities.
type NativeAPI struct {
	api     EdgeServicesAPI
	project string
	health  OriginHealthProbe
	purges  PurgeRequestStore
}

// NewNativeAPI constructs the provider translator around the official SDK
// surface. A health probe is mandatory for apply and verify so an unhealthy
// origin cannot become the active edge head.
func NewNativeAPI(api EdgeServicesAPI, project string, health OriginHealthProbe) (*NativeAPI, error) {
	return newNativeAPI(api, project, health, nil)
}

// NewNativeAPIWithPurgeStore constructs a Scaleway translator with the
// durable coordination required by purge-on-deploy plans.
func NewNativeAPIWithPurgeStore(api EdgeServicesAPI, project string, health OriginHealthProbe, purges PurgeRequestStore) (*NativeAPI, error) {
	if purges == nil {
		return nil, errors.New("Scaleway Edge Services purge request store is required")
	}
	return newNativeAPI(api, project, health, purges)
}

func newNativeAPI(api EdgeServicesAPI, project string, health OriginHealthProbe, purges PurgeRequestStore) (*NativeAPI, error) {
	if api == nil {
		return nil, errors.New("Scaleway Edge Services API is required")
	}
	if strings.TrimSpace(project) == "" || strings.ContainsAny(project, "\r\n\x00") {
		return nil, errors.New("Scaleway Edge Services project is required and must be single-line")
	}
	if health == nil {
		return nil, errors.New("Scaleway Edge Services origin health probe is required")
	}
	return &NativeAPI{api: api, project: project, health: health, purges: purges}, nil
}

// NewScalewayEdgeServicesSDKClient creates a provider translator from the
// official Scaleway SDK. Credentials remain in scw.ClientOption and never
// enter the portable plan or evidence model.
func NewScalewayEdgeServicesSDKClient(ctx context.Context, project, region string, health OriginHealthProbe, opts ...scw.ClientOption) (*NativeAPI, error) {
	return newScalewayEdgeServicesSDKClient(ctx, project, region, health, nil, opts...)
}

// NewScalewayEdgeServicesSDKClientWithPurgeStore is the production constructor
// for profiles that enable purge-on-deploy.
func NewScalewayEdgeServicesSDKClientWithPurgeStore(ctx context.Context, project, region string, health OriginHealthProbe, purges PurgeRequestStore, opts ...scw.ClientOption) (*NativeAPI, error) {
	if purges == nil {
		return nil, errors.New("Scaleway Edge Services purge request store is required")
	}
	return newScalewayEdgeServicesSDKClient(ctx, project, region, health, purges, opts...)
}

func newScalewayEdgeServicesSDKClient(ctx context.Context, project, region string, health OriginHealthProbe, purges PurgeRequestStore, opts ...scw.ClientOption) (*NativeAPI, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway Edge Services context is required")
	}
	if strings.TrimSpace(region) == "" {
		return nil, errors.New("Scaleway Edge Services region is required")
	}
	clientOptions := append([]scw.ClientOption(nil), opts...)
	clientOptions = append(clientOptions, scw.WithDefaultProjectID(project), scw.WithDefaultRegion(scw.Region(region)))
	client, err := scw.NewClient(clientOptions...)
	if err != nil {
		return nil, errors.New("create Scaleway SDK client failed")
	}
	return newNativeAPI(sdkEdgeServicesAPI{api: edgeservices.NewAPI(client)}, project, health, purges)
}

type edgePlan struct {
	project         string
	pipelineName    string
	description     string
	origin          originConfig
	cache           cacheConfig
	waf             wafConfig
	domains         []string
	managedTLS      bool
	tlsSecrets      []tlsSecretConfig
	wildcardDomain  bool
	fullPrivate     bool
	purgeOnDeploy   bool
	originHealthRef string
	ownershipMarker string
}

type originConfig struct {
	kind            string
	bucketName      string
	bucketRegion    string
	isWebsite       bool
	loadBalancerIDs []string
	containerID     string
	functionID      string
	region          string
}

type cacheConfig struct {
	enabled        bool
	ttlSeconds     int64
	includeCookies bool
}

type wafConfig struct {
	enabled       bool
	mode          edgeservices.WafStageMode
	paranoiaLevel uint32
}

type tlsSecretConfig struct {
	id     string
	region string
}

func (api *NativeAPI) Plan(ctx context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if api == nil || api.api == nil {
		return sdk.EdgePlan{}, errors.New("Scaleway Edge Services API is required")
	}
	if ctx == nil {
		return sdk.EdgePlan{}, errors.New("Scaleway Edge Services planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.TargetProvider != sdk.ProviderID("scaleway") {
		return sdk.EdgePlan{}, fmt.Errorf("Scaleway Edge Services target provider must be scaleway, got %q", request.TargetProvider)
	}
	if err := sdk.ValidateEdgePlanRequest(request); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.Intent.Mode == "none" {
		return sdk.EdgePlan{AdapterID: scalewayEdgeAdapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, Opaque: edgePlan{}}, nil
	}
	if strings.TrimSpace(request.Intent.FailoverPolicyRef) != "" {
		return sdk.EdgePlan{}, sdk.EdgeCapabilityError{AdapterID: scalewayEdgeAdapterID, Action: sdk.EdgeFailover, Status: sdk.EdgeCapabilityUnsupported, Reason: "Scaleway Edge Services multi-backend routing is path/method based and does not provide a health-driven failover or rollback controller"}
	}
	config, err := parseEdgePlan(request)
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("parse Scaleway Edge Services configuration: %w", err)
	}
	if config.purgeOnDeploy && api.purges == nil {
		return sdk.EdgePlan{}, errors.New("Scaleway purge-on-deploy requires a durable purge request store")
	}
	config.project = api.project
	return sdk.EdgePlan{
		AdapterID:       scalewayEdgeAdapterID,
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		OwnershipMarker: request.Intent.OwnershipMarker,
		Outputs: []sdk.AdapterOutput{
			{Key: "pipelineName", Value: config.pipelineName},
			{Key: "originKind", Value: config.origin.kind},
		},
		Opaque: config,
	}, nil
}

func (api *NativeAPI) Start(ctx context.Context, operation sdk.EdgeOperationRequest) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway Edge Services API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway Edge Services context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	config, ok := operation.Request.Plan.Opaque.(edgePlan)
	if !ok {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway Edge Services plan is missing provider state")
	}
	if config.project != api.project {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway Edge Services plan project does not match the client")
	}
	if operation.Action == sdk.EdgeApply || operation.Action == sdk.EdgeVerify {
		if err := api.health.VerifyOrigin(ctx, config.originHealthRef); err != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("verify Scaleway edge origin health before mutation: %w", err)
		}
	}
	switch operation.Action {
	case sdk.EdgeApply:
		return api.startApply(ctx, operation, config)
	case sdk.EdgeVerify:
		pipelineID, err := pipelineIDFromReferences(operation.Request.ResourceReferences)
		if err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		return pendingOperation(sdk.EdgeVerify, pipelineOperationID(sdk.EdgeVerify, pipelineID), operation.Request.OwnershipMarker), nil
	case sdk.EdgeDestroy:
		pipelineID, err := pipelineIDFromReferences(operation.Request.ResourceReferences)
		if err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		if err := api.deleteOwnedPipeline(ctx, pipelineID, operation.Request.OwnershipMarker); err != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("delete Scaleway edge pipeline: %w", err)
		}
		return succeededOperation(sdk.EdgeDestroy, pipelineOperationID(sdk.EdgeDestroy, pipelineID), operation.Request.OwnershipMarker, []string{resourcePrefix + pipelineID}), nil
	default:
		return sdk.EdgeOperationObservation{}, fmt.Errorf("unsupported Scaleway edge action %q", operation.Action)
	}
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway Edge Services API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway Edge Services context is required")
	}
	action, kind, id, err := parseOperationID(operationID)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	if kind == "purge" {
		purge, err := api.api.GetPurgeRequest(ctx, &edgeservices.GetPurgeRequestRequest{PurgeRequestID: id})
		if err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		if purge == nil {
			return sdk.EdgeOperationObservation{}, errors.New("Scaleway purge response is empty")
		}
		switch purge.Status {
		case edgeservices.PurgeRequestStatusPending:
			return pendingOperation(action, operationID, ""), nil
		case edgeservices.PurgeRequestStatusDone:
			pipeline, pipelineErr := api.api.GetPipeline(ctx, &edgeservices.GetPipelineRequest{PipelineID: purge.PipelineID})
			if pipelineErr != nil {
				return sdk.EdgeOperationObservation{}, pipelineErr
			}
			if pipeline == nil {
				return sdk.EdgeOperationObservation{}, errors.New("Scaleway edge pipeline response is empty")
			}
			marker, markerErr := markerFromDescription(pipeline.Description)
			if markerErr != nil {
				return failedOperation(action, operationID, markerErr.Error()), nil
			}
			return succeededOperation(action, operationID, marker, []string{resourcePrefix + purge.PipelineID, "scaleway.edge.purge:" + id}), nil
		default:
			return failedOperation(action, operationID, "Scaleway Edge Services purge failed"), nil
		}
	}
	pipeline, err := api.api.GetPipeline(ctx, &edgeservices.GetPipelineRequest{PipelineID: id})
	if err != nil {
		if action == sdk.EdgeDestroy && isScalewayNotFound(err) {
			return succeededOperation(action, operationID, "", []string{resourcePrefix + id}), nil
		}
		return sdk.EdgeOperationObservation{}, err
	}
	if pipeline == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Scaleway edge pipeline response is empty")
	}
	if action == sdk.EdgeDestroy {
		return pendingOperation(action, operationID, ""), nil
	}
	switch pipeline.Status {
	case edgeservices.PipelineStatusPending:
		return pendingOperation(action, operationID, ""), nil
	case edgeservices.PipelineStatusReady:
		marker, markerErr := markerFromDescription(pipeline.Description)
		if markerErr != nil {
			return failedOperation(action, operationID, markerErr.Error()), nil
		}
		return succeededOperation(action, operationID, marker, []string{resourcePrefix + pipeline.ID}), nil
	case edgeservices.PipelineStatusError, edgeservices.PipelineStatusWarning, edgeservices.PipelineStatusLocked:
		return failedOperation(action, operationID, "Scaleway Edge Services pipeline is not healthy"), nil
	default:
		return pendingOperation(action, operationID, ""), nil
	}
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]sdk.EdgeInventoryResource, error) {
	if api == nil || api.api == nil {
		return nil, errors.New("Scaleway Edge Services API is required")
	}
	if ctx == nil {
		return nil, errors.New("Scaleway Edge Services inventory context is required")
	}
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("Scaleway Edge Services ownership marker is required and must be single-line")
	}
	all, err := api.listPipelines(ctx)
	if err != nil {
		return nil, err
	}
	resources := make([]sdk.EdgeInventoryResource, 0)
	for _, pipeline := range all {
		if pipeline == nil || pipeline.ProjectID != api.project || pipeline.Description != ownedDescription+marker {
			continue
		}
		live := pipeline.Status != edgeservices.PipelineStatusError
		resources = append(resources, sdk.EdgeInventoryResource{Identity: resourcePrefix + pipeline.ID, OwnershipMarker: marker, Owned: true, Live: live})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (api *NativeAPI) startApply(ctx context.Context, operation sdk.EdgeOperationRequest, config edgePlan) (sdk.EdgeOperationObservation, error) {
	pipelineID, err := pipelineIDFromReferences(operation.Request.ResourceReferences)
	created := false
	if err != nil {
		pipelineID, err = api.findOwnedPipeline(ctx, config.pipelineName, operation.Request.OwnershipMarker)
		if err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		if pipelineID == "" {
			pipeline, createErr := api.api.CreatePipeline(ctx, &edgeservices.CreatePipelineRequest{ProjectID: api.project, Name: config.pipelineName, Description: config.description})
			if createErr != nil {
				return sdk.EdgeOperationObservation{}, fmt.Errorf("create Scaleway edge pipeline: %w", createErr)
			}
			if pipeline == nil || strings.TrimSpace(pipeline.ID) == "" {
				return sdk.EdgeOperationObservation{}, errors.New("create Scaleway edge pipeline returned no identity")
			}
			pipelineID = pipeline.ID
			created = true
			if err := api.createStages(ctx, pipelineID, config); err != nil {
				cleanupErr := api.api.DeletePipeline(ctx, &edgeservices.DeletePipelineRequest{PipelineID: pipelineID})
				return sdk.EdgeOperationObservation{}, errors.Join(err, cleanupErr)
			}
		}
	} else if pipeline, err := api.api.GetPipeline(ctx, &edgeservices.GetPipelineRequest{PipelineID: pipelineID}); err != nil {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("load referenced Scaleway edge pipeline: %w", err)
	} else if pipeline == nil || pipeline.ProjectID != api.project || pipeline.Description != config.description {
		return sdk.EdgeOperationObservation{}, errors.New("referenced Scaleway edge pipeline is not owned by this plan")
	}
	if config.purgeOnDeploy {
		purgeID, found, lookupErr := api.purgeIDFromReferences(ctx, operation.Request.ResourceReferences, pipelineID)
		if lookupErr != nil {
			return sdk.EdgeOperationObservation{}, lookupErr
		}
		if !found {
			claim, claimErr := api.purges.Claim(ctx, operation.Request.IdempotencyKey, pipelineID)
			if claimErr != nil {
				return sdk.EdgeOperationObservation{}, fmt.Errorf("claim Scaleway edge purge idempotency record: %w", claimErr)
			}
			if claim.PurgeID != "" {
				purgeID = claim.PurgeID
			} else {
				if claim.ReservationID == "" {
					return sdk.EdgeOperationObservation{}, errors.New("Scaleway purge store returned an empty reservation")
				}
				purge, purgeErr := api.api.CreatePurgeRequest(ctx, &edgeservices.CreatePurgeRequestRequest{PipelineID: pipelineID, All: boolPointer(true)})
				if purgeErr != nil {
					return sdk.EdgeOperationObservation{}, fmt.Errorf("purge Scaleway edge pipeline: %w", purgeErr)
				}
				if purge == nil || strings.TrimSpace(purge.ID) == "" {
					return sdk.EdgeOperationObservation{}, errors.New("Scaleway edge purge returned no identity")
				}
				purgeID = purge.ID
				if err := api.purges.Commit(ctx, operation.Request.IdempotencyKey, pipelineID, claim.ReservationID, purgeID); err != nil {
					return sdk.EdgeOperationObservation{}, fmt.Errorf("commit Scaleway edge purge idempotency record %q: %w", purgeID, err)
				}
			}
		}
		purge, purgeErr := api.api.GetPurgeRequest(ctx, &edgeservices.GetPurgeRequestRequest{PurgeRequestID: purgeID})
		if purgeErr != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("load Scaleway edge purge %q: %w", purgeID, purgeErr)
		}
		if purge == nil || purge.PipelineID != pipelineID {
			return sdk.EdgeOperationObservation{}, errors.New("Scaleway edge purge does not match the owned pipeline")
		}
		return pendingOperation(sdk.EdgeApply, purgeOperationID(sdk.EdgeApply, purgeID), operation.Request.OwnershipMarker), nil
	}
	status := pendingOperation(sdk.EdgeApply, pipelineOperationID(sdk.EdgeApply, pipelineID), operation.Request.OwnershipMarker)
	if !created {
		status.Detail = "reusing an ownership-verified Scaleway edge pipeline"
	}
	return status, nil
}

func (api *NativeAPI) createStages(ctx context.Context, pipelineID string, config edgePlan) error {
	backendRequest := &edgeservices.CreateBackendStageRequest{PipelineID: pipelineID}
	switch config.origin.kind {
	case "s3":
		backendRequest.ScalewayS3 = &edgeservices.ScalewayS3BackendConfig{BucketName: stringPointer(config.origin.bucketName), BucketRegion: stringPointer(config.origin.bucketRegion), IsWebsite: boolPointer(config.origin.isWebsite)}
	case "load-balancer":
		loadBalancers := make([]*edgeservices.ScalewayLB, 0, len(config.origin.loadBalancerIDs))
		for _, id := range config.origin.loadBalancerIDs {
			loadBalancers = append(loadBalancers, &edgeservices.ScalewayLB{ID: id})
		}
		backendRequest.ScalewayLB = &edgeservices.ScalewayLBBackendConfig{LBs: loadBalancers}
	case "serverless-container":
		backendRequest.ScalewayServerlessContainer = &edgeservices.ScalewayServerlessContainerBackendConfig{Region: scw.Region(config.origin.region), ContainerID: config.origin.containerID}
	case "serverless-function":
		backendRequest.ScalewayServerlessFunction = &edgeservices.ScalewayServerlessFunctionBackendConfig{Region: scw.Region(config.origin.region), FunctionID: config.origin.functionID}
	default:
		return fmt.Errorf("unsupported Scaleway edge origin kind %q", config.origin.kind)
	}
	backend, err := api.api.CreateBackendStage(ctx, backendRequest)
	if err != nil {
		return fmt.Errorf("create Scaleway backend stage: %w", err)
	}
	if backend == nil || backend.ID == "" {
		return errors.New("Scaleway backend stage returned no identity")
	}
	head := stageReference{kind: "backend", id: backend.ID}
	if config.waf.enabled {
		waf, createErr := api.api.CreateWAFStage(ctx, &edgeservices.CreateWafStageRequest{PipelineID: pipelineID, Mode: config.waf.mode, ParanoiaLevel: config.waf.paranoiaLevel, BackendStageID: stringPointer(head.id)})
		if createErr != nil {
			return fmt.Errorf("create Scaleway WAF stage: %w", createErr)
		}
		if waf == nil || waf.ID == "" {
			return errors.New("Scaleway WAF stage returned no identity")
		}
		head = stageReference{kind: "waf", id: waf.ID}
	}
	if config.cache.enabled {
		cacheRequest := &edgeservices.CreateCacheStageRequest{PipelineID: pipelineID, FallbackTTL: scw.NewDurationFromTimeDuration(time.Duration(config.cache.ttlSeconds) * time.Second), IncludeCookies: boolPointer(config.cache.includeCookies)}
		setCacheParent(cacheRequest, head)
		cache, createErr := api.api.CreateCacheStage(ctx, cacheRequest)
		if createErr != nil {
			return fmt.Errorf("create Scaleway cache stage: %w", createErr)
		}
		if cache == nil || cache.ID == "" {
			return errors.New("Scaleway cache stage returned no identity")
		}
		head = stageReference{kind: "cache", id: cache.ID}
	}
	if config.managedTLS || len(config.tlsSecrets) > 0 {
		tlsRequest := &edgeservices.CreateTLSStageRequest{PipelineID: pipelineID, ManagedCertificate: boolPointer(config.managedTLS)}
		setTLSParent(tlsRequest, head)
		for _, secret := range config.tlsSecrets {
			tlsRequest.Secrets = append(tlsRequest.Secrets, &edgeservices.TLSSecret{SecretID: secret.id, Region: scw.Region(secret.region)})
		}
		tls, createErr := api.api.CreateTLSStage(ctx, tlsRequest)
		if createErr != nil {
			return fmt.Errorf("create Scaleway TLS stage: %w", createErr)
		}
		if tls == nil || tls.ID == "" {
			return errors.New("Scaleway TLS stage returned no identity")
		}
		head = stageReference{kind: "tls", id: tls.ID}
	}
	dnsRequest := &edgeservices.CreateDNSStageRequest{PipelineID: pipelineID, WildcardDomain: boolPointer(config.wildcardDomain), FullPrivate: boolPointer(config.fullPrivate)}
	if len(config.domains) > 0 {
		domains := append([]string(nil), config.domains...)
		dnsRequest.Fqdns = &domains
	}
	setDNSParent(dnsRequest, head)
	dns, err := api.api.CreateDNSStage(ctx, dnsRequest)
	if err != nil {
		return fmt.Errorf("create Scaleway DNS stage: %w", err)
	}
	if dns == nil || dns.ID == "" {
		return errors.New("Scaleway DNS stage returned no identity")
	}
	if _, err := api.api.SetHeadStage(ctx, &edgeservices.SetHeadStageRequest{PipelineID: pipelineID, AddNewHeadStage: &edgeservices.SetHeadStageRequestAddNewHeadStage{NewStageID: dns.ID}}); err != nil {
		return fmt.Errorf("activate Scaleway DNS head stage: %w", err)
	}
	return nil
}

func (api *NativeAPI) deleteOwnedPipeline(ctx context.Context, pipelineID, marker string) error {
	pipelines, err := api.listPipelines(ctx)
	if err != nil {
		return err
	}
	var pipeline *edgeservices.Pipeline
	for _, candidate := range pipelines {
		if candidate != nil && candidate.ID == pipelineID {
			pipeline = candidate
			break
		}
	}
	if pipeline == nil {
		// The resource reference came from a previous owned result. Absence from
		// the owning-service inventory is an idempotent delete success.
		return nil
	}
	if pipeline.Description != ownedDescription+marker || pipeline.ProjectID != api.project {
		return errors.New("refusing to delete a Scaleway edge pipeline without an ownership match")
	}
	return api.api.DeletePipeline(ctx, &edgeservices.DeletePipelineRequest{PipelineID: pipelineID})
}

func (api *NativeAPI) findOwnedPipeline(ctx context.Context, name, marker string) (string, error) {
	pipelines, err := api.listPipelines(ctx)
	if err != nil {
		return "", err
	}
	var found string
	for _, pipeline := range pipelines {
		if pipeline == nil || pipeline.ProjectID != api.project || pipeline.Name != name {
			continue
		}
		if pipeline.Description != ownedDescription+marker {
			return "", errors.New("Scaleway edge pipeline name is already used by an unowned resource")
		}
		if found != "" && found != pipeline.ID {
			return "", errors.New("multiple owned Scaleway edge pipelines match the deterministic name")
		}
		found = pipeline.ID
	}
	return found, nil
}

func (api *NativeAPI) listPipelines(ctx context.Context) ([]*edgeservices.Pipeline, error) {
	var result []*edgeservices.Pipeline
	for page := int32(1); ; page++ {
		response, err := api.api.ListPipelinesWithStages(ctx, &edgeservices.ListPipelinesWithStagesRequest{ProjectID: stringPointer(api.project), Page: &page, PageSize: uint32Pointer(100)})
		if err != nil {
			return nil, fmt.Errorf("list Scaleway edge pipelines: %w", err)
		}
		if response == nil {
			return nil, errors.New("list Scaleway edge pipelines returned no response")
		}
		for _, pipeline := range response.Pipelines {
			if pipeline != nil && pipeline.Pipeline != nil {
				result = append(result, pipeline.Pipeline)
			}
		}
		if len(response.Pipelines) == 0 || uint64(len(result)) >= response.TotalCount {
			return result, nil
		}
	}
}

func parseEdgePlan(request sdk.EdgePlanRequest) (edgePlan, error) {
	if strings.TrimSpace(request.Intent.OwnershipMarker) == "" {
		return edgePlan{}, errors.New("Scaleway edge ownership marker is required")
	}
	config := edgePlan{project: "", pipelineName: pipelineName(request.Intent.OwnershipMarker), description: ownedDescription + request.Intent.OwnershipMarker, originHealthRef: request.Intent.OriginHealthRef, ownershipMarker: request.Intent.OwnershipMarker, domains: append([]string(nil), request.Intent.Domains...), purgeOnDeploy: request.Intent.PurgeOnDeploy}
	var err error
	config.origin, err = parseOrigin(request.Intent, request.Configuration)
	if err != nil {
		return edgePlan{}, err
	}
	if err := validateOrigin(config.origin); err != nil {
		return edgePlan{}, err
	}
	config.pipelineName = stringValue(request.Configuration, "pipelineName", config.pipelineName)
	config.pipelineName = normalizePipelineName(config.pipelineName)
	config.cache, err = parseCache(request.Intent, request.Configuration)
	if err != nil {
		return edgePlan{}, err
	}
	config.waf, err = parseWAF(request.Intent, request.Configuration)
	if err != nil {
		return edgePlan{}, err
	}
	config.managedTLS, config.tlsSecrets, err = parseTLS(request.Intent, request.Configuration)
	if err != nil {
		return edgePlan{}, err
	}
	config.wildcardDomain = boolValue(request.Configuration, "wildcardDomain", false)
	config.fullPrivate = boolValue(request.Configuration, "fullPrivate", false)
	return config, nil
}

func parseOrigin(intent sdk.EdgeIntent, configuration map[string]any) (originConfig, error) {
	originValue, ok := configuration["origin"]
	if !ok {
		if strings.HasPrefix(intent.ServiceReference, "scaleway-lb:") {
			return originConfig{kind: "load-balancer", loadBalancerIDs: []string{strings.TrimPrefix(intent.ServiceReference, "scaleway-lb:")}}, nil
		}
		return originConfig{}, errors.New("Scaleway edge configuration requires an origin object")
	}
	origin, ok := originValue.(map[string]any)
	if !ok {
		return originConfig{}, errors.New("Scaleway edge origin must be an object")
	}
	result := originConfig{kind: stringValue(origin, "kind", ""), bucketName: stringValue(origin, "bucketName", ""), bucketRegion: stringValue(origin, "bucketRegion", ""), isWebsite: boolValue(origin, "isWebsite", false), containerID: stringValue(origin, "containerID", ""), functionID: stringValue(origin, "functionID", ""), region: stringValue(origin, "region", "")}
	result.loadBalancerIDs = stringSliceValue(origin, "loadBalancerIDs")
	if result.kind == "" && strings.HasPrefix(intent.ServiceReference, "scaleway-lb:") {
		result.kind = "load-balancer"
		result.loadBalancerIDs = []string{strings.TrimPrefix(intent.ServiceReference, "scaleway-lb:")}
	}
	return result, nil
}

func validateOrigin(origin originConfig) error {
	switch origin.kind {
	case "s3":
		if strings.TrimSpace(origin.bucketName) == "" || strings.TrimSpace(origin.bucketRegion) == "" {
			return errors.New("Scaleway S3 edge origin requires bucketName and bucketRegion")
		}
	case "load-balancer":
		if len(origin.loadBalancerIDs) == 0 {
			return errors.New("Scaleway load-balancer edge origin requires loadBalancerIDs")
		}
	case "serverless-container":
		if strings.TrimSpace(origin.containerID) == "" || strings.TrimSpace(origin.region) == "" {
			return errors.New("Scaleway serverless-container edge origin requires containerID and region")
		}
	case "serverless-function":
		if strings.TrimSpace(origin.functionID) == "" || strings.TrimSpace(origin.region) == "" {
			return errors.New("Scaleway serverless-function edge origin requires functionID and region")
		}
	default:
		return fmt.Errorf("Scaleway edge origin kind %q is unsupported", origin.kind)
	}
	return nil
}

func parseCache(intent sdk.EdgeIntent, configuration map[string]any) (cacheConfig, error) {
	if strings.TrimSpace(intent.CachePolicyRef) == "" {
		return cacheConfig{}, nil
	}
	ttl := int64Value(configuration, "cacheTTLSeconds", 0)
	if ttl <= 0 {
		return cacheConfig{}, errors.New("Scaleway edge cache requires a positive cacheTTLSeconds")
	}
	return cacheConfig{enabled: true, ttlSeconds: ttl, includeCookies: boolValue(configuration, "includeCookies", false)}, nil
}

func parseWAF(intent sdk.EdgeIntent, configuration map[string]any) (wafConfig, error) {
	if strings.TrimSpace(intent.WAFPolicyRef) == "" {
		return wafConfig{}, nil
	}
	mode := stringValue(configuration, "wafMode", string(edgeservices.WafStageModeEnable))
	if mode != string(edgeservices.WafStageModeDisable) && mode != string(edgeservices.WafStageModeLogOnly) && mode != string(edgeservices.WafStageModeEnable) {
		return wafConfig{}, fmt.Errorf("invalid Scaleway WAF mode %q", mode)
	}
	paranoia := int64Value(configuration, "wafParanoiaLevel", int64(waf.ScalewayMaxParanoiaLevel))
	if paranoia < 1 || paranoia > int64(waf.ScalewayMaxParanoiaLevel) {
		return wafConfig{}, fmt.Errorf("Magento-safe Scaleway WAF paranoia level must be %d", waf.ScalewayMaxParanoiaLevel)
	}
	return wafConfig{enabled: true, mode: edgeservices.WafStageMode(mode), paranoiaLevel: uint32(paranoia)}, nil
}

func parseTLS(intent sdk.EdgeIntent, configuration map[string]any) (bool, []tlsSecretConfig, error) {
	if !intent.TLS {
		return false, nil, nil
	}
	mode := strings.ToLower(strings.TrimSpace(intent.TLSMode))
	if mode == "managed" || boolValue(configuration, "managedCertificate", false) {
		return true, nil, nil
	}
	value, ok := configuration["tlsSecrets"]
	if !ok {
		return false, nil, errors.New("Scaleway custom TLS requires tlsSecrets references")
	}
	items, itemsErr := objectSliceValue(value)
	if itemsErr != nil || len(items) == 0 {
		return false, nil, errors.New("Scaleway tlsSecrets must be a non-empty array")
	}
	secrets := make([]tlsSecretConfig, 0, len(items))
	for _, item := range items {
		id := stringValue(item, "id", "")
		region := stringValue(item, "region", "")
		if id == "" || region == "" {
			return false, nil, errors.New("Scaleway TLS secret entries require id and region")
		}
		secrets = append(secrets, tlsSecretConfig{id: id, region: region})
	}
	return false, secrets, nil
}

func pipelineName(marker string) string {
	return ownedPipelinePrefix + markerDigest(marker)
}

func normalizePipelineName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || !strings.HasPrefix(value, ownedPipelinePrefix) {
		return pipelineName(value)
	}
	return value
}

func markerDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:16])
}

func markerFromDescription(description string) (string, error) {
	if !strings.HasPrefix(description, ownedDescription) {
		return "", errors.New("Scaleway edge pipeline ownership marker is missing")
	}
	marker := strings.TrimPrefix(description, ownedDescription)
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return "", errors.New("Scaleway edge pipeline ownership marker is invalid")
	}
	return marker, nil
}

func pipelineOperationID(action sdk.EdgeAction, pipelineID string) string {
	return operationPrefix + ":pipeline:" + string(action) + ":" + pipelineID
}

func purgeOperationID(action sdk.EdgeAction, purgeID string) string {
	return operationPrefix + ":purge:" + string(action) + ":" + purgeID
}

func parseOperationID(value string) (sdk.EdgeAction, string, string, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 4 || parts[0] != "scaleway.edge" || (parts[1] != "pipeline" && parts[1] != "purge") {
		return "", "", "", fmt.Errorf("invalid Scaleway edge operation ID %q", value)
	}
	action := sdk.EdgeAction(parts[2])
	if action != sdk.EdgeApply && action != sdk.EdgeVerify && action != sdk.EdgeDestroy {
		return "", "", "", fmt.Errorf("invalid Scaleway edge operation action %q", action)
	}
	if parts[3] == "" {
		return "", "", "", errors.New("Scaleway edge operation ID is incomplete")
	}
	return action, parts[1], parts[3], nil
}

func isScalewayNotFound(err error) bool {
	var notFound *scw.ResourceNotFoundError
	return errors.As(err, &notFound)
}

func pipelineIDFromReferences(references []string) (string, error) {
	var pipelineID string
	for _, reference := range references {
		if !strings.HasPrefix(reference, resourcePrefix) {
			continue
		}
		candidate := strings.TrimPrefix(reference, resourcePrefix)
		if candidate == "" || strings.ContainsAny(candidate, "\r\n\x00:") {
			return "", fmt.Errorf("invalid Scaleway edge pipeline reference %q", reference)
		}
		if pipelineID != "" && pipelineID != candidate {
			return "", errors.New("multiple Scaleway edge pipeline references were supplied")
		}
		pipelineID = candidate
	}
	if pipelineID == "" {
		return "", errors.New("Scaleway edge pipeline reference is required")
	}
	return pipelineID, nil
}

func (api *NativeAPI) purgeIDFromReferences(ctx context.Context, references []string, pipelineID string) (string, bool, error) {
	const prefix = "scaleway.edge.purge:"
	var purgeID string
	for _, reference := range references {
		if !strings.HasPrefix(reference, prefix) {
			continue
		}
		candidate := strings.TrimPrefix(reference, prefix)
		if candidate == "" || strings.ContainsAny(candidate, "\r\n\x00:") {
			return "", false, fmt.Errorf("invalid Scaleway edge purge reference %q", reference)
		}
		if purgeID != "" && purgeID != candidate {
			return "", false, errors.New("multiple Scaleway edge purge references were supplied")
		}
		purgeID = candidate
	}
	if purgeID == "" {
		return "", false, nil
	}
	purge, err := api.api.GetPurgeRequest(ctx, &edgeservices.GetPurgeRequestRequest{PurgeRequestID: purgeID})
	if err != nil {
		return "", false, fmt.Errorf("load referenced Scaleway edge purge: %w", err)
	}
	if purge == nil || purge.PipelineID != pipelineID {
		return "", false, errors.New("referenced Scaleway edge purge is not attached to the owned pipeline")
	}
	return purgeID, true, nil
}

func validatePurgeStoreInput(ctx context.Context, key, pipelineID string) error {
	if ctx == nil {
		return errors.New("Scaleway purge store context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(key) == "" || strings.ContainsAny(key, "\r\n\x00") {
		return errors.New("Scaleway purge store key is required and must be single-line")
	}
	if strings.TrimSpace(pipelineID) == "" || strings.ContainsAny(pipelineID, "\r\n\x00:") {
		return errors.New("Scaleway purge store pipeline ID is required and must be opaque")
	}
	return nil
}

func purgeStoreKey(key, pipelineID string) string { return key + "\x00" + pipelineID }

func pendingOperation(action sdk.EdgeAction, operationID, marker string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationPending, Action: action, OperationID: operationID, OwnershipMarker: marker, OwnershipVerified: marker != "", IdempotencyVerified: true}
}

func succeededOperation(action sdk.EdgeAction, operationID, marker string, refs []string) sdk.EdgeOperationObservation {
	var proofs []string
	if action == sdk.EdgeApply || action == sdk.EdgeVerify {
		proofs = []string{"scaleway.edge." + sdk.EdgeProofOriginHealth}
	}
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationSucceeded, Action: action, OperationID: operationID, ResourceRefs: append([]string(nil), refs...), ProofRefs: proofs, OwnershipMarker: marker, OwnershipVerified: marker != "", IdempotencyVerified: true}
}

func failedOperation(action sdk.EdgeAction, operationID, detail string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationFailed, Action: action, OperationID: operationID, IdempotencyVerified: true, Detail: detail}
}

func stringValue(values map[string]any, key, fallback string) string {
	value, ok := values[key].(string)
	if !ok {
		return fallback
	}
	return strings.TrimSpace(value)
}

func boolValue(values map[string]any, key string, fallback bool) bool {
	value, ok := values[key].(bool)
	if !ok {
		return fallback
	}
	return value
}

func int64Value(values map[string]any, key string, fallback int64) int64 {
	value, ok := values[key]
	if !ok {
		return fallback
	}
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func stringSliceValue(values map[string]any, key string) []string {
	value, ok := values[key]
	if !ok {
		return nil
	}
	var result []string
	switch typed := value.(type) {
	case []string:
		result = append(result, typed...)
	case []any:
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
			}
		}
	}
	sort.Strings(result)
	return result
}

func objectSliceValue(value any) ([]map[string]any, error) {
	switch typed := value.(type) {
	case []map[string]any:
		return append([]map[string]any(nil), typed...), nil
	case []any:
		objects := make([]map[string]any, 0, len(typed))
		for _, item := range typed {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, errors.New("array contains a non-object value")
			}
			objects = append(objects, object)
		}
		return objects, nil
	default:
		return nil, errors.New("value is not an object array")
	}
}

func stringPointer(value string) *string { return &value }

func boolPointer(value bool) *bool { return &value }

func uint32Pointer(value uint32) *uint32 { return &value }

type stageReference struct {
	kind string
	id   string
}

func setCacheParent(request *edgeservices.CreateCacheStageRequest, parent stageReference) {
	switch parent.kind {
	case "backend":
		request.BackendStageID = stringPointer(parent.id)
	case "waf":
		request.WafStageID = stringPointer(parent.id)
	case "route":
		request.RouteStageID = stringPointer(parent.id)
	}
}

func setTLSParent(request *edgeservices.CreateTLSStageRequest, parent stageReference) {
	switch parent.kind {
	case "backend":
		request.BackendStageID = stringPointer(parent.id)
	case "waf":
		request.WafStageID = stringPointer(parent.id)
	case "cache":
		request.CacheStageID = stringPointer(parent.id)
	case "route":
		request.RouteStageID = stringPointer(parent.id)
	}
}

func setDNSParent(request *edgeservices.CreateDNSStageRequest, parent stageReference) {
	switch parent.kind {
	case "backend":
		request.BackendStageID = stringPointer(parent.id)
	case "cache":
		request.CacheStageID = stringPointer(parent.id)
	case "tls":
		request.TLSStageID = stringPointer(parent.id)
	}
}
