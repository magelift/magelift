package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/ovh/go-ovh/ovh"
)

const (
	ovhLogsDataPlatformDestination = "ovh-logs-data-platform"
	ovhKubeAuditKind               = "audit"
)

// OVHSubscription is the provider-neutral subset of the MKS log-subscription
// response. The OVH API does not expose arbitrary ownership labels on this
// resource, so the service/cluster/stream tuple identifies the intended
// attachment, while lifecycle references distinguish resources created by this
// run from pre-existing resources that must be preserved.
type OVHSubscription struct {
	CreatedAt      string                  `json:"createdAt"`
	Kind           string                  `json:"kind"`
	Resource       OVHSubscriptionResource `json:"resource"`
	ServiceName    string                  `json:"serviceName"`
	StreamID       string                  `json:"streamId"`
	SubscriptionID string                  `json:"subscriptionId"`
	UpdatedAt      string                  `json:"updatedAt"`
}

type OVHSubscriptionResource struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type OVHSubscriptionOperation struct {
	OperationID string `json:"operationId"`
	ServiceName string `json:"serviceName"`
}

// OVHKubeLogsAPI is the narrow semantic surface used by the OVH lifecycle
// translator. Provider SDK response types stop at this boundary.
type OVHKubeLogsAPI interface {
	ListSubscriptions(context.Context, string, string) ([]string, error)
	GetSubscription(context.Context, string, string, string) (OVHSubscription, error)
	CreateSubscription(context.Context, string, string, string, string) (OVHSubscriptionOperation, error)
	DeleteSubscription(context.Context, string, string, string) (OVHSubscriptionOperation, error)
}

type sdkKubeLogsAPI struct {
	client *ovh.Client
}

// NewOVHCloudProjectKubeLogsSDKClient constructs the shared lifecycle client
// from an already-authenticated official OVH SDK client. Authentication and
// secret resolution stay in the provider boundary; no credential value enters
// the portable observability plan.
func NewOVHCloudProjectKubeLogsSDKClient(client *ovh.Client, serviceName, kubeID, streamID string) (*providerobservability.ManagedLifecycleClient, error) {
	if client == nil {
		return nil, errors.New("OVH SDK client is required")
	}
	backend, err := NewOVHKubeAuditLogsBackend(serviceName, kubeID, streamID, sdkKubeLogsAPI{client: client})
	if err != nil {
		return nil, err
	}
	return providerobservability.NewManagedLifecycleClient(backend)
}

func (api sdkKubeLogsAPI) ListSubscriptions(ctx context.Context, serviceName, kubeID string) ([]string, error) {
	var subscriptionIDs []string
	if err := api.client.GetWithContext(ctx, subscriptionPath(serviceName, kubeID), &subscriptionIDs); err != nil {
		return nil, err
	}
	return subscriptionIDs, nil
}

func (api sdkKubeLogsAPI) GetSubscription(ctx context.Context, serviceName, kubeID, subscriptionID string) (OVHSubscription, error) {
	var subscription OVHSubscription
	if err := api.client.GetWithContext(ctx, subscriptionPath(serviceName, kubeID)+"/"+escapePath(subscriptionID), &subscription); err != nil {
		return OVHSubscription{}, err
	}
	return subscription, nil
}

func (api sdkKubeLogsAPI) CreateSubscription(ctx context.Context, serviceName, kubeID, kind, streamID string) (OVHSubscriptionOperation, error) {
	request := struct {
		Kind     string `json:"kind"`
		StreamID string `json:"streamId"`
	}{Kind: kind, StreamID: streamID}
	var operation OVHSubscriptionOperation
	if err := api.client.PostWithContext(ctx, subscriptionPath(serviceName, kubeID), request, &operation); err != nil {
		return OVHSubscriptionOperation{}, err
	}
	return operation, nil
}

func (api sdkKubeLogsAPI) DeleteSubscription(ctx context.Context, serviceName, kubeID, subscriptionID string) (OVHSubscriptionOperation, error) {
	var operation OVHSubscriptionOperation
	if err := api.client.DeleteWithContext(ctx, subscriptionPath(serviceName, kubeID)+"/"+escapePath(subscriptionID), &operation); err != nil {
		return OVHSubscriptionOperation{}, err
	}
	return operation, nil
}

