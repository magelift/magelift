package plugin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"gopkg.in/yaml.v3"
)

func shopRequest(t *testing.T, runtime string) *sdk.PlanRequest {
	t.Helper()
	const key = "00000000-0000-0000-0000-000000000000"
	const arn = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:"
	target := config.AWSTarget{VPCCIDR: "10.42.0.0/16", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"},
		ImageDigest:  "registry.example.test/shop@sha256:" + strings.Repeat("7", 64),
		KMSKeyARN:    "arn:aws:kms:eu-west-3:123456789012:key/" + key,
		HostedZoneID: "Z123456789", CloudFrontCertificateARN: "arn:aws:acm:us-east-1:123456789012:certificate/" + key, ALBCertificateARN: "arn:aws:acm:eu-west-3:123456789012:certificate/" + key,
		DatabaseName: "magento", MasterUsername: "magento", MediaDomain: "media.example.test",
		CacheSecretARN: arn + "cache", SessionSecretARN: arn + "session", QueueSecretARN: arn + "queue", EncryptionKeySecretARN: arn + "crypt",
		Catalog: config.AWSCatalog{Version: "v1", Aurora: config.AWSCatalogAurora{InstanceClass: "db.t4g.micro", InstanceCount: 1},
			Valkey:    config.AWSCatalogValkey{NodeType: "cache.t4g.micro"},
			Fargate:   config.AWSCatalogFargate{CPU: 512, MemoryMiB: 1024, DesiredCount: 1},
			Retention: config.AWSCatalogRetention{LogDays: 7, BackupDays: 1, ArtifactDays: 7},
			Versions:  config.AWSCatalogVersions{MySQL: "8.4.10", Valkey: "8.1", RabbitMQ: "3.13"}, DatabaseEngine: "rds-mysql", SearchMode: "disabled", QueueMode: "db"}}
	block, err := yaml.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	return &sdk.PlanRequest{ProtocolVersion: sdk.ProtocolV1, Runtime: runtime, TargetBlock: block, UseProviderDefaultEdge: true,
		Envelope:    sdk.Envelope{Project: "shop", Environment: "pr-23", StateBackendURL: "", Account: "123456789012", Region: "eu-west-3", EnvironmentClass: "preview", MonthlyBudgetCents: 5000, Preset: "preview", AppVersion: "2.4.9", ExpiresAt: "2099-01-01T00:00:00Z", Domain: "pr-23.example.test", StackName: platform.FormatStackName("shop", "pr-23", "aws", sdk.RuntimeID(runtime))},
		Application: sdk.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"}, StateEnvironment: "preview",
	}
}

func TestAWSPlansAreOfflineAndRetainTheSelectedRuntime(t *testing.T) {
	for _, runtime := range []string{"ecs-fargate", "eks"} {
		t.Run(runtime, func(t *testing.T) {
			req := shopRequest(t, runtime)
			s := Server{admit: func(context.Context, platform.StackModule, platform.PlannedStack) (platform.PlannedStack, error) {
				t.Fatal("local planning contacted AWS")
				return nil, nil
			}}
			var result sdk.PlanResult
			if err := s.Plan(req, &result); err != nil || result.Error != nil {
				t.Fatalf("plan: %v %v", err, result.Error)
			}
			body, _, planned, err := checked(req.Envelope, result.Plan)
			if err != nil || planned.Runtime() != sdk.RuntimeID(runtime) || !strings.Contains(result.Plan.StateBackendURL, "s3://") {
				t.Fatalf("stored plan %s: %v", runtime, err)
			}
			if !strings.Contains(planned.(interface{ StateBackendURL() string }).StateBackendURL(), "shop-preview-") {
				t.Fatalf("wrong shared state bucket for %s: %s", runtime, result.Plan.StateBackendURL)
			}
			if body.ECS != nil && (body.ECS.Identity.StateEnvironment != "preview" || body.ECS.Edge.NativeProvider != "cloudfront-waf") {
				t.Fatalf("ECS plan lost its shared backend or default edge: %#v", body.ECS.Identity)
			}
			var change sdk.LifecycleResult
			if err := s.Apply(&sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: req.Envelope, Plan: result.Plan}, &change); err != nil || change.Error == nil || change.Error.Code != sdk.ErrCodeInvalid {
				t.Fatalf("apply without admission: %v %v", err, change.Error)
			}
			wrong := req.Envelope
			wrong.Account = "999999999999"
			if _, _, _, err := checked(wrong, result.Plan); err == nil {
				t.Fatal("stored plan accepted another account")
			}
		})
	}
}

