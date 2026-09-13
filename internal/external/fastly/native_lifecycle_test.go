package fastly

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/provider"
)

type nativeLifecycleAPI struct {
	services   map[string]NativeService
	domains    map[string][]NativeDomain
	domainVers []int
	deleteVers []int
	created    int
	deleted    int
	purges     int
	serviceSeq int
	events     []string
}

type nativeLifecycleVersions struct {
	api       *nativeLifecycleAPI
	cloned    []int
	validated []int
	activated []int
	next      int
}

type nativeFailoverController struct {
	failoverCalls int
	rollbackCalls int
}

func (controller *nativeFailoverController) Failover(_ context.Context, _ Request, result Result) (FailoverObservation, error) {
	controller.failoverCalls++
	result.FailoverApplied = true
	return FailoverObservation{OperationID: "failover-1", FailoverVerified: true}, nil
}

func (controller *nativeFailoverController) Rollback(_ context.Context, _ Request, result Result) (FailoverObservation, error) {
	controller.rollbackCalls++
	result.RollbackApplied = true
	return FailoverObservation{OperationID: "rollback-1", RollbackVerified: true}, nil
}

func (versions *nativeLifecycleVersions) CloneVersion(_ context.Context, serviceID string, version int) (NativeVersion, error) {
	versions.api.events = append(versions.api.events, fmt.Sprintf("clone:%d", version))
	versions.cloned = append(versions.cloned, version)
	if versions.next <= version {
		versions.next = version + 1
	} else {
		versions.next++
	}
	return NativeVersion{Number: versions.next, ServiceID: serviceID}, nil
}

func (versions *nativeLifecycleVersions) ValidateVersion(_ context.Context, _ string, version int) (NativeVersionValidation, error) {
	versions.api.events = append(versions.api.events, fmt.Sprintf("validate:%d", version))
	versions.validated = append(versions.validated, version)
	return NativeVersionValidation{Status: "ok", Message: "valid"}, nil
}

func (versions *nativeLifecycleVersions) ActivateVersion(_ context.Context, serviceID string, version int) (NativeVersion, error) {
	versions.api.events = append(versions.api.events, fmt.Sprintf("activate:%d", version))
	versions.activated = append(versions.activated, version)
	service := versions.api.services[serviceID]
	service.ActiveVersion = version
	versions.api.services[serviceID] = service
	return NativeVersion{Number: version, ServiceID: serviceID, Active: true, Locked: true}, nil
}

type nativeLifecycleFeatures struct {
	vcls       map[string]NativeVCL
	tls        map[string]NativeTLSSubscription
	createdVCL int
	deletedVCL int
	createdTLS int
	deletedTLS int
}

func newNativeLifecycleFeatures() *nativeLifecycleFeatures {
	return &nativeLifecycleFeatures{vcls: make(map[string]NativeVCL), tls: make(map[string]NativeTLSSubscription)}
}

func (features *nativeLifecycleFeatures) ListVCLs(context.Context, string, int) ([]NativeVCL, error) {
	result := make([]NativeVCL, 0, len(features.vcls))
	for _, vcl := range features.vcls {
		result = append(result, vcl)
	}
	return result, nil
}

func (features *nativeLifecycleFeatures) CreateVCL(_ context.Context, serviceID string, version int, name, content string) (NativeVCL, error) {
	vcl := NativeVCL{ServiceID: serviceID, Version: version, Name: name, Content: content}
	features.vcls[name] = vcl
	features.createdVCL++
	return vcl, nil
}

func (features *nativeLifecycleFeatures) SetVCLMain(_ context.Context, _ string, _ int, name string) error {
	vcl, ok := features.vcls[name]
	if !ok {
		return fmt.Errorf("VCL %q not found", name)
	}
	vcl.Main = true
	features.vcls[name] = vcl
	return nil
}

