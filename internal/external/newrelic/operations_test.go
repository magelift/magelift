package newrelic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	provider "github.com/magelift/magelift/internal/provider"
	"github.com/magelift/magelift/sdk"
)

type nerdGraphFake struct {
	testing                *testing.T
	mu                     sync.Mutex
	queries                []string
	secretSeen             bool
	policyLive             bool
	conditionLive          bool
	dashboardLive          bool
	serviceLevelLive       bool
	failServiceLevelCreate bool
}

func (fake *nerdGraphFake) handler(w http.ResponseWriter, request *http.Request) {
	fake.testing.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		fake.testing.Fatalf("read request: %v", err)
	}
	if strings.Contains(string(body), "nerdgraph-user-key") {
		fake.mu.Lock()
		fake.secretSeen = true
		fake.mu.Unlock()
	}
	var envelope struct {
		Query     string         `json:"query"`
		Variables map[string]any `json:"variables"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		fake.testing.Fatalf("decode request: %v", err)
	}
	fake.mu.Lock()
	fake.queries = append(fake.queries, envelope.Query)
	fake.mu.Unlock()

	query := envelope.Query
	var data any
	switch {
	case strings.Contains(query, "policiesSearch"):
		policies := []any{}
		if fake.policyLive {
			policies = []any{map[string]any{"id": "policy-1", "name": newRelicPolicyName(testMarker), "incidentPreference": "PER_CONDITION"}}
		}
		data = map[string]any{"actor": map[string]any{"account": map[string]any{"alerts": map[string]any{"policiesSearch": map[string]any{"policies": policies}}}}}
	case strings.Contains(query, "alertsPolicyCreate"):
		fake.policyLive = true
		data = map[string]any{"alertsPolicyCreate": map[string]any{"id": "policy-1", "name": newRelicPolicyName(testMarker), "incidentPreference": "PER_CONDITION"}}
	case strings.Contains(query, "nrqlConditionsSearch"):
		conditions := []any{}
		if fake.conditionLive {
			conditions = []any{fakeCondition()}
		}
		data = map[string]any{"actor": map[string]any{"account": map[string]any{"alerts": map[string]any{"nrqlConditionsSearch": map[string]any{"nrqlConditions": conditions}}}}}
	case strings.Contains(query, "alertsNrqlConditionStaticCreate"):
		fake.conditionLive = true
		data = map[string]any{"alertsNrqlConditionStaticCreate": fakeCondition()}
	case strings.Contains(query, "entitySearch"):
		entities := []any{}
		if fake.dashboardLive {
			entities = []any{map[string]any{"guid": "dashboard-guid-1", "name": newRelicDashboardName(testMarker, "operations"), "entityType": "DASHBOARD"}}
		}
		data = map[string]any{"actor": map[string]any{"entitySearch": map[string]any{"results": map[string]any{"entities": entities}}}}
	case strings.Contains(query, "DashboardEntity"):
		data = map[string]any{"actor": map[string]any{"entity": fakeDashboard()}}
	case strings.Contains(query, "dashboardCreate"):
		fake.dashboardLive = true
		data = map[string]any{"dashboardCreate": map[string]any{"entityResult": fakeDashboard(), "errors": []any{}}}
	case strings.Contains(query, "serviceLevel {"):
		levels := []any{}
		if fake.serviceLevelLive {
			levels = []any{fakeServiceLevel()}
		}
		data = map[string]any{"actor": map[string]any{"entity": map[string]any{"guid": testEntityGUID, "serviceLevel": map[string]any{"indicators": levels}}}}
	case strings.Contains(query, "serviceLevelCreate"):
		if fake.failServiceLevelCreate {
			writeGraphQLError(w)
			return
		}
		fake.serviceLevelLive = true
		data = map[string]any{"serviceLevelCreate": fakeServiceLevel()}
	case strings.Contains(query, "alertsConditionDelete"), strings.Contains(query, "alertsPolicyDelete"), strings.Contains(query, "dashboardDelete"), strings.Contains(query, "serviceLevelDelete"):
		if strings.Contains(query, "alertsConditionDelete") {
			fake.conditionLive = false
		}
		if strings.Contains(query, "alertsPolicyDelete") {
			fake.policyLive = false
		}
		if strings.Contains(query, "dashboardDelete") {
			fake.dashboardLive = false
		}
		if strings.Contains(query, "serviceLevelDelete") {
			fake.serviceLevelLive = false
		}
		data = map[string]any{}
	default:
		fake.testing.Fatalf("unexpected NerdGraph query: %s", query)
	}
	response := map[string]any{"data": data}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		fake.testing.Fatalf("encode response: %v", err)
	}
}

const (
	testMarker     = "magelift/test/newrelic-operations"
	testEntityGUID = "entity-guid-1"
)

func fakeCondition() map[string]any {
	return map[string]any{
		"id": "condition-1", "name": newRelicConditionName(testMarker, "health"),
		"description": operationalDescription("alert", testMarker, "health", "platform-oncall", "https://runbooks.example/health", "deduplication=health"),
		"enabled":     true, "policyId": "policy-1", "runbookUrl": "https://runbooks.example/health", "type": "STATIC",
		"nrql":   map[string]any{"query": "SELECT count(*) FROM Metric WHERE `magelift.ownership_marker` = '" + testMarker + "'"},
		"signal": map[string]any{"aggregationWindow": 60},
		"terms":  []any{map[string]any{"operator": "ABOVE", "threshold": 1, "thresholdDuration": 60, "thresholdOccurrences": "AT_LEAST_ONCE", "priority": "CRITICAL"}},
	}
}

func fakeDashboard() map[string]any {
	return map[string]any{
		"guid": "dashboard-guid-1", "name": newRelicDashboardName(testMarker, "operations"),
		"description": newRelicDashboardDescription(testMarker, sdk.DashboardIntent{ID: "operations", Signals: []string{"metrics"}, Owner: "platform-oncall"}),
		"permissions": "PRIVATE",
		"pages": []any{map[string]any{"guid": "page-guid-1", "name": "Signals", "widgets": []any{map[string]any{
			"id": "widget-1", "title": "metrics", "visualization": map[string]any{"id": "viz.billboard"},
			"rawConfiguration": map[string]any{"nrqlQueries": []any{map[string]any{"query": "SELECT count(*) FROM Metric WHERE `magelift.ownership_marker` = '" + testMarker + "'"}}},
		}}}},
	}
}

func fakeServiceLevel() map[string]any {
	return map[string]any{
		"id": "slo-1", "entityGuid": testEntityGUID, "name": newRelicSLOName(testMarker, "availability"),
		"description": newRelicSLODescription(testMarker, sdk.SLOIntent{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 24 * 60 * 60, Owner: "platform-oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page"}),
		"objectives":  []any{map[string]any{"target": 99.9, "timeWindow": map[string]any{"rolling": map[string]any{"count": 1, "unit": "DAY"}}}},
	}
}

func writeGraphQLError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"data":null,"errors":[{"message":"synthetic failure"}]}`)
}

