package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/googleapi"
	loggingv2 "google.golang.org/api/logging/v2"
	monitoringv1 "google.golang.org/api/monitoring/v1"
	monitoringv3 "google.golang.org/api/monitoring/v3"
	"google.golang.org/api/option"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk/v1"
)

const googleCloudOperationsDestination = "google-cloud-operations"

// GoogleMonitoringAPI is the narrow semantic surface used by the GCP
// observability translator. The generated Google SDK types stay behind this
// boundary, while fakes can still exercise the exact request models.
type GoogleMonitoringAPI interface {
	CreateTimeSeries(context.Context, string, *monitoringv3.CreateTimeSeriesRequest) error
	ListTimeSeries(context.Context, string, string, time.Time, time.Time) ([]*monitoringv3.TimeSeries, error)
	GetMetricDescriptor(context.Context, string, string) (*monitoringv3.MetricDescriptor, error)
	ListMetricDescriptors(context.Context, string, string) ([]*monitoringv3.MetricDescriptor, error)
	DeleteMetricDescriptor(context.Context, string) error
	CreateAlertPolicy(context.Context, string, *monitoringv3.AlertPolicy) (*monitoringv3.AlertPolicy, error)
	ListAlertPolicies(context.Context, string) ([]*monitoringv3.AlertPolicy, error)
	GetAlertPolicy(context.Context, string) (*monitoringv3.AlertPolicy, error)
	DeleteAlertPolicy(context.Context, string) error
}

// GoogleServiceLevelObjectiveAPI is the narrow Cloud Monitoring SLO surface
// used by the lifecycle. SLOs are children of an existing Monitoring Service;
// the service resource name remains an opaque native reference in the
// provider-neutral plan.
type GoogleServiceLevelObjectiveAPI interface {
	CreateServiceLevelObjective(context.Context, string, string, *monitoringv3.ServiceLevelObjective) (*monitoringv3.ServiceLevelObjective, error)
	ListServiceLevelObjectives(context.Context, string) ([]*monitoringv3.ServiceLevelObjective, error)
	GetServiceLevelObjective(context.Context, string) (*monitoringv3.ServiceLevelObjective, error)
	DeleteServiceLevelObjective(context.Context, string) error
}

// GoogleDashboardAPI is the dashboard subset required by the lifecycle.
type GoogleDashboardAPI interface {
	CreateDashboard(context.Context, string, *monitoringv1.Dashboard) (*monitoringv1.Dashboard, error)
	ListDashboards(context.Context, string) ([]*monitoringv1.Dashboard, error)
	GetDashboard(context.Context, string) (*monitoringv1.Dashboard, error)
	DeleteDashboard(context.Context, string) error
}

// GoogleLoggingAPI is the write/list surface used for delivery probes. Log
// entries themselves are immutable; they are intentionally not represented as
// deletable lifecycle resources by this adapter.
type GoogleLoggingAPI interface {
	Write(context.Context, *loggingv2.WriteLogEntriesRequest) error
	List(context.Context, *loggingv2.ListLogEntriesRequest) (*loggingv2.ListLogEntriesResponse, error)
}

// GoogleLogBucket is the provider-local shape required to manage a project
// logging bucket. The generated Google API model does not cross this adapter
// boundary.
type GoogleLogBucket struct {
	Name           string
	Description    string
	RetentionDays  int
	Locked         bool
	LifecycleState string
}

// GoogleLogBucketAPI is the narrow owning-service surface used for managed log
// retention. A deterministic name lets inventory and cleanup address exactly
// one MageLift bucket without enumerating or mutating user buckets.
type GoogleLogBucketAPI interface {
	Get(context.Context, string) (*GoogleLogBucket, error)
	Create(context.Context, string, string, *GoogleLogBucket) (*GoogleLogBucket, error)
	Patch(context.Context, string, *GoogleLogBucket) (*GoogleLogBucket, error)
	Delete(context.Context, string) error
}

// GoogleLogSink is the provider-local shape required to route MageLift log
// probes into the owned bucket.
type GoogleLogSink struct {
	Name        string
	Description string
	Destination string
	Filter      string
	Disabled    bool
}

// GoogleLogSinkAPI is the narrow project-sink surface paired with
// GoogleLogBucketAPI. A bucket without this routing resource would not prove
// retention for the emitted probe entries.
type GoogleLogSinkAPI interface {
	Get(context.Context, string) (*GoogleLogSink, error)
	Create(context.Context, string, *GoogleLogSink) (*GoogleLogSink, error)
	Patch(context.Context, string, *GoogleLogSink) (*GoogleLogSink, error)
	Delete(context.Context, string) error
}

type googleMonitoringAPI struct {
	service *monitoringv3.Service
}

func (api googleMonitoringAPI) CreateTimeSeries(ctx context.Context, project string, request *monitoringv3.CreateTimeSeriesRequest) error {
	_, err := api.service.Projects.TimeSeries.Create(projectName(project), request).Context(ctx).Do()
	return err
}

