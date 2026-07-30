package kube

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/acourtiol/magelift/internal/automation"
	deployflow "github.com/acourtiol/magelift/internal/deploy"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
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

type recordingCandidate struct {
	requests []CandidateRequest
	failRun  bool
}

func (c *recordingCandidate) RegisterCandidate(_ context.Context, request CandidateRequest) (Candidate, error) {
	c.requests = append(c.requests, request)
	return Candidate{JobName: "magelift-migrate-test", Namespace: "default", Outputs: request.Outputs}, nil
}
func (c *recordingCandidate) RunMigrations(context.Context, Candidate) error {
	if c.failRun {
		return errors.New("setup:upgrade failed")
	}
	return nil
}
func (*recordingCandidate) Cleanup(context.Context, Candidate) error { return nil }

type stepsRuntime struct{}

func (stepsRuntime) Check(context.Context, string, string) (ServiceHealth, error) {
	return ServiceHealth{DesiredReplicas: 1, ReadyReplicas: 1, Available: true}, nil
}

type fakeJobs struct {
	created []*batchv1.Job
	waited  []string
	deleted []string
}

func (f *fakeJobs) CreateJob(_ context.Context, _ string, job *batchv1.Job) (string, error) {
	f.created = append(f.created, job)
	return job.Name, nil
}
func (f *fakeJobs) WaitJob(_ context.Context, _, name string, _ time.Duration) error {
	f.waited = append(f.waited, name)
	return nil
}
func (f *fakeJobs) DeleteJob(_ context.Context, _, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

func TestStepsSequence(t *testing.T) {
	digest := "ghcr.io/acourtiol/magento@sha256:" + strings.Repeat("a", 64)
	jobs := &fakeJobs{}
	candidate, err := NewCandidateFromClient(jobs)
	if err != nil {
		t.Fatal(err)
	}
	replicas := int32(1)
	cs := fake.NewClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ReadyReplicas: 1,
			Conditions: []appsv1.DeploymentCondition{{
				Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue,
			}},
		},
	})
	runtime, err := NewRuntimeFromClient(cs)
	if err != nil {
		t.Fatal(err)
	}
	backend := &stepsBackend{outputs: map[string]any{
		platform.OutputClusterName:    "shop-gke",
		platform.OutputServiceName:    "shop-web",
		platform.OutputDatabaseWriter: "10.0.0.1",
		platform.OutputCacheEndpoint:  "10.0.0.2",
	}}
	recorded := false
	steps, err := New(backend, testDeploySpec(digest), candidate, runtime, io.Discard,
		func(context.Context, deployflow.Request, deployflow.Result) error {
			recorded = true
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	steps.waitInterval = time.Millisecond
	steps.waitTimeout = time.Second

	req := deployRequest(digest)
	ctx := context.Background()
	if err := steps.Validate(ctx, req); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if err := steps.RegisterCandidate(ctx, req); err != nil {
		t.Fatalf("RegisterCandidate: %v", err)
	}
	if len(jobs.created) != 1 {
		t.Fatalf("expected one migrate Job, got %d", len(jobs.created))
	}
	if err := steps.RunMigrations(ctx, req); err != nil {
		t.Fatalf("RunMigrations: %v", err)
	}
	if len(jobs.waited) != 1 || len(jobs.deleted) != 1 {
		t.Fatalf("waited=%v deleted=%v", jobs.waited, jobs.deleted)
	}
	if err := steps.CleanupCandidate(ctx, req); err != nil {
		t.Fatalf("CleanupCandidate: %v", err)
	}
	if _, err := steps.UpdateServices(ctx, req); err != nil {
		t.Fatalf("UpdateServices: %v", err)
	}
	if err := steps.Stabilize(ctx, req); err != nil {
		t.Fatalf("Stabilize: %v", err)
	}
	if err := steps.Health(ctx, req); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := steps.Record(ctx, req, deployflow.Result{}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if !recorded {
		t.Fatal("Record hook was not invoked")
	}
}

func TestValidateRejectsDigestMismatch(t *testing.T) {
	steps, err := New(&stepsBackend{outputs: map[string]any{}}, testDeploySpec("ghcr.io/acourtiol/magento@sha256:"+strings.Repeat("a", 64)), &recordingCandidate{}, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
	err = steps.Validate(context.Background(), deployRequest("ghcr.io/acourtiol/magento@sha256:"+strings.Repeat("b", 64)))
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestRegisterCandidateBootstrapsGreenfieldStack(t *testing.T) {
	backend := &stepsBackend{
		outputs: map[string]any{},
		afterUpdate: map[string]any{
			platform.OutputClusterName:    "shop-preview-app-gke",
			platform.OutputServiceName:    "shop-preview-app-web",
			platform.OutputDatabaseWriter: "10.20.1.5",
			platform.OutputCacheEndpoint:  "10.20.2.5",
		},
	}
	candidate := &recordingCandidate{}
	digest := "ghcr.io/acourtiol/magento@sha256:" + strings.Repeat("a", 64)
	steps, err := New(backend, testDeploySpec(digest), candidate, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	candidate := &recordingCandidate{failRun: true}
	digest := "ghcr.io/acourtiol/magento@sha256:" + strings.Repeat("a", 64)
	steps, err := New(&stepsBackend{outputs: map[string]any{
		platform.OutputClusterName:    "c",
		platform.OutputServiceName:    "s",
		platform.OutputDatabaseWriter: "10.0.0.1",
		platform.OutputCacheEndpoint:  "10.0.0.2",
	}}, testDeploySpec(digest), candidate, stepsRuntime{}, io.Discard, nil)
	if err != nil {
		t.Fatal(err)
	}
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

func testDeploySpec(digest string) DeploySpec {
	return DeploySpec{
		ImageDigest:     digest,
		DatabaseName:    "magento",
		ApplicationMode: "integrated",
		WebRuntime:      "nginx-fpm",
		CPURequest:      "500m",
		MemoryRequest:   "1Gi",
		CloudProject:    "digital-lab-341608",
		Region:          "europe-west1",
	}
}

func deployRequest(digest string) deployflow.Request {
	return deployflow.Request{
		Target:      sdk.TargetDescriptor{ID: "gcp.gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot"},
		ImageDigest: digest,
	}
}

func TestWaitForHealthyService(t *testing.T) {
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

func TestFourModuleNewDeployStepsTypeIdentity(t *testing.T) {
	// SC2 concrete type in this package is *Steps. Cross-module registration
	// (gcp/eksops/ovh/scaleway → *kube.Steps) is asserted in identity_test.go
	// (package kube_test) to avoid provider→kube import cycles.
	var _ deployflow.Steps = (*Steps)(nil)
}
