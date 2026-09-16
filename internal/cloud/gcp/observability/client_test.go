package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/api/googleapi"
	loggingv2 "google.golang.org/api/logging/v2"
	monitoringv1 "google.golang.org/api/monitoring/v1"
	monitoringv3 "google.golang.org/api/monitoring/v3"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
)

type fakeGoogleMonitoring struct {
	series            []*monitoringv3.TimeSeries
	metricDescriptors []*monitoringv3.MetricDescriptor
	policies          []*monitoringv3.AlertPolicy
}

func TestGoogleMetricAlertFilterIncludesRequiredResourceRestriction(t *testing.T) {
	condition, err := alertCondition("marker", sdk.AlertIntent{ID: "delivery", Signal: "metrics", Operator: "gt", Threshold: 0, WindowSeconds: 60})
	if err != nil {
		t.Fatal(err)
	}
	filter := condition.ConditionThreshold.Filter
	if !strings.Contains(filter, `resource.type = "global"`) || !strings.Contains(filter, `metric.type = "custom.googleapis.com/magelift/`) {
		t.Fatalf("metric alert filter = %q", filter)
	}
}

func TestGoogleCloudAlertValidationRejectsUnsupportedOperatorBeforeMutation(t *testing.T) {
	backend, monitoring, dashboards, logging := newFakeGoogleBackend(t)
	marker := "magelift/gcp/unsupported-alert"
	_, err := backend.Apply(context.Background(), providerobservability.Plan{
		OwnershipMarker: marker,
		Bindings:        []providerobservability.SignalBinding{{Signal: "metrics", Destination: googleCloudOperationsDestination, OwnershipMarker: marker}},
		Alerts:          []sdk.AlertIntent{{ID: "equality", Signal: "metrics", Severity: "warning", Operator: "eq", Threshold: 1, WindowSeconds: 60, Owner: "platform-oncall", RunbookURL: "https://runbooks.example/equality", DeduplicationKey: "equality"}},
	})
	if err == nil || !strings.Contains(err.Error(), "operator") {
		t.Fatalf("apply error = %v, want unsupported operator", err)
	}
	if len(monitoring.series) != 0 || len(dashboards.dashboards) != 0 || len(logging.entries) != 0 {
		t.Fatalf("provider mutated before unsupported alert rejection: monitoring=%v dashboards=%v logs=%v", monitoring.series, dashboards.dashboards, logging.entries)
	}
}

type fakeGoogleSLOs struct {
	slos []*monitoringv3.ServiceLevelObjective
}

func (fake *fakeGoogleSLOs) CreateServiceLevelObjective(_ context.Context, parent, id string, objective *monitoringv3.ServiceLevelObjective) (*monitoringv3.ServiceLevelObjective, error) {
	copy := *objective
	copy.Name = parent + "/serviceLevelObjectives/" + id
	fake.slos = append(fake.slos, &copy)
	return &copy, nil
}

func (fake *fakeGoogleSLOs) ListServiceLevelObjectives(_ context.Context, _ string) ([]*monitoringv3.ServiceLevelObjective, error) {
	return append([]*monitoringv3.ServiceLevelObjective(nil), fake.slos...), nil
}

func (fake *fakeGoogleSLOs) GetServiceLevelObjective(_ context.Context, name string) (*monitoringv3.ServiceLevelObjective, error) {
	for _, slo := range fake.slos {
		if slo.Name == name {
			return slo, nil
		}
	}
	return nil, errors.New("SLO missing")
}

func (fake *fakeGoogleSLOs) DeleteServiceLevelObjective(_ context.Context, name string) error {
	for index, slo := range fake.slos {
		if slo.Name == name {
			fake.slos = append(fake.slos[:index], fake.slos[index+1:]...)
			return nil
		}
	}
	return errors.New("SLO missing")
}

