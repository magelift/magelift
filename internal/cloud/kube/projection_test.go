package kube

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	cloudrecovery "github.com/magelift/magelift/internal/shared/recovery"
	"github.com/magelift/magelift/sdk"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/rest"
)

type fakeProjectionCommandRunner struct {
	request ProjectionCommandRequest
	result  ProjectionCommandResult
	err     error
}

func (runner *fakeProjectionCommandRunner) Run(_ context.Context, request ProjectionCommandRequest) (ProjectionCommandResult, error) {
	runner.request = request
	if runner.err != nil {
		return ProjectionCommandResult{}, runner.err
	}
	return runner.result, nil
}

func validKubernetesProjectionRequest(dataClass string, action sdk.ResilienceAction) cloudrecovery.ProjectionRequest {
	return cloudrecovery.ProjectionRequest{
		Action: action, DataClass: dataClass, SourceReference: "runtime://source", TargetReference: "runtime://target",
		Destination: sdk.RecoverySameRegionIsolated, FixtureID: "fixture-1", OwnershipMarker: "magelift/test/projection", IdempotencyKey: "projection-1",
	}
}

func validKubernetesProjectionVerifier(_ context.Context, request cloudrecovery.ProjectionRequest, _ ProjectionCommandRequest, execution ProjectionCommandResult) (cloudrecovery.ProjectionResult, error) {
	return cloudrecovery.ProjectionResult{
		Status: execution.Status, OperationID: execution.OperationID, ResourceReference: execution.ResourceReference,
		FixtureID: request.FixtureID, OwnershipMarker: request.OwnershipMarker, IdempotencyVerified: true,
		ManifestVerified:         request.DataClass == cloudrecovery.ProjectionSearchIndex,
		CountsVerified:           request.DataClass == cloudrecovery.ProjectionSearchIndex,
		ApplicationReadsVerified: true, PermissionsVerified: true, SecretReferencesVerified: true, ServiceHealthVerified: true,
		CacheLossClassified: request.DataClass == cloudrecovery.ProjectionCache,
	}, nil
}

func TestKubernetesProjectionBackendMapsIndependentMagentoCommands(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name      string
		dataClass string
		action    sdk.ResilienceAction
		want      []string
	}{
		{name: "search rebuild", dataClass: cloudrecovery.ProjectionSearchIndex, action: sdk.ResilienceRestore, want: []string{"bin/magento", "indexer:reindex"}},
		{name: "search verify", dataClass: cloudrecovery.ProjectionSearchIndex, action: sdk.ResilienceIntegrityCheck, want: []string{"bin/magento", "indexer:status"}},
		{name: "cache reconstruct", dataClass: cloudrecovery.ProjectionCache, action: sdk.ResilienceRestore, want: []string{"bin/magento", "cache:flush"}},
		{name: "cache verify", dataClass: cloudrecovery.ProjectionCache, action: sdk.ResilienceIntegrityCheck, want: []string{"bin/magento", "cache:status"}},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runner := &fakeProjectionCommandRunner{result: ProjectionCommandResult{
				Status: sdk.ResilienceOperationSucceeded, OperationID: "operation-1", ResourceReference: "kubernetes-projection://default/pod-1", RestoreDurationSeconds: 2,
			}}
			backend, err := NewKubernetesProjectionBackend(KubernetesProjectionConfig{
				Runner: runner, Verifier: validKubernetesProjectionVerifier,
			})
			if err != nil {
				t.Fatal(err)
			}
			request := validKubernetesProjectionRequest(test.dataClass, test.action)
			lifecycle, err := cloudrecovery.NewProjectionLifecycle(backend)
			if err != nil {
				t.Fatal(err)
			}
			var result cloudrecovery.ProjectionResult
			if test.action == sdk.ResilienceRestore {
				result, err = lifecycle.Rebuild(context.Background(), request)
			} else {
				result, err = lifecycle.Verify(context.Background(), request)
			}
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(runner.request.Command, test.want) {
				t.Fatalf("command = %#v, want %#v", runner.request.Command, test.want)
			}
			if result.ResourceReference != runner.result.ResourceReference {
				t.Fatalf("resource reference = %q, want %q", result.ResourceReference, runner.result.ResourceReference)
			}
		})
	}
}

