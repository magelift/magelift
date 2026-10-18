package edge

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
)

const (
	googleEdgeAdapterID       = "gcp.edge.native"
	googleOwnedPrefix         = "magelift ownership="
	googleResourcePrefix      = "gcp.edge."
	googleEdgeOperationPrefix = "gcp.edge:v1:"
)

// GoogleEdgeAPI is the provider-owned lifecycle surface for a global external
// Application Load Balancer front end. The generated Compute API types stay
// inside this package; only semantic state crosses into the shared edge port.
type GoogleEdgeAPI interface {
	Apply(context.Context, GoogleEdgeApplyRequest) (GoogleEdgeState, error)
	Verify(context.Context, GoogleEdgeApplyRequest) (GoogleEdgeState, error)
	Destroy(context.Context, GoogleEdgeApplyRequest, GoogleEdgeState) error
	Purge(context.Context, GoogleEdgeApplyRequest) (GoogleEdgeState, error)
	Inventory(context.Context, string) ([]sdk.EdgeInventoryResource, error)
}

// GoogleEdgeApplyRequest is intentionally provider-owned. It captures the
// minimum official Compute resources needed to build a safe front end while
// allowing the workload runtime to own the backend service and NEG lifecycle.
type GoogleEdgeApplyRequest struct {
	Project                    string
	OwnershipMarker            string
	OriginHealthRef            string
	OriginHealthRefs           []string
	URLMapName                 string
	TargetProxyName            string
	ForwardingName             string
	BackendServiceURL          string
	SecondaryBackendServiceURL string
	ExpectedBackendServiceURL  string
	SSLCertificateURL          string
	SecurityPolicyURL          string
	CloudCDNRequired           bool
	PurgeOnDeploy              bool
	PurgePaths                 []string
	Domains                    []string
}

// GoogleEdgeState contains opaque resource identities and user-visible
// outputs. It does not expose generated Compute API models.
type GoogleEdgeState struct {
	Project              string
	URLMapName           string
	TargetProxyName      string
	ForwardingName       string
	BackendService       string
	ActiveBackendService string
	IPAddress            string
	DomainName           string
	Invalidation         string
}

// GoogleEdgeTransitionAPI is optional because provider implementations may
// expose only the common apply/verify/destroy path. The native Compute
// implementation uses URL-map backend switching for an explicit two-origin
// failover boundary; community implementations can opt into the same seam
// without importing Google API models into the SDK.
type GoogleEdgeTransitionAPI interface {
	Transition(context.Context, GoogleEdgeApplyRequest, string) (GoogleEdgeState, error)
}

// GoogleOriginHealthProbe is injected by the workload runtime and must prove
// application health before an edge route is created or verified.
type GoogleOriginHealthProbe interface {
	VerifyOrigin(context.Context, string) error
}

// AlwaysHealthy is an origin probe for purge-only adapters where Magento
// origin health is not part of the requested action.
type AlwaysHealthy struct{}

func (AlwaysHealthy) VerifyOrigin(context.Context, string) error { return nil }

// NativeAPI translates the public edge contract to an injected Google Cloud
// API implementation. The shared SDK owns composition and result validation;
// this package owns Google resource ordering and generated API types.
type NativeAPI struct {
	api     GoogleEdgeAPI
	project string
	health  GoogleOriginHealthProbe
}

func NewNativeAPI(api GoogleEdgeAPI, project string, health GoogleOriginHealthProbe) (*NativeAPI, error) {
	if api == nil {
		return nil, errors.New("Google Cloud edge API is required")
	}
	if strings.TrimSpace(project) == "" || strings.ContainsAny(project, "\r\n\x00") {
		return nil, errors.New("Google Cloud edge project is required and must be single-line")
	}
	if health == nil {
		return nil, errors.New("Google Cloud edge origin health probe is required")
	}
	return &NativeAPI{api: api, project: project, health: health}, nil
}

type googleEdgePlan struct {
	request GoogleEdgeApplyRequest
}

func (api *NativeAPI) Plan(ctx context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	if api == nil || api.api == nil {
		return sdk.EdgePlan{}, errors.New("Google Cloud edge API is required")
	}
	if ctx == nil {
		return sdk.EdgePlan{}, errors.New("Google Cloud edge planning context is required")
	}
	if err := ctx.Err(); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.TargetProvider != "gcp" {
		return sdk.EdgePlan{}, fmt.Errorf("Google Cloud edge target provider must be gcp, got %q", request.TargetProvider)
	}
	if err := sdk.ValidateEdgePlanRequest(request); err != nil {
		return sdk.EdgePlan{}, err
	}
	if request.Intent.Mode == "none" {
		return sdk.EdgePlan{AdapterID: googleEdgeAdapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime}, nil
	}
	if request.Intent.Mode != "native" && request.Intent.Mode != "both" {
		return sdk.EdgePlan{}, fmt.Errorf("Google Cloud edge adapter cannot handle edge mode %q", request.Intent.Mode)
	}
	if !validNativeProvider(request.Intent.NativeProvider) {
		return sdk.EdgePlan{}, fmt.Errorf("unsupported GCP native edge provider %q", request.Intent.NativeProvider)
	}
	if len(request.Intent.Domains) == 0 {
		return sdk.EdgePlan{}, errors.New("Google Cloud edge requires at least one domain")
	}
	if !request.Intent.TLS {
		return sdk.EdgePlan{}, errors.New("Google Cloud external HTTPS edge requires TLS")
	}
	backendURL, err := backendServiceURL(api.project, stringOption(request.Configuration, "backendService"))
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("parse GCP backend service: %w", err)
	}
	secondaryBackendURL, originHealthRefs, err := parseGoogleOriginGroup(api.project, request.Intent, request.Configuration, backendURL)
	if err != nil {
		return sdk.EdgePlan{}, err
	}
	certificateURL := stringOption(request.Configuration, "sslCertificate")
	if certificateURL == "" {
		return sdk.EdgePlan{}, errors.New("GCP TLS edge requires provider configuration sslCertificate")
	}
	certificateURL, err = globalResourceURL(api.project, certificateURL, "sslCertificates")
	if err != nil {
		return sdk.EdgePlan{}, fmt.Errorf("parse GCP SSL certificate: %w", err)
	}
	securityPolicyURL := stringOption(request.Configuration, "securityPolicy")
	if request.Intent.WAFPolicyRef != "" && securityPolicyURL == "" {
		return sdk.EdgePlan{}, errors.New("GCP WAFPolicyRef requires provider configuration securityPolicy")
	}
	if securityPolicyURL != "" {
		securityPolicyURL, err = globalResourceURL(api.project, securityPolicyURL, "securityPolicies")
		if err != nil {
			return sdk.EdgePlan{}, fmt.Errorf("parse GCP security policy: %w", err)
		}
	}
	purgePaths, err := parseGooglePurgePaths(request.Configuration, request.Intent.PurgeOnDeploy)
	if err != nil {
		return sdk.EdgePlan{}, err
	}
	marker := strings.TrimSpace(request.Intent.OwnershipMarker)
	digest := markerDigest(marker)
	planRequest := GoogleEdgeApplyRequest{
		Project: api.project, OwnershipMarker: marker, OriginHealthRef: request.Intent.OriginHealthRef, OriginHealthRefs: originHealthRefs,
		URLMapName: "magelift-" + digest + "-urlmap", TargetProxyName: "magelift-" + digest + "-https-proxy", ForwardingName: "magelift-" + digest + "-https-forwarding",
		BackendServiceURL: backendURL, SecondaryBackendServiceURL: secondaryBackendURL, SSLCertificateURL: certificateURL, SecurityPolicyURL: securityPolicyURL,
		CloudCDNRequired: request.Intent.NativeProvider == "cloud-cdn" || boolOption(request.Configuration, "cloudCDN"), PurgeOnDeploy: request.Intent.PurgeOnDeploy, PurgePaths: purgePaths,
		Domains: sortedCopy(request.Intent.Domains),
	}
	return sdk.EdgePlan{AdapterID: googleEdgeAdapterID, TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, OwnershipMarker: marker,
		Outputs: []sdk.AdapterOutput{{Key: "forwardingRuleName"}, {Key: "forwardingIPAddress"}, {Key: "targetHttpsProxyName"}, {Key: "urlMapName"}}, Opaque: googleEdgePlan{request: planRequest}}, nil
}

