package plugin

import (
	"context"
	"encoding/json"
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
}

func (f *recordingJobs) CreateJob(_ context.Context, _ string, job *batchv1.Job) (string, error) {
	f.created = append(f.created, job.Name)
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
	outputs, err := json.Marshal(map[string]any{
		"clusterName": "shop-cluster", "serviceName": "shop-web",
		"databaseWriter": "10.0.0.1", "cacheEndpoint": "10.0.0.2:6379",
		"databaseSecretName": "db-secret", "encryptionKeySecretName": "crypt-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	return outputs
}

func deployTestPlan(t *testing.T) sdk.StoredPlan {
	t.Helper()
	plan := storedTestPlan(t, testSpec())
	inputs, err := json.Marshal(deployTestInputs())
	if err != nil {
		t.Fatal(err)
	}
	plan.DeployInputsJSON = inputs
	return plan
}

func deployTestServer(jobs *recordingJobs, objects ...k8sruntime.Object) *Server {
	return &Server{
		NewDeployStores: func(kube.ClientFactory, map[string]any) (*kube.CandidateStore, *kube.DeploymentRuntime, error) {
			candidate, err := kube.NewCandidateFromClient(jobs)
			if err != nil {
				return nil, nil, err
			}
			runtime, err := kube.NewRuntimeFromClient(k8sfake.NewClientset(objects...))
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
	outputs := deployTestOutputs(t)
	if _, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseStabilize, plan, outputs, nil)); operr != nil {
		t.Fatal(operr)
	}
	if _, operr := server.DeployAppPhase(context.Background(), deployPhaseCall(sdk.DeployPhaseHealth, plan, outputs, nil)); operr != nil {
		t.Fatal(operr)
	}
	if len(jobs.created) != 1 {
		t.Fatalf("probe jobs = %v", jobs.created)
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