func (fake *fakeGoogleMonitoring) CreateTimeSeries(_ context.Context, project string, request *monitoringv3.CreateTimeSeriesRequest) error {
	fake.series = append(fake.series, request.TimeSeries...)
	for _, series := range request.TimeSeries {
		if series == nil || series.Metric == nil {
			continue
		}
		found := false
		for _, descriptor := range fake.metricDescriptors {
			if descriptor != nil && descriptor.Type == series.Metric.Type {
				found = true
				break
			}
		}
		if found {
			continue
		}
		labels := make([]*monitoringv3.LabelDescriptor, 0, len(series.Metric.Labels))
		for key := range series.Metric.Labels {
			labels = append(labels, &monitoringv3.LabelDescriptor{Key: key, ValueType: "STRING"})
		}
		fake.metricDescriptors = append(fake.metricDescriptors, &monitoringv3.MetricDescriptor{Name: metricDescriptorName(project, series.Metric.Type), Type: series.Metric.Type, MetricKind: "GAUGE", ValueType: "DOUBLE", Labels: labels})
	}
	return nil
}

func (fake *fakeGoogleMonitoring) ListTimeSeries(_ context.Context, _ string, _ string, _, _ time.Time) ([]*monitoringv3.TimeSeries, error) {
	return append([]*monitoringv3.TimeSeries(nil), fake.series...), nil
}

func (fake *fakeGoogleMonitoring) GetMetricDescriptor(_ context.Context, _ string, metric string) (*monitoringv3.MetricDescriptor, error) {
	for _, descriptor := range fake.metricDescriptors {
		if descriptor != nil && descriptor.Type == metric {
			return descriptor, nil
		}
	}
	return nil, &googleapi.Error{Code: 404}
}

func (fake *fakeGoogleMonitoring) ListMetricDescriptors(_ context.Context, _ string, _ string) ([]*monitoringv3.MetricDescriptor, error) {
	return append([]*monitoringv3.MetricDescriptor(nil), fake.metricDescriptors...), nil
}

func (fake *fakeGoogleMonitoring) DeleteMetricDescriptor(_ context.Context, name string) error {
	for index, descriptor := range fake.metricDescriptors {
		if descriptor != nil && descriptor.Name == name {
			fake.metricDescriptors = append(fake.metricDescriptors[:index], fake.metricDescriptors[index+1:]...)
			return nil
		}
	}
	return &googleapi.Error{Code: 404}
}

func (fake *fakeGoogleMonitoring) CreateAlertPolicy(_ context.Context, project string, policy *monitoringv3.AlertPolicy) (*monitoringv3.AlertPolicy, error) {
	copy := *policy
	copy.Name = projectName(project) + "/alertPolicies/" + markerDigest(policy.DisplayName)
	fake.policies = append(fake.policies, &copy)
	return &copy, nil
}

func (fake *fakeGoogleMonitoring) ListAlertPolicies(_ context.Context, _ string) ([]*monitoringv3.AlertPolicy, error) {
	return append([]*monitoringv3.AlertPolicy(nil), fake.policies...), nil
}

func (fake *fakeGoogleMonitoring) GetAlertPolicy(_ context.Context, name string) (*monitoringv3.AlertPolicy, error) {
	for _, policy := range fake.policies {
		if policy.Name == name {
			return policy, nil
		}
	}
	return nil, errors.New("alert policy missing")
}

func (fake *fakeGoogleMonitoring) DeleteAlertPolicy(_ context.Context, name string) error {
	for index, policy := range fake.policies {
		if policy.Name == name {
			fake.policies = append(fake.policies[:index], fake.policies[index+1:]...)
			return nil
		}
	}
	return errors.New("alert policy missing")
}

type fakeGoogleDashboards struct {
	dashboards []*monitoringv1.Dashboard
}

func (fake *fakeGoogleDashboards) CreateDashboard(_ context.Context, project string, dashboard *monitoringv1.Dashboard) (*monitoringv1.Dashboard, error) {
	copy := *dashboard
	copy.Name = projectName(project) + "/dashboards/" + markerDigest(dashboard.DisplayName)
	fake.dashboards = append(fake.dashboards, &copy)
	return &copy, nil
}

