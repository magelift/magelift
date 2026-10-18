package fastly

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	fastlysdk "github.com/fastly/go-fastly/fastly"
	provider "github.com/magelift/magelift/internal/provider"
)

// NativeService is the provider-owned service identity needed by edge
// lifecycle code. It intentionally avoids exposing the Fastly SDK model.
type NativeService struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Comment       string `json:"comment"`
	ActiveVersion int    `json:"active_version"`
	Version       int    `json:"version"`
}

// UnmarshalJSON accepts both Fastly service response shapes: the service list
// exposes a scalar version, while service details expose version objects with
// a number field. The lifecycle only needs the number and must not assume that
// the two endpoints serialize it identically.
func (service *NativeService) UnmarshalJSON(data []byte) error {
	var response struct {
		ID            string          `json:"id"`
		Name          string          `json:"name"`
		Comment       string          `json:"comment"`
		ActiveVersion json.RawMessage `json:"active_version"`
		Version       json.RawMessage `json:"version"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return err
	}
	activeVersion, err := nativeVersionNumber(response.ActiveVersion)
	if err != nil {
		return fmt.Errorf("decode Fastly active service version: %w", err)
	}
	version, err := nativeVersionNumber(response.Version)
	if err != nil {
		return fmt.Errorf("decode Fastly service version: %w", err)
	}
	*service = NativeService{
		ID:            response.ID,
		Name:          response.Name,
		Comment:       response.Comment,
		ActiveVersion: activeVersion,
		Version:       version,
	}
	return nil
}

func nativeVersionNumber(raw json.RawMessage) (int, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, nil
	}
	var number int
	if err := json.Unmarshal(raw, &number); err == nil {
		return number, nil
	}
	var object struct {
		Number json.RawMessage `json:"number"`
	}
	if err := json.Unmarshal(raw, &object); err != nil {
		return 0, err
	}
	if len(object.Number) == 0 {
		return 0, errors.New("version object has no number")
	}
	if err := json.Unmarshal(object.Number, &number); err != nil {
		return 0, err
	}
	return number, nil
}

// NativeVersion is the provider-owned identity returned by Fastly's service
// version API. Versioned configuration must be written to an editable clone
// and activated only after validation; the portable edge contract does not
// expose Fastly's version model.
type NativeVersion struct {
	Number    int    `json:"number"`
	ServiceID string `json:"service_id"`
	Active    bool   `json:"active"`
	Locked    bool   `json:"locked"`
}

// NativeVersionValidation is the small status response returned by Fastly's
// version validation endpoint.
type NativeVersionValidation struct {
	Status  string `json:"status"`
	Message string `json:"msg"`
}

// NativeDomain is the provider-owned domain identity needed for ownership
// and cleanup checks.
type NativeDomain struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	FQDN      string `json:"fqdn"`
	Comment   string `json:"comment"`
	ServiceID string `json:"service_id"`
	Version   int    `json:"version"`
}

// NativeVCL is the provider-owned identity of one custom VCL object. Fastly
// does not expose an ownership comment on VCL objects, so the lifecycle
// adapter uses an explicit MageLift-owned name convention and never adopts an
// object with a different name or content.
type NativeVCL struct {
	ServiceID string `json:"service_id"`
	Version   int    `json:"version"`
	Name      string `json:"name"`
	Main      bool   `json:"main"`
	Content   string `json:"content"`
}

// NativeTLSSubscription is the safe subset of Fastly's JSON:API TLS
// subscription response. TLS subscriptions do not have a free-form ownership
// marker; created IDs are therefore retained in the result and pre-existing
// subscriptions are never adopted implicitly.
type NativeTLSSubscription struct {
	ID      string
	State   string
	Domains []string
}

// NativeToken is the provider-owned identity returned by Fastly token
// management. AccessToken is used only inside the credential lifecycle
// callback and is never returned through the public SDK result.
type NativeToken struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	AccessToken string   `json:"access_token"`
	Scope       string   `json:"scope"`
	Services    []string `json:"services,omitempty"`
}

// TokenAPI is the minimal Fastly token-management surface. It is separate
// from NativeAPI so edge lifecycle clients cannot accidentally gain token
// creation or revocation authority.
type TokenAPI interface {
	CurrentToken(context.Context) (NativeToken, error)
	CreateToken(context.Context, string, string, []string) (NativeToken, error)
	RevokeToken(context.Context, string) error
}

// NativeAPI is the minimum context-aware Fastly surface used by the native
// lifecycle adapter. It is intentionally independent from the public SDK.
type NativeAPI interface {
	ListServices(context.Context) ([]NativeService, error)
	CreateService(context.Context, string, string) (NativeService, error)
	GetService(context.Context, string) (NativeService, error)
	DeleteService(context.Context, string) error
	ListServiceDomains(context.Context, string) ([]NativeDomain, error)
	CreateDomain(context.Context, string, int, string, string) (NativeDomain, error)
	DeleteDomain(context.Context, string, int, string) error
	PurgeAll(context.Context, string) (string, error)
	Inventory(context.Context, string) ([]provider.InventoryResource, error)
}

// NativeFeatureAPI contains the Fastly APIs that are not part of the core
// service/domain surface. It is separate from NativeAPI so a service-only
// implementation can remain small, while a first-party implementation can
// opt into VCL and managed TLS with the same ownership gates as the CLI path.
type NativeFeatureAPI interface {
	ListVCLs(context.Context, string, int) ([]NativeVCL, error)
	CreateVCL(context.Context, string, int, string, string) (NativeVCL, error)
	SetVCLMain(context.Context, string, int, string) error
	DeleteVCL(context.Context, string, int, string) error
	ListTLSSubscriptions(context.Context) ([]NativeTLSSubscription, error)
	CreateTLSSubscription(context.Context, []string, string) (NativeTLSSubscription, error)
	DeleteTLSSubscription(context.Context, string) error
}

// NativeVersionAPI is separate from NativeAPI and NativeFeatureAPI because
// service-version mutation is an independent Fastly capability. A community
// adapter that only supports service discovery can remain small and will be
// rejected before any versioned mutation is attempted.
type NativeVersionAPI interface {
	CloneVersion(context.Context, string, int) (NativeVersion, error)
	ValidateVersion(context.Context, string, int) (NativeVersionValidation, error)
	ActivateVersion(context.Context, string, int) (NativeVersion, error)
}

// FailoverObservation is the provider-neutral proof returned by an injected
// Fastly failover controller. Fastly failover is configured through versioned
// backends, health checks, directors, and VCL rather than a single universal
// API call, so that policy-specific translation belongs behind this port.
type FailoverObservation struct {
	OperationID      string
	FailoverVerified bool
	RollbackVerified bool
}

// FailoverController is an optional provider-owned Fastly policy translator.
// It receives only the opaque policy reference and lifecycle identities; the
// implementation owns backend, health-check, director, and VCL API models.
// A controller must prove route convergence before returning success.
type FailoverController interface {
	Failover(context.Context, Request, Result) (FailoverObservation, error)
	Rollback(context.Context, Request, Result) (FailoverObservation, error)
}

// SDKClient wraps the official Fastly Go SDK with context-aware requests.
// The upstream methods used for service/domain operations predate context
// parameters, so this wrapper uses its exported RawRequest boundary and
// attaches the caller context before sending the request.
type SDKClient struct {
	client *fastlysdk.Client
}

var _ NativeAPI = (*SDKClient)(nil)
var _ NativeFeatureAPI = (*SDKClient)(nil)
var _ NativeVersionAPI = (*SDKClient)(nil)
var _ TokenAPI = (*SDKClient)(nil)

// NewSDKClient resolves a Fastly token only inside this provider boundary.
// The token is retained only by the SDK client for authenticated requests; it
// is never placed in a public plan, result, log, or error.
func NewSDKClient(ctx context.Context, resolver provider.CredentialResolver, credentialRef string) (*SDKClient, error) {
	var result *SDKClient
	err := provider.UseCredential(ctx, resolver, credentialRef, func(value []byte) error {
		client, err := fastlysdk.NewClient(string(value))
		if err != nil {
			return err
		}
		result = &SDKClient{client: client}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// NewSDKClientFromClient injects an initialized official SDK client for
// deterministic tests and approved endpoint wrappers.
func NewSDKClientFromClient(client *fastlysdk.Client) (*SDKClient, error) {
	if client == nil {
		return nil, errors.New("Fastly SDK client is required")
	}
	return &SDKClient{client: client}, nil
}

func (c *SDKClient) ListServices(ctx context.Context) ([]NativeService, error) {
	var services []NativeService
	if err := c.decode(ctx, http.MethodGet, "/service", nil, nil, &services); err != nil {
		return nil, err
	}
	return services, nil
}

func (c *SDKClient) CreateService(ctx context.Context, name, marker string) (NativeService, error) {
	values := url.Values{"name": {name}, "comment": {marker}, "type": {"vcl"}}
	var service NativeService
	if err := c.decode(ctx, http.MethodPost, "/service", strings.NewReader(values.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, &service); err != nil {
		return NativeService{}, err
	}
	return service, nil
}

func (c *SDKClient) GetService(ctx context.Context, id string) (NativeService, error) {
	var service NativeService
	if err := c.decode(ctx, http.MethodGet, "/service/"+url.PathEscape(id), nil, nil, &service); err != nil {
		return NativeService{}, err
	}
	return service, nil
}

func (c *SDKClient) DeleteService(ctx context.Context, id string) error {
	return c.decode(ctx, http.MethodDelete, "/service/"+url.PathEscape(id), nil, nil, nil)
}

func (c *SDKClient) ListServiceDomains(ctx context.Context, serviceID string) ([]NativeDomain, error) {
	var domains []NativeDomain
	if err := c.decode(ctx, http.MethodGet, "/service/"+url.PathEscape(serviceID)+"/domain", nil, nil, &domains); err != nil {
		return nil, err
	}
	return domains, nil
}

func (c *SDKClient) CreateDomain(ctx context.Context, serviceID string, version int, name, marker string) (NativeDomain, error) {
	if version <= 0 {
		return NativeDomain{}, errors.New("Fastly active service version is required")
	}
	values := url.Values{"name": {name}, "comment": {marker}}
	var domain NativeDomain
	path := fmt.Sprintf("/service/%s/version/%d/domain", url.PathEscape(serviceID), version)
	if err := c.decode(ctx, http.MethodPost, path, strings.NewReader(values.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, &domain); err != nil {
		return NativeDomain{}, err
	}
	return domain, nil
}

func (c *SDKClient) DeleteDomain(ctx context.Context, serviceID string, version int, name string) error {
	if version <= 0 {
		return errors.New("Fastly domain version is required")
	}
	path := fmt.Sprintf("/service/%s/version/%d/domain/%s", url.PathEscape(serviceID), version, url.PathEscape(name))
	return c.decode(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (c *SDKClient) PurgeAll(ctx context.Context, serviceID string) (string, error) {
	var purge fastlysdk.Purge
	path := "/service/" + url.PathEscape(serviceID) + "/purge_all"
	if err := c.decode(ctx, http.MethodPost, path, nil, nil, &purge); err != nil {
		return "", err
	}
	if strings.TrimSpace(purge.ID) == "" {
		return "", errors.New("Fastly purge response has no operation identity")
	}
	return purge.ID, nil
}

func (c *SDKClient) ListVCLs(ctx context.Context, serviceID string, version int) ([]NativeVCL, error) {
	if version <= 0 {
		return nil, errors.New("Fastly VCL version is required")
	}
	var vcls []NativeVCL
	path := fmt.Sprintf("/service/%s/version/%d/vcl", url.PathEscape(serviceID), version)
	if err := c.decode(ctx, http.MethodGet, path, nil, nil, &vcls); err != nil {
		return nil, err
	}
	return vcls, nil
}

func (c *SDKClient) CreateVCL(ctx context.Context, serviceID string, version int, name, content string) (NativeVCL, error) {
	if version <= 0 {
		return NativeVCL{}, errors.New("Fastly VCL version is required")
	}
	values := url.Values{"name": {name}, "content": {content}, "main": {"1"}}
	var vcl NativeVCL
	path := fmt.Sprintf("/service/%s/version/%d/vcl", url.PathEscape(serviceID), version)
	if err := c.decode(ctx, http.MethodPost, path, strings.NewReader(values.Encode()), map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, &vcl); err != nil {
		return NativeVCL{}, err
	}
	return vcl, nil
}

func (c *SDKClient) SetVCLMain(ctx context.Context, serviceID string, version int, name string) error {
	if version <= 0 {
		return errors.New("Fastly VCL version is required")
	}
	path := fmt.Sprintf("/service/%s/version/%d/vcl/%s/main", url.PathEscape(serviceID), version, url.PathEscape(name))
	return c.decode(ctx, http.MethodPut, path, nil, nil, nil)
}

func (c *SDKClient) DeleteVCL(ctx context.Context, serviceID string, version int, name string) error {
	if version <= 0 {
		return errors.New("Fastly VCL version is required")
	}
	path := fmt.Sprintf("/service/%s/version/%d/vcl/%s", url.PathEscape(serviceID), version, url.PathEscape(name))
	return c.decode(ctx, http.MethodDelete, path, nil, nil, nil)
}

func (c *SDKClient) ListTLSSubscriptions(ctx context.Context) ([]NativeTLSSubscription, error) {
	var envelope tlsSubscriptionListResponse
	if err := c.decode(ctx, http.MethodGet, "/tls/subscriptions", nil, nil, &envelope); err != nil {
		return nil, err
	}
	result := make([]NativeTLSSubscription, 0, len(envelope.Data))
	for _, item := range envelope.Data {
		result = append(result, item.native())
	}
	return result, nil
}

func (c *SDKClient) CreateTLSSubscription(ctx context.Context, domains []string, commonName string) (NativeTLSSubscription, error) {
	if len(domains) == 0 {
		return NativeTLSSubscription{}, errors.New("Fastly TLS subscription requires at least one domain")
	}
	if strings.TrimSpace(commonName) == "" {
		commonName = domains[0]
	}
	relationshipDomains := make([]map[string]any, 0, len(domains))
	for _, domain := range domains {
		relationshipDomains = append(relationshipDomains, map[string]any{"type": "tls_domain", "id": domain})
	}
	payload, err := json.Marshal(map[string]any{"data": map[string]any{
		"type":       "tls_subscription",
		"attributes": map[string]any{"certificate_authority": "lets-encrypt"},
		"relationships": map[string]any{
			"common_name": map[string]any{"data": map[string]any{"type": "tls_domain", "id": commonName}},
			"tls_domains": map[string]any{"data": relationshipDomains},
		},
	}})
	if err != nil {
		return NativeTLSSubscription{}, errors.New("marshal Fastly TLS subscription request failed")
	}
	var response tlsSubscriptionResponse
	if err := c.decode(ctx, http.MethodPost, "/tls/subscriptions", strings.NewReader(string(payload)), map[string]string{"Content-Type": "application/vnd.api+json", "Accept": "application/vnd.api+json"}, &response); err != nil {
		return NativeTLSSubscription{}, err
	}
	result := response.Data.native()
	if strings.TrimSpace(result.ID) == "" {
		return NativeTLSSubscription{}, errors.New("Fastly TLS subscription response has no identity")
	}
	return result, nil
}

func (c *SDKClient) DeleteTLSSubscription(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n\x00") {
		return errors.New("Fastly TLS subscription identity is required and must be single-line")
	}
	return c.decode(ctx, http.MethodDelete, "/tls/subscriptions/"+url.PathEscape(id), nil, map[string]string{"Accept": "application/vnd.api+json"}, nil)
}

func (c *SDKClient) CloneVersion(ctx context.Context, serviceID string, version int) (NativeVersion, error) {
	if strings.TrimSpace(serviceID) == "" {
		return NativeVersion{}, errors.New("Fastly service identity is required")
	}
	if version <= 0 {
		return NativeVersion{}, errors.New("Fastly source version is required")
	}
	path := fmt.Sprintf("/service/%s/version/%d/clone", url.PathEscape(serviceID), version)
	var result NativeVersion
	if err := c.decode(ctx, http.MethodPut, path, nil, nil, &result); err != nil {
		return NativeVersion{}, err
	}
	if result.Number <= 0 {
		return NativeVersion{}, errors.New("Fastly clone response has no version identity")
	}
	return result, nil
}

func (c *SDKClient) ValidateVersion(ctx context.Context, serviceID string, version int) (NativeVersionValidation, error) {
	if strings.TrimSpace(serviceID) == "" {
		return NativeVersionValidation{}, errors.New("Fastly service identity is required")
	}
	if version <= 0 {
		return NativeVersionValidation{}, errors.New("Fastly version is required")
	}
	path := fmt.Sprintf("/service/%s/version/%d/validate", url.PathEscape(serviceID), version)
	var result NativeVersionValidation
	if err := c.decode(ctx, http.MethodGet, path, nil, nil, &result); err != nil {
		return NativeVersionValidation{}, err
	}
	return result, nil
}

func (c *SDKClient) ActivateVersion(ctx context.Context, serviceID string, version int) (NativeVersion, error) {
	if strings.TrimSpace(serviceID) == "" {
		return NativeVersion{}, errors.New("Fastly service identity is required")
	}
	if version <= 0 {
		return NativeVersion{}, errors.New("Fastly version is required")
	}
	path := fmt.Sprintf("/service/%s/version/%d/activate", url.PathEscape(serviceID), version)
	var result NativeVersion
	if err := c.decode(ctx, http.MethodPut, path, nil, nil, &result); err != nil {
		return NativeVersion{}, err
	}
	if result.Number <= 0 {
		return NativeVersion{}, errors.New("Fastly activation response has no version identity")
	}
	return result, nil
}

type tlsSubscriptionListResponse struct {
	Data []tlsSubscriptionResource `json:"data"`
}

type tlsSubscriptionResponse struct {
	Data tlsSubscriptionResource `json:"data"`
}

type tlsSubscriptionResource struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Attributes struct {
		State string `json:"state"`
	} `json:"attributes"`
	Relationships struct {
		TLSDomains struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		} `json:"tls_domains"`
	} `json:"relationships"`
}

func (resource tlsSubscriptionResource) native() NativeTLSSubscription {
	domains := make([]string, 0, len(resource.Relationships.TLSDomains.Data))
	for _, domain := range resource.Relationships.TLSDomains.Data {
		if strings.TrimSpace(domain.ID) != "" {
			domains = append(domains, domain.ID)
		}
	}
	return NativeTLSSubscription{ID: resource.ID, State: resource.Attributes.State, Domains: domains}
}

func (c *SDKClient) CurrentToken(ctx context.Context) (NativeToken, error) {
	var token NativeToken
	if err := c.decode(ctx, http.MethodGet, "/tokens/self", nil, nil, &token); err != nil {
		return NativeToken{}, err
	}
	return token, nil
}

func (c *SDKClient) CreateToken(ctx context.Context, name, scope string, services []string) (NativeToken, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n\x00") {
		return NativeToken{}, errors.New("Fastly token name is required and must be single-line")
	}
	if strings.TrimSpace(scope) == "" || strings.ContainsAny(scope, "\r\n\x00") {
		return NativeToken{}, errors.New("Fastly token scope is required and must be single-line")
	}
	for _, service := range services {
		if strings.TrimSpace(service) == "" || strings.ContainsAny(service, "\r\n\x00") {
			return NativeToken{}, errors.New("Fastly token service identity is invalid")
		}
	}
	payload, err := json.Marshal(map[string]any{
		"attributes": map[string]any{"name": name, "role": "engineer", "scope": scope, "services": services},
	})
	if err != nil {
		return NativeToken{}, errors.New("marshal Fastly automation token request failed")
	}
	var token NativeToken
	if err := c.decode(ctx, http.MethodPost, "/automation-tokens", strings.NewReader(string(payload)), map[string]string{"Content-Type": "application/json"}, &token); err != nil {
		return NativeToken{}, err
	}
	return token, nil
}

func (c *SDKClient) RevokeToken(ctx context.Context, id string) error {
	if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n\x00") {
		return errors.New("Fastly token identity is required and must be single-line")
	}
	return c.decode(ctx, http.MethodDelete, "/automation-tokens/"+url.PathEscape(id), nil, nil, nil)
}

// Inventory returns exact marker-owned services and domains from Fastly's
// owning API. It is not an index lookup and is safe to use as cleanup truth.
func (c *SDKClient) Inventory(ctx context.Context, marker string) ([]provider.InventoryResource, error) {
	if strings.TrimSpace(marker) == "" {
		return nil, errors.New("Fastly ownership marker is required")
	}
	services, err := c.ListServices(ctx)
	if err != nil {
		return nil, err
	}
	resources := make([]provider.InventoryResource, 0)
	for _, service := range services {
		if service.Comment != marker || service.ID == "" {
			continue
		}
		resources = append(resources, provider.InventoryResource{Identity: "service:" + service.ID, Owned: true, Live: true})
		domains, err := c.ListServiceDomains(ctx, service.ID)
		if err != nil {
			return nil, err
		}
		for _, domain := range domains {
			if domain.Comment == marker {
				identity := domain.ID
				if identity == "" {
					identity = domain.FQDN
				}
				resources = append(resources, provider.InventoryResource{Identity: "domain:" + identity, Owned: true, Live: true})
			}
		}
	}
	return resources, nil
}

func (c *SDKClient) decode(ctx context.Context, method, path string, body io.Reader, headers map[string]string, result any) error {
	if c == nil || c.client == nil || c.client.HTTPClient == nil {
		return errors.New("Fastly SDK client is required")
	}
	if ctx == nil {
		return errors.New("Fastly API context is required")
	}
	request, err := c.client.RawRequest(method, path, &fastlysdk.RequestOptions{Body: body, Headers: headers})
	if err != nil {
		return errors.New("build Fastly API request failed")
	}
	response, err := c.client.HTTPClient.Do(request.WithContext(ctx))
	if err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		return errors.New("Fastly API request failed")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("Fastly API request returned status %d", response.StatusCode)
	}
	if result == nil || response.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(response.Body).Decode(result); err != nil {
		return errors.New("decode Fastly API response failed")
	}
	return nil
}

// FastlyAPIVersion converts a provider version identity without allowing
// malformed or negative values to reach a delete/create request.
func FastlyAPIVersion(value int64) (int, error) {
	if value <= 0 || value > int64(^uint(0)>>1) {
		return 0, errors.New("Fastly version is outside the supported range")
	}
	return strconv.Atoi(strconv.FormatInt(value, 10))
}
