package plugin

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"google.golang.org/api/googleapi"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/cloud/kube"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	"github.com/magelift/magelift/sdk"
)

func testSpec() gcpstack.Spec {
	return gcpstack.Spec{
		Identity: gcpstack.Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "staging",
			Region: "europe-west1", EnvironmentClass: "staging", Preset: sdk.PresetStandard,
			Runtime: "gke-autopilot", Labels: map[string]string{"magelift-managed-by": "magelift"},
		},
		Application:  gcpstack.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     gcpstack.Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64)},
		Policy:       gcpstack.NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Catalog:      gcpstack.CatalogSelection{CloudSQLTier: "db-perf-optimized-N-2", CloudSQLAvailability: "REGIONAL", ValkeyRequirement: "9", MemorystoreEngineVersion: "VALKEY_9_0", MemorystoreNodeType: "STANDARD_SMALL", AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi", DesiredWebReplicas: 2},
		Dependencies: gcpstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-crypt-key"},
	}
}

func testEnvelope() sdk.Envelope {
	return sdk.Envelope{
		Project: "shop", Environment: "staging", Region: "europe-west1",
		EnvironmentClass: "staging", StackName: "shop-staging-gcp-gke-autopilot",
		StateBackendURL: "gs://shop-staging-gcp-state",
		Preset:          "standard", AppVersion: "2.4.9",
	}
}

func storedTestPlan(t *testing.T, spec gcpstack.Spec) sdk.StoredPlan {
	t.Helper()
	opaque, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	planned := gcpstack.SpecPlanned{Spec: spec}
	return sdk.StoredPlan{
		StackName: planned.StackName(), Provider: "gcp", Runtime: string(planned.Runtime()),
		ImageDigest: spec.Artifact.ImageDigest, StateBackendURL: "gs://shop-staging-gcp-state",
		Opaque: opaque,
	}
}

func testTargetBlock() []byte {
	return []byte("project: example-gcp-project\nregion: europe-west1\nnetworkCidr: 10.20.0.0/16\nimageDigest: ghcr.io/magelift/magento@sha256:" + strings.Repeat("a", 64) + "\nencryptionKeySecret: magento-crypt-key\n")
}

type stubAdmission struct {
	admit func(gcpstack.Spec) (gcpstack.Spec, error)
}

func (s stubAdmission) AdmitSpec(_ context.Context, spec gcpstack.Spec) (gcpstack.Spec, error) {
	if s.admit != nil {
		return s.admit(spec)
	}
	return spec, nil
}

type stubAutomation struct {
	preview map[string]int
	update  map[string]int
	destroy map[string]int
	outputs auto.OutputMap
	err     error
	calls   []string
}

func (s *stubAutomation) Preview(_ context.Context, _ automation.Request, _ io.Writer) (map[string]int, error) {
	s.calls = append(s.calls, "preview")
	if s.err != nil {
		return nil, s.err
	}
	return s.preview, nil
}

func (s *stubAutomation) Update(_ context.Context, _ automation.Request, _ io.Writer) (map[string]int, error) {
	s.calls = append(s.calls, "update")
	if s.err != nil {
		return nil, s.err
	}
	return s.update, nil
}

func (s *stubAutomation) Destroy(_ context.Context, _ automation.Request, _ io.Writer) (map[string]int, error) {
	s.calls = append(s.calls, "destroy")
	if s.err != nil {
		return nil, s.err
	}
	return s.destroy, nil
}

func (s *stubAutomation) Outputs(context.Context) (auto.OutputMap, error) {
	s.calls = append(s.calls, "outputs")
	if s.err != nil {
		return nil, s.err
	}
	return s.outputs, nil
}