func TestAWSAdmittedPlanRetainsServicesAndManagedEmail(t *testing.T) {
	req := shopRequest(t, "ecs-fargate")
	req.Admit = true
	req.Application.Email = sdk.EmailSettings{Mode: "ses", From: "shop@example.test", ManagedDomain: "example.test", ManagedHostedZoneID: "Z123456789"}
	calls := 0
	s := Server{admit: func(_ context.Context, _ platform.StackModule, p platform.PlannedStack) (platform.PlannedStack, error) {
		calls++
		return p, nil
	}}
	var result sdk.PlanResult
	if err := s.Plan(req, &result); err != nil || result.Error != nil {
		t.Fatalf("plan: %v %v", err, result.Error)
	}
	var b stored
	if err := json.Unmarshal(result.Plan.Opaque, &b); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !b.Admitted || b.ECS.Email.Domain != "example.test" || b.ECS.Dependencies.SessionSecretARN == b.ECS.Dependencies.CacheSecretARN {
		t.Fatal("admission lost the account, managed sender or session settings")
	}
}

func TestAWSTeardownRejectsApplyAndProtectedDestroy(t *testing.T) {
	req := shopRequest(t, "ecs-fargate")
	req.AllowExpiredPreview = true
	req.Envelope.Protected = true
	var s Server
	var p sdk.PlanResult
	if err := s.Plan(req, &p); err != nil || p.Error != nil {
		t.Fatalf("teardown: %v %v", err, p.Error)
	}
	call := &sdk.StackCall{ProtocolVersion: sdk.ProtocolV1, Envelope: req.Envelope, Plan: p.Plan}
	for _, operation := range []func(*sdk.StackCall, *sdk.LifecycleResult) error{s.Destroy, s.Apply} {
		var reply sdk.LifecycleResult
		if err := operation(call, &reply); err != nil || reply.Error == nil || reply.Error.Code != sdk.ErrCodeInvalid {
			t.Fatalf("unsafe teardown operation: %v %v", err, reply.Error)
		}
	}
}

func TestStateOnlyPlansDoNotRequireAnImageOrNetwork(t *testing.T) {
	for _, runtime := range []string{"ecs-fargate", "eks"} {
		t.Run(runtime, func(t *testing.T) {
			req := shopRequest(t, runtime)
			req.AccountOnly = true
			req.TargetBlock = []byte("{}")
			var result sdk.PlanResult
			if err := new(Server).Plan(req, &result); err != nil || result.Error != nil {
				t.Fatalf("state plan: %v %v", err, result.Error)
			}
			if result.Plan.ImageDigest != "" || !strings.Contains(result.Plan.StateBackendURL, "shop-preview-") {
				t.Fatalf("state backend = %q, image = %q", result.Plan.StateBackendURL, result.Plan.ImageDigest)
			}
		})
	}
}

func TestAWSPlanRejectsCloudSettingsFromAnotherProvider(t *testing.T) {
	for _, runtime := range []string{"ecs-fargate", "eks"} {
		for _, change := range []func(*sdk.PlanRequest){func(r *sdk.PlanRequest) { r.Edge.NativeProvider = "cloud-cdn" }, func(r *sdk.PlanRequest) { r.Observability.NativeProvider = "google-cloud-operations" }} {
			req := shopRequest(t, runtime)
			change(req)
			var result sdk.PlanResult
			if err := new(Server).Plan(req, &result); err != nil || result.Error == nil || result.Error.Code != sdk.ErrCodeInvalid {
				t.Fatalf("%s accepted mismatched telemetry or edge: %v %v", runtime, err, result.Error)
			}
		}
	}
}

func TestAttachedDatabaseSurvivesTeardownPlanning(t *testing.T) {
	req := shopRequest(t, "ecs-fargate")
	req.AllowExpiredPreview = true
	target, err := parseTarget(req.TargetBlock)
	if err != nil {
		t.Fatal(err)
	}
	target.Existing.Database = &config.AWSExistingDatabase{Provider: "aws", Kind: "database", ExternalID: "existing-shop-db"}
	req.TargetBlock, err = yaml.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var result sdk.PlanResult
	if err := new(Server).Plan(req, &result); err != nil || result.Error != nil {
		t.Fatalf("teardown plan: %v %v", err, result.Error)
	}
	if len(result.Plan.Adopted) != 1 || result.Plan.Adopted[0].Line() != "ADOPT database existing-shop-db" {
		t.Fatalf("missing existing database: %v", result.Plan.Adopted)
	}
	result.Plan.Adopted = nil
	if _, _, _, err := checked(req.Envelope, result.Plan); err == nil {
		t.Fatal("edited adoption report accepted")
	}
}