func (api *NativeAPI) Start(ctx context.Context, operation sdk.EdgeOperationRequest) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Google Cloud edge API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Google Cloud edge execution context is required")
	}
	if operation.Provider != "gcp" {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("Google Cloud edge operation provider must be gcp, got %q", operation.Provider)
	}
	plan, ok := operation.Request.Plan.Opaque.(googleEdgePlan)
	if !ok {
		return sdk.EdgeOperationObservation{}, errors.New("Google Cloud edge plan is missing provider configuration")
	}
	if plan.request.OwnershipMarker != operation.Request.OwnershipMarker {
		return sdk.EdgeOperationObservation{}, errors.New("Google Cloud edge ownership marker does not match the plan")
	}
	if operation.Action == sdk.EdgeApply || operation.Action == sdk.EdgeVerify || operation.Action == sdk.EdgeFailover || operation.Action == sdk.EdgeRollback {
		if err := verifyGoogleOriginHealth(ctx, api.health, plan.request); err != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("verify GCP edge origin health before mutation: %w", err)
		}
	}
	if operation.Action == sdk.EdgeFailover || operation.Action == sdk.EdgeRollback {
		transitionAPI, ok := api.api.(GoogleEdgeTransitionAPI)
		if !ok {
			return sdk.EdgeOperationObservation{}, sdk.EdgeCapabilityError{AdapterID: googleEdgeAdapterID, Action: operation.Action, Status: sdk.EdgeCapabilityUnsupported, Reason: "the GCP edge implementation does not expose a provider-owned two-origin transition"}
		}
		target := plan.request.SecondaryBackendServiceURL
		if operation.Action == sdk.EdgeRollback {
			target = plan.request.BackendServiceURL
		}
		transitionRequest := plan.request
		transitionRequest.ExpectedBackendServiceURL = target
		if _, err := transitionAPI.Transition(ctx, transitionRequest, target); err != nil {
			return sdk.EdgeOperationObservation{}, err
		}
		return pendingGoogleEdgeOperation(operation.Action, googleEdgeOperationID(operation.Action, transitionRequest, ""), operation.Request.OwnershipMarker, "GCP URL-map backend transition submitted"), nil
	}
	var state GoogleEdgeState
	var err error
	switch operation.Action {
	case sdk.EdgeApply:
		state, err = api.api.Apply(ctx, plan.request)
	case sdk.EdgeVerify:
		state, err = api.api.Verify(ctx, plan.request)
	case sdk.EdgeDestroy:
		state = stateFromReferences(plan.request, operation.Request.ResourceReferences)
		err = api.api.Destroy(ctx, plan.request, state)
	case sdk.EdgePurge:
		request := plan.request
		request.PurgeOnDeploy = true
		if len(request.PurgePaths) == 0 {
			request.PurgePaths = []string{"/*"}
		}
		state, err = api.api.Purge(ctx, request)
	default:
		return sdk.EdgeOperationObservation{}, fmt.Errorf("unsupported GCP edge action %q", operation.Action)
	}
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	operationID := googleEdgeOperationID(operation.Action, plan.request, state.Invalidation)
	if operation.Action == sdk.EdgeDestroy {
		remaining, inventoryErr := api.api.Inventory(ctx, plan.request.OwnershipMarker)
		if inventoryErr != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("verify GCP edge owning-service cleanup: %w", inventoryErr)
		}
		if len(remaining) > 0 {
			return pendingGoogleEdgeOperation(operation.Action, operationID, operation.Request.OwnershipMarker, "GCP edge resources remain in the owning-service inventory"), nil
		}
	}
	return googleEdgeObservation(operation.Action, operationID, plan.request, state), nil
}

func (api *NativeAPI) Poll(ctx context.Context, operationID string) (sdk.EdgeOperationObservation, error) {
	if api == nil || api.api == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Google Cloud edge API is required")
	}
	if ctx == nil {
		return sdk.EdgeOperationObservation{}, errors.New("Google Cloud edge polling context is required")
	}
	action, request, invalidation, err := parseGoogleEdgeOperationID(operationID)
	if err != nil {
		return sdk.EdgeOperationObservation{}, err
	}
	if request.Project != api.project {
		return sdk.EdgeOperationObservation{}, errors.New("GCP edge operation project does not match the client")
	}
	if action == sdk.EdgeDestroy {
		resources, inventoryErr := api.api.Inventory(ctx, request.OwnershipMarker)
		if inventoryErr != nil {
			return sdk.EdgeOperationObservation{}, fmt.Errorf("poll GCP edge owning-service cleanup: %w", inventoryErr)
		}
		if len(resources) > 0 {
			return pendingGoogleEdgeOperation(action, operationID, request.OwnershipMarker, "GCP edge resources remain in the owning-service inventory"), nil
		}
		return googleEdgeObservation(action, operationID, request, GoogleEdgeState{Project: request.Project, Invalidation: invalidation}), nil
	}
	if err := verifyGoogleOriginHealth(ctx, api.health, request); err != nil {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("verify GCP edge origin health while polling: %w", err)
	}
	state, err := api.api.Verify(ctx, request)
	if err != nil {
		return sdk.EdgeOperationObservation{}, fmt.Errorf("poll GCP edge verification: %w", err)
	}
	state.Invalidation = invalidation
	return googleEdgeObservation(action, operationID, request, state), nil
}