func (fake *fakeGoogleDashboards) ListDashboards(_ context.Context, _ string) ([]*monitoringv1.Dashboard, error) {
	return append([]*monitoringv1.Dashboard(nil), fake.dashboards...), nil
}

func (fake *fakeGoogleDashboards) GetDashboard(_ context.Context, name string) (*monitoringv1.Dashboard, error) {
	for _, dashboard := range fake.dashboards {
		if dashboard.Name == name {
			return dashboard, nil
		}
	}
	return nil, errors.New("dashboard missing")
}

func (fake *fakeGoogleDashboards) DeleteDashboard(_ context.Context, name string) error {
	for index, dashboard := range fake.dashboards {
		if dashboard.Name == name {
			fake.dashboards = append(fake.dashboards[:index], fake.dashboards[index+1:]...)
			return nil
		}
	}
	return errors.New("dashboard missing")
}

type fakeGoogleLogging struct {
	entries      []*loggingv2.LogEntry
	listRequests []*loggingv2.ListLogEntriesRequest
}

func (fake *fakeGoogleLogging) Write(_ context.Context, request *loggingv2.WriteLogEntriesRequest) error {
	fake.entries = append(fake.entries, request.Entries...)
	return nil
}

func (fake *fakeGoogleLogging) List(_ context.Context, request *loggingv2.ListLogEntriesRequest) (*loggingv2.ListLogEntriesResponse, error) {
	fake.listRequests = append(fake.listRequests, request)
	return &loggingv2.ListLogEntriesResponse{Entries: append([]*loggingv2.LogEntry(nil), fake.entries...)}, nil
}

type fakeGoogleLogBuckets struct {
	buckets map[string]*GoogleLogBucket
	creates int
	patches int
	deletes int
}

func (fake *fakeGoogleLogBuckets) Get(_ context.Context, name string) (*GoogleLogBucket, error) {
	bucket, ok := fake.buckets[name]
	if !ok {
		return nil, &googleapi.Error{Code: 404}
	}
	copy := *bucket
	return &copy, nil
}

func (fake *fakeGoogleLogBuckets) Create(_ context.Context, _ string, _ string, bucket *GoogleLogBucket) (*GoogleLogBucket, error) {
	fake.creates++
	copy := *bucket
	copy.LifecycleState = "ACTIVE"
	fake.buckets[copy.Name] = &copy
	return &copy, nil
}

func (fake *fakeGoogleLogBuckets) Patch(_ context.Context, name string, bucket *GoogleLogBucket) (*GoogleLogBucket, error) {
	fake.patches++
	copy := *bucket
	copy.Name = name
	copy.LifecycleState = "ACTIVE"
	fake.buckets[name] = &copy
	return &copy, nil
}

func (fake *fakeGoogleLogBuckets) Delete(_ context.Context, name string) error {
	fake.deletes++
	if _, ok := fake.buckets[name]; !ok {
		return &googleapi.Error{Code: 404}
	}
	delete(fake.buckets, name)
	return nil
}

type fakeGoogleLogSinks struct {
	sinks   map[string]*GoogleLogSink
	creates int
	patches int
	deletes int
}

func TestGoogleLogSinkSDKConversionUsesTheCanonicalResourceID(t *testing.T) {
	request := googleLogSinkToSDK(&GoogleLogSink{Name: "projects/project/sinks/magelift-1234"})
	if request.Name != "magelift-1234" {
		t.Fatalf("sink request name = %q", request.Name)
	}
	response := googleLogSinkFromSDK("project", &loggingv2.LogSink{Name: "magelift-1234"})
	if response.Name != "projects/project/sinks/magelift-1234" {
		t.Fatalf("sink response name = %q", response.Name)
	}
}

func (fake *fakeGoogleLogSinks) Get(_ context.Context, name string) (*GoogleLogSink, error) {
	sink, ok := fake.sinks[name]
	if !ok {
		return nil, &googleapi.Error{Code: 404}
	}
	copy := *sink
	return &copy, nil
}