// OVHKubeAuditLogsBackend owns only the documented MKS audit subscription
// targeting the exact stream reference supplied by the architecture. OVH's
// current provider API does not expose arbitrary ownership labels, generic
// workload telemetry, or stream retention/redaction control at this seam.
type OVHKubeAuditLogsBackend struct {
	serviceName          string
	kubeID               string
	streamID             string
	api                  OVHKubeLogsAPI
	pollAttempts         int
	pollInterval         time.Duration
	createdMu            sync.RWMutex
	createdSubscriptions map[string]struct{}
}

var _ providerobservability.Backend = (*OVHKubeAuditLogsBackend)(nil)

func NewOVHKubeAuditLogsBackend(serviceName, kubeID, streamID string, api OVHKubeLogsAPI) (*OVHKubeAuditLogsBackend, error) {
	if strings.TrimSpace(serviceName) == "" || strings.TrimSpace(kubeID) == "" || strings.TrimSpace(streamID) == "" {
		return nil, errors.New("OVH service, MKS cluster, and Logs Data Platform stream are required")
	}
	if strings.ContainsAny(serviceName+kubeID+streamID, "\r\n\x00") {
		return nil, errors.New("OVH service, MKS cluster, and Logs Data Platform stream must be single-line")
	}
	if api == nil {
		return nil, errors.New("OVH Kubernetes logs API is required")
	}
	return &OVHKubeAuditLogsBackend{
		serviceName:          serviceName,
		kubeID:               kubeID,
		streamID:             streamID,
		api:                  api,
		pollAttempts:         20,
		pollInterval:         250 * time.Millisecond,
		createdSubscriptions: make(map[string]struct{}),
	}, nil
}

