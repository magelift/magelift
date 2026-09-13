package observability

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	cockpit "github.com/scaleway/scaleway-sdk-go/api/cockpit/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
	collectorlogsv1 "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	collectormetricsv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
)

const scalewayCockpitDestination = "scaleway-cockpit"

// CockpitAPI is the narrow official SDK surface used by the Scaleway
// lifecycle translator. Data-source and token names are the ownership boundary
// because Cockpit resources do not expose arbitrary user labels in the current
// API.
type CockpitAPI interface {
	CreateDataSource(context.Context, *cockpit.RegionalAPICreateDataSourceRequest) (*cockpit.DataSource, error)
	GetDataSource(context.Context, *cockpit.RegionalAPIGetDataSourceRequest) (*cockpit.DataSource, error)
	DeleteDataSource(context.Context, *cockpit.RegionalAPIDeleteDataSourceRequest) error
	ListDataSources(context.Context, *cockpit.RegionalAPIListDataSourcesRequest) ([]*cockpit.DataSource, error)
	CreateToken(context.Context, *cockpit.RegionalAPICreateTokenRequest) (*cockpit.Token, error)
	DeleteToken(context.Context, *cockpit.RegionalAPIDeleteTokenRequest) error
	ListTokens(context.Context, *cockpit.RegionalAPIListTokensRequest) ([]*cockpit.Token, error)
}

type sdkCockpitAPI struct {
	api    *cockpit.RegionalAPI
	region scw.Region
}

func (api sdkCockpitAPI) CreateDataSource(ctx context.Context, request *cockpit.RegionalAPICreateDataSourceRequest) (*cockpit.DataSource, error) {
	return api.api.CreateDataSource(request, scw.WithContext(ctx))
}

func (api sdkCockpitAPI) GetDataSource(ctx context.Context, request *cockpit.RegionalAPIGetDataSourceRequest) (*cockpit.DataSource, error) {
	return api.api.GetDataSource(request, scw.WithContext(ctx))
}

func (api sdkCockpitAPI) DeleteDataSource(ctx context.Context, request *cockpit.RegionalAPIDeleteDataSourceRequest) error {
	return api.api.DeleteDataSource(request, scw.WithContext(ctx))
}

func (api sdkCockpitAPI) ListDataSources(ctx context.Context, request *cockpit.RegionalAPIListDataSourcesRequest) ([]*cockpit.DataSource, error) {
	const pageSize int32 = 100
	var result []*cockpit.DataSource
	for page := int32(1); ; page++ {
		copy := *request
		copy.Region = api.region
		copy.Page = &page
		copy.PageSize = uint32Pointer(uint32(pageSize))
		response, err := api.api.ListDataSources(&copy, scw.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list Scaleway Cockpit data sources: %w", err)
		}
		result = append(result, response.DataSources...)
		if len(response.DataSources) == 0 || uint64(len(result)) >= response.TotalCount {
			return result, nil
		}
	}
}

func (api sdkCockpitAPI) CreateToken(ctx context.Context, request *cockpit.RegionalAPICreateTokenRequest) (*cockpit.Token, error) {
	return api.api.CreateToken(request, scw.WithContext(ctx))
}

func (api sdkCockpitAPI) DeleteToken(ctx context.Context, request *cockpit.RegionalAPIDeleteTokenRequest) error {
	return api.api.DeleteToken(request, scw.WithContext(ctx))
}

func (api sdkCockpitAPI) ListTokens(ctx context.Context, request *cockpit.RegionalAPIListTokensRequest) ([]*cockpit.Token, error) {
	const pageSize uint32 = 100
	var result []*cockpit.Token
	for page := int32(1); ; page++ {
		copy := *request
		copy.Region = api.region
		copy.Page = &page
		copy.PageSize = uint32Pointer(pageSize)
		response, err := api.api.ListTokens(&copy, scw.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("list Scaleway Cockpit tokens: %w", err)
		}
		result = append(result, response.Tokens...)
		if len(response.Tokens) == 0 || uint64(len(result)) >= response.TotalCount {
			return result, nil
		}
	}
}

