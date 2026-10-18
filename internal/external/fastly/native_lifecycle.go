package fastly

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NativeLifecycle is the official-SDK Fastly lifecycle backend. The CLI
// backend remains available for features that are not represented by the
// narrow NativeAPI contract; this backend refuses those features before
// creating a service so a partial edge apply cannot be mistaken for support.
type NativeLifecycle struct {
	API                NativeAPI
	Features           NativeFeatureAPI
	Versions           NativeVersionAPI
	FailoverController FailoverController
	Probe              HealthProbe
	DeletionPolicy     DeletionPolicy
}

// NewNativeLifecycle returns a Fastly lifecycle backend backed by the
// official Go SDK boundary. Health verification is mandatory because service
// and domain CRUD alone cannot prove that an origin is safe to route.
func NewNativeLifecycle(api NativeAPI, probe HealthProbe) NativeLifecycle {
	lifecycle := NativeLifecycle{API: api, Probe: probe}
	if versions, ok := api.(NativeVersionAPI); ok {
		lifecycle.Versions = versions
	}
	return lifecycle
}

// NewNativeLifecycleWithFeatures opts a service lifecycle into the official
// VCL and TLS APIs. Keeping these features optional preserves a small service
// CRUD implementation for community adapters while ensuring a request that
// needs VCL or managed TLS fails before any mutation when the feature client
// is absent.
func NewNativeLifecycleWithFeatures(api NativeAPI, features NativeFeatureAPI, probe HealthProbe) NativeLifecycle {
	lifecycle := NativeLifecycle{API: api, Features: features, Probe: probe}
	if versions, ok := api.(NativeVersionAPI); ok {
		lifecycle.Versions = versions
	}
	return lifecycle
}

// NewNativeLifecycleWithFeaturesAndVersions opts into the complete first-party
// Fastly service-version lifecycle. Versions is deliberately a separate port:
// a community implementation can provide service/domain CRUD or feature APIs
// without pretending it can safely clone, validate, and activate versions.
func NewNativeLifecycleWithFeaturesAndVersions(api NativeAPI, features NativeFeatureAPI, versions NativeVersionAPI, probe HealthProbe) NativeLifecycle {
	return NativeLifecycle{API: api, Features: features, Versions: versions, Probe: probe}
}

// NewNativeLifecycleWithFailover adds an explicitly injected Fastly policy
// translator. The controller is optional because Fastly failover semantics
// depend on the selected backend/health-check/VCL policy; a service lifecycle
// without it must not advertise failover or rollback capabilities.
func NewNativeLifecycleWithFailover(api NativeAPI, features NativeFeatureAPI, versions NativeVersionAPI, failover FailoverController, probe HealthProbe) NativeLifecycle {
	return NativeLifecycle{API: api, Features: features, Versions: versions, FailoverController: failover, Probe: probe}
}

// SupportsFailover reports whether this lifecycle has a provider-owned
// failover translator. It is used by the SDK bridge to advertise only actual
// mutations, not health-proof fields that happen to exist in a request.
func (lifecycle NativeLifecycle) SupportsFailover() bool { return lifecycle.FailoverController != nil }

func (lifecycle NativeLifecycle) Plan(request Request) (Plan, error) {
	plan, err := planRequest(request, lifecycle.FailoverController != nil)
	if err != nil {
		return Plan{}, err
	}
	return plan, nil
}