func TestKubernetesProjectionBackendRequiresVerifierAndPreservesExecutionIdentity(t *testing.T) {
	t.Parallel()
	runner := &fakeProjectionCommandRunner{result: ProjectionCommandResult{Status: sdk.ResilienceOperationSucceeded, OperationID: "operation-1", ResourceReference: "kubernetes-projection://default/pod-1", RestoreDurationSeconds: 2}}
	if _, err := NewKubernetesProjectionBackend(KubernetesProjectionConfig{Runner: runner}); err == nil || !strings.Contains(err.Error(), "verifier is required") {
		t.Fatalf("missing verifier error = %v", err)
	}
	backend, err := NewKubernetesProjectionBackend(KubernetesProjectionConfig{Runner: runner, Verifier: func(context.Context, cloudrecovery.ProjectionRequest, ProjectionCommandRequest, ProjectionCommandResult) (cloudrecovery.ProjectionResult, error) {
		return cloudrecovery.ProjectionResult{Status: sdk.ResilienceOperationSucceeded, OperationID: "foreign-operation"}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	request := validKubernetesProjectionRequest(cloudrecovery.ProjectionSearchIndex, sdk.ResilienceRestore)
	if _, err := backend.Rebuild(context.Background(), request); err == nil || !strings.Contains(err.Error(), "different operation identity") {
		t.Fatalf("identity mismatch error = %v", err)
	}
}

func TestNewKubernetesProjectionLifecycleBuildsFromKubeconfig(t *testing.T) {
	t.Parallel()
	kubeconfig := BuildStaticTokenKubeconfig("shop-cluster", "https://1.2.3.4:6443", testCAData, "mock-token")
	lifecycle, err := NewKubernetesProjectionLifecycle(
		[]byte(kubeconfig),
		KubernetesCommandRunnerConfig{Namespace: "default", Workload: "shop-web", Container: "php"},
		validKubernetesProjectionVerifier,
	)
	if err != nil {
		t.Fatalf("NewKubernetesProjectionLifecycle: %v", err)
	}
	if lifecycle == nil {
		t.Fatal("NewKubernetesProjectionLifecycle returned nil lifecycle")
	}
}

func TestNewKubernetesProjectionLifecycleRejectsInvalidBoundaryInputs(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		kubeconfig []byte
		verifier   ProjectionVerifier
		want       string
	}{
		{name: "empty kubeconfig", kubeconfig: nil, verifier: validKubernetesProjectionVerifier, want: "kubeconfig is empty"},
		{name: "missing verifier", kubeconfig: []byte(BuildStaticTokenKubeconfig("shop-cluster", "https://1.2.3.4:6443", testCAData, "mock-token")), want: "verifier is required"},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewKubernetesProjectionLifecycle(
				test.kubeconfig,
				KubernetesCommandRunnerConfig{Namespace: "default", Workload: "shop-web"},
				test.verifier,
			)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestKubernetesProjectionBackendDoesNotRunAfterCancellation(t *testing.T) {
	t.Parallel()
	runner := &fakeProjectionCommandRunner{result: ProjectionCommandResult{Status: sdk.ResilienceOperationSucceeded, RestoreDurationSeconds: 1}}
	backend, err := NewKubernetesProjectionBackend(KubernetesProjectionConfig{Runner: runner, Verifier: validKubernetesProjectionVerifier})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = backend.Rebuild(ctx, validKubernetesProjectionRequest(cloudrecovery.ProjectionSearchIndex, sdk.ResilienceRestore))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context cancellation", err)
	}
	if runner.request.Command != nil {
		t.Fatalf("runner was called after cancellation: %#v", runner.request)
	}
}

func TestKubernetesCommandRunnerSelectsDeterministicReadyPodAndContainer(t *testing.T) {
	t.Parallel()
	ready := func(name string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default", Labels: map[string]string{"app": "shop-web"}},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "php"}}},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning, Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}}},
		}
	}
	runner, err := NewKubernetesCommandRunner(fake.NewSimpleClientset(ready("pod-z"), ready("pod-a")), &rest.Config{Host: "https://kubernetes.invalid"}, KubernetesCommandRunnerConfig{Namespace: "default", Workload: "shop-web"})
	if err != nil {
		t.Fatal(err)
	}
	pod, container, err := runner.readyPod(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pod.Name != "pod-a" || container != "php" {
		t.Fatalf("selected pod/container = %s/%s, want pod-a/php", pod.Name, container)
	}
}

func TestKubernetesProjectionBackendWipesTransientCommandOutput(t *testing.T) {
	t.Parallel()
	runner := &fakeProjectionCommandRunner{result: ProjectionCommandResult{
		Status: sdk.ResilienceOperationSucceeded, OperationID: "operation-1", ResourceReference: "kubernetes-projection://default/pod-1",
		Stdout: []byte("sensitive-output"), Stderr: []byte("diagnostic-output"), RestoreDurationSeconds: 2,
	}}
	verifier := func(ctx context.Context, request cloudrecovery.ProjectionRequest, command ProjectionCommandRequest, execution ProjectionCommandResult) (cloudrecovery.ProjectionResult, error) {
		if string(execution.Stdout) != "sensitive-output" || string(execution.Stderr) != "diagnostic-output" {
			t.Fatalf("verifier did not receive command output")
		}
		return validKubernetesProjectionVerifier(ctx, request, command, execution)
	}
	backend, err := NewKubernetesProjectionBackend(KubernetesProjectionConfig{Runner: runner, Verifier: verifier})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Rebuild(context.Background(), validKubernetesProjectionRequest(cloudrecovery.ProjectionSearchIndex, sdk.ResilienceRestore)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(runner.result.Stdout, make([]byte, len(runner.result.Stdout))) || !bytes.Equal(runner.result.Stderr, make([]byte, len(runner.result.Stderr))) {
		t.Fatalf("transient command output was not wiped: stdout=%q stderr=%q", runner.result.Stdout, runner.result.Stderr)
	}
}
