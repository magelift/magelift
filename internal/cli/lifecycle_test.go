package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/automation"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/cosign"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/releasejournal"
	"go.yaml.in/yaml/v4"
)

type fakeInfrastructureBackend struct {
	calls      []string
	outputs    map[string]any
	previewErr error
	updateErr  error
	destroyErr error
}

func (b *fakeInfrastructureBackend) Outputs(context.Context) (map[string]any, error) {
	b.calls = append(b.calls, "outputs")
	return b.outputs, nil
}

func (b *fakeInfrastructureBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	b.calls = append(b.calls, "preview")
	if b.previewErr != nil {
		return nil, b.previewErr
	}
	return map[string]int{"create": 2}, nil
}

func (b *fakeInfrastructureBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	b.calls = append(b.calls, "update")
	if b.updateErr != nil {
		return nil, b.updateErr
	}
	return map[string]int{"update": 1}, nil
}

func (b *fakeInfrastructureBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	b.calls = append(b.calls, "destroy")
	if b.destroyErr != nil {
		return nil, b.destroyErr
	}
	return map[string]int{"delete": 1}, nil
}

type fakeDeploymentSteps struct {
	order   *[]string
	request *deployflow.Request
}

func (s fakeDeploymentSteps) Validate(_ context.Context, request deployflow.Request) error {
	if s.request != nil {
		*s.request = request
	}
	*s.order = append(*s.order, "validate")
	return nil
}
func (s fakeDeploymentSteps) Preview(context.Context, deployflow.Request) (automation.ChangeSummary, error) {
	*s.order = append(*s.order, "preview")
	return automation.ChangeSummary{Total: 1}, nil
}
func (s fakeDeploymentSteps) RegisterCandidate(context.Context, deployflow.Request) error {
	*s.order = append(*s.order, "candidate")
	return nil
}
func (s fakeDeploymentSteps) RunMigrations(context.Context, deployflow.Request) error {
	*s.order = append(*s.order, "migrate")
	return nil
}
func (s fakeDeploymentSteps) CleanupCandidate(context.Context, deployflow.Request) error {
	*s.order = append(*s.order, "cleanup")
	return nil
}
func (s fakeDeploymentSteps) UpdateServices(context.Context, deployflow.Request) (automation.ChangeSummary, error) {
	*s.order = append(*s.order, "update")
	return automation.ChangeSummary{Total: 2}, nil
}
func (s fakeDeploymentSteps) Stabilize(context.Context, deployflow.Request) error {
	*s.order = append(*s.order, "stabilize")
	return nil
}
func (s fakeDeploymentSteps) Health(context.Context, deployflow.Request) error {
	*s.order = append(*s.order, "health")
	return nil
}
func (s fakeDeploymentSteps) Record(context.Context, deployflow.Request, deployflow.Result) error {
	*s.order = append(*s.order, "record")
	return nil
}

type fakeReleaseStore struct {
	entries []releasejournal.Entry
}

func (s fakeReleaseStore) List(context.Context) ([]releasejournal.Entry, error) {
	return s.entries, nil
}
func (s fakeReleaseStore) Append(context.Context, releasejournal.Entry) (releasejournal.Entry, error) {
	return releasejournal.Entry{}, nil
}

