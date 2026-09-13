package newrelic

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	provider "github.com/magelift/magelift/internal/provider"
	sdk "github.com/magelift/magelift/sdk/v1"
)

// NerdGraphOperations owns only the New Relic operational objects declared by
// the portable observability plan. It is deliberately separate from OTLP
// ingestion: a license key can publish data, while this adapter requires a
// user-key credential with NerdGraph permissions to manage alerting,
// dashboards, and service levels.
type NerdGraphOperations struct {
	httpClient    HTTPDoer
	resolver      provider.CredentialResolver
	endpoint      string
	accountID     int64
	credentialRef string
}

// NewNerdGraphOperations constructs the provider-local operational-object
// translator. The credential reference must resolve to a New Relic user key;
// the secret is held only while the provider API call is in flight.
func NewNerdGraphOperations(httpClient HTTPDoer, resolver provider.CredentialResolver, endpoint string, accountID int64, credentialRef string) (*NerdGraphOperations, error) {
	if httpClient == nil {
		return nil, errors.New("New Relic NerdGraph operations HTTP client is required")
	}
	if resolver == nil {
		return nil, errors.New("New Relic NerdGraph operations credential resolver is required")
	}
	if accountID <= 0 || accountID > (1<<31)-1 {
		return nil, errors.New("New Relic account ID must fit the NerdGraph account ID range")
	}
	if err := sdk.ValidateCredentialReference(credentialRef); err != nil {
		return nil, fmt.Errorf("New Relic NerdGraph operations credential reference: %w", err)
	}
	normalized, err := normalizeNerdGraphEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	return &NerdGraphOperations{httpClient: httpClient, resolver: resolver, endpoint: normalized, accountID: accountID, credentialRef: credentialRef}, nil
}

type newRelicPolicy struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	IncidentPreference string `json:"incidentPreference"`
}

type newRelicCondition struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
	PolicyID    string `json:"policyId"`
	RunbookURL  string `json:"runbookUrl"`
	Type        string `json:"type"`
	NRQL        struct {
		Query string `json:"query"`
	} `json:"nrql"`
	Signal struct {
		AggregationWindow int `json:"aggregationWindow"`
	} `json:"signal"`
	Terms []struct {
		Operator             string  `json:"operator"`
		Threshold            float64 `json:"threshold"`
		ThresholdDuration    int     `json:"thresholdDuration"`
		ThresholdOccurrences string  `json:"thresholdOccurrences"`
		Priority             string  `json:"priority"`
	} `json:"terms"`
}

type newRelicDashboard struct {
	GUID        string `json:"guid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Permissions string `json:"permissions"`
	Pages       []struct {
		GUID    string `json:"guid"`
		Name    string `json:"name"`
		Widgets []struct {
			ID            string `json:"id"`
			Title         string `json:"title"`
			Visualization struct {
				ID string `json:"id"`
			} `json:"visualization"`
			RawConfiguration map[string]any `json:"rawConfiguration"`
		} `json:"widgets"`
	} `json:"pages"`
}

type newRelicServiceLevel struct {
	ID          string `json:"id"`
	EntityGUID  string `json:"entityGuid"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Objectives  []struct {
		Target     float64 `json:"target"`
		TimeWindow struct {
			Rolling struct {
				Count int    `json:"count"`
				Unit  string `json:"unit"`
			} `json:"rolling"`
		} `json:"timeWindow"`
	} `json:"objectives"`
}

type newRelicOperationalState struct {
	Policies      []newRelicPolicy
	Conditions    []newRelicCondition
	Dashboards    []newRelicDashboard
	ServiceLevels []newRelicServiceLevel
}

func (client *NerdGraphOperations) Apply(ctx context.Context, plan providerobservability.Plan) ([]string, error) {
	if err := validateNerdGraphPlan(ctx, plan); err != nil {
		return nil, err
	}
	if len(plan.Alerts) == 0 && len(plan.Dashboards) == 0 && len(plan.SLOs) == 0 {
		return nil, nil
	}

	policyName := newRelicPolicyName(plan.OwnershipMarker)
	var refs []string
	var createdPolicy string
	var createdConditions []string
	var createdDashboards []string
	var createdServiceLevels []string
	rollback := func(cause error) ([]string, error) {
		cleanupErr := client.rollback(ctx, createdPolicy, createdConditions, createdDashboards, createdServiceLevels)
		if cleanupErr != nil {
			return nil, errors.Join(cause, fmt.Errorf("rollback New Relic operational objects: %w", cleanupErr))
		}
		return nil, cause
	}

	err := client.withCredential(ctx, func(apiKey []byte) error {
		if len(plan.Alerts) > 0 {
			policies, err := client.listPolicies(ctx, apiKey, policyName)
			if err != nil {
				return err
			}
			var policy newRelicPolicy
			switch len(policies) {
			case 0:
				policy, err = client.createPolicy(ctx, apiKey, policyName)
				if err != nil {
					return err
				}
				createdPolicy = policy.ID
			case 1:
				policy = policies[0]
				if policy.ID == "" || policy.Name != policyName {
					return errors.New("New Relic operational policy identity is not owned by MageLift")
				}
			default:
				return fmt.Errorf("New Relic operational policy name %q is not unique", policyName)
			}

			conditions, err := client.listPolicyConditions(ctx, apiKey, policy.ID)
			if err != nil {
				return err
			}
			for _, alert := range plan.Alerts {
				conditionName := newRelicConditionName(plan.OwnershipMarker, alert.ID)
				condition, found, err := findCondition(conditions, conditionName)
				if err != nil {
					return err
				}
				if found {
					if !newRelicConditionMatches(condition, policy.ID, plan.OwnershipMarker, alert) {
						return fmt.Errorf("New Relic alert %q has ownership or configuration drift", alert.ID)
					}
					refs = append(refs, newRelicConditionRef(condition.ID))
					continue
				}
				foreign, err := client.listNamedConditions(ctx, apiKey, conditionName)
				if err != nil {
					return err
				}
				for _, candidate := range foreign {
					if candidate.PolicyID != policy.ID {
						return fmt.Errorf("New Relic alert %q collides with a condition in another policy", alert.ID)
					}
				}
				created, err := client.createCondition(ctx, apiKey, policy.ID, plan.OwnershipMarker, alert)
				if err != nil {
					return err
				}
				if created.ID != "" {
					createdConditions = append(createdConditions, created.ID)
				}
				if created.ID == "" || !newRelicConditionMatches(created, policy.ID, plan.OwnershipMarker, alert) {
					return fmt.Errorf("New Relic alert %q create did not return the owned expected condition", alert.ID)
				}
				refs = append(refs, newRelicConditionRef(created.ID))
				conditions = append(conditions, created)
			}
		}

		for _, dashboard := range plan.Dashboards {
			name := newRelicDashboardName(plan.OwnershipMarker, dashboard.ID)
			found, err := client.findDashboard(ctx, apiKey, name)
			if err != nil {
				return err
			}
			if found != nil {
				if !newRelicDashboardMatches(*found, plan.OwnershipMarker, dashboard) {
					return fmt.Errorf("New Relic dashboard %q has ownership or configuration drift", dashboard.ID)
				}
				refs = append(refs, newRelicDashboardRef(found.GUID))
				continue
			}
			created, err := client.createDashboard(ctx, apiKey, plan.OwnershipMarker, dashboard)
			if err != nil {
				return err
			}
			if created.GUID != "" {
				createdDashboards = append(createdDashboards, created.GUID)
			}
			if created.GUID == "" || !newRelicDashboardMatches(created, plan.OwnershipMarker, dashboard) {
				return fmt.Errorf("New Relic dashboard %q create did not return the owned expected dashboard", dashboard.ID)
			}
			refs = append(refs, newRelicDashboardRef(created.GUID))
		}

		if len(plan.SLOs) > 0 {
			levels, err := client.listServiceLevels(ctx, apiKey, plan.NativeReference)
			if err != nil {
				return err
			}
			for _, slo := range plan.SLOs {
				level, found, err := findServiceLevel(levels, newRelicSLOName(plan.OwnershipMarker, slo.ID))
				if err != nil {
					return err
				}
				if found {
					if !newRelicServiceLevelMatches(level, plan.NativeReference, plan.OwnershipMarker, slo) {
						return fmt.Errorf("New Relic SLO %q has ownership or configuration drift", slo.ID)
					}
					refs = append(refs, newRelicServiceLevelRef(serviceLevelGUID(client.accountID, level.ID)))
					continue
				}
				created, err := client.createServiceLevel(ctx, apiKey, plan.NativeReference, plan.OwnershipMarker, slo)
				if err != nil {
					return err
				}
				if created.ID != "" {
					createdServiceLevels = append(createdServiceLevels, serviceLevelGUID(client.accountID, created.ID))
				}
				if created.ID == "" || !newRelicServiceLevelMatches(created, plan.NativeReference, plan.OwnershipMarker, slo) {
					return fmt.Errorf("New Relic SLO %q create did not return the owned expected service level", slo.ID)
				}
				refs = append(refs, newRelicServiceLevelRef(serviceLevelGUID(client.accountID, created.ID)))
			}
		}
		return nil
	})
	if err != nil {
		return rollback(err)
	}
	sort.Strings(refs)
	return refs, nil
}

