package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	cockpit "github.com/scaleway/scaleway-sdk-go/api/cockpit/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/proto"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	v1 "github.com/magelift/magelift/sdk/v1"
)

type fakeCockpit struct {
	sources      []*cockpit.DataSource
	tokens       []*cockpit.Token
	creates      int
	tokenCreates int
}

func (fake *fakeCockpit) CreateDataSource(_ context.Context, request *cockpit.RegionalAPICreateDataSourceRequest) (*cockpit.DataSource, error) {
	if request.RetentionDays == nil {
		return nil, errors.New("retention is required")
	}
	fake.creates++
	source := &cockpit.DataSource{ID: "source-" + request.Name, ProjectID: request.ProjectID, Name: request.Name, Type: request.Type, Origin: cockpit.DataSourceOriginCustom, RetentionDays: *request.RetentionDays, Region: request.Region}
	fake.sources = append(fake.sources, source)
	return source, nil
}

func (fake *fakeCockpit) GetDataSource(_ context.Context, request *cockpit.RegionalAPIGetDataSourceRequest) (*cockpit.DataSource, error) {
	for _, source := range fake.sources {
		if source.ID == request.DataSourceID {
			return source, nil
		}
	}
	return nil, errors.New("data source missing")
}

func (fake *fakeCockpit) DeleteDataSource(_ context.Context, request *cockpit.RegionalAPIDeleteDataSourceRequest) error {
	for index, source := range fake.sources {
		if source.ID == request.DataSourceID {
			fake.sources = append(fake.sources[:index], fake.sources[index+1:]...)
			return nil
		}
	}
	return errors.New("data source missing")
}

func (fake *fakeCockpit) ListDataSources(_ context.Context, request *cockpit.RegionalAPIListDataSourcesRequest) ([]*cockpit.DataSource, error) {
	result := make([]*cockpit.DataSource, 0, len(fake.sources))
	for _, source := range fake.sources {
		if source.ProjectID != request.ProjectID || source.Region != request.Region {
			continue
		}
		if len(request.Types) > 0 && source.Type != request.Types[0] {
			continue
		}
		result = append(result, source)
	}
	return result, nil
}

func (fake *fakeCockpit) CreateToken(_ context.Context, request *cockpit.RegionalAPICreateTokenRequest) (*cockpit.Token, error) {
	fake.tokenCreates++
	secret := "test-secret"
	token := &cockpit.Token{ID: "token-" + request.Name, ProjectID: request.ProjectID, Name: request.Name, Region: request.Region, SecretKey: &secret, Scopes: request.TokenScopes}
	fake.tokens = append(fake.tokens, token)
	return token, nil
}

func (fake *fakeCockpit) DeleteToken(_ context.Context, request *cockpit.RegionalAPIDeleteTokenRequest) error {
	for index, token := range fake.tokens {
		if token.ID == request.TokenID {
			fake.tokens = append(fake.tokens[:index], fake.tokens[index+1:]...)
			return nil
		}
	}
	return errors.New("token missing")
}

func (fake *fakeCockpit) ListTokens(_ context.Context, request *cockpit.RegionalAPIListTokensRequest) ([]*cockpit.Token, error) {
	result := make([]*cockpit.Token, 0, len(fake.tokens))
	for _, token := range fake.tokens {
		if token.ProjectID == request.ProjectID && token.Region == request.Region {
			result = append(result, token)
		}
	}
	return result, nil
}

func newFakeCockpitBackend(t *testing.T) (*ScalewayCockpitBackend, *fakeCockpit) {
	t.Helper()
	api := &fakeCockpit{}
	backend, err := NewScalewayCockpitBackend("project", "fr-par", api)
	if err != nil {
		t.Fatal(err)
	}
	return backend, api
}

func TestScalewayCockpitBackendUsesOfficialSDKModelsAndReusesSources(t *testing.T) {
	backend, api := newFakeCockpitBackend(t)
	marker := "magelift/scaleway/observability-test"
	plan := providerobservability.Plan{
		OwnershipMarker: marker,
		Bindings: []providerobservability.SignalBinding{
			{Signal: "logs", Destination: scalewayCockpitDestination, Mode: "native", OwnershipMarker: marker, RetentionDays: 30},
			{Signal: "metrics", Destination: scalewayCockpitDestination, Mode: "native", OwnershipMarker: marker},
			{Signal: "traces", Destination: scalewayCockpitDestination, Mode: "native", OwnershipMarker: marker},
		},
	}
	result, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified || len(result.ResourceRefs) != 4 || api.creates != 3 || api.tokenCreates != 1 {
		t.Fatalf("apply result = %#v, creates=%d", result, api.creates)
	}
	for _, source := range api.sources {
		if source.Origin != cockpit.DataSourceOriginCustom || source.ProjectID != "project" || source.Region != scw.Region("fr-par") {
			t.Fatalf("unsafe source identity = %#v", source)
		}
	}

	second, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.ResourceRefs) != 4 || api.creates != 3 || api.tokenCreates != 1 || len(api.sources) != 3 {
		t.Fatalf("second apply was not idempotent: result=%#v creates=%d sources=%d", second, api.creates, len(api.sources))
	}

	for _, binding := range plan.Bindings {
		observation, err := backend.VerifySignal(context.Background(), binding)
		if err != nil {
			t.Fatal(err)
		}
		if observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified || observation.Reason == "" {
			t.Fatalf("verification incorrectly claimed delivery: %#v", observation)
		}
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
}

