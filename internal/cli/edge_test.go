package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	fastlyedge "github.com/magelift/magelift/internal/external/fastly"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

type recordingFastly struct {
	planRequest    fastlyedge.Request
	applyRequest   fastlyedge.Request
	destroyRequest fastlyedge.Request
	result         fastlyedge.Result
}

type recordingNativeEdge struct {
	plans      []sdk.EdgePlanRequest
	executions []sdk.EdgeExecutionRequest
}

func (e *recordingNativeEdge) EdgeDescriptor() sdk.EdgeAdapterDescriptor { return sdk.EdgeAdapterDescriptor{ID: "test.edge"} }
func (e *recordingNativeEdge) PlanEdge(_ context.Context, request sdk.EdgePlanRequest) (sdk.EdgePlan, error) {
	e.plans = append(e.plans, request)
	return sdk.EdgePlan{AdapterID: "test.edge", TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, OwnershipMarker: request.Intent.OwnershipMarker}, nil
}
func (e *recordingNativeEdge) ExecuteEdge(_ context.Context, request sdk.EdgeExecutionRequest) (sdk.EdgeExecutionResult, error) {
	e.executions = append(e.executions, request)
	return sdk.EdgeExecutionResult{Action: request.Action, OperationID: "operation-1", OwnershipMarker: request.OwnershipMarker}, nil
}

func TestNativeEdgePurgeUsesOwnedResourcesAndFreshIdempotencyKeys(t *testing.T) {
	planned := stubPlanned{stackName: "shop-staging", provider: "aws", runtime: "ecs-fargate"}
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"edgeDistributionId": "E123"}}
	o := &options{newBackend: func(context.Context, platform.PlannedStack, string) (infrastructureBackend, error) { return backend, nil }}
	adapter := &recordingNativeEdge{}
	cfg := config.Config{
		Project: config.Project{Name: "shop"}, Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"}, Domain: "shop.example.com",
		Edge: config.EdgeConfig{Mode: "native", NativeProvider: "cloudfront-waf", Domains: []string{"shop.example.com"}, TLS: true, TLSMode: "existing", DNSMode: "existing", OriginHealthRef: "aws/alb"},
	}
	for range 2 {
		if _, err := o.executeNativeEdgePurge(context.Background(), "staging", cfg, planned, adapter); err != nil {
			t.Fatal(err)
		}
	}
	if len(adapter.plans) != 2 || len(adapter.executions) != 2 {
		t.Fatalf("plans=%d executions=%d", len(adapter.plans), len(adapter.executions))
	}
	first, second := adapter.executions[0], adapter.executions[1]
	if first.IdempotencyKey == second.IdempotencyKey || first.IdempotencyKey == "purge/staging" {
		t.Fatalf("purge keys must be unique per invocation: %q %q", first.IdempotencyKey, second.IdempotencyKey)
	}
	if len(first.ResourceReferences) != 1 || first.ResourceReferences[0] != "aws.cloudfront.distribution:E123" {
		t.Fatalf("AWS purge references = %v", first.ResourceReferences)
	}
	if adapter.plans[0].Configuration != nil || adapter.plans[0].Intent.NativeProvider != "cloudfront-waf" {
		t.Fatalf("AWS purge plan inputs = %#v", adapter.plans[0])
	}
	defaultAdapter := &recordingNativeEdge{}
	defaultConfig := cfg
	defaultConfig.Edge = config.EdgeConfig{}
	if _, err := o.executeNativeEdgePurge(context.Background(), "staging", defaultConfig, planned, defaultAdapter); err != nil {
		t.Fatal(err)
	}
	if len(defaultAdapter.plans) != 1 || defaultAdapter.plans[0].Intent.NativeProvider != "cloudfront-waf" || defaultAdapter.plans[0].Intent.Domains[0] != defaultConfig.Domain {
		t.Fatalf("default AWS edge intent = %#v", defaultAdapter.plans)
	}
}