func (fake *fakeGoogleLogSinks) Create(_ context.Context, _ string, sink *GoogleLogSink) (*GoogleLogSink, error) {
	fake.creates++
	copy := *sink
	fake.sinks[copy.Name] = &copy
	return &copy, nil
}

func (fake *fakeGoogleLogSinks) Patch(_ context.Context, name string, sink *GoogleLogSink) (*GoogleLogSink, error) {
	fake.patches++
	copy := *sink
	copy.Name = name
	fake.sinks[name] = &copy
	return &copy, nil
}

func (fake *fakeGoogleLogSinks) Delete(_ context.Context, name string) error {
	fake.deletes++
	if _, ok := fake.sinks[name]; !ok {
		return &googleapi.Error{Code: 404}
	}
	delete(fake.sinks, name)
	return nil
}

func newFakeGoogleBackend(t *testing.T) (*GoogleCloudOperationsBackend, *fakeGoogleMonitoring, *fakeGoogleDashboards, *fakeGoogleLogging) {
	t.Helper()
	monitoring := &fakeGoogleMonitoring{}
	dashboards := &fakeGoogleDashboards{}
	logging := &fakeGoogleLogging{}
	backend, err := NewGoogleCloudOperationsBackend("project", monitoring, dashboards, logging)
	if err != nil {
		t.Fatal(err)
	}
	backend.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	return backend, monitoring, dashboards, logging
}