func writeLifecycleConfig(t *testing.T, class string, protection bool) string {
	t.Helper()
	aws := config.AWSTarget{
		KMSKeyARN:                "arn:aws:kms:eu-west-3:123456789012:key/01234567-89ab-cdef-0123-456789abcdef",
		HostedZoneID:             "Z123456789",
		CloudFrontCertificateARN: "arn:aws:acm:us-east-1:123456789012:certificate/01234567-89ab-cdef-0123-456789abcdef",
		ALBCertificateARN:        "arn:aws:acm:eu-west-3:123456789012:certificate/abcdef01-2345-6789-abcd-ef0123456789",
		ImageDigest:              "ghcr.io/acourtiol/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		CacheSecretARN:           "arn:aws:secretsmanager:eu-west-3:123456789012:secret:cache-token",
		SessionSecretARN:         "arn:aws:secretsmanager:eu-west-3:123456789012:secret:session-token",
		QueueSecretARN:           "arn:aws:secretsmanager:eu-west-3:123456789012:secret:queue-token",
		EncryptionKeySecretARN:   "arn:aws:secretsmanager:eu-west-3:123456789012:secret:encryption-key",
		DatabaseName:             "magento",
		MasterUsername:           "magento_admin",
		VPCCIDR:                  "10.20.0.0/16",
		AvailabilityZones:        []string{"eu-west-3a", "eu-west-3b"},
		MediaDomain:              "media.shop.example",
		Catalog: config.AWSCatalog{
			Version:   "catalog-2026-07-01",
			Aurora:    config.AWSCatalogAurora{InstanceClass: "db.r7g.large", InstanceCount: 2},
			Valkey:    config.AWSCatalogValkey{NodeType: "cache.r7g.large", ReplicaCount: 1},
			Search:    config.AWSCatalogSearch{InstanceType: "r7g.large.search", InstanceCount: 2, EBSVolumeType: "gp3", EBSVolumeSizeGiB: 100},
			RabbitMQ:  config.AWSCatalogRabbitMQ{InstanceType: "mq.m7g.large"},
			Fargate:   config.AWSCatalogFargate{CPU: 1024, MemoryMiB: 2048, DesiredCount: 2},
			Retention: config.AWSCatalogRetention{LogDays: 30, BackupDays: 7, ArtifactDays: 30},
			Versions:  config.AWSCatalogVersions{AuroraMySQL: "8.0.mysql_aurora.3.12", Valkey: "8.1", OpenSearch: "OpenSearch_3.1", RabbitMQ: "3.13"},
		},
	}
	document := map[string]any{
		"schemaVersion": 1,
		"project":       config.Project{Name: "shop"},
		"application":   config.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		"build":         config.Build{PHP: "8.3"},
		"target":        config.Target{Provider: "aws", Runtime: "ecs-fargate", AWS: &aws},
		"defaults":      config.Defaults{Region: "eu-west-3", Preset: "standard"},
		"environments": map[string]any{"staging": map[string]any{
			"account":            "123456789012",
			"class":              class,
			"domain":             "shop.example",
			"protection":         protection,
			"monthlyBudgetCents": int64(250000),
		}},
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "magelift.yaml")
	if err := writeFile(path, data); err != nil {
		t.Fatal(err)
	}
	return path
}

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o600)
}

func TestDeployRunsPreviewAndUpdateThroughBackendBoundary(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{outputs: map[string]any{"url": "https://shop.example"}}
	var out bytes.Buffer
	var plannedDigest string
	o := testOptions(&out, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(_ context.Context, _ string, spec awsstack.Spec, _ string) (infrastructureBackend, error) {
		plannedDigest = spec.Artifact.ImageDigest
		return backend, nil
	}
	o.newLock = func(context.Context, awsstack.Spec) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy", "--digest", "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(backend.calls, []string{"preview", "update"}) {
		t.Fatalf("backend calls = %v", backend.calls)
	}
	if !bytes.Contains(out.Bytes(), []byte(`"update"`)) {
		t.Fatalf("deployment summary missing: %s", out.String())
	}
	if plannedDigest != "ghcr.io/acourtiol/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd" {
		t.Fatalf("deployment digest override = %q", plannedDigest)
	}
}

func TestDeployUsesPreTrafficWorkflowWhenConfigured(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{}
	order := []string{}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newDeploySteps = func(context.Context, infrastructureBackend, awsstack.Spec, io.Writer) (deployflow.Steps, error) {
		return fakeDeploymentSteps{order: &order}, nil
	}
	o.newLock = func(context.Context, awsstack.Spec) (func(context.Context) error, error) {
		order = append(order, "lock.acquire")
		return func(context.Context) error { order = append(order, "lock.release"); return nil }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--output", "json", "deploy"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []string{"validate", "lock.acquire", "preview", "candidate", "migrate", "update", "stabilize", "health", "record", "cleanup", "lock.release"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("workflow order = %v, want %v", order, want)
	}
	if !bytes.Contains(o.stdout.(*bytes.Buffer).Bytes(), []byte(`"update"`)) {
		t.Fatal("workflow result did not include update summary")
	}
}

func TestRollbackDeploymentPassesForwardOnlyFlagsToWorkflow(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{}
	order := []string{}
	var request deployflow.Request
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newDeploySteps = func(context.Context, infrastructureBackend, awsstack.Spec, io.Writer) (deployflow.Steps, error) {
		return fakeDeploymentSteps{order: &order, request: &request}, nil
	}
	o.newLock = func(context.Context, awsstack.Spec) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	}
	_, spec, err := o.infrastructureSpec()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := o.runDeploymentWithOptions(context.Background(), "staging", spec, spec.Artifact.ImageDigest, deploymentOptions{rollback: true, acknowledgeForwardOnlyDB: true}); err != nil {
		t.Fatal(err)
	}
	if !request.Rollback || !request.AcknowledgeForwardOnlyDB {
		t.Fatalf("workflow request did not carry rollback acknowledgement: %#v", request)
	}
}

