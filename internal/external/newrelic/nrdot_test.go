package newrelic

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/sdk"
)

func TestBuildNRDOTKubernetesValuesUsesDocumentedSecretAndSignalKeys(t *testing.T) {
	values, err := BuildNRDOTKubernetesValues(kube.HelmCollectorReleaseRequest{
		TargetProvider: "gcp", TargetRuntime: "gke-autopilot", NativeReference: "cluster-ref",
		Namespace: "magelift", ChartReference: NRDOTKubernetesChartReference, ChartVersion: "1.19.0", Distribution: NRDOTKubernetesDistribution,
		Endpoint: "https://otlp.eu01.nr-data.net", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", OwnershipMarker: "magelift/test/nrdot",
		Signals: []string{"logs", "metrics", "events"},
	}, "gke-autopilot-cluster")
	if err != nil {
		t.Fatal(err)
	}
	if values.Cluster != "gke-autopilot-cluster" || values.Provider != "GKE_AUTOPILOT" || values.CustomSecretName != "new-relic" || values.CustomSecretLicenseKey != "license-key" {
		t.Fatalf("values = %#v", values)
	}
	if values.Images.Collector.Repository != "newrelic/nrdot-collector" || !values.RBAC.Create || !values.ServiceAccount.Create {
		t.Fatalf("chart identity/RBAC values = %#v", values)
	}
	if values.Labels["magelift.dev/managed-by"] != "magelift" || values.Labels["magelift.dev/ownership-digest"] == "" || values.PodLabels["magelift.dev/ownership-digest"] != values.Labels["magelift.dev/ownership-digest"] {
		t.Fatalf("ownership labels = %#v podLabels = %#v", values.Labels, values.PodLabels)
	}
	if !values.Receivers.Filelog.Enabled || !values.Receivers.Prometheus.Enabled || !values.Receivers.K8sEvents.Enabled {
		t.Fatalf("receiver values = %#v", values.Receivers)
	}
	for _, arg := range append(values.DaemonSet.ExtraArgs, values.Deployment.ExtraArgs...) {
		if !strings.Contains(arg, "https://otlp.eu01.nr-data.net") {
			t.Fatalf("endpoint argument = %q", arg)
		}
	}
}

func TestBuildNRDOTKubernetesValuesSupportsFirstPartyManagedKubernetesTargets(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider sdk.ProviderID
		runtime  sdk.RuntimeID
	}{
		{name: "scaleway kapsule", provider: "scaleway", runtime: "kapsule"},
		{name: "ovh mks", provider: "ovh", runtime: "mks"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := BuildNRDOTKubernetesValues(kube.HelmCollectorReleaseRequest{
				TargetProvider: test.provider, TargetRuntime: test.runtime, NativeReference: "cluster-ref",
				Namespace: "magelift", ChartReference: NRDOTKubernetesChartReference, ChartVersion: "1.19.0", Distribution: NRDOTKubernetesDistribution,
				Endpoint: "https://otlp.nr-data.net", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", OwnershipMarker: "magelift/test/nrdot", Signals: []string{"metrics"},
			}, "managed-kubernetes-cluster")
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestBuildNRDOTKubernetesValuesRejectsUndocumentedTraceSignalAndForeignSecretNamespace(t *testing.T) {
	base := kube.HelmCollectorReleaseRequest{
		TargetProvider: "aws", TargetRuntime: "eks", NativeReference: "cluster-ref",
		Namespace: "magelift", ChartReference: NRDOTKubernetesChartReference, ChartVersion: "1.19.0", Distribution: NRDOTKubernetesDistribution,
		Endpoint: "https://otlp.nr-data.net", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", OwnershipMarker: "magelift/test/nrdot", Signals: []string{"traces"},
	}
	if _, err := BuildNRDOTKubernetesValues(base, "eks-cluster"); err == nil || !strings.Contains(err.Error(), "does not document") {
		t.Fatalf("trace error = %v", err)
	}
	base.Signals = []string{"metrics"}
	base.CredentialRef = "kubernetes-secret://other/new-relic#license-key"
	if _, err := BuildNRDOTKubernetesValues(base, "eks-cluster"); err == nil || !strings.Contains(err.Error(), "release namespace") {
		t.Fatalf("namespace error = %v", err)
	}
}

func TestBuildNRDOTKubernetesValuesRejectsMutableChartVersion(t *testing.T) {
	request := kube.HelmCollectorReleaseRequest{
		TargetProvider: "aws", TargetRuntime: "eks", NativeReference: "cluster-ref",
		Namespace: "magelift", ChartReference: NRDOTKubernetesChartReference, ChartVersion: "latest",
		Distribution: NRDOTKubernetesDistribution, Endpoint: "https://otlp.nr-data.net", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", OwnershipMarker: "magelift/test/nrdot", Signals: []string{"metrics"},
	}
	if _, err := BuildNRDOTKubernetesValues(request, "eks-cluster"); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("version error = %v", err)
	}
}

func TestNRDOTHelmCollectorAPITranslatesBeforeProviderInstall(t *testing.T) {
	api := &fakeNRDOTHelmReleaseAPI{}
	adapter, err := NewNRDOTHelmCollectorAPI(api, func(_ context.Context, reference string) (string, error) {
		if reference != "cluster-ref" {
			t.Fatalf("cluster reference = %q", reference)
		}
		return "eks-cluster", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	request := kube.HelmCollectorReleaseRequest{
		TargetProvider: "aws", TargetRuntime: "eks", NativeReference: "cluster-ref",
		Namespace: "magelift", Name: "nrdot", ChartReference: NRDOTKubernetesChartReference, ChartVersion: "1.19.0",
		Distribution: NRDOTKubernetesDistribution, Endpoint: "https://otlp.nr-data.net", CredentialRef: "kubernetes-secret://magelift/new-relic#license-key", OwnershipMarker: "magelift/test/nrdot", Signals: []string{"metrics"},
	}
	if _, err := adapter.InstallRelease(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if api.values.Cluster != "eks-cluster" || api.values.CustomSecretName != "new-relic" || api.values.CustomSecretLicenseKey != "license-key" {
		t.Fatalf("translated values = %#v", api.values)
	}
	if api.values.Images.Collector.Repository == "license-key" || api.values.CustomSecretName == "license-value" {
		t.Fatal("secret value crossed the provider values boundary")
	}
}

type fakeNRDOTHelmReleaseAPI struct {
	values NRDOTKubernetesValues
}

func (api *fakeNRDOTHelmReleaseAPI) GetRelease(context.Context, string, string) (kube.HelmCollectorRelease, bool, error) {
	return kube.HelmCollectorRelease{}, false, nil
}

func (api *fakeNRDOTHelmReleaseAPI) InstallNRDOTRelease(_ context.Context, request kube.HelmCollectorReleaseRequest, values NRDOTKubernetesValues) (kube.HelmCollectorRelease, error) {
	api.values = values
	return kube.HelmCollectorRelease{
		Namespace: request.Namespace, Name: request.Name, ChartReference: request.ChartReference, ChartVersion: request.ChartVersion,
		Distribution: request.Distribution, ConfigurationID: request.ConfigurationID, OwnershipMarker: request.OwnershipMarker,
		Ready: true, Healthy: true,
	}, nil
}

func (api *fakeNRDOTHelmReleaseAPI) UninstallRelease(context.Context, string, string) error {
	return nil
}

var _ kube.HelmCollectorAPI = (*NRDOTHelmCollectorAPI)(nil)
