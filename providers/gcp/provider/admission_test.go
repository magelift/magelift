package gcpprovider

import (
	"context"
	"strings"
	"testing"
)

type fakeCapabilityAPI struct {
	project      ProjectIdentity
	billing      bool
	services     map[string]bool
	zones        []string
	sqlVersions  map[string]bool
	sqlTiers     map[string]bool
	valkeyShapes map[string]bool
	kubeVersions map[string]bool
	machineTypes map[string]bool
	calls        map[string]int
}

func (f *fakeCapabilityAPI) call(name string) { f.calls[name]++ }
func (f *fakeCapabilityAPI) Project(context.Context, string) (ProjectIdentity, error) {
	f.call("Project")
	return f.project, nil
}
func (f *fakeCapabilityAPI) BillingEnabled(context.Context, string) (bool, error) {
	f.call("BillingEnabled")
	return f.billing, nil
}
func (f *fakeCapabilityAPI) EnabledServices(_ context.Context, _ ProjectIdentity, services []string) (map[string]bool, error) {
	f.call("EnabledServices")
	result := make(map[string]bool, len(services))
	for _, service := range services {
		result[service] = f.services[service]
	}
	return result, nil
}
func (f *fakeCapabilityAPI) RegionZones(context.Context, string, string) ([]string, error) {
	f.call("RegionZones")
	return f.zones, nil
}
func (f *fakeCapabilityAPI) RegionQuota(_ context.Context, _, _, metric string) (float64, error) {
	f.call("RegionQuota:" + metric)
	return 100, nil
}
func (f *fakeCapabilityAPI) CloudSQLDatabaseVersion(_ context.Context, _, version string) (bool, error) {
	f.call("CloudSQLDatabaseVersion:" + version)
	return f.sqlVersions[version], nil
}
func (f *fakeCapabilityAPI) CloudSQLTier(_ context.Context, _, _, tier string) (bool, error) {
	f.call("CloudSQLTier:" + tier)
	return f.sqlTiers[tier], nil
}
func (f *fakeCapabilityAPI) MemorystoreValkey(_ context.Context, _, _, version, node, _ string) (bool, error) {
	f.call("MemorystoreValkey:" + version + ":" + node)
	return f.valkeyShapes[version+":"+node], nil
}
func (f *fakeCapabilityAPI) KubernetesVersion(_ context.Context, _, _, _, version string) (bool, error) {
	f.call("KubernetesVersion:" + version)
	return f.kubeVersions[version], nil
}
func (f *fakeCapabilityAPI) MachineType(_ context.Context, _, zone, machineType string) (bool, error) {
	f.call("MachineType:" + zone + ":" + machineType)
	return f.machineTypes[zone+":"+machineType], nil
}
func (f *fakeCapabilityAPI) MachineTypeCPUs(_ context.Context, _, zone, machineType string) (int64, error) {
	f.call("MachineTypeCPUs:" + zone + ":" + machineType)
	if !f.machineTypes[zone+":"+machineType] {
		return 0, nil
	}
	return 4, nil
}

func validFakeCapabilityAPI() *fakeCapabilityAPI {
	return &fakeCapabilityAPI{
		project:      ProjectIdentity{ID: "shop-prod", Number: 1234},
		billing:      true,
		services:     map[string]bool{"compute.googleapis.com": true, "container.googleapis.com": true},
		zones:        []string{"europe-west1-b", "europe-west1-c", "europe-west1-d"},
		sqlVersions:  map[string]bool{"MYSQL_8_4": true},
		sqlTiers:     map[string]bool{"db-perf-optimized-N-2": true},
		valkeyShapes: map[string]bool{"VALKEY_9_0:STANDARD_SMALL": true},
		kubeVersions: map[string]bool{"1.34.5-gke.123": true},
		machineTypes: map[string]bool{"europe-west1-b:e2-standard-4": true, "europe-west1-c:e2-standard-4": true},
		calls:        make(map[string]int),
	}
}

func validAdmissionSelection() AdmissionSelection {
	return AdmissionSelection{
		ProjectID: "shop-prod", Region: "europe-west1", Zones: []string{"europe-west1-b", "europe-west1-c"}, RequiredServices: []string{"compute.googleapis.com"},
		CloudSQLVersion: "MYSQL_8_4", CloudSQLTier: "db-perf-optimized-N-2",
		MemorystoreVersion: "VALKEY_9_0", MemorystoreNode: "STANDARD_SMALL", MemorystoreMode: "CLUSTER",
		KubernetesVersion: "1.34.5-gke.123", ReleaseChannel: "REGULAR", MachineType: "e2-standard-4", StandardNodeCount: 2, Runtime: "gke-standard",
	}
}

