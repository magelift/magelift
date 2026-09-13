package newrelic

import (
	"context"
	"strings"
	"testing"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"google.golang.org/protobuf/proto"
)

type captureExporter struct {
	requests []ExportRequest
}

func (exporter *captureExporter) Export(_ context.Context, request ExportRequest) (ExportResult, error) {
	exporter.requests = append(exporter.requests, request)
	return ExportResult{Signal: request.Signal, Endpoint: request.Endpoint, Accepted: true, OperationID: "accepted"}, nil
}

type captureMarkerQueryer struct {
	request MarkerQueryRequest
	result  MarkerQueryResult
}

func (queryer *captureMarkerQueryer) QueryMarker(_ context.Context, request MarkerQueryRequest) (MarkerQueryResult, error) {
	queryer.request = request
	return queryer.result, nil
}

func TestOTLPLifecycleBuildsTypedOwnedProbesAndCleansDataPlaneSafely(t *testing.T) {
	exporter := &captureExporter{}
	client, err := NewOTLPLifecycleClient(exporter)
	if err != nil {
		t.Fatal(err)
	}
	plan := providerobservability.Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/newrelic",
		Bindings: []providerobservability.SignalBinding{
			{Signal: "logs", Destination: "newrelic", Mode: "otlp", CredentialRef: "secrets://newrelic/license", Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic"},
			{Signal: "metrics", Destination: "newrelic", Mode: "otlp", CredentialRef: "secrets://newrelic/license", Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic"},
			{Signal: "traces", Destination: "newrelic", Mode: "otlp", CredentialRef: "secrets://newrelic/license", Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic"},
		},
	}
	result, err := client.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(exporter.requests) != 3 || len(result.ResourceRefs) != 3 || !result.OwnershipVerified || !result.IdempotencyVerified {
		t.Fatalf("apply result = %#v, requests = %d", result, len(exporter.requests))
	}
	for index, request := range exporter.requests {
		if request.Payload == nil || !strings.Contains(string(mustMarshal(t, request.Payload)), "magelift") {
			t.Fatalf("request %d payload did not carry the ownership probe: %#v", index, request.Payload)
		}
		if request.CredentialRef == "" || strings.Contains(string(mustMarshal(t, request.Payload)), "license") {
			t.Fatalf("request %d contained unsafe credential data", index)
		}
	}
	observation, err := client.VerifySignal(context.Background(), plan.Bindings[0])
	if err != nil || observation.Delivered || !strings.Contains(observation.Reason, "not queryable delivery proof") {
		t.Fatalf("verification = %#v, err = %v", observation, err)
	}
	cleanup, err := client.Destroy(context.Background(), plan, result.ResourceRefs)
	if err != nil || !cleanup.Complete || !cleanup.UnownedPreserved {
		t.Fatalf("cleanup = %#v, err = %v", cleanup, err)
	}
}

func TestOTLPLifecycleRejectsUnmanagedRetentionAndRedactionBeforeExport(t *testing.T) {
	exporter := &captureExporter{}
	client, err := NewOTLPLifecycleClient(exporter)
	if err != nil {
		t.Fatal(err)
	}
	base := providerobservability.Plan{OwnershipMarker: "magelift/test/newrelic", Bindings: []providerobservability.SignalBinding{{Signal: "metrics", Destination: "newrelic", CredentialRef: "secrets://newrelic/license", Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic"}}}
	for name, mutate := range map[string]func(*providerobservability.Plan){
		"retention": func(plan *providerobservability.Plan) { plan.Bindings[0].RetentionDays = 30 },
		"redaction": func(plan *providerobservability.Plan) { plan.Bindings[0].RedactionPolicy = "policy://redact" },
	} {
		t.Run(name, func(t *testing.T) {
			plan := base
			mutate(&plan)
			if _, err := client.Apply(context.Background(), plan); err == nil {
				t.Fatal("unmanaged requirement was accepted")
			}
			if len(exporter.requests) != 0 {
				t.Fatalf("exporter was called before validation: %d", len(exporter.requests))
			}
		})
	}
}

func TestOTLPLifecycleRejectsMissingCredentialBeforeExport(t *testing.T) {
	exporter := &captureExporter{}
	client, err := NewOTLPLifecycleClient(exporter)
	if err != nil {
		t.Fatal(err)
	}
	plan := providerobservability.Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/newrelic",
		Bindings: []providerobservability.SignalBinding{{
			Signal: "metrics", Destination: "newrelic", Mode: "otlp", Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic",
		}},
	}
	if _, err := client.Apply(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("missing credential error = %v", err)
	}
	if len(exporter.requests) != 0 {
		t.Fatalf("exporter was called before credential validation: %d", len(exporter.requests))
	}
}

func TestOTLPLifecycleRejectsEmptyAndDriftedPlansBeforeProviderActions(t *testing.T) {
	exporter := &captureExporter{}
	client, err := NewOTLPLifecycleClient(exporter)
	if err != nil {
		t.Fatal(err)
	}
	base := providerobservability.Plan{
		TargetProvider:  "aws",
		TargetRuntime:   "ecs-fargate",
		OwnershipMarker: "magelift/test/newrelic-boundary",
		Bindings: []providerobservability.SignalBinding{{
			Signal: "metrics", Destination: "newrelic", Mode: "otlp", CredentialRef: "secrets://newrelic/license",
			Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/newrelic-boundary",
		}},
	}
	for name, plan := range map[string]providerobservability.Plan{
		"empty": {TargetProvider: base.TargetProvider, TargetRuntime: base.TargetRuntime, OwnershipMarker: base.OwnershipMarker},
		"binding ownership drift": func() providerobservability.Plan {
			plan := base
			plan.Bindings = append([]providerobservability.SignalBinding(nil), base.Bindings...)
			plan.Bindings[0].OwnershipMarker = "magelift/test/other"
			return plan
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := client.VerifyOperations(context.Background(), plan); err == nil {
				t.Fatal("tampered New Relic plan was accepted by VerifyOperations")
			}
			if _, err := client.Destroy(context.Background(), plan, nil); err == nil {
				t.Fatal("tampered New Relic plan was accepted by Destroy")
			}
			if len(exporter.requests) != 0 {
				t.Fatalf("provider exporter was called during validation: %d", len(exporter.requests))
			}
		})
	}
}

func TestOTLPLifecycleUsesInjectedQueryableDeliveryProof(t *testing.T) {
	queryer := &captureMarkerQueryer{result: MarkerQueryResult{Count: 1, LabelsVerified: true}}
	client, err := NewOTLPLifecycleClientWithQuery(&captureExporter{}, queryer)
	if err != nil {
		t.Fatal(err)
	}
	binding := providerobservability.SignalBinding{
		Signal: "metrics", Destination: "newrelic", Mode: "otlp", CredentialRef: "secrets://newrelic/license",
		Endpoint: "https://otlp.eu01.nr-data.net", OwnershipMarker: "magelift/test/queryable",
	}
	observation, err := client.VerifySignal(context.Background(), binding)
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Delivered || !observation.LabelsVerified || !strings.Contains(observation.Reason, "queryable") {
		t.Fatalf("queryable observation = %#v", observation)
	}
	if queryer.request.Signal != SignalMetrics || queryer.request.CredentialRef != binding.CredentialRef || queryer.request.OwnershipMarker != binding.OwnershipMarker {
		t.Fatalf("query request = %#v", queryer.request)
	}
}

func mustMarshal(t *testing.T, message proto.Message) []byte {
	t.Helper()
	value, err := proto.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
