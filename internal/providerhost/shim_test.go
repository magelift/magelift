package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func shimTestConfig() config.Config {
	return config.Config{
		SchemaVersion: 1,
		Project:       config.Project{Name: "shop"},
		Application: config.Application{
			Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm",
			Magento: config.MagentoRuntime{FrontName: "admin_abc", Consumers: config.MagentoConsumers{Mode: "processes"}},
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

func cannedStoredPlan() sdk.StoredPlan {
	return sdk.StoredPlan{
		StackName: "shop-staging-gcp-gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot",
		Tier: sdk.ExtensionTierExperimental, ImageDigest: "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64),
		StateBackendURL: "gs://shop-staging-state", Opaque: []byte(`{"stored":true}`),
	}
}

func shimTestModule(t *testing.T, caller *stubCaller) *ShimModule {
	t.Helper()
	module, err := NewShimModule("gke-autopilot", &Client{caller: caller, describe: cannedDescribe()})
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
	if _, err := NewShimModule("gke-autopilot", nil); err == nil {
		t.Fatal("nil client was accepted")
	}
	if _, err := NewShimModule("gke-autopilot", &Client{}); err != nil {
		t.Fatalf("lazy client was refused: %v", err)
	}
	if _, err := NewShimModule("ecs-fargate", &Client{describe: cannedDescribe()}); err == nil {
		t.Fatal("unserved runtime was accepted")
	}
	module := shimTestModule(t, &stubCaller{})
	if got := module.Descriptor(); got.ID != "gcp.gke-autopilot" || got.Provider != "gcp" || got.Runtime != "gke-autopilot" {
		t.Fatalf("descriptor = %#v", got)
	}
	if module.CertificationTier() != platform.TierCertified {
		t.Fatalf("tier = %q", module.CertificationTier())
	}
	standard, err := NewShimModule("gke-standard", &Client{describe: cannedDescribe()})
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
	if captured.Application.Magento.FrontName != "admin_abc" || captured.Application.Magento.ConsumersMode != "processes" {
		t.Fatalf("application = %#v", captured.Application)
	}
	if !strings.Contains(string(captured.TargetBlock), "example-gcp-project") {
		t.Fatalf("target block = %s", captured.TargetBlock)
	}
	if captured.Runtime != "gke-autopilot" || captured.ValkeyRequirement != "9" || !captured.AllowExpiredPreview {
		t.Fatalf("request = %#v", captured)
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
	if _, ok := steps.(*kube.Steps); !ok {
		t.Fatalf("steps = %T", steps)
	}
	if _, err := module.Ops().NewDeploySteps(context.Background(), struct{}{}, planned, io.Discard); err == nil || !strings.Contains(err.Error(), "backend with outputs") {
		t.Fatalf("wrong backend error = %v", err)
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
			*(reply.(*sdk.StateLockResult)) = sdk.StateLockResult{Locked: true}
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
}
