package plugin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/sdk"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type recordingJobs struct {
	created []string
	deleted []string
	jobs    map[string]*batchv1.Job
}

func (f *recordingJobs) CreateJob(_ context.Context, _ string, job *batchv1.Job) (string, error) {
	f.created = append(f.created, job.Name)
	if f.jobs == nil {
		f.jobs = map[string]*batchv1.Job{}
	}
	f.jobs[job.Name] = job
	return job.Name, nil
}

func (f *recordingJobs) WaitJob(_ context.Context, _, _ string, _ time.Duration) error {
	return nil
}

func (f *recordingJobs) DeleteJob(_ context.Context, _ string, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

const deployTestDigest = "ghcr.io/magelift/magento@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func deployTestInputs() sdk.DeployInputs {
	return sdk.DeployInputs{
		ImageDigest: deployTestDigest, DatabaseName: "magento",
		ApplicationMode: "integrated", ApplicationVersion: "2.4.9", WebRuntime: "nginx-fpm",
		CPURequest: "500m", MemoryRequest: "1Gi", CloudProject: "example-gcp-project", Region: "europe-west1",
	}
}

func deployTestOutputs(t *testing.T) []byte {
	t.Helper()
	return deployTestOutputsWithURL(t, "")
}

func deployTestOutputsWithURL(t *testing.T, appURL string) []byte {
	t.Helper()
	outputs := map[string]any{
		"clusterName": "shop-cluster", "serviceName": "shop-web",
		"databaseWriter": "10.0.0.1", "cacheEndpoint": "10.0.0.2:6379",
		"databaseSecretName": "db-secret", "encryptionKeySecretName": "crypt-secret",
		"searchEndpoint": "http://search.internal:9200",
	}
	if appURL != "" {
		outputs["applicationURL"] = appURL
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func deployTestPlan(t *testing.T) sdk.StoredPlan {
	t.Helper()
	spec := testSpec()
	spec.Catalog.SearchMode = "opensearch"
	plan := storedTestPlan(t, spec)
	inputs, err := json.Marshal(deployTestInputs())
	if err != nil {
		t.Fatal(err)
	}
	plan.DeployInputsJSON = inputs
	return plan
}

type stubHTTPDoer struct {
	status int
	sawURL string
}

func (s *stubHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	s.sawURL = request.URL.String()
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader("store")),
		Header:     http.Header{},
		Request:    request,
	}, nil
}

func deployTestServer(jobs *recordingJobs, objects ...k8sruntime.Object) *Server {
	clientset := k8sfake.NewClientset(objects...)
	return &Server{
		KubeClients: fakeKubeFactory(clientset),
		HTTPDoer:    &stubHTTPDoer{status: http.StatusOK},
		NewDeployStores: func(kube.ClientFactory, map[string]any) (*kube.CandidateStore, *kube.DeploymentRuntime, error) {
			candidate, err := kube.NewCandidateFromClient(jobs)
			if err != nil {
				return nil, nil, err
			}
			runtime, err := kube.NewRuntimeFromClient(clientset)
			if err != nil {
				return nil, nil, err
			}
			return candidate, runtime, nil
		},
	}
}

func deployReadyDeployment() *appsv1.Deployment {
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default", Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: deployTestDigest}}}}},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 1, UpdatedReplicas: 1, ObservedGeneration: 3, Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
		}},
	}
}

func deployPhaseCall(phase sdk.DeployAppPhase, plan sdk.StoredPlan, outputs, state []byte) *sdk.DeployAppPhaseCall {
	return &sdk.DeployAppPhaseCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), Plan: plan,
		Phase: phase, ImageDigest: deployTestDigest, OutputsJSON: outputs, StateJSON: state,
	}
}

func TestDeployRegisterCreatesCandidateJob(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs)
	result, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, deployTestPlan(t), deployTestOutputs(t), nil))
	if operr != nil {
		t.Fatal(operr)
	}
	if len(jobs.created) != 1 || !strings.HasPrefix(jobs.created[0], "magelift-migrate-") {
		t.Fatalf("created = %v", jobs.created)
	}
	var candidate kube.Candidate
	if err := json.Unmarshal(result.StateJSON, &candidate); err != nil {
		t.Fatal(err)
	}
	if candidate.JobName != jobs.created[0] {
		t.Fatalf("candidate = %+v", candidate)
	}
}