func newTestNerdGraphOperations(t *testing.T, fake *nerdGraphFake) *NerdGraphOperations {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(fake.handler))
	t.Cleanup(server.Close)
	client := server.Client()
	operations, err := NewNerdGraphOperations(client, provider.CredentialResolverFunc(func(_ context.Context, reference string, consume func([]byte) error) error {
		if reference != "secrets://newrelic/user-key" {
			return nil
		}
		return consume([]byte("nerdgraph-user-key"))
	}), server.URL, 12345, "secrets://newrelic/user-key")
	if err != nil {
		t.Fatal(err)
	}
	return operations
}
func testOperationsPlan() providerobservability.Plan {
	return providerobservability.Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: testMarker,
		NativeReference: testEntityGUID,
		Alerts: []sdk.AlertIntent{{
			ID: "health", Signal: "metrics", Severity: "critical", Operator: "gt", Threshold: 1,
			WindowSeconds: 60, Owner: "platform-oncall", RunbookURL: "https://runbooks.example/health", DeduplicationKey: "health",
		}},
		Dashboards: []sdk.DashboardIntent{{ID: "operations", Signals: []string{"metrics"}, Owner: "platform-oncall"}},
		SLOs: []sdk.SLOIntent{{
			ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 24 * 60 * 60,
			Owner: "platform-oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page",
		}},
	}
}

func TestNerdGraphOperationsApplyVerifyDestroyAndRedactsCredentials(t *testing.T) {
	fake := &nerdGraphFake{testing: t}
	operations := newTestNerdGraphOperations(t, fake)
	plan := testOperationsPlan()

	refs, err := operations.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 3 || !containsString(refs, "newrelic:alert-condition:condition-1") || !containsString(refs, "newrelic:dashboard:dashboard-guid-1") {
		t.Fatalf("apply refs = %v", refs)
	}
	observation, err := operations.VerifyOperations(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.AlertsVerified || !observation.DashboardsVerified || !observation.SLOsVerified {
		t.Fatalf("verification = %#v", observation)
	}
	if err := operations.Destroy(context.Background(), plan); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	secretSeen := fake.secretSeen
	queryCount := len(fake.queries)
	fake.mu.Unlock()
	if secretSeen {
		t.Fatal("credential value entered the NerdGraph request body")
	}
	if queryCount < 15 {
		t.Fatalf("query count = %d, CRUD boundary was not exercised", queryCount)
	}
}

func TestNerdGraphOperationsRejectsUnsupportedSemanticsBeforeHTTP(t *testing.T) {
	fake := &nerdGraphFake{testing: t}
	operations := newTestNerdGraphOperations(t, fake)
	plan := testOperationsPlan()
	plan.Alerts[0].Operator = "eq"
	if _, err := operations.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "only gt and lt") {
		t.Fatalf("equality operator error = %v", err)
	}
	fake.mu.Lock()
	queries := len(fake.queries)
	fake.mu.Unlock()
	if queries != 0 {
		t.Fatalf("provider was called before semantic validation: %d", queries)
	}
}