func (lifecycle NativeLifecycle) Apply(ctx context.Context, request Request) (result Result, err error) {
	if ctx == nil {
		return Result{}, errors.New("Fastly native apply context is required")
	}
	if lifecycle.API == nil {
		return Result{}, errors.New("Fastly native API client is required")
	}
	if lifecycle.Probe == nil {
		return Result{}, errors.New("Fastly origin safety probe is required before apply")
	}
	plan, err := lifecycle.Plan(request)
	if err != nil {
		return Result{}, err
	}
	if err := validateNativeCapabilities(plan.ProviderConfig, lifecycle.Features); err != nil {
		return Result{}, err
	}
	preflight, err := lifecycle.Probe.Preflight(ctx, request, plan.ServiceID)
	if err != nil {
		return Result{}, fmt.Errorf("Fastly native origin preflight: %w", err)
	}
	if !preflight.OriginHealthy {
		return Result{}, errors.New("Fastly native origin preflight did not verify a healthy origin")
	}

	result = Result{
		ServiceID:       plan.ServiceID,
		CreatedService:  plan.CreateService,
		OwnershipMarker: plan.OwnershipMarker,
		Version:         plan.ProviderConfig.Version,
	}
	cleanupResult := result
	cleanupRequired := false
	previousVersion := 0
	versionRestoreNeeded := false
	updateCleanupResult := func() {
		cleanupResult = result
	}
	defer func() {
		if err == nil || !cleanupRequired {
			return
		}
		cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if versionRestoreNeeded && previousVersion > 0 {
			if restoreErr := lifecycle.restoreVersion(cleanupContext, cleanupResult.ServiceID, previousVersion); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore Fastly service version %d after failed apply: %w", previousVersion, restoreErr))
			} else {
				versionRestoreNeeded = false
			}
		}
		if cleanupErr := lifecycle.Destroy(cleanupContext, request, cleanupResult); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup Fastly apply failure: %w", cleanupErr))
		}
	}()
	if plan.CreateService {
		name := request.ServiceName
		if name == "" {
			name = request.OwnershipMarker
		}
		service, err := lifecycle.API.CreateService(ctx, name, request.OwnershipMarker)
		if err != nil {
			return Result{}, fmt.Errorf("create Fastly service: %w", err)
		}
		if strings.TrimSpace(service.ID) == "" || service.Comment != request.OwnershipMarker {
			return Result{}, errors.New("refusing to mutate Fastly service without an exact ownership comment")
		}
		cleanupRequired = true
		result.ServiceID = service.ID
		result.Version = nativeServiceVersion(service)
		updateCleanupResult()
	} else {
		service, err := lifecycle.API.GetService(ctx, plan.ServiceID)
		if err != nil {
			return Result{}, fmt.Errorf("get Fastly service: %w", err)
		}
		if strings.TrimSpace(service.ID) == "" || service.ID != plan.ServiceID {
			return Result{}, errors.New("Fastly service response does not match the requested service")
		}
		if service.Comment != request.OwnershipMarker {
			return Result{}, errors.New("refusing to mutate Fastly service without an exact ownership comment")
		}
		result.Version = nativeServiceVersion(service)
	}
	if strings.TrimSpace(result.ServiceID) == "" {
		return Result{}, errors.New("Fastly service response has no service identity")
	}
	version, err := strconv.Atoi(result.Version)
	if err != nil || version <= 0 {
		return Result{}, errors.New("Fastly service response has no active version")
	}

	domains, err := lifecycle.API.ListServiceDomains(ctx, result.ServiceID)
	if err != nil {
		return Result{}, fmt.Errorf("list Fastly domains: %w", err)
	}
	owned := make(map[string]struct{}, len(domains))
	for _, domain := range domains {
		if domain.Comment == plan.OwnershipMarker {
			owned[domain.FQDN] = struct{}{}
		}
	}
	toCreate := make([]string, 0, len(plan.Domains))
	for _, domain := range plan.Domains {
		if _, exists := owned[domain]; exists {
			result.CreatedDomains = append(result.CreatedDomains, domain)
			continue
		}
		for _, existing := range domains {
			if existing.FQDN == domain {
				return Result{}, fmt.Errorf("refusing to change unowned Fastly domain %q", domain)
			}
		}
		toCreate = append(toCreate, domain)
	}

	if !plan.CreateService && (len(toCreate) > 0 || plan.ProviderConfig.VCLContent != "") {
		if lifecycle.Versions == nil {
			return Result{}, errors.New("Fastly native version lifecycle client is required for existing-service mutations")
		}
		previousVersion = version
		cloned, err := lifecycle.Versions.CloneVersion(ctx, result.ServiceID, version)
		if err != nil {
			return Result{}, fmt.Errorf("clone Fastly service version %d: %w", version, err)
		}
		if cloned.Number <= 0 {
			return Result{}, errors.New("Fastly clone response has no version identity")
		}
		version = cloned.Number
		versionRestoreNeeded = true
		result.Version = strconv.Itoa(version)
		cleanupRequired = true
		updateCleanupResult()
	}
	for _, domain := range toCreate {
		if _, err := lifecycle.API.CreateDomain(ctx, result.ServiceID, version, domain, plan.OwnershipMarker); err != nil {
			return Result{}, fmt.Errorf("create Fastly domain %q: %w", domain, err)
		}
		result.CreatedDomains = append(result.CreatedDomains, domain)
		cleanupRequired = true
		updateCleanupResult()
	}
	if featureErr := lifecycle.applyFeatures(ctx, plan, result.ServiceID, version, &result); featureErr != nil {
		updateCleanupResult()
		return Result{}, featureErr
	}
	if result.CreatedVCL || result.CreatedTLS {
		cleanupRequired = true
		updateCleanupResult()
	}
	if previousVersion > 0 {
		if err := lifecycle.activateValidatedVersion(ctx, result.ServiceID, version); err != nil {
			return Result{}, err
		}
		result.VersionActivated = true
		updateCleanupResult()
	}
	if plan.PurgeOnDeploy {
		purgeID, err := lifecycle.API.PurgeAll(ctx, result.ServiceID)
		if err != nil {
			return Result{}, fmt.Errorf("purge Fastly service: %w", err)
		}
		result.PurgeRequested = true
		if strings.TrimSpace(purgeID) == "" {
			return Result{}, errors.New("Fastly purge response has no operation identity")
		}
	}
	if err := lifecycle.verify(ctx, request, plan, result); err != nil {
		if previousVersion > 0 {
			if rollbackErr := lifecycle.restoreVersion(ctx, result.ServiceID, previousVersion); rollbackErr != nil {
				return Result{}, errors.Join(err, fmt.Errorf("restore Fastly service version %d after verification failure: %w", previousVersion, rollbackErr))
			}
			versionRestoreNeeded = false
		}
		return Result{}, err
	}
	versionRestoreNeeded = false
	result.Outputs = fastlyOutputs(plan, &result)
	return result, nil
}