func TestGoogleCloudOperationsBackendUsesOfficialSDKModelsAndIsIdempotent(t *testing.T) {
	backend, monitoring, dashboards, logging := newFakeGoogleBackend(t)
	marker := "magelift/gcp/observability-test"
	plan := providerobservability.Plan{
		OwnershipMarker: marker,
		Bindings: []providerobservability.SignalBinding{
			{Signal: "logs", Destination: googleCloudOperationsDestination, Mode: "native", OwnershipMarker: marker},
			{Signal: "metrics", Destination: googleCloudOperationsDestination, Mode: "native", OwnershipMarker: marker},
		},
		Dashboards: []sdk.DashboardIntent{{ID: "runtime", Signals: []string{"metrics"}, Owner: "oncall"}},
		Alerts: []sdk.AlertIntent{
			{ID: "runtime-log", Signal: "logs", Severity: "critical", Operator: "gt", WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/logs", DeduplicationKey: "runtime-log"},
			{ID: "runtime-metric", Signal: "metrics", Severity: "warning", Operator: "lt", Threshold: 1, WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/metrics", DeduplicationKey: "runtime-metric"},
		},
	}

	result, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified || len(result.ResourceRefs) != 5 {
		t.Fatalf("apply result = %#v", result)
	}
	if len(logging.entries) != 1 || logging.entries[0].Labels[ownershipLabel] != markerDigest(marker) || logging.entries[0].LogName != logName("project", marker) {
		t.Fatalf("log request = %#v", logging.entries)
	}
	if len(monitoring.series) != 1 || monitoring.series[0].Metric.Type != metricType(marker, "metrics") || monitoring.series[0].Metric.Labels[ownershipLabel] != markerDigest(marker) {
		t.Fatalf("metric request = %#v", monitoring.series)
	}
	if len(dashboards.dashboards) != 1 || len(monitoring.policies) != 2 {
		t.Fatalf("provider resources = dashboards=%d policies=%d", len(dashboards.dashboards), len(monitoring.policies))
	}
	for _, policy := range monitoring.policies {
		var intent sdk.AlertIntent
		for _, candidate := range plan.Alerts {
			if policy.DisplayName == alertPolicyName(marker, candidate.ID) {
				intent = candidate
				break
			}
		}
		if !googleAlertPolicyMatches(policy, marker, intent) {
			t.Fatalf("alert policy metadata = %#v, want intent=%#v", policy, intent)
		}
	}

	second, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ResourceRefs) != len(result.ResourceRefs) || len(dashboards.dashboards) != 1 || len(monitoring.policies) != 2 {
		t.Fatalf("second apply was not idempotent: result=%#v dashboards=%d policies=%d", second, len(dashboards.dashboards), len(monitoring.policies))
	}

	for _, binding := range plan.Bindings {
		observation, err := backend.VerifySignal(context.Background(), binding)
		if err != nil {
			t.Fatal(err)
		}
		if !observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified {
			t.Fatalf("signal observation = %#v", observation)
		}
	}
	operations, err := backend.VerifyOperations(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !operations.AlertsVerified || !operations.DashboardsVerified || !operations.SLOsVerified {
		t.Fatalf("operations = %#v", operations)
	}
	inventory, err := backend.Inventory(context.Background(), marker)
	if err != nil || len(inventory) != 4 {
		t.Fatalf("inventory = %#v, err=%v", inventory, err)
	}

	if err := backend.Destroy(context.Background(), plan, result.ResourceRefs); err != nil {
		t.Fatal(err)
	}
	inventory, err = backend.Inventory(context.Background(), marker)
	if err != nil || len(inventory) != 0 {
		t.Fatalf("post-destroy inventory = %#v, err=%v", inventory, err)
	}
	if len(logging.entries) == 0 || len(monitoring.series) == 0 {
		t.Fatal("immutable probe data was unexpectedly treated as deletable infrastructure")
	}
	if len(monitoring.metricDescriptors) != 0 {
		t.Fatalf("metric descriptors were not cleaned: %#v", monitoring.metricDescriptors)
	}
}

func TestGoogleCloudOperationsBackendRejectsWrongOwnershipBeforeMutation(t *testing.T) {
	backend, monitoring, dashboards, logging := newFakeGoogleBackend(t)
	_, err := backend.Apply(context.Background(), providerobservability.Plan{
		OwnershipMarker: "magelift/gcp/test",
		Bindings:        []providerobservability.SignalBinding{{Signal: "metrics", Destination: googleCloudOperationsDestination, Mode: "native", OwnershipMarker: "other"}},
	})
	if err == nil {
		t.Fatal("wrong ownership was accepted")
	}
	if len(monitoring.series) != 0 || len(dashboards.dashboards) != 0 || len(logging.entries) != 0 {
		t.Fatalf("provider was mutated before ownership rejection: monitoring=%v dashboards=%v logs=%v", monitoring.series, dashboards.dashboards, logging.entries)
	}
}

func TestGoogleCloudOperationsBackendRejectsUnmanagedRetentionAndRedaction(t *testing.T) {
	for _, test := range []struct {
		name    string
		binding providerobservability.SignalBinding
	}{
		{name: "retention", binding: providerobservability.SignalBinding{Signal: "logs", Destination: googleCloudOperationsDestination, OwnershipMarker: "marker", RetentionDays: 30}},
		{name: "redaction", binding: providerobservability.SignalBinding{Signal: "metrics", Destination: googleCloudOperationsDestination, OwnershipMarker: "marker", RedactionPolicy: "policy"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			backend, monitoring, dashboards, logging := newFakeGoogleBackend(t)
			_, err := backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: "marker", Bindings: []providerobservability.SignalBinding{test.binding}})
			if err == nil {
				t.Fatal("unmanaged requirement was accepted")
			}
			if len(monitoring.series) != 0 || len(dashboards.dashboards) != 0 || len(logging.entries) != 0 {
				t.Fatalf("provider was mutated before rejecting requirement")
			}
		})
	}
}