func TestValidateAccountPrepDoesNotQueryDeployCatalogs(t *testing.T) {
	client := validFakeCapabilityAPI()
	client.services["storage.googleapis.com"] = true
	selection := validAdmissionSelection()
	if err := ValidateAccountPrep(context.Background(), client, selection); err != nil {
		t.Fatal(err)
	}
	for name := range client.calls {
		switch name {
		case "Project", "BillingEnabled", "EnabledServices":
		default:
			t.Fatalf("account prep called %s", name)
		}
	}
}

func TestValidateSelectionUsesReadOnlyGCPCapabilityPort(t *testing.T) {
	client := validFakeCapabilityAPI()
	if err := ValidateSelection(context.Background(), client, validAdmissionSelection()); err != nil {
		t.Fatalf("ValidateSelection() error = %v", err)
	}
	for _, name := range []string{
		"Project", "BillingEnabled", "EnabledServices", "RegionZones", "CloudSQLDatabaseVersion:MYSQL_8_4", "CloudSQLTier:db-perf-optimized-N-2",
		"MemorystoreValkey:VALKEY_9_0:STANDARD_SMALL", "KubernetesVersion:1.34.5-gke.123",
		"MachineTypeCPUs:europe-west1-b:e2-standard-4", "RegionQuota:CPUS",
		"MachineType:europe-west1-b:e2-standard-4", "MachineType:europe-west1-c:e2-standard-4",
	} {
		if client.calls[name] != 1 {
			t.Errorf("capability call %q count = %d, want 1", name, client.calls[name])
		}
	}
}

func TestValidateSelectionFailsClosedForCatalogGaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeCapabilityAPI, *AdmissionSelection)
		want   string
	}{
		{name: "project mismatch", mutate: func(_ *fakeCapabilityAPI, selection *AdmissionSelection) { selection.ProjectID = "other" }, want: "project"},
		{name: "zone gap", mutate: func(_ *fakeCapabilityAPI, selection *AdmissionSelection) { selection.Zones[1] = "europe-west1-f" }, want: "zone"},
		{name: "tier gap", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) {
			client.sqlTiers["db-perf-optimized-N-2"] = false
		}, want: "tier"},
		{name: "Valkey 9.1 gap", mutate: func(client *fakeCapabilityAPI, selection *AdmissionSelection) {
			selection.MemorystoreVersion = "VALKEY_9_1"
			client.valkeyShapes["VALKEY_9_1:STANDARD_SMALL"] = false
		}, want: "Valkey"},
		{name: "GKE gap", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) { client.kubeVersions["1.34.5-gke.123"] = false }, want: "Kubernetes"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := validFakeCapabilityAPI()
			selection := validAdmissionSelection()
			test.mutate(client, &selection)
			if err := ValidateSelection(context.Background(), client, selection); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ValidateSelection() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateSelectionFailsClosedForBillingServiceAndQuotaGaps(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeCapabilityAPI, *AdmissionSelection)
		want   string
	}{
		{name: "billing disabled", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) { client.billing = false }, want: "billing"},
		{name: "service disabled", mutate: func(client *fakeCapabilityAPI, _ *AdmissionSelection) {
			client.services["compute.googleapis.com"] = false
		}, want: "service"},
		{name: "quota insufficient", mutate: func(_ *fakeCapabilityAPI, selection *AdmissionSelection) {
			selection.RequiredQuotas = map[string]float64{"CPUS": 101}
		}, want: "quota"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := validFakeCapabilityAPI()
			selection := validAdmissionSelection()
			test.mutate(client, &selection)
			if err := ValidateSelection(context.Background(), client, selection); err == nil || !strings.Contains(strings.ToLower(err.Error()), test.want) {
				t.Fatalf("ValidateSelection() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestCurrentMemorystoreCatalogIncludesGAAndPreviewValkeyNine(t *testing.T) {
	for _, version := range []string{"VALKEY_9_0", "VALKEY_9_1"} {
		if !supportedValkeyVersions[version] {
			t.Errorf("supportedValkeyVersions[%q] = false", version)
		}
	}
	for _, nodeType := range []string{"STANDARD_SMALL", "CUSTOM_MICRO", "HIGHMEM_2XLARGE"} {
		if !supportedValkeyNodeTypes[nodeType] {
			t.Errorf("supportedValkeyNodeTypes[%q] = false", nodeType)
		}
	}
}