func (lifecycle NativeLifecycle) Verify(ctx context.Context, request Request, result Result) error {
	if lifecycle.Probe == nil {
		return errors.New("Fastly origin safety probe is required before verification")
	}
	plan, err := lifecycle.Plan(request)
	if err != nil {
		return err
	}
	if err := validateNativeCapabilities(plan.ProviderConfig, lifecycle.Features); err != nil {
		return err
	}
	return lifecycle.verify(ctx, request, plan, result)
}

// Failover executes the selected Fastly failover policy and verifies that the
// route converged to the failover state. It intentionally does not infer a
// mutation from HealthObservation flags alone.
func (lifecycle NativeLifecycle) Failover(ctx context.Context, request Request, result Result) (Result, error) {
	if lifecycle.FailoverController == nil {
		return Result{}, errors.New("Fastly failover controller is required")
	}
	if result.ServiceID == "" || result.OwnershipMarker != request.OwnershipMarker {
		return Result{}, errors.New("Fastly failover ownership proof does not match the request")
	}
	plan, err := lifecycle.Plan(request)
	if err != nil {
		return Result{}, err
	}
	if plan.ProviderConfig.FailoverPolicyRef == "" {
		return Result{}, errors.New("Fastly failover policy reference is required")
	}
	if err := lifecycle.preflightRouteMutation(ctx, request, result, "failover"); err != nil {
		return Result{}, err
	}
	observation, err := lifecycle.FailoverController.Failover(ctx, request, result)
	if err != nil {
		return Result{}, fmt.Errorf("execute Fastly failover: %w", err)
	}
	if strings.TrimSpace(observation.OperationID) == "" || !observation.FailoverVerified {
		return Result{}, errors.New("Fastly failover controller did not prove route convergence")
	}
	result.FailoverApplied = true
	result.RollbackApplied = false
	if err := lifecycle.verifyFailoverState(ctx, request, plan, result, true); err != nil {
		return Result{}, err
	}
	return result, nil
}

