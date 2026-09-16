package kube

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk"
)

func TestHelmCollectorBackendLifecycleUsesOpaqueCredentialReference(t *testing.T) {
	api := &fakeHelmCollectorAPI{}
	backend, err := NewHelmCollectorBackend(HelmCollectorConfig{
		Namespace: "magelift", ReleaseName: "nrdot-collector", ChartReference: "newrelic/nr-k8s-otel-collector", ChartVersion: "1.4.2",
		API: api, Probe: collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := providerobservability.NewCollectorSDKAdapterWithDistribution(
		"gke.helm.collector", "gcp", "kubernetes",
		func(runtime sdk.RuntimeID) bool { return strings.HasPrefix(string(runtime), "gke-") },
		func(distribution string) bool { return distribution == "newrelic/nr-k8s-otel-collector" }, backend,
	)
	if err != nil {
		t.Fatal(err)
	}
	request := sdk.CollectorDeploymentPlanRequest{
		TargetProvider: "gcp", TargetRuntime: "gke-standard", Workload: "kubernetes", Distribution: "newrelic/nr-k8s-otel-collector",
		CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.eu01.nr-data.net",
		NativeReference: "projects/example/locations/europe-west1/clusters/magelift", OwnershipMarker: "magelift/test/helm-collector",
		Signals: []string{"logs", "metrics", "traces"},
	}
	plan, err := adapter.PlanCollector(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	execution := sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/helm/apply/1", OwnershipMarker: plan.OwnershipMarker}
	result, err := adapter.ExecuteCollector(context.Background(), execution)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !result.OwnershipVerified || !result.ReadyVerified || !result.HealthVerified || api.installCalls != 1 {
		t.Fatalf("apply result = %#v API = %#v", result, api)
	}
	if api.lastRequest.CredentialRef != request.CredentialRef || api.lastRequest.Endpoint != request.Endpoint {
		t.Fatalf("semantic install request = %#v", api.lastRequest)
	}
	if api.lastRequest.CredentialRef == "license-key" || api.lastRequest.ConfigurationID == "" {
		t.Fatalf("install request does not preserve an opaque reference/configuration proof: %#v", api.lastRequest)
	}

	repeated, err := adapter.ExecuteCollector(context.Background(), execution)
	if err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if repeated.OperationID != result.OperationID || api.installCalls != 1 {
		t.Fatalf("repeat apply = %#v API = %#v", repeated, api)
	}

	destroy := execution
	destroy.Action = sdk.CollectorDestroy
	destroy.IdempotencyKey = "collector/helm/destroy/1"
	destroyed, err := adapter.ExecuteCollector(context.Background(), destroy)
	if err != nil {
		t.Fatalf("destroy: %v", err)
	}
	if !destroyed.CleanupVerified || api.uninstallCalls != 1 || api.release != nil {
		t.Fatalf("destroy result = %#v API = %#v", destroyed, api)
	}
}

func TestHelmCollectorBackendRefusesUnownedCollisionAndConfigurationDrift(t *testing.T) {
	tests := []struct {
		name        string
		release     HelmCollectorRelease
		wantMessage string
	}{
		{
			name: "unowned collision", release: HelmCollectorRelease{Namespace: "magelift", Name: "collector", OwnershipMarker: "other", ConfigurationID: "foreign"}, wantMessage: "unowned",
		},
		{
			name: "configuration drift", release: HelmCollectorRelease{Namespace: "magelift", Name: "collector", OwnershipMarker: "magelift/test/helm", ConfigurationID: "stale"}, wantMessage: "configuration drift",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			api := &fakeHelmCollectorAPI{release: &test.release}
			backend, err := NewHelmCollectorBackend(HelmCollectorConfig{
				Namespace: "magelift", ReleaseName: "collector", ChartReference: "oci://registry.example/collector", ChartVersion: "1.0.0", API: api, Probe: collectorProbe{delivered: true},
			})
			if err != nil {
				t.Fatal(err)
			}
			plan := helmTestPlan(test.release.OwnershipMarker)
			if test.name == "unowned collision" {
				plan.OwnershipMarker = "magelift/test/helm"
			}
			resource, found, err := backend.Find(context.Background(), plan)
			if err != nil {
				t.Fatal(err)
			}
			if !found || resource.Owned != (test.name == "configuration drift") {
				t.Fatalf("find = %#v, found %v", resource, found)
			}
			if _, err := backend.Ensure(context.Background(), plan); err == nil || !strings.Contains(err.Error(), test.wantMessage) {
				t.Fatalf("ensure error = %v, want %q", err, test.wantMessage)
			}
			if api.installCalls != 0 {
				t.Fatalf("Ensure mutated a pre-existing release: %#v", api)
			}
		})
	}
}

func TestHelmCollectorBackendRefusesChartIdentityDrift(t *testing.T) {
	api := &fakeHelmCollectorAPI{}
	backend, err := NewHelmCollectorBackend(HelmCollectorConfig{
		Namespace: "magelift", ReleaseName: "collector", ChartReference: "oci://registry.example/collector", ChartVersion: "1.0.0", API: api, Probe: collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := helmTestPlan("magelift/test/chart-drift")
	api.release = &HelmCollectorRelease{
		Namespace: "magelift", Name: "collector", ChartReference: "oci://registry.example/foreign", ChartVersion: "1.0.0",
		Distribution: plan.Distribution, ConfigurationID: helmCollectorConfigurationID(plan, "oci://registry.example/collector", "1.0.0"), OwnershipMarker: plan.OwnershipMarker,
	}
	if _, err := backend.Ensure(context.Background(), plan); err == nil || !strings.Contains(err.Error(), "configuration drift") {
		t.Fatalf("ensure error = %v, want chart identity drift refusal", err)
	}
	if api.installCalls != 0 {
		t.Fatalf("Ensure mutated a drifted release: %#v", api)
	}
}

func TestHelmCollectorBackendDoesNotReportAsyncCleanupAsComplete(t *testing.T) {
	api := &fakeHelmCollectorAPI{retainAfterUninstall: true}
	backend, err := NewHelmCollectorBackend(HelmCollectorConfig{
		Namespace: "magelift", ReleaseName: "collector", ChartReference: "oci://registry.example/collector", ChartVersion: "1.0.0", API: api, Probe: collectorProbe{delivered: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	plan := helmTestPlan("magelift/test/async-cleanup")
	resource, err := backend.Ensure(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := backend.Destroy(ctx, plan, resource); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("destroy error = %v, want context deadline while release remains", err)
	}
	if api.release == nil {
		t.Fatal("fake Helm API unexpectedly removed the asynchronously terminating release")
	}
}

func TestHelmCollectorBackendRollsBackWhenSignalDeliveryIsNotProven(t *testing.T) {
	api := &fakeHelmCollectorAPI{}
	backend, err := NewHelmCollectorBackend(HelmCollectorConfig{
		Namespace: "magelift", ReleaseName: "collector", ChartReference: "oci://registry.example/collector", ChartVersion: "1.0.0", API: api, Probe: collectorProbe{delivered: false},
	})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := providerobservability.NewCollectorSDKAdapter("gke.helm.collector", "gcp", "kubernetes", func(runtime sdk.RuntimeID) bool { return strings.HasPrefix(string(runtime), "gke-") }, backend)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := adapter.PlanCollector(context.Background(), helmTestRequest("magelift/test/rollback"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.ExecuteCollector(context.Background(), sdk.CollectorDeploymentExecutionRequest{Plan: plan, Action: sdk.CollectorApply, IdempotencyKey: "collector/helm/apply/rollback", OwnershipMarker: plan.OwnershipMarker})
	if err == nil || !strings.Contains(err.Error(), "did not prove readiness") || api.uninstallCalls != 1 || api.release != nil {
		t.Fatalf("apply error = %v API = %#v, want signal-gated rollback", err, api)
	}
}

func TestNewHelmCollectorBackendRejectsMutableChartVersion(t *testing.T) {
	_, err := NewHelmCollectorBackend(HelmCollectorConfig{
		Namespace: "magelift", ReleaseName: "collector", ChartReference: "oci://registry.example/collector", ChartVersion: "latest", API: &fakeHelmCollectorAPI{}, Probe: collectorProbe{delivered: true},
	})
	if err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error = %v, want immutable version refusal", err)
	}
}

func helmTestRequest(marker string) sdk.CollectorDeploymentPlanRequest {
	return sdk.CollectorDeploymentPlanRequest{
		TargetProvider: "gcp", TargetRuntime: "gke-standard", Workload: "kubernetes", Distribution: "opentelemetry-collector-contrib",
		CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", Endpoint: "https://otlp.nr-data.net", NativeReference: "cluster/example", OwnershipMarker: marker, Signals: []string{"metrics"},
	}
}

func helmTestPlan(marker string) sdk.CollectorDeploymentPlan {
	request := helmTestRequest(marker)
	return sdk.CollectorDeploymentPlan{
		AdapterID: "gke.helm.collector", TargetProvider: request.TargetProvider, TargetRuntime: request.TargetRuntime, Workload: request.Workload, Distribution: request.Distribution,
		CredentialRef: request.CredentialRef, Endpoint: request.Endpoint, NativeReference: request.NativeReference, OwnershipMarker: request.OwnershipMarker, Signals: request.Signals,
	}
}

type fakeHelmCollectorAPI struct {
	release              *HelmCollectorRelease
	lastRequest          HelmCollectorReleaseRequest
	installCalls         int
	uninstallCalls       int
	retainAfterUninstall bool
}

func (api *fakeHelmCollectorAPI) GetRelease(_ context.Context, namespace, name string) (HelmCollectorRelease, bool, error) {
	if api.release == nil {
		return HelmCollectorRelease{}, false, nil
	}
	release := *api.release
	if release.Namespace == "" {
		release.Namespace = namespace
	}
	if release.Name == "" {
		release.Name = name
	}
	return release, true, nil
}

func (api *fakeHelmCollectorAPI) InstallRelease(_ context.Context, request HelmCollectorReleaseRequest) (HelmCollectorRelease, error) {
	api.installCalls++
	api.lastRequest = request
	api.release = &HelmCollectorRelease{
		Namespace: request.Namespace, Name: request.Name, ChartReference: request.ChartReference, ChartVersion: request.ChartVersion,
		Distribution: request.Distribution, ConfigurationID: request.ConfigurationID, OwnershipMarker: request.OwnershipMarker, Ready: true, Healthy: true,
	}
	return *api.release, nil
}

func (api *fakeHelmCollectorAPI) UninstallRelease(_ context.Context, namespace, name string) error {
	api.uninstallCalls++
	if api.release == nil {
		return errors.New("release already absent")
	}
	if api.release.Namespace != namespace || api.release.Name != name {
		return errors.New("wrong release selected")
	}
	if !api.retainAfterUninstall {
		api.release = nil
	}
	return nil
}

var _ providerobservability.CollectorBackend = (*HelmCollectorBackend)(nil)
var _ HelmCollectorAPI = (*fakeHelmCollectorAPI)(nil)