func TestScalewayCockpitBackendRejectsUnsafeExistingSourceAndUnsupportedOperations(t *testing.T) {
	backend, api := newFakeCockpitBackend(t)
	marker := "magelift/scaleway/test"
	api.sources = append(api.sources, &cockpit.DataSource{ID: "collision", ProjectID: "project", Name: dataSourceName(marker, "logs"), Type: cockpit.DataSourceTypeLogs, Origin: cockpit.DataSourceOriginScaleway, RetentionDays: 7, Region: scw.Region("fr-par")})
	_, err := backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: marker, Bindings: []providerobservability.SignalBinding{{Signal: "logs", Destination: scalewayCockpitDestination, OwnershipMarker: marker}}})
	if err == nil {
		t.Fatal("unowned source collision was accepted")
	}
	if api.creates != 0 {
		t.Fatalf("provider mutated after collision: creates=%d", api.creates)
	}

	_, err = backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: marker, Alerts: []v1.AlertIntent{{ID: "alert", Signal: "metrics", Severity: "warning", Operator: "gt", Threshold: 1, WindowSeconds: 60, Owner: "oncall", RunbookURL: "https://runbooks.example/alert", DeduplicationKey: "alert"}}})
	if err == nil {
		t.Fatal("unsupported alert lifecycle was accepted")
	}
}

func TestScalewayCockpitBackendVerifiesOTLPAndQueryContracts(t *testing.T) {
	backend, _ := newFakeCockpitBackend(t)
	backend.tokenSecret = "test-secret"
	marker := "magelift/scaleway/observability/http-test"
	digest := markerDigest(marker)
	var metricPosted, logPosted bool
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get("X-TOKEN"); got != backend.tokenSecret {
			t.Errorf("token header = %q", got)
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/otlp/v1/metrics":
			metricPosted = true
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read metrics request: %v", err)
				return
			}
			var payload collectormetricsv1.ExportMetricsServiceRequest
			if err := proto.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode metrics request: %v", err)
			} else if len(payload.ResourceMetrics) != 1 {
				t.Errorf("metrics resource count = %d", len(payload.ResourceMetrics))
			}
			response.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && request.URL.Path == "/prometheus/api/v1/query":
			query := request.URL.Query().Get("query")
			if !strings.Contains(query, `magelift_delivery{magelift_ownership="`+digest+`"}`) {
				t.Errorf("metrics query = %q", query)
			}
			io.WriteString(response, `{"status":"success","data":{"result":[{"metric":{"magelift_ownership":"`+digest+`"}}]}}`)
		case request.Method == http.MethodPost && request.URL.Path == "/otlp/v1/logs":
			logPosted = true
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read logs request: %v", err)
				return
			}
			var payload collectorlogsv1.ExportLogsServiceRequest
			if err := proto.Unmarshal(body, &payload); err != nil {
				t.Errorf("decode logs request: %v", err)
			} else if len(payload.ResourceLogs) != 1 {
				t.Errorf("logs resource count = %d", len(payload.ResourceLogs))
			}
			response.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && request.URL.Path == "/loki/api/v1/query_range":
			query := request.URL.Query().Get("query")
			if !strings.Contains(query, `{service_name="magelift-observability"}`) || !strings.Contains(query, `| magelift_ownership="`+digest+`"`) {
				t.Errorf("logs query = %q", query)
			}
			io.WriteString(response, `{"status":"success","data":{"result":[{"values":[["1","magelift-probe marker=`+digest+`"]]}]}}`)
		default:
			http.NotFound(response, request)
		}
	}))
	defer server.Close()
	backend.httpClient = server.Client()
	source := &cockpit.DataSource{URL: server.URL}

	metricDelivered, err := backend.verifyMetricProbe(context.Background(), source, marker)
	if err != nil || !metricDelivered {
		t.Fatalf("metric verification = delivered=%t err=%v", metricDelivered, err)
	}
	logDelivered, err := backend.verifyLogProbe(context.Background(), source, marker)
	if err != nil || !logDelivered {
		t.Fatalf("log verification = delivered=%t err=%v", logDelivered, err)
	}
	if !metricPosted || !logPosted {
		t.Fatalf("OTLP posts = metrics=%t logs=%t", metricPosted, logPosted)
	}
}