func TestDescribe(t *testing.T) {
	t.Parallel()
	result, operr := (&Server{Version: "v1.2.3"}).Describe(context.Background(), &sdk.DescribeRequest{ProtocolVersion: sdk.ProtocolV1})
	if operr != nil {
		t.Fatal(operr)
	}
	if result.ProviderID != "gcp" || result.ProviderVersion != "v1.2.3" || result.ProtocolVersion != sdk.ProtocolV1 {
		t.Fatalf("identity = %#v", result)
	}
	if len(result.Operations) != 33 {
		t.Fatalf("operations = %d, want 33", len(result.Operations))
	}
	advertised := false
	for _, operation := range result.Operations {
		if operation.Version != "1.0" {
			t.Fatalf("operation %q version = %q", operation.Name, operation.Version)
		}
		if operation.Name == string(sdk.OpDeployAppPhase) {
			advertised = true
		}
	}
	if !advertised {
		t.Fatal("deploy-app-phase is not advertised")
	}
	if len(result.Runtimes) != 2 {
		t.Fatalf("runtimes = %#v", result.Runtimes)
	}
	tiers := map[string]sdk.ExtensionCertificationTier{}
	for _, runtime := range result.Runtimes {
		tiers[runtime.Runtime] = runtime.Tier
	}
	if tiers["gke-autopilot"] != sdk.ExtensionTierCertified || tiers["gke-standard"] != sdk.ExtensionTierExperimental {
		t.Fatalf("runtime tiers = %#v", tiers)
	}
	if len(result.OutputKeys) == 0 {
		t.Fatal("output keys are empty")
	}
	if result.Edge == nil || result.Edge.Provider != "gcp" || result.Resilience == nil || result.Resilience.Provider != "gcp" {
		t.Fatalf("adapter descriptors = %#v %#v", result.Edge, result.Resilience)
	}
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()
	server := &Server{}
	result, operr := server.ValidateConfig(context.Background(), &sdk.ValidateConfigRequest{ProtocolVersion: sdk.ProtocolV1, Runtime: "gke-autopilot", TargetBlock: testTargetBlock()})
	if operr != nil {
		t.Fatal(operr)
	}
	if !result.Valid {
		t.Fatalf("problems = %#v", result.Problems)
	}
	empty, operr := server.ValidateConfig(context.Background(), &sdk.ValidateConfigRequest{ProtocolVersion: sdk.ProtocolV1, Runtime: "gke-autopilot", TargetBlock: []byte("region: europe-west1\n")})
	if operr != nil {
		t.Fatal(operr)
	}
	if empty.Valid || len(empty.Problems) == 0 {
		t.Fatal("missing project was accepted")
	}
	if _, operr := server.ValidateConfig(context.Background(), &sdk.ValidateConfigRequest{ProtocolVersion: sdk.ProtocolV1, TargetBlock: []byte(":\t:")}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("malformed YAML error = %v", operr)
	}
}

func testPlanRequest() *sdk.PlanRequest {
	return &sdk.PlanRequest{
		ProtocolVersion:   sdk.ProtocolV1,
		Envelope:          testEnvelope(),
		Application:       sdk.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		TargetBlock:       testTargetBlock(),
		Runtime:           "gke-autopilot",
		ValkeyRequirement: "9",
	}
}

func TestPlanAccountOnlySkipsImageAndEncryptionKey(t *testing.T) {
	t.Parallel()
	server := &Server{Admission: stubAdmission{}}
	request := testPlanRequest()
	request.AccountOnly = true
	request.TargetBlock = []byte("project: example-gcp-project\nregion: europe-west1\n")
	result, operr := server.Plan(context.Background(), request)
	if operr != nil {
		t.Fatal(operr)
	}
	var spec gcpstack.Spec
	if err := json.Unmarshal(result.Plan.Opaque, &spec); err != nil {
		t.Fatal(err)
	}
	if !spec.AccountOnly || spec.Artifact.ImageDigest != "" || spec.Dependencies.EncryptionKeySecret != "" {
		t.Fatalf("account plan = %#v", spec)
	}
}

func TestPlan(t *testing.T) {
	t.Parallel()
	server := &Server{Admission: stubAdmission{}}
	result, operr := server.Plan(context.Background(), testPlanRequest())
	if operr != nil {
		t.Fatal(operr)
	}
	if result.Plan.StackName == "" || result.Plan.StateBackendURL == "" || len(result.Plan.Opaque) == 0 {
		t.Fatalf("plan = %#v", result.Plan)
	}
	if result.Plan.Tier != sdk.ExtensionTierExperimental {
		t.Fatalf("standard plan tier = %q, want experimental", result.Plan.Tier)
	}
	var spec gcpstack.Spec
	if err := json.Unmarshal(result.Plan.Opaque, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.Identity.GCPProject != "example-gcp-project" || spec.Catalog.MemorystoreEngineVersion != "VALKEY_9_0" {
		t.Fatalf("spec = %#v", spec.Identity)
	}
	var deploy kube.DeploySpec
	if err := json.Unmarshal(result.Plan.DeployInputsJSON, &deploy); err != nil {
		t.Fatal(err)
	}
	if deploy.CloudProject != "example-gcp-project" || deploy.Region != "europe-west1" {
		t.Fatalf("deploy inputs = %#v", deploy)
	}
}

func TestPlanRejectsBadBlock(t *testing.T) {
	t.Parallel()
	server := &Server{Admission: stubAdmission{}}
	bad := testPlanRequest()
	bad.TargetBlock = []byte("region: europe-west1\n")
	if _, operr := server.Plan(context.Background(), bad); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("missing project error = %v", operr)
	}
	bad.TargetBlock = []byte(":\t:")
	if _, operr := server.Plan(context.Background(), bad); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("malformed YAML error = %v", operr)
	}
	failing := &Server{Admission: stubAdmission{admit: func(gcpstack.Spec) (gcpstack.Spec, error) {
		return gcpstack.Spec{}, &googleapi.Error{Code: 403, Message: "forbidden"}
	}}}
	if _, operr := failing.Plan(context.Background(), testPlanRequest()); operr == nil || operr.Code != sdk.ErrCodeCredential {
		t.Fatalf("admission error = %v", operr)
	}
}