type googleEdgeOperation struct {
	Action       sdk.EdgeAction         `json:"action"`
	Request      GoogleEdgeApplyRequest `json:"request"`
	Invalidation string                 `json:"invalidation,omitempty"`
}

func googleEdgeOperationID(action sdk.EdgeAction, request GoogleEdgeApplyRequest, invalidation string) string {
	payload, _ := json.Marshal(googleEdgeOperation{Action: action, Request: request, Invalidation: invalidation})
	return googleEdgeOperationPrefix + base64.RawURLEncoding.EncodeToString(payload)
}

func parseGoogleEdgeOperationID(operationID string) (sdk.EdgeAction, GoogleEdgeApplyRequest, string, error) {
	const prefix = googleEdgeOperationPrefix
	if !strings.HasPrefix(operationID, prefix) {
		return "", GoogleEdgeApplyRequest{}, "", fmt.Errorf("invalid GCP edge operation ID %q", operationID)
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(operationID, prefix))
	if err != nil {
		return "", GoogleEdgeApplyRequest{}, "", fmt.Errorf("decode GCP edge operation ID: %w", err)
	}
	var operation googleEdgeOperation
	if err := json.Unmarshal(payload, &operation); err != nil {
		return "", GoogleEdgeApplyRequest{}, "", fmt.Errorf("decode GCP edge operation payload: %w", err)
	}
	if operation.Action != sdk.EdgeApply && operation.Action != sdk.EdgeVerify && operation.Action != sdk.EdgeFailover && operation.Action != sdk.EdgeRollback && operation.Action != sdk.EdgeDestroy {
		return "", GoogleEdgeApplyRequest{}, "", fmt.Errorf("invalid GCP edge operation action %q", operation.Action)
	}
	if err := validateGoogleEdgeApplyRequest(operation.Request); err != nil {
		return "", GoogleEdgeApplyRequest{}, "", fmt.Errorf("validate GCP edge operation payload: %w", err)
	}
	if strings.ContainsAny(operation.Invalidation, "\r\n\x00") {
		return "", GoogleEdgeApplyRequest{}, "", errors.New("GCP edge operation invalidation identity must be single-line")
	}
	return operation.Action, operation.Request, operation.Invalidation, nil
}

func googleEdgeObservation(action sdk.EdgeAction, operationID string, request GoogleEdgeApplyRequest, state GoogleEdgeState) sdk.EdgeOperationObservation {
	proofs := []string{"gcp.edge.ownership", "gcp.edge.origin-health", "gcp.edge.tls", "gcp.edge.security-policy"}
	if request.SecondaryBackendServiceURL != "" {
		proofs = append(proofs, "gcp.edge.origin-failover-configured")
	}
	if action == sdk.EdgeFailover {
		proofs = append(proofs, "gcp.edge.failover-control-plane-converged")
	}
	if action == sdk.EdgeRollback {
		proofs = append(proofs, "gcp.edge.rollback-control-plane-converged")
	}
	if request.CloudCDNRequired {
		proofs = append(proofs, "gcp.edge.cloud-cdn")
	}
	if request.PurgeOnDeploy && state.Invalidation != "" {
		proofs = append(proofs, "gcp.edge.purge-complete")
	}
	if action == sdk.EdgeDestroy {
		proofs = []string{"gcp.edge.ownership", "gcp.edge.owning-service-inventory-empty"}
	}
	sort.Strings(proofs)
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationSucceeded, Action: action, OperationID: operationID,
		ResourceRefs: stateReferences(state), ProofRefs: proofs, Outputs: stateOutputs(state), OwnershipMarker: request.OwnershipMarker, OwnershipVerified: true, IdempotencyVerified: true}
}

func pendingGoogleEdgeOperation(action sdk.EdgeAction, operationID, marker, detail string) sdk.EdgeOperationObservation {
	return sdk.EdgeOperationObservation{Status: sdk.EdgeOperationPending, Action: action, OperationID: operationID, OwnershipMarker: marker, OwnershipVerified: true, IdempotencyVerified: true, Detail: detail}
}

func (api *NativeAPI) Inventory(ctx context.Context, marker string) ([]sdk.EdgeInventoryResource, error) {
	if api == nil || api.api == nil {
		return nil, errors.New("Google Cloud edge API is required")
	}
	return api.api.Inventory(ctx, marker)
}

// computeGoogleEdgeAPI is the official Compute Engine API implementation.
// It owns resource ordering, long-running operation polling, and exact
// ownership descriptions; the public SDK sees only GoogleEdgeState.
type computeGoogleEdgeAPI struct {
	service       *compute.Service
	project       string
	operationWait time.Duration
	pollInterval  time.Duration
}

func newComputeGoogleEdgeAPI(service *compute.Service, project string) (*computeGoogleEdgeAPI, error) {
	if service == nil {
		return nil, errors.New("Google Compute service is required")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("Google Compute project is required")
	}
	return &computeGoogleEdgeAPI{service: service, project: project, operationWait: 30 * time.Minute, pollInterval: 2 * time.Second}, nil
}

