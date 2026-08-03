package ops

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/automation"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func TestRuntimeObserveTypeIdentity(t *testing.T) {
	obs := platform.ModuleRuntimeObserve(Module{})
	if obs == nil {
		t.Fatal("RuntimeObserve returned nil")
	}
	if _, ok := obs.(*kube.Observe); !ok {
		t.Fatalf("want *kube.Observe, got %T", obs)
	}
}

func TestNewDeployStepsTypeIdentity(t *testing.T) {
	ops := Ops{
		NewCandidate: func(context.Context, kube.Backend) (kube.CandidateRunner, error) {
			return &stubCandidate{}, nil
		},
		NewRuntime: func(context.Context, kube.Backend) (kube.RuntimeChecker, error) {
			return stubRuntime{}, nil
		},
	}
	backend := &stubBackend{outputs: map[string]any{}}
	planned := gcpstack.Planned{Spec: gcpTestSpec()}
	steps, err := ops.NewDeploySteps(context.Background(), backend, planned, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := steps.(*kube.Steps); !ok {
		t.Fatalf("want *kube.Steps, got %T", steps)
	}
}

func TestNewDeployStepsRejectsWrongBackend(t *testing.T) {
	_, err := (Ops{}).NewDeploySteps(context.Background(), struct{}{}, gcpstack.Planned{Spec: gcpTestSpec()}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "backend with outputs") {
		t.Fatalf("expected wrong-backend error, got %v", err)
	}
}

type stubCandidate struct{}

func (*stubCandidate) RegisterCandidate(context.Context, kube.CandidateRequest) (kube.Candidate, error) {
	return kube.Candidate{}, nil
}
func (*stubCandidate) RunMigrations(context.Context, kube.Candidate) error { return nil }
func (*stubCandidate) Cleanup(context.Context, kube.Candidate) error       { return nil }

type stubRuntime struct{}

func (stubRuntime) Check(context.Context, string, string) (kube.ServiceHealth, error) {
	return kube.ServiceHealth{DesiredReplicas: 1, ReadyReplicas: 1, Available: true}, nil
}

type stubBackend struct {
	outputs map[string]any
}

func (b *stubBackend) Outputs(context.Context) (map[string]any, error) { return b.outputs, nil }
func (*stubBackend) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*stubBackend) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}
func (*stubBackend) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return map[string]int{}, nil
}

func gcpTestSpec() gcpstack.Spec {
	return gcpstack.Spec{
		Identity: gcpstack.Identity{
			Project: "shop", GCPProject: "example-gcp-project", Environment: "preview",
			Region: "europe-west1", EnvironmentClass: "preview", Preset: sdk.PresetPreview,
		},
		Application:  gcpstack.Application{Edition: "open-source", Version: "2.4.8", Mode: "integrated", WebRuntime: "nginx-fpm"},
		Artifact:     gcpstack.Artifact{ImageDigest: "ghcr.io/acourtiol/magento@sha256:" + strings.Repeat("a", 64)},
		Policy:       gcpstack.NetworkPolicy{NetworkCIDR: "10.20.0.0/16", Zones: []string{"europe-west1-b"}},
		Catalog:      gcpstack.CatalogSelection{DesiredWebReplicas: 1, AutopilotCPURequest: "500m", AutopilotMemoryRequest: "1Gi"},
		Dependencies: gcpstack.Dependencies{DatabaseName: "magento", MasterUsername: "magento"},
	}
}
