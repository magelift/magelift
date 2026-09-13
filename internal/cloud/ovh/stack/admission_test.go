package stack

import (
	"context"
	"fmt"
	"strings"
	"testing"

	ovhprovider "github.com/magelift/magelift/internal/cloud/ovh/provider"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

type admissionAPIFunc func(context.Context, string, interface{}) error

func (f admissionAPIFunc) GetWithContext(ctx context.Context, path string, response interface{}) error {
	return f(ctx, path, response)
}

func TestRegionAdmissionRejectsUnsupportedMKSPlanBeforeMutation(t *testing.T) {
	var paths []string
	planned := Planned{Spec: validAdmissionSpec(sdk.PresetPreview, "free")}
	_, err := (RegionAdmission{NewClient: func(endpoint string) (ovhprovider.API, error) {
		if endpoint != "ovh-eu" {
			t.Fatalf("OVH endpoint = %q", endpoint)
		}
		return admissionAPIFunc(func(_ context.Context, path string, response interface{}) error {
			paths = append(paths, path)
			return setAdmissionResponse(response, "ENABLED", "eu-west-par-a", "eu-west-par-b", "eu-west-par-c")
		}), nil
	}}).Admit(context.Background(), planned)
	if err == nil || !strings.Contains(err.Error(), `plan "free" is not available in region "EU-WEST-PAR"`) {
		t.Fatalf("unsupported plan error = %v", err)
	}
	wantPaths := []string{"/cloud/project/project-1", "/cloud/project/project-1/region/EU-WEST-PAR"}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("read-only admission paths = %#v", paths)
	}
}

func TestRegionAdmissionResolvesProgressiveZoneDefaults(t *testing.T) {
	tests := []struct {
		name      string
		preset    sdk.PresetID
		wantZones []string
		wantNodes int
	}{
		{name: "preview selects one cheapest zone", preset: sdk.PresetPreview, wantZones: []string{"eu-west-par-a"}, wantNodes: 1},
		{name: "high availability selects all zones", preset: sdk.PresetHighAvailability, wantZones: []string{"eu-west-par-a", "eu-west-par-b", "eu-west-par-c"}, wantNodes: 3},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			planned := Planned{Spec: validAdmissionSpec(testCase.preset, "standard")}
			admitted, err := (RegionAdmission{NewClient: func(string) (ovhprovider.API, error) {
				return admissionAPIFunc(func(_ context.Context, _ string, response interface{}) error {
					return setAdmissionResponse(response, "UP", "eu-west-par-c", "eu-west-par-a", "eu-west-par-b")
				}), nil
			}}).Admit(context.Background(), planned)
			if err != nil {
				t.Fatal(err)
			}
			got := admitted.(Planned)
			if strings.Join(got.Spec.Policy.Zones, ",") != strings.Join(testCase.wantZones, ",") || got.Spec.Catalog.NodeCount != testCase.wantNodes {
				t.Fatalf("admitted topology = zones:%v nodes:%d", got.Spec.Policy.Zones, got.Spec.Catalog.NodeCount)
			}
		})
	}
}

func TestRegionAdmissionRejectsUnknownExplicitZone(t *testing.T) {
	spec := validAdmissionSpec(sdk.PresetStandard, "standard")
	spec.Policy.Zones = []string{"eu-west-par-z"}
	spec.Policy.ZonesExplicit = true
	_, err := (RegionAdmission{NewClient: func(string) (ovhprovider.API, error) {
		return admissionAPIFunc(func(_ context.Context, _ string, response interface{}) error {
			return setAdmissionResponse(response, "ENABLED", "eu-west-par-a", "eu-west-par-b", "eu-west-par-c")
		}), nil
	}}).Admit(context.Background(), Planned{Spec: spec})
	if err == nil || !strings.Contains(err.Error(), `availability zone "eu-west-par-z" is not available`) {
		t.Fatalf("unknown zone error = %v", err)
	}
}

