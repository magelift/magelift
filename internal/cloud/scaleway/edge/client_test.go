package edge

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	edgeservices "github.com/scaleway/scaleway-sdk-go/api/edge_services/v1beta1"
	"github.com/scaleway/scaleway-sdk-go/scw"

	"github.com/magelift/magelift/sdk"
)

type fakeOriginHealth struct {
	err        error
	references []string
}

func (probe *fakeOriginHealth) VerifyOrigin(_ context.Context, reference string) error {
	probe.references = append(probe.references, reference)
	return probe.err
}

type fakeEdgeServicesAPI struct {
	pipelines       map[string]*edgeservices.Pipeline
	createdPipeline int
	deletedPipeline int
	createdBackend  int
	createdCache    int
	createdDNS      int
	createdTLS      int
	createdWAF      int
	createdPurge    int
	cacheRequests   []*edgeservices.CreateCacheStageRequest
	dnsRequests     []*edgeservices.CreateDNSStageRequest
	tlsRequests     []*edgeservices.CreateTLSStageRequest
	wafRequests     []*edgeservices.CreateWafStageRequest
	headRequests    []*edgeservices.SetHeadStageRequest
	purges          map[string]*edgeservices.PurgeRequest
	ready           bool
	getPipelineErr  error
}

func newFakeEdgeServicesAPI() *fakeEdgeServicesAPI {
	return &fakeEdgeServicesAPI{pipelines: make(map[string]*edgeservices.Pipeline), purges: make(map[string]*edgeservices.PurgeRequest), ready: true}
}

func (api *fakeEdgeServicesAPI) ListPipelinesWithStages(_ context.Context, request *edgeservices.ListPipelinesWithStagesRequest) (*edgeservices.ListPipelinesWithStagesResponse, error) {
	response := &edgeservices.ListPipelinesWithStagesResponse{}
	for _, pipeline := range api.pipelines {
		if request.ProjectID != nil && pipeline.ProjectID != *request.ProjectID {
			continue
		}
		response.Pipelines = append(response.Pipelines, &edgeservices.PipelineStages{Pipeline: pipeline})
	}
	response.TotalCount = uint64(len(response.Pipelines))
	return response, nil
}

func (api *fakeEdgeServicesAPI) CreatePipeline(_ context.Context, request *edgeservices.CreatePipelineRequest) (*edgeservices.Pipeline, error) {
	api.createdPipeline++
	id := "pipeline-1"
	pipeline := &edgeservices.Pipeline{ID: id, Name: request.Name, Description: request.Description, ProjectID: request.ProjectID, Status: edgeservices.PipelineStatusPending}
	api.pipelines[id] = pipeline
	return pipeline, nil
}

func (api *fakeEdgeServicesAPI) GetPipeline(_ context.Context, request *edgeservices.GetPipelineRequest) (*edgeservices.Pipeline, error) {
	if api.getPipelineErr != nil {
		return nil, api.getPipelineErr
	}
	pipeline, ok := api.pipelines[request.PipelineID]
	if !ok {
		return nil, &scw.ResourceNotFoundError{Resource: "pipeline", ResourceID: request.PipelineID}
	}
	if api.ready {
		pipeline.Status = edgeservices.PipelineStatusReady
	}
	return pipeline, nil
}

func (api *fakeEdgeServicesAPI) DeletePipeline(_ context.Context, request *edgeservices.DeletePipelineRequest) error {
	api.deletedPipeline++
	delete(api.pipelines, request.PipelineID)
	return nil
}

func (api *fakeEdgeServicesAPI) CreateBackendStage(_ context.Context, _ *edgeservices.CreateBackendStageRequest) (*edgeservices.BackendStage, error) {
	api.createdBackend++
	return &edgeservices.BackendStage{ID: "backend-1"}, nil
}

func (api *fakeEdgeServicesAPI) CreateCacheStage(_ context.Context, request *edgeservices.CreateCacheStageRequest) (*edgeservices.CacheStage, error) {
	api.createdCache++
	api.cacheRequests = append(api.cacheRequests, request)
	return &edgeservices.CacheStage{ID: "cache-1"}, nil
}

