package plugin

import (
	"context"
	"encoding/json"
	"errors"
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
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

type recordingJobs struct {
	created []string
	deleted []string
	jobs    map[string]*batchv1.Job
	waitErr error
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
	return f.waitErr
}

func (f *recordingJobs) DeleteJob(_ context.Context, _ string, name string) error {
	f.deleted = append(f.deleted, name)
	return nil
}

const deployTestDigest = "registry.example/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

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
		"searchEndpoint":  "http://search.internal:9200",
		"imagePullSecret": "registry-credentials", "mediaClaim": "magelift-media",
		"databaseTLSCA": "/etc/ssl/certs/ca-certificates.crt",
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
	var result sdk.PlanResult
	if err := (&Server{}).Plan(testPlanRequest(), &result); err != nil || result.Error != nil {
		t.Fatalf("plan %v: %v", err, result.Error)
	}
	return result.Plan
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
		KubeClients: func(map[string]any) (kubernetes.Interface, error) { return clientset, nil },
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

func deployPhaseCall(phase sdk.DeployAppPhase, plan sdk.StoredPlan, outputs, state []byte) *sdk.DeployAppPhaseCall {
	return &sdk.DeployAppPhaseCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: testPlanRequest().Envelope, Plan: plan,
		Phase: phase, ImageDigest: deployTestDigest, OutputsJSON: outputs, StateJSON: state,
	}
}

func TestDeployRegisterCreatesCandidateJob(t *testing.T) {
	t.Parallel()
	jobs := &recordingJobs{}
	server := deployTestServer(jobs)
	result, operr := server.deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, deployTestPlan(t), deployTestOutputs(t), nil))
	if operr != nil {
		t.Fatal(operr)
	}
	if len(jobs.created) != 1 || !strings.HasPrefix(jobs.created[0], "magelift-migrate-") {
		t.Fatalf("created = %v", jobs.created)
	}
	job := jobs.jobs[jobs.created[0]]
	pod := job.Spec.Template.Spec
	if len(pod.ImagePullSecrets) != 1 || pod.ImagePullSecrets[0].Name != "registry-credentials" {
		t.Fatalf("image pull secrets = %#v", pod.ImagePullSecrets)
	}
	if len(pod.Volumes) != 1 || pod.Volumes[0].PersistentVolumeClaim.ClaimName != "magelift-media" || len(pod.Containers[0].VolumeMounts) != 1 || pod.Containers[0].VolumeMounts[0].MountPath != "/app/pub/media" {
		t.Fatalf("migration media mount missing: %#v", pod)
	}
	if !strings.Contains(strings.Join(pod.Containers[0].Command, " "), "magelift_ensure_initial_database") {
		t.Fatal("migration job does not invoke the initial database bootstrap lifecycle")
	}
	foundTLS := false
	for _, value := range pod.Containers[0].Env {
		if value.Name == "MAGELIFT_DATABASE_TLS_CA" && value.Value == "/etc/ssl/certs/ca-certificates.crt" {
			foundTLS = true
		}
	}
	if !foundTLS {
		t.Fatal("migration job must verify the database TLS certificate")
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
	registered, operr := server.deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, plan, outputs, nil))
	if operr != nil {
		t.Fatal(operr)
	}
	result, operr := server.deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseMigrate, plan, outputs, registered.StateJSON))
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

func TestDeployRejectsChangedPlanInputs(t *testing.T) {
	plan := deployTestPlan(t)
	var inputs sdk.DeployInputs
	if err := json.Unmarshal(plan.DeployInputsJSON, &inputs); err != nil {
		t.Fatal(err)
	}
	inputs.DatabaseName = "wrong"
	plan.DeployInputsJSON, _ = json.Marshal(inputs)
	jobs := &recordingJobs{}
	_, opErr := deployTestServer(jobs).deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, plan, deployTestOutputs(t), nil))
	if opErr == nil || !strings.Contains(opErr.Message, "do not match") {
		t.Fatalf("unexpected error: %#v", opErr)
	}
	if len(jobs.created) != 0 {
		t.Fatal("created a job for altered plan")
	}
}

func TestDeployMigrationFailureCleansCandidate(t *testing.T) {
	jobs := &recordingJobs{waitErr: errors.New("migration failed")}
	server := deployTestServer(jobs)
	plan, outputs := deployTestPlan(t), deployTestOutputs(t)
	registered, opErr := server.deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseRegister, plan, outputs, nil))
	if opErr != nil {
		t.Fatal(opErr)
	}
	_, opErr = server.deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseMigrate, plan, outputs, registered.StateJSON))
	if opErr == nil || !strings.Contains(opErr.Message, "migration failed") || len(jobs.deleted) != 1 {
		t.Fatalf("failure = %#v, deleted = %v", opErr, jobs.deleted)
	}
}

func TestPreviousDigestCanBeReplannedForForwardOnlyRollback(t *testing.T) {
	request := testPlanRequest()
	previous := "registry.example/shop@sha256:" + strings.Repeat("b", 64)
	request.TargetBlock = []byte(strings.Replace(string(request.TargetBlock), deployTestDigest, previous, 1))
	var result sdk.PlanResult
	server := deployTestServer(&recordingJobs{})
	if err := server.Plan(request, &result); err != nil || result.Error != nil {
		t.Fatalf("replan previous digest: %v %#v", err, result.Error)
	}
	call := deployPhaseCall(sdk.DeployPhaseRegister, result.Plan, deployTestOutputs(t), nil)
	call.ImageDigest = previous
	phase, opErr := server.deployAppPhase(context.Background(), call)
	if opErr != nil || len(phase.StateJSON) == 0 {
		t.Fatalf("register previous digest: %#v, %#v", phase, opErr)
	}
}

func TestDeployHealthChecksActiveSearchAdapter(t *testing.T) {
	replicas := int32(1)
	ready := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default", Generation: 3}, Spec: appsv1.DeploymentSpec{Replicas: &replicas, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "web", Image: deployTestDigest}}}}}, Status: appsv1.DeploymentStatus{ReadyReplicas: 1, UpdatedReplicas: 1, ObservedGeneration: 3, Conditions: []appsv1.DeploymentCondition{{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue}}}}
	jobs := &recordingJobs{}
	server := deployTestServer(jobs, ready)
	if _, err := server.deployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseHealth, deployTestPlan(t), deployTestOutputsWithURL(t, "https://shop.example/"), nil)); err != nil {
		t.Fatal(err)
	}
	if len(jobs.created) != 1 {
		t.Fatalf("probe jobs = %v", jobs.created)
	}
	script := strings.Join(jobs.jobs[jobs.created[0]].Spec.Template.Spec.Containers[0].Command, " ")
	for _, want := range []string{"smile_elasticsuite_core_base_settings/es_client/servers", "CONFIG__DEFAULT__CATALOG__SEARCH__ENGINE", "opensearch_server_hostname"} {
		if !strings.Contains(script, want) {
			t.Fatalf("probe missing %s: %s", want, script)
		}
	}
}