func TestRegionAdmissionRejectsUnavailableManagedServiceBeforeMutation(t *testing.T) {
	var paths []string
	spec := validAdmissionSpec(sdk.PresetPreview, "standard")
	spec.Catalog.DatabaseFlavor = "b3-999"
	_, err := (RegionAdmission{NewClient: func(string) (ovhprovider.API, error) {
		return admissionAPIFunc(func(_ context.Context, path string, response interface{}) error {
			paths = append(paths, path)
			return setAdmissionResponse(response, "ENABLED", "eu-west-par-a", "eu-west-par-b", "eu-west-par-c")
		}), nil
	}}).Admit(context.Background(), Planned{Spec: spec})
	if err == nil || !strings.Contains(err.Error(), "MySQL selection is not available") {
		t.Fatalf("unavailable managed service error = %v", err)
	}
	wantPaths := []string{
		"/cloud/project/project-1",
		"/cloud/project/project-1/region/EU-WEST-PAR",
		"/cloud/project/project-1/database/availability",
	}
	if strings.Join(paths, ",") != strings.Join(wantPaths, ",") {
		t.Fatalf("read-only admission paths = %#v, want %#v", paths, wantPaths)
	}
}

func TestRegionAdmissionRejectsPublicOnlyManagedServiceCombination(t *testing.T) {
	spec := validAdmissionSpec(sdk.PresetPreview, "standard")
	_, err := (RegionAdmission{NewClient: func(string) (ovhprovider.API, error) {
		return admissionAPIFunc(func(_ context.Context, responsePath string, response interface{}) error {
			switch value := response.(type) {
			case *ovhprovider.ProjectCapability:
				*value = ovhprovider.ProjectCapability{ID: "project-1"}
			case *ovhprovider.RegionCapability:
				*value = ovhprovider.RegionCapability{
					Name: "EU-WEST-PAR", Status: "ENABLED", AvailabilityZones: []string{"eu-west-par-a"},
				}
			case *[]ovhprovider.DatabaseAvailability:
				*value = []ovhprovider.DatabaseAvailability{
					{
						Engine: "mysql", Version: "8.4", Plan: "discovery", Flavor: "b3-8", Region: "EU-WEST-PAR", Network: "public", MinNodeNumber: 1, MaxNodeNumber: 1,
					},
					{
						Engine: "valkey", Version: "8.1", Plan: "discovery", Flavor: "b3-8", Region: "EU-WEST-PAR", Network: "public", MinNodeNumber: 1, MaxNodeNumber: 1,
					},
				}
			default:
				return fmt.Errorf("unexpected admission response for %s: %T", responsePath, response)
			}
			return nil
		}), nil
	}}).Admit(context.Background(), Planned{Spec: spec})
	if err == nil || !strings.Contains(err.Error(), `network="private"`) {
		t.Fatalf("public-only managed service error = %v", err)
	}
}

func validAdmissionSpec(preset sdk.PresetID, mksPlan string) Spec {
	return Spec{
		Identity: Identity{Project: "shop", ServiceName: "project-1", APIEndpoint: "ovh-eu", Environment: "preview", Region: "EU-WEST-PAR", EnvironmentClass: "preview", Preset: preset},
		Artifact: Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:   NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"EU-WEST-PAR"}},
		Catalog: CatalogSelection{
			DatabaseFlavor: "b3-8", DatabasePlan: "discovery", DatabaseVersion: "8.4", DatabaseNodeCount: 1,
			ValkeyFlavor: "b3-8", ValkeyPlan: "discovery", ValkeyVersion: "8.1", ValkeyNodeCount: 1,
			MKSPlan: mksPlan, NodeCount: 1, DesiredWebReplicas: 1,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-key"},
	}
}

func setAdmissionResponse(response interface{}, status string, zones ...string) error {
	switch value := response.(type) {
	case *ovhprovider.ProjectCapability:
		*value = ovhprovider.ProjectCapability{ID: "project-1"}
	case *ovhprovider.RegionCapability:
		*value = ovhprovider.RegionCapability{Name: "EU-WEST-PAR", Status: status, AvailabilityZones: zones}
	case *[]ovhprovider.DatabaseAvailability:
		*value = []ovhprovider.DatabaseAvailability{
			{Engine: "mysql", Version: "8.4", Plan: "discovery", Flavor: "b3-8", Region: "EU-WEST-PAR", Network: "private", MinNodeNumber: 1, MaxNodeNumber: 1},
			{Engine: "valkey", Version: "8.1", Plan: "discovery", Flavor: "b3-8", Region: "EU-WEST-PAR", Network: "private", MinNodeNumber: 1, MaxNodeNumber: 1},
		}
	default:
		return fmt.Errorf("unexpected admission response type %T", response)
	}
	return nil
}

var _ platform.PlannedStack = Planned{}
