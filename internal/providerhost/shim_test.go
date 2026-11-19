package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	deployflow "github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func deployRequestForTest(digest string) deployflow.Request {
	return deployflow.Request{Target: sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"}, ImageDigest: digest}
}

func shimTestConfig() config.Config {
	return config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application: config.Application{
			Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm",
			Magento: config.MagentoRuntime{FrontName: "admin_abc", Consumers: config.MagentoConsumers{Mode: "processes"}},
		},
		Email: config.EmailConfig{
			Mode: "smtp", Host: "smtp.example.com", Port: 587, Username: "mailer",
			From: "shop@example.com", Credential: "gcp-secret-manager://projects/example-gcp-project/secrets/smtp/versions/latest",
		},
		Target: config.Target{
			Provider: "gcp", Runtime: "gke-autopilot",
			GCP: &config.GCPTarget{
				Project:             "example-gcp-project",
				Region:              "europe-west1",
				NetworkCIDR:         "10.20.0.0/16",
				ImageDigest:         "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
				EncryptionKeySecret: "magento-crypt-key",
			},
		},
		Defaults:           config.Defaults{Region: "europe-west1", Preset: "standard"},
		Class:              "staging",
		Preset:             "standard",
		ExpiresAt:          time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		MonthlyBudgetCents: 50000,
	}
}

func awsShimTestConfig() config.Config {
	cfg := shimTestConfig()
	cfg.Target.Provider = "aws"
	cfg.Target.Runtime = "ecs-fargate"
	cfg.Target.GCP = nil
	cfg.Target.AWS = &config.AWSTarget{}
	cfg.Defaults.Region = "eu-west-3"
	return cfg
}

func cannedStoredPlan() sdk.StoredPlan {
	return sdk.StoredPlan{
		StackName: "shop-staging-gcp-gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot",
		Tier: sdk.ExtensionTierExperimental, ImageDigest: "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		StateBackendURL: "gs://shop-staging-state", Opaque: []byte(`{"stored":true}`),
	}
}

func shimTestModule(t *testing.T, caller *stubCaller) *ShimModule {
	t.Helper()
	module, err := NewShimModule("gcp", "gke-autopilot", &Client{caller: caller, describe: cannedDescribe()})
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func awsShimTestModule(t *testing.T, caller *stubCaller) *ShimModule {
	t.Helper()
	module, err := newShimModule("aws", "ecs-fargate", platform.TierBuiltIn, awsOutputKeys(), &Client{caller: caller, describe: cannedDescribe()})
	if err != nil {
		t.Fatal(err)
	}
	return module
}

func planResponder(plan sdk.StoredPlan) func(string, any) error {
	return func(method string, reply any) error {
		if method != "Plugin.Plan" {
			return errors.New("unexpected method " + method)
		}
		*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: plan}
		return nil
	}
}

func TestNewShimModule(t *testing.T) {
	t.Parallel()
	if _, err := NewShimModule("gcp", "gke-autopilot", nil); err == nil {
		t.Fatal("nil client was accepted")
	}
	if _, err := NewShimModule("gcp", "gke-autopilot", &Client{}); err != nil {
		t.Fatalf("lazy client was refused: %v", err)
	}
	if _, err := NewShimModule("gcp", "ecs-fargate", &Client{describe: cannedDescribe()}); err == nil {
		t.Fatal("unserved runtime was accepted")
	}
	module := shimTestModule(t, &stubCaller{})
	if got := module.Descriptor(); got.ID != "gcp.gke-autopilot" || got.Provider != "gcp" || got.Runtime != "gke-autopilot" {
		t.Fatalf("descriptor = %#v", got)
	}
	if module.CertificationTier() != platform.TierBuiltIn {
		t.Fatalf("tier = %q", module.CertificationTier())
	}
	standard, err := NewShimModule("gcp", "gke-standard", &Client{describe: cannedDescribe()})
	if err != nil {
		t.Fatal(err)
	}
	if standard.CertificationTier() != platform.TierExperimental {
		t.Fatalf("tier = %q", standard.CertificationTier())
	}
	keys := module.OutputKeys()
	if len(keys) < len(platform.RequiredOutputKeys()) {
		t.Fatalf("output keys = %#v", keys)
	}
	for _, want := range []string{"mediaURL", "mediaBucket", platform.OutputDatabaseConnectionName, "securityPolicyName"} {
		found := false
		for _, key := range keys {
			if key == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("output keys = %#v, want %q", keys, want)
		}
	}
	if _, err := module.Program(nil); err == nil {
		t.Fatal("Program was accepted")
	}
}