func (api *fakeEdgeServicesAPI) CreateDNSStage(_ context.Context, request *edgeservices.CreateDNSStageRequest) (*edgeservices.DNSStage, error) {
	api.createdDNS++
	api.dnsRequests = append(api.dnsRequests, request)
	return &edgeservices.DNSStage{ID: "dns-1"}, nil
}

func (api *fakeEdgeServicesAPI) CreateTLSStage(_ context.Context, request *edgeservices.CreateTLSStageRequest) (*edgeservices.TLSStage, error) {
	api.createdTLS++
	api.tlsRequests = append(api.tlsRequests, request)
	return &edgeservices.TLSStage{ID: "tls-1"}, nil
}

func (api *fakeEdgeServicesAPI) CreateWAFStage(_ context.Context, request *edgeservices.CreateWafStageRequest) (*edgeservices.WafStage, error) {
	api.createdWAF++
	api.wafRequests = append(api.wafRequests, request)
	return &edgeservices.WafStage{ID: "waf-1"}, nil
}

func (api *fakeEdgeServicesAPI) SetHeadStage(_ context.Context, request *edgeservices.SetHeadStageRequest) (*edgeservices.HeadStageResponse, error) {
	api.headRequests = append(api.headRequests, request)
	return &edgeservices.HeadStageResponse{}, nil
}

func (api *fakeEdgeServicesAPI) CreatePurgeRequest(_ context.Context, request *edgeservices.CreatePurgeRequestRequest) (*edgeservices.PurgeRequest, error) {
	api.createdPurge++
	id := "purge-1"
	purge := &edgeservices.PurgeRequest{ID: id, PipelineID: request.PipelineID, Status: edgeservices.PurgeRequestStatusDone}
	api.purges[id] = purge
	return purge, nil
}

func (api *fakeEdgeServicesAPI) GetPurgeRequest(_ context.Context, request *edgeservices.GetPurgeRequestRequest) (*edgeservices.PurgeRequest, error) {
	purge, ok := api.purges[request.PurgeRequestID]
	if !ok {
		return nil, errors.New("purge not found")
	}
	return purge, nil
}

func testEdgePlanRequest() sdk.EdgePlanRequest {
	return sdk.EdgePlanRequest{
		TargetProvider: "scaleway",
		TargetRuntime:  "kapsule",
		Intent: sdk.EdgeIntent{
			Mode:            "native",
			NativeProvider:  "scaleway-edge-services",
			OriginHealthRef: "health/origin",
			OwnershipMarker: "magelift/test/scaleway-edge",
			Domains:         []string{"shop.example.com"},
			TLS:             true,
			TLSMode:         "managed",
			DNSMode:         "provider",
		},
		Configuration: map[string]any{
			"origin": map[string]any{"kind": "load-balancer", "loadBalancerIDs": []any{"lb-1"}},
		},
	}
}

func TestNativeAPIPlansWithoutMutationAndRequiresProviderOriginConfiguration(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	probe := &fakeOriginHealth{}
	api, err := NewNativeAPI(providerAPI, "project-1", probe)
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	plan, err := api.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID != scalewayEdgeAdapterID || plan.OwnershipMarker != request.Intent.OwnershipMarker {
		t.Fatalf("plan = %#v", plan)
	}
	if providerAPI.createdPipeline != 0 || len(probe.references) != 0 {
		t.Fatalf("planning mutated provider: pipelines=%d health=%v", providerAPI.createdPipeline, probe.references)
	}
	request.Configuration = nil
	if _, err := api.Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "origin object") {
		t.Fatalf("missing origin error = %v", err)
	}
}

