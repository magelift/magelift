package edge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/sdk"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
)

type fakeGoogleEdgeAPI struct {
	applyCalls      int
	verifyCalls     int
	destroyCalls    int
	purgeCalls      int
	transitionCalls int
	state           GoogleEdgeState
	err             error
	inventory       []sdk.EdgeInventoryResource
	inventoryErr    error
}

func (api *fakeGoogleEdgeAPI) Apply(_ context.Context, request GoogleEdgeApplyRequest) (GoogleEdgeState, error) {
	api.applyCalls++
	if api.err != nil {
		return GoogleEdgeState{}, api.err
	}
	api.state = GoogleEdgeState{Project: request.Project, URLMapName: request.URLMapName, TargetProxyName: request.TargetProxyName, ForwardingName: request.ForwardingName, BackendService: "backend-1", ActiveBackendService: "backend-1", IPAddress: "203.0.113.10", DomainName: request.Domains[0], Invalidation: "operation-1"}
	return api.state, nil
}

func (api *fakeGoogleEdgeAPI) Verify(_ context.Context, request GoogleEdgeApplyRequest) (GoogleEdgeState, error) {
	api.verifyCalls++
	if api.err != nil {
		return GoogleEdgeState{}, api.err
	}
	if request.ExpectedBackendServiceURL != "" && api.state.ActiveBackendService != resourceNameFromURL(request.ExpectedBackendServiceURL) {
		return GoogleEdgeState{}, errors.New("fake GCP URL map is on the wrong backend")
	}
	return api.state, nil
}

func (api *fakeGoogleEdgeAPI) Transition(_ context.Context, request GoogleEdgeApplyRequest, target string) (GoogleEdgeState, error) {
	api.transitionCalls++
	if api.err != nil {
		return GoogleEdgeState{}, api.err
	}
	api.state.ActiveBackendService = resourceNameFromURL(target)
	return api.state, nil
}

func (api *fakeGoogleEdgeAPI) Destroy(_ context.Context, _ GoogleEdgeApplyRequest, state GoogleEdgeState) error {
	api.destroyCalls++
	if state.URLMapName == "" || state.ForwardingName == "" {
		return errors.New("destroy state is incomplete")
	}
	return api.err
}

func (api *fakeGoogleEdgeAPI) Purge(_ context.Context, request GoogleEdgeApplyRequest) (GoogleEdgeState, error) {
	api.purgeCalls++
	if api.err != nil {
		return GoogleEdgeState{}, api.err
	}
	api.state.Invalidation = "purge-1"
	if len(request.PurgePaths) == 0 {
		api.state.Invalidation = "purge-all"
	}
	return api.state, nil
}

func (api *fakeGoogleEdgeAPI) Inventory(_ context.Context, marker string) ([]sdk.EdgeInventoryResource, error) {
	if api.err != nil {
		return nil, api.err
	}
	if api.inventoryErr != nil {
		return nil, api.inventoryErr
	}
	if api.inventory != nil {
		return append([]sdk.EdgeInventoryResource(nil), api.inventory...), nil
	}
	if marker == "magelift/test/gcp-edge" {
		return nil, nil
	}
	return []sdk.EdgeInventoryResource{{Identity: "gcp.edge.forwarding-rule:unrelated", OwnershipMarker: "other", Owned: false, Live: true}}, nil
}

type fakeGoogleHealth struct {
	calls      int
	references []string
	err        error
}

func (probe *fakeGoogleHealth) VerifyOrigin(_ context.Context, reference string) error {
	probe.calls++
	probe.references = append(probe.references, reference)
	return probe.err
}

func googleEdgeTestRequest() sdk.EdgePlanRequest {
	return sdk.EdgePlanRequest{
		TargetProvider: "gcp", TargetRuntime: "gke-standard",
		Intent:        sdk.EdgeIntent{Mode: "native", NativeProvider: "cloud-cdn", OwnershipMarker: "magelift/test/gcp-edge", OriginHealthRef: "health/origin", Domains: []string{"shop.example.com"}, TLS: true, TLSMode: "managed", DNSMode: "external", PurgeOnDeploy: true, WAFPolicyRef: "armor/policy"},
		Configuration: map[string]any{"backendService": "backend-1", "sslCertificate": "certificate-1", "securityPolicy": "armor-1"},
	}
}