func TestGKENativeEdgePurgeRequiresCurrentControllerOutputs(t *testing.T) {
	planned := stubPlanned{stackName: "shop-staging", provider: "gcp", runtime: "gke-autopilot"}
	marker := "magelift/edge/shop/staging"
	outputs := map[string]any{
		"nativeEdgeURLMapName": "k8s2-um-serving-shop", "nativeEdgeIngressName": "magelift-shop",
		"nativeEdgeIngressNamespace": "default", "nativeEdgeIngressOwner": marker,
	}
	o := &options{newBackend: func(context.Context, platform.PlannedStack, string) (infrastructureBackend, error) {
		return &fakeInfrastructureBackend{outputs: outputs}, nil
	}}
	cfg := config.Config{Project: config.Project{Name: "shop"}, Target: config.Target{Provider: "gcp", Runtime: "gke-autopilot"}, Domain: "shop.example.com",
		Edge: config.EdgeConfig{Mode: "native", NativeProvider: "cloud-cdn", Domains: []string{"shop.example.com"}, TLS: true, TLSMode: "gke-managed", DNSMode: "customer-managed", OriginHealthRef: "health/shop", OwnershipMarker: marker}}
	configuration, references, err := o.nativeEdgePurgeInputs(context.Background(), "staging", cfg, planned)
	if err != nil {
		t.Fatal(err)
	}
	gke, ok := configuration["gkeIngress"].(map[string]any)
	if !ok || gke["urlMapName"] != outputs["nativeEdgeURLMapName"] || gke["hostname"] != "shop.example.com" || len(references) != 0 {
		t.Fatalf("GKE purge configuration=%#v references=%v", configuration, references)
	}
	delete(outputs, "nativeEdgeURLMapName")
	if _, _, err := o.nativeEdgePurgeInputs(context.Background(), "staging", cfg, planned); err == nil || !strings.Contains(err.Error(), "refresh the infrastructure outputs") {
		t.Fatalf("missing URL map error = %v", err)
	}
}

func (client *recordingFastly) Plan(request fastlyedge.Request) (fastlyedge.Plan, error) {
	client.planRequest = request
	return fastlyedge.Plan{ServiceID: request.ServiceID, Domains: request.Domains, OwnershipMarker: request.OwnershipMarker}, nil
}

func (client *recordingFastly) Apply(_ context.Context, request fastlyedge.Request) (fastlyedge.Result, error) {
	client.applyRequest = request
	if client.result.ServiceID == "" {
		client.result = fastlyedge.Result{ServiceID: request.ServiceID, CreatedDomains: request.Domains, OwnershipMarker: request.OwnershipMarker}
	}
	return client.result, nil
}

func (client *recordingFastly) Destroy(_ context.Context, request fastlyedge.Request, result fastlyedge.Result) error {
	client.destroyRequest = request
	client.result = result
	return nil
}

func (client *recordingFastly) Purge(_ context.Context, request fastlyedge.Request) (fastlyedge.Result, error) {
	client.applyRequest = request
	if client.result.ServiceID == "" {
		client.result = fastlyedge.Result{ServiceID: request.ServiceID, OwnershipMarker: request.OwnershipMarker, PurgeRequested: true}
	}
	client.result.PurgeRequested = true
	return client.result, nil
}