func (api *computeGoogleEdgeAPI) Apply(ctx context.Context, request GoogleEdgeApplyRequest) (GoogleEdgeState, error) {
	if err := validateGoogleEdgeApplyRequest(request); err != nil {
		return GoogleEdgeState{}, err
	}
	comment := googleOwnedPrefix + request.OwnershipMarker
	backend, err := api.service.BackendServices.Get(request.Project, resourceNameFromURL(request.BackendServiceURL)).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP backend service: %w", err)
	}
	if backend == nil || backend.SelfLink == "" {
		return GoogleEdgeState{}, errors.New("GCP backend service response is empty")
	}
	if request.CloudCDNRequired && !backend.EnableCDN {
		return GoogleEdgeState{}, errors.New("GCP backend service does not have Cloud CDN enabled")
	}
	if request.SecurityPolicyURL != "" && backend.SecurityPolicy != request.SecurityPolicyURL {
		return GoogleEdgeState{}, errors.New("GCP backend service is not attached to the requested Cloud Armor policy")
	}
	if request.SecondaryBackendServiceURL != "" {
		secondary, secondaryErr := api.service.BackendServices.Get(request.Project, resourceNameFromURL(request.SecondaryBackendServiceURL)).Context(ctx).Do()
		if secondaryErr != nil {
			return GoogleEdgeState{}, fmt.Errorf("read GCP secondary backend service: %w", secondaryErr)
		}
		if secondary == nil || secondary.SelfLink == "" {
			return GoogleEdgeState{}, errors.New("GCP secondary backend service response is empty")
		}
		if request.CloudCDNRequired && !secondary.EnableCDN {
			return GoogleEdgeState{}, errors.New("GCP secondary backend service does not have Cloud CDN enabled")
		}
		if request.SecurityPolicyURL != "" && secondary.SecurityPolicy != request.SecurityPolicyURL {
			return GoogleEdgeState{}, errors.New("GCP secondary backend service is not attached to the requested Cloud Armor policy")
		}
	}
	urlMap, err := api.ensureURLMap(ctx, request, comment, backend.SelfLink)
	if err != nil {
		return GoogleEdgeState{}, err
	}
	proxy, err := api.ensureTargetHTTPSProxy(ctx, request, comment, urlMap.SelfLink)
	if err != nil {
		return GoogleEdgeState{}, err
	}
	forwarding, err := api.ensureForwardingRule(ctx, request, comment, proxy.SelfLink)
	if err != nil {
		return GoogleEdgeState{}, err
	}
	state := GoogleEdgeState{Project: request.Project, URLMapName: request.URLMapName, TargetProxyName: request.TargetProxyName, ForwardingName: request.ForwardingName, BackendService: resourceNameFromURL(request.BackendServiceURL), ActiveBackendService: resourceNameFromURL(urlMap.DefaultService), IPAddress: forwarding.IPAddress, DomainName: firstString(request.Domains)}
	if request.PurgeOnDeploy {
		invalidation, invalidationErr := api.invalidateCache(ctx, request)
		if invalidationErr != nil {
			return GoogleEdgeState{}, invalidationErr
		}
		state.Invalidation = invalidation
	}
	return state, nil
}

func (api *computeGoogleEdgeAPI) Purge(ctx context.Context, request GoogleEdgeApplyRequest) (GoogleEdgeState, error) {
	if err := validateGoogleEdgeApplyRequest(request); err != nil {
		return GoogleEdgeState{}, err
	}
	if len(request.PurgePaths) == 0 {
		request.PurgePaths = []string{"/*"}
	}
	invalidation, err := api.invalidateCache(ctx, request)
	if err != nil {
		return GoogleEdgeState{}, err
	}
	return GoogleEdgeState{Project: request.Project, URLMapName: request.URLMapName, TargetProxyName: request.TargetProxyName, ForwardingName: request.ForwardingName, Invalidation: invalidation}, nil
}

func (api *computeGoogleEdgeAPI) Verify(ctx context.Context, request GoogleEdgeApplyRequest) (GoogleEdgeState, error) {
	if err := validateGoogleEdgeApplyRequest(request); err != nil {
		return GoogleEdgeState{}, err
	}
	backend, err := api.service.BackendServices.Get(request.Project, resourceNameFromURL(request.BackendServiceURL)).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP backend service: %w", err)
	}
	if backend == nil || backend.SelfLink == "" {
		return GoogleEdgeState{}, errors.New("GCP backend service response is empty")
	}
	if request.CloudCDNRequired && !backend.EnableCDN {
		return GoogleEdgeState{}, errors.New("GCP backend service does not have Cloud CDN enabled")
	}
	if request.SecurityPolicyURL != "" && backend.SecurityPolicy != request.SecurityPolicyURL {
		return GoogleEdgeState{}, errors.New("GCP backend service is not attached to the requested Cloud Armor policy")
	}
	if request.SecondaryBackendServiceURL != "" {
		secondary, secondaryErr := api.service.BackendServices.Get(request.Project, resourceNameFromURL(request.SecondaryBackendServiceURL)).Context(ctx).Do()
		if secondaryErr != nil {
			return GoogleEdgeState{}, fmt.Errorf("read GCP secondary backend service: %w", secondaryErr)
		}
		if secondary == nil || secondary.SelfLink == "" {
			return GoogleEdgeState{}, errors.New("GCP secondary backend service response is empty")
		}
		if request.CloudCDNRequired && !secondary.EnableCDN {
			return GoogleEdgeState{}, errors.New("GCP secondary backend service does not have Cloud CDN enabled")
		}
		if request.SecurityPolicyURL != "" && secondary.SecurityPolicy != request.SecurityPolicyURL {
			return GoogleEdgeState{}, errors.New("GCP secondary backend service is not attached to the requested Cloud Armor policy")
		}
	}
	forwarding, err := api.service.GlobalForwardingRules.Get(request.Project, request.ForwardingName).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP forwarding rule: %w", err)
	}
	proxy, err := api.service.TargetHttpsProxies.Get(request.Project, request.TargetProxyName).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP target HTTPS proxy: %w", err)
	}
	urlMap, err := api.service.UrlMaps.Get(request.Project, request.URLMapName).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP URL map: %w", err)
	}
	comment := googleOwnedPrefix + request.OwnershipMarker
	if !forwardingMatchesRequest(forwarding, request, comment, proxy.SelfLink) || !targetHTTPSProxyMatchesRequest(proxy, request, comment, urlMap.SelfLink) || !urlMapMatchesRequest(urlMap, request, comment, backend.SelfLink) {
		return GoogleEdgeState{}, errors.New("GCP edge resources do not match the requested ownership or route")
	}
	return GoogleEdgeState{Project: request.Project, URLMapName: request.URLMapName, TargetProxyName: request.TargetProxyName, ForwardingName: request.ForwardingName, BackendService: resourceNameFromURL(request.BackendServiceURL), ActiveBackendService: resourceNameFromURL(urlMap.DefaultService), IPAddress: forwarding.IPAddress, DomainName: firstString(request.Domains)}, nil
}