func TestDeployMigrateRunsAndCleans(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs)
	plan := deployTestPlan(t)
	outputs := deployTestOutputs(t)
	registered, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, plan, outputs, nil))
	if operr != nil {
		t.Fatal(operr)
	}
	result, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseMigrate, plan, outputs, registered.StateJSON))
	if operr != nil {
		t.Fatal(operr)
	}
	if result.Message == "" {
		t.Fatal("empty migrate message")
	}
	if len(jobs.deleted) != 1 || jobs.deleted[0] != jobs.created[0] {
		t.Fatalf("deleted = %v, created = %v", jobs.deleted, jobs.created)
	}
}

func TestDeployCleanupIsEmptyStateNoop(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs)
	_, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseCleanup, deployTestPlan(t), deployTestOutputs(t), nil))
	if operr != nil {
		t.Fatal(operr)
	}
	if len(jobs.deleted) != 0 {
		t.Fatalf("deleted = %v", jobs.deleted)
	}
}

func TestDeployStabilizeAndHealthPassOnReadyRollout(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs, deployReadyDeployment())
	plan := deployTestPlan(t)
	outputs := deployTestOutputsWithURL(t, "https://shop.example.com/")
	if _, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseStabilize, plan, outputs, nil)); operr != nil {
		t.Fatal(operr)
	}
	if _, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseHealth, plan, outputs, nil)); operr != nil {
		t.Fatal(operr)
	}
	if len(jobs.created) != 1 {
		t.Fatalf("probe jobs = %v", jobs.created)
	}
	probe := jobs.jobs[jobs.created[0]]
	script := strings.Join(probe.Spec.Template.Spec.Containers[0].Command, " ")
	for _, want := range []string{"opensearch_server_hostname", "= 'opensearch'"} {
		if !strings.Contains(script, want) {
			t.Fatalf("probe command = %q, want %q", script, want)
		}
	}
}

func TestDeployHealthFailsOnBrokenServingPath(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs, deployReadyDeployment())
	server.HTTPDoer = &stubHTTPDoer{status: http.StatusBadGateway}
	plan := deployTestPlan(t)
	outputs := deployTestOutputsWithURL(t, "https://shop.example.com/")
	_, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseHealth, plan, outputs, nil))
	if operr == nil || !strings.Contains(operr.Message, "serving path unhealthy") {
		t.Fatalf("operr = %+v, want serving-path failure", operr)
	}
	if len(jobs.created) != 0 {
		t.Fatalf("probe ran despite broken serving path: %v", jobs.created)
	}
}

func TestDeployHealthRefusesWithoutStorefront(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs, deployReadyDeployment())
	plan := deployTestPlan(t)
	_, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseHealth, plan, deployTestOutputs(t), nil))
	if operr == nil || !strings.Contains(operr.Message, "cannot be determined") {
		t.Fatalf("operr = %+v, want storefront refusal", operr)
	}
}

func TestDeployPhaseRejectsBadInput(t *testing.T) {
	t.Parallel()
	server := deployTestServer(&recordingJobs{})
	plan := deployTestPlan(t)
	outputs := deployTestOutputs(t)
	if _, operr := server.DeployAppPhase(context.Background(), nil); operr == nil {
		t.Fatal("nil call accepted")
	}
	bad := deployPhaseCall(sdk.DeployAppPhase("nope"), plan, outputs, nil)
	if _, operr := server.DeployAppPhase(context.Background(), bad); operr == nil {
		t.Fatal("bad phase accepted")
	}
	mismatch := deployPhaseCall(sdk.DeployPhaseHealth, plan, outputs, nil)
	mismatch.ImageDigest = "ghcr.io/magelift/magento@sha256:" + strings.Repeat("b", 64)
	if _, operr := server.DeployAppPhase(context.Background(), mismatch); operr == nil {
		t.Fatal("digest mismatch accepted")
	}
	empty := deployTestPlan(t)
	empty.DeployInputsJSON = []byte(`{}`)
	if _, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, empty, outputs, nil)); operr == nil {
		t.Fatal("invalid inputs accepted")
	}
}
