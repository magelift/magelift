package newrelic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
	logsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracesv1 "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// OTLPExporter is the only New Relic data-plane dependency needed by the
// shared lifecycle backend. Client credentials remain inside Client.Export.
type OTLPExporter interface {
	Export(context.Context, ExportRequest) (ExportResult, error)
}

// OTLPLifecycleBackend proves acceptance by New Relic's OTLP endpoint and,
// when configured, delayed queryable delivery through a provider-owned NRQL
// verifier. OTLP ingestion does not create a deletable MageLift-owned object;
// cleanup therefore inventories no provider resource and never revokes the
// ingest credential as a side effect.
type OTLPLifecycleBackend struct {
	exporter   OTLPExporter
	query      MarkerQueryer
	operations *NerdGraphOperations
	now        func() time.Time
}

var _ providerobservability.Backend = (*OTLPLifecycleBackend)(nil)

// NewOTLPLifecycleClient creates the shared lifecycle client around a
// provider-owned OTLP exporter. Callers that need delivery proof can inject an
// NRQL verifier with NewOTLPLifecycleClientWithQuery.
func NewOTLPLifecycleClient(exporter OTLPExporter) (*providerobservability.ManagedLifecycleClient, error) {
	return NewOTLPLifecycleClientWithQuery(exporter, nil)
}

// NewOTLPLifecycleClientWithQuery adds provider-owned queryable delivery
// verification without putting NerdGraph or NRQL concepts into the shared
// observability contract.
func NewOTLPLifecycleClientWithQuery(exporter OTLPExporter, query MarkerQueryer) (*providerobservability.ManagedLifecycleClient, error) {
	return NewOTLPLifecycleClientWithQueryAndOperations(exporter, query, nil)
}

// NewOTLPLifecycleClientWithQueryAndOperations composes the OTLP data plane
// with the optional NerdGraph operational-object plane. The exporter may be
// nil only when the caller intentionally manages an operations-only plan.
func NewOTLPLifecycleClientWithQueryAndOperations(exporter OTLPExporter, query MarkerQueryer, operations *NerdGraphOperations) (*providerobservability.ManagedLifecycleClient, error) {
	if exporter == nil && operations == nil {
		return nil, errors.New("New Relic OTLP exporter or NerdGraph operations client is required")
	}
	backend := &OTLPLifecycleBackend{exporter: exporter, query: query, operations: operations, now: time.Now}
	return providerobservability.NewManagedLifecycleClient(backend)
}

// NewNerdGraphLifecycleClient constructs an operations-only lifecycle client
// for plans that declare alerts, dashboards, or SLOs without OTLP bindings.
func NewNerdGraphLifecycleClient(operations *NerdGraphOperations) (*providerobservability.ManagedLifecycleClient, error) {
	if operations == nil {
		return nil, errors.New("New Relic NerdGraph operations client is required")
	}
	return NewOTLPLifecycleClientWithQueryAndOperations(nil, nil, operations)
}