func (backend *OVHKubeAuditLogsBackend) Apply(ctx context.Context, plan providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	}
	if len(plan.Alerts) > 0 || len(plan.Dashboards) > 0 || len(plan.SLOs) > 0 {
		return providerobservability.LifecycleResult{}, errors.New("OVH Logs Data Platform audit adapter does not implement alert, dashboard, or SLO lifecycle")
	}
	resources, err := backend.inventorySubscriptions(ctx)
	if err != nil {
		return providerobservability.LifecycleResult{}, errors.New("inventory OVH MKS log subscriptions before apply failed")
	}
	createdThisApply := make(map[string]struct{})
	refs := make(map[string]struct{}, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		if binding.OwnershipMarker != plan.OwnershipMarker {
			return providerobservability.LifecycleResult{}, errors.New("OVH binding ownership does not match the plan")
		}
		if binding.Destination != ovhLogsDataPlatformDestination {
			return providerobservability.LifecycleResult{}, errors.New("OVH binding destination does not match the adapter")
		}
		subscription, created, err := backend.ensureSubscription(ctx, resources, createdThisApply)
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		resources = append(resources, subscription)
		refs[subscriptionReference(subscription.SubscriptionID, created)] = struct{}{}
	}
	resourceRefs := sortedKeys(refs)
	return providerobservability.LifecycleResult{
		OperationID:         "ovh-logs-data-platform:observability:apply:" + markerDigest(plan.OwnershipMarker),
		ResourceRefs:        resourceRefs,
		ProofRefs:           []string{"ovh-mks-audit:identity-by-service-cluster-stream", "ovh-mks-audit:explicit-created-or-existing-reference", "ovh-mks-audit:direct-api", "ovh-mks-audit:bounded-poll"},
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (backend *OVHKubeAuditLogsBackend) VerifySignal(ctx context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	if ctx == nil {
		return providerobservability.SignalObservation{}, errors.New("OVH observability signal context is required")
	}
	if err := ctx.Err(); err != nil {
		return providerobservability.SignalObservation{}, err
	}
	observation := providerobservability.SignalObservation{Signal: binding.Signal, Destination: binding.Destination}
	if binding.Destination != ovhLogsDataPlatformDestination {
		observation.Reason = "OVH Logs Data Platform destination does not match the adapter"
		return observation, nil
	}
	if binding.Signal != "audit-events" && binding.Signal != "provider-operations" {
		observation.Reason = "OVH MKS audit forwarding does not provide this signal"
		return observation, nil
	}
	if binding.OwnershipMarker == "" {
		return providerobservability.SignalObservation{}, errors.New("OVH observability signal ownership marker is required")
	}
	subscription, found, err := backend.findOwnedSubscription(ctx)
	if err != nil {
		return providerobservability.SignalObservation{}, errors.New("verify OVH MKS log subscription failed")
	}
	if !found {
		observation.Reason = "OVH MKS audit subscription is missing"
		return observation, nil
	}
	observation.LabelsVerified = subscription.ServiceName == backend.serviceName && subscription.Kind == ovhKubeAuditKind && subscription.StreamID == backend.streamID
	observation.RetentionVerified = binding.RetentionDays == 0
	observation.RedactionVerified = strings.TrimSpace(binding.RedactionPolicy) == ""
	// The control-plane subscription API proves attachment, not arrival in the
	// destination stream. A Logs Data Platform query/live-tail proof must be
	// supplied by a separate probe before certification can pass.
	observation.Reason = "OVH MKS subscription is attached, but the current lifecycle API does not prove destination-stream delivery"
	return observation, nil
}

func (backend *OVHKubeAuditLogsBackend) VerifyOperations(ctx context.Context, plan providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	return providerobservability.OperationalObservation{
		AlertsVerified:     len(plan.Alerts) == 0,
		DashboardsVerified: len(plan.Dashboards) == 0,
		SLOsVerified:       len(plan.SLOs) == 0,
		Reason:             "OVH Logs Data Platform audit forwarding has no provider-neutral alert, dashboard, or SLO lifecycle in this adapter",
	}, nil
}

func (backend *OVHKubeAuditLogsBackend) Destroy(ctx context.Context, plan providerobservability.Plan, resourceReferences []string) error {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return err
	}
	if len(resourceReferences) == 0 {
		return errors.New("OVH MKS cleanup requires explicit subscription references; refusing tuple-based deletion")
	}
	targets := make(map[string]bool, len(resourceReferences))
	for _, reference := range resourceReferences {
		subscriptionID, created, err := parseSubscriptionReference(reference)
		if err != nil {
			return err
		}
		if previous, exists := targets[subscriptionID]; exists && previous != created {
			return fmt.Errorf("OVH cleanup received conflicting ownership references for subscription %q", subscriptionID)
		}
		targets[subscriptionID] = created
	}
	resources, err := backend.inventorySubscriptions(ctx)
	if err != nil {
		return errors.New("inventory OVH MKS log subscriptions for cleanup failed")
	}
	byID := make(map[string]OVHSubscription, len(resources))
	for _, subscription := range resources {
		byID[subscription.SubscriptionID] = subscription
	}
	for subscriptionID, created := range targets {
		subscription, present := byID[subscriptionID]
		if !present {
			backend.forgetCreatedSubscription(subscriptionID)
			continue
		}
		if !backend.owns(subscription) {
			return fmt.Errorf("OVH cleanup refused subscription %q because its identity no longer matches the configured service, cluster, and stream", subscriptionID)
		}
		if !created {
			// The subscription existed before this Apply call. OVH has no
			// ownership label that would make deleting it safe.
			continue
		}
		operation, err := backend.api.DeleteSubscription(ctx, backend.serviceName, backend.kubeID, subscription.SubscriptionID)
		if err != nil || strings.TrimSpace(operation.OperationID) == "" {
			return errors.New("delete OVH MKS log subscription failed")
		}
		if _, err := backend.waitForSubscriptionID(ctx, subscription.SubscriptionID, false); err != nil {
			return errors.New("wait for OVH MKS log subscription deletion failed")
		}
		backend.forgetCreatedSubscription(subscriptionID)
	}
	return nil
}