func TestGoogleCloudOperationsBackendManagesLogRetentionAndRouting(t *testing.T) {
	monitoring := &fakeGoogleMonitoring{}
	dashboards := &fakeGoogleDashboards{}
	logging := &fakeGoogleLogging{}
	buckets := &fakeGoogleLogBuckets{buckets: map[string]*GoogleLogBucket{}}
	sinks := &fakeGoogleLogSinks{sinks: map[string]*GoogleLogSink{}}
	backend, err := NewGoogleCloudOperationsBackendWithRetention("project", monitoring, dashboards, logging, buckets, sinks, "global")
	if err != nil {
		t.Fatal(err)
	}
	marker := "magelift/gcp/retention-test"
	plan := providerobservability.Plan{
		OwnershipMarker: marker,
		Bindings:        []providerobservability.SignalBinding{{Signal: "logs", Destination: googleCloudOperationsDestination, Mode: "native", OwnershipMarker: marker, RetentionDays: 30}},
	}
	first, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ResourceRefs) != 3 || buckets.creates != 1 || sinks.creates != 1 {
		t.Fatalf("retention apply = %#v, bucket creates=%d, sink creates=%d", first, buckets.creates, sinks.creates)
	}
	bucketName := googleLogBucketName("project", "global", marker)
	sinkName := googleLogSinkName("project", marker)
	if buckets.buckets[bucketName].RetentionDays != 30 {
		t.Fatalf("bucket retention = %#v", buckets.buckets[bucketName])
	}
	if sinks.sinks[sinkName].Destination != "logging.googleapis.com/"+bucketName || sinks.sinks[sinkName].Filter != `logName = "projects/project/logs/magelift-observability-`+markerDigest(marker)+`"` {
		t.Fatalf("sink routing = %#v", sinks.sinks[sinkName])
	}
	second, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ResourceRefs) != 3 || buckets.creates != 1 || sinks.creates != 1 || buckets.patches != 0 || sinks.patches != 0 {
		t.Fatalf("retention apply was not idempotent: result=%#v buckets=%#v sinks=%#v", second, buckets, sinks)
	}
	observation, err := backend.VerifySignal(context.Background(), plan.Bindings[0])
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified {
		t.Fatalf("retention signal observation = %#v", observation)
	}
	if len(logging.listRequests) < 2 || len(logging.listRequests[len(logging.listRequests)-1].ResourceNames) != 1 || logging.listRequests[len(logging.listRequests)-1].ResourceNames[0] != googleLogBucketViewName(bucketName) {
		t.Fatalf("retention verification did not query the owned bucket: %#v", logging.listRequests)
	}
	inventory, err := backend.Inventory(context.Background(), marker)
	if err != nil || len(inventory) != 2 {
		t.Fatalf("retention inventory = %#v, err=%v", inventory, err)
	}
	if err := backend.Destroy(context.Background(), plan, first.ResourceRefs); err != nil {
		t.Fatal(err)
	}
	if len(buckets.buckets) != 0 || len(sinks.sinks) != 0 {
		t.Fatalf("retention resources survived cleanup: buckets=%#v sinks=%#v", buckets.buckets, sinks.sinks)
	}
}