// Rollback restores the previous Fastly route through the same provider-owned
// policy translator and verifies rollback convergence before returning.
func (lifecycle NativeLifecycle) Rollback(ctx context.Context, request Request, result Result) (Result, error) {
	if lifecycle.FailoverController == nil {
		return Result{}, errors.New("Fastly failover controller is required")
	}
	if result.ServiceID == "" || result.OwnershipMarker != request.OwnershipMarker {
		return Result{}, errors.New("Fastly rollback ownership proof does not match the request")
	}
	plan, err := lifecycle.Plan(request)
	if err != nil {
		return Result{}, err
	}
	if plan.ProviderConfig.FailoverPolicyRef == "" {
		return Result{}, errors.New("Fastly rollback policy reference is required")
	}
	if err := lifecycle.preflightRouteMutation(ctx, request, result, "rollback"); err != nil {
		return Result{}, err
	}
	observation, err := lifecycle.FailoverController.Rollback(ctx, request, result)
	if err != nil {
		return Result{}, fmt.Errorf("execute Fastly rollback: %w", err)
	}
	if strings.TrimSpace(observation.OperationID) == "" || !observation.RollbackVerified {
		return Result{}, errors.New("Fastly rollback controller did not prove route convergence")
	}
	result.FailoverApplied = false
	result.RollbackApplied = true
	if err := lifecycle.verifyFailoverState(ctx, request, plan, result, false); err != nil {
		return Result{}, err
	}
	return result, nil
}

func (lifecycle NativeLifecycle) preflightRouteMutation(ctx context.Context, request Request, result Result, action string) error {
	if lifecycle.Probe == nil {
		return errors.New("Fastly origin safety probe is required before " + action)
	}
	observation, err := lifecycle.Probe.Preflight(ctx, request, result.ServiceID)
	if err != nil {
		return fmt.Errorf("Fastly %s origin preflight: %w", action, err)
	}
	if !observation.OriginHealthy {
		return fmt.Errorf("Fastly %s origin preflight did not verify a healthy origin", action)
	}
	return nil
}

func (lifecycle NativeLifecycle) verifyFailoverState(ctx context.Context, request Request, plan Plan, result Result, failover bool) error {
	if lifecycle.Probe == nil {
		return errors.New("Fastly origin safety probe is required before failover verification")
	}
	observation, err := lifecycle.Probe.Verify(ctx, request, result)
	if err != nil {
		return fmt.Errorf("verify Fastly route after %s: %w", failoverAction(failover), err)
	}
	if !observation.OriginHealthy || !observation.RouteHealthy {
		return fmt.Errorf("verify Fastly route after %s: origin and route are not healthy", failoverAction(failover))
	}
	if failover && !observation.FailoverVerified {
		return errors.New("Fastly route verification did not prove failover convergence")
	}
	if !failover && !observation.RollbackVerified {
		return errors.New("Fastly route verification did not prove rollback convergence")
	}
	if len(plan.Domains) > 0 && !observation.DNSOwnershipVerified {
		return errors.New("Fastly route verification did not prove DNS ownership")
	}
	return nil
}

func failoverAction(failover bool) string {
	if failover {
		return "failover"
	}
	return "rollback"
}

func (lifecycle NativeLifecycle) verify(ctx context.Context, request Request, plan Plan, result Result) error {
	postflight, err := lifecycle.Probe.Verify(ctx, request, result)
	if err != nil {
		return fmt.Errorf("Fastly native route verification: %w", err)
	}
	if !postflight.OriginHealthy || !postflight.RouteHealthy {
		return errors.New("Fastly native route verification did not verify a healthy origin and route")
	}
	if len(plan.Domains) > 0 && !postflight.DNSOwnershipVerified {
		return errors.New("Fastly native route verification did not verify DNS ownership")
	}
	if plan.PurgeOnDeploy && !postflight.PurgeVerified {
		return errors.New("Fastly native route verification did not verify purge completion")
	}
	if plan.ProviderConfig.CachePolicyRef != "" && !postflight.CachePolicyVerified {
		return errors.New("Fastly native route verification did not verify the cache policy")
	}
	if plan.ProviderConfig.WAFPolicyRef != "" && !postflight.WAFVerified {
		return errors.New("Fastly native route verification did not verify the WAF or security policy")
	}
	if plan.ProviderConfig.FailoverPolicyRef != "" && (!postflight.FailoverVerified || !postflight.RollbackVerified) {
		return errors.New("Fastly native route verification did not verify failover and rollback")
	}
	return nil
}