// NewScalewayCockpitSDKClient creates a lifecycle client from the official
// Scaleway SDK. Authentication remains in scw.ClientOption and is never
// copied into the portable core.
func NewScalewayCockpitSDKClient(ctx context.Context, project, region string, opts ...scw.ClientOption) (*providerobservability.ManagedLifecycleClient, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway Cockpit observability context is required")
	}
	if strings.TrimSpace(project) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("Scaleway Cockpit project and region are required")
	}
	config, err := scw.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load Scaleway SDK config: %w", err)
	}
	profile, err := config.GetActiveProfile()
	if err != nil {
		return nil, fmt.Errorf("load active Scaleway SDK profile: %w", err)
	}
	clientOptions := []scw.ClientOption{scw.WithProfile(profile), scw.WithEnv()}
	clientOptions = append(clientOptions, opts...)
	clientOptions = append(clientOptions, scw.WithDefaultProjectID(project), scw.WithDefaultRegion(scw.Region(region)))
	client, err := scw.NewClient(clientOptions...)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway SDK client: %w", err)
	}
	backend, err := NewScalewayCockpitBackend(project, region, sdkCockpitAPI{api: cockpit.NewRegionalAPI(client), region: scw.Region(region)})
	if err != nil {
		return nil, err
	}
	return providerobservability.NewManagedLifecycleClient(backend)
}

// ScalewayCockpitBackend owns custom Cockpit data sources and a short-lived
// marker-scoped token used only by the provider-local delivery verifier. The
// token secret never crosses the portable lifecycle boundary.
type ScalewayCockpitBackend struct {
	project     string
	region      scw.Region
	api         CockpitAPI
	httpClient  *http.Client
	tokenID     string
	tokenSecret string
}

var _ providerobservability.Backend = (*ScalewayCockpitBackend)(nil)

func NewScalewayCockpitBackend(project, region string, api CockpitAPI) (*ScalewayCockpitBackend, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(region) == "" {
		return nil, errors.New("Scaleway Cockpit project and region are required")
	}
	if api == nil {
		return nil, errors.New("Scaleway Cockpit API is required")
	}
	return &ScalewayCockpitBackend{project: project, region: scw.Region(region), api: api, httpClient: http.DefaultClient}, nil
}