func (api googleMonitoringAPI) ListTimeSeries(ctx context.Context, project, filter string, start, end time.Time) ([]*monitoringv3.TimeSeries, error) {
	var result []*monitoringv3.TimeSeries
	pageToken := ""
	for {
		call := api.service.Projects.TimeSeries.List(projectName(project)).
			Filter(filter).
			IntervalStartTime(start.UTC().Format(time.RFC3339Nano)).
			IntervalEndTime(end.UTC().Format(time.RFC3339Nano)).
			PageSize(1000)
		if pageToken != "" {
			call.PageToken(pageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		result = append(result, response.TimeSeries...)
		if response.NextPageToken == "" {
			return result, nil
		}
		pageToken = response.NextPageToken
	}
}

func (api googleMonitoringAPI) GetMetricDescriptor(ctx context.Context, project, metric string) (*monitoringv3.MetricDescriptor, error) {
	return api.service.Projects.MetricDescriptors.Get(metricDescriptorName(project, metric)).Context(ctx).Do()
}

func (api googleMonitoringAPI) ListMetricDescriptors(ctx context.Context, project, filter string) ([]*monitoringv3.MetricDescriptor, error) {
	var result []*monitoringv3.MetricDescriptor
	pageToken := ""
	for {
		call := api.service.Projects.MetricDescriptors.List(projectName(project)).Filter(filter).PageSize(1000)
		if pageToken != "" {
			call.PageToken(pageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		result = append(result, response.MetricDescriptors...)
		if response.NextPageToken == "" {
			return result, nil
		}
		pageToken = response.NextPageToken
	}
}

func (api googleMonitoringAPI) DeleteMetricDescriptor(ctx context.Context, name string) error {
	_, err := api.service.Projects.MetricDescriptors.Delete(name).Context(ctx).Do()
	return err
}

func (api googleMonitoringAPI) CreateAlertPolicy(ctx context.Context, project string, policy *monitoringv3.AlertPolicy) (*monitoringv3.AlertPolicy, error) {
	return api.service.Projects.AlertPolicies.Create(projectName(project), policy).Context(ctx).Do()
}

func (api googleMonitoringAPI) ListAlertPolicies(ctx context.Context, project string) ([]*monitoringv3.AlertPolicy, error) {
	var result []*monitoringv3.AlertPolicy
	pageToken := ""
	for {
		call := api.service.Projects.AlertPolicies.List(projectName(project)).PageSize(1000)
		if pageToken != "" {
			call.PageToken(pageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		result = append(result, response.AlertPolicies...)
		if response.NextPageToken == "" {
			return result, nil
		}
		pageToken = response.NextPageToken
	}
}

func (api googleMonitoringAPI) GetAlertPolicy(ctx context.Context, name string) (*monitoringv3.AlertPolicy, error) {
	return api.service.Projects.AlertPolicies.Get(name).Context(ctx).Do()
}

func (api googleMonitoringAPI) DeleteAlertPolicy(ctx context.Context, name string) error {
	_, err := api.service.Projects.AlertPolicies.Delete(name).Context(ctx).Do()
	return err
}

func (api googleMonitoringAPI) CreateServiceLevelObjective(ctx context.Context, parent, id string, objective *monitoringv3.ServiceLevelObjective) (*monitoringv3.ServiceLevelObjective, error) {
	return api.service.Services.ServiceLevelObjectives.Create(parent, objective).ServiceLevelObjectiveId(id).Context(ctx).Do()
}

func (api googleMonitoringAPI) ListServiceLevelObjectives(ctx context.Context, parent string) ([]*monitoringv3.ServiceLevelObjective, error) {
	var result []*monitoringv3.ServiceLevelObjective
	pageToken := ""
	for {
		call := api.service.Services.ServiceLevelObjectives.List(parent).PageSize(500)
		if pageToken != "" {
			call.PageToken(pageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		result = append(result, response.ServiceLevelObjectives...)
		if response.NextPageToken == "" {
			return result, nil
		}
		pageToken = response.NextPageToken
	}
}

func (api googleMonitoringAPI) GetServiceLevelObjective(ctx context.Context, name string) (*monitoringv3.ServiceLevelObjective, error) {
	return api.service.Services.ServiceLevelObjectives.Get(name).Context(ctx).Do()
}

func (api googleMonitoringAPI) DeleteServiceLevelObjective(ctx context.Context, name string) error {
	_, err := api.service.Services.ServiceLevelObjectives.Delete(name).Context(ctx).Do()
	return err
}

type googleDashboardAPI struct {
	service *monitoringv1.Service
}

func (api googleDashboardAPI) CreateDashboard(ctx context.Context, project string, dashboard *monitoringv1.Dashboard) (*monitoringv1.Dashboard, error) {
	return api.service.Projects.Dashboards.Create(projectName(project), dashboard).Context(ctx).Do()
}

func (api googleDashboardAPI) ListDashboards(ctx context.Context, project string) ([]*monitoringv1.Dashboard, error) {
	var result []*monitoringv1.Dashboard
	pageToken := ""
	for {
		call := api.service.Projects.Dashboards.List(projectName(project)).PageSize(1000)
		if pageToken != "" {
			call.PageToken(pageToken)
		}
		response, err := call.Context(ctx).Do()
		if err != nil {
			return nil, err
		}
		result = append(result, response.Dashboards...)
		if response.NextPageToken == "" {
			return result, nil
		}
		pageToken = response.NextPageToken
	}
}

func (api googleDashboardAPI) GetDashboard(ctx context.Context, name string) (*monitoringv1.Dashboard, error) {
	return api.service.Projects.Dashboards.Get(name).Context(ctx).Do()
}

func (api googleDashboardAPI) DeleteDashboard(ctx context.Context, name string) error {
	_, err := api.service.Projects.Dashboards.Delete(name).Context(ctx).Do()
	return err
}

type googleLoggingAPI struct {
	service *loggingv2.Service
}

func (api googleLoggingAPI) Write(ctx context.Context, request *loggingv2.WriteLogEntriesRequest) error {
	_, err := api.service.Entries.Write(request).Context(ctx).Do()
	return err
}

func (api googleLoggingAPI) List(ctx context.Context, request *loggingv2.ListLogEntriesRequest) (*loggingv2.ListLogEntriesResponse, error) {
	return api.service.Entries.List(request).Context(ctx).Do()
}

type googleLogBucketAPI struct {
	service *loggingv2.Service
}

func (api googleLogBucketAPI) Get(ctx context.Context, name string) (*GoogleLogBucket, error) {
	bucket, err := api.service.Projects.Locations.Buckets.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return googleLogBucketFromSDK(bucket), nil
}

func (api googleLogBucketAPI) Create(ctx context.Context, parent, id string, bucket *GoogleLogBucket) (*GoogleLogBucket, error) {
	created, err := api.service.Projects.Locations.Buckets.Create(parent, googleLogBucketToSDK(bucket)).BucketId(id).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return googleLogBucketFromSDK(created), nil
}

func (api googleLogBucketAPI) Patch(ctx context.Context, name string, bucket *GoogleLogBucket) (*GoogleLogBucket, error) {
	updated, err := api.service.Projects.Locations.Buckets.Patch(name, googleLogBucketToSDK(bucket)).UpdateMask("retention_days").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return googleLogBucketFromSDK(updated), nil
}

func (api googleLogBucketAPI) Delete(ctx context.Context, name string) error {
	_, err := api.service.Projects.Locations.Buckets.Delete(name).Context(ctx).Do()
	return err
}

type googleLogSinkAPI struct {
	service *loggingv2.Service
}

func (api googleLogSinkAPI) Get(ctx context.Context, name string) (*GoogleLogSink, error) {
	sink, err := api.service.Projects.Sinks.Get(name).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return googleLogSinkFromSDK(googleLogSinkProject(name), sink), nil
}

func (api googleLogSinkAPI) Create(ctx context.Context, project string, sink *GoogleLogSink) (*GoogleLogSink, error) {
	created, err := api.service.Projects.Sinks.Create(projectName(project), googleLogSinkToSDK(sink)).Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return googleLogSinkFromSDK(project, created), nil
}

func (api googleLogSinkAPI) Patch(ctx context.Context, name string, sink *GoogleLogSink) (*GoogleLogSink, error) {
	updated, err := api.service.Projects.Sinks.Patch(name, googleLogSinkToSDK(sink)).UpdateMask("description,destination,filter,disabled").Context(ctx).Do()
	if err != nil {
		return nil, err
	}
	return googleLogSinkFromSDK(googleLogSinkProject(name), updated), nil
}

func (api googleLogSinkAPI) Delete(ctx context.Context, name string) error {
	_, err := api.service.Projects.Sinks.Delete(name).Context(ctx).Do()
	return err
}

func googleLogBucketFromSDK(bucket *loggingv2.LogBucket) *GoogleLogBucket {
	if bucket == nil {
		return nil
	}
	return &GoogleLogBucket{Name: bucket.Name, Description: bucket.Description, RetentionDays: int(bucket.RetentionDays), Locked: bucket.Locked, LifecycleState: bucket.LifecycleState}
}

func googleLogBucketToSDK(bucket *GoogleLogBucket) *loggingv2.LogBucket {
	if bucket == nil {
		return nil
	}
	return &loggingv2.LogBucket{
		Description:     bucket.Description,
		RetentionDays:   int64(bucket.RetentionDays),
		Locked:          bucket.Locked,
		ForceSendFields: []string{"Description", "RetentionDays", "Locked"},
	}
}

func googleLogSinkFromSDK(project string, sink *loggingv2.LogSink) *GoogleLogSink {
	if sink == nil {
		return nil
	}
	name := sink.ResourceName
	if name == "" {
		name = sink.Name
	}
	if name != "" && !strings.HasPrefix(name, "projects/") && strings.TrimSpace(project) != "" {
		name = projectName(project) + "/sinks/" + name
	}
	return &GoogleLogSink{Name: name, Description: sink.Description, Destination: sink.Destination, Filter: sink.Filter, Disabled: sink.Disabled}
}

func googleLogSinkToSDK(sink *GoogleLogSink) *loggingv2.LogSink {
	if sink == nil {
		return nil
	}
	return &loggingv2.LogSink{Name: googleLogSinkIDFromResourceName(sink.Name), Description: sink.Description, Destination: sink.Destination, Filter: sink.Filter, Disabled: sink.Disabled, ForceSendFields: []string{"Description", "Destination", "Filter", "Disabled"}}
}

// NewGoogleCloudOperationsSDKClient creates a lifecycle client from the
// official Google APIs. Authentication and project permissions are resolved by
// the caller's Google credential boundary through option.ClientOption.
func NewGoogleCloudOperationsSDKClient(ctx context.Context, project string, opts ...option.ClientOption) (*providerobservability.ManagedLifecycleClient, error) {
	if ctx == nil {
		return nil, errors.New("Google Cloud observability context is required")
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("Google Cloud observability project is required")
	}
	monitoringService, err := monitoringv3.NewService(ctx, opts...)
	if err != nil {
		return nil, errors.New("create Google Cloud Monitoring service failed")
	}
	dashboardService, err := monitoringv1.NewService(ctx, opts...)
	if err != nil {
		return nil, errors.New("create Google Cloud dashboard service failed")
	}
	loggingService, err := loggingv2.NewService(ctx, opts...)
	if err != nil {
		return nil, errors.New("create Google Cloud Logging service failed")
	}
	monitoringAPI := googleMonitoringAPI{service: monitoringService}
	backend, err := NewGoogleCloudOperationsBackendWithSLOAndRetention(project, monitoringAPI, googleDashboardAPI{service: dashboardService}, googleLoggingAPI{service: loggingService}, monitoringAPI, googleLogBucketAPI{service: loggingService}, googleLogSinkAPI{service: loggingService}, googleCloudOperationsDefaultLogLocation)
	if err != nil {
		return nil, err
	}
	return providerobservability.NewManagedLifecycleClient(backend)
}

// GoogleCloudOperationsBackend translates the portable observability plan to
// Cloud Monitoring dashboards/alerts, Cloud Monitoring custom metrics, and
// Cloud Logging delivery probes. It owns only resources carrying the exact
// marker digest in provider labels.
type GoogleCloudOperationsBackend struct {
	project     string
	monitoring  GoogleMonitoringAPI
	slos        GoogleServiceLevelObjectiveAPI
	dashboards  GoogleDashboardAPI
	logging     GoogleLoggingAPI
	logBuckets  GoogleLogBucketAPI
	logSinks    GoogleLogSinkAPI
	logLocation string
	now         func() time.Time
}

var _ providerobservability.Backend = (*GoogleCloudOperationsBackend)(nil)

func NewGoogleCloudOperationsBackend(project string, monitoring GoogleMonitoringAPI, dashboards GoogleDashboardAPI, logging GoogleLoggingAPI) (*GoogleCloudOperationsBackend, error) {
	return newGoogleCloudOperationsBackend(project, monitoring, nil, dashboards, logging, nil, nil, "")
}

// NewGoogleCloudOperationsBackendWithSLO enables the complete native
// operational-object lifecycle, including SLOs attached to an existing
// Monitoring Service. The original constructor remains useful for clients
// that intentionally do not expose SLO operations.
func NewGoogleCloudOperationsBackendWithSLO(project string, monitoring GoogleMonitoringAPI, dashboards GoogleDashboardAPI, logging GoogleLoggingAPI, slos GoogleServiceLevelObjectiveAPI) (*GoogleCloudOperationsBackend, error) {
	if slos == nil {
		return nil, errors.New("Google Cloud Service Monitoring SLO client is required")
	}
	return newGoogleCloudOperationsBackend(project, monitoring, slos, dashboards, logging, nil, nil, "")
}

// NewGoogleCloudOperationsBackendWithRetention enables the provider-owned
// Cloud Logging bucket and project-sink lifecycle without requiring SLO
// support. The location is provider-specific configuration; "global" is the
// usual default, but callers may select any supported Cloud Logging location.
func NewGoogleCloudOperationsBackendWithRetention(project string, monitoring GoogleMonitoringAPI, dashboards GoogleDashboardAPI, logging GoogleLoggingAPI, buckets GoogleLogBucketAPI, sinks GoogleLogSinkAPI, location string) (*GoogleCloudOperationsBackend, error) {
	return newGoogleCloudOperationsBackend(project, monitoring, nil, dashboards, logging, buckets, sinks, location)
}

// NewGoogleCloudOperationsBackendWithSLOAndRetention enables both the native
// Service Monitoring SLO lifecycle and managed Cloud Logging retention.
func NewGoogleCloudOperationsBackendWithSLOAndRetention(project string, monitoring GoogleMonitoringAPI, dashboards GoogleDashboardAPI, logging GoogleLoggingAPI, slos GoogleServiceLevelObjectiveAPI, buckets GoogleLogBucketAPI, sinks GoogleLogSinkAPI, location string) (*GoogleCloudOperationsBackend, error) {
	if slos == nil {
		return nil, errors.New("Google Cloud Service Monitoring SLO client is required")
	}
	return newGoogleCloudOperationsBackend(project, monitoring, slos, dashboards, logging, buckets, sinks, location)
}

func newGoogleCloudOperationsBackend(project string, monitoring GoogleMonitoringAPI, slos GoogleServiceLevelObjectiveAPI, dashboards GoogleDashboardAPI, logging GoogleLoggingAPI, buckets GoogleLogBucketAPI, sinks GoogleLogSinkAPI, location string) (*GoogleCloudOperationsBackend, error) {
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("Google Cloud observability project is required")
	}
	if monitoring == nil || dashboards == nil || logging == nil {
		return nil, errors.New("Google Cloud Monitoring, dashboard, and Logging clients are required")
	}
	if (buckets == nil) != (sinks == nil) {
		return nil, errors.New("Google Cloud log bucket and sink clients must be configured together")
	}
	if buckets != nil && !validGoogleLogLocation(location) {
		return nil, errors.New("Google Cloud Logging location is required and must be a single path segment")
	}
	return &GoogleCloudOperationsBackend{project: project, monitoring: monitoring, slos: slos, dashboards: dashboards, logging: logging, logBuckets: buckets, logSinks: sinks, logLocation: location, now: time.Now}, nil
}

func (backend *GoogleCloudOperationsBackend) Apply(ctx context.Context, plan providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	}
	refs := make(map[string]struct{})
	if bucket, sink, enabled, err := backend.ensureLogRetention(ctx, plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	} else if enabled {
		refs["google-cloud-operations:log-bucket:"+bucket] = struct{}{}
		refs["google-cloud-operations:log-sink:"+sink] = struct{}{}
	}
	for _, binding := range plan.Bindings {
		if signalKind(binding.Signal) == "logs" {
			if err := backend.writeLogProbe(ctx, binding); err != nil {
				return providerobservability.LifecycleResult{}, err
			}
			refs["google-cloud-operations:log-probe:"+logName(backend.project, binding.OwnershipMarker)] = struct{}{}
		} else {
			if err := backend.writeMetricProbe(ctx, binding); err != nil {
				return providerobservability.LifecycleResult{}, err
			}
			refs["google-cloud-operations:metric:"+metricType(binding.OwnershipMarker, binding.Signal)] = struct{}{}
		}
	}
	for _, dashboard := range plan.Dashboards {
		resource, err := backend.ensureDashboard(ctx, plan.OwnershipMarker, dashboard)
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs["google-cloud-operations:dashboard:"+resourceName(resource.Name, dashboardName(plan.OwnershipMarker, dashboard.ID))] = struct{}{}
	}
	for _, alert := range plan.Alerts {
		resource, err := backend.ensureAlertPolicy(ctx, plan.OwnershipMarker, alert)
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs["google-cloud-operations:alert-policy:"+resourceName(resource.Name, alertPolicyName(plan.OwnershipMarker, alert.ID))] = struct{}{}
	}
	for _, slo := range plan.SLOs {
		resource, err := backend.ensureSLO(ctx, plan.NativeReference, plan.OwnershipMarker, slo)
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs["google-cloud-operations:slo:"+resource.Name] = struct{}{}
	}
	resourceRefs := make([]string, 0, len(refs))
	for ref := range refs {
		resourceRefs = append(resourceRefs, ref)
	}
	sort.Strings(resourceRefs)
	return providerobservability.LifecycleResult{
		OperationID:         "google-cloud-operations:observability:apply:" + markerDigest(plan.OwnershipMarker),
		ResourceRefs:        resourceRefs,
		ProofRefs:           []string{"google-cloud-operations:ownership-labels", "google-cloud-operations:direct-api"},
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (backend *GoogleCloudOperationsBackend) VerifySignal(ctx context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	if ctx == nil {
		return providerobservability.SignalObservation{}, errors.New("Google Cloud observability signal context is required")
	}
	if err := ctx.Err(); err != nil {
		return providerobservability.SignalObservation{}, err
	}
	if strings.TrimSpace(binding.OwnershipMarker) == "" {
		return providerobservability.SignalObservation{}, errors.New("Google Cloud observability signal ownership marker is required")
	}
	observation := providerobservability.SignalObservation{Signal: binding.Signal, Destination: binding.Destination}
	if binding.Destination != googleCloudOperationsDestination {
		observation.Reason = "Google Cloud observability destination does not match the adapter"
		return observation, nil
	}
	if signalKind(binding.Signal) == "logs" {
		entries, err := backend.listLogProbes(ctx, binding)
		if err != nil {
			return providerobservability.SignalObservation{}, fmt.Errorf("Google Cloud log delivery verification failed: %w", err)
		}
		for _, entry := range entries {
			if entry == nil || entry.Labels[ownershipLabel] != markerDigest(binding.OwnershipMarker) {
				continue
			}
			if strings.Contains(entry.TextPayload, probeMessage(binding.OwnershipMarker, binding.Signal)) {
				observation.Delivered = true
				observation.LabelsVerified = true
				break
			}
		}
	} else {
		filter := fmt.Sprintf(`metric.type = %s AND metric.labels.%s = %s`, quote(metricType(binding.OwnershipMarker, binding.Signal)), ownershipLabel, quote(markerDigest(binding.OwnershipMarker)))
		series, err := backend.monitoring.ListTimeSeries(ctx, backend.project, filter, backend.now().Add(-10*time.Minute), backend.now().Add(2*time.Minute))
		if err != nil {
			return providerobservability.SignalObservation{}, fmt.Errorf("Google Cloud metric delivery verification failed: %w", err)
		}
		for _, item := range series {
			if item == nil || item.Metric == nil || item.Metric.Labels[ownershipLabel] != markerDigest(binding.OwnershipMarker) {
				continue
			}
			if len(item.Points) > 0 {
				observation.Delivered = true
				observation.LabelsVerified = true
				break
			}
		}
	}
	observation.RetentionVerified = binding.RetentionDays == 0
	retentionReason := "Google Cloud log retention bucket or routing sink does not match the plan"
	if signalKind(binding.Signal) == "logs" && binding.RetentionDays > 0 {
		verified, reason, err := backend.verifyLogRetention(ctx, binding.OwnershipMarker, binding.RetentionDays)
		if err != nil {
			return providerobservability.SignalObservation{}, fmt.Errorf("Google Cloud log retention verification failed: %w", err)
		}
		observation.RetentionVerified = verified
		if reason != "" {
			retentionReason = reason
		}
	}
	observation.RedactionVerified = strings.TrimSpace(binding.RedactionPolicy) == ""
	if !observation.Delivered {
		observation.Reason = "Google Cloud observability probe was not found"
	} else if !observation.RetentionVerified {
		observation.Reason = retentionReason
	} else if !observation.RedactionVerified {
		observation.Reason = "Google Cloud redaction policy is not managed by this lifecycle adapter"
	}
	return observation, nil
}

// PublishSignalProbe implements the optional acceptance-only data-plane seam.
// Google Cloud log routing is eventually consistent after a new sink is
// created, so a caller may republish the same marker-scoped probe while it
// waits. It never creates or updates a dashboard, alert, bucket, sink, or
// metric descriptor.
func (backend *GoogleCloudOperationsBackend) PublishSignalProbe(ctx context.Context, binding providerobservability.SignalBinding) error {
	if ctx == nil {
		return errors.New("Google Cloud observability signal publication context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(binding.OwnershipMarker) == "" || binding.Destination != googleCloudOperationsDestination {
		return errors.New("Google Cloud observability signal publication identity does not match the adapter")
	}
	if signalKind(binding.Signal) == "unsupported" {
		return fmt.Errorf("Google Cloud observability signal %q is not supported by the adapter", binding.Signal)
	}
	if signalKind(binding.Signal) == "logs" {
		return backend.writeLogProbe(ctx, binding)
	}
	return backend.writeMetricProbe(ctx, binding)
}

func (backend *GoogleCloudOperationsBackend) VerifyOperations(ctx context.Context, plan providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	observation := providerobservability.OperationalObservation{AlertsVerified: len(plan.Alerts) == 0, DashboardsVerified: len(plan.Dashboards) == 0, SLOsVerified: len(plan.SLOs) == 0}
	if len(plan.Dashboards) > 0 {
		observation.DashboardsVerified = true
		dashboards, err := backend.dashboards.ListDashboards(ctx, backend.project)
		if err != nil {
			return providerobservability.OperationalObservation{}, errors.New("inventory Google Cloud dashboards failed")
		}
		for _, intent := range plan.Dashboards {
			resource := findDashboard(dashboards, plan.OwnershipMarker, intent.ID)
			if resource == nil || resource.Name == "" {
				observation.DashboardsVerified = false
				continue
			}
			if _, err := backend.dashboards.GetDashboard(ctx, resource.Name); err != nil {
				observation.DashboardsVerified = false
			}
		}
	}
	if len(plan.Alerts) > 0 {
		observation.AlertsVerified = true
		policies, err := backend.monitoring.ListAlertPolicies(ctx, backend.project)
		if err != nil {
			return providerobservability.OperationalObservation{}, errors.New("inventory Google Cloud alert policies failed")
		}
		for _, intent := range plan.Alerts {
			resource := findAlertPolicy(policies, plan.OwnershipMarker, intent.ID)
			if resource == nil || resource.Name == "" {
				observation.AlertsVerified = false
				continue
			}
			if _, err := backend.monitoring.GetAlertPolicy(ctx, resource.Name); err != nil {
				observation.AlertsVerified = false
			}
		}
	}
	if len(plan.SLOs) > 0 {
		observation.SLOsVerified = true
		slos, err := backend.slos.ListServiceLevelObjectives(ctx, plan.NativeReference)
		if err != nil {
			return providerobservability.OperationalObservation{}, errors.New("inventory Google Cloud SLOs failed")
		}
		for _, intent := range plan.SLOs {
			resource := findSLO(slos, plan.OwnershipMarker, intent.ID)
			if resource == nil || resource.Name == "" {
				observation.SLOsVerified = false
				continue
			}
			actual, err := backend.slos.GetServiceLevelObjective(ctx, resource.Name)
			if err != nil || !googleSLOMatches(actual, plan.NativeReference, plan.OwnershipMarker, intent) {
				observation.SLOsVerified = false
			}
		}
		if !observation.SLOsVerified {
			observation.Reason = "Google Cloud SLO ownership or configuration could not be verified"
		}
	}
	return observation, nil
}

func (backend *GoogleCloudOperationsBackend) Destroy(ctx context.Context, plan providerobservability.Plan, _ []string) error {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return err
	}
	dashboards, err := backend.dashboards.ListDashboards(ctx, backend.project)
	if err != nil {
		return errors.New("inventory Google Cloud dashboards for cleanup failed")
	}
	for _, dashboard := range dashboards {
		if dashboard != nil && ownedDashboard(dashboard, plan.OwnershipMarker) && dashboard.Name != "" {
			if err := backend.dashboards.DeleteDashboard(ctx, dashboard.Name); err != nil {
				return errors.New("delete Google Cloud dashboard failed")
			}
		}
	}
	policies, err := backend.monitoring.ListAlertPolicies(ctx, backend.project)
	if err != nil {
		return errors.New("inventory Google Cloud alert policies for cleanup failed")
	}
	for _, policy := range policies {
		if policy != nil && ownedAlertPolicy(policy, plan.OwnershipMarker) && policy.Name != "" {
			if err := backend.monitoring.DeleteAlertPolicy(ctx, policy.Name); err != nil {
				return errors.New("delete Google Cloud alert policy failed")
			}
		}
	}
	if len(plan.SLOs) > 0 {
		slos, err := backend.slos.ListServiceLevelObjectives(ctx, plan.NativeReference)
		if err != nil {
			return errors.New("inventory Google Cloud SLOs for cleanup failed")
		}
		for _, slo := range slos {
			if slo != nil && ownedSLO(slo, plan.OwnershipMarker) && slo.Name != "" {
				if err := backend.slos.DeleteServiceLevelObjective(ctx, slo.Name); err != nil {
					return errors.New("delete Google Cloud SLO failed")
				}
			}
		}
		remaining, err := backend.slos.ListServiceLevelObjectives(ctx, plan.NativeReference)
		if err != nil {
			return errors.New("verify Google Cloud SLO cleanup failed")
		}
		for _, slo := range remaining {
			if ownedSLO(slo, plan.OwnershipMarker) {
				return errors.New("Google Cloud SLO cleanup left owned resources")
			}
		}
	}
	if err := backend.deleteMetricDescriptors(ctx, plan.OwnershipMarker); err != nil {
		return err
	}
	if err := backend.deleteLogRetention(ctx, plan.OwnershipMarker); err != nil {
		return err
	}
	return nil
}

func (backend *GoogleCloudOperationsBackend) Inventory(ctx context.Context, marker string) ([]providerobservability.InventoryResource, error) {
	if ctx == nil || strings.TrimSpace(marker) == "" {
		return nil, errors.New("Google Cloud observability inventory requires context and ownership marker")
	}
	return backend.inventory(ctx, marker, "")
}

// InventoryForPlan supplies the native service parent needed to enumerate
// Google Cloud SLOs. The generic marker-only inventory remains available for
// callers that only need dashboards and alert policies.
func (backend *GoogleCloudOperationsBackend) InventoryForPlan(ctx context.Context, plan providerobservability.Plan) ([]providerobservability.InventoryResource, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return nil, err
	}
	return backend.inventory(ctx, plan.OwnershipMarker, plan.NativeReference)
}

func (backend *GoogleCloudOperationsBackend) inventory(ctx context.Context, marker, serviceParent string) ([]providerobservability.InventoryResource, error) {
	dashboards, err := backend.dashboards.ListDashboards(ctx, backend.project)
	if err != nil {
		return nil, errors.New("inventory Google Cloud dashboards failed")
	}
	policies, err := backend.monitoring.ListAlertPolicies(ctx, backend.project)
	if err != nil {
		return nil, errors.New("inventory Google Cloud alert policies failed")
	}
	metricDescriptors, err := backend.monitoring.ListMetricDescriptors(ctx, backend.project, metricDescriptorFilter(marker))
	if err != nil {
		return nil, errors.New("inventory Google Cloud metric descriptors failed")
	}
	resources := make([]providerobservability.InventoryResource, 0)
	if backend.retentionConfigured() {
		bucket, sink, err := backend.ownedLogRetention(ctx, marker)
		if err != nil {
			return nil, err
		}
		if bucket != nil {
			resources = append(resources, providerobservability.InventoryResource{Identity: "google-cloud-operations:log-bucket:" + bucket.Name, OwnershipMarker: marker, Owned: true, Live: bucket.LifecycleState != "DELETE_REQUESTED"})
		}
		if sink != nil {
			resources = append(resources, providerobservability.InventoryResource{Identity: "google-cloud-operations:log-sink:" + sink.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	for _, dashboard := range dashboards {
		if dashboard != nil && ownedDashboard(dashboard, marker) && dashboard.Name != "" {
			resources = append(resources, providerobservability.InventoryResource{Identity: "google-cloud-operations:dashboard:" + dashboard.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	for _, policy := range policies {
		if policy != nil && ownedAlertPolicy(policy, marker) && policy.Name != "" {
			resources = append(resources, providerobservability.InventoryResource{Identity: "google-cloud-operations:alert-policy:" + policy.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	for _, descriptor := range metricDescriptors {
		if descriptor != nil && metricDescriptorOwned(descriptor, marker) && descriptor.Name != "" {
			resources = append(resources, providerobservability.InventoryResource{Identity: "google-cloud-operations:metric-descriptor:" + descriptor.Name, OwnershipMarker: marker, Owned: true, Live: true})
		}
	}
	if strings.TrimSpace(serviceParent) != "" {
		if backend.slos == nil {
			return nil, errors.New("Google Cloud SLO inventory is not configured")
		}
		slos, err := backend.slos.ListServiceLevelObjectives(ctx, serviceParent)
		if err != nil {
			return nil, errors.New("inventory Google Cloud SLOs failed")
		}
		for _, slo := range slos {
			if slo != nil && ownedSLO(slo, marker) && slo.Name != "" {
				resources = append(resources, providerobservability.InventoryResource{Identity: "google-cloud-operations:slo:" + slo.Name, OwnershipMarker: marker, Owned: true, Live: true})
			}
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (backend *GoogleCloudOperationsBackend) writeLogProbe(ctx context.Context, binding providerobservability.SignalBinding) error {
	request := &loggingv2.WriteLogEntriesRequest{LogName: logName(backend.project, binding.OwnershipMarker), Entries: []*loggingv2.LogEntry{{LogName: logName(backend.project, binding.OwnershipMarker), Labels: ownershipLabels(binding.OwnershipMarker), TextPayload: probeMessage(binding.OwnershipMarker, binding.Signal), Resource: &loggingv2.MonitoredResource{Type: "global", Labels: map[string]string{"project_id": backend.project}}}}}
	if err := backend.logging.Write(ctx, request); err != nil {
		return fmt.Errorf("publish Google Cloud log probe failed: %w", err)
	}
	return nil
}

func (backend *GoogleCloudOperationsBackend) writeMetricProbe(ctx context.Context, binding providerobservability.SignalBinding) error {
	metric := metricType(binding.OwnershipMarker, binding.Signal)
	descriptor, err := backend.monitoring.GetMetricDescriptor(ctx, backend.project, metric)
	if err != nil && !googleNotFound(err) {
		return errors.New("inspect Google Cloud metric descriptor failed")
	}
	if descriptor != nil && !metricDescriptorMatches(descriptor, binding.OwnershipMarker, binding.Signal) {
		return errors.New("refusing to replace a foreign or drifted Google Cloud metric descriptor")
	}
	value := float64(1)
	request := &monitoringv3.CreateTimeSeriesRequest{TimeSeries: []*monitoringv3.TimeSeries{{Metric: &monitoringv3.Metric{Type: metric, Labels: ownershipLabels(binding.OwnershipMarker)}, Resource: &monitoringv3.MonitoredResource{Type: "global", Labels: map[string]string{"project_id": backend.project}}, MetricKind: "GAUGE", Points: []*monitoringv3.Point{{Interval: &monitoringv3.TimeInterval{EndTime: backend.now().UTC().Format(time.RFC3339Nano)}, Value: &monitoringv3.TypedValue{DoubleValue: &value}}}}}}
	if err := backend.monitoring.CreateTimeSeries(ctx, backend.project, request); err != nil {
		return fmt.Errorf("publish Google Cloud metric probe failed: %w", err)
	}
	return nil
}

const googleCloudOperationsDefaultLogLocation = "global"

func (backend *GoogleCloudOperationsBackend) retentionConfigured() bool {
	return backend.logBuckets != nil && backend.logSinks != nil
}

func (backend *GoogleCloudOperationsBackend) ensureLogRetention(ctx context.Context, plan providerobservability.Plan) (bucketName, sinkName string, enabled bool, err error) {
	retentionDays := 0
	for _, binding := range plan.Bindings {
		if signalKind(binding.Signal) == "logs" && binding.RetentionDays > 0 {
			retentionDays = binding.RetentionDays
			break
		}
	}
	if retentionDays == 0 {
		return "", "", false, nil
	}
	if !backend.retentionConfigured() {
		return "", "", false, errors.New("Google Cloud log retention clients are not configured")
	}

	expectedBucket := googleLogBucketFor(backend.project, backend.logLocation, plan.OwnershipMarker, retentionDays)
	expectedSink := googleLogSinkFor(backend.project, backend.logLocation, plan.OwnershipMarker)
	actualBucket, bucketErr := backend.logBuckets.Get(ctx, expectedBucket.Name)
	if bucketErr != nil && !googleNotFound(bucketErr) {
		return "", "", false, errors.New("inspect Google Cloud log retention bucket failed")
	}
	actualSink, sinkErr := backend.logSinks.Get(ctx, expectedSink.Name)
	if sinkErr != nil && !googleNotFound(sinkErr) {
		return "", "", false, errors.New("inspect Google Cloud log routing sink failed")
	}
	if actualBucket != nil && !googleLogBucketOwned(actualBucket, expectedBucket) {
		return "", "", false, errors.New("Google Cloud log retention bucket collides with a foreign or drifted resource")
	}
	if actualSink != nil && !googleLogSinkOwned(actualSink, expectedSink) {
		return "", "", false, errors.New("Google Cloud log routing sink collides with a foreign or drifted resource")
	}

	if actualBucket != nil {
		if actualBucket.Locked {
			return "", "", false, errors.New("Google Cloud log retention bucket is locked and cannot be safely managed or cleaned up")
		}
		if actualBucket.LifecycleState != "" && actualBucket.LifecycleState != "ACTIVE" {
			return "", "", false, fmt.Errorf("Google Cloud log retention bucket is not active: %s", actualBucket.LifecycleState)
		}
		if actualBucket.RetentionDays != retentionDays {
			actualBucket, err = backend.logBuckets.Patch(ctx, expectedBucket.Name, expectedBucket)
			if err != nil {
				return "", "", false, errors.New("update Google Cloud log retention bucket failed")
			}
			if !googleLogBucketMatches(actualBucket, expectedBucket) {
				return "", "", false, errors.New("Google Cloud log retention bucket update did not converge to the requested configuration")
			}
		}
	} else {
		actualBucket, err = backend.logBuckets.Create(ctx, googleLogBucketParent(backend.project, backend.logLocation), googleLogBucketID(plan.OwnershipMarker), expectedBucket)
		if err != nil {
			return "", "", false, errors.New("create Google Cloud log retention bucket failed")
		}
		if !googleLogBucketMatches(actualBucket, expectedBucket) {
			return "", "", false, errors.New("Google Cloud log retention bucket create did not return the expected owned resource")
		}
	}

	if actualSink != nil {
		if !googleLogSinkMatches(actualSink, expectedSink) {
			actualSink, err = backend.logSinks.Patch(ctx, expectedSink.Name, expectedSink)
			if err != nil {
				return "", "", false, errors.New("update Google Cloud log routing sink failed")
			}
			if !googleLogSinkMatches(actualSink, expectedSink) {
				return "", "", false, errors.New("Google Cloud log routing sink update did not converge to the requested configuration")
			}
		}
	} else {
		actualSink, err = backend.logSinks.Create(ctx, backend.project, expectedSink)
		if err != nil {
			return "", "", false, errors.New("create Google Cloud log routing sink failed")
		}
		if !googleLogSinkMatches(actualSink, expectedSink) {
			return "", "", false, errors.New("Google Cloud log routing sink create did not return the expected owned resource")
		}
	}
	return expectedBucket.Name, expectedSink.Name, true, nil
}

func (backend *GoogleCloudOperationsBackend) verifyLogRetention(ctx context.Context, marker string, retentionDays int) (bool, string, error) {
	if !backend.retentionConfigured() {
		return false, "", errors.New("Google Cloud log retention clients are not configured")
	}
	expectedBucket := googleLogBucketFor(backend.project, backend.logLocation, marker, retentionDays)
	expectedSink := googleLogSinkFor(backend.project, backend.logLocation, marker)
	bucket, sink, err := backend.ownedLogRetention(ctx, marker)
	if err != nil {
		return false, "", err
	}
	if bucket == nil {
		return false, "Google Cloud managed log-retention bucket is missing", nil
	}
	if !googleLogBucketMatches(bucket, expectedBucket) {
		return false, fmt.Sprintf("Google Cloud managed log-retention bucket configuration mismatch: lifecycle=%q retentionDays=%d locked=%t", bucket.LifecycleState, bucket.RetentionDays, bucket.Locked), nil
	}
	if sink == nil {
		return false, "Google Cloud managed log-routing sink is missing", nil
	}
	if !googleLogSinkMatches(sink, expectedSink) {
		return false, fmt.Sprintf("Google Cloud managed log-routing sink configuration mismatch: destination=%q filter=%q disabled=%t", sink.Destination, sink.Filter, sink.Disabled), nil
	}
	entries, err := backend.listLogProbesInResource(ctx, marker, googleLogBucketViewName(bucket.Name))
	if err != nil {
		return false, "", err
	}
	for _, entry := range entries {
		if entry != nil && entry.Labels[ownershipLabel] == markerDigest(marker) && strings.HasPrefix(entry.TextPayload, "magelift-probe signal=") {
			return true, "", nil
		}
	}
	return false, "Google Cloud managed log bucket and sink match, but the routed probe is not visible in the bucket _AllLogs view", nil
}

func (backend *GoogleCloudOperationsBackend) ownedLogRetention(ctx context.Context, marker string) (*GoogleLogBucket, *GoogleLogSink, error) {
	if !backend.retentionConfigured() {
		return nil, nil, nil
	}
	expectedBucket := googleLogBucketFor(backend.project, backend.logLocation, marker, 0)
	expectedSink := googleLogSinkFor(backend.project, backend.logLocation, marker)
	bucket, err := backend.logBuckets.Get(ctx, expectedBucket.Name)
	if err != nil && !googleNotFound(err) {
		return nil, nil, errors.New("inventory Google Cloud log retention bucket failed")
	}
	sink, sinkErr := backend.logSinks.Get(ctx, expectedSink.Name)
	if sinkErr != nil && !googleNotFound(sinkErr) {
		return nil, nil, errors.New("inventory Google Cloud log routing sink failed")
	}
	if bucket != nil && !googleLogBucketOwned(bucket, expectedBucket) {
		return nil, nil, errors.New("Google Cloud log retention bucket is not owned by this marker")
	}
	if sink != nil && !googleLogSinkOwned(sink, expectedSink) {
		return nil, nil, errors.New("Google Cloud log routing sink is not owned by this marker")
	}
	return bucket, sink, nil
}

func (backend *GoogleCloudOperationsBackend) deleteLogRetention(ctx context.Context, marker string) error {
	if !backend.retentionConfigured() {
		return nil
	}
	bucket, sink, err := backend.ownedLogRetention(ctx, marker)
	if err != nil {
		return err
	}
	if sink != nil {
		if err := backend.logSinks.Delete(ctx, sink.Name); err != nil && !googleNotFound(err) {
			return errors.New("delete Google Cloud log routing sink failed")
		}
		if err := waitForGoogleLogGone(ctx, func(ctx context.Context) error {
			_, err := backend.logSinks.Get(ctx, sink.Name)
			return err
		}); err != nil {
			return errors.New("verify Google Cloud log routing sink cleanup failed")
		}
	}
	if bucket != nil {
		if bucket.LifecycleState == "DELETE_REQUESTED" {
			return nil
		}
		if bucket.Locked {
			return errors.New("Google Cloud log retention bucket is locked and cannot be cleaned up")
		}
		if err := backend.logBuckets.Delete(ctx, bucket.Name); err != nil && !googleNotFound(err) {
			return errors.New("delete Google Cloud log retention bucket failed")
		}
		if err := waitForGoogleLogBucketDeletion(ctx, backend.logBuckets, bucket.Name); err != nil {
			return errors.New("verify Google Cloud log retention bucket cleanup failed")
		}
	}
	return nil
}

func waitForGoogleLogGone(ctx context.Context, get func(context.Context) error) error {
	if err := get(ctx); googleNotFound(err) {
		return nil
	} else if err != nil {
		return err
	}
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("Google Cloud log resource deletion did not converge")
		case <-ticker.C:
			if err := get(ctx); googleNotFound(err) {
				return nil
			} else if err != nil {
				return err
			}
		}
	}
}

func waitForGoogleLogBucketDeletion(ctx context.Context, api GoogleLogBucketAPI, name string) error {
	check := func() (bool, error) {
		bucket, err := api.Get(ctx, name)
		if googleNotFound(err) {
			return true, nil
		}
		if err != nil {
			return false, err
		}
		// Cloud Logging intentionally retains an unlocked deleted bucket in
		// DELETE_REQUESTED for seven days. It no longer accepts live writes once
		// the owned sink is removed, so this is the provider's terminal cleanup
		// state rather than a reason to wait for a week.
		return bucket != nil && bucket.LifecycleState == "DELETE_REQUESTED", nil
	}
	if done, err := check(); err != nil || done {
		return err
	}
	timer := time.NewTimer(90 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("Google Cloud log bucket deletion did not converge")
		case <-ticker.C:
			done, err := check()
			if err != nil {
				return err
			}
			if done {
				return nil
			}
		}
	}
}

func googleLogBucketFor(project, location, marker string, retentionDays int) *GoogleLogBucket {
	return &GoogleLogBucket{Name: googleLogBucketName(project, location, marker), Description: googleLogBucketDescription(marker), RetentionDays: retentionDays}
}

func googleLogSinkFor(project, location, marker string) *GoogleLogSink {
	return &GoogleLogSink{Name: googleLogSinkName(project, marker), Description: googleLogSinkDescription(marker), Destination: "logging.googleapis.com/" + googleLogBucketName(project, location, marker), Filter: `logName = ` + quote(logName(project, marker))}
}

func googleLogBucketOwned(actual, expected *GoogleLogBucket) bool {
	return actual != nil && expected != nil && actual.Name == expected.Name && actual.Description == expected.Description
}

func googleLogBucketMatches(actual, expected *GoogleLogBucket) bool {
	return googleLogBucketOwned(actual, expected) && !actual.Locked && (actual.LifecycleState == "" || actual.LifecycleState == "ACTIVE") && (expected.RetentionDays == 0 || actual.RetentionDays == expected.RetentionDays)
}

func googleLogSinkOwned(actual, expected *GoogleLogSink) bool {
	return actual != nil && expected != nil && actual.Name == expected.Name && actual.Description == expected.Description
}

func googleLogSinkMatches(actual, expected *GoogleLogSink) bool {
	return googleLogSinkOwned(actual, expected) && actual.Destination == expected.Destination && actual.Filter == expected.Filter && !actual.Disabled
}

func (backend *GoogleCloudOperationsBackend) deleteMetricDescriptors(ctx context.Context, marker string) error {
	descriptors, err := backend.monitoring.ListMetricDescriptors(ctx, backend.project, metricDescriptorFilter(marker))
	if err != nil {
		return errors.New("inventory Google Cloud metric descriptors for cleanup failed")
	}
	for _, descriptor := range descriptors {
		if descriptor == nil || !metricDescriptorOwned(descriptor, marker) || descriptor.Name == "" {
			continue
		}
		if err := backend.monitoring.DeleteMetricDescriptor(ctx, descriptor.Name); err != nil && !googleNotFound(err) {
			return errors.New("delete Google Cloud metric descriptor failed")
		}
	}
	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		remaining, err := backend.monitoring.ListMetricDescriptors(ctx, backend.project, metricDescriptorFilter(marker))
		if err != nil {
			return errors.New("verify Google Cloud metric descriptor cleanup failed")
		}
		ownedRemaining := false
		for _, descriptor := range remaining {
			if metricDescriptorOwned(descriptor, marker) {
				ownedRemaining = true
				break
			}
		}
		if !ownedRemaining {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("Google Cloud metric descriptor cleanup left owned resources")
		case <-ticker.C:
		}
	}
}

func (backend *GoogleCloudOperationsBackend) listLogProbes(ctx context.Context, binding providerobservability.SignalBinding) ([]*loggingv2.LogEntry, error) {
	return backend.listLogProbesInResource(ctx, binding.OwnershipMarker, projectName(backend.project))
}

func (backend *GoogleCloudOperationsBackend) listLogProbesInResource(ctx context.Context, marker, resourceName string) ([]*loggingv2.LogEntry, error) {
	filter := fmt.Sprintf(`logName = %s AND labels.%s = %s AND textPayload:%s`, quote(logName(backend.project, marker)), ownershipLabel, quote(markerDigest(marker)), quote("magelift-probe signal="))
	var entries []*loggingv2.LogEntry
	pageToken := ""
	for {
		request := &loggingv2.ListLogEntriesRequest{ResourceNames: []string{resourceName}, Filter: filter, OrderBy: "timestamp desc", PageSize: 100}
		if pageToken != "" {
			request.PageToken = pageToken
		}
		response, err := backend.logging.List(ctx, request)
		if err != nil {
			return nil, err
		}
		entries = append(entries, response.Entries...)
		if response.NextPageToken == "" {
			return entries, nil
		}
		pageToken = response.NextPageToken
	}
}

func (backend *GoogleCloudOperationsBackend) ensureDashboard(ctx context.Context, marker string, intent v1.DashboardIntent) (*monitoringv1.Dashboard, error) {
	dashboards, err := backend.dashboards.ListDashboards(ctx, backend.project)
	if err != nil {
		return nil, errors.New("inventory Google Cloud dashboards before apply failed")
	}
	if existing := findDashboard(dashboards, marker, intent.ID); existing != nil {
		return existing, nil
	}
	dashboard := &monitoringv1.Dashboard{DisplayName: dashboardName(marker, intent.ID), Labels: ownershipLabels(marker), GridLayout: &monitoringv1.GridLayout{Columns: 1, Widgets: []*monitoringv1.Widget{{Title: "MageLift observability", Text: &monitoringv1.Text{Content: "Signals: " + strings.Join(v1.SortedStrings(intent.Signals), ", "), Format: "MARKDOWN"}}}}}
	created, err := backend.dashboards.CreateDashboard(ctx, backend.project, dashboard)
	if err != nil {
		return nil, errors.New("create Google Cloud dashboard failed")
	}
	return created, nil
}

func (backend *GoogleCloudOperationsBackend) ensureAlertPolicy(ctx context.Context, marker string, intent v1.AlertIntent) (*monitoringv3.AlertPolicy, error) {
	policies, err := backend.monitoring.ListAlertPolicies(ctx, backend.project)
	if err != nil {
		return nil, errors.New("inventory Google Cloud alert policies before apply failed")
	}
	if existing := findAlertPolicy(policies, marker, intent.ID); existing != nil {
		if !googleAlertPolicyMatches(existing, marker, intent) {
			return nil, fmt.Errorf("refusing to reuse drifted Google Cloud alert policy %q", intent.ID)
		}
		return existing, nil
	}
	condition, err := alertCondition(marker, intent)
	if err != nil {
		return nil, err
	}
	policy := &monitoringv3.AlertPolicy{DisplayName: alertPolicyName(marker, intent.ID), Combiner: "OR", Enabled: true, Conditions: []*monitoringv3.Condition{condition}, Documentation: googleAlertDocumentation(intent), Severity: googleAlertSeverity(intent.Severity), UserLabels: googleAlertLabels(marker, intent)}
	created, err := backend.monitoring.CreateAlertPolicy(ctx, backend.project, policy)
	if err != nil {
		return nil, errors.New("create Google Cloud alert policy failed")
	}
	return created, nil
}

func (backend *GoogleCloudOperationsBackend) ensureSLO(ctx context.Context, parent, marker string, intent v1.SLOIntent) (*monitoringv3.ServiceLevelObjective, error) {
	if backend.slos == nil {
		return nil, errors.New("Google Cloud SLO lifecycle requires the Service Monitoring API")
	}
	if _, err := googleSLOServiceID(backend.project, parent); err != nil {
		return nil, err
	}
	slos, err := backend.slos.ListServiceLevelObjectives(ctx, parent)
	if err != nil {
		return nil, errors.New("inventory Google Cloud SLOs before apply failed")
	}
	displayName := googleSLODisplayName(marker, intent.ID)
	id := googleSLOID(marker, intent.ID)
	for _, existing := range slos {
		if existing == nil {
			continue
		}
		identityCollision := strings.HasSuffix(existing.Name, "/serviceLevelObjectives/"+id) || existing.DisplayName == displayName
		if !identityCollision {
			continue
		}
		if !ownedSLO(existing, marker) {
			return nil, fmt.Errorf("Google Cloud SLO %q collides with an unowned resource", intent.ID)
		}
		if !googleSLOMatches(existing, parent, marker, intent) {
			return nil, fmt.Errorf("Google Cloud SLO %q has configuration drift", intent.ID)
		}
		return existing, nil
	}
	objective := googleSLOObjective(marker, intent)
	created, err := backend.slos.CreateServiceLevelObjective(ctx, parent, id, objective)
	if err != nil {
		return nil, errors.New("create Google Cloud SLO failed")
	}
	if created == nil || created.Name == "" || !googleSLOMatches(created, parent, marker, intent) {
		return nil, errors.New("Google Cloud SLO create did not return the owned expected resource")
	}
	return created, nil
}

func validateGoogleSLO(intent v1.SLOIntent) error {
	if intent.Signal != "application-health" {
		return fmt.Errorf("Google Cloud SLO %q supports only application-health", intent.ID)
	}
	if intent.Target <= 0 || intent.Target > 0.9999 {
		return fmt.Errorf("Google Cloud SLO %q target must be in (0, 0.9999]", intent.ID)
	}
	const day = int64(24 * time.Hour / time.Second)
	if intent.WindowSeconds <= 0 || intent.WindowSeconds%day != 0 || intent.WindowSeconds/day > 30 {
		return fmt.Errorf("Google Cloud SLO %q rolling window must be a whole number of days from 1 to 30", intent.ID)
	}
	return nil
}

func googleSLOServiceID(project, parent string) (string, error) {
	parts := strings.Split(strings.TrimSpace(parent), "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "services" {
		return "", errors.New("Google Cloud SLO native reference must be projects/<project>/services/<service>")
	}
	if parts[1] != strings.TrimSpace(project) {
		return "", errors.New("Google Cloud SLO native reference belongs to a different project")
	}
	if parts[3] == "" || strings.ContainsAny(parts[3], " \t\r\n\x00/") {
		return "", errors.New("Google Cloud SLO native reference has an invalid service identity")
	}
	return parts[3], nil
}

func googleSLOObjective(marker string, intent v1.SLOIntent) *monitoringv3.ServiceLevelObjective {
	return &monitoringv3.ServiceLevelObjective{
		DisplayName:           googleSLODisplayName(marker, intent.ID),
		Goal:                  intent.Target,
		RollingPeriod:         fmt.Sprintf("%ds", intent.WindowSeconds),
		ServiceLevelIndicator: &monitoringv3.ServiceLevelIndicator{BasicSli: &monitoringv3.BasicSli{Availability: &monitoringv3.AvailabilityCriteria{}}},
		UserLabels:            ownershipLabels(marker),
	}
}

func findSLO(slos []*monitoringv3.ServiceLevelObjective, marker, id string) *monitoringv3.ServiceLevelObjective {
	for _, slo := range slos {
		if slo != nil && ownedSLO(slo, marker) && slo.DisplayName == googleSLODisplayName(marker, id) {
			return slo
		}
	}
	return nil
}

func googleSLOMatches(slo *monitoringv3.ServiceLevelObjective, parent, marker string, intent v1.SLOIntent) bool {
	if slo == nil || !ownedSLO(slo, marker) || slo.DisplayName != googleSLODisplayName(marker, intent.ID) || slo.Name != googleSLOResourceName(parent, marker, intent.ID) {
		return false
	}
	if _, err := googleSLOServiceIDFromParent(parent); err != nil {
		return false
	}
	if slo.Goal != intent.Target || slo.RollingPeriod != fmt.Sprintf("%ds", intent.WindowSeconds) {
		return false
	}
	return slo.CalendarPeriod == "" && slo.ServiceLevelIndicator != nil && slo.ServiceLevelIndicator.BasicSli != nil && slo.ServiceLevelIndicator.BasicSli.Availability != nil
}

func googleSLOServiceIDFromParent(parent string) (string, error) {
	parts := strings.Split(strings.TrimSpace(parent), "/")
	if len(parts) != 4 || parts[0] != "projects" || parts[2] != "services" || parts[1] == "" || parts[3] == "" || strings.ContainsAny(parts[1]+parts[3], " \t\r\n\x00") {
		return "", errors.New("Google Cloud SLO resource has an invalid service parent")
	}
	return parts[3], nil
}

func googleSLOResourceName(parent, marker, id string) string {
	return strings.TrimSuffix(strings.TrimSpace(parent), "/") + "/serviceLevelObjectives/" + googleSLOID(marker, id)
}

func ownedSLO(slo *monitoringv3.ServiceLevelObjective, marker string) bool {
	return slo != nil && slo.UserLabels[ownershipLabel] == markerDigest(marker)
}

func googleSLOID(marker, id string) string {
	return "magelift-" + markerDigest(marker) + "-" + markerDigest(id)
}

func googleSLODisplayName(marker, id string) string {
	return "MageLift " + markerDigest(marker) + " SLO " + markerDigest(id)
}

func alertCondition(marker string, intent v1.AlertIntent) (*monitoringv3.Condition, error) {
	if intent.WindowSeconds <= 0 || intent.WindowSeconds%60 != 0 {
		return nil, fmt.Errorf("Google Cloud alert %q window must be a whole number of minutes", intent.ID)
	}
	if signalKind(intent.Signal) == "logs" {
		if intent.Operator != "gt" || intent.Threshold != 0 {
			return nil, fmt.Errorf("Google Cloud log alert %q requires operator gt and threshold 0", intent.ID)
		}
		return &monitoringv3.Condition{DisplayName: intent.ID, ConditionMatchedLog: &monitoringv3.LogMatch{Filter: fmt.Sprintf(`labels.%s = %s`, ownershipLabel, quote(markerDigest(marker)))}}, nil
	}
	comparison, ok := map[string]string{"gt": "COMPARISON_GT", "lt": "COMPARISON_LT"}[intent.Operator]
	if !ok {
		return nil, fmt.Errorf("Google Cloud metric alert %q does not support operator %q", intent.ID, intent.Operator)
	}
	return &monitoringv3.Condition{DisplayName: intent.ID, ConditionThreshold: &monitoringv3.MetricThreshold{Filter: fmt.Sprintf(`resource.type = "global" AND metric.type = %s AND metric.labels.%s = %s`, quote(metricType(marker, intent.Signal)), ownershipLabel, quote(markerDigest(marker))), Comparison: comparison, ThresholdValue: intent.Threshold, Duration: fmt.Sprintf("%ds", intent.WindowSeconds), EvaluationMissingData: "EVALUATION_MISSING_DATA_NO_OP"}}, nil
}

func (backend *GoogleCloudOperationsBackend) validatePlan(ctx context.Context, plan providerobservability.Plan) error {
	if ctx == nil {
		return errors.New("Google Cloud observability context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(backend.project) == "" || strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("Google Cloud observability project and ownership marker are required")
	}
	retentionDays := 0
	for _, binding := range plan.Bindings {
		if binding.Destination != googleCloudOperationsDestination || binding.OwnershipMarker != plan.OwnershipMarker {
			return errors.New("Google Cloud observability binding ownership or destination does not match the plan")
		}
		if binding.RetentionDays > 0 {
			if signalKind(binding.Signal) != "logs" {
				return fmt.Errorf("Google Cloud observability metric retention for %q requires a provider-managed metric policy; this adapter supports retention only for logs", binding.Signal)
			}
			if retentionDays == 0 {
				retentionDays = binding.RetentionDays
			} else if retentionDays != binding.RetentionDays {
				return errors.New("Google Cloud observability log bindings must use one retention period because they share one managed bucket")
			}
		}
		if strings.TrimSpace(binding.RedactionPolicy) != "" {
			return errors.New("Google Cloud observability redaction requires a managed data-protection policy")
		}
		if signalKind(binding.Signal) == "unsupported" {
			return fmt.Errorf("Google Cloud observability signal %q is not implemented by this adapter", binding.Signal)
		}
	}
	if retentionDays > 0 && !backend.retentionConfigured() {
		return errors.New("Google Cloud observability log retention requires managed log bucket and sink clients")
	}
	if len(plan.SLOs) > 0 {
		if backend.slos == nil {
			return errors.New("Google Cloud SLO lifecycle requires the Service Monitoring API")
		}
		if _, err := googleSLOServiceID(backend.project, plan.NativeReference); err != nil {
			return err
		}
		for _, intent := range plan.SLOs {
			if err := validateGoogleSLO(intent); err != nil {
				return err
			}
		}
	}
	for _, intent := range plan.Alerts {
		if err := v1.ValidateObservabilityIntent(v1.ObservabilityIntent{NativeProvider: googleCloudOperationsDestination, OwnershipMarker: plan.OwnershipMarker, Signals: []string{intent.Signal}, Alerts: []v1.AlertIntent{intent}}); err != nil {
			return fmt.Errorf("validate Google Cloud alert %q: %w", intent.ID, err)
		}
		if _, err := alertCondition(plan.OwnershipMarker, intent); err != nil {
			return err
		}
	}
	return nil
}

func findDashboard(dashboards []*monitoringv1.Dashboard, marker, id string) *monitoringv1.Dashboard {
	for _, dashboard := range dashboards {
		if dashboard != nil && ownedDashboard(dashboard, marker) && dashboard.DisplayName == dashboardName(marker, id) {
			return dashboard
		}
	}
	return nil
}

func findAlertPolicy(policies []*monitoringv3.AlertPolicy, marker, id string) *monitoringv3.AlertPolicy {
	for _, policy := range policies {
		if policy != nil && ownedAlertPolicy(policy, marker) && policy.DisplayName == alertPolicyName(marker, id) {
			return policy
		}
	}
	return nil
}

func ownedDashboard(dashboard *monitoringv1.Dashboard, marker string) bool {
	return dashboard != nil && dashboard.Labels[ownershipLabel] == markerDigest(marker)
}

func ownedAlertPolicy(policy *monitoringv3.AlertPolicy, marker string) bool {
	return policy != nil && policy.UserLabels[ownershipLabel] == markerDigest(marker)
}

func googleAlertLabels(marker string, intent v1.AlertIntent) map[string]string {
	labels := ownershipLabels(marker)
	labels["magelift_owner"] = googleLabelValue(intent.Owner)
	labels["magelift_deduplication"] = googleLabelValue(intent.DeduplicationKey)
	return labels
}

func googleLabelValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' || char == '-' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	value = strings.Trim(builder.String(), "-")
	if value == "" {
		return "value-" + markerDigest(value)
	}
	if len(value) <= 63 {
		return value
	}
	return value[:46] + "-" + markerDigest(value)
}

func googleAlertDocumentation(intent v1.AlertIntent) *monitoringv3.Documentation {
	documentation := &monitoringv3.Documentation{Content: fmt.Sprintf("MageLift alert %s. Owner: %s. Deduplication key: %s.", intent.ID, intent.Owner, intent.DeduplicationKey), MimeType: "text/markdown"}
	if intent.RunbookURL != "" {
		documentation.Links = []*monitoringv3.Link{{DisplayName: "runbook", Url: intent.RunbookURL}}
	}
	return documentation
}

func googleAlertPolicyMatches(policy *monitoringv3.AlertPolicy, marker string, intent v1.AlertIntent) bool {
	if !ownedAlertPolicy(policy, marker) || policy.Severity != googleAlertSeverity(intent.Severity) {
		return false
	}
	wantLabels := googleAlertLabels(marker, intent)
	for key, value := range wantLabels {
		if policy.UserLabels[key] != value {
			return false
		}
	}
	if policy.Documentation == nil || !strings.Contains(policy.Documentation.Content, "Owner: "+intent.Owner) || !strings.Contains(policy.Documentation.Content, "Deduplication key: "+intent.DeduplicationKey) {
		return false
	}
	for _, link := range policy.Documentation.Links {
		if link != nil && link.Url == intent.RunbookURL {
			return true
		}
	}
	return intent.RunbookURL == ""
}

func ownershipLabels(marker string) map[string]string {
	return map[string]string{ownershipLabel: markerDigest(marker), managedLabel: "true"}
}

func resourceName(name, fallback string) string {
	if strings.TrimSpace(name) != "" {
		return name
	}
	return fallback
}

func validGoogleLogLocation(location string) bool {
	return strings.TrimSpace(location) != "" && !strings.ContainsAny(location, "/\r\n\x00")
}

func googleLogBucketID(marker string) string {
	return "magelift-" + markerDigest(marker)
}

func googleLogBucketName(project, location, marker string) string {
	return projectName(project) + "/locations/" + strings.TrimSpace(location) + "/buckets/" + googleLogBucketID(marker)
}

func googleLogBucketParent(project, location string) string {
	return projectName(project) + "/locations/" + strings.TrimSpace(location)
}

func googleLogBucketViewName(bucketName string) string {
	return strings.TrimSuffix(strings.TrimSpace(bucketName), "/") + "/views/_AllLogs"
}

func googleLogBucketDescription(marker string) string {
	return "MageLift managed observability retention " + markerDigest(marker)
}

func googleLogSinkID(marker string) string {
	return "magelift-" + markerDigest(marker)
}

func googleLogSinkName(project, marker string) string {
	return projectName(project) + "/sinks/" + googleLogSinkID(marker)
}

func googleLogSinkDescription(marker string) string {
	return "MageLift managed observability routing " + markerDigest(marker)
}

func googleLogSinkIDFromResourceName(name string) string {
	parts := strings.Split(strings.TrimSpace(name), "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return strings.TrimSpace(name)
}

func googleLogSinkProject(name string) string {
	parts := strings.Split(strings.TrimSpace(name), "/")
	if len(parts) >= 2 && parts[0] == "projects" {
		return parts[1]
	}
	return ""
}

const (
	ownershipLabel = "magelift_ownership"
	managedLabel   = "magelift_managed"
)

func projectName(project string) string { return "projects/" + strings.TrimSpace(project) }

func markerDigest(marker string) string {
	sum := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(sum[:])[:16]
}

func logName(project, marker string) string {
	return projectName(project) + "/logs/magelift-observability-" + markerDigest(marker)
}

func metricType(marker, signal string) string {
	signal = strings.ToLower(strings.TrimSpace(signal))
	signal = strings.NewReplacer("-", "_", ".", "_", "/", "_").Replace(signal)
	return "custom.googleapis.com/magelift/" + markerDigest(marker) + "/" + signal
}

func metricDescriptorName(project, metric string) string {
	return projectName(project) + "/metricDescriptors/" + metric
}

func metricDescriptorFilter(marker string) string {
	return `metric.type = starts_with("custom.googleapis.com/magelift/` + markerDigest(marker) + `/")`
}

func metricDescriptorOwned(descriptor *monitoringv3.MetricDescriptor, marker string) bool {
	if descriptor == nil || !strings.HasPrefix(descriptor.Type, "custom.googleapis.com/magelift/"+markerDigest(marker)+"/") {
		return false
	}
	hasOwnership, hasManaged := false, false
	for _, label := range descriptor.Labels {
		if label == nil {
			continue
		}
		switch label.Key {
		case ownershipLabel:
			hasOwnership = true
		case managedLabel:
			hasManaged = true
		}
	}
	return hasOwnership && hasManaged
}

func metricDescriptorMatches(descriptor *monitoringv3.MetricDescriptor, marker, signal string) bool {
	return metricDescriptorOwned(descriptor, marker) && descriptor.Type == metricType(marker, signal) && descriptor.MetricKind == "GAUGE" && descriptor.ValueType == "DOUBLE"
}

func googleNotFound(err error) bool {
	var apiErr *googleapi.Error
	return errors.As(err, &apiErr) && apiErr.Code == 404
}

func dashboardName(marker, id string) string {
	return "MageLift " + markerDigest(marker) + " Dashboard " + markerDigest(id)
}

func alertPolicyName(marker, id string) string {
	return "MageLift " + markerDigest(marker) + " Alert " + markerDigest(id)
}

func signalKind(signal string) string {
	switch signal {
	case "logs", "audit-events", "provider-operations", "cleanup-failure":
		return "logs"
	case "traces":
		return "unsupported"
	default:
		return "metrics"
	}
}

func probeMessage(marker, signal string) string {
	return "magelift-probe signal=" + signal + " marker=" + markerDigest(marker)
}

func googleAlertSeverity(severity string) string {
	switch severity {
	case "critical":
		return "CRITICAL"
	case "warning":
		return "WARNING"
	default:
		return "ERROR"
	}
}

func quote(value string) string { return strconv.Quote(value) }