func (backend *OTLPLifecycleBackend) Apply(ctx context.Context, plan providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	if err := validateOTLPPlan(ctx, plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	}
	if len(plan.UnavailableOperations) > 0 {
		return providerobservability.LifecycleResult{}, fmt.Errorf("New Relic observability operations are unavailable: %s", strings.Join(plan.UnavailableOperations, ", "))
	}
	if len(plan.Bindings) > 0 && (backend == nil || backend.exporter == nil) {
		return providerobservability.LifecycleResult{}, errors.New("New Relic OTLP exporter is required for signal bindings")
	}
	if hasOperationalObjects(plan) {
		if backend == nil || backend.operations == nil {
			return providerobservability.LifecycleResult{}, errors.New("New Relic NerdGraph operations client is required for alerts, dashboards, or SLOs")
		}
		if err := validateNerdGraphPlan(ctx, plan); err != nil {
			return providerobservability.LifecycleResult{}, err
		}
	}
	refs := make([]string, 0, len(plan.Bindings))
	for _, binding := range plan.Bindings {
		if binding.Destination != "newrelic" || binding.OwnershipMarker != plan.OwnershipMarker {
			return providerobservability.LifecycleResult{}, errors.New("New Relic binding ownership or destination does not match the plan")
		}
		if binding.RetentionDays > 0 {
			return providerobservability.LifecycleResult{}, errors.New("New Relic OTLP retention must be managed by an explicit New Relic policy, not this exporter")
		}
		if strings.TrimSpace(binding.RedactionPolicy) != "" {
			return providerobservability.LifecycleResult{}, errors.New("New Relic OTLP redaction requires an explicit collector redaction policy")
		}
		payload, signal, err := probePayload(binding.Signal, binding.OwnershipMarker, backend.now())
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		accepted, err := backend.exporter.Export(ctx, ExportRequest{
			Endpoint: binding.Endpoint, CredentialRef: binding.CredentialRef, Signal: signal,
			Payload: payload, OwnershipMarker: binding.OwnershipMarker,
		})
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		if !accepted.Accepted {
			return providerobservability.LifecycleResult{}, errors.New("New Relic OTLP endpoint did not accept the probe")
		}
		refs = append(refs, otlpResourceRef(binding.OwnershipMarker, binding.Signal))
	}
	if hasOperationalObjects(plan) {
		operationRefs, err := backend.operations.Apply(ctx, plan)
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs = append(refs, operationRefs...)
	}
	operationID := "newrelic:observability:apply:" + markerDigest(plan.OwnershipMarker)
	proofRefs := make([]string, 0, 3)
	if len(plan.Bindings) > 0 {
		operationID = "newrelic:otlp:apply:" + markerDigest(plan.OwnershipMarker)
		proofRefs = append(proofRefs, "newrelic:otlp:accepted", "newrelic:otlp:ownership-attributes")
	}
	proofRefs = append(proofRefs, operationalProofRef(plan)...)
	return providerobservability.LifecycleResult{
		OperationID:         operationID,
		ResourceRefs:        refs,
		ProofRefs:           proofRefs,
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (backend *OTLPLifecycleBackend) VerifySignal(ctx context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	if ctx == nil {
		return providerobservability.SignalObservation{}, errors.New("New Relic OTLP verification context is required")
	}
	if backend == nil || backend.exporter == nil {
		return providerobservability.SignalObservation{}, errors.New("New Relic OTLP exporter is required")
	}
	if err := ctx.Err(); err != nil {
		return providerobservability.SignalObservation{}, err
	}
	if err := validateOTLPBinding(binding, false); err != nil {
		return providerobservability.SignalObservation{}, err
	}
	observation := providerobservability.SignalObservation{
		Signal:            binding.Signal,
		Destination:       binding.Destination,
		RetentionVerified: binding.RetentionDays == 0,
		RedactionVerified: strings.TrimSpace(binding.RedactionPolicy) == "",
		Reason:            "New Relic OTLP acceptance is not queryable delivery proof; a provider query or application smoke probe is required",
	}
	if binding.RetentionDays > 0 {
		observation.Reason = "New Relic OTLP retention was not managed by this exporter"
		return observation, nil
	}
	if strings.TrimSpace(binding.RedactionPolicy) != "" {
		observation.Reason = "New Relic OTLP redaction was not managed by this exporter"
		return observation, nil
	}
	if backend.query != nil {
		result, err := backend.query.QueryMarker(ctx, MarkerQueryRequest{
			Signal:          Signal(binding.Signal),
			CredentialRef:   binding.CredentialRef,
			OwnershipMarker: binding.OwnershipMarker,
		})
		if err != nil {
			return providerobservability.SignalObservation{}, err
		}
		observation.Delivered = result.Count > 0
		observation.LabelsVerified = result.LabelsVerified
		if observation.Delivered && observation.LabelsVerified {
			observation.Reason = "New Relic OTLP ownership marker was queryable through NerdGraph NRQL"
		} else {
			observation.Reason = "New Relic OTLP endpoint accepted the probe but its ownership marker was not visible through NerdGraph NRQL"
		}
	}
	if !observation.RetentionVerified {
		observation.Reason = "New Relic OTLP retention was not managed by this exporter"
	} else if !observation.RedactionVerified {
		observation.Reason = "New Relic OTLP redaction was not managed by this exporter"
	}
	return observation, nil
}

func (backend *OTLPLifecycleBackend) VerifyOperations(ctx context.Context, plan providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	if err := validateOTLPPlan(ctx, plan); err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	if !hasOperationalObjects(plan) {
		return providerobservability.OperationalObservation{AlertsVerified: true, DashboardsVerified: true, SLOsVerified: true}, nil
	}
	if backend != nil && backend.operations != nil {
		return backend.operations.VerifyOperations(ctx, plan)
	}
	return providerobservability.OperationalObservation{
		Reason: "New Relic NerdGraph operations client is not configured for the declared operational objects",
	}, nil
}

func (backend *OTLPLifecycleBackend) Destroy(ctx context.Context, plan providerobservability.Plan, _ []string) error {
	if err := validateOTLPPlan(ctx, plan); err != nil {
		return err
	}
	if !hasOperationalObjects(plan) {
		return nil
	}
	if backend == nil || backend.operations == nil {
		return errors.New("New Relic NerdGraph operations client is required for operational cleanup")
	}
	return backend.operations.Destroy(ctx, plan)
}

func (backend *OTLPLifecycleBackend) Inventory(ctx context.Context, marker string) ([]providerobservability.InventoryResource, error) {
	if backend != nil && backend.operations != nil {
		return backend.operations.Inventory(ctx, marker)
	}
	if ctx == nil || strings.TrimSpace(marker) == "" {
		return nil, errors.New("New Relic OTLP inventory requires context and ownership marker")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// OTLP data is provider-retained data, not a deletable MageLift-owned
	// resource. Returning an empty direct inventory prevents false cleanup
	// claims while avoiding an unsafe attempt to delete customer telemetry.
	return []providerobservability.InventoryResource{}, nil
}

func (backend *OTLPLifecycleBackend) InventoryForPlan(ctx context.Context, plan providerobservability.Plan) ([]providerobservability.InventoryResource, error) {
	if err := validateOTLPPlan(ctx, plan); err != nil {
		return nil, err
	}
	if !hasOperationalObjects(plan) {
		return []providerobservability.InventoryResource{}, nil
	}
	if backend == nil || backend.operations == nil {
		return nil, errors.New("New Relic NerdGraph operations client is required for operational inventory")
	}
	return backend.operations.InventoryForPlan(ctx, plan)
}

func validateOTLPPlan(ctx context.Context, plan providerobservability.Plan) error {
	if ctx == nil {
		return errors.New("New Relic OTLP context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateNewRelicTarget(plan.TargetProvider, plan.TargetRuntime); err != nil {
		return err
	}
	if err := validateOwnershipMarker(plan.OwnershipMarker); err != nil {
		return err
	}
	if len(plan.Bindings) == 0 && !hasOperationalObjects(plan) {
		return errors.New("New Relic OTLP plan requires a signal binding or operational object")
	}
	for _, binding := range plan.Bindings {
		if err := validateOTLPBinding(binding, true); err != nil {
			return err
		}
		if binding.OwnershipMarker != plan.OwnershipMarker {
			return errors.New("New Relic OTLP binding ownership marker does not match the plan")
		}
	}
	return nil
}

func hasOperationalObjects(plan providerobservability.Plan) bool {
	return len(plan.Alerts) > 0 || len(plan.Dashboards) > 0 || len(plan.SLOs) > 0
}

func operationalProofRef(plan providerobservability.Plan) []string {
	if !hasOperationalObjects(plan) {
		return nil
	}
	return []string{"newrelic:nerdgraph:operations"}
}

func validateOTLPBinding(binding providerobservability.SignalBinding, requireManagedPolicy bool) error {
	if binding.Destination != "newrelic" {
		return errors.New("New Relic OTLP plan contains a binding for another destination")
	}
	if binding.Mode != "otlp" {
		return fmt.Errorf("New Relic OTLP binding mode must be %q", "otlp")
	}
	if err := validateOwnershipMarker(binding.OwnershipMarker); err != nil {
		return fmt.Errorf("New Relic OTLP binding ownership marker: %w", err)
	}
	if err := sdk.ValidateCredentialReference(binding.CredentialRef); err != nil {
		return fmt.Errorf("New Relic OTLP credential reference: %w", err)
	}
	if err := validateSignal(Signal(binding.Signal)); err != nil {
		return err
	}
	if _, err := endpointForSignal(binding.Endpoint, Signal(binding.Signal)); err != nil {
		return fmt.Errorf("New Relic OTLP endpoint for %s: %w", binding.Signal, err)
	}
	if requireManagedPolicy && binding.RetentionDays > 0 {
		return errors.New("New Relic OTLP retention must be managed by an explicit New Relic policy, not this exporter")
	}
	if requireManagedPolicy && strings.TrimSpace(binding.RedactionPolicy) != "" {
		return errors.New("New Relic OTLP redaction requires an explicit collector redaction policy")
	}
	return nil
}

func probePayload(signal string, marker string, now time.Time) (proto.Message, Signal, error) {
	var payload proto.Message
	var normalized Signal
	resource := &resourcepb.Resource{Attributes: []*commonv1.KeyValue{
		attribute("magelift.ownership_marker", marker),
		attribute("magelift.signal", signal),
		attribute("service.name", "magelift-observability-probe"),
	}}
	scopeName := &commonv1.InstrumentationScope{Name: "github.com/magelift/magelift/observability", Version: "1.0.0"}
	nanos := uint64(now.UnixNano())
	switch signal {
	case string(SignalLogs):
		normalized = SignalLogs
		payload = &logsv1.ExportLogsServiceRequest{ResourceLogs: []*logspb.ResourceLogs{{Resource: resource, ScopeLogs: []*logspb.ScopeLogs{{Scope: scopeName, LogRecords: []*logspb.LogRecord{{TimeUnixNano: nanos, ObservedTimeUnixNano: nanos, Body: stringValue("magelift probe signal=" + signal + " marker=" + marker), Attributes: []*commonv1.KeyValue{attribute("magelift.ownership_marker", marker), attribute("magelift.signal", signal)}}}}}}}}
	case string(SignalMetrics):
		normalized = SignalMetrics
		payload = &metricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricspb.ResourceMetrics{{Resource: resource, ScopeMetrics: []*metricspb.ScopeMetrics{{Scope: scopeName, Metrics: []*metricspb.Metric{{Name: "magelift.observability.probe", Unit: "1", Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: []*metricspb.NumberDataPoint{{TimeUnixNano: nanos, Value: &metricspb.NumberDataPoint_AsDouble{AsDouble: 1}, Attributes: []*commonv1.KeyValue{attribute("magelift.ownership_marker", marker), attribute("magelift.signal", signal)}}}}}}}}}}}}
	case string(SignalTraces):
		normalized = SignalTraces
		digest := sha256.Sum256([]byte(marker + "\x00" + signal))
		payload = &tracesv1.ExportTraceServiceRequest{ResourceSpans: []*tracepb.ResourceSpans{{Resource: resource, ScopeSpans: []*tracepb.ScopeSpans{{Scope: scopeName, Spans: []*tracepb.Span{{TraceId: append([]byte(nil), digest[:16]...), SpanId: append([]byte(nil), digest[:8]...), Name: "magelift.observability.probe", StartTimeUnixNano: nanos - uint64(time.Millisecond), EndTimeUnixNano: nanos, Attributes: []*commonv1.KeyValue{attribute("magelift.ownership_marker", marker), attribute("magelift.signal", signal)}}}}}}}}
	default:
		return nil, "", fmt.Errorf("New Relic OTLP lifecycle does not implement signal %q", signal)
	}
	return payload, normalized, nil
}

func attribute(key, value string) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: key, Value: stringValue(value)}
}

func stringValue(value string) *commonv1.AnyValue {
	return &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: value}}
}

func otlpResourceRef(marker, signal string) string {
	return "newrelic:otlp:" + signal + ":" + markerDigest(marker)
}

func markerDigest(marker string) string {
	digest := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(digest[:])[:16]
}