func TestNerdGraphOperationsRejectsUnsupportedMaintenancePolicyBeforeHTTP(t *testing.T) {
	fake := &nerdGraphFake{testing: t}
	operations := newTestNerdGraphOperations(t, fake)
	plan := testOperationsPlan()
	plan.Alerts[0].MaintenancePolicyRef = "maintenance/quiet-hours"
	if _, err := operations.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "maintenance policy") {
		t.Fatalf("maintenance policy error = %v", err)
	}
	fake.mu.Lock()
	queries := len(fake.queries)
	fake.mu.Unlock()
	if queries != 0 {
		t.Fatalf("provider was called before maintenance policy validation: %d", queries)
	}
}

func TestNewRelicDashboardOwnershipRequiresExactMarkerPrefix(t *testing.T) {
	tests := []struct {
		name      string
		dashboard newRelicDashboard
		wantOwned bool
	}{
		{name: "owned", dashboard: newRelicDashboard{GUID: "dashboard-1", Description: "magelift-ownership=" + testMarker + "; kind=dashboard"}, wantOwned: true},
		{name: "missing marker", dashboard: newRelicDashboard{GUID: "dashboard-1", Description: "kind=dashboard"}, wantOwned: false},
		{name: "marker is only a prefix of another marker", dashboard: newRelicDashboard{GUID: "dashboard-1", Description: "magelift-ownership=" + testMarker + "-foreign; kind=dashboard"}, wantOwned: false},
		{name: "missing guid", dashboard: newRelicDashboard{Description: "magelift-ownership=" + testMarker + "; kind=dashboard"}, wantOwned: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := newRelicDashboardOwned(test.dashboard, testMarker); got != test.wantOwned {
				t.Fatalf("dashboard ownership = %v, want %v", got, test.wantOwned)
			}
		})
	}
}

func TestNewRelicDashboardMatchesProviderPayloadShape(t *testing.T) {
	var dashboard newRelicDashboard
	payload, err := json.Marshal(fakeDashboard())
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &dashboard); err != nil {
		t.Fatal(err)
	}
	intent := sdk.DashboardIntent{ID: "operations", Signals: []string{"metrics"}, Owner: "platform-oncall"}
	if !newRelicDashboardMatches(dashboard, testMarker, intent) {
		t.Fatalf("provider-shaped dashboard did not match: %#v", dashboard)
	}
}

func TestNerdGraphOperationsRollbackDeletesCreatedDashboardAndSLO(t *testing.T) {
	fake := &nerdGraphFake{testing: t, failServiceLevelCreate: true}
	operations := newTestNerdGraphOperations(t, fake)
	plan := testOperationsPlan()
	plan.Alerts = nil
	if _, err := operations.Apply(context.Background(), plan); err == nil {
		t.Fatal("synthetic create failure was accepted")
	}
	fake.mu.Lock()
	queries := append([]string(nil), fake.queries...)
	fake.mu.Unlock()
	if !containsQuery(queries, "dashboardDelete") {
		t.Fatalf("rollback did not delete the created dashboard: %v", queries)
	}
	if containsQuery(queries, "serviceLevelDelete") {
		t.Fatal("rollback deleted an SLO that was never created")
	}
}

func TestNerdGraphLifecycleSupportsOperationsOnlyPlans(t *testing.T) {
	fake := &nerdGraphFake{testing: t}
	operations := newTestNerdGraphOperations(t, fake)
	client, err := NewNerdGraphLifecycleClient(operations)
	if err != nil {
		t.Fatal(err)
	}
	plan := testOperationsPlan()
	plan.Alerts = nil
	plan.SLOs = nil
	result, err := client.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified || len(result.ResourceRefs) != 1 {
		t.Fatalf("operations-only result = %#v", result)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsQuery(values []string, fragment string) bool {
	for _, value := range values {
		if strings.Contains(value, fragment) {
			return true
		}
	}
	return false
}