func (api *computeGoogleEdgeAPI) Transition(ctx context.Context, request GoogleEdgeApplyRequest, target string) (GoogleEdgeState, error) {
	if err := validateGoogleEdgeApplyRequest(request); err != nil {
		return GoogleEdgeState{}, err
	}
	if request.SecondaryBackendServiceURL == "" {
		return GoogleEdgeState{}, sdk.EdgeCapabilityError{AdapterID: googleEdgeAdapterID, Action: sdk.EdgeFailover, Status: sdk.EdgeCapabilityUnsupported, Reason: "GCP edge failover requires a two-origin group"}
	}
	if target != request.BackendServiceURL && target != request.SecondaryBackendServiceURL {
		return GoogleEdgeState{}, errors.New("GCP edge transition target is outside the planned origin group")
	}
	backend, err := api.service.BackendServices.Get(request.Project, resourceNameFromURL(target)).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP transition backend service: %w", err)
	}
	if backend == nil || backend.SelfLink == "" {
		return GoogleEdgeState{}, errors.New("GCP transition backend service response is empty")
	}
	if request.CloudCDNRequired && !backend.EnableCDN {
		return GoogleEdgeState{}, errors.New("GCP transition backend service does not have Cloud CDN enabled")
	}
	if request.SecurityPolicyURL != "" && backend.SecurityPolicy != request.SecurityPolicyURL {
		return GoogleEdgeState{}, errors.New("GCP transition backend service is not attached to the requested Cloud Armor policy")
	}
	urlMap, err := api.service.UrlMaps.Get(request.Project, request.URLMapName).Context(ctx).Do()
	if err != nil {
		return GoogleEdgeState{}, fmt.Errorf("read GCP URL map for transition: %w", err)
	}
	comment := googleOwnedPrefix + request.OwnershipMarker
	ownedRequest := request
	ownedRequest.ExpectedBackendServiceURL = ""
	if !urlMapMatchesRequest(urlMap, ownedRequest, comment, request.BackendServiceURL) {
		return GoogleEdgeState{}, errors.New("refusing to transition an unowned or drifted GCP URL map")
	}
	if urlMap.DefaultService != target {
		next := cloneURLMapForBackend(urlMap, target)
		operation, updateErr := api.service.UrlMaps.Update(request.Project, request.URLMapName, next).Context(ctx).Do()
		if updateErr != nil {
			return GoogleEdgeState{}, fmt.Errorf("update GCP URL map for transition: %w", updateErr)
		}
		if err := api.waitGlobalOperation(ctx, operation); err != nil {
			return GoogleEdgeState{}, fmt.Errorf("wait for GCP URL map transition: %w", err)
		}
		urlMap, err = api.service.UrlMaps.Get(request.Project, request.URLMapName).Context(ctx).Do()
		if err != nil {
			return GoogleEdgeState{}, fmt.Errorf("read GCP URL map after transition: %w", err)
		}
	}
	request.ExpectedBackendServiceURL = target
	if !urlMapMatchesRequest(urlMap, request, comment, request.BackendServiceURL) {
		return GoogleEdgeState{}, errors.New("GCP URL map transition did not converge to the requested backend")
	}
	return GoogleEdgeState{Project: request.Project, URLMapName: request.URLMapName, BackendService: resourceNameFromURL(request.BackendServiceURL), ActiveBackendService: resourceNameFromURL(urlMap.DefaultService), DomainName: firstString(request.Domains)}, nil
}

func (api *computeGoogleEdgeAPI) Destroy(ctx context.Context, request GoogleEdgeApplyRequest, state GoogleEdgeState) error {
	if err := validateGoogleEdgeApplyRequest(request); err != nil {
		return err
	}
	comment := googleOwnedPrefix + request.OwnershipMarker
	if state.ForwardingName != "" {
		forwarding, err := api.service.GlobalForwardingRules.Get(request.Project, state.ForwardingName).Context(ctx).Do()
		if err == nil {
			if forwarding.Description != comment {
				return errors.New("refusing to destroy an unowned GCP forwarding rule")
			}
			operation, deleteErr := api.service.GlobalForwardingRules.Delete(request.Project, state.ForwardingName).Context(ctx).Do()
			if deleteErr != nil {
				return fmt.Errorf("delete GCP forwarding rule: %w", deleteErr)
			}
			if err := api.waitGlobalOperation(ctx, operation); err != nil {
				return fmt.Errorf("delete GCP forwarding rule: %w", err)
			}
		} else if !isGoogleNotFound(err) {
			return err
		}
	}
	if state.TargetProxyName != "" {
		proxy, err := api.service.TargetHttpsProxies.Get(request.Project, state.TargetProxyName).Context(ctx).Do()
		if err == nil {
			if proxy.Description != comment {
				return errors.New("refusing to destroy an unowned GCP target HTTPS proxy")
			}
			operation, deleteErr := api.service.TargetHttpsProxies.Delete(request.Project, state.TargetProxyName).Context(ctx).Do()
			if deleteErr != nil {
				return fmt.Errorf("delete GCP target HTTPS proxy: %w", deleteErr)
			}
			if err := api.waitGlobalOperation(ctx, operation); err != nil {
				return fmt.Errorf("delete GCP target HTTPS proxy: %w", err)
			}
		} else if !isGoogleNotFound(err) {
			return err
		}
	}
	if state.URLMapName != "" {
		urlMap, err := api.service.UrlMaps.Get(request.Project, state.URLMapName).Context(ctx).Do()
		if err == nil {
			if urlMap.Description != comment {
				return errors.New("refusing to destroy an unowned GCP URL map")
			}
			operation, deleteErr := api.service.UrlMaps.Delete(request.Project, state.URLMapName).Context(ctx).Do()
			if deleteErr != nil {
				return fmt.Errorf("delete GCP URL map: %w", deleteErr)
			}
			if err := api.waitGlobalOperation(ctx, operation); err != nil {
				return fmt.Errorf("delete GCP URL map: %w", err)
			}
		} else if !isGoogleNotFound(err) {
			return err
		}
	}
	return nil
}

