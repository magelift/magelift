package observability

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, args)
	state := args.Inputs.Copy()
	state["name"] = resource.NewStringProperty(args.Name)
	return args.Name, state, nil
}

func (m *mocks) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func TestNewCreatesNativeDashboardAndLogAlert(t *testing.T) {
	observabilityMocks := &mocks{}
	intent := sdk.ObservabilityIntent{
		NativeProvider: "google-cloud-operations", OwnershipMarker: "magelift/architecture/test",
		Signals: []string{"logs", "metrics"},
		Alerts:  []sdk.AlertIntent{{ID: "log-failure", Signal: "logs", Severity: "critical", Operator: "gt", WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/logs", DeduplicationKey: "logs"}},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{Project: "project", Region: "europe-west1", ClusterName: pulumi.String("shop-cluster"), Intent: intent})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err != nil {
		t.Fatal(err)
	}
	seenDashboard, seenAlert := false, false
	for _, resource := range observabilityMocks.resources {
		switch resource.TypeToken {
		case "gcp:monitoring/dashboard:Dashboard":
			seenDashboard = true
		case "gcp:monitoring/alertPolicy:AlertPolicy":
			seenAlert = true
		case "gcp:logging/projectBucketConfig:ProjectBucketConfig", "gcp:logging/projectSink:ProjectSink":
			t.Fatalf("retention resources registered with RetentionDays==0: %#v", observabilityMocks.resources)
		}
	}
	if !seenDashboard || !seenAlert {
		t.Fatalf("native GCP observability resources = %#v", observabilityMocks.resources)
	}
	if got, want := unavailableOperations(intent), []string{"alert.notification-channel"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GCP unavailable operations = %v, want %v", got, want)
	}
}

func TestNewRegistersNativeLogRetentionResourcesOnPreview(t *testing.T) {
	const marker = "magelift/gcp/observability-retention"
	observabilityMocks := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{
			Project: "project", Region: "europe-west1", ClusterName: pulumi.String("shop-cluster"),
			Intent: sdk.ObservabilityIntent{
				NativeProvider: "google-cloud-operations", OwnershipMarker: marker,
				Signals: []string{"logs"}, RetentionDays: 30,
			},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err != nil {
		t.Fatal(err)
	}

	var bucket, sink *pulumi.MockResourceArgs
	for index := range observabilityMocks.resources {
		registered := &observabilityMocks.resources[index]
		switch registered.TypeToken {
		case "gcp:logging/projectBucketConfig:ProjectBucketConfig":
			bucket = registered
		case "gcp:logging/projectSink:ProjectSink":
			sink = registered
		}
	}
	if bucket == nil || sink == nil {
		t.Fatalf("native log retention resources = %#v", observabilityMocks.resources)
	}
	if got, want := bucket.Inputs[resource.PropertyKey("bucketId")].StringValue(), googleLogBucketID(marker); got != want {
		t.Fatalf("log retention bucket ID = %q, want %q", got, want)
	}
	if got, want := bucket.Inputs[resource.PropertyKey("location")].StringValue(), googleCloudOperationsDefaultLogLocation; got != want {
		t.Fatalf("log retention bucket location = %q, want %q", got, want)
	}
	if got, want := bucket.Inputs[resource.PropertyKey("retentionDays")].NumberValue(), float64(30); got != want {
		t.Fatalf("log retention bucket retention = %v, want %v", got, want)
	}
	if got, want := sink.Inputs[resource.PropertyKey("name")].StringValue(), googleLogSinkID(marker); got != want {
		t.Fatalf("log retention sink ID = %q, want %q", got, want)
	}
	if got, want := sink.Inputs[resource.PropertyKey("destination")].StringValue(), "logging.googleapis.com/"+googleLogBucketName("project", googleCloudOperationsDefaultLogLocation, marker); got != want {
		t.Fatalf("log retention sink destination = %q, want %q", got, want)
	}
	if got, want := sink.Inputs[resource.PropertyKey("filter")].StringValue(), `logName = `+quote(logName("project", marker)); got != want {
		t.Fatalf("log retention sink filter = %q, want %q", got, want)
	}
}

func TestNewRejectsUnsupportedAlertBeforeRegisteringResources(t *testing.T) {
	observabilityMocks := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{
			Project: "project", Region: "europe-west1", ClusterName: pulumi.String("shop-cluster"),
			Intent: sdk.ObservabilityIntent{
				NativeProvider: "google-cloud-operations", OwnershipMarker: "magelift/architecture/test", Signals: []string{"metrics"},
				Alerts: []sdk.AlertIntent{{ID: "bad", Signal: "queue-health", Severity: "critical", Operator: "gt", Threshold: 1, WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/queue", DeduplicationKey: "queue"}},
			},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err == nil || !strings.Contains(err.Error(), "unsupported signal") {
		t.Fatalf("unsupported GCP alert error = %v", err)
	}
	if len(observabilityMocks.resources) != 0 {
		t.Fatalf("unsupported alert registered resources before failing: %#v", observabilityMocks.resources)
	}
}

func TestNewRejectsInvalidRetentionAndUnsupportedRedactionBeforeRegisteringResources(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*sdk.ObservabilityIntent)
		want   string
	}{
		{name: "retention below minimum", mutate: func(intent *sdk.ObservabilityIntent) { intent.RetentionDays = -1 }, want: "retention"},
		{name: "retention above maximum", mutate: func(intent *sdk.ObservabilityIntent) { intent.RetentionDays = 3651 }, want: "retention"},
		{name: "redaction", mutate: func(intent *sdk.ObservabilityIntent) { intent.RedactionPolicyRef = "policy://redact" }, want: "redaction"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			observabilityMocks := &mocks{}
			intent := sdk.ObservabilityIntent{
				NativeProvider: "google-cloud-operations", OwnershipMarker: "magelift/architecture/test", Signals: []string{"logs"},
			}
			test.mutate(&intent)
			err := pulumi.RunErr(func(ctx *pulumi.Context) error {
				_, err := New(ctx, "shop-observability", Args{
					Project: "project", Region: "europe-west1", ClusterName: pulumi.String("shop-cluster"), Intent: intent,
				})
				return err
			}, pulumi.WithMocks("magelift", "test", observabilityMocks))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("invalid GCP observability input error = %v, want %q", err, test.want)
			}
			if len(observabilityMocks.resources) != 0 {
				t.Fatalf("invalid input registered resources before failing: %#v", observabilityMocks.resources)
			}
		})
	}
}

func TestNewCreatesRequestedDashboardsInsteadOfReportingThemUnavailable(t *testing.T) {
	observabilityMocks := &mocks{}
	intent := sdk.ObservabilityIntent{
		NativeProvider: "google-cloud-operations", OwnershipMarker: "magelift/observability/dashboards",
		Signals: []string{"logs", "metrics"},
		Dashboards: []sdk.DashboardIntent{
			{ID: "resilience", Signals: []string{"metrics"}, Owner: "oncall"},
			{ID: "delivery", Signals: []string{"logs", "traces"}, Owner: "oncall"},
		},
	}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{
			Project: "project", Region: "europe-west1", ClusterName: pulumi.String("shop-cluster"), Intent: intent,
		})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err != nil {
		t.Fatal(err)
	}

	var dashboards []pulumi.MockResourceArgs
	for _, registered := range observabilityMocks.resources {
		if registered.TypeToken == "gcp:monitoring/dashboard:Dashboard" {
			dashboards = append(dashboards, registered)
		}
	}
	if got, want := len(dashboards), 2; got != want {
		t.Fatalf("requested GCP dashboards = %d, want %d: %#v", got, want, observabilityMocks.resources)
	}
	if got, want := unavailableOperations(intent), []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GCP requested dashboard operations = %v, want %v", got, want)
	}
	if got, want := unavailableSignals(intent), []string{"traces"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GCP requested dashboard signals = %v, want %v", got, want)
	}
}

func TestNativeGapOutputsKeepUnsupportedSignalsAndOperationsExplicit(t *testing.T) {
	intent := sdk.ObservabilityIntent{
		NativeProvider: "google-cloud-operations", OwnershipMarker: "magelift/observability/test",
		Signals:    []string{"logs", "metrics", "traces", "audit-events"},
		Dashboards: []sdk.DashboardIntent{{ID: "resilience", Signals: []string{"metrics"}, Owner: "oncall"}},
		SLOs:       []sdk.SLOIntent{{ID: "availability"}},
	}
	if got, want := unavailableSignals(intent), []string{"audit-events", "traces"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GCP unavailable signals = %v, want %v", got, want)
	}
	if got, want := unavailableOperations(intent), []string{"slo:availability"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("GCP unavailable operations = %v, want %v", got, want)
	}
}