func (client *NerdGraphOperations) VerifyOperations(ctx context.Context, plan providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	if err := validateNerdGraphPlan(ctx, plan); err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	if len(plan.Alerts) == 0 && len(plan.Dashboards) == 0 && len(plan.SLOs) == 0 {
		return providerobservability.OperationalObservation{AlertsVerified: true, DashboardsVerified: true, SLOsVerified: true}, nil
	}
	state, err := client.inventoryState(ctx, plan)
	if err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	observation := providerobservability.OperationalObservation{AlertsVerified: true, DashboardsVerified: true, SLOsVerified: true}
	policyID := ""
	if len(plan.Alerts) > 0 {
		policies, err := client.listPoliciesWithCredential(ctx, newRelicPolicyName(plan.OwnershipMarker))
		if err != nil {
			return providerobservability.OperationalObservation{}, err
		}
		if len(policies) != 1 {
			observation.AlertsVerified = false
		} else {
			policyID = policies[0].ID
			for _, alert := range plan.Alerts {
				condition, found, findErr := findCondition(state.Conditions, newRelicConditionName(plan.OwnershipMarker, alert.ID))
				if findErr != nil || !found || !newRelicConditionMatches(condition, policyID, plan.OwnershipMarker, alert) {
					observation.AlertsVerified = false
				}
			}
		}
	}
	for _, dashboard := range plan.Dashboards {
		found := false
		for _, candidate := range state.Dashboards {
			if candidate.Name == newRelicDashboardName(plan.OwnershipMarker, dashboard.ID) {
				found = newRelicDashboardMatches(candidate, plan.OwnershipMarker, dashboard)
				if !found {
					observation.Reason = newRelicDashboardMismatchReason(candidate, plan.OwnershipMarker, dashboard)
				}
				break
			}
		}
		if !found {
			observation.DashboardsVerified = false
		}
	}
	for _, slo := range plan.SLOs {
		level, found, findErr := findServiceLevel(state.ServiceLevels, newRelicSLOName(plan.OwnershipMarker, slo.ID))
		if findErr != nil || !found || !newRelicServiceLevelMatches(level, plan.NativeReference, plan.OwnershipMarker, slo) {
			observation.SLOsVerified = false
		}
	}
	if !observation.AlertsVerified || !observation.DashboardsVerified || !observation.SLOsVerified {
		if observation.Reason == "" {
			observation.Reason = fmt.Sprintf("New Relic operational object ownership or configuration could not be verified (policies=%d conditions=%d dashboards=%d serviceLevels=%d)", len(state.Policies), len(state.Conditions), len(state.Dashboards), len(state.ServiceLevels))
		}
	}
	return observation, nil
}