func (api *computeGoogleEdgeAPI) Inventory(ctx context.Context, marker string) ([]sdk.EdgeInventoryResource, error) {
	if strings.TrimSpace(marker) == "" || strings.ContainsAny(marker, "\r\n\x00") {
		return nil, errors.New("GCP edge ownership marker is required and must be single-line")
	}
	comment := googleOwnedPrefix + strings.TrimSpace(marker)
	var resources []sdk.EdgeInventoryResource
	forwarding, err := api.service.GlobalForwardingRules.List(api.project).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	for _, item := range forwarding.Items {
		if item.Description == comment {
			resources = append(resources, sdk.EdgeInventoryResource{Identity: googleResourcePrefix + "forwarding-rule:" + item.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	proxies, err := api.service.TargetHttpsProxies.List(api.project).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	for _, item := range proxies.Items {
		if item.Description == comment {
			resources = append(resources, sdk.EdgeInventoryResource{Identity: googleResourcePrefix + "target-https-proxy:" + item.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	urlMaps, err := api.service.UrlMaps.List(api.project).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	for _, item := range urlMaps.Items {
		if item.Description == comment {
			resources = append(resources, sdk.EdgeInventoryResource{Identity: googleResourcePrefix + "url-map:" + item.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func urlMapMatchesRequest(value *compute.UrlMap, request GoogleEdgeApplyRequest, comment, backendURL string) bool {
	if value == nil || value.Description != comment || len(value.HostRules) != 1 || len(value.PathMatchers) != 1 {
		return false
	}
	allowedBackend := value.DefaultService == backendURL
	if request.SecondaryBackendServiceURL != "" {
		allowedBackend = allowedBackend || value.DefaultService == request.SecondaryBackendServiceURL
	}
	if request.ExpectedBackendServiceURL != "" {
		allowedBackend = value.DefaultService == request.ExpectedBackendServiceURL
	}
	if !allowedBackend {
		return false
	}
	hostRule := value.HostRules[0]
	pathMatcher := value.PathMatchers[0]
	if hostRule == nil || pathMatcher == nil || hostRule.PathMatcher != "magelift-default" || pathMatcher.Name != "magelift-default" || pathMatcher.DefaultService != value.DefaultService {
		return false
	}
	return stringSlicesEqual(hostRule.Hosts, request.Domains)
}

func cloneURLMapForBackend(value *compute.UrlMap, backendURL string) *compute.UrlMap {
	next := *value
	next.DefaultService = backendURL
	next.PathMatchers = append([]*compute.PathMatcher(nil), value.PathMatchers...)
	if len(value.PathMatchers) > 0 && value.PathMatchers[0] != nil {
		pathMatcher := *value.PathMatchers[0]
		pathMatcher.DefaultService = backendURL
		next.PathMatchers[0] = &pathMatcher
	}
	return &next
}

func targetHTTPSProxyMatchesRequest(value *compute.TargetHttpsProxy, request GoogleEdgeApplyRequest, comment, urlMapURL string) bool {
	return targetHTTPSProxyOwnedByPlan(value, comment, urlMapURL) && len(value.SslCertificates) == 1 && value.SslCertificates[0] == request.SSLCertificateURL
}

func targetHTTPSProxyOwnedByPlan(value *compute.TargetHttpsProxy, comment, urlMapURL string) bool {
	return value != nil && value.Description == comment && value.UrlMap == urlMapURL
}

func forwardingMatchesRequest(value *compute.ForwardingRule, request GoogleEdgeApplyRequest, comment, targetURL string) bool {
	return value != nil && value.Description == comment && value.Target == targetURL && value.IPProtocol == "TCP" && forwardingPortRangeMatches(value.PortRange) && value.LoadBalancingScheme == "EXTERNAL_MANAGED" && value.NetworkTier == "PREMIUM"
}

func forwardingPortRangeMatches(portRange string) bool {
	switch portRange {
	case "443", "443-443":
		return true
	default:
		return false
	}
}

func stringSlicesEqual(left, right []string) bool {
	return strings.Join(sortedCopy(left), "\x00") == strings.Join(sortedCopy(right), "\x00")
}

func (api *computeGoogleEdgeAPI) ensureURLMap(ctx context.Context, request GoogleEdgeApplyRequest, comment, backendURL string) (*compute.UrlMap, error) {
	urlMap, err := api.service.UrlMaps.Get(request.Project, request.URLMapName).Context(ctx).Do()
	if err == nil {
		if !urlMapMatchesRequest(urlMap, request, comment, backendURL) {
			return nil, errors.New("existing GCP URL map is not owned by this edge plan")
		}
		return urlMap, nil
	}
	if !isGoogleNotFound(err) {
		return nil, err
	}
	operation, err := api.service.UrlMaps.Insert(request.Project, &compute.UrlMap{
		Name: request.URLMapName, Description: comment, DefaultService: backendURL,
		HostRules:    []*compute.HostRule{{Hosts: append([]string(nil), request.Domains...), PathMatcher: "magelift-default"}},
		PathMatchers: []*compute.PathMatcher{{Name: "magelift-default", DefaultService: backendURL}},
	}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create GCP URL map: %w", err)
	}
	if err := api.waitGlobalOperation(ctx, operation); err != nil {
		return nil, fmt.Errorf("wait for GCP URL map: %w", err)
	}
	urlMap, err = api.service.UrlMaps.Get(request.Project, request.URLMapName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if !urlMapMatchesRequest(urlMap, request, comment, backendURL) {
		return nil, errors.New("created GCP URL map does not match the requested route")
	}
	return urlMap, nil
}

func (api *computeGoogleEdgeAPI) ensureTargetHTTPSProxy(ctx context.Context, request GoogleEdgeApplyRequest, comment, urlMapURL string) (*compute.TargetHttpsProxy, error) {
	proxy, err := api.service.TargetHttpsProxies.Get(request.Project, request.TargetProxyName).Context(ctx).Do()
	if err == nil {
		if !targetHTTPSProxyOwnedByPlan(proxy, comment, urlMapURL) {
			return nil, errors.New("existing GCP target HTTPS proxy is not owned by this edge plan")
		}
		if targetHTTPSProxyMatchesRequest(proxy, request, comment, urlMapURL) {
			return proxy, nil
		}
		operation, err := api.service.TargetHttpsProxies.SetSslCertificates(request.Project, request.TargetProxyName, &compute.TargetHttpsProxiesSetSslCertificatesRequest{
			SslCertificates: []string{request.SSLCertificateURL},
		}).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("rotate GCP target HTTPS proxy TLS certificate: %w", err)
		}
		if err := api.waitGlobalOperation(ctx, operation); err != nil {
			return nil, fmt.Errorf("wait for GCP target HTTPS proxy TLS certificate rotation: %w", err)
		}
		proxy, err = api.service.TargetHttpsProxies.Get(request.Project, request.TargetProxyName).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("read GCP target HTTPS proxy after TLS certificate rotation: %w", err)
		}
		if !targetHTTPSProxyMatchesRequest(proxy, request, comment, urlMapURL) {
			return nil, errors.New("updated GCP target HTTPS proxy does not match the requested TLS route")
		}
		return proxy, nil
	}
	if !isGoogleNotFound(err) {
		return nil, err
	}
	operation, err := api.service.TargetHttpsProxies.Insert(request.Project, &compute.TargetHttpsProxy{Name: request.TargetProxyName, Description: comment, UrlMap: urlMapURL, SslCertificates: []string{request.SSLCertificateURL}}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create GCP target HTTPS proxy: %w", err)
	}
	if err := api.waitGlobalOperation(ctx, operation); err != nil {
		return nil, fmt.Errorf("wait for GCP target HTTPS proxy: %w", err)
	}
	proxy, err = api.service.TargetHttpsProxies.Get(request.Project, request.TargetProxyName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if !targetHTTPSProxyMatchesRequest(proxy, request, comment, urlMapURL) {
		return nil, errors.New("created GCP target HTTPS proxy does not match the requested TLS route")
	}
	return proxy, nil
}

func (api *computeGoogleEdgeAPI) ensureForwardingRule(ctx context.Context, request GoogleEdgeApplyRequest, comment, targetURL string) (*compute.ForwardingRule, error) {
	forwarding, err := api.service.GlobalForwardingRules.Get(request.Project, request.ForwardingName).Context(ctx).Do()
	if err == nil {
		if !forwardingMatchesRequest(forwarding, request, comment, targetURL) {
			return nil, errors.New("existing GCP forwarding rule is not owned by this edge plan")
		}
		return forwarding, nil
	}
	if !isGoogleNotFound(err) {
		return nil, err
	}
	operation, err := api.service.GlobalForwardingRules.Insert(request.Project, &compute.ForwardingRule{Name: request.ForwardingName, Description: comment, Target: targetURL, IPProtocol: "TCP", PortRange: "443", LoadBalancingScheme: "EXTERNAL_MANAGED", NetworkTier: "PREMIUM"}).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("create GCP forwarding rule: %w", err)
	}
	if err := api.waitGlobalOperation(ctx, operation); err != nil {
		return nil, fmt.Errorf("wait for GCP forwarding rule: %w", err)
	}
	forwarding, err = api.service.GlobalForwardingRules.Get(request.Project, request.ForwardingName).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	if !forwardingMatchesRequest(forwarding, request, comment, targetURL) {
		return nil, errors.New("created GCP forwarding rule does not match the requested listener")
	}
	return forwarding, nil
}

func (api *computeGoogleEdgeAPI) invalidateCache(ctx context.Context, request GoogleEdgeApplyRequest) (string, error) {
	var operationNames []string
	for _, path := range request.PurgePaths {
		op, err := api.service.UrlMaps.InvalidateCache(request.Project, request.URLMapName, &compute.CacheInvalidationRule{Path: path}).Context(ctx).Do()
		if err != nil {
			return "", fmt.Errorf("invalidate GCP URL map cache for %q: %w", path, err)
		}
		if err := api.waitGlobalOperation(ctx, op); err != nil {
			return "", fmt.Errorf("wait for GCP cache invalidation for %q: %w", path, err)
		}
		operationNames = append(operationNames, op.Name)
	}
	return strings.Join(operationNames, ","), nil
}

func (api *computeGoogleEdgeAPI) waitGlobalOperation(ctx context.Context, operation *compute.Operation) error {
	if operation == nil || operation.Name == "" {
		return errors.New("GCP operation identity is required")
	}
	operationCtx, cancel := context.WithTimeout(ctx, api.operationWait)
	defer cancel()
	for {
		if err := operationCtx.Err(); err != nil {
			return err
		}
		current, err := api.service.GlobalOperations.Get(api.project, operation.Name).Context(operationCtx).Do()
		if err != nil {
			return err
		}
		if current.Status == "DONE" {
			if current.Error != nil && len(current.Error.Errors) > 0 {
				return errors.New(current.Error.Errors[0].Message)
			}
			return nil
		}
		timer := time.NewTimer(api.pollInterval)
		select {
		case <-operationCtx.Done():
			timer.Stop()
			return operationCtx.Err()
		case <-timer.C:
		}
	}
}

func validateGoogleEdgeApplyRequest(request GoogleEdgeApplyRequest) error {
	for name, value := range map[string]string{"project": request.Project, "ownership marker": request.OwnershipMarker, "origin health reference": request.OriginHealthRef, "URL map name": request.URLMapName, "target HTTPS proxy name": request.TargetProxyName, "forwarding rule name": request.ForwardingName, "backend service URL": request.BackendServiceURL, "SSL certificate URL": request.SSLCertificateURL} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("GCP edge %s is required", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("GCP edge %s must be single-line", name)
		}
	}
	refs := request.OriginHealthRefs
	if len(refs) == 0 {
		refs = []string{request.OriginHealthRef}
	}
	if len(refs) > 2 {
		return errors.New("GCP edge supports at most two origin health references")
	}
	for index, reference := range refs {
		if strings.TrimSpace(reference) == "" {
			return fmt.Errorf("GCP edge origin health reference %d is required", index)
		}
		if strings.ContainsAny(reference, "\r\n\x00") {
			return fmt.Errorf("GCP edge origin health reference %d must be single-line", index)
		}
	}
	if request.SecondaryBackendServiceURL != "" {
		if request.SecondaryBackendServiceURL == request.BackendServiceURL {
			return errors.New("GCP edge primary and secondary backend services must differ")
		}
		if len(refs) != 2 {
			return errors.New("GCP edge origin groups require primary and secondary health references")
		}
	}
	if request.ExpectedBackendServiceURL != "" && request.ExpectedBackendServiceURL != request.BackendServiceURL && request.ExpectedBackendServiceURL != request.SecondaryBackendServiceURL {
		return errors.New("GCP edge expected backend service is outside the planned origin group")
	}
	if request.PurgeOnDeploy && len(request.PurgePaths) == 0 {
		return errors.New("GCP edge purge requires at least one path")
	}
	return nil
}

func verifyGoogleOriginHealth(ctx context.Context, health GoogleOriginHealthProbe, request GoogleEdgeApplyRequest) error {
	if health == nil {
		return errors.New("GCP edge origin health probe is required")
	}
	refs := request.OriginHealthRefs
	if len(refs) == 0 {
		refs = []string{request.OriginHealthRef}
	}
	for _, reference := range refs {
		if err := health.VerifyOrigin(ctx, reference); err != nil {
			return err
		}
	}
	return nil
}

func stateFromReferences(request GoogleEdgeApplyRequest, references []string) GoogleEdgeState {
	state := GoogleEdgeState{Project: request.Project, URLMapName: request.URLMapName, TargetProxyName: request.TargetProxyName, ForwardingName: request.ForwardingName, BackendService: resourceNameFromURL(request.BackendServiceURL)}
	for _, reference := range references {
		switch {
		case strings.HasPrefix(reference, googleResourcePrefix+"url-map:"):
			state.URLMapName = strings.TrimPrefix(reference, googleResourcePrefix+"url-map:")
		case strings.HasPrefix(reference, googleResourcePrefix+"target-https-proxy:"):
			state.TargetProxyName = strings.TrimPrefix(reference, googleResourcePrefix+"target-https-proxy:")
		case strings.HasPrefix(reference, googleResourcePrefix+"forwarding-rule:"):
			state.ForwardingName = strings.TrimPrefix(reference, googleResourcePrefix+"forwarding-rule:")
		case strings.HasPrefix(reference, googleResourcePrefix+"invalidation:"):
			state.Invalidation = strings.TrimPrefix(reference, googleResourcePrefix+"invalidation:")
		}
	}
	return state
}

func stateReferences(state GoogleEdgeState) []string {
	refs := []string{}
	if state.URLMapName != "" {
		refs = append(refs, googleResourcePrefix+"url-map:"+state.URLMapName)
	}
	if state.TargetProxyName != "" {
		refs = append(refs, googleResourcePrefix+"target-https-proxy:"+state.TargetProxyName)
	}
	if state.ForwardingName != "" {
		refs = append(refs, googleResourcePrefix+"forwarding-rule:"+state.ForwardingName)
	}
	if state.BackendService != "" {
		refs = append(refs, googleResourcePrefix+"backend-service:"+state.BackendService)
	}
	if state.Invalidation != "" {
		refs = append(refs, googleResourcePrefix+"invalidation:"+state.Invalidation)
	}
	sort.Strings(refs)
	return refs
}

func stateOutputs(state GoogleEdgeState) []sdk.AdapterOutput {
	return []sdk.AdapterOutput{{Key: "forwardingRuleName", Value: state.ForwardingName}, {Key: "forwardingIPAddress", Value: state.IPAddress}, {Key: "targetHttpsProxyName", Value: state.TargetProxyName}, {Key: "urlMapName", Value: state.URLMapName}, {Key: "activeBackendService", Value: state.ActiveBackendService}, {Key: "domainName", Value: state.DomainName}}
}

func backendServiceURL(project, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("backendService is required")
	}
	return globalResourceURL(project, value, "backendServices")
}

func parseGoogleOriginGroup(project string, intent sdk.EdgeIntent, configuration map[string]any, primaryURL string) (string, []string, error) {
	groupValue, found := configuration["originGroup"]
	if !found {
		if strings.TrimSpace(intent.FailoverPolicyRef) != "" {
			return "", nil, sdk.EdgeCapabilityError{AdapterID: googleEdgeAdapterID, Action: sdk.EdgeFailover, Status: sdk.EdgeCapabilityUnsupported, Reason: "GCP edge failover requires a provider-owned originGroup with primary and secondary backend services"}
		}
		return "", []string{intent.OriginHealthRef}, nil
	}
	group, ok := groupValue.(map[string]any)
	if !ok {
		return "", nil, errors.New("GCP edge originGroup must be an object")
	}
	if strings.TrimSpace(intent.FailoverPolicyRef) == "" {
		return "", nil, errors.New("GCP edge originGroup requires FailoverPolicyRef")
	}
	primary, err := googleOriginBackendURL(project, group["primary"])
	if err != nil {
		return "", nil, fmt.Errorf("parse GCP originGroup primary backend: %w", err)
	}
	if primary != primaryURL {
		return "", nil, errors.New("GCP originGroup primary backend must match backendService")
	}
	secondary, err := googleOriginBackendURL(project, group["secondary"])
	if err != nil {
		return "", nil, fmt.Errorf("parse GCP originGroup secondary backend: %w", err)
	}
	if secondary == primary {
		return "", nil, errors.New("GCP originGroup primary and secondary backends must differ")
	}
	primaryHealth := stringOption(group, "primaryHealthRef")
	if primaryHealth == "" {
		primaryHealth = intent.OriginHealthRef
	}
	if primaryHealth != intent.OriginHealthRef {
		return "", nil, errors.New("GCP originGroup primaryHealthRef must match the portable origin health reference")
	}
	secondaryHealth := stringOption(group, "secondaryHealthRef")
	if secondaryHealth == "" {
		return "", nil, errors.New("GCP originGroup secondaryHealthRef is required")
	}
	return secondary, []string{primaryHealth, secondaryHealth}, nil
}

func googleOriginBackendURL(project string, value any) (string, error) {
	switch typed := value.(type) {
	case string:
		return backendServiceURL(project, typed)
	case map[string]any:
		return backendServiceURL(project, stringOption(typed, "backendService"))
	default:
		return "", errors.New("backend service must be a string or object with backendService")
	}
}

func globalResourceURL(project, value, kind string) (string, error) {
	if strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("resource reference must be single-line")
	}
	if strings.HasPrefix(value, "https://") {
		return value, nil
	}
	if strings.Contains(value, "/") {
		return "", fmt.Errorf("resource reference %q must be a full https URL or a simple global name", value)
	}
	return "https://www.googleapis.com/compute/v1/projects/" + project + "/global/" + kind + "/" + value, nil
}

func resourceNameFromURL(value string) string {
	value = strings.TrimSuffix(value, "/")
	if index := strings.LastIndexByte(value, '/'); index >= 0 {
		return value[index+1:]
	}
	return value
}

func markerDigest(marker string) string {
	// Keep names within Compute's DNS-compatible resource-name limits.
	hash := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(hash[:])[:20]
}

func parseGooglePurgePaths(configuration map[string]any, enabled bool) ([]string, error) {
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
		for _, item := range values {
			path, ok := item.(string)
			if !ok {
				return nil, errors.New("GCP purgePaths must contain only strings")
			}
			paths = append(paths, path)
		}
	default:
		return nil, errors.New("GCP purgePaths must be a string array")
	}
	if len(paths) == 0 {
		return nil, errors.New("GCP purgePaths must contain at least one path")
	}
	for index, path := range paths {
		if strings.TrimSpace(path) == "" || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\r\n\x00") {
			return nil, fmt.Errorf("GCP purge path %d is invalid", index)
		}
		paths[index] = strings.TrimSpace(path)
	}
	return sortedCopy(paths), nil
}

func stringOption(configuration map[string]any, key string) string {
	value, _ := configuration[key].(string)
	return strings.TrimSpace(value)
}

func boolOption(configuration map[string]any, key string) bool {
	value, _ := configuration[key].(bool)
	return value
}

func sortedCopy(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}

func firstString(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func isGoogleNotFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound
}