func (features *nativeLifecycleFeatures) DeleteVCL(_ context.Context, _ string, _ int, name string) error {
	delete(features.vcls, name)
	features.deletedVCL++
	return nil
}

func (features *nativeLifecycleFeatures) ListTLSSubscriptions(context.Context) ([]NativeTLSSubscription, error) {
	result := make([]NativeTLSSubscription, 0, len(features.tls))
	for _, subscription := range features.tls {
		result = append(result, subscription)
	}
	return result, nil
}

func (features *nativeLifecycleFeatures) CreateTLSSubscription(_ context.Context, domains []string, _ string) (NativeTLSSubscription, error) {
	subscription := NativeTLSSubscription{ID: "tls-1", Domains: append([]string(nil), domains...), State: "pending"}
	features.tls[subscription.ID] = subscription
	features.createdTLS++
	return subscription, nil
}

func (features *nativeLifecycleFeatures) DeleteTLSSubscription(_ context.Context, id string) error {
	delete(features.tls, id)
	features.deletedTLS++
	return nil
}

func newNativeLifecycleAPI() *nativeLifecycleAPI {
	return &nativeLifecycleAPI{services: make(map[string]NativeService), domains: make(map[string][]NativeDomain)}
}

func (api *nativeLifecycleAPI) ListServices(context.Context) ([]NativeService, error) {
	result := make([]NativeService, 0, len(api.services))
	for _, service := range api.services {
		result = append(result, service)
	}
	return result, nil
}

func (api *nativeLifecycleAPI) CreateService(_ context.Context, name, marker string) (NativeService, error) {
	api.serviceSeq++
	service := NativeService{ID: fmt.Sprintf("svc-%d", api.serviceSeq), Name: name, Comment: marker, ActiveVersion: 1}
	api.services[service.ID] = service
	api.created++
	return service, nil
}

func (api *nativeLifecycleAPI) GetService(_ context.Context, id string) (NativeService, error) {
	service, ok := api.services[id]
	if !ok {
		return NativeService{}, fmt.Errorf("service %q not found", id)
	}
	return service, nil
}

func (api *nativeLifecycleAPI) DeleteService(_ context.Context, id string) error {
	delete(api.services, id)
	delete(api.domains, id)
	api.deleted++
	return nil
}

func (api *nativeLifecycleAPI) ListServiceDomains(_ context.Context, serviceID string) ([]NativeDomain, error) {
	return append([]NativeDomain(nil), api.domains[serviceID]...), nil
}

func (api *nativeLifecycleAPI) CreateDomain(_ context.Context, serviceID string, version int, name, marker string) (NativeDomain, error) {
	api.events = append(api.events, fmt.Sprintf("create-domain:%d", version))
	api.domainVers = append(api.domainVers, version)
	domain := NativeDomain{ID: "domain-" + name, Name: name, FQDN: name, Comment: marker, ServiceID: serviceID, Version: version}
	api.domains[serviceID] = append(api.domains[serviceID], domain)
	return domain, nil
}

func (api *nativeLifecycleAPI) DeleteDomain(_ context.Context, serviceID string, version int, name string) error {
	api.events = append(api.events, fmt.Sprintf("delete-domain:%d", version))
	api.deleteVers = append(api.deleteVers, version)
	domains := api.domains[serviceID]
	remaining := domains[:0]
	for _, domain := range domains {
		if domain.Name != name && domain.FQDN != name {
			remaining = append(remaining, domain)
		}
	}
	api.domains[serviceID] = remaining
	return nil
}

func (api *nativeLifecycleAPI) PurgeAll(context.Context, string) (string, error) {
	api.events = append(api.events, "purge")
	api.purges++
	return "purge-1", nil
}