func TestGoogleEdgePlanIsProviderTypedAndSideEffectFree(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	native, err := NewNativeAPI(providerAPI, "project-1", &fakeGoogleHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := googleEdgeTestRequest()
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID != googleEdgeAdapterID || plan.OwnershipMarker != request.Intent.OwnershipMarker || providerAPI.applyCalls != 0 {
		t.Fatalf("plan = %#v, API = %#v", plan, providerAPI)
	}
	request.Configuration["backendService"] = "https://www.googleapis.com/compute/v1/projects/project-1/global/backendServices/backend-1"
	if _, err := native.Plan(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	request.Configuration["backendService"] = "backend/unsafe"
	if _, err := native.Plan(context.Background(), request); err == nil || !strings.Contains(err.Error(), "simple global name") {
		t.Fatalf("unsafe backend reference accepted: %v", err)
	}
}

func TestGoogleEdgePlanCarriesCDNBypassTLSDNSWAFAndPurgePolicy(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*sdk.EdgePlanRequest)
		wantCDN    bool
		wantPurge  []string
		wantPolicy string
	}{
		{
			name: "CDN cache and explicit purge paths",
			mutate: func(request *sdk.EdgePlanRequest) {
				request.Configuration["purgePaths"] = []any{"/products/*", "/"}
			},
			wantCDN: true, wantPurge: []string{"/", "/products/*"}, wantPolicy: "armor-1",
		},
		{
			name: "edge without CDN is an explicit cache bypass",
			mutate: func(request *sdk.EdgePlanRequest) {
				request.Intent.NativeProvider = "cloud-armor"
				request.Intent.PurgeOnDeploy = false
				request.Intent.WAFPolicyRef = ""
				delete(request.Configuration, "securityPolicy")
			},
			wantCDN: false, wantPolicy: "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := googleEdgeTestRequest()
			test.mutate(&request)
			native, err := NewNativeAPI(&fakeGoogleEdgeAPI{}, "project-1", &fakeGoogleHealth{})
			if err != nil {
				t.Fatal(err)
			}
			plan, err := native.Plan(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			planned := plan.Opaque.(googleEdgePlan).request
			if planned.CloudCDNRequired != test.wantCDN {
				t.Errorf("Cloud CDN required = %t, want %t", planned.CloudCDNRequired, test.wantCDN)
			}
			if !reflect.DeepEqual(planned.PurgePaths, test.wantPurge) {
				t.Errorf("purge paths = %v, want %v", planned.PurgePaths, test.wantPurge)
			}
			if planned.SecurityPolicyURL == "" && test.wantPolicy != "" {
				t.Errorf("security policy URL is empty, want configured policy")
			}
			if !strings.HasSuffix(planned.SSLCertificateURL, "/sslCertificates/certificate-1") {
				t.Errorf("TLS certificate URL = %q", planned.SSLCertificateURL)
			}
			if !reflect.DeepEqual(planned.Domains, []string{"shop.example.com"}) {
				t.Errorf("DNS domains = %v", planned.Domains)
			}
		})
	}
}

