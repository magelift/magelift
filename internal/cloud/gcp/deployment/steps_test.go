package deployment

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	gcpoperations "github.com/acourtiol/magelift/internal/cloud/gcp/operations"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

type stepsBackend struct {
	outputs     map[string]any
	updateCalls int
	afterUpdate map[string]any
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

func (stepsRuntime) Check(context.Context, string, string) (gcpoperations.ServiceHealth, error) {
	return gcpoperations.ServiceHealth{DesiredReplicas: 1, ReadyReplicas: 1, Available: true}, nil
}

type recordingCandidate struct {
	requests []gcpoperations.CandidateRequest
	failRun  bool
}

func (c *recordingCandidate) RegisterCandidate(_ context.Context, request gcpoperations.CandidateRequest) (gcpoperations.Candidate, error) {
	c.requests = append(c.requests, request)
	return gcpoperations.Candidate{JobName: "magelift-migrate-test", Namespace: "default"}, nil
}
func (c *recordingCandidate) RunMigrations(context.Context, gcpoperations.Candidate) error {
	if c.failRun {
		return errors.New("setup:upgrade failed")
	}
	return nil
}
func (*recordingCandidate) Cleanup(context.Context, gcpoperations.Candidate) error { return nil }

func TestValidateRejectsDigestMismatch(t *testing.T) {
	t.Parallel()
	steps, err := New(&stepsBackend{outputs: map[string]any{}}, testSpec(t), &recordingCandidate{}, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = steps.Validate(context.Background(), deployRequest("ghcr.io/acourtiol/magento@sha256:"+strings.Repeat("b", 64)))
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestRegisterCandidateBootstrapsGreenfieldStack(t *testing.T) {
	t.Parallel()
	backend := &stepsBackend{
		outputs: map[string]any{},
		afterUpdate: map[string]any{
			platform.OutputClusterName:     "shop-preview-app-gke",
			platform.OutputServiceName:     "shop-preview-app-web",
			platform.OutputDatabaseWriter:  "10.20.1.5",
			platform.OutputCacheEndpoint:   "10.20.2.5",
			platform.OutputPrivateSubnetIDs: []any{"subnet-a"},
		},
	}
	candidate := &recordingCandidate{}
	steps, err := New(backend, testSpec(t), candidate, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := testSpec(t).Artifact.ImageDigest
	if err := steps.RegisterCandidate(context.Background(), deployRequest(digest)); err != nil {
		t.Fatal(err)
	}
	if backend.updateCalls != 1 {
		t.Fatalf("expected one bootstrap update, got %d", backend.updateCalls)
	}
	if len(candidate.requests) != 1 || candidate.requests[0].Cluster != "shop-preview-app-gke" {
		t.Fatalf("candidate request = %#v", candidate.requests)
	}
}

func TestRunMigrationsCleansUpOnFailure(t *testing.T) {
	t.Parallel()
	candidate := &recordingCandidate{failRun: true}
	steps, err := New(&stepsBackend{outputs: map[string]any{
		platform.OutputClusterName:    "c",
		platform.OutputServiceName:    "s",
		platform.OutputDatabaseWriter: "10.0.0.1",
		platform.OutputCacheEndpoint:  "10.0.0.2",
	}}, testSpec(t), candidate, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	digest := testSpec(t).Artifact.ImageDigest
	if err := steps.RegisterCandidate(context.Background(), deployRequest(digest)); err != nil {
		t.Fatal(err)
	}
	err = steps.RunMigrations(context.Background(), deployRequest(digest))
	if err == nil || !strings.Contains(err.Error(), "setup:upgrade failed") {
		t.Fatalf("expected migrate failure, got %v", err)
	}
	if steps.registeredSet {
		t.Fatal("candidate should be cleared after failed migrate cleanup")
	}
}

func TestWaitForHealthyService(t *testing.T) {
	t.Parallel()
	steps := &Steps{
		backend: &stepsBackend{outputs: map[string]any{
			platform.OutputClusterName: "c", platform.OutputServiceName: "s",
		}},
		runtime: stepsRuntime{}, waitInterval: time.Millisecond, waitTimeout: time.Second,
	}
	if err := steps.waitForHealthyService(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func testSpec(t *testing.T) gcpstack.Spec {
	t.Helper()
	return gcpstack.Spec{
		Identity: gcpstack.Identity{
			Project: "shop", GCPProject: "digital-lab-341608", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: sdk.PresetPreview,
		},
		Application:  gcpstack.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     gcpstack.Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:" + strings.Repeat("a", 64)},
		Policy:       gcpstack.NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b"}},
		Catalog:      gcpstack.CatalogSelection{DesiredWebReplicas: 1, AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi"},
		Dependencies: gcpstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}

func deployRequest(digest string) deployflow.Request {
	return deployflow.Request{
		Target:      sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"},
		ImageDigest: digest,
	}
}