func (backend *ScalewayCockpitBackend) Apply(ctx context.Context, plan providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	}
	resources, err := backend.api.ListDataSources(ctx, &cockpit.RegionalAPIListDataSourcesRequest{ProjectID: backend.project, Region: backend.region})
	if err != nil {
		return providerobservability.LifecycleResult{}, fmt.Errorf("inventory Scaleway Cockpit data sources before apply: %w", err)
	}
	refs := make(map[string]struct{})
	for _, binding := range plan.Bindings {
		source, err := backend.ensureSource(ctx, plan.OwnershipMarker, binding, resources)
		if err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs["scaleway-cockpit:data-source:"+source.ID] = struct{}{}
		if source.ID != "" {
			resources = append(resources, source)
		}
	}
	if len(plan.Bindings) > 0 {
		if _, err := backend.ensureToken(ctx, plan); err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs["scaleway-cockpit:token:"+backend.tokenID] = struct{}{}
	}
	resourceRefs := make([]string, 0, len(refs))
	for ref := range refs {
		resourceRefs = append(resourceRefs, ref)
	}
	sort.Strings(resourceRefs)
	return providerobservability.LifecycleResult{
		OperationID:         "scaleway-cockpit:observability:apply:" + markerDigest(plan.OwnershipMarker),
		ResourceRefs:        resourceRefs,
		ProofRefs:           []string{"scaleway-cockpit:ownership-by-name", "scaleway-cockpit:direct-api"},
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (backend *ScalewayCockpitBackend) VerifySignal(ctx context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	if ctx == nil {
		return providerobservability.SignalObservation{}, errors.New("Scaleway Cockpit signal context is required")
	}
	if err := ctx.Err(); err != nil {
		return providerobservability.SignalObservation{}, err
	}
	observation := providerobservability.SignalObservation{Signal: binding.Signal, Destination: binding.Destination}
	if binding.Destination != scalewayCockpitDestination {
		observation.Reason = "Scaleway Cockpit destination does not match the adapter"
		return observation, nil
	}
	typeValue, ok := sourceType(binding.Signal)
	if !ok {
		observation.Reason = "Scaleway Cockpit signal is not backed by a data source"
		return observation, nil
	}
	sources, err := backend.api.ListDataSources(ctx, &cockpit.RegionalAPIListDataSourcesRequest{ProjectID: backend.project, Region: backend.region, Types: []cockpit.DataSourceType{typeValue}})
	if err != nil {
		return providerobservability.SignalObservation{}, errors.New("inventory Scaleway Cockpit data sources for verification failed")
	}
	for _, source := range sources {
		if source == nil || source.Name != dataSourceName(binding.OwnershipMarker, binding.Signal) {
			continue
		}
		if !ownedSource(source, backend.project, backend.region, typeValue) {
			observation.Reason = "Scaleway Cockpit data source ownership or type does not match"
			return observation, nil
		}
		observation.LabelsVerified = true
		observation.RetentionVerified = binding.RetentionDays == 0 || uint32(binding.RetentionDays) == source.RetentionDays
		if binding.RetentionDays == 0 {
			observation.RetentionVerified = source.RetentionDays > 0
		}
		observation.RedactionVerified = strings.TrimSpace(binding.RedactionPolicy) == ""
		if source.URL == "" || backend.tokenSecret == "" {
			observation.Reason = "Scaleway Cockpit source exists, but no provider-local probe credentials or endpoint are available"
			return observation, nil
		}
		var delivered bool
		switch binding.Signal {
		case "metrics":
			delivered, err = backend.verifyMetricProbe(ctx, source, binding.OwnershipMarker)
		case "logs":
			delivered, err = backend.verifyLogProbe(ctx, source, binding.OwnershipMarker)
		default:
			observation.Reason = "Scaleway Cockpit trace delivery requires a provider-local query verifier"
			return observation, nil
		}
		if err != nil {
			observation.Reason = "Scaleway Cockpit probe delivery or query failed: " + err.Error()
			return observation, nil
		}
		observation.Delivered = delivered
		if !delivered {
			observation.Reason = "Scaleway Cockpit probe was accepted but was not visible through the provider query API"
			return observation, nil
		}
		observation.Reason = "Scaleway Cockpit probe was delivered and returned by the provider query API"
		return observation, nil
	}
	observation.Reason = "Scaleway Cockpit data source is missing"
	return observation, nil
}

func (backend *ScalewayCockpitBackend) VerifyOperations(ctx context.Context, plan providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	return providerobservability.OperationalObservation{AlertsVerified: len(plan.Alerts) == 0, DashboardsVerified: len(plan.Dashboards) == 0, SLOsVerified: len(plan.SLOs) == 0, Reason: unsupportedOperationsReason(plan)}, nil
}

func (backend *ScalewayCockpitBackend) Destroy(ctx context.Context, plan providerobservability.Plan, _ []string) error {
	if err := backend.validatePlan(ctx, plan); err != nil {
		return err
	}
	sources, err := backend.api.ListDataSources(ctx, &cockpit.RegionalAPIListDataSourcesRequest{ProjectID: backend.project, Region: backend.region})
	if err != nil {
		return errors.New("inventory Scaleway Cockpit data sources for cleanup failed")
	}
	for _, source := range sources {
		if source == nil || !strings.HasPrefix(source.Name, dataSourcePrefix(plan.OwnershipMarker)) {
			continue
		}
		if !ownedSourceName(source, backend.project, backend.region) {
			return errors.New("Scaleway Cockpit cleanup encountered an ownership mismatch")
		}
		if err := backend.api.DeleteDataSource(ctx, &cockpit.RegionalAPIDeleteDataSourceRequest{Region: backend.region, DataSourceID: source.ID}); err != nil {
			return errors.New("delete Scaleway Cockpit data source failed")
		}
	}
	tokens, err := backend.api.ListTokens(ctx, &cockpit.RegionalAPIListTokensRequest{ProjectID: backend.project, Region: backend.region})
	if err != nil {
		return errors.New("inventory Scaleway Cockpit tokens for cleanup failed")
	}
	for _, token := range tokens {
		if token == nil || token.Name != tokenName(plan.OwnershipMarker) {
			continue
		}
		if token.ProjectID != backend.project || token.Region != backend.region || token.ID == "" {
			return errors.New("Scaleway Cockpit cleanup encountered an unsafe token identity")
		}
		if err := backend.api.DeleteToken(ctx, &cockpit.RegionalAPIDeleteTokenRequest{Region: backend.region, TokenID: token.ID}); err != nil {
			return errors.New("delete Scaleway Cockpit token failed")
		}
	}
	backend.tokenID = ""
	backend.tokenSecret = ""
	return nil
}

func (backend *ScalewayCockpitBackend) Inventory(ctx context.Context, marker string) ([]providerobservability.InventoryResource, error) {
	if ctx == nil || strings.TrimSpace(marker) == "" {
		return nil, errors.New("Scaleway Cockpit inventory requires context and ownership marker")
	}
	sources, err := backend.api.ListDataSources(ctx, &cockpit.RegionalAPIListDataSourcesRequest{ProjectID: backend.project, Region: backend.region})
	if err != nil {
		return nil, errors.New("inventory Scaleway Cockpit data sources failed")
	}
	resources := make([]providerobservability.InventoryResource, 0)
	for _, source := range sources {
		if source == nil || !strings.HasPrefix(source.Name, dataSourcePrefix(marker)) {
			continue
		}
		if !ownedSourceName(source, backend.project, backend.region) {
			return nil, errors.New("Scaleway Cockpit inventory encountered an ownership mismatch")
		}
		resources = append(resources, providerobservability.InventoryResource{Identity: "scaleway-cockpit:data-source:" + source.ID, OwnershipMarker: marker, Owned: true, Live: true})
	}
	tokens, err := backend.api.ListTokens(ctx, &cockpit.RegionalAPIListTokensRequest{ProjectID: backend.project, Region: backend.region})
	if err != nil {
		return nil, errors.New("inventory Scaleway Cockpit tokens failed")
	}
	for _, token := range tokens {
		if token == nil || token.Name != tokenName(marker) {
			continue
		}
		if token.ProjectID != backend.project || token.Region != backend.region || token.ID == "" {
			return nil, errors.New("Scaleway Cockpit inventory encountered an unsafe token identity")
		}
		resources = append(resources, providerobservability.InventoryResource{Identity: "scaleway-cockpit:token:" + token.ID, OwnershipMarker: marker, Owned: true, Live: true})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (backend *ScalewayCockpitBackend) ensureSource(ctx context.Context, marker string, binding providerobservability.SignalBinding, sources []*cockpit.DataSource) (*cockpit.DataSource, error) {
	typeValue, ok := sourceType(binding.Signal)
	if !ok {
		return nil, fmt.Errorf("Scaleway Cockpit signal %q is not backed by a data source", binding.Signal)
	}
	retention := binding.RetentionDays
	if retention == 0 {
		retention = defaultRetentionDays(binding.Signal)
	}
	name := dataSourceName(marker, binding.Signal)
	for _, source := range sources {
		if source == nil || source.Name != name {
			continue
		}
		if !ownedSource(source, backend.project, backend.region, typeValue) || source.RetentionDays != uint32(retention) {
			return nil, errors.New("Scaleway Cockpit data source ownership, type, or retention does not match the plan")
		}
		return source, nil
	}
	created, err := backend.api.CreateDataSource(ctx, &cockpit.RegionalAPICreateDataSourceRequest{Region: backend.region, ProjectID: backend.project, Name: name, Type: typeValue, RetentionDays: uint32Pointer(uint32(retention))})
	if err != nil {
		return nil, errors.New("create Scaleway Cockpit data source failed")
	}
	if created == nil || created.ID == "" || created.Name != name || !ownedSource(created, backend.project, backend.region, typeValue) || created.RetentionDays != uint32(retention) {
		return nil, errors.New("Scaleway Cockpit returned an unsafe data source identity")
	}
	return created, nil
}

func (backend *ScalewayCockpitBackend) validatePlan(ctx context.Context, plan providerobservability.Plan) error {
	if ctx == nil {
		return errors.New("Scaleway Cockpit observability context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(backend.project) == "" || strings.TrimSpace(string(backend.region)) == "" || strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("Scaleway Cockpit project, region, and ownership marker are required")
	}
	if len(plan.Alerts) > 0 || len(plan.Dashboards) > 0 || len(plan.SLOs) > 0 {
		return errors.New("Scaleway Cockpit alert, dashboard, and SLO lifecycle is not implemented by this adapter")
	}
	for _, binding := range plan.Bindings {
		if binding.Destination != scalewayCockpitDestination || binding.OwnershipMarker != plan.OwnershipMarker {
			return errors.New("Scaleway Cockpit binding ownership or destination does not match the plan")
		}
		if strings.TrimSpace(binding.RedactionPolicy) != "" {
			return errors.New("Scaleway Cockpit redaction policy is not managed by this adapter")
		}
		if _, ok := sourceType(binding.Signal); !ok {
			return fmt.Errorf("Scaleway Cockpit signal %q is not implemented by this adapter", binding.Signal)
		}
	}
	return nil
}

func (backend *ScalewayCockpitBackend) ensureToken(ctx context.Context, plan providerobservability.Plan) (*cockpit.Token, error) {
	if backend.tokenID != "" && backend.tokenSecret != "" {
		return &cockpit.Token{ID: backend.tokenID, Name: tokenName(plan.OwnershipMarker), ProjectID: backend.project, Region: backend.region}, nil
	}
	tokens, err := backend.api.ListTokens(ctx, &cockpit.RegionalAPIListTokensRequest{ProjectID: backend.project, Region: backend.region})
	if err != nil {
		return nil, errors.New("inventory Scaleway Cockpit tokens before apply failed")
	}
	for _, token := range tokens {
		if token != nil && token.Name == tokenName(plan.OwnershipMarker) {
			return nil, errors.New("Scaleway Cockpit token ownership collision requires cleanup before apply")
		}
	}
	scopes := make([]cockpit.TokenScope, 0, len(plan.Bindings)*2)
	seen := make(map[cockpit.TokenScope]struct{})
	for _, binding := range plan.Bindings {
		var required []cockpit.TokenScope
		switch binding.Signal {
		case "metrics":
			required = []cockpit.TokenScope{cockpit.TokenScopeWriteOnlyMetrics, cockpit.TokenScopeReadOnlyMetrics}
		case "logs":
			required = []cockpit.TokenScope{cockpit.TokenScopeWriteOnlyLogs, cockpit.TokenScopeReadOnlyLogs}
		case "traces":
			required = []cockpit.TokenScope{cockpit.TokenScopeWriteOnlyTraces, cockpit.TokenScopeReadOnlyTraces}
		default:
			return nil, fmt.Errorf("Scaleway Cockpit signal %q cannot select token scopes", binding.Signal)
		}
		for _, scope := range required {
			if _, exists := seen[scope]; exists {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
	}
	token, err := backend.api.CreateToken(ctx, &cockpit.RegionalAPICreateTokenRequest{Region: backend.region, ProjectID: backend.project, Name: tokenName(plan.OwnershipMarker), TokenScopes: scopes})
	if err != nil {
		return nil, errors.New("create Scaleway Cockpit token failed")
	}
	if token == nil || token.ID == "" || token.Name != tokenName(plan.OwnershipMarker) || token.ProjectID != backend.project || token.Region != backend.region || token.SecretKey == nil || strings.TrimSpace(*token.SecretKey) == "" {
		return nil, errors.New("Scaleway Cockpit returned an unsafe token identity")
	}
	backend.tokenID = token.ID
	backend.tokenSecret = *token.SecretKey
	return token, nil
}

func (backend *ScalewayCockpitBackend) verifyMetricProbe(ctx context.Context, source *cockpit.DataSource, marker string) (bool, error) {
	metricName := "magelift_delivery"
	request := &collectormetricsv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricsv1.ResourceMetrics{{Resource: &resourcev1.Resource{Attributes: attributes(marker)}, ScopeMetrics: []*metricsv1.ScopeMetrics{{Metrics: []*metricsv1.Metric{{Name: metricName, Unit: "1", Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{DataPoints: []*metricsv1.NumberDataPoint{{TimeUnixNano: uint64(time.Now().UnixNano()), Value: &metricsv1.NumberDataPoint_AsDouble{AsDouble: 1}, Attributes: attributes(marker)}}}}}}}}}}}
	if err := backend.pushOTLP(ctx, source.URL, "/otlp/v1/metrics", request); err != nil {
		return false, err
	}
	query := metricName + `{magelift_ownership="` + markerDigest(marker) + `"}`
	response, err := backend.queryJSON(ctx, source.URL, "/prometheus/api/v1/query", url.Values{"query": []string{query}})
	if err != nil {
		return false, err
	}
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Metric map[string]string `json:"metric"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return false, err
	}
	if result.Status != "success" {
		return false, fmt.Errorf("Scaleway Mimir query did not succeed: %s", result.Error)
	}
	for _, sample := range result.Data.Result {
		if sample.Metric["magelift_ownership"] == markerDigest(marker) {
			return true, nil
		}
	}
	return false, nil
}

func (backend *ScalewayCockpitBackend) verifyLogProbe(ctx context.Context, source *cockpit.DataSource, marker string) (bool, error) {
	probe := "magelift-probe marker=" + markerDigest(marker)
	request := &collectorlogsv1.ExportLogsServiceRequest{ResourceLogs: []*logsv1.ResourceLogs{{Resource: &resourcev1.Resource{Attributes: attributes(marker)}, ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: []*logsv1.LogRecord{{TimeUnixNano: uint64(time.Now().UnixNano()), Body: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: probe}}, Attributes: attributes(marker)}}}}}}}
	if err := backend.pushOTLP(ctx, source.URL, "/otlp/v1/logs", request); err != nil {
		return false, err
	}
	query := `{service_name="magelift-observability"} | magelift_ownership="` + markerDigest(marker) + `" |= ` + strconv.Quote(probe)
	values := url.Values{"query": []string{query}, "limit": []string{"100"}, "start": []string{strconv.FormatInt(time.Now().Add(-5*time.Minute).UnixNano(), 10)}, "end": []string{strconv.FormatInt(time.Now().Add(time.Minute).UnixNano(), 10)}}
	response, err := backend.queryJSON(ctx, source.URL, "/loki/api/v1/query_range", values)
	if err != nil {
		return false, err
	}
	var result struct {
		Status string `json:"status"`
		Error  string `json:"error"`
		Data   struct {
			Result []struct {
				Values [][]string `json:"values"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := json.Unmarshal(response, &result); err != nil {
		return false, err
	}
	if result.Status != "success" {
		return false, fmt.Errorf("Scaleway Loki query did not succeed: %s", result.Error)
	}
	for _, stream := range result.Data.Result {
		for _, value := range stream.Values {
			if len(value) > 1 && strings.Contains(value[1], probe) {
				return true, nil
			}
		}
	}
	return false, nil
}

func (backend *ScalewayCockpitBackend) pushOTLP(ctx context.Context, baseURL, path string, payload proto.Message) error {
	body, err := proto.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/x-protobuf")
	request.Header.Set("X-TOKEN", backend.tokenSecret)
	client := backend.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return httpResponseError("Scaleway Cockpit OTLP endpoint", response.StatusCode, responseBody)
	}
	return nil
}

func (backend *ScalewayCockpitBackend) queryJSON(ctx context.Context, baseURL, path string, values url.Values) ([]byte, error) {
	endpoint := strings.TrimRight(baseURL, "/") + path + "?" + values.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("X-TOKEN", backend.tokenSecret)
	client := backend.httpClient
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, httpResponseError("Scaleway Cockpit query endpoint", response.StatusCode, body)
	}
	return body, nil
}

func httpResponseError(operation string, statusCode int, body []byte) error {
	detail := strings.TrimSpace(string(body))
	if len(detail) > 512 {
		detail = detail[:512] + "..."
	}
	if detail == "" {
		return fmt.Errorf("%s returned HTTP %d", operation, statusCode)
	}
	return fmt.Errorf("%s returned HTTP %d: %s", operation, statusCode, detail)
}

func attributes(marker string) []*commonv1.KeyValue {
	return []*commonv1.KeyValue{
		{Key: "magelift_ownership", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: markerDigest(marker)}}},
		{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "magelift-observability"}}},
	}
}