func TestComputeGoogleEdgeAPIReconcilesOwnedCertificateDrift(t *testing.T) {
	tests := []struct {
		name          string
		owned         bool
		wantSetCalls  int
		wantErrorText string
	}{
		{name: "rotates owned proxy", owned: true, wantSetCalls: 1},
		{name: "refuses foreign proxy", owned: false, wantSetCalls: 0, wantErrorText: "not owned"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			const (
				project       = "project-1"
				proxyName     = "magelift-proxy"
				urlMapURL     = "https://www.googleapis.com/compute/v1/projects/project-1/global/urlMaps/magelift-urlmap"
				oldCertURL    = "https://www.googleapis.com/compute/v1/projects/project-1/global/sslCertificates/old"
				newCertURL    = "https://www.googleapis.com/compute/v1/projects/project-1/global/sslCertificates/new"
				operationName = "rotate-certificate"
			)

			getCalls := 0
			setCalls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/compute/v1/projects/%s/global/targetHttpsProxies/%s", project, proxyName) {
					getCalls++
					certificates := []string{oldCertURL}
					if getCalls > 1 {
						certificates = []string{newCertURL}
					}
					description := googleOwnedPrefix + "magelift/test/gcp-edge"
					if !test.owned {
						description = "foreign ownership"
					}
					writeJSON(w, &compute.TargetHttpsProxy{Name: proxyName, Description: description, UrlMap: urlMapURL, SslCertificates: certificates})
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == fmt.Sprintf("/compute/v1/projects/%s/targetHttpsProxies/%s/setSslCertificates", project, proxyName) {
					setCalls++
					writeJSON(w, &compute.Operation{Name: operationName, Status: "PENDING"})
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == fmt.Sprintf("/compute/v1/projects/%s/global/operations/%s", project, operationName) {
					writeJSON(w, &compute.Operation{Name: operationName, Status: "DONE"})
					return
				}
				t.Logf("unexpected Compute API request: %s %s", r.Method, r.URL.Path)
				http.NotFound(w, r)
			}))
			defer server.Close()

			service, err := compute.NewService(context.Background(), option.WithEndpoint(server.URL+"/compute/v1/"), option.WithoutAuthentication(), option.WithHTTPClient(server.Client()))
			if err != nil {
				t.Fatal(err)
			}
			api, err := newComputeGoogleEdgeAPI(service, project)
			if err != nil {
				t.Fatal(err)
			}
			api.operationWait = time.Second
			api.pollInterval = time.Millisecond

			_, err = api.ensureTargetHTTPSProxy(context.Background(), GoogleEdgeApplyRequest{
				Project: project, OwnershipMarker: "magelift/test/gcp-edge", TargetProxyName: proxyName, SSLCertificateURL: newCertURL,
			}, googleOwnedPrefix+"magelift/test/gcp-edge", urlMapURL)
			if test.wantErrorText == "" && err != nil {
				t.Fatal(err)
			}
			if test.wantErrorText != "" && (err == nil || !strings.Contains(err.Error(), test.wantErrorText)) {
				t.Fatalf("error = %v, want substring %q", err, test.wantErrorText)
			}
			if setCalls != test.wantSetCalls {
				t.Fatalf("set SSL certificate calls = %d, want %d", setCalls, test.wantSetCalls)
			}
		})
	}
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, "%s", mustJSON(value))
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

func TestGoogleEdgePurgePathValidationIsDeterministic(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
	}{
		{name: "non-string path", value: []any{"/ok", 42}},
		{name: "empty list", value: []any{}},
		{name: "relative path", value: []any{"products"}},
		{name: "control character", value: []any{"/products\n"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := googleEdgeTestRequest()
			request.Configuration["purgePaths"] = test.value
			native, err := NewNativeAPI(&fakeGoogleEdgeAPI{}, "project-1", &fakeGoogleHealth{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := native.Plan(context.Background(), request); err == nil {
				t.Fatal("invalid purge paths were accepted")
			}
		})
	}
}

func TestGoogleEdgeHealthGateAndSharedLifecycle(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	health := &fakeGoogleHealth{err: errors.New("origin is unhealthy")}
	native, err := NewNativeAPI(providerAPI, "project-1", health)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), googleEdgeTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "gcp", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-health-fail", OwnershipMarker: plan.OwnershipMarker}}); err == nil || providerAPI.applyCalls != 0 {
		t.Fatalf("unhealthy origin was mutated: err=%v api=%#v", err, providerAPI)
	}
	health.err = nil
	adapter, err := NewNativeLifecycleAdapter(native, sdk.EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-1", OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if providerAPI.applyCalls != 1 || health.calls != 2 || len(result.ResourceRefs) < 4 || !containsGoogleString(result.ProofRefs, "gcp.edge.cloud-cdn") {
		t.Fatalf("apply = %#v health=%d api=%#v", result, health.calls, providerAPI)
	}
	if len(health.references) != 2 || health.references[0] != "health/origin" || health.references[1] != "health/origin" {
		t.Fatalf("health references = %#v", health.references)
	}
	destroy, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy-1", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: result.ResourceRefs})
	if err != nil {
		t.Fatal(err)
	}
	if destroy.Action != sdk.EdgeDestroy || providerAPI.destroyCalls != 1 {
		t.Fatalf("destroy = %#v api=%#v", destroy, providerAPI)
	}
}