func TestGoogleCloudOperationsBackendExplainsLogRetentionVerificationBoundary(t *testing.T) {
	marker := "magelift/gcp/retention-diagnostic"
	plan := providerobservability.Plan{
		OwnershipMarker: marker,
		Bindings:        []providerobservability.SignalBinding{{Signal: "logs", Destination: googleCloudOperationsDestination, Mode: "native", OwnershipMarker: marker, RetentionDays: 30}},
	}

	t.Run("routed probe missing", func(t *testing.T) {
		monitoring := &fakeGoogleMonitoring{}
		dashboards := &fakeGoogleDashboards{}
		logging := &fakeGoogleLogging{}
		buckets := &fakeGoogleLogBuckets{buckets: map[string]*GoogleLogBucket{}}
		sinks := &fakeGoogleLogSinks{sinks: map[string]*GoogleLogSink{}}
		backend, err := NewGoogleCloudOperationsBackendWithRetention("project", monitoring, dashboards, logging, buckets, sinks, "global")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := backend.Apply(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
		logging.entries = nil
		verified, reason, err := backend.verifyLogRetention(context.Background(), marker, 30)
		if err != nil {
			t.Fatal(err)
		}
		if verified || !strings.Contains(reason, "routed probe") {
			t.Fatalf("verified=%t reason=%q, want a routed-probe diagnostic", verified, reason)
		}
	})

	t.Run("bucket configuration drift", func(t *testing.T) {
		monitoring := &fakeGoogleMonitoring{}
		dashboards := &fakeGoogleDashboards{}
		logging := &fakeGoogleLogging{}
		buckets := &fakeGoogleLogBuckets{buckets: map[string]*GoogleLogBucket{}}
		sinks := &fakeGoogleLogSinks{sinks: map[string]*GoogleLogSink{}}
		backend, err := NewGoogleCloudOperationsBackendWithRetention("project", monitoring, dashboards, logging, buckets, sinks, "global")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := backend.Apply(context.Background(), plan); err != nil {
			t.Fatal(err)
		}
		bucketName := googleLogBucketName("project", "global", marker)
		buckets.buckets[bucketName].RetentionDays = 7
		verified, reason, err := backend.verifyLogRetention(context.Background(), marker, 30)
		if err != nil {
			t.Fatal(err)
		}
		if verified || !strings.Contains(reason, "configuration mismatch") {
			t.Fatalf("verified=%t reason=%q, want a bucket-configuration diagnostic", verified, reason)
		}
	})
}

func TestGoogleCloudOperationsBackendRejectsMetricRetentionBeforeMutation(t *testing.T) {
	monitoring := &fakeGoogleMonitoring{}
	dashboards := &fakeGoogleDashboards{}
	logging := &fakeGoogleLogging{}
	buckets := &fakeGoogleLogBuckets{buckets: map[string]*GoogleLogBucket{}}
	sinks := &fakeGoogleLogSinks{sinks: map[string]*GoogleLogSink{}}
	backend, err := NewGoogleCloudOperationsBackendWithRetention("project", monitoring, dashboards, logging, buckets, sinks, "global")
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Apply(context.Background(), providerobservability.Plan{
		OwnershipMarker: "marker",
		Bindings:        []providerobservability.SignalBinding{{Signal: "metrics", Destination: googleCloudOperationsDestination, OwnershipMarker: "marker", RetentionDays: 30}},
	})
	if err == nil || !strings.Contains(err.Error(), "metric retention") {
		t.Fatalf("metric retention error = %v", err)
	}
	if len(buckets.buckets) != 0 || len(sinks.sinks) != 0 || len(logging.entries) != 0 || len(monitoring.series) != 0 {
		t.Fatal("metric retention rejection mutated provider resources")
	}
}

func TestGoogleCloudOperationsBackendManagesSLOsThroughScopedNativeReference(t *testing.T) {
	monitoring := &fakeGoogleMonitoring{}
	dashboards := &fakeGoogleDashboards{}
	logging := &fakeGoogleLogging{}
	slos := &fakeGoogleSLOs{}
	backend, err := NewGoogleCloudOperationsBackendWithSLO("project", monitoring, dashboards, logging, slos)
	if err != nil {
		t.Fatal(err)
	}
	marker := "magelift/gcp/slo-test"
	parent := "projects/project/services/checkout"
	plan := providerobservability.Plan{
		NativeReference: parent,
		OwnershipMarker: marker,
		SLOs:            []sdk.SLOIntent{{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 7 * 24 * 60 * 60, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page-on-burn"}},
	}
	first, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.ResourceRefs) != 1 || len(slos.slos) != 1 {
		t.Fatalf("first SLO apply = %#v, resources=%d", first, len(slos.slos))
	}
	second, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ResourceRefs) != 1 || len(slos.slos) != 1 {
		t.Fatalf("SLO apply was not idempotent: result=%#v resources=%d", second, len(slos.slos))
	}
	operations, err := backend.VerifyOperations(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !operations.SLOsVerified {
		t.Fatalf("SLO operations = %#v", operations)
	}
	inventory, err := backend.InventoryForPlan(context.Background(), plan)
	if err != nil || len(inventory) != 1 || inventory[0].Identity != "google-cloud-operations:slo:"+slos.slos[0].Name {
		t.Fatalf("scoped SLO inventory = %#v, err=%v", inventory, err)
	}
	if err := backend.Destroy(context.Background(), plan, first.ResourceRefs); err != nil {
		t.Fatal(err)
	}
	inventory, err = backend.InventoryForPlan(context.Background(), plan)
	if err != nil || len(inventory) != 0 {
		t.Fatalf("post-destroy SLO inventory = %#v, err=%v", inventory, err)
	}
}

func TestGoogleCloudOperationsBackendRejectsForeignSLOCollisionBeforeMutation(t *testing.T) {
	monitoring := &fakeGoogleMonitoring{}
	dashboards := &fakeGoogleDashboards{}
	logging := &fakeGoogleLogging{}
	slos := &fakeGoogleSLOs{slos: []*monitoringv3.ServiceLevelObjective{{
		Name:        "projects/project/services/checkout/serviceLevelObjectives/foreign",
		DisplayName: googleSLODisplayName("magelift/gcp/slo-collision", "availability"),
		UserLabels:  map[string]string{"owner": "someone-else"},
	}}}
	backend, err := NewGoogleCloudOperationsBackendWithSLO("project", monitoring, dashboards, logging, slos)
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Apply(context.Background(), providerobservability.Plan{
		NativeReference: "projects/project/services/checkout",
		OwnershipMarker: "magelift/gcp/slo-collision",
		SLOs:            []sdk.SLOIntent{{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 86400, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page-on-burn"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unowned resource") {
		t.Fatalf("foreign SLO collision error = %v", err)
	}
	if len(slos.slos) != 1 {
		t.Fatalf("SLO provider mutated after collision: %#v", slos.slos)
	}
}

func TestGoogleCloudOperationsBackendRejectsSLOFromDifferentServiceParent(t *testing.T) {
	monitoring := &fakeGoogleMonitoring{}
	dashboards := &fakeGoogleDashboards{}
	logging := &fakeGoogleLogging{}
	intent := sdk.SLOIntent{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 24 * 60 * 60}
	parent := "projects/project/services/checkout"
	foreignParent := "projects/project/services/other"
	slos := &fakeGoogleSLOs{slos: []*monitoringv3.ServiceLevelObjective{{
		Name:                  googleSLOResourceName(foreignParent, "magelift/test", intent.ID),
		DisplayName:           googleSLODisplayName("magelift/test", intent.ID),
		Goal:                  intent.Target,
		RollingPeriod:         "86400s",
		UserLabels:            ownershipLabels("magelift/test"),
		ServiceLevelIndicator: &monitoringv3.ServiceLevelIndicator{BasicSli: &monitoringv3.BasicSli{Availability: &monitoringv3.AvailabilityCriteria{}}},
	}}}
	backend, err := NewGoogleCloudOperationsBackendWithSLO("project", monitoring, dashboards, logging, slos)
	if err != nil {
		t.Fatal(err)
	}
	plan := providerobservability.Plan{NativeReference: parent, OwnershipMarker: "magelift/test", SLOs: []sdk.SLOIntent{intent}}
	operations, err := backend.VerifyOperations(context.Background(), plan)
	if err != nil {
		t.Fatalf("VerifyOperations() error = %v", err)
	}
	if operations.SLOsVerified {
		t.Fatalf("foreign-parent SLO was incorrectly verified: %#v", operations)
	}
}

func TestGoogleCloudOperationsBackendRequiresSLOAPIBeforeMutation(t *testing.T) {
	backend, monitoring, dashboards, logging := newFakeGoogleBackend(t)
	_, err := backend.Apply(context.Background(), providerobservability.Plan{
		NativeReference: "projects/project/services/checkout",
		OwnershipMarker: "magelift/gcp/slo-missing-api",
		SLOs:            []sdk.SLOIntent{{ID: "availability", Signal: "application-health", Target: 0.999, WindowSeconds: 86400, Owner: "oncall", RunbookURL: "https://runbooks.example/availability", ErrorBudgetPolicy: "page-on-burn"}},
	})
	if err == nil || !strings.Contains(err.Error(), "Service Monitoring API") {
		t.Fatalf("missing SLO API error = %v", err)
	}
	if len(monitoring.series) != 0 || len(dashboards.dashboards) != 0 || len(logging.entries) != 0 {
		t.Fatalf("provider mutated before missing SLO API rejection")
	}
}