func TestNativeAPIPlanReturnsTypedFailoverCapabilityError(t *testing.T) {
	api, err := NewNativeAPI(newFakeEdgeServicesAPI(), "project-1", &fakeOriginHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	request.Intent.FailoverPolicyRef = "scaleway/failover"
	_, err = api.Plan(context.Background(), request)
	var capabilityErr sdk.EdgeCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Status != sdk.EdgeCapabilityUnsupported || capabilityErr.Action != sdk.EdgeFailover {
		t.Fatalf("typed failover error = %v", err)
	}
}

func TestNativeAPIAppliesPollsInventoriesAndDestroysOwnedPipeline(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	probe := &fakeOriginHealth{}
	api, err := NewNativeAPI(providerAPI, "project-1", probe)
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	plan, err := api.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	start, err := api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-1", OwnershipMarker: request.Intent.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}
	if start.Status != sdk.EdgeOperationPending || providerAPI.createdPipeline != 1 || providerAPI.createdBackend != 1 || providerAPI.createdTLS != 1 || providerAPI.createdDNS != 1 {
		t.Fatalf("apply start = %#v, api = %#v", start, providerAPI)
	}
	completed, err := api.Poll(context.Background(), start.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != sdk.EdgeOperationSucceeded || completed.OwnershipMarker != request.Intent.OwnershipMarker || len(completed.ProofRefs) != 1 || completed.ProofRefs[0] != "scaleway.edge."+sdk.EdgeProofOriginHealth {
		t.Fatalf("apply poll = %#v", completed)
	}
	resources, err := api.Inventory(context.Background(), request.Intent.OwnershipMarker)
	if err != nil || len(resources) != 1 || !resources[0].Live || !resources[0].Owned {
		t.Fatalf("inventory = %#v, err = %v", resources, err)
	}
	destroy, err := api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeDestroy, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy-1", OwnershipMarker: request.Intent.OwnershipMarker, ResourceReferences: completed.ResourceRefs}})
	if err != nil {
		t.Fatal(err)
	}
	if destroy.Status != sdk.EdgeOperationSucceeded || providerAPI.deletedPipeline != 1 {
		t.Fatalf("destroy = %#v, api = %#v", destroy, providerAPI)
	}
	resources, err = api.Inventory(context.Background(), request.Intent.OwnershipMarker)
	if err != nil || len(resources) != 0 {
		t.Fatalf("post-destroy inventory = %#v, err = %v", resources, err)
	}
}

func TestNativeAPIConfiguresCacheWAFManagedTLSAndDNSChain(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	api, err := NewNativeAPI(providerAPI, "project-1", &fakeOriginHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	request.Intent.CachePolicyRef = "cache/shop"
	request.Intent.WAFPolicyRef = "waf/shop"
	request.Configuration["cacheTTLSeconds"] = int64(300)
	request.Configuration["includeCookies"] = true
	request.Configuration["wafMode"] = string(edgeservices.WafStageModeEnable)
	request.Configuration["wafParanoiaLevel"] = int64(1)
	plan, err := api.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	started, err := api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "stages-1", OwnershipMarker: plan.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Poll(context.Background(), started.OperationID); err != nil {
		t.Fatal(err)
	}
	if providerAPI.createdWAF != 1 || providerAPI.createdCache != 1 || providerAPI.createdTLS != 1 || providerAPI.createdDNS != 1 || len(providerAPI.headRequests) != 1 {
		t.Fatalf("stage creation = %#v", providerAPI)
	}
	cache := providerAPI.cacheRequests[0]
	if cache.FallbackTTL == nil || cache.FallbackTTL.ToTimeDuration() == nil || *cache.FallbackTTL.ToTimeDuration() != 300*time.Second || cache.IncludeCookies == nil || !*cache.IncludeCookies || cache.WafStageID == nil || *cache.WafStageID != "waf-1" {
		t.Fatalf("cache stage request = %#v", cache)
	}
	waf := providerAPI.wafRequests[0]
	if waf.Mode != edgeservices.WafStageModeEnable || waf.ParanoiaLevel != 1 || waf.BackendStageID == nil || *waf.BackendStageID != "backend-1" {
		t.Fatalf("WAF stage request = %#v", waf)
	}
	tls := providerAPI.tlsRequests[0]
	if tls.ManagedCertificate == nil || !*tls.ManagedCertificate || tls.CacheStageID == nil || *tls.CacheStageID != "cache-1" {
		t.Fatalf("TLS stage request = %#v", tls)
	}
	dns := providerAPI.dnsRequests[0]
	if dns.Fqdns == nil || !reflect.DeepEqual(*dns.Fqdns, []string{"shop.example.com"}) || dns.TLSStageID == nil || *dns.TLSStageID != "tls-1" {
		t.Fatalf("DNS stage request = %#v", dns)
	}
	if providerAPI.headRequests[0].AddNewHeadStage == nil || providerAPI.headRequests[0].AddNewHeadStage.NewStageID != "dns-1" {
		t.Fatalf("head activation request = %#v", providerAPI.headRequests[0])
	}
}

func TestNativeAPIRejectsMagentoUnsafeWAFParanoia(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	api, err := NewNativeAPI(providerAPI, "project-1", &fakeOriginHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	request.Intent.WAFPolicyRef = "waf/shop"
	request.Configuration["wafParanoiaLevel"] = int64(3)
	_, err = api.Plan(context.Background(), request)
	if err == nil || !strings.Contains(err.Error(), "Magento-safe") {
		t.Fatalf("unsafe paranoia error = %v", err)
	}
	if providerAPI.createdWAF != 0 {
		t.Fatalf("unsafe WAF reached Scaleway: %#v", providerAPI)
	}
}

func TestNativeAPIHealthFailureHappensBeforeProviderMutation(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	probe := &fakeOriginHealth{err: errors.New("origin is unhealthy")}
	api, err := NewNativeAPI(providerAPI, "project-1", probe)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := api.Plan(context.Background(), testEdgePlanRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-health-fail", OwnershipMarker: "magelift/test/scaleway-edge"}})
	if err == nil || !strings.Contains(err.Error(), "origin health") {
		t.Fatalf("health failure = %v", err)
	}
	if providerAPI.createdPipeline != 0 {
		t.Fatalf("provider mutated before health gate: %#v", providerAPI)
	}
}

func TestNativeAPIPurgeRequiresDurableStoreAndResumesFromCheckpoint(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	probe := &fakeOriginHealth{}
	request := testEdgePlanRequest()
	request.Intent.PurgeOnDeploy = true

	withoutStore, err := NewNativeAPI(providerAPI, "project-1", probe)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := withoutStore.Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "durable purge request store") {
		t.Fatalf("purge plan without store error = %v", err)
	}
	if providerAPI.createdPipeline != 0 || providerAPI.createdPurge != 0 {
		t.Fatalf("purge plan mutated provider: %#v", providerAPI)
	}

	store := NewMemoryPurgeRequestStore()
	api, err := NewNativeAPIWithPurgeStore(providerAPI, "project-1", probe, store)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := api.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	start, err := api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "purge-apply", OwnershipMarker: request.Intent.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := api.Poll(context.Background(), start.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != sdk.EdgeOperationSucceeded || len(completed.ResourceRefs) != 2 || providerAPI.createdPurge != 1 {
		t.Fatalf("purge result = %#v, api = %#v", completed, providerAPI)
	}
	if _, err := api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "purge-apply", OwnershipMarker: request.Intent.OwnershipMarker, ResourceReferences: completed.ResourceRefs}}); err != nil {
		t.Fatal(err)
	}
	if providerAPI.createdPurge != 1 {
		t.Fatalf("checkpoint retry created a duplicate purge: %#v", providerAPI)
	}
}

func TestMemoryPurgeRequestStoreClaimsExactlyOneReservation(t *testing.T) {
	store := NewMemoryPurgeRequestStore()
	const callers = 16
	claims := make(chan PurgeRequestClaim, callers)
	errorsSeen := make(chan error, callers)
	var group sync.WaitGroup
	for range callers {
		group.Go(func() {
			claim, err := store.Claim(context.Background(), "apply-1", "pipeline-1")
			claims <- claim
			errorsSeen <- err
		})
	}
	group.Wait()
	close(claims)
	close(errorsSeen)

	var owner PurgeRequestClaim
	reservationCount := 0
	for claim := range claims {
		if claim.ReservationID != "" {
			owner = claim
			reservationCount++
		}
	}
	if reservationCount != 1 {
		t.Fatalf("reservation count = %d, want 1", reservationCount)
	}
	inFlightErrors := 0
	for err := range errorsSeen {
		if err == nil {
			continue
		}
		if !errors.Is(err, ErrPurgeRequestInFlight) {
			t.Fatalf("claim error = %v, want ErrPurgeRequestInFlight", err)
		}
		inFlightErrors++
	}
	if inFlightErrors != callers-1 {
		t.Fatalf("in-flight error count = %d, want %d", inFlightErrors, callers-1)
	}
	if err := store.Commit(context.Background(), "apply-1", "pipeline-1", owner.ReservationID, "purge-1"); err != nil {
		t.Fatal(err)
	}
	claim, err := store.Claim(context.Background(), "apply-1", "pipeline-1")
	if err != nil || claim.PurgeID != "purge-1" || claim.ReservationID != "" {
		t.Fatalf("committed claim = %#v, err = %v", claim, err)
	}
}

func TestNativeAPIDestroyPollOnlyTreatsOfficialNotFoundAsDeleted(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	api, err := NewNativeAPI(providerAPI, "project-1", &fakeOriginHealth{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Poll(context.Background(), pipelineOperationID(sdk.EdgeDestroy, "missing-pipeline")); err != nil {
		t.Fatalf("official not-found should be an idempotent delete: %v", err)
	}
	providerAPI.getPipelineErr = errors.New("provider unavailable")
	if _, err := api.Poll(context.Background(), pipelineOperationID(sdk.EdgeDestroy, "missing-pipeline")); err == nil {
		t.Fatal("provider read failure was incorrectly treated as deletion")
	}
}

func TestScalewayTLSParserAcceptsTypedObjectSlice(t *testing.T) {
	intent := testEdgePlanRequest().Intent
	intent.TLSMode = "custom"
	config := map[string]any{
		"tlsSecrets": []map[string]any{{"id": "secret-1", "region": "fr-par"}},
	}
	managed, secrets, err := parseTLS(intent, config)
	if err != nil || managed || len(secrets) != 1 || secrets[0].id != "secret-1" {
		t.Fatalf("typed TLS configuration = managed=%v secrets=%#v err=%v", managed, secrets, err)
	}
}

func TestNativeAPIPreservesUnownedPipelineWithMatchingName(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	marker := "magelift/test/scaleway-edge"
	name := pipelineName(marker)
	providerAPI.pipelines["user-pipeline"] = &edgeservices.Pipeline{ID: "user-pipeline", ProjectID: "project-1", Name: name, Description: "user-owned", Status: edgeservices.PipelineStatusReady}
	api, err := NewNativeAPI(providerAPI, "project-1", &fakeOriginHealth{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := api.Plan(context.Background(), testEdgePlanRequest())
	if err != nil {
		t.Fatal(err)
	}
	_, err = api.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "scaleway", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-owned-check", OwnershipMarker: marker}})
	if err == nil || !strings.Contains(err.Error(), "unowned") {
		t.Fatalf("unowned pipeline error = %v", err)
	}
	if providerAPI.createdPipeline != 0 {
		t.Fatalf("created a replacement beside an unowned pipeline: %#v", providerAPI)
	}
}

func TestNativeLifecycleUsesSharedPollingAndCleanupContract(t *testing.T) {
	providerAPI := newFakeEdgeServicesAPI()
	translator, err := NewNativeAPI(providerAPI, "project-1", &fakeOriginHealth{})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := NewNativeLifecycleAdapter(translator, sdk.EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 3})
	if err != nil {
		t.Fatal(err)
	}
	request := testEdgePlanRequest()
	plan, err := lifecycle.PlanEdge(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, err := lifecycle.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "shared-apply", OwnershipMarker: request.Intent.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ResourceRefs) != 1 {
		t.Fatalf("shared apply result = %#v", result)
	}
	if _, err := lifecycle.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "shared-destroy", OwnershipMarker: request.Intent.OwnershipMarker, ResourceReferences: result.ResourceRefs}); err != nil {
		t.Fatal(err)
	}
}