func (api *nativeLifecycleAPI) Inventory(_ context.Context, marker string) ([]provider.InventoryResource, error) {
	resources := make([]provider.InventoryResource, 0)
	for id, service := range api.services {
		if service.Comment != marker {
			continue
		}
		resources = append(resources, provider.InventoryResource{Identity: "service:" + id, Owned: true, Live: true})
		for _, domain := range api.domains[id] {
			if domain.Comment == marker {
				resources = append(resources, provider.InventoryResource{Identity: "domain:" + domain.ID, Owned: true, Live: true})
			}
		}
	}
	return resources, nil
}

func TestNativeLifecycleUsesOfficialAPIForOwnedServiceDomainAndPurgeFlow(t *testing.T) {
	api := newNativeLifecycleAPI()
	lifecycle := NativeLifecycle{API: api, Probe: healthyFastlyProbe(), DeletionPolicy: DeletionPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2}}
	request := fastlyRequest()
	result, err := lifecycle.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.ServiceID != "svc-1" || len(result.CreatedDomains) != 1 || !result.PurgeRequested || api.created != 1 || api.purges != 1 {
		t.Fatalf("native apply result = %#v, api = %#v", result, api)
	}
	if err := lifecycle.Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	if api.deleted != 1 {
		t.Fatalf("native delete count = %d", api.deleted)
	}
}

func TestNativeLifecycleClonesExistingServiceBeforeVersionedMutationAndCleanup(t *testing.T) {
	api := newNativeLifecycleAPI()
	api.services["svc-existing"] = NativeService{ID: "svc-existing", Comment: "magelift/acceptance/test-1", ActiveVersion: 1}
	versions := &nativeLifecycleVersions{api: api}
	lifecycle := NativeLifecycle{
		API: api, Versions: versions, Probe: healthyFastlyProbe(),
		DeletionPolicy: DeletionPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2},
	}
	request := fastlyRequest()
	request.ServiceID = "svc-existing"
	request.CreateService = false
	request.PurgeOnDeploy = false

	result, err := lifecycle.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Version != "2" || len(versions.cloned) != 1 || versions.cloned[0] != 1 || len(versions.validated) != 1 || len(versions.activated) != 1 || versions.activated[0] != 2 {
		t.Fatalf("versioned apply = result %#v, versions %#v", result, versions)
	}
	if len(api.domainVers) != 1 || api.domainVers[0] != 2 {
		t.Fatalf("domain created on active version instead of clone: %#v", api.domainVers)
	}
	if want := []string{"clone:1", "create-domain:2", "validate:2", "activate:2"}; !sameStrings(api.events, want) {
		t.Fatalf("versioned apply order = %#v, want %#v", api.events, want)
	}

	if err := lifecycle.Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	if len(api.deleteVers) != 1 || api.deleteVers[0] != 3 || len(versions.activated) != 2 || versions.activated[1] != 3 {
		t.Fatalf("versioned cleanup = delete versions %#v, activated %#v", api.deleteVers, versions.activated)
	}
	if want := []string{"clone:1", "create-domain:2", "validate:2", "activate:2", "clone:2", "delete-domain:3", "validate:3", "activate:3"}; !sameStrings(api.events, want) {
		t.Fatalf("versioned cleanup order = %#v, want %#v", api.events, want)
	}
}

func TestNativeLifecycleActivatesAndValidatesBeforePurge(t *testing.T) {
	api := newNativeLifecycleAPI()
	api.services["svc-existing"] = NativeService{ID: "svc-existing", Comment: "magelift/acceptance/test-1", ActiveVersion: 1}
	versions := &nativeLifecycleVersions{api: api}
	lifecycle := NativeLifecycle{API: api, Versions: versions, Probe: healthyFastlyProbe()}
	request := fastlyRequest()
	request.ServiceID = "svc-existing"
	request.CreateService = false

	result, err := lifecycle.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.VersionActivated || !result.PurgeRequested {
		t.Fatalf("native result = %#v", result)
	}
	want := []string{"clone:1", "create-domain:2", "validate:2", "activate:2", "purge"}
	if !sameStrings(api.events, want) {
		t.Fatalf("native mutation order = %#v, want %#v", api.events, want)
	}
}