func TestShimPlan(t *testing.T) {
	t.Parallel()
	caller := &stubCaller{respond: planResponder(cannedStoredPlan())}
	module := shimTestModule(t, caller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{AllowExpiredPreview: true})
	if err != nil {
		t.Fatal(err)
	}
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	if shim.Project() != "shop" || shim.Environment() != "staging" || shim.Region() != "europe-west1" {
		t.Fatalf("identity = %q %q %q", shim.Project(), shim.Environment(), shim.Region())
	}
	if shim.ImageDigest() == "" || shim.StateBackendURL() != "gs://shop-staging-state" {
		t.Fatalf("stored = %#v", shim.stored)
	}
	if shim.CertificationTier() != platform.TierExperimental {
		t.Fatalf("tier = %q", shim.CertificationTier())
	}
	if got := shim.TargetDescriptor(); got.ID != "gcp.gke-autopilot" {
		t.Fatalf("descriptor = %#v", got)
	}
	if len(caller.args) != 1 {
		t.Fatalf("calls = %d", len(caller.args))
	}
	captured, ok := caller.args[0].(*sdk.PlanRequest)
	if !ok {
		t.Fatalf("request = %T", caller.args[0])
	}
	if captured.Envelope.Project != "shop" || captured.Envelope.Preset != "standard" || captured.Envelope.AppVersion != "2.4.9" {
		t.Fatalf("envelope = %#v", captured.Envelope)
	}
	if captured.Application.Email.Mode != "smtp" || captured.Application.Email.Host != "smtp.example.com" || captured.Application.Email.Port != 587 || captured.Application.Email.Credential == "" {
		t.Fatalf("email = %#v", captured.Application.Email)
	}
	if captured.Application.Magento.FrontName != "admin_abc" || captured.Application.Magento.ConsumersMode != "processes" {
		t.Fatalf("application = %#v", captured.Application)
	}
	if !strings.Contains(string(captured.TargetBlock), "example-gcp-project") {
		t.Fatalf("target block = %s", captured.TargetBlock)
	}
	if captured.Runtime != "gke-autopilot" || captured.ValkeyRequirement != "" || !captured.AllowExpiredPreview || captured.AccountOnly || captured.RuntimeObserve {
		t.Fatalf("request = %#v", captured)
	}
}

func TestShimPlanDefaultsMissingTierToNegotiatedTier(t *testing.T) {
	t.Parallel()
	plan := cannedStoredPlan()
	plan.Tier = ""
	module := shimTestModule(t, &stubCaller{respond: planResponder(plan)})
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if planned.CertificationTier() != platform.TierBuiltIn {
		t.Fatalf("plan tier = %q, want negotiated built-in tier", planned.CertificationTier())
	}
}

func TestShimPlanOutputKeysFallBackWhenDescribeOmitsThem(t *testing.T) {
	t.Parallel()
	module := shimTestModule(t, &stubCaller{respond: planResponder(cannedStoredPlan())})
	module.client.describe.OutputKeys = nil
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	if !slices.Equal(shim.OutputKeys(), module.OutputKeys()) {
		t.Fatalf("planned output keys = %v, module fallback = %v", shim.OutputKeys(), module.OutputKeys())
	}
}

