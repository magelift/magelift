package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"sync"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/ovh/go-ovh/ovh"
)

type fakeKubeLogsAPI struct {
	mu            sync.Mutex
	subscriptions map[string]OVHSubscription
	createCalls   int
	deleteCalls   int
	listCalls     int
	visibleAfter  int
	pendingCreate *OVHSubscription
}

func newFakeKubeLogsAPI() *fakeKubeLogsAPI {
	return &fakeKubeLogsAPI{subscriptions: map[string]OVHSubscription{}}
}

func (api *fakeKubeLogsAPI) ListSubscriptions(_ context.Context, _, _ string) ([]string, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.listCalls++
	if api.pendingCreate != nil && api.listCalls >= api.visibleAfter {
		api.subscriptions[api.pendingCreate.SubscriptionID] = *api.pendingCreate
		api.pendingCreate = nil
	}
	ids := make([]string, 0, len(api.subscriptions))
	for id := range api.subscriptions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

func (api *fakeKubeLogsAPI) GetSubscription(_ context.Context, _, _, id string) (OVHSubscription, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	return api.subscriptions[id], nil
}

func (api *fakeKubeLogsAPI) CreateSubscription(_ context.Context, serviceName, _, kind, streamID string) (OVHSubscriptionOperation, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.createCalls++
	api.pendingCreate = &OVHSubscription{Kind: kind, ServiceName: serviceName, StreamID: streamID, SubscriptionID: "subscription-1"}
	if api.visibleAfter == 0 {
		api.subscriptions[api.pendingCreate.SubscriptionID] = *api.pendingCreate
		api.pendingCreate = nil
	}
	return OVHSubscriptionOperation{OperationID: "operation-create-1", ServiceName: serviceName}, nil
}

func (api *fakeKubeLogsAPI) DeleteSubscription(_ context.Context, _, _, id string) (OVHSubscriptionOperation, error) {
	api.mu.Lock()
	defer api.mu.Unlock()
	api.deleteCalls++
	delete(api.subscriptions, id)
	return OVHSubscriptionOperation{OperationID: "operation-delete-1"}, nil
}

func TestOVHKubeAuditBackendReusesDelayedSubscriptionAndCleansOnlyOwnedStream(t *testing.T) {
	api := newFakeKubeLogsAPI()
	api.visibleAfter = 3
	other := OVHSubscription{Kind: ovhKubeAuditKind, ServiceName: "project", StreamID: "other-stream", SubscriptionID: "subscription-other"}
	api.subscriptions[other.SubscriptionID] = other
	backend, err := NewOVHKubeAuditLogsBackend("project", "cluster-1", "stream-1", api)
	if err != nil {
		t.Fatal(err)
	}
	backend.pollInterval = 0
	backend.pollAttempts = 4
	plan := providerobservability.Plan{
		TargetProvider:  "ovh",
		TargetRuntime:   "mks",
		OwnershipMarker: "magelift/architecture/test",
		Bindings: []providerobservability.SignalBinding{
			{Signal: "audit-events", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: "magelift/architecture/test"},
			{Signal: "provider-operations", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: "magelift/architecture/test"},
		},
	}
	result, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if api.createCalls != 1 {
		t.Fatalf("create calls = %d, want 1", api.createCalls)
	}
	wantRefs := []string{subscriptionReference("subscription-1", true)}
	if !reflect.DeepEqual(result.ResourceRefs, wantRefs) {
		t.Fatalf("resource refs = %v, want %v", result.ResourceRefs, wantRefs)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified {
		t.Fatalf("lifecycle proof = %#v", result)
	}
	inventory, err := backend.Inventory(context.Background(), plan.OwnershipMarker)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || inventory[0].Identity != subscriptionReference("subscription-1", true) || !inventory[0].Owned {
		t.Fatalf("inventory = %#v", inventory)
	}
	if err := backend.Destroy(context.Background(), plan, result.ResourceRefs); err != nil {
		t.Fatal(err)
	}
	if api.deleteCalls != 1 {
		t.Fatalf("delete calls = %d, want 1", api.deleteCalls)
	}
	if _, ok := api.subscriptions[other.SubscriptionID]; !ok {
		t.Fatal("cleanup deleted an unowned stream subscription")
	}
}

func TestOVHKubeAuditBackendPreservesPreexistingSubscription(t *testing.T) {
	api := newFakeKubeLogsAPI()
	api.subscriptions["subscription-existing"] = OVHSubscription{
		Kind: ovhKubeAuditKind, ServiceName: "project", StreamID: "stream-1", SubscriptionID: "subscription-existing",
	}
	backend, err := NewOVHKubeAuditLogsBackend("project", "cluster-1", "stream-1", api)
	if err != nil {
		t.Fatal(err)
	}
	plan := providerobservability.Plan{
		TargetProvider:  "ovh",
		TargetRuntime:   "mks",
		OwnershipMarker: "magelift/architecture/test",
		Bindings: []providerobservability.SignalBinding{
			{Signal: "audit-events", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: "magelift/architecture/test"},
		},
	}
	result, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	wantRef := subscriptionReference("subscription-existing", false)
	if !reflect.DeepEqual(result.ResourceRefs, []string{wantRef}) {
		t.Fatalf("resource refs = %v, want %v", result.ResourceRefs, []string{wantRef})
	}
	if api.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", api.createCalls)
	}
	if err := backend.Destroy(context.Background(), plan, result.ResourceRefs); err != nil {
		t.Fatal(err)
	}
	if api.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", api.deleteCalls)
	}
	if _, ok := api.subscriptions["subscription-existing"]; !ok {
		t.Fatal("cleanup deleted a pre-existing subscription")
	}
	inventory, err := backend.Inventory(context.Background(), plan.OwnershipMarker)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory) != 1 || inventory[0].Owned || inventory[0].OwnershipMarker != "" || inventory[0].Identity != wantRef {
		t.Fatalf("pre-existing inventory = %#v", inventory)
	}
}