func fastlyCLIConfig(t *testing.T) string {
	t.Helper()
	input := strings.Replace(starterConfig,
		"defaults:\n  region: eu-west-3\n  preset: preview",
		"edge:\n  externalProvider: fastly\n  serviceId: svc-fastly-123\n  tokenSecret: aws-secrets-manager://magelift/fastly-token\n  domains: [shop.example.com]\n  originHealthRef: health/magento\n  health: {expectedCname: dualstack.e.sni.global.fastly.net}\n  purgeOnDeploy: true\ndefaults:\n  region: eu-west-3\n  preset: preview",
		1,
	)
	if input == starterConfig {
		t.Fatal("starter config shape changed; update the Fastly test fixture")
	}
	path := filepath.Join(t.TempDir(), "magelift.php")
	if err := writeTestProjectFile(path, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFastlyEdgeApplyAndDestroyPersistOwnershipState(t *testing.T) {
	configPath := fastlyCLIConfig(t)
	client := &recordingFastly{}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	o.newFastly = func(fastlyedge.Request) (fastlyLifecycle, error) { return client, nil }

	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", configPath, "--env", "staging", "--output", "json", "edge", "apply"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	statePath := fastlyEdgeStatePath(configPath, "staging")
	info, err := os.Stat(statePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state mode = %o, want 600", info.Mode().Perm())
	}
	if client.applyRequest.OwnershipMarker != "magelift/edge/example-shop/staging" {
		t.Fatalf("ownership marker = %q", client.applyRequest.OwnershipMarker)
	}

	o.yes = true
	command = newCommandWithOptions(o)
	command.SetArgs([]string{"--config", configPath, "--env", "staging", "--output", "json", "edge", "destroy"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if client.destroyRequest.OwnershipMarker != client.applyRequest.OwnershipMarker {
		t.Fatalf("destroy marker = %q, apply marker = %q", client.destroyRequest.OwnershipMarker, client.applyRequest.OwnershipMarker)
	}
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("Fastly state still exists: %v", err)
	}
}

func TestFastlyEdgeApplyBlocksBeforeProviderLifecycleMutation(t *testing.T) {
	configPath := fastlyCLIConfig(t)
	client := &recordingFastly{}
	admission := &rejectingBootstrapPlanAdmission{err: errors.New("edge quota admission failed")}
	modules := platform.NewModuleRegistry()
	if err := modules.RegisterModule(bootstrapAdmissionModule{admission: admission}); err != nil {
		t.Fatal(err)
	}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.modules = modules
	o.configPath = configPath
	o.environment = "staging"
	o.newFastly = func(fastlyedge.Request) (fastlyLifecycle, error) { return client, nil }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", configPath, "--env", "staging", "edge", "apply"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "provider plan admission") || !strings.Contains(err.Error(), "edge quota admission failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if admission.calls != 1 {
		t.Fatalf("plan admission calls = %d, want 1", admission.calls)
	}
	if client.planRequest.OwnershipMarker != "" || client.applyRequest.OwnershipMarker != "" {
		t.Fatalf("Fastly lifecycle was reached after blocked admission: plan=%#v apply=%#v", client.planRequest, client.applyRequest)
	}
}

func TestFastlyOwnershipMarkerSanitizesProjectAndEnvironment(t *testing.T) {
	if got := fastlyOwnershipMarker("shop / eu", "preview branch"); got != "magelift/edge/shop---eu/preview-branch" {
		t.Fatalf("marker = %q", got)
	}
}

func TestFastlyRequestMapsTypedHealthPolicy(t *testing.T) {
	request, err := fastlyRequest(config.Config{
		Project: config.Project{Name: "shop"},
		Edge: config.EdgeConfig{
			ExternalProvider: "fastly", ServiceID: "service-123", TokenSecret: "aws-secrets-manager://magelift/fastly-token",
			Domains: []string{"shop.example.com"}, Health: &config.EdgeHealthConfig{
				ExpectedCNAME: "dualstack.e.sni.global.fastly.net", RoutePath: "/health", ExpectedStatus: 200, RouteTimeoutSeconds: 90, RoutePollSeconds: 2,
			},
		},
		Extensions: map[string]any{"fastly.edge": map[string]any{"originAddress": "origin.internal", "originPort": 8443, "originUseTls": true}},
	}, "staging")
	if err != nil {
		t.Fatal(err)
	}
	if request.ProviderConfig.HealthExpectedCNAME != "dualstack.e.sni.global.fastly.net" || request.ProviderConfig.HealthRoutePath != "/health" || request.ProviderConfig.HealthOriginHost != "shop.example.com" {
		t.Fatalf("Fastly health configuration = %#v", request.ProviderConfig)
	}
	if request.ProviderConfig.OriginAddress != "origin.internal" || request.ProviderConfig.OriginPort != 8443 || !request.ProviderConfig.OriginUseTLS {
		t.Fatalf("Fastly advanced configuration = %#v", request.ProviderConfig)
	}
}

func TestFastlyEdgeStatePathSanitizesEnvironment(t *testing.T) {
	path := fastlyEdgeStatePath("/tmp/project/magelift.php", "../staging branch")
	if path != "/tmp/project/.magelift/edge/..-staging-branch/fastly.json" {
		t.Fatalf("state path = %q", path)
	}
}

func TestEdgePurgeFastlyIssuesProviderPurge(t *testing.T) {
	configPath := fastlyCLIConfig(t)
	client := &recordingFastly{}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath = configPath
	o.environment = "staging"
	o.output = "json"
	o.newFastly = func(fastlyedge.Request) (fastlyLifecycle, error) { return client, nil }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", configPath, "--env", "staging", "--output", "json", "edge", "purge"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !client.result.PurgeRequested {
		t.Fatal("Fastly purge was not requested")
	}
	if !strings.Contains(output.String(), `"purged": true`) {
		t.Fatalf("output = %s", output.String())
	}
}

func TestEdgePurgeReturnsTypedUnsupportedWithoutCache(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "magelift.php")
	if err := writeTestProjectFile(path, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"--config", path, "--env", "staging", "edge", "purge"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error=%v code=%d", err, ExitCode(err))
	}
}