func (lifecycle NativeLifecycle) activateValidatedVersion(ctx context.Context, serviceID string, version int) error {
	if lifecycle.Versions == nil {
		return errors.New("Fastly native version lifecycle client is required")
	}
	validation, err := lifecycle.Versions.ValidateVersion(ctx, serviceID, version)
	if err != nil {
		return fmt.Errorf("validate Fastly service version %d: %w", version, err)
	}
	if !strings.EqualFold(validation.Status, "ok") {
		message := strings.TrimSpace(validation.Message)
		if message == "" {
			message = "Fastly did not report the version as valid"
		}
		return fmt.Errorf("validate Fastly service version %d: %s", version, message)
	}
	activated, err := lifecycle.Versions.ActivateVersion(ctx, serviceID, version)
	if err != nil {
		return fmt.Errorf("activate Fastly service version %d: %w", version, err)
	}
	if activated.Number != version {
		return fmt.Errorf("Fastly activation response selected version %d, expected %d", activated.Number, version)
	}
	return nil
}

func (lifecycle NativeLifecycle) restoreVersion(ctx context.Context, serviceID string, version int) error {
	if lifecycle.Versions == nil {
		return errors.New("Fastly native version lifecycle client is required")
	}
	activated, err := lifecycle.Versions.ActivateVersion(ctx, serviceID, version)
	if err != nil {
		return fmt.Errorf("activate previous Fastly service version %d: %w", version, err)
	}
	if activated.Number != version {
		return fmt.Errorf("Fastly rollback response selected version %d, expected %d", activated.Number, version)
	}
	return nil
}

func (lifecycle NativeLifecycle) Destroy(ctx context.Context, request Request, result Result) error {
	if lifecycle.API == nil {
		return errors.New("Fastly native API client is required")
	}
	if result.ServiceID == "" || result.OwnershipMarker != request.OwnershipMarker {
		return errors.New("Fastly native cleanup ownership proof does not match the request")
	}
	plan, err := lifecycle.Plan(request)
	if err != nil {
		return err
	}
	if err := validateNativeCapabilities(plan.ProviderConfig, lifecycle.Features); err != nil {
		return err
	}
	domains, err := lifecycle.API.ListServiceDomains(ctx, result.ServiceID)
	if err != nil {
		return fmt.Errorf("list Fastly domains for cleanup: %w", err)
	}
	service, err := lifecycle.API.GetService(ctx, result.ServiceID)
	if err != nil {
		return fmt.Errorf("get Fastly service for cleanup: %w", err)
	}
	if service.Comment != request.OwnershipMarker {
		return errors.New("refusing to clean Fastly service without an exact ownership comment")
	}
	version, err := strconv.Atoi(nativeServiceVersion(service))
	if err != nil || version <= 0 {
		return errors.New("Fastly service cleanup has no active version")
	}
	if result.CreatedService {
		if result.CreatedTLS && result.TLSSubscriptionID != "" {
			if lifecycle.Features == nil {
				return errors.New("Fastly native TLS feature client is required to remove an owned subscription")
			}
			if err := lifecycle.Features.DeleteTLSSubscription(ctx, result.TLSSubscriptionID); err != nil {
				return fmt.Errorf("delete Fastly TLS subscription: %w", err)
			}
		}
		if err := lifecycle.API.DeleteService(ctx, result.ServiceID); err != nil {
			return fmt.Errorf("delete Fastly service: %w", err)
		}
		return lifecycle.waitForDeletion(ctx, request, result)
	}

	ownedDomains := make([]NativeDomain, 0, len(domains))
	for _, domain := range domains {
		if domain.Comment == request.OwnershipMarker {
			ownedDomains = append(ownedDomains, domain)
		}
	}
	if len(ownedDomains) > 0 || result.CreatedVCL {
		if lifecycle.Versions == nil {
			return errors.New("Fastly native version lifecycle client is required for existing-service cleanup")
		}
		cloned, err := lifecycle.Versions.CloneVersion(ctx, result.ServiceID, version)
		if err != nil {
			return fmt.Errorf("clone Fastly service version %d for cleanup: %w", version, err)
		}
		if cloned.Number <= 0 {
			return errors.New("Fastly cleanup clone response has no version identity")
		}
		cleanupVersion := cloned.Number
		for _, domain := range ownedDomains {
			if err := lifecycle.API.DeleteDomain(ctx, result.ServiceID, cleanupVersion, domain.NameOrFQDN()); err != nil {
				return fmt.Errorf("delete Fastly domain %q: %w", domain.FQDN, err)
			}
		}
		if result.CreatedVCL {
			if lifecycle.Features == nil {
				return errors.New("Fastly native VCL feature client is required to remove an owned VCL")
			}
			if err := lifecycle.Features.DeleteVCL(ctx, result.ServiceID, cleanupVersion, result.VCLName); err != nil {
				return fmt.Errorf("delete Fastly VCL %q: %w", result.VCLName, err)
			}
		}
		if err := lifecycle.activateValidatedVersion(ctx, result.ServiceID, cleanupVersion); err != nil {
			return err
		}
	}
	if result.CreatedTLS && result.TLSSubscriptionID != "" {
		if lifecycle.Features == nil {
			return errors.New("Fastly native TLS feature client is required to remove an owned subscription")
		}
		if err := lifecycle.Features.DeleteTLSSubscription(ctx, result.TLSSubscriptionID); err != nil {
			return fmt.Errorf("delete Fastly TLS subscription: %w", err)
		}
	}
	return lifecycle.waitForDeletion(ctx, request, result)
}