func TestProductionDeploymentRequiresVerifiedReleaseMetadata(t *testing.T) {
	path := writeLifecycleConfig(t, "production", false)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.yes = true
	o.newReleaseStore = func(string, string) (releaseStore, error) { return fakeReleaseStore{}, nil }
	digest := "ghcr.io/acourtiol/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := o.requireSignedRelease(context.Background(), "staging", digest); err == nil || ExitCode(err) != 3 {
		t.Fatalf("unsigned production release = %v", err)
	}
	o.newReleaseStore = func(string, string) (releaseStore, error) {
		return fakeReleaseStore{entries: []releasejournal.Entry{{DigestReference: digest, SignatureIdentity: "release@example.invalid", SignatureIssuer: "https://issuer.example.invalid"}}}, nil
	}
	var verifiedDigest string
	var verifiedOptions cosign.VerifyOptions
	o.verifyRelease = func(_ context.Context, reference string, options cosign.VerifyOptions) error {
		verifiedDigest = reference
		verifiedOptions = options
		return nil
	}
	if err := o.requireSignedRelease(context.Background(), "staging", digest); err != nil {
		t.Fatal(err)
	}
	if verifiedDigest != digest || verifiedOptions.CertificateIdentity != "release@example.invalid" || verifiedOptions.OIDCIssuer != "https://issuer.example.invalid" {
		t.Fatalf("production deployment did not re-verify the recorded release: digest=%q options=%#v", verifiedDigest, verifiedOptions)
	}
}

func TestProductionDeploymentRejectsJournalWhenSignatureCannotBeReverified(t *testing.T) {
	path := writeLifecycleConfig(t, "production", false)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newReleaseStore = func(string, string) (releaseStore, error) {
		return fakeReleaseStore{entries: []releasejournal.Entry{{DigestReference: "ghcr.io/acourtiol/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", SignatureIdentity: "release@example.invalid", SignatureIssuer: "https://issuer.example.invalid"}}}, nil
	}
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error { return errors.New("invalid signature") }
	if err := o.requireSignedRelease(context.Background(), "staging", "ghcr.io/acourtiol/magento@sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"); err == nil || ExitCode(err) != 3 || !strings.Contains(err.Error(), "signature verification failed") {
		t.Fatalf("invalid production signature = %v", err)
	}
}

func TestProductionAndProtectedDestroyRequireApproval(t *testing.T) {
	for _, test := range []struct {
		name       string
		class      string
		protection bool
		command    string
		message    string
	}{
		{name: "production deploy", class: "production", command: "deploy", message: "production changes require"},
		{name: "protected destroy", class: "staging", protection: true, command: "destroy", message: "must be unprotected"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := writeLifecycleConfig(t, test.class, test.protection)
			o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
			o.configPath = path
			o.environment = "staging"
			o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
				return &fakeInfrastructureBackend{}, nil
			}
			o.newLock = func(context.Context, awsstack.Spec) (func(context.Context) error, error) {
				return func(context.Context) error { return nil }, nil
			}
			cmd := newCommandWithOptions(o)
			cmd.SetArgs([]string{"--config", path, "--env", "staging", test.command})
			err := cmd.Execute()
			if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("error/code = %v/%d", err, ExitCode(err))
			}
		})
	}
}

func TestProtectedDestroyCannotBeBypassedWithYes(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", true)
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		t.Fatal("protected destroy reached the infrastructure backend")
		return nil, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "--yes", "destroy"})
	err := cmd.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "must be unprotected") {
		t.Fatalf("error/code = %v/%d", err, ExitCode(err))
	}
}

func TestDeployReleasesLockAfterBackendFailure(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{previewErr: errors.New("preview failed")}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	released := false
	o.newLock = func(context.Context, awsstack.Spec) (func(context.Context) error, error) {
		return func(context.Context) error { released = true; return nil }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "deploy"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "infrastructure preview failed") || !released {
		t.Fatalf("error=%v released=%v", err, released)
	}
}

func TestDeployReportsLockReleaseFailure(t *testing.T) {
	path := writeLifecycleConfig(t, "staging", false)
	backend := &fakeInfrastructureBackend{}
	o := testOptions(&bytes.Buffer{}, &fakeTerminal{interactive: false})
	o.configPath = path
	o.environment = "staging"
	o.newBackend = func(context.Context, string, awsstack.Spec, string) (infrastructureBackend, error) {
		return backend, nil
	}
	o.newLock = func(context.Context, awsstack.Spec) (func(context.Context) error, error) {
		return func(context.Context) error { return errors.New("release failed") }, nil
	}
	cmd := newCommandWithOptions(o)
	cmd.SetArgs([]string{"--config", path, "--env", "staging", "deploy"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "release deployment lock") {
		t.Fatalf("unexpected error: %v", err)
	}
}
