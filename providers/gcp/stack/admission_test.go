package stack

import (
	"context"
	"errors"
	"testing"

	gcpprovider "github.com/magelift/magelift/providers/gcp/provider"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
	"github.com/magelift/magelift/sdk"
)

func TestSelectionFromSpecUsesReleaseAwareCloudSQLAndGCPCatalog(t *testing.T) {
	spec := Spec{
		Identity:    Identity{GCPProject: "shop-prod", Region: "europe-west1", Runtime: gcptarget.RuntimeStandardID, Preset: sdk.PresetStandard},
		Policy:      NetworkPolicy{Zones: []string{"europe-west1-b", "europe-west1-c"}},
		Application: Application{Version: "2.4.9"},
		Catalog: CatalogSelection{
			CloudSQLTier: "db-perf-optimized-N-2", MemorystoreEngineVersion: "VALKEY_9_0", MemorystoreNodeType: "STANDARD_SMALL", MemorystoreMode: "CLUSTER",
			KubernetesVersion: "1.34.5-gke.123", ReleaseChannel: "REGULAR", StandardNodeType: "e2-standard-4",
		},
	}
	selection := selectionFromSpec(spec)
	if selection.CloudSQLVersion != "MYSQL_8_4" {
		t.Fatalf("Cloud SQL version = %q, want MYSQL_8_4", selection.CloudSQLVersion)
	}
	if selection.MemorystoreVersion != "VALKEY_9_0" || selection.MemorystoreNode != "STANDARD_SMALL" {
		t.Fatalf("Memorystore selection = %#v", selection)
	}
	if selection.Runtime != string(gcptarget.RuntimeStandardID) || selection.MachineType != "e2-standard-4" {
		t.Fatalf("GKE selection = %#v", selection)
	}
	if len(selection.RequiredServices) < 8 {
		t.Fatalf("required GCP services = %#v, want core service gate", selection.RequiredServices)
	}
}

func TestAdmitSpecFailsClosed(t *testing.T) {
	t.Parallel()
	var nilCtx context.Context
	if _, err := (RegionAdmission{}).AdmitSpec(nilCtx, Spec{}); err == nil {
		t.Fatal("AdmitSpec(nil ctx) succeeded, want error")
	}
	failing := RegionAdmission{NewClient: func(context.Context) (gcpprovider.CapabilityAPI, error) {
		return nil, errors.New("boom")
	}}
	if _, err := failing.AdmitSpec(context.Background(), Spec{}); err == nil {
		t.Fatal("AdmitSpec(client error) succeeded, want error")
	}
}

func TestTeardownDoesNotDependOnCurrentCloudServiceCatalog(t *testing.T) {
	admission := RegionAdmission{NewClient: func(context.Context) (gcpprovider.CapabilityAPI, error) {
		return nil, errors.New("catalog unavailable")
	}}
	spec := Spec{AllowExpiredPreview: true}
	if _, err := admission.AdmitSpec(context.Background(), spec); err != nil {
		t.Fatalf("teardown admission: %v", err)
	}
}

func TestAdmissionSelectsManagedSessionWhenCacheIsSelfHosted(t *testing.T) {
	spec := Spec{Services: map[string]sdk.ServiceBinding{
		"cache":   {Family: "valkey", Ownership: "self-hosted", Version: "7.2"},
		"session": {Family: "valkey", Ownership: "managed", Version: "8.1"},
	}}
	got := selectionFromSpec(spec)
	if got.SkipMemorystore || got.MemorystoreVersion != "VALKEY_8_1" {
		t.Fatalf("selection = %#v", got)
	}
}