func TestPreviewApplyAndDestroy(t *testing.T) {
	t.Parallel()
	backend := &stubAutomation{preview: map[string]int{"create": 3}, update: map[string]int{"create": 3, "same": 10}, destroy: map[string]int{"delete": 3}}
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) { return backend, nil }}
	envelope, plan := testEnvelope(), storedTestPlan(t, testSpec())
	envelope.StackName = plan.StackName
	previewed, operr := server.Preview(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if previewed.Summary.Create != 3 {
		t.Fatalf("preview summary = %#v", previewed.Summary)
	}
	applied, operr := server.Apply(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if applied.Summary.Create != 3 || applied.Summary.Same != 10 {
		t.Fatalf("summary = %#v", applied.Summary)
	}
	destroyed, operr := server.Destroy(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if destroyed.Summary.Delete != 3 {
		t.Fatalf("summary = %#v", destroyed.Summary)
	}
	if len(backend.calls) != 3 || backend.calls[0] != "preview" || backend.calls[1] != "update" || backend.calls[2] != "destroy" {
		t.Fatalf("calls = %#v", backend.calls)
	}
}

func TestSummarizeChanges(t *testing.T) {
	t.Parallel()
	summary := summarizeChanges(map[string]int{"create": 1, "update": 2, "delete": 3, "replace": 4, "same": 5, "mystery": 9})
	if summary.Create != 1 || summary.Update != 2 || summary.Delete != 3 || summary.Replace != 4 || summary.Same != 5 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestStackCallIntegrity(t *testing.T) {
	t.Parallel()
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) {
		return &stubAutomation{}, nil
	}}
	_, plan := testEnvelope(), storedTestPlan(t, testSpec())
	wrong := testEnvelope()
	wrong.Environment = "production"
	if _, operr := server.Apply(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: wrong, Plan: plan}); operr == nil || operr.Code != sdk.ErrCodeIntegrity {
		t.Fatalf("envelope mismatch error = %v", operr)
	}
	corrupt := plan
	corrupt.Opaque = []byte("{nope")
	if _, operr := server.Outputs(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), Plan: corrupt}); operr == nil || operr.Code != sdk.ErrCodeIntegrity {
		t.Fatalf("corrupt plan error = %v", operr)
	}
	empty := plan
	empty.Opaque = nil
	if _, operr := server.Destroy(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), Plan: empty}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("empty plan error = %v", operr)
	}
	badMeta := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), Plan: plan, PreviewMetadataJSON: []byte("{nope")}
	badMeta.Envelope.StackName = plan.StackName
	if _, operr := server.Apply(context.Background(), badMeta); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("preview metadata error = %v", operr)
	}
}

func TestOutputs(t *testing.T) {
	t.Parallel()
	backend := &stubAutomation{outputs: auto.OutputMap{
		"clusterName": {Value: "shop-staging-gke"},
		"kubeconfig":  {Value: "secret-config", Secret: true},
	}}
	server := &Server{NewStack: func(context.Context, string, gcpstack.Spec, string) (Automation, error) { return backend, nil }}
	envelope, plan := testEnvelope(), storedTestPlan(t, testSpec())
	envelope.StackName = plan.StackName
	result, operr := server.Outputs(context.Background(), &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	var values map[string]any
	if err := json.Unmarshal(result.ValuesJSON, &values); err != nil {
		t.Fatal(err)
	}
	if values["clusterName"] != "shop-staging-gke" || values["kubeconfig"] != "secret-config" {
		t.Fatalf("values = %v", values)
	}
	if len(result.SecretKeys) != 1 || result.SecretKeys[0] != "kubeconfig" {
		t.Fatalf("secret keys = %#v", result.SecretKeys)
	}
}