func (backend *OVHKubeAuditLogsBackend) Inventory(ctx context.Context, marker string) ([]providerobservability.InventoryResource, error) {
	if ctx == nil || strings.TrimSpace(marker) == "" {
		return nil, errors.New("OVH observability inventory requires context and ownership marker")
	}
	resources, err := backend.inventorySubscriptions(ctx)
	if err != nil {
		return nil, errors.New("inventory OVH MKS log subscriptions failed")
	}
	result := make([]providerobservability.InventoryResource, 0, len(resources))
	for _, subscription := range resources {
		if !backend.owns(subscription) {
			continue
		}
		owned := backend.isCreatedSubscription(subscription.SubscriptionID)
		ownershipMarker := ""
		if owned {
			ownershipMarker = marker
		}
		result = append(result, providerobservability.InventoryResource{
			Identity:        subscriptionReference(subscription.SubscriptionID, owned),
			OwnershipMarker: ownershipMarker,
			Owned:           owned,
			Live:            true,
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Identity < result[j].Identity })
	return result, nil
}

func (backend *OVHKubeAuditLogsBackend) ensureSubscription(ctx context.Context, resources []OVHSubscription, createdThisApply map[string]struct{}) (OVHSubscription, bool, error) {
	var matching *OVHSubscription
	for _, subscription := range resources {
		if backend.owns(subscription) {
			if matching != nil {
				return OVHSubscription{}, false, errors.New("OVH MKS API returned multiple subscriptions for the configured service, cluster, and stream")
			}
			candidate := subscription
			matching = &candidate
		}
	}
	if matching != nil {
		_, created := createdThisApply[matching.SubscriptionID]
		return *matching, created, nil
	}
	operation, err := backend.api.CreateSubscription(ctx, backend.serviceName, backend.kubeID, ovhKubeAuditKind, backend.streamID)
	if err != nil || strings.TrimSpace(operation.OperationID) == "" {
		return OVHSubscription{}, false, errors.New("create OVH MKS audit log subscription failed")
	}
	subscription, err := backend.waitForSubscription(ctx, true)
	if err != nil {
		return OVHSubscription{}, false, err
	}
	createdThisApply[subscription.SubscriptionID] = struct{}{}
	backend.recordCreatedSubscription(subscription.SubscriptionID)
	return subscription, true, nil
}

func (backend *OVHKubeAuditLogsBackend) findOwnedSubscription(ctx context.Context) (OVHSubscription, bool, error) {
	resources, err := backend.inventorySubscriptions(ctx)
	if err != nil {
		return OVHSubscription{}, false, err
	}
	for _, subscription := range resources {
		if backend.owns(subscription) {
			return subscription, true, nil
		}
	}
	return OVHSubscription{}, false, nil
}

func (backend *OVHKubeAuditLogsBackend) inventorySubscriptions(ctx context.Context) ([]OVHSubscription, error) {
	ids, err := backend.api.ListSubscriptions(ctx, backend.serviceName, backend.kubeID)
	if err != nil {
		return nil, err
	}
	result := make([]OVHSubscription, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if strings.TrimSpace(id) == "" || strings.ContainsAny(id, "\r\n\x00") {
			return nil, errors.New("OVH API returned an invalid log subscription identity")
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		subscription, err := backend.api.GetSubscription(ctx, backend.serviceName, backend.kubeID, id)
		if err != nil {
			return nil, err
		}
		if subscription.SubscriptionID == "" {
			return nil, errors.New("OVH API returned a log subscription without an identity")
		}
		if subscription.SubscriptionID != id {
			return nil, errors.New("OVH API returned a mismatched log subscription identity")
		}
		result = append(result, subscription)
	}
	return result, nil
}

func (backend *OVHKubeAuditLogsBackend) waitForSubscription(ctx context.Context, wantPresent bool) (OVHSubscription, error) {
	attempts := backend.pollAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		resources, err := backend.inventorySubscriptions(ctx)
		if err != nil {
			return OVHSubscription{}, err
		}
		owned := false
		for _, subscription := range resources {
			if backend.owns(subscription) {
				owned = true
				if wantPresent {
					return subscription, nil
				}
			}
		}
		if !wantPresent && !owned {
			return OVHSubscription{}, nil
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(backend.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return OVHSubscription{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if wantPresent {
		return OVHSubscription{}, errors.New("OVH MKS audit subscription did not become visible before the polling budget expired")
	}
	return OVHSubscription{}, errors.New("OVH MKS audit subscription remained visible after the polling budget expired")
}

func (backend *OVHKubeAuditLogsBackend) waitForSubscriptionID(ctx context.Context, subscriptionID string, wantPresent bool) (OVHSubscription, error) {
	attempts := backend.pollAttempts
	if attempts < 1 {
		attempts = 1
	}
	for attempt := 0; attempt < attempts; attempt++ {
		resources, err := backend.inventorySubscriptions(ctx)
		if err != nil {
			return OVHSubscription{}, err
		}
		for _, subscription := range resources {
			if subscription.SubscriptionID != subscriptionID {
				continue
			}
			if !backend.owns(subscription) {
				return OVHSubscription{}, errors.New("OVH MKS subscription identity changed while polling")
			}
			if wantPresent {
				return subscription, nil
			}
			break
		}
		if !wantPresent {
			return OVHSubscription{}, nil
		}
		if attempt+1 < attempts {
			timer := time.NewTimer(backend.pollInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return OVHSubscription{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if wantPresent {
		return OVHSubscription{}, errors.New("OVH MKS audit subscription did not become visible before the polling budget expired")
	}
	return OVHSubscription{}, errors.New("OVH MKS audit subscription remained visible after the polling budget expired")
}

func (backend *OVHKubeAuditLogsBackend) owns(subscription OVHSubscription) bool {
	return subscription.SubscriptionID != "" &&
		subscription.ServiceName == backend.serviceName &&
		subscription.Kind == ovhKubeAuditKind &&
		subscription.StreamID == backend.streamID
}

func (backend *OVHKubeAuditLogsBackend) recordCreatedSubscription(subscriptionID string) {
	backend.createdMu.Lock()
	defer backend.createdMu.Unlock()
	backend.createdSubscriptions[subscriptionID] = struct{}{}
}

func (backend *OVHKubeAuditLogsBackend) forgetCreatedSubscription(subscriptionID string) {
	backend.createdMu.Lock()
	defer backend.createdMu.Unlock()
	delete(backend.createdSubscriptions, subscriptionID)
}

func (backend *OVHKubeAuditLogsBackend) isCreatedSubscription(subscriptionID string) bool {
	backend.createdMu.RLock()
	defer backend.createdMu.RUnlock()
	_, ok := backend.createdSubscriptions[subscriptionID]
	return ok
}

func (backend *OVHKubeAuditLogsBackend) validatePlan(ctx context.Context, plan providerobservability.Plan) error {
	if ctx == nil {
		return errors.New("OVH observability context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("OVH observability ownership marker is required")
	}
	for _, binding := range plan.Bindings {
		if binding.Destination != ovhLogsDataPlatformDestination {
			return errors.New("OVH observability plan contains a binding for another destination")
		}
		if binding.Signal != "audit-events" && binding.Signal != "provider-operations" {
			return fmt.Errorf("OVH MKS audit forwarding does not implement signal %q", binding.Signal)
		}
		if binding.OwnershipMarker != plan.OwnershipMarker {
			return errors.New("OVH observability binding ownership marker does not match the plan")
		}
		if binding.RetentionDays > 0 {
			return errors.New("OVH MKS adapter cannot manage retention on an existing Logs Data Platform stream")
		}
		if strings.TrimSpace(binding.RedactionPolicy) != "" {
			return errors.New("OVH MKS adapter cannot manage redaction on an existing Logs Data Platform stream")
		}
	}
	return nil
}

func subscriptionPath(serviceName, kubeID string) string {
	return "/cloud/project/" + escapePath(serviceName) + "/kube/" + escapePath(kubeID) + "/log/subscription"
}

func escapePath(value string) string {
	return url.PathEscape(value)
}

func subscriptionReference(subscriptionID string, created bool) string {
	state := "existing"
	if created {
		state = "created"
	}
	return "ovh-mks-audit:subscription:" + state + ":" + subscriptionID
}

func parseSubscriptionReference(reference string) (string, bool, error) {
	const prefix = "ovh-mks-audit:subscription:"
	if !strings.HasPrefix(reference, prefix) {
		return "", false, fmt.Errorf("OVH cleanup received an unsupported subscription reference %q", reference)
	}
	value := strings.TrimPrefix(reference, prefix)
	state, subscriptionID, ok := strings.Cut(value, ":")
	if !ok || strings.TrimSpace(subscriptionID) == "" || strings.ContainsAny(subscriptionID, "\r\n\x00") {
		return "", false, fmt.Errorf("OVH cleanup received an invalid subscription reference %q", reference)
	}
	switch state {
	case "created":
		return subscriptionID, true, nil
	case "existing":
		return subscriptionID, false, nil
	default:
		return "", false, fmt.Errorf("OVH cleanup received an unsupported subscription reference state %q", state)
	}
}

func markerDigest(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(digest[:8])
}

func sortedKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