func (client *NerdGraphOperations) Destroy(ctx context.Context, plan providerobservability.Plan) error {
	if err := validateNerdGraphPlan(ctx, plan); err != nil {
		return err
	}
	return client.withCredential(ctx, func(apiKey []byte) error {
		if len(plan.Alerts) > 0 {
			policyName := newRelicPolicyName(plan.OwnershipMarker)
			policies, err := client.listPolicies(ctx, apiKey, policyName)
			if err != nil {
				return err
			}
			for _, policy := range policies {
				if policy.Name != policyName || policy.ID == "" {
					return errors.New("New Relic operational cleanup encountered an unsafe policy identity")
				}
				conditions, err := client.listPolicyConditions(ctx, apiKey, policy.ID)
				if err != nil {
					return err
				}
				ownedConditions := make([]newRelicCondition, 0, len(conditions))
				for _, condition := range conditions {
					if !strings.HasPrefix(condition.Name, "MageLift Alert "+markerDigest(plan.OwnershipMarker)+" ") {
						return errors.New("New Relic operational cleanup encountered a foreign condition in the owned policy")
					}
					if !strings.Contains(condition.Description, "magelift-ownership="+plan.OwnershipMarker) || condition.ID == "" {
						return errors.New("New Relic operational cleanup encountered an ownership mismatch")
					}
					ownedConditions = append(ownedConditions, condition)
				}
				for _, condition := range ownedConditions {
					if err := client.deleteCondition(ctx, apiKey, condition.ID); err != nil {
						return err
					}
				}
				if err := client.deletePolicy(ctx, apiKey, policy.ID); err != nil {
					return err
				}
			}
		}

		for _, dashboard := range plan.Dashboards {
			found, err := client.findDashboardEventually(ctx, apiKey, newRelicDashboardName(plan.OwnershipMarker, dashboard.ID))
			if err != nil {
				return err
			}
			if found == nil {
				continue
			}
			if !newRelicDashboardMatches(*found, plan.OwnershipMarker, dashboard) {
				return fmt.Errorf("New Relic dashboard %q has an ownership or configuration mismatch during cleanup", dashboard.ID)
			}
			if err := client.deleteDashboard(ctx, apiKey, found.GUID); err != nil {
				return err
			}
		}

		if len(plan.SLOs) > 0 {
			levels, err := client.listServiceLevels(ctx, apiKey, plan.NativeReference)
			if err != nil {
				return err
			}
			for _, slo := range plan.SLOs {
				level, found, err := findServiceLevel(levels, newRelicSLOName(plan.OwnershipMarker, slo.ID))
				if err != nil {
					return err
				}
				if !found {
					continue
				}
				if !newRelicServiceLevelMatches(level, plan.NativeReference, plan.OwnershipMarker, slo) {
					return fmt.Errorf("New Relic SLO %q has an ownership or configuration mismatch during cleanup", slo.ID)
				}
				if err := client.deleteServiceLevel(ctx, apiKey, serviceLevelGUID(client.accountID, level.ID)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (client *NerdGraphOperations) Inventory(ctx context.Context, marker string) ([]providerobservability.InventoryResource, error) {
	if err := validateOperationContext(ctx, marker); err != nil {
		return nil, err
	}
	return withCredentialResult(ctx, client, func(apiKey []byte) ([]providerobservability.InventoryResource, error) {
		resources := make([]providerobservability.InventoryResource, 0)
		policies, err := client.listPolicies(ctx, apiKey, newRelicPolicyName(marker))
		if err != nil {
			return nil, err
		}
		for _, policy := range policies {
			if policy.Name != newRelicPolicyName(marker) || policy.ID == "" {
				return nil, errors.New("New Relic operational inventory encountered an unsafe policy identity")
			}
			resources = append(resources, providerobservability.InventoryResource{Identity: "newrelic:alert-policy:" + policy.ID, OwnershipMarker: marker, Owned: true, Live: true})
			conditions, err := client.listPolicyConditions(ctx, apiKey, policy.ID)
			if err != nil {
				return nil, err
			}
			for _, condition := range conditions {
				if condition.ID == "" || !strings.HasPrefix(condition.Name, "MageLift Alert "+markerDigest(marker)+" ") || !strings.Contains(condition.Description, "magelift-ownership="+marker) {
					return nil, errors.New("New Relic operational inventory encountered an unsafe alert condition identity")
				}
				resources = append(resources, providerobservability.InventoryResource{Identity: newRelicConditionRef(condition.ID), OwnershipMarker: marker, Owned: true, Live: true})
			}
		}
		dashboards, err := client.findDashboards(ctx, apiKey, marker)
		if err != nil {
			return nil, err
		}
		for _, dashboard := range dashboards {
			if !newRelicDashboardOwned(dashboard, marker) {
				return nil, errors.New("New Relic operational inventory encountered an unsafe dashboard identity")
			}
			resources = append(resources, providerobservability.InventoryResource{Identity: newRelicDashboardRef(dashboard.GUID), OwnershipMarker: marker, Owned: true, Live: true})
		}
		sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
		return resources, nil
	})
}

func (client *NerdGraphOperations) InventoryForPlan(ctx context.Context, plan providerobservability.Plan) ([]providerobservability.InventoryResource, error) {
	if err := validateNerdGraphPlan(ctx, plan); err != nil {
		return nil, err
	}
	resources, err := client.Inventory(ctx, plan.OwnershipMarker)
	if err != nil {
		return nil, err
	}
	if len(plan.SLOs) == 0 {
		return resources, nil
	}
	return withCredentialResult(ctx, client, func(apiKey []byte) ([]providerobservability.InventoryResource, error) {
		levels, err := client.listServiceLevels(ctx, apiKey, plan.NativeReference)
		if err != nil {
			return nil, err
		}
		for _, slo := range plan.SLOs {
			level, found, err := findServiceLevel(levels, newRelicSLOName(plan.OwnershipMarker, slo.ID))
			if err != nil {
				return nil, err
			}
			if found {
				resources = append(resources, providerobservability.InventoryResource{Identity: newRelicServiceLevelRef(serviceLevelGUID(client.accountID, level.ID)), OwnershipMarker: plan.OwnershipMarker, Owned: newRelicServiceLevelMatches(level, plan.NativeReference, plan.OwnershipMarker, slo), Live: true})
			}
		}
		sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
		return resources, nil
	})
}

func (client *NerdGraphOperations) inventoryState(ctx context.Context, plan providerobservability.Plan) (newRelicOperationalState, error) {
	if err := validateNerdGraphPlan(ctx, plan); err != nil {
		return newRelicOperationalState{}, err
	}
	return withCredentialResult(ctx, client, func(apiKey []byte) (newRelicOperationalState, error) {
		state := newRelicOperationalState{}
		if len(plan.Alerts) > 0 {
			policies, err := client.listPolicies(ctx, apiKey, newRelicPolicyName(plan.OwnershipMarker))
			if err != nil {
				return state, err
			}
			state.Policies = policies
			for _, policy := range policies {
				conditions, err := client.listPolicyConditions(ctx, apiKey, policy.ID)
				if err != nil {
					return state, err
				}
				state.Conditions = append(state.Conditions, conditions...)
			}
		}
		if len(plan.Dashboards) > 0 {
			for _, dashboard := range plan.Dashboards {
				found, err := client.findDashboardEventually(ctx, apiKey, newRelicDashboardName(plan.OwnershipMarker, dashboard.ID))
				if err != nil {
					return state, err
				}
				if found != nil {
					state.Dashboards = append(state.Dashboards, *found)
				}
			}
		}
		if len(plan.SLOs) > 0 {
			levels, err := client.listServiceLevels(ctx, apiKey, plan.NativeReference)
			if err != nil {
				return state, err
			}
			state.ServiceLevels = levels
		}
		return state, nil
	})
}

func (client *NerdGraphOperations) withCredential(ctx context.Context, fn func([]byte) error) error {
	if client == nil || client.resolver == nil || client.httpClient == nil {
		return errors.New("New Relic NerdGraph operations client is required")
	}
	var operationErr error
	err := provider.UseCredential(ctx, client.resolver, client.credentialRef, func(apiKey []byte) error {
		operationErr = fn(apiKey)
		return operationErr
	})
	if operationErr != nil {
		// The provider credential boundary intentionally redacts consumer
		// errors. This adapter's consumer errors are already normalized and
		// contain no credential material, so preserve them for actionable
		// operation diagnostics while keeping resolver failures redacted.
		return operationErr
	}
	return err
}

func withCredentialResult[T any](ctx context.Context, client *NerdGraphOperations, fn func([]byte) (T, error)) (T, error) {
	var result T
	err := client.withCredential(ctx, func(apiKey []byte) error {
		var err error
		result, err = fn(apiKey)
		return err
	})
	return result, err
}

func validateOperationContext(ctx context.Context, marker string) error {
	if ctx == nil {
		return errors.New("New Relic operational context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return validateOwnershipMarker(marker)
}

func validateNerdGraphPlan(ctx context.Context, plan providerobservability.Plan) error {
	if err := validateOperationContext(ctx, plan.OwnershipMarker); err != nil {
		return err
	}
	if err := validateNewRelicTarget(plan.TargetProvider, plan.TargetRuntime); err != nil {
		return err
	}
	if err := sdk.ValidateObservabilityIntent(sdk.ObservabilityIntent{Alerts: plan.Alerts, Dashboards: plan.Dashboards, SLOs: plan.SLOs}); err != nil {
		return fmt.Errorf("validate New Relic operational intent: %w", err)
	}
	for _, alert := range plan.Alerts {
		if strings.TrimSpace(alert.MaintenancePolicyRef) != "" {
			return fmt.Errorf("New Relic alert %q does not support maintenance policy references", alert.ID)
		}
		if alert.Severity == "info" {
			return fmt.Errorf("New Relic alert %q does not support info severity", alert.ID)
		}
		if alert.Operator != "gt" && alert.Operator != "lt" {
			return fmt.Errorf("New Relic alert %q supports only gt and lt operators", alert.ID)
		}
		if alert.WindowSeconds < 30 || alert.WindowSeconds > 120*60 || alert.WindowSeconds%15 != 0 {
			return fmt.Errorf("New Relic alert %q window must be 30 seconds to 120 minutes in 15-second increments", alert.ID)
		}
		if err := validateSingleLine("New Relic alert owner", alert.Owner); err != nil {
			return err
		}
		if err := validateSingleLine("New Relic alert deduplication key", alert.DeduplicationKey); err != nil {
			return err
		}
		if _, err := nrqlEventTypeForOperationalSignal(alert.Signal); err != nil {
			return fmt.Errorf("New Relic alert %q: %w", alert.ID, err)
		}
	}
	for _, dashboard := range plan.Dashboards {
		if err := validateSingleLine("New Relic dashboard owner", dashboard.Owner); err != nil {
			return err
		}
		for _, signal := range dashboard.Signals {
			if _, err := nrqlEventTypeForOperationalSignal(signal); err != nil {
				return fmt.Errorf("New Relic dashboard %q: %w", dashboard.ID, err)
			}
		}
	}
	if len(plan.SLOs) > 0 {
		if strings.TrimSpace(plan.NativeReference) == "" || strings.ContainsAny(plan.NativeReference, "\r\n\x00") {
			return errors.New("New Relic SLO lifecycle requires the target service entity GUID in the native reference")
		}
		for _, slo := range plan.SLOs {
			if slo.Signal != "application-health" {
				return fmt.Errorf("New Relic SLO %q supports only application-health", slo.ID)
			}
			if slo.WindowSeconds%(24*60*60) != 0 {
				return fmt.Errorf("New Relic SLO %q rolling window must be a whole number of days", slo.ID)
			}
			days := slo.WindowSeconds / (24 * 60 * 60)
			if days != 1 && days != 7 && days != 14 && days != 28 && days != 30 {
				return fmt.Errorf("New Relic SLO %q rolling window must be 1, 7, 14, 28, or 30 days", slo.ID)
			}
			if err := validateSingleLine("New Relic SLO owner", slo.Owner); err != nil {
				return err
			}
			if err := validateSingleLine("New Relic SLO error-budget policy", slo.ErrorBudgetPolicy); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSingleLine(name, value string) error {
	if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%s must be non-empty and single-line", name)
	}
	return nil
}

func nrqlEventTypeForOperationalSignal(signal string) (string, error) {
	switch signal {
	case "logs":
		return "Log", nil
	case "metrics":
		return "Metric", nil
	case "traces", "application-health":
		return "Span", nil
	default:
		return "", fmt.Errorf("signal %q is not supported by the New Relic NerdGraph translator", signal)
	}
}

func newRelicPolicyName(marker string) string {
	return "MageLift Observability " + markerDigest(marker)
}

func newRelicConditionName(marker, id string) string {
	return "MageLift Alert " + markerDigest(marker) + " " + markerDigest(id)
}

func newRelicDashboardName(marker, id string) string {
	return "MageLift Dashboard " + markerDigest(marker) + " " + markerDigest(id)
}

func newRelicSLOName(marker, id string) string {
	return "MageLift SLO " + markerDigest(marker) + " " + markerDigest(id)
}

func newRelicConditionRef(id string) string      { return "newrelic:alert-condition:" + id }
func newRelicDashboardRef(guid string) string    { return "newrelic:dashboard:" + guid }
func newRelicServiceLevelRef(guid string) string { return "newrelic:service-level:" + guid }

func operationalDescription(kind, marker, id, owner, runbook, extra string) string {
	return fmt.Sprintf("magelift-ownership=%s; kind=%s; id=%s; owner=%s; runbook=%s; %s", marker, kind, id, owner, runbook, extra)
}

func newRelicAlertDescription(marker string, alert sdk.AlertIntent) string {
	return operationalDescription("alert", marker, alert.ID, alert.Owner, alert.RunbookURL, "deduplication="+alert.DeduplicationKey)
}

func newRelicDashboardDescription(marker string, dashboard sdk.DashboardIntent) string {
	return operationalDescription("dashboard", marker, dashboard.ID, dashboard.Owner, "", "signals="+strings.Join(sdk.SortedStrings(dashboard.Signals), ","))
}

func newRelicSLODescription(marker string, slo sdk.SLOIntent) string {
	return operationalDescription("slo", marker, slo.ID, slo.Owner, slo.RunbookURL, "error-budget-policy="+slo.ErrorBudgetPolicy)
}

func newRelicConditionMatches(condition newRelicCondition, policyID, marker string, alert sdk.AlertIntent) bool {
	eventType, err := nrqlEventTypeForOperationalSignal(alert.Signal)
	if err != nil {
		return false
	}
	query, err := buildOperationalNRQL(eventType, marker)
	if err != nil {
		return false
	}
	wantOperator := "ABOVE"
	if alert.Operator == "lt" {
		wantOperator = "BELOW"
	}
	wantPriority := "CRITICAL"
	if alert.Severity == "warning" {
		wantPriority = "WARNING"
	}
	return condition.ID != "" && condition.Name == newRelicConditionName(marker, alert.ID) && condition.Description == newRelicAlertDescription(marker, alert) && condition.Enabled && condition.PolicyID == policyID && condition.RunbookURL == alert.RunbookURL && condition.Type == "STATIC" && condition.NRQL.Query == query && condition.Signal.AggregationWindow == int(alert.WindowSeconds) && len(condition.Terms) == 1 && condition.Terms[0].Operator == wantOperator && condition.Terms[0].Threshold == alert.Threshold && condition.Terms[0].ThresholdDuration == int(alert.WindowSeconds) && condition.Terms[0].ThresholdOccurrences == "AT_LEAST_ONCE" && condition.Terms[0].Priority == wantPriority
}

func newRelicDashboardMatches(dashboard newRelicDashboard, marker string, intent sdk.DashboardIntent) bool {
	if dashboard.GUID == "" || dashboard.Name != newRelicDashboardName(marker, intent.ID) || dashboard.Description != newRelicDashboardDescription(marker, intent) || dashboard.Permissions != "PRIVATE" || len(dashboard.Pages) != 1 || dashboard.Pages[0].Name != "Signals" || len(dashboard.Pages[0].Widgets) != len(intent.Signals) {
		return false
	}
	expectedSignals := sdk.SortedStrings(intent.Signals)
	for index, signal := range expectedSignals {
		widget := dashboard.Pages[0].Widgets[index]
		eventType, err := nrqlEventTypeForOperationalSignal(signal)
		if err != nil || widget.Title != signal || widget.Visualization.ID != "viz.billboard" {
			return false
		}
		query, err := buildOperationalNRQL(eventType, marker)
		if err != nil || dashboardWidgetQuery(widget.RawConfiguration) != query {
			return false
		}
	}
	return true
}

func newRelicDashboardOwned(dashboard newRelicDashboard, marker string) bool {
	return dashboard.GUID != "" && strings.HasPrefix(dashboard.Description, "magelift-ownership="+marker+";")
}

func newRelicDashboardMismatchReason(dashboard newRelicDashboard, marker string, intent sdk.DashboardIntent) string {
	if dashboard.GUID == "" {
		return "New Relic dashboard verification failed: provider returned no dashboard GUID"
	}
	if dashboard.Name != newRelicDashboardName(marker, intent.ID) {
		return "New Relic dashboard verification failed: name drift"
	}
	if dashboard.Description != newRelicDashboardDescription(marker, intent) {
		return "New Relic dashboard verification failed: ownership description drift"
	}
	if dashboard.Permissions != "PRIVATE" {
		return "New Relic dashboard verification failed: permissions drift"
	}
	if len(dashboard.Pages) != 1 || dashboard.Pages[0].Name != "Signals" {
		return "New Relic dashboard verification failed: page topology drift"
	}
	if len(dashboard.Pages[0].Widgets) != len(intent.Signals) {
		return "New Relic dashboard verification failed: widget count drift"
	}
	for index, signal := range sdk.SortedStrings(intent.Signals) {
		widget := dashboard.Pages[0].Widgets[index]
		if widget.Title != signal || widget.Visualization.ID != "viz.billboard" {
			return "New Relic dashboard verification failed: widget metadata drift"
		}
		query, err := buildOperationalNRQL(mustNRQLEventType(signal), marker)
		if err != nil || dashboardWidgetQuery(widget.RawConfiguration) != query {
			return "New Relic dashboard verification failed: widget query drift"
		}
	}
	return "New Relic dashboard verification failed: unknown configuration drift"
}

func mustNRQLEventType(signal string) string {
	eventType, _ := nrqlEventTypeForOperationalSignal(signal)
	return eventType
}

func newRelicServiceLevelMatches(level newRelicServiceLevel, entityGUID, marker string, intent sdk.SLOIntent) bool {
	days := int(intent.WindowSeconds / (24 * 60 * 60))
	return level.ID != "" && level.EntityGUID == entityGUID && level.Name == newRelicSLOName(marker, intent.ID) && level.Description == newRelicSLODescription(marker, intent) && len(level.Objectives) == 1 && math.Abs(level.Objectives[0].Target-intent.Target*100) < 0.000001 && level.Objectives[0].TimeWindow.Rolling.Count == days && level.Objectives[0].TimeWindow.Rolling.Unit == "DAY"
}

func buildOperationalNRQL(eventType, marker string) (string, error) {
	quoted, err := quoteNRQLString(marker)
	if err != nil {
		return "", err
	}
	return "SELECT count(*) FROM " + eventType + " WHERE `magelift.ownership_marker` = " + quoted, nil
}

func dashboardWidgetQuery(configuration map[string]any) string {
	queries, ok := configuration["nrqlQueries"].([]any)
	if !ok || len(queries) != 1 {
		return ""
	}
	query, ok := queries[0].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := query["query"].(string)
	return value
}

func findCondition(conditions []newRelicCondition, name string) (newRelicCondition, bool, error) {
	var found newRelicCondition
	count := 0
	for _, condition := range conditions {
		if condition.Name == name {
			found = condition
			count++
		}
	}
	if count > 1 {
		return newRelicCondition{}, false, fmt.Errorf("New Relic condition name %q is not unique", name)
	}
	return found, count == 1, nil
}

func findServiceLevel(levels []newRelicServiceLevel, name string) (newRelicServiceLevel, bool, error) {
	var found newRelicServiceLevel
	count := 0
	for _, level := range levels {
		if level.Name == name {
			found = level
			count++
		}
	}
	if count > 1 {
		return newRelicServiceLevel{}, false, fmt.Errorf("New Relic service-level name %q is not unique", name)
	}
	return found, count == 1, nil
}

func serviceLevelGUID(accountID int64, id string) string {
	value := fmt.Sprintf("%d|EXT|SERVICE_LEVEL|%s", accountID, id)
	return base64.RawStdEncoding.EncodeToString([]byte(value))
}

func (client *NerdGraphOperations) rollback(ctx context.Context, policyID string, conditionIDs, dashboardGUIDs, serviceLevelGUIDs []string) error {
	if policyID == "" && len(conditionIDs) == 0 && len(dashboardGUIDs) == 0 && len(serviceLevelGUIDs) == 0 {
		return nil
	}
	return client.withCredential(ctx, func(apiKey []byte) error {
		var rollbackErrs []error
		// Delete child objects before their parent policy. The exact IDs were
		// recorded immediately after each successful create so a malformed
		// post-create response cannot leave a newly-created object behind.
		for _, guid := range serviceLevelGUIDs {
			if err := client.deleteServiceLevel(ctx, apiKey, guid); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
		for _, guid := range dashboardGUIDs {
			if err := client.deleteDashboard(ctx, apiKey, guid); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
		for _, conditionID := range conditionIDs {
			if err := client.deleteCondition(ctx, apiKey, conditionID); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
		if policyID != "" {
			if err := client.deletePolicy(ctx, apiKey, policyID); err != nil {
				rollbackErrs = append(rollbackErrs, err)
			}
		}
		return errors.Join(rollbackErrs...)
	})
}

func (client *NerdGraphOperations) listPoliciesWithCredential(ctx context.Context, name string) ([]newRelicPolicy, error) {
	return withCredentialResult(ctx, client, func(apiKey []byte) ([]newRelicPolicy, error) { return client.listPolicies(ctx, apiKey, name) })
}

func (client *NerdGraphOperations) listPolicies(ctx context.Context, apiKey []byte, name string) ([]newRelicPolicy, error) {
	query := `query($accountId: Int!, $cursor: String, $searchCriteria: AlertsPoliciesSearchCriteriaInput) { actor { account(id: $accountId) { alerts { policiesSearch(cursor: $cursor, searchCriteria: $searchCriteria) { nextCursor policies { id name incidentPreference } } } } } }`
	policies := make([]newRelicPolicy, 0)
	seenCursors := make(map[string]struct{})
	var cursor *string
	for {
		var response struct {
			Actor struct {
				Account struct {
					Alerts struct {
						PoliciesSearch struct {
							NextCursor *string          `json:"nextCursor"`
							Policies   []newRelicPolicy `json:"policies"`
						} `json:"policiesSearch"`
					} `json:"alerts"`
				} `json:"account"`
			} `json:"actor"`
		}
		if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, map[string]any{"accountId": client.accountID, "cursor": cursor, "searchCriteria": map[string]any{"name": name}}, &response); err != nil {
			return nil, err
		}
		policies = append(policies, response.Actor.Account.Alerts.PoliciesSearch.Policies...)
		if response.Actor.Account.Alerts.PoliciesSearch.NextCursor == nil || *response.Actor.Account.Alerts.PoliciesSearch.NextCursor == "" {
			return policies, nil
		}
		next := *response.Actor.Account.Alerts.PoliciesSearch.NextCursor
		if _, exists := seenCursors[next]; exists {
			return nil, errors.New("New Relic alert policy pagination repeated a cursor")
		}
		seenCursors[next] = struct{}{}
		cursor = &next
	}
}

func (client *NerdGraphOperations) createPolicy(ctx context.Context, apiKey []byte, name string) (newRelicPolicy, error) {
	query := `mutation { alertsPolicyCreate(accountId: ` + strconv.FormatInt(client.accountID, 10) + `, policy: { name: ` + graphQLString(name) + `, incidentPreference: PER_CONDITION }) { id name incidentPreference } }`
	var response struct {
		Policy newRelicPolicy `json:"alertsPolicyCreate"`
	}
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, nil, &response); err != nil && response.Policy.ID == "" {
		return newRelicPolicy{}, errors.New("create New Relic alert policy failed")
	}
	if response.Policy.ID == "" || response.Policy.Name != name {
		return newRelicPolicy{}, errors.New("New Relic alert policy create returned an invalid identity")
	}
	return response.Policy, nil
}

func (client *NerdGraphOperations) deletePolicy(ctx context.Context, apiKey []byte, id string) error {
	if err := validateProviderIdentity("New Relic alert policy ID", id); err != nil {
		return err
	}
	query := `mutation { alertsPolicyDelete(accountId: ` + strconv.FormatInt(client.accountID, 10) + `, id: ` + graphQLString(id) + `) { id } }`
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, nil, nil); err != nil {
		return errors.New("delete New Relic alert policy failed")
	}
	return nil
}

func (client *NerdGraphOperations) listPolicyConditions(ctx context.Context, apiKey []byte, policyID string) ([]newRelicCondition, error) {
	return client.listConditions(ctx, apiKey, map[string]any{"policyId": policyID})
}

func (client *NerdGraphOperations) listNamedConditions(ctx context.Context, apiKey []byte, name string) ([]newRelicCondition, error) {
	return client.listConditions(ctx, apiKey, map[string]any{"name": name})
}

func (client *NerdGraphOperations) listConditions(ctx context.Context, apiKey []byte, searchCriteria map[string]any) ([]newRelicCondition, error) {
	query := `query($accountId: Int!, $searchCriteria: AlertsNrqlConditionsSearchCriteriaInput, $cursor: String) { actor { account(id: $accountId) { alerts { nrqlConditionsSearch(searchCriteria: $searchCriteria, cursor: $cursor) { nextCursor nrqlConditions { id name description enabled policyId runbookUrl type nrql { query } signal { aggregationWindow } terms { operator threshold thresholdDuration thresholdOccurrences priority } } } } } } }`
	conditions := make([]newRelicCondition, 0)
	seenCursors := make(map[string]struct{})
	var cursor *string
	for {
		variables := map[string]any{"accountId": client.accountID, "searchCriteria": searchCriteria, "cursor": cursor}
		var response struct {
			Actor struct {
				Account struct {
					Alerts struct {
						Search struct {
							NextCursor *string             `json:"nextCursor"`
							Conditions []newRelicCondition `json:"nrqlConditions"`
						} `json:"nrqlConditionsSearch"`
					} `json:"alerts"`
				} `json:"account"`
			} `json:"actor"`
		}
		if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, variables, &response); err != nil {
			return nil, errors.New("list New Relic alert conditions failed")
		}
		conditions = append(conditions, response.Actor.Account.Alerts.Search.Conditions...)
		if response.Actor.Account.Alerts.Search.NextCursor == nil || *response.Actor.Account.Alerts.Search.NextCursor == "" {
			return conditions, nil
		}
		next := *response.Actor.Account.Alerts.Search.NextCursor
		if _, exists := seenCursors[next]; exists {
			return nil, errors.New("New Relic alert condition pagination repeated a cursor")
		}
		seenCursors[next] = struct{}{}
		cursor = &next
	}
}

func (client *NerdGraphOperations) createCondition(ctx context.Context, apiKey []byte, policyID, marker string, alert sdk.AlertIntent) (newRelicCondition, error) {
	eventType, err := nrqlEventTypeForOperationalSignal(alert.Signal)
	if err != nil {
		return newRelicCondition{}, err
	}
	query, err := buildOperationalNRQL(eventType, marker)
	if err != nil {
		return newRelicCondition{}, err
	}
	operator := "ABOVE"
	if alert.Operator == "lt" {
		operator = "BELOW"
	}
	priority := "CRITICAL"
	if alert.Severity == "warning" {
		priority = "WARNING"
	}
	mutation := `mutation { alertsNrqlConditionStaticCreate(accountId: ` + strconv.FormatInt(client.accountID, 10) + `, policyId: ` + graphQLString(policyID) + `, condition: { name: ` + graphQLString(newRelicConditionName(marker, alert.ID)) + `, description: ` + graphQLString(newRelicAlertDescription(marker, alert)) + `, enabled: true, nrql: { query: ` + graphQLString(query) + ` }, signal: { aggregationWindow: ` + strconv.FormatInt(alert.WindowSeconds, 10) + `, aggregationMethod: EVENT_FLOW, aggregationDelay: 120 }, terms: { threshold: ` + strconv.FormatFloat(alert.Threshold, 'f', -1, 64) + `, thresholdOccurrences: AT_LEAST_ONCE, thresholdDuration: ` + strconv.FormatInt(alert.WindowSeconds, 10) + `, operator: ` + operator + `, priority: ` + priority + ` }, valueFunction: SINGLE_VALUE, runbookUrl: ` + graphQLString(alert.RunbookURL) + `, violationTimeLimitSeconds: 86400 }) { id name description enabled policyId runbookUrl type nrql { query } signal { aggregationWindow } terms { operator threshold thresholdDuration thresholdOccurrences priority } } }`
	var response struct {
		Condition newRelicCondition `json:"alertsNrqlConditionStaticCreate"`
	}
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, mutation, nil, &response); err != nil && response.Condition.ID == "" {
		return newRelicCondition{}, errors.New("create New Relic NRQL condition failed")
	}
	return response.Condition, nil
}

func (client *NerdGraphOperations) deleteCondition(ctx context.Context, apiKey []byte, id string) error {
	if err := validateProviderIdentity("New Relic alert condition ID", id); err != nil {
		return err
	}
	query := `mutation { alertsConditionDelete(accountId: ` + strconv.FormatInt(client.accountID, 10) + `, id: ` + graphQLString(id) + `) { id } }`
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, nil, nil); err != nil {
		return errors.New("delete New Relic NRQL condition failed")
	}
	return nil
}

func (client *NerdGraphOperations) findDashboard(ctx context.Context, apiKey []byte, name string) (*newRelicDashboard, error) {
	if err := validateSingleLine("New Relic dashboard name", name); err != nil {
		return nil, err
	}
	dashboards, err := client.findDashboardsBySearch(ctx, apiKey, "accountId = "+strconv.FormatInt(client.accountID, 10)+" AND type = 'DASHBOARD' AND name = "+entitySearchString(name))
	if err != nil {
		return nil, err
	}
	for index := range dashboards {
		if dashboards[index].Name == name {
			candidate := dashboards[index]
			return &candidate, nil
		}
	}
	return nil, nil
}

func (client *NerdGraphOperations) findDashboardEventually(ctx context.Context, apiKey []byte, name string) (*newRelicDashboard, error) {
	deadline := time.Now().Add(30 * time.Second)
	for {
		found, err := client.findDashboard(ctx, apiKey, name)
		if err != nil || found != nil || time.Now().After(deadline) {
			return found, err
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (client *NerdGraphOperations) findDashboards(ctx context.Context, apiKey []byte, marker string) ([]newRelicDashboard, error) {
	search := "accountId = " + strconv.FormatInt(client.accountID, 10) + " AND type = 'DASHBOARD'"
	if marker != "" {
		search += " AND name LIKE 'MageLift Dashboard " + markerDigest(marker) + " %'"
	}
	return client.findDashboardsBySearch(ctx, apiKey, search)
}

func (client *NerdGraphOperations) findDashboardsBySearch(ctx context.Context, apiKey []byte, search string) ([]newRelicDashboard, error) {
	query := `query($query: String!, $cursor: String) { actor { entitySearch(query: $query) { results(cursor: $cursor) { nextCursor entities { guid name entityType } } } } }`
	dashboards := make([]newRelicDashboard, 0)
	seenCursors := make(map[string]struct{})
	var cursor *string
	for {
		var response struct {
			Actor struct {
				EntitySearch struct {
					Results struct {
						NextCursor *string `json:"nextCursor"`
						Entities   []struct {
							GUID       string `json:"guid"`
							Name       string `json:"name"`
							EntityType string `json:"entityType"`
						} `json:"entities"`
					} `json:"results"`
				} `json:"entitySearch"`
			} `json:"actor"`
		}
		if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, map[string]any{"query": search, "cursor": cursor}, &response); err != nil {
			return nil, errors.New("search New Relic dashboards failed")
		}
		for _, entity := range response.Actor.EntitySearch.Results.Entities {
			// New Relic indexes dashboard pages as DashboardEntity records too.
			// They are not independently owned dashboards and would otherwise
			// make an exact parent match nondeterministic during propagation.
			if entity.GUID == "" || strings.HasSuffix(entity.Name, " / Signals") || (entity.EntityType != "DASHBOARD" && entity.EntityType != "DASHBOARD_ENTITY") {
				continue
			}
			dashboard, err := client.getDashboard(ctx, apiKey, entity.GUID)
			if err != nil {
				return nil, errors.New("read New Relic dashboard failed")
			}
			if dashboard != nil {
				dashboards = append(dashboards, *dashboard)
			}
		}
		nextCursor := response.Actor.EntitySearch.Results.NextCursor
		if nextCursor == nil || *nextCursor == "" {
			break
		}
		next := *nextCursor
		if _, exists := seenCursors[next]; exists {
			return nil, errors.New("New Relic dashboard pagination repeated a cursor")
		}
		seenCursors[next] = struct{}{}
		cursor = &next
	}
	sort.Slice(dashboards, func(i, j int) bool { return dashboards[i].Name < dashboards[j].Name })
	return dashboards, nil
}

func (client *NerdGraphOperations) getDashboard(ctx context.Context, apiKey []byte, guid string) (*newRelicDashboard, error) {
	query := `query($guid: EntityGuid!) { actor { entity(guid: $guid) { ... on DashboardEntity { guid name description permissions pages { guid name widgets { id title visualization { id } rawConfiguration } } } } } }`
	var response struct {
		Actor struct {
			Entity *newRelicDashboard `json:"entity"`
		} `json:"actor"`
	}
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, map[string]any{"guid": guid}, &response); err != nil {
		return nil, err
	}
	return response.Actor.Entity, nil
}

func (client *NerdGraphOperations) createDashboard(ctx context.Context, apiKey []byte, marker string, intent sdk.DashboardIntent) (newRelicDashboard, error) {
	query := `mutation CreateDashboard($accountId: Int!, $dashboard: DashboardInput!) { dashboardCreate(accountId: $accountId, dashboard: $dashboard) { entityResult { guid name description permissions pages { guid name widgets { id title visualization { id } rawConfiguration } } } errors { type description } } }`
	var response struct {
		Create struct {
			EntityResult newRelicDashboard `json:"entityResult"`
		} `json:"dashboardCreate"`
	}
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, map[string]any{"accountId": client.accountID, "dashboard": dashboardInput(client.accountID, marker, intent)}, &response); err != nil && response.Create.EntityResult.GUID == "" {
		return newRelicDashboard{}, errors.New("create New Relic dashboard failed")
	}
	return response.Create.EntityResult, nil
}

func dashboardInput(accountID int64, marker string, intent sdk.DashboardIntent) map[string]any {
	widgets := make([]any, 0, len(intent.Signals))
	for index, signal := range sdk.SortedStrings(intent.Signals) {
		eventType, _ := nrqlEventTypeForOperationalSignal(signal)
		query, _ := buildOperationalNRQL(eventType, marker)
		widgets = append(widgets, map[string]any{"visualization": map[string]any{"id": "viz.billboard"}, "layout": map[string]any{"column": (index%4)*4 + 1, "row": (index/4)*3 + 1, "height": 3, "width": 4}, "title": signal, "rawConfiguration": map[string]any{"nrqlQueries": []any{map[string]any{"accountIds": []int64{accountID}, "query": query}}}})
	}
	return map[string]any{"name": newRelicDashboardName(marker, intent.ID), "description": newRelicDashboardDescription(marker, intent), "permissions": "PRIVATE", "pages": []any{map[string]any{"name": "Signals", "description": "MageLift-owned signal views", "widgets": widgets}}}
}

func (client *NerdGraphOperations) deleteDashboard(ctx context.Context, apiKey []byte, guid string) error {
	if err := validateProviderIdentity("New Relic dashboard GUID", guid); err != nil {
		return err
	}
	query := `mutation { dashboardDelete(guid: ` + graphQLString(guid) + `) { status errors { type description } } }`
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, nil, nil); err != nil {
		return errors.New("delete New Relic dashboard failed")
	}
	return nil
}

func (client *NerdGraphOperations) listServiceLevels(ctx context.Context, apiKey []byte, entityGUID string) ([]newRelicServiceLevel, error) {
	query := `query($guid: EntityGuid!) { actor { entity(guid: $guid) { guid serviceLevel { indicators { id entityGuid name description objectives { target timeWindow { rolling { count unit } } } } } } } }`
	var response struct {
		Actor struct {
			Entity *struct {
				ServiceLevel struct {
					Indicators []newRelicServiceLevel `json:"indicators"`
				} `json:"serviceLevel"`
			} `json:"entity"`
		} `json:"actor"`
	}
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, map[string]any{"guid": entityGUID}, &response); err != nil {
		return nil, errors.New("list New Relic service levels failed")
	}
	if response.Actor.Entity == nil {
		return nil, errors.New("New Relic SLO target entity was not found")
	}
	return response.Actor.Entity.ServiceLevel.Indicators, nil
}

func (client *NerdGraphOperations) createServiceLevel(ctx context.Context, apiKey []byte, entityGUID, marker string, intent sdk.SLOIntent) (newRelicServiceLevel, error) {
	days := intent.WindowSeconds / (24 * 60 * 60)
	validWhere, err := buildOperationalNRQLWhere(marker)
	if err != nil {
		return newRelicServiceLevel{}, err
	}
	goodWhere := validWhere + " AND error IS NOT TRUE"
	mutation := `mutation { serviceLevelCreate(entityGuid: ` + graphQLString(entityGUID) + `, indicator: { name: ` + graphQLString(newRelicSLOName(marker, intent.ID)) + `, description: ` + graphQLString(newRelicSLODescription(marker, intent)) + `, events: { validEvents: { from: "Span", where: ` + graphQLString(validWhere) + ` }, goodEvents: { from: "Span", where: ` + graphQLString(goodWhere) + ` }, accountId: ` + strconv.FormatInt(client.accountID, 10) + ` }, objectives: { target: ` + strconv.FormatFloat(intent.Target*100, 'f', -1, 64) + `, timeWindow: { rolling: { count: ` + strconv.FormatInt(days, 10) + `, unit: DAY } } } }) { id entityGuid name description objectives { target timeWindow { rolling { count unit } } } } }`
	var response struct {
		ServiceLevel newRelicServiceLevel `json:"serviceLevelCreate"`
	}
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, mutation, nil, &response); err != nil && response.ServiceLevel.ID == "" {
		return newRelicServiceLevel{}, errors.New("create New Relic service level failed")
	}
	return response.ServiceLevel, nil
}

func (client *NerdGraphOperations) deleteServiceLevel(ctx context.Context, apiKey []byte, guid string) error {
	if err := validateProviderIdentity("New Relic service-level GUID", guid); err != nil {
		return err
	}
	query := `mutation { serviceLevelDelete(guid: ` + graphQLString(guid) + `) { id } }`
	if err := executeNerdGraph(ctx, client.httpClient, client.endpoint, apiKey, query, nil, nil); err != nil {
		return errors.New("delete New Relic service level failed")
	}
	return nil
}

func buildOperationalNRQLWhere(marker string) (string, error) {
	quoted, err := quoteNRQLString(marker)
	if err != nil {
		return "", err
	}
	return "`magelift.ownership_marker` = " + quoted, nil
}

func graphQLString(value string) string { return strconv.Quote(value) }

func entitySearchString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `'`, `\'`)
	return "'" + value + "'"
}