func TestAWSAccountOnlyAdmissionVerifiesIdentityBeforeBootstrapMutation(t *testing.T) {
	t.Parallel()
	stored := cannedStoredPlan()
	stored.Provider, stored.Runtime = "aws", "ecs-fargate"
	stored.Tier = sdk.ExtensionTierBuiltIn
	var methods []string
	caller := &stubCaller{respond: func(method string, reply any) error {
		methods = append(methods, method)
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: stored}
		case "Plugin.BootstrapVerify":
			*(reply.(*sdk.BootstrapVerifyResult)) = sdk.BootstrapVerifyResult{Verified: true}
		case "Plugin.BootstrapEnsure":
			*(reply.(*sdk.BootstrapEnsureResult)) = sdk.BootstrapEnsureResult{ResultJSON: []byte(`{"backendURL":"s3://state","keyRef":"key"}`)}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}}
	module := awsShimTestModule(t, caller)
	planned, err := module.Plan(awsShimTestConfig(), "staging", platform.PlanOptions{AccountOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	admitted, err := platform.AdmitPlan(context.Background(), module, planned)
	if err != nil {
		t.Fatal(err)
	}
	if admitted != planned {
		t.Fatal("account-only bootstrap admission should leave the plan unchanged")
	}
	bootstrap := module.Bootstrap()
	if err := bootstrap.VerifyAccount(context.Background(), admitted); err != nil {
		t.Fatal(err)
	}
	if _, err := bootstrap.Ensure(context.Background(), admitted, platform.BootstrapRequest{}); err != nil {
		t.Fatal(err)
	}
	if len(caller.args) != 3 {
		t.Fatalf("provider RPC count = %d", len(caller.args))
	}
	planRequest, ok := caller.args[0].(*sdk.PlanRequest)
	if !ok || planRequest.Admit || !planRequest.AccountOnly {
		t.Fatalf("account-only plan request = %#v", caller.args[0])
	}
	wantMethods := []string{"Plugin.Plan", "Plugin.BootstrapVerify", "Plugin.BootstrapEnsure"}
	if !slices.Equal(methods, wantMethods) {
		t.Fatalf("provider call order = %v, want %v", methods, wantMethods)
	}
}

func TestAWSShimValidatesFirstPartyEdgeAndObservabilityBeforePlanning(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*config.Config)
		want   string
	}{
		{
			name:   "edge",
			mutate: func(cfg *config.Config) { cfg.Edge.NativeProvider = "cloud-cdn" },
			want:   `native edge provider "cloud-cdn" does not match target.provider "aws"`,
		},
		{
			name:   "observability",
			mutate: func(cfg *config.Config) { cfg.Observability.NativeProvider = "google-cloud-operations" },
			want:   `observability provider "google-cloud-operations" requires target.provider gcp`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &stubCaller{respond: planResponder(cannedStoredPlan())}
			module := awsShimTestModule(t, caller)
			cfg := awsShimTestConfig()
			test.mutate(&cfg)
			if _, err := module.Plan(cfg, "staging", platform.PlanOptions{}); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Plan error = %v, want %q", err, test.want)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("provider called before host validation: %v", caller.calls)
			}
		})
	}
}