func TestNativeLifecycleRejectsUnownedExistingServiceBeforeMutation(t *testing.T) {
	api := newNativeLifecycleAPI()
	api.services["svc-existing"] = NativeService{ID: "svc-existing", Comment: "user-owned", ActiveVersion: 1}
	versions := &nativeLifecycleVersions{api: api}
	lifecycle := NativeLifecycle{API: api, Versions: versions, Probe: healthyFastlyProbe()}
	request := fastlyRequest()
	request.ServiceID = "svc-existing"
	request.CreateService = false
	request.PurgeOnDeploy = false

	if _, err := lifecycle.Apply(context.Background(), request); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("unowned existing service error = %v", err)
	}
	if len(versions.cloned) != 0 || len(api.domainVers) != 0 || len(versions.activated) != 0 {
		t.Fatalf("unowned service was mutated: versions=%#v domains=%#v", versions, api.domainVers)
	}
}

func TestNativeLifecycleRejectsCleanupAfterExistingServiceOwnershipDrift(t *testing.T) {
	api := newNativeLifecycleAPI()
	api.services["svc-existing"] = NativeService{ID: "svc-existing", Comment: "user-owned", ActiveVersion: 1}
	versions := &nativeLifecycleVersions{api: api}
	lifecycle := NativeLifecycle{API: api, Versions: versions, Probe: healthyFastlyProbe()}
	request := fastlyRequest()
	request.ServiceID = "svc-existing"
	request.CreateService = false

	result := Result{ServiceID: request.ServiceID, OwnershipMarker: request.OwnershipMarker, CreatedDomains: []string{"preview.example.com"}}
	if err := lifecycle.Destroy(context.Background(), request, result); err == nil || !strings.Contains(err.Error(), "ownership") {
		t.Fatalf("ownership drift cleanup error = %v", err)
	}
	if len(versions.cloned) != 0 || len(api.deleteVers) != 0 {
		t.Fatalf("ownership drift cleanup mutated resources: versions=%#v delete versions=%#v", versions, api.deleteVers)
	}
}

func TestNativeLifecycleRejectsUnsupportedFeaturesBeforeMutation(t *testing.T) {
	api := newNativeLifecycleAPI()
	lifecycle := NativeLifecycle{API: api, Probe: healthyFastlyProbe()}
	request := fastlyRequest()
	request.ProviderConfig.TLS = true
	request.ProviderConfig.TLSMode = "fastly-managed"
	if _, err := lifecycle.Apply(context.Background(), request); err == nil || !strings.Contains(err.Error(), "TLS") {
		t.Fatalf("unsupported native TLS error = %v", err)
	}
	if api.created != 0 || api.purges != 0 {
		t.Fatalf("native API mutated before unsupported feature was rejected: %#v", api)
	}
}

func TestNativeLifecycleRejectsOpaquePoliciesBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "cache", config: Config{CachePolicyRef: "cache/policy"}},
		{name: "purge", config: Config{PurgePolicyRef: "purge/policy"}},
		{name: "WAF", config: Config{WAFPolicyRef: "waf/policy"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := newNativeLifecycleAPI()
			lifecycle := NativeLifecycle{API: api, Probe: healthyFastlyProbe()}
			request := fastlyRequest()
			request.ProviderConfig = test.config
			request.ProviderConfig.OriginHealthRef = "health/magento"
			if _, err := lifecycle.Apply(context.Background(), request); err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsupported") {
				t.Fatalf("opaque policy error = %v", err)
			}
			if api.created != 0 || len(api.domainVers) != 0 || api.purges != 0 {
				t.Fatalf("opaque policy reached Fastly before rejection: %#v", api)
			}
		})
	}
}