func TestGoogleEdgeOriginGroupSupportsFailoverAndRollback(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	health := &fakeGoogleHealth{}
	native, err := NewNativeAPI(providerAPI, "project-1", health)
	if err != nil {
		t.Fatal(err)
	}
	request := googleEdgeTestRequest()
	request.Intent.FailoverPolicyRef = "gcp/failover-policy"
	request.Configuration["originGroup"] = map[string]any{
		"primary":            "backend-1",
		"secondary":          "backend-2",
		"secondaryHealthRef": "health/secondary",
	}
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewNativeLifecycleAdapter(native, sdk.EdgeOperationPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2})
	if err != nil {
		t.Fatal(err)
	}
	failover, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeFailover, IdempotencyKey: "failover-1", OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if providerAPI.transitionCalls != 1 || providerAPI.state.ActiveBackendService != "backend-2" || !containsGoogleString(failover.ProofRefs, "gcp.edge.failover-control-plane-converged") || containsGoogleString(failover.ProofRefs, "gcp.edge.traffic-failover-proven") {
		t.Fatalf("failover = %#v api = %#v", failover, providerAPI)
	}
	rollback, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeRollback, IdempotencyKey: "rollback-1", OwnershipMarker: plan.OwnershipMarker})
	if err != nil {
		t.Fatal(err)
	}
	if providerAPI.transitionCalls != 2 || providerAPI.state.ActiveBackendService != "backend-1" || !containsGoogleString(rollback.ProofRefs, "gcp.edge.rollback-control-plane-converged") {
		t.Fatalf("rollback = %#v api = %#v", rollback, providerAPI)
	}
	if len(health.references) != 8 {
		t.Fatalf("health references = %#v", health.references)
	}
}

func TestGoogleEdgeFailoverRequiresOriginGroup(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	native, err := NewNativeAPI(providerAPI, "project-1", &fakeGoogleHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := googleEdgeTestRequest()
	request.Intent.FailoverPolicyRef = "gcp/failover-policy"
	_, err = native.Plan(context.Background(), request)
	var capabilityErr sdk.EdgeCapabilityError
	if !errors.As(err, &capabilityErr) || capabilityErr.Status != sdk.EdgeCapabilityUnsupported || capabilityErr.Action != sdk.EdgeFailover {
		t.Fatalf("failover without origin group error = %v", err)
	}
}

func TestGoogleEdgePollRevalidatesProviderState(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	health := &fakeGoogleHealth{}
	native, err := NewNativeAPI(providerAPI, "project-1", health)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), googleEdgeTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	started, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "gcp", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-1", OwnershipMarker: plan.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}
	polled, err := native.Poll(context.Background(), started.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if polled.Status != sdk.EdgeOperationSucceeded || polled.Action != sdk.EdgeApply || providerAPI.verifyCalls != 1 || !containsGoogleString(polled.ProofRefs, "gcp.edge.purge-complete") {
		t.Fatalf("polled = %#v, api = %#v", polled, providerAPI)
	}
}

func TestGoogleEdgePollPreservesPurgeProofAfterCheckpointResume(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	native, err := NewNativeAPI(providerAPI, "project-1", &fakeGoogleHealth{})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := native.Plan(context.Background(), googleEdgeTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	started, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "gcp", Action: sdk.EdgeApply, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeApply, IdempotencyKey: "apply-checkpoint-resume", OwnershipMarker: plan.OwnershipMarker}})
	if err != nil {
		t.Fatal(err)
	}

	// Poll receives only the opaque operation ID, just as it would after a
	// durable certification checkpoint is loaded. The invalidation identity
	// must survive that boundary so the universal purge proof is retained.
	resumed, err := native.Poll(context.Background(), started.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != sdk.EdgeOperationSucceeded || !containsGoogleString(resumed.ProofRefs, "gcp.edge.purge-complete") || !containsGoogleString(resumed.ResourceRefs, "gcp.edge.invalidation:operation-1") {
		t.Fatalf("resumed = %#v", resumed)
	}
}