func TestAWSShimBuildsNativeEdgeAdapterFromDescribe(t *testing.T) {
	t.Parallel()
	stored := cannedStoredPlan()
	stored.Provider, stored.Runtime = "aws", "ecs-fargate"
	described := cannedDescribe()
	described.ProviderID = "aws"
	described.Edge = &sdk.EdgeAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "aws.edge.cloudfront", Provider: "aws", Version: "1.0.0"}
	caller := &stubCaller{respond: planResponder(stored)}
	module, err := newShimModule("aws", "ecs-fargate", platform.TierBuiltIn, awsOutputKeys(), &Client{caller: caller, describe: described})
	if err != nil {
		t.Fatal(err)
	}
	planned, err := module.Plan(awsShimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if module.Edge == nil {
		t.Fatal("AWS shim has no edge factory")
	}
	adapter, err := module.Edge(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.EdgeDescriptor().ID != described.Edge.ID {
		t.Fatalf("edge descriptor = %#v", adapter.EdgeDescriptor())
	}
}

func TestShimPreservesBootstrapEnvironmentAndUnlistedServiceVersion(t *testing.T) {
	caller := &stubCaller{respond: planResponder(cannedStoredPlan())}
	module := shimTestModule(t, caller)
	cfg := shimTestConfig()
	cfg.Class = "preview"
	cfg.StateEnvironment = "preview"
	cfg.Application.Version = "2.9.1"
	cfg.Services = map[string]sdk.ServiceBinding{"cache": {Family: "valkey", Version: "15.1"}}
	_, err := module.Plan(cfg, "pr-42", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	request := caller.args[0].(*sdk.PlanRequest)
	if request.StateEnvironment != "preview" || request.Envelope.Environment != "pr-42" || request.ValkeyRequirement != "15.1" {
		t.Fatalf("projected state or service input changed: %#v", request)
	}
}

func TestShimPlanForwardsRuntimeObserve(t *testing.T) {
	t.Parallel()
	caller := &stubCaller{respond: planResponder(cannedStoredPlan())}
	module := shimTestModule(t, caller)
	if _, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{RuntimeObserve: true}); err != nil {
		t.Fatal(err)
	}
	captured, ok := caller.args[0].(*sdk.PlanRequest)
	if !ok || !captured.RuntimeObserve {
		t.Fatalf("request = %#v", caller.args[0])
	}
}

func TestShimPlanRejects(t *testing.T) {
	t.Parallel()
	module := shimTestModule(t, &stubCaller{})
	aws := shimTestConfig()
	aws.Target.Provider = "aws"
	if _, err := module.Plan(aws, "staging", platform.PlanOptions{}); err == nil {
		t.Fatal("aws target was accepted")
	}
	standard := shimTestConfig()
	standard.Target.Runtime = "gke-standard"
	if _, err := module.Plan(standard, "staging", platform.PlanOptions{}); err == nil {
		t.Fatal("runtime mismatch was accepted")
	}
	missing := shimTestConfig()
	missing.Target.GCP = nil
	if _, err := module.Plan(missing, "staging", platform.PlanOptions{}); err == nil {
		t.Fatal("missing target block was accepted")
	}
	failing := shimTestModule(t, &stubCaller{respond: func(string, any) error {
		return errors.New("transport boom")
	}})
	if _, err := failing.Plan(shimTestConfig(), "staging", platform.PlanOptions{}); err == nil {
		t.Fatal("transport failure was accepted")
	}
	serverInvalid := shimTestModule(t, &stubCaller{respond: func(method string, reply any) error {
		*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Error: &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: "bad preset"}}
		return nil
	}})
	if _, err := serverInvalid.Plan(shimTestConfig(), "staging", platform.PlanOptions{}); err == nil {
		t.Fatal("server invalid was accepted")
	} else if pluginErr, ok := err.(*PluginError); !ok || pluginErr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("error = %#v", err)
	}
}