func TestNativeLifecycleRejectsOriginWithoutBackendLifecycleBeforeMutation(t *testing.T) {
	api := newNativeLifecycleAPI()
	lifecycle := NativeLifecycle{API: api, Probe: healthyFastlyProbe()}
	request := fastlyRequest()
	request.ProviderConfig.OriginAddress = "origin.example.com"
	request.ProviderConfig.OriginHealthRef = "health/magento"

	if _, err := lifecycle.Apply(context.Background(), request); err == nil || !strings.Contains(err.Error(), "backend/origin") {
		t.Fatalf("unsupported native origin error = %v", err)
	}
	if api.created != 0 || len(api.domainVers) != 0 || api.purges != 0 {
		t.Fatalf("native origin reached Fastly before rejection: %#v", api)
	}
}

func TestNativeLifecycleCleansCreatedServiceAfterVerificationFailure(t *testing.T) {
	api := newNativeLifecycleAPI()
	probe := healthyFastlyProbe()
	probe.verify.RouteHealthy = false
	lifecycle := NativeLifecycle{API: api, Probe: probe, DeletionPolicy: DeletionPolicy{Timeout: time.Second, PollInterval: time.Millisecond, MaxAttempts: 2}}

	_, err := lifecycle.Apply(context.Background(), fastlyRequest())
	if err == nil || !strings.Contains(err.Error(), "route verification") {
		t.Fatalf("verification failure = %v", err)
	}
	if api.created != 1 || api.deleted != 1 || len(api.services) != 0 {
		t.Fatalf("failed apply cleanup = created %d deleted %d services %#v", api.created, api.deleted, api.services)
	}
}

func TestNativeLifecycleUsesOwnedVCLAndManagedTLSFeatureAPIs(t *testing.T) {
	api := newNativeLifecycleAPI()
	features := newNativeLifecycleFeatures()
	lifecycle := NewNativeLifecycleWithFeatures(api, features, healthyFastlyProbe())
	request := fastlyRequest()
	request.ProviderConfig = Config{
		TLS: true, TLSMode: "fastly-managed", VCLName: "magelift-main",
		VCLContent: "sub vcl_recv { return (hash); }", OriginHealthRef: "health/magento",
	}
	result, err := lifecycle.Apply(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !result.CreatedVCL || !result.VCLApplied || result.VCLName != "magelift-main" || !result.CreatedTLS || result.TLSSubscriptionID != "tls-1" {
		t.Fatalf("feature result = %#v", result)
	}
	if features.createdVCL != 1 || features.createdTLS != 1 {
		t.Fatalf("feature creation = VCL %d TLS %d", features.createdVCL, features.createdTLS)
	}
	if err := lifecycle.Destroy(context.Background(), request, result); err != nil {
		t.Fatal(err)
	}
	if features.deletedVCL != 0 || features.deletedTLS != 1 {
		t.Fatalf("feature deletion = VCL %d TLS %d", features.deletedVCL, features.deletedTLS)
	}
}

func TestNativeLifecycleRejectsUnownedVCLNameBeforeMutation(t *testing.T) {
	api := newNativeLifecycleAPI()
	features := newNativeLifecycleFeatures()
	lifecycle := NewNativeLifecycleWithFeatures(api, features, healthyFastlyProbe())
	request := fastlyRequest()
	request.ProviderConfig = Config{VCLName: "user-main", VCLContent: "sub vcl_recv {}", OriginHealthRef: "health/magento"}
	if _, err := lifecycle.Apply(context.Background(), request); err == nil || !strings.Contains(err.Error(), "magelift-") {
		t.Fatalf("unowned VCL name error = %v", err)
	}
	if api.created != 0 || features.createdVCL != 0 {
		t.Fatalf("VCL ownership rejection mutated resources: API=%d VCL=%d", api.created, features.createdVCL)
	}
}

func TestNativeLifecycleExecutesFailoverAndRollbackOnlyThroughInjectedController(t *testing.T) {
	controller := &nativeFailoverController{}
	lifecycle := NewNativeLifecycleWithFailover(nil, nil, nil, controller, healthyFastlyProbe())
	request := fastlyRequest()
	request.ProviderConfig.FailoverPolicyRef = "fastly-policy://magelift/primary-secondary"
	result := Result{ServiceID: "svc-existing", OwnershipMarker: request.OwnershipMarker, Version: "1"}

	failedOver, err := lifecycle.Failover(context.Background(), request, result)
	if err != nil {
		t.Fatal(err)
	}
	if !failedOver.FailoverApplied || failedOver.RollbackApplied || controller.failoverCalls != 1 {
		t.Fatalf("failover result = %#v controller = %#v", failedOver, controller)
	}

	rolledBack, err := lifecycle.Rollback(context.Background(), request, failedOver)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.FailoverApplied || !rolledBack.RollbackApplied || controller.rollbackCalls != 1 {
		t.Fatalf("rollback result = %#v controller = %#v", rolledBack, controller)
	}
}

func TestNativeLifecycleGatesFailoverAndRollbackBeforeControllerMutation(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "failover", action: "failover"},
		{name: "rollback", action: "rollback"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			controller := &nativeFailoverController{}
			probe := healthyFastlyProbe()
			probe.preflight.OriginHealthy = false
			lifecycle := NewNativeLifecycleWithFailover(nil, nil, nil, controller, probe)
			request := fastlyRequest()
			request.ProviderConfig.FailoverPolicyRef = "fastly-policy://magelift/primary-secondary"
			result := Result{ServiceID: "svc-existing", OwnershipMarker: request.OwnershipMarker, Version: "1"}

			var err error
			if test.action == "failover" {
				_, err = lifecycle.Failover(context.Background(), request, result)
			} else {
				_, err = lifecycle.Rollback(context.Background(), request, result)
			}
			if err == nil || !strings.Contains(err.Error(), "origin preflight") {
				t.Fatalf("%s unhealthy preflight error = %v", test.action, err)
			}
			if controller.failoverCalls != 0 || controller.rollbackCalls != 0 {
				t.Fatalf("%s controller mutated before health gate: %#v", test.action, controller)
			}
		})
	}
}