func (lifecycle NativeLifecycle) waitForDeletion(ctx context.Context, request Request, result Result) error {
	policy := lifecycle.DeletionPolicy
	if policy.Timeout == 0 && policy.PollInterval == 0 && policy.MaxAttempts == 0 {
		policy = DeletionPolicy{Timeout: 2 * time.Minute, PollInterval: 2 * time.Second, MaxAttempts: 60}
	}
	observation, err := WaitForDeletion(ctx, nativeDeletionProbe{api: lifecycle.API, features: lifecycle.Features, request: request, result: result}, request, result, policy)
	if err != nil {
		return err
	}
	if !observation.Complete {
		return fmt.Errorf("verify Fastly native cleanup: %s", deletionDetail(observation))
	}
	return nil
}

type nativeDeletionProbe struct {
	api      NativeAPI
	features NativeFeatureAPI
	request  Request
	result   Result
}

func (probe nativeDeletionProbe) PollDeletion(ctx context.Context, request Request, result Result) (DeletionObservation, error) {
	resources, err := probe.api.Inventory(ctx, request.OwnershipMarker)
	if err != nil {
		return DeletionObservation{}, err
	}
	remaining := make([]string, 0, len(resources))
	for _, resource := range resources {
		if !resource.Owned || !resource.Live {
			continue
		}
		if result.CreatedService || strings.HasPrefix(resource.Identity, "domain:") {
			remaining = append(remaining, resource.Identity)
		}
	}
	if probe.result.CreatedVCL && probe.features != nil {
		service, serviceErr := probe.api.GetService(ctx, probe.result.ServiceID)
		if serviceErr == nil && service.ActiveVersion > 0 {
			vcls, vclErr := probe.features.ListVCLs(ctx, probe.result.ServiceID, service.ActiveVersion)
			if vclErr != nil {
				return DeletionObservation{}, vclErr
			}
			for _, vcl := range vcls {
				if vcl.Name == probe.result.VCLName {
					remaining = append(remaining, "vcl:"+vcl.Name)
				}
			}
		}
	}
	if probe.result.CreatedTLS && probe.result.TLSSubscriptionID != "" && probe.features != nil {
		subscriptions, subscriptionErr := probe.features.ListTLSSubscriptions(ctx)
		if subscriptionErr != nil {
			return DeletionObservation{}, subscriptionErr
		}
		for _, subscription := range subscriptions {
			if subscription.ID == probe.result.TLSSubscriptionID {
				remaining = append(remaining, "tls-subscription:"+subscription.ID)
			}
		}
	}
	return DeletionObservation{Complete: len(remaining) == 0, ResourceRefs: remaining}, nil
}

func nativeServiceVersion(service NativeService) string {
	if service.ActiveVersion > 0 {
		return strconv.Itoa(service.ActiveVersion)
	}
	if service.Version > 0 {
		return strconv.Itoa(service.Version)
	}
	return ""
}