func TestOVHKubeAuditBackendRefusesImplicitCleanup(t *testing.T) {
	api := newFakeKubeLogsAPI()
	api.subscriptions["subscription-existing"] = OVHSubscription{
		Kind: ovhKubeAuditKind, ServiceName: "project", StreamID: "stream-1", SubscriptionID: "subscription-existing",
	}
	backend, err := NewOVHKubeAuditLogsBackend("project", "cluster-1", "stream-1", api)
	if err != nil {
		t.Fatal(err)
	}
	plan := providerobservability.Plan{TargetProvider: "ovh", TargetRuntime: "mks", OwnershipMarker: "magelift/architecture/test"}
	if err := backend.Destroy(context.Background(), plan, nil); err == nil {
		t.Fatal("cleanup without references unexpectedly succeeded")
	}
	if api.deleteCalls != 0 {
		t.Fatalf("delete calls = %d, want 0", api.deleteCalls)
	}
}

func TestOVHKubeAuditBackendDoesNotClaimDestinationDelivery(t *testing.T) {
	api := newFakeKubeLogsAPI()
	api.subscriptions["subscription-1"] = OVHSubscription{Kind: ovhKubeAuditKind, ServiceName: "project", StreamID: "stream-1", SubscriptionID: "subscription-1"}
	backend, err := NewOVHKubeAuditLogsBackend("project", "cluster-1", "stream-1", api)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := backend.VerifySignal(context.Background(), providerobservability.SignalBinding{
		Signal: "audit-events", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: "magelift/architecture/test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified {
		t.Fatalf("unsafe OVH observation = %#v", observation)
	}
	if observation.Reason == "" {
		t.Fatal("missing explicit OVH delivery-proof gap")
	}
}

func TestOVHKubeAuditBackendRejectsRetentionAndRedactionBeforeMutation(t *testing.T) {
	api := newFakeKubeLogsAPI()
	backend, err := NewOVHKubeAuditLogsBackend("project", "cluster-1", "stream-1", api)
	if err != nil {
		t.Fatal(err)
	}
	base := providerobservability.Plan{
		TargetProvider:  "ovh",
		TargetRuntime:   "mks",
		OwnershipMarker: "magelift/architecture/test",
		Bindings:        []providerobservability.SignalBinding{{Signal: "audit-events", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: "magelift/architecture/test"}},
	}
	for name, binding := range map[string]providerobservability.SignalBinding{
		"retention": {Signal: "audit-events", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: base.OwnershipMarker, RetentionDays: 30},
		"redaction": {Signal: "audit-events", Destination: ovhLogsDataPlatformDestination, OwnershipMarker: base.OwnershipMarker, RedactionPolicy: "policy-1"},
	} {
		plan := base
		plan.Bindings = []providerobservability.SignalBinding{binding}
		if _, err := backend.Apply(context.Background(), plan); err == nil {
			t.Fatalf("%s plan unexpectedly applied", name)
		}
	}
	if api.createCalls != 0 {
		t.Fatalf("create calls = %d, want 0", api.createCalls)
	}
}

func TestSDKKubeLogsAPIUsesDocumentedMKSPaths(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/cloud/project/project/kube/cluster-1/log/subscription" && request.Method == http.MethodGet {
			_, _ = response.Write([]byte(`["subscription-1"]`))
			return
		}
		if request.URL.Path == "/cloud/project/project/kube/cluster-1/log/subscription/subscription-1" && request.Method == http.MethodGet {
			_, _ = response.Write([]byte(`{"kind":"audit","serviceName":"project","streamId":"stream-1","subscriptionId":"subscription-1"}`))
			return
		}
		if request.URL.Path == "/cloud/project/project/kube/cluster-1/log/subscription" && request.Method == http.MethodPost {
			_, _ = response.Write([]byte(`{"operationId":"operation-1","serviceName":"project"}`))
			return
		}
		if request.URL.Path == "/cloud/project/project/kube/cluster-1/log/subscription/subscription-1" && request.Method == http.MethodDelete {
			_, _ = response.Write([]byte(`{"operationId":"operation-2","serviceName":"project"}`))
			return
		}
		http.NotFound(response, request)
	}))
	defer server.Close()
	client, err := ovh.NewAccessTokenClient(server.URL, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	api := sdkKubeLogsAPI{client: client}
	ids, err := api.ListSubscriptions(context.Background(), "project", "cluster-1")
	if err != nil || !reflect.DeepEqual(ids, []string{"subscription-1"}) {
		t.Fatalf("list IDs = %v, err = %v", ids, err)
	}
	subscription, err := api.GetSubscription(context.Background(), "project", "cluster-1", "subscription-1")
	if err != nil || subscription.StreamID != "stream-1" {
		t.Fatalf("subscription = %#v, err = %v", subscription, err)
	}
	created, err := api.CreateSubscription(context.Background(), "project", "cluster-1", "audit", "stream-1")
	if err != nil || created.OperationID != "operation-1" {
		t.Fatalf("create operation = %#v, err = %v", created, err)
	}
	deleted, err := api.DeleteSubscription(context.Background(), "project", "cluster-1", "subscription-1")
	if err != nil || deleted.OperationID != "operation-2" {
		t.Fatalf("delete operation = %#v, err = %v", deleted, err)
	}
}