func TestNativeLifecycleDoesNotAdvertiseFailoverWithoutController(t *testing.T) {
	lifecycle := NewNativeLifecycle(nil, healthyFastlyProbe())
	if lifecycle.SupportsFailover() {
		t.Fatal("Fastly lifecycle advertised failover without a controller")
	}
	request := fastlyRequest()
	request.ProviderConfig.FailoverPolicyRef = "fastly-policy://magelift/primary-secondary"
	if _, err := lifecycle.Failover(context.Background(), request, Result{ServiceID: "svc", OwnershipMarker: request.OwnershipMarker}); err == nil || !strings.Contains(err.Error(), "controller") {
		t.Fatalf("failover without controller error = %v", err)
	}
}

func TestNativeLifecycleRejectsVCLContentAboveFastlyPerFileLimitBeforeMutation(t *testing.T) {
	api := newNativeLifecycleAPI()
	features := newNativeLifecycleFeatures()
	lifecycle := NewNativeLifecycleWithFeatures(api, features, healthyFastlyProbe())
	request := fastlyRequest()
	request.ProviderConfig = Config{
		VCLName:         "magelift-main",
		VCLContent:      strings.Repeat("x", maxFastlyVCLBytes+1),
		OriginHealthRef: "health/magento",
	}
	if _, err := lifecycle.Apply(context.Background(), request); err == nil || !strings.Contains(err.Error(), "per-file limit") {
		t.Fatalf("oversized VCL error = %v", err)
	}
	if api.created != 0 || features.createdVCL != 0 {
		t.Fatalf("oversized VCL mutated resources: API=%d VCL=%d", api.created, features.createdVCL)
	}
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