func TestShimWithReplans(t *testing.T) {
	t.Parallel()
	caller := &stubCaller{respond: planResponder(cannedStoredPlan())}
	module := shimTestModule(t, caller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	next, err := shim.WithImageDigest("ghcr.io/magelift/magento@sha256:" + strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := AsShimPlanned(next); !ok {
		t.Fatalf("replanned = %T", next)
	}
	if len(caller.args) != 2 {
		t.Fatalf("calls = %d", len(caller.args))
	}
	retargeted := caller.args[1].(*sdk.PlanRequest)
	if !strings.Contains(string(retargeted.TargetBlock), strings.Repeat("b", 64)) {
		t.Fatalf("retargeted block = %s", retargeted.TargetBlock)
	}
	live, err := shim.WithLiveQueueReplicas(3)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := AsShimPlanned(live); !ok {
		t.Fatalf("replanned = %T", live)
	}
	if len(caller.args) != 3 {
		t.Fatalf("calls = %d", len(caller.args))
	}
	if got := caller.args[2].(*sdk.PlanRequest).LiveQueueReplicas; got != 3 {
		t.Fatalf("live replicas = %d", got)
	}
}

type fakeForeignPlanned struct{}

func (fakeForeignPlanned) StackName() string        { return "foreign" }
func (fakeForeignPlanned) Provider() sdk.ProviderID { return "aws" }
func (fakeForeignPlanned) Runtime() sdk.RuntimeID   { return "ecs-fargate" }
func (fakeForeignPlanned) Project() string          { return "shop" }
func (fakeForeignPlanned) Environment() string      { return "staging" }
func (fakeForeignPlanned) Region() string           { return "eu-west-1" }
func (fakeForeignPlanned) CertificationTier() platform.CertificationTier {
	return platform.TierExperimental
}
func (fakeForeignPlanned) EnvironmentClass() string { return "staging" }
func (fakeForeignPlanned) Protected() bool          { return false }
func (fakeForeignPlanned) ImageDigest() string      { return "" }
func (fakeForeignPlanned) WithImageDigest(string) (platform.PlannedStack, error) {
	return fakeForeignPlanned{}, nil
}
func (fakeForeignPlanned) TargetDescriptor() sdk.TargetDescriptor {
	return sdk.TargetDescriptor{ID: "aws.ecs-fargate", Provider: "aws", Runtime: "ecs-fargate"}
}

type stubKubeBackend struct {
	outputs map[string]any
}

func (b *stubKubeBackend) Outputs(context.Context) (map[string]any, error) {
	return b.outputs, nil
}

func (b *stubKubeBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

func (b *stubKubeBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

func (b *stubKubeBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

func TestShimOpsDeploySteps(t *testing.T) {
	t.Parallel()
	deployInputs, err := json.Marshal(kube.DeploySpec{ImageDigest: "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64), DatabaseName: "magento", Region: "europe-west1"})
	if err != nil {
		t.Fatal(err)
	}
	stored := cannedStoredPlan()
	stored.DeployInputsJSON = deployInputs
	caller := &stubCaller{respond: planResponder(stored)}
	module := shimTestModule(t, caller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	steps, err := module.Ops().NewDeploySteps(context.Background(), &stubKubeBackend{}, planned, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steps.(*shimDeploySteps); !ok {
		t.Fatalf("steps = %T", steps)
	}
	if _, err := module.Ops().NewDeploySteps(context.Background(), struct{}{}, planned, io.Discard); err == nil || !strings.Contains(err.Error(), "backend with outputs") {
		t.Fatalf("wrong backend error = %v", err)
	}
}

func TestShimDeployStepsDrivePluginPhases(t *testing.T) {
	t.Parallel()
	deployInputs, err := json.Marshal(sdk.DeployInputs{ImageDigest: "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64), DatabaseName: "magento", Region: "europe-west1"})
	if err != nil {
		t.Fatal(err)
	}
	stored := cannedStoredPlan()
	stored.Provider = "aws"
	stored.DeployInputsJSON = deployInputs
	var phases []string
	caller := &stubCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: stored}
			return nil
		case "Plugin.DeployAppPhase":
			phases = append(phases, "phase")
			state := `{"candidate":true}`
			if len(phases) >= 2 {
				state = `{"migrated":true}`
			}
			*(reply.(*sdk.DeployAppPhaseResult)) = sdk.DeployAppPhaseResult{StateJSON: []byte(state), Message: "ok"}
			return nil
		default:
			return errors.New("unexpected method " + method)
		}
	}}
	module := shimTestModule(t, caller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	backend := &stubKubeBackend{outputs: map[string]any{"clusterName": "c"}}
	steps, err := module.Ops().NewDeploySteps(context.Background(), backend, planned, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	request := deployRequestForTest(stored.ImageDigest)
	if err := steps.RegisterCandidate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := steps.RunMigrations(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := steps.Stabilize(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := steps.Health(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(phases) != 4 {
		t.Fatalf("phase calls = %d, want 4", len(phases))
	}
	var phaseCalls []sdk.DeployAppPhaseCall
	for _, arg := range caller.args {
		if call, ok := arg.(*sdk.DeployAppPhaseCall); ok {
			phaseCalls = append(phaseCalls, *call)
		}
	}
	if len(phaseCalls) != 4 {
		t.Fatalf("phase requests = %d, want 4", len(phaseCalls))
	}
	for i, phase := range []sdk.DeployAppPhase{sdk.DeployPhaseRegister, sdk.DeployPhaseMigrate, sdk.DeployPhaseStabilize, sdk.DeployPhaseHealth} {
		if phaseCalls[i].Phase != phase {
			t.Errorf("phase request %d = %q, want %q", i, phaseCalls[i].Phase, phase)
		}
		wantState := `{"candidate":true}`
		if i > 1 {
			wantState = `{"migrated":true}`
		}
		if i > 0 && string(phaseCalls[i].StateJSON) != wantState {
			t.Errorf("phase %q candidate state = %s", phase, phaseCalls[i].StateJSON)
		}
	}
	if len(steps.(*shimDeploySteps).state) != 0 {
		t.Fatal("candidate state retained after successful health check")
	}
	// Stored inputs pass through undecoded: corrupt bytes still build
	// steps (the plugin validates at execution).
	stored.DeployInputsJSON = []byte(`{invalid`)
	caller.respond = func(method string, reply any) error {
		*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: stored}
		return nil
	}
	replanned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := module.Ops().NewDeploySteps(context.Background(), backend, replanned, io.Discard); err != nil {
		t.Fatalf("opaque inputs refused: %v", err)
	}
}

func TestShimDeployStepsClearStateWhenMigrationReturnsNoContinuation(t *testing.T) {
	t.Parallel()
	stored := cannedStoredPlan()
	var phaseIndex int
	caller := &stubCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: stored}
		case "Plugin.DeployAppPhase":
			phaseIndex++
			state := []byte(nil)
			if phaseIndex == 1 {
				state = []byte(`{"candidate":true}`)
			}
			*(reply.(*sdk.DeployAppPhaseResult)) = sdk.DeployAppPhaseResult{StateJSON: state}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}}
	module := shimTestModule(t, caller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	backend := &stubKubeBackend{outputs: map[string]any{"clusterName": "c"}}
	steps, err := module.Ops().NewDeploySteps(context.Background(), backend, planned, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	request := deployRequestForTest(stored.ImageDigest)
	if err := steps.RegisterCandidate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := steps.RunMigrations(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := steps.Stabilize(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := steps.Health(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	var phases []sdk.DeployAppPhaseCall
	for _, arg := range caller.args {
		if call, ok := arg.(*sdk.DeployAppPhaseCall); ok {
			phases = append(phases, *call)
		}
	}
	if len(phases) != 4 || string(phases[0].StateJSON) != "" || string(phases[1].StateJSON) != `{"candidate":true}` || len(phases[2].StateJSON) != 0 || len(phases[3].StateJSON) != 0 {
		t.Fatalf("phase state sequence = %#v", phases)
	}
	if len(steps.(*shimDeploySteps).state) != 0 {
		t.Fatal("candidate state retained after successful health check")
	}
}

func TestShimOpsAcquireLock(t *testing.T) {
	t.Parallel()
	var unlocked int
	caller := &stubCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Plan":
			*(reply.(*sdk.PlanResult)) = sdk.PlanResult{Plan: cannedStoredPlan()}
			return nil
		case "Plugin.StateLock":
			*(reply.(*sdk.StateLockResult)) = sdk.StateLockResult{Locked: true, ReleaseToken: "receipt-1"}
			return nil
		case "Plugin.StateUnlock":
			unlocked++
			*(reply.(*sdk.StateUnlockResult)) = sdk.StateUnlockResult{}
			return nil
		default:
			return errors.New("unexpected method " + method)
		}
	}}
	module := shimTestModule(t, caller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	release, err := module.Ops().AcquireLock(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 2 || caller.calls[1] != "Plugin.StateLock" {
		t.Fatalf("calls = %#v", caller.calls)
	}
	lockReq, ok := caller.args[1].(*sdk.StateLockCall)
	if !ok || !strings.HasPrefix(lockReq.Owner, "magelift-cli-") {
		t.Fatalf("lock owner = %#v", caller.args[1])
	}
	if err := release(context.Background()); err != nil {
		t.Fatal(err)
	}
	if unlocked != 1 {
		t.Fatalf("unlocked = %d", unlocked)
	}
	unlockRequest, ok := caller.args[2].(*sdk.StateUnlockCall)
	if !ok || unlockRequest.ReleaseToken != "receipt-1" {
		t.Fatalf("unlock release token = %#v", caller.args[2])
	}
}

func TestShimCarriesServiceBindingsAcrossReplan(t *testing.T) {
	caller := &stubCaller{respond: planResponder(cannedStoredPlan())}
	module := shimTestModule(t, caller)
	cfg := shimTestConfig()
	cfg.Services = map[string]config.ServiceBinding{"search": {Family: "opensearch", Ownership: "external", Endpoint: "https://search.example"}}
	planned, err := module.Plan(cfg, "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Services["search"] = config.ServiceBinding{Family: "other"}
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	if _, err := shim.WithImageDigest("ghcr.io/magelift/magento@sha256:" + strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	for _, arg := range caller.args {
		request := arg.(*sdk.PlanRequest)
		if request.Services["search"].Endpoint != "https://search.example" {
			t.Fatalf("request services = %#v", request.Services)
		}
	}
}