func TestGoogleEdgeDestroyPollWaitsForOwningInventory(t *testing.T) {
	providerAPI := &fakeGoogleEdgeAPI{}
	native, err := NewNativeAPI(providerAPI, "project-1", &fakeGoogleHealth{})
	if err != nil {
		t.Fatal(err)
	}
	request := googleEdgeTestRequest()
	plan, err := native.Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	providerAPI.inventory = []sdk.EdgeInventoryResource{{Identity: "gcp.edge.url-map:url-map", OwnershipMarker: plan.OwnershipMarker, Owned: true, Live: true}}
	started, err := native.Start(context.Background(), sdk.EdgeOperationRequest{Provider: "gcp", Action: sdk.EdgeDestroy, Request: sdk.EdgeExecutionRequest{Plan: plan, Action: sdk.EdgeDestroy, IdempotencyKey: "destroy-1", OwnershipMarker: plan.OwnershipMarker, ResourceReferences: []string{"gcp.edge.url-map:url-map", "gcp.edge.forwarding-rule:forwarding"}}})
	if err != nil {
		t.Fatal(err)
	}
	if started.Status != sdk.EdgeOperationPending {
		t.Fatalf("destroy start = %#v", started)
	}
	polled, err := native.Poll(context.Background(), started.OperationID)
	if err != nil || polled.Status != sdk.EdgeOperationPending {
		t.Fatalf("pending destroy poll = %#v, err = %v", polled, err)
	}
	providerAPI.inventory = nil
	polled, err = native.Poll(context.Background(), started.OperationID)
	if err != nil || polled.Status != sdk.EdgeOperationSucceeded || !containsGoogleString(polled.ProofRefs, "gcp.edge.owning-service-inventory-empty") {
		t.Fatalf("completed destroy poll = %#v, err = %v", polled, err)
	}
}

func TestGoogleEdgeDesiredResourceComparisonsRejectDrift(t *testing.T) {
	request := GoogleEdgeApplyRequest{Project: "project-1", OwnershipMarker: "magelift/test", OriginHealthRef: "health/origin", URLMapName: "urlmap", TargetProxyName: "proxy", ForwardingName: "forwarding", BackendServiceURL: "backend", SSLCertificateURL: "certificate", Domains: []string{"b.example.com", "a.example.com"}}
	comment := googleOwnedPrefix + request.OwnershipMarker
	if !urlMapMatchesRequest(&compute.UrlMap{Description: comment, DefaultService: request.BackendServiceURL, HostRules: []*compute.HostRule{{Hosts: []string{"a.example.com", "b.example.com"}, PathMatcher: "magelift-default"}}, PathMatchers: []*compute.PathMatcher{{Name: "magelift-default", DefaultService: request.BackendServiceURL}}}, request, comment, request.BackendServiceURL) {
		t.Fatal("equivalent URL map was rejected")
	}
	failover := request
	failover.BackendServiceURL = "https://www.googleapis.com/compute/v1/projects/project-1/global/backendServices/backend-1"
	failover.SecondaryBackendServiceURL = "https://www.googleapis.com/compute/v1/projects/project-1/global/backendServices/backend-2"
	failover.ExpectedBackendServiceURL = failover.SecondaryBackendServiceURL
	failover.Domains = []string{"shop.example.com"}
	current := &compute.UrlMap{Description: comment, DefaultService: failover.BackendServiceURL, HostRules: []*compute.HostRule{{Hosts: []string{"shop.example.com"}, PathMatcher: "magelift-default"}}, PathMatchers: []*compute.PathMatcher{{Name: "magelift-default", DefaultService: failover.BackendServiceURL}}}
	if urlMapMatchesRequest(current, failover, comment, failover.BackendServiceURL) {
		t.Fatal("failover expected backend accepted the still-primary URL map")
	}
	pre := failover
	pre.ExpectedBackendServiceURL = ""
	if !urlMapMatchesRequest(current, pre, comment, failover.BackendServiceURL) {
		t.Fatal("origin-group URL map on the primary backend was treated as unowned before failover")
	}
	proxy := &compute.TargetHttpsProxy{Description: comment, UrlMap: "urlmap-url", SslCertificates: []string{request.SSLCertificateURL}}
	if !targetHTTPSProxyMatchesRequest(proxy, request, comment, "urlmap-url") {
		t.Fatal("matching HTTPS proxy was rejected")
	}
	proxy.SslCertificates[0] = "wrong-certificate"
	if targetHTTPSProxyMatchesRequest(proxy, request, comment, "urlmap-url") {
		t.Fatal("certificate drift was accepted")
	}
	forwarding := &compute.ForwardingRule{Description: comment, Target: "proxy-url", IPProtocol: "TCP", PortRange: "443-443", LoadBalancingScheme: "EXTERNAL_MANAGED", NetworkTier: "PREMIUM"}
	if !forwardingMatchesRequest(forwarding, request, comment, "proxy-url") {
		t.Fatal("GCP forwarding rule port range 443-443 was rejected")
	}
}

func containsGoogleString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