func (domain NativeDomain) NameOrFQDN() string {
	if strings.TrimSpace(domain.Name) != "" {
		return domain.Name
	}
	return domain.FQDN
}

func validateNativeCapabilities(config Config, features NativeFeatureAPI) error {
	if strings.TrimSpace(config.OriginAddress) != "" {
		return errors.New("Fastly official SDK lifecycle does not implement versioned backend/origin lifecycle")
	}
	unsupported := make([]string, 0, 4)
	if config.TLS && config.TLSMode == "fastly-managed" && config.TLSSubscriptionID == "" && features == nil {
		unsupported = append(unsupported, "TLS/certificate lifecycle")
	}
	if config.VCLContent != "" && features == nil {
		unsupported = append(unsupported, "VCL lifecycle")
	}
	if config.VCLContent != "" && !strings.HasPrefix(config.VCLName, "magelift-") {
		unsupported = append(unsupported, "VCL ownership name (must start with magelift-)")
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("Fastly official SDK lifecycle requires a feature API for %s", strings.Join(unsupported, ", "))
	}
	return nil
}

func (lifecycle NativeLifecycle) applyFeatures(ctx context.Context, plan Plan, serviceID string, version int, result *Result) error {
	if result == nil {
		return errors.New("Fastly native feature result is required")
	}
	config := plan.ProviderConfig
	if config.VCLContent != "" {
		if lifecycle.Features == nil {
			return errors.New("Fastly native VCL feature client is required")
		}
		if !strings.HasPrefix(config.VCLName, "magelift-") {
			return errors.New("Fastly native VCL name must start with magelift- for ownership")
		}
		vcls, err := lifecycle.Features.ListVCLs(ctx, serviceID, version)
		if err != nil {
			return fmt.Errorf("list Fastly VCLs: %w", err)
		}
		for _, vcl := range vcls {
			if vcl.Name != config.VCLName {
				continue
			}
			if vcl.Content != config.VCLContent {
				return fmt.Errorf("refusing to change existing Fastly VCL %q with different content", config.VCLName)
			}
			result.VCLName = config.VCLName
			result.VCLApplied = true
			return lifecycle.applyTLS(ctx, plan, result)
		}
		if _, err := lifecycle.Features.CreateVCL(ctx, serviceID, version, config.VCLName, config.VCLContent); err != nil {
			return fmt.Errorf("create Fastly VCL %q: %w", config.VCLName, err)
		}
		result.VCLName = config.VCLName
		result.CreatedVCL = true
		if err := lifecycle.Features.SetVCLMain(ctx, serviceID, version, config.VCLName); err != nil {
			return fmt.Errorf("set Fastly VCL %q as main: %w", config.VCLName, err)
		}
		result.VCLApplied = true
	}
	return lifecycle.applyTLS(ctx, plan, result)
}

func (lifecycle NativeLifecycle) applyTLS(ctx context.Context, plan Plan, result *Result) error {
	config := plan.ProviderConfig
	if !config.TLS || config.TLSMode != "fastly-managed" || config.TLSSubscriptionID != "" {
		result.TLSSubscriptionID = config.TLSSubscriptionID
		return nil
	}
	if lifecycle.Features == nil {
		return errors.New("Fastly native TLS feature client is required")
	}
	existing, err := lifecycle.Features.ListTLSSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("list Fastly TLS subscriptions: %w", err)
	}
	wanted := sortedUniqueStrings(plan.Domains)
	for _, subscription := range existing {
		if sameStringSet(subscription.Domains, wanted) {
			return errors.New("Fastly TLS subscription already exists for the requested domains; select its opaque ID explicitly")
		}
	}
	subscription, err := lifecycle.Features.CreateTLSSubscription(ctx, wanted, wanted[0])
	if err != nil {
		return fmt.Errorf("create Fastly TLS subscription: %w", err)
	}
	if strings.TrimSpace(subscription.ID) == "" {
		return errors.New("Fastly TLS subscription response has no identity")
	}
	result.TLSSubscriptionID = subscription.ID
	result.CreatedTLS = true
	return nil
}

func sortedUniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sameStringSet(left, right []string) bool {
	left = sortedUniqueStrings(left)
	right = sortedUniqueStrings(right)
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