func ownedSource(source *cockpit.DataSource, project string, region scw.Region, sourceType cockpit.DataSourceType) bool {
	return ownedSourceName(source, project, region) && source.Type == sourceType && source.Origin == cockpit.DataSourceOriginCustom
}

func ownedSourceName(source *cockpit.DataSource, project string, region scw.Region) bool {
	return source != nil && source.ProjectID == project && source.Region == region && strings.HasPrefix(source.Name, "magelift-")
}

func sourceType(signal string) (cockpit.DataSourceType, bool) {
	switch signal {
	case "logs":
		return cockpit.DataSourceTypeLogs, true
	case "metrics":
		return cockpit.DataSourceTypeMetrics, true
	case "traces":
		return cockpit.DataSourceTypeTraces, true
	default:
		return cockpit.DataSourceTypeUnknownType, false
	}
}

func defaultRetentionDays(signal string) int {
	if signal == "metrics" {
		return 31
	}
	return 7
}

func dataSourcePrefix(marker string) string { return "magelift-" + markerDigest(marker) + "-" }

func dataSourceName(marker, signal string) string { return dataSourcePrefix(marker) + signal }

func tokenName(marker string) string { return dataSourcePrefix(marker) + "token" }

func markerDigest(marker string) string {
	sum := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(sum[:])[:16]
}

func unsupportedOperationsReason(plan providerobservability.Plan) string {
	if len(plan.Alerts) > 0 || len(plan.Dashboards) > 0 || len(plan.SLOs) > 0 {
		return "Scaleway Cockpit currently exposes preconfigured alert-manager controls but no ownership-scoped generic alert/dashboard/SLO lifecycle"
	}
	return ""
}

func uint32Pointer(value uint32) *uint32 { return &value }
