package deployment

import (
	"context"
	"io"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type stepsBackend struct {
	outputs      map[string]any
	updateCalls  int
	afterUpdate  map[string]any
	candidateErr error
}

func (b *stepsBackend) Outputs(context.Context) (map[string]any, error) { return b.outputs, nil }
func (*stepsBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (b *stepsBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	b.updateCalls++
	if b.afterUpdate != nil {
		b.outputs = b.afterUpdate
	}
	return map[string]int{"create": 1}, nil
}
func (*stepsBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

type stepsRuntime struct{}

func (stepsRuntime) Check(context.Context, string, string) (awsoperations.ServiceHealth, error) {
	return awsoperations.ServiceHealth{DesiredCount: 1, RunningCount: 1, PrimaryRollout: "COMPLETED"}, nil
}

func TestRequiredOutputDecoding(t *testing.T) {
	outputs := map[string]any{"name": "shop", "subnets": []any{"subnet-a", "subnet-b"}}
	if got, err := requiredString(outputs, "name"); err != nil || got != "shop" {
		t.Fatalf("requiredString = %q, %v", got, err)
	}
	if got, err := requiredStrings(outputs, "subnets"); err != nil || strings.Join(got, ",") != "subnet-a,subnet-b" {
		t.Fatalf("requiredStrings = %v, %v", got, err)
	}
	if _, err := requiredString(outputs, "missing"); err == nil {
		t.Fatal("missing output was accepted")
	}
}

func TestWaitForHealthyServiceUsesRuntimeEvidence(t *testing.T) {
	steps := &Steps{backend: &stepsBackend{outputs: map[string]any{"clusterName": "shop-cluster", "serviceName": "shop-web-service"}}, runtime: stepsRuntime{}, waitInterval: time.Millisecond, waitTimeout: time.Second}
	if err := steps.waitForHealthyService(context.Background()); err != nil {
		t.Fatal(err)
	}
}

type recordingCandidate struct {
	requests []awsoperations.CandidateRequest
}

func (c *recordingCandidate) RegisterCandidate(_ context.Context, request awsoperations.CandidateRequest) (awsoperations.Candidate, error) {
	c.requests = append(c.requests, request)
	return awsoperations.Candidate{TaskDefinitionARN: "arn:aws:ecs:eu-north-1:123:task-definition/deploy:1"}, nil
}
func (*recordingCandidate) RunMigrations(context.Context, awsoperations.Candidate) error { return nil }
func (*recordingCandidate) Cleanup(context.Context, awsoperations.Candidate) error       { return nil }

func TestRegisterCandidateBootstrapsGreenfieldStack(t *testing.T) {
	backend := &stepsBackend{
		outputs: map[string]any{},
		afterUpdate: map[string]any{
			"clusterName":             "acceptance-preview-cluster",
			"deployTaskDefinitionArn": "arn:aws:ecs:eu-north-1:123:task-definition/deploy:1",
			"securityGroupId":         "sg-web",
			"privateSubnetIds":        []any{"subnet-a", "subnet-b"},
		},
	}
	candidate := &recordingCandidate{}
	steps, err := New(backend, testSpec(t), candidate, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := "669890779205.dkr.ecr.eu-north-1.amazonaws.com/magelift-acceptance@sha256:" + strings.Repeat("a", 64)
	if err := steps.RegisterCandidate(context.Background(), deployRequest(digest)); err != nil {
		t.Fatal(err)
	}
	if backend.updateCalls != 1 {
		t.Fatalf("expected one bootstrap update, got %d", backend.updateCalls)
	}
	if len(candidate.requests) != 1 || candidate.requests[0].Cluster != "acceptance-preview-cluster" {
		t.Fatalf("candidate request = %#v", candidate.requests)
	}
}

func testSpec(t *testing.T) awsstack.Spec {
	t.Helper()
	hostedZone := sdk.ExistingResourceRef{ID: "zone", Provider: "aws", Kind: sdk.ExistingDNSZone, ExternalID: "Z123456789"}
	certificate := sdk.ExistingResourceRef{ID: "certificate", Provider: "aws", Kind: sdk.ExistingCertificate, ExternalID: "arn:aws:acm:us-east-1:123456789012:certificate/00000000-0000-0000-0000-000000000000"}
	albCertificate := sdk.ExistingResourceRef{ID: "alb-certificate", Provider: "aws", Kind: sdk.ExistingCertificate, ExternalID: "arn:aws:acm:eu-west-3:123456789012:certificate/11111111-1111-1111-1111-111111111111"}
	return awsstack.Spec{
		Identity:     awsstack.Identity{Project: "shop", Environment: "preview-1", AccountID: "123456789012", Region: "eu-west-3", EnvironmentClass: "preview", Preset: sdk.PresetPreview},
		Application:  awsstack.Application{Edition: "open-source", Version: "2.4.9", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     awsstack.Artifact{ImageDigest: "ghcr.io/acourtiol/shop@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", CompatibilityStatus: "compatible", RequiredRuntimeCapabilities: []sdk.CapabilityID{sdk.CapabilityDatabaseMySQL, sdk.CapabilityCacheValkey}},
		Lifecycle:    awsstack.Lifecycle{ExpiresAt: time.Date(2026, time.December, 31, 0, 0, 0, 0, time.UTC), MonthlyBudgetCents: 25000},
		Existing:     awsstack.ExistingResources{HostedZone: &hostedZone, Certificate: &certificate, ALBCertificate: &albCertificate},
		Dependencies: awsstack.Dependencies{KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555", CacheSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-cache-token", EncryptionKeyARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop-encryption-key", DatabaseName: "magento", MasterUsername: "magento"},
		Policy:       awsstack.NetworkPolicy{VPCCIDR: netip.MustParsePrefix("10.42.0.0/16"), AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, ApplicationDomain: "preview.example.com", MediaDomain: "media.preview.example.com"},
		Catalog:      awsstack.CatalogSelection{Version: "2026.07-preview.1", Aurora: awsstack.AuroraPreviewProfile{MinimumACU: 0, MaximumACU: 4, AutoPauseSeconds: 900, EngineSupportsAutoPause: true}, Valkey: awsstack.ValkeyPreviewProfile{NodeType: "cache.t4g.micro"}, Search: awsstack.SearchPreviewProfile{MaximumIndexingOCU: 2, MaximumSearchOCU: 2, AcceptColdStarts: true}, Fargate: awsstack.FargatePreviewProfile{CPU: 512, MemoryMiB: 1024, DesiredCount: 1}, Retention: awsstack.RetentionProfile{LogDays: 7, BackupDays: 1, ArtifactDays: 7}, Versions: awsstack.ServiceVersions{AuroraMySQL: "8.0.mysql_aurora.3.12", Valkey: "8.1", OpenSearch: "OpenSearch_3.1", RabbitMQ: "3.13"}},
	}
}

func deployRequest(digest string) deployflow.Request {
	return deployflow.Request{
		Target:      sdk.TargetDescriptor{ID: sdk.TargetID("aws.ecs-fargate"), Provider: "aws", Runtime: "ecs-fargate"},
		ImageDigest: digest,
	}
}
