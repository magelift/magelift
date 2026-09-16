package stack

import (
	"context"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	scwprovider "github.com/magelift/magelift/internal/cloud/scaleway/provider"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

type fakeCapabilityAPI struct {
	project                scwprovider.ProjectIdentity
	kubernetesVersions     []scwprovider.KubernetesVersion
	kubernetesClusterTypes []scwprovider.KubernetesClusterType
	instanceTypes          map[string]scwprovider.InstanceType
	instanceAvailability   map[string]string
	databaseEngines        []scwprovider.DatabaseEngine
	databaseNodeTypes      []scwprovider.DatabaseNodeType
	redisVersions          []scwprovider.RedisVersion
	redisNodeTypes         []scwprovider.RedisNodeType
	calls                  int32
}

func (f *fakeCapabilityAPI) Project(context.Context, string) (scwprovider.ProjectIdentity, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.project, nil
}

func (f *fakeCapabilityAPI) KubernetesVersions(context.Context, string) ([]scwprovider.KubernetesVersion, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.kubernetesVersions, nil
}

func (f *fakeCapabilityAPI) KubernetesClusterTypes(context.Context, string) ([]scwprovider.KubernetesClusterType, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.kubernetesClusterTypes, nil
}

func (f *fakeCapabilityAPI) InstanceTypes(context.Context, string) (map[string]scwprovider.InstanceType, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.instanceTypes, nil
}

func (f *fakeCapabilityAPI) InstanceTypeAvailability(context.Context, string) (map[string]string, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.instanceAvailability, nil
}

func (f *fakeCapabilityAPI) DatabaseEngines(context.Context, string) ([]scwprovider.DatabaseEngine, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.databaseEngines, nil
}

func (f *fakeCapabilityAPI) DatabaseNodeTypes(context.Context, string) ([]scwprovider.DatabaseNodeType, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.databaseNodeTypes, nil
}

func (f *fakeCapabilityAPI) RedisVersions(context.Context, string) ([]scwprovider.RedisVersion, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.redisVersions, nil
}

func (f *fakeCapabilityAPI) RedisNodeTypes(context.Context, string) ([]scwprovider.RedisNodeType, error) {
	atomic.AddInt32(&f.calls, 1)
	return f.redisNodeTypes, nil
}

func TestRegionAdmissionReadsAllCatalogsWithoutMutation(t *testing.T) {
	fake := validCapabilityAPI()
	planned := Planned{Spec: validAdmissionSpec()}
	var gotProject, gotRegion, gotZone string

	admitted, err := (RegionAdmission{
		NewClient: func(_ context.Context, project, region, zone string) (scwprovider.CapabilityAPI, error) {
			gotProject, gotRegion, gotZone = project, region, zone
			return fake, nil
		},
		Now: func() time.Time { return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC) },
	}).Admit(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(admitted, planned) {
		t.Fatalf("admission changed an otherwise valid plan: %#v", admitted)
	}
	if gotProject != "project-1" || gotRegion != "fr-par" || gotZone != "fr-par-1" {
		t.Fatalf("capability client scope = %q/%q/%q", gotProject, gotRegion, gotZone)
	}
	if calls := atomic.LoadInt32(&fake.calls); calls != 9 {
		t.Fatalf("read-only capability calls = %d, want 9", calls)
	}
	// CapabilityAPI intentionally has no create/update/delete operation. The
	// fake therefore gives this test a compile-time no-mutation boundary.
}

func TestRegionAdmissionRejectsDeprecatedKubernetesVersionBeforeMutation(t *testing.T) {
	fake := validCapabilityAPI()
	past := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	fake.kubernetesVersions[0].DeprecatedAt = &past

	_, err := (RegionAdmission{
		NewClient: func(context.Context, string, string, string) (scwprovider.CapabilityAPI, error) {
			return fake, nil
		},
		Now: func() time.Time { return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC) },
	}).Admit(context.Background(), Planned{Spec: validAdmissionSpec()})
	if err == nil || !strings.Contains(err.Error(), `Kapsule version "1.36.1" is deprecated`) {
		t.Fatalf("deprecated Kubernetes version error = %v", err)
	}
}

func TestRegionAdmissionRejectsUnavailableNodeTypeBeforeMutation(t *testing.T) {
	fake := validCapabilityAPI()
	fake.instanceAvailability["DEV1-M"] = "out_of_stock"

	_, err := (RegionAdmission{
		NewClient: func(context.Context, string, string, string) (scwprovider.CapabilityAPI, error) {
			return fake, nil
		},
	}).Admit(context.Background(), Planned{Spec: validAdmissionSpec()})
	if err == nil || !strings.Contains(err.Error(), `Instance node type "DEV1-M" is unavailable`) {
		t.Fatalf("unavailable Instance node type error = %v", err)
	}
}

func TestRegionAdmissionRejectsHARequiredDatabaseNodeType(t *testing.T) {
	fake := validCapabilityAPI()
	fake.databaseNodeTypes[0].HARequired = true

	_, err := (RegionAdmission{
		NewClient: func(context.Context, string, string, string) (scwprovider.CapabilityAPI, error) {
			return fake, nil
		},
	}).Admit(context.Background(), Planned{Spec: validAdmissionSpec()})
	if err == nil || !strings.Contains(err.Error(), `database node type "DB-DEV-S" requires high availability`) {
		t.Fatalf("HA-required database node type error = %v", err)
	}
}

func TestRegionAdmissionRejectsRedisZoneMismatch(t *testing.T) {
	fake := validCapabilityAPI()
	fake.redisNodeTypes[0].Zone = "fr-par-2"

	_, err := (RegionAdmission{
		NewClient: func(context.Context, string, string, string) (scwprovider.CapabilityAPI, error) {
			return fake, nil
		},
	}).Admit(context.Background(), Planned{Spec: validAdmissionSpec()})
	if err == nil || !strings.Contains(err.Error(), `Redis node type "RED1-MICRO" is not available in zone`) {
		t.Fatalf("Redis zone mismatch error = %v", err)
	}
}

func TestRegionAdmissionRejectsRedisVersionPastEndOfLifeBeforeMutation(t *testing.T) {
	fake := validCapabilityAPI()
	past := time.Date(2026, time.August, 10, 0, 0, 0, 0, time.UTC)
	fake.redisVersions[0].EndOfLifeAt = &past

	_, err := (RegionAdmission{
		NewClient: func(context.Context, string, string, string) (scwprovider.CapabilityAPI, error) {
			return fake, nil
		},
		Now: func() time.Time { return time.Date(2026, time.August, 11, 0, 0, 0, 0, time.UTC) },
	}).Admit(context.Background(), Planned{Spec: validAdmissionSpec()})
	if err == nil || !strings.Contains(err.Error(), `Redis version "8.6.3" is past end of life`) {
		t.Fatalf("past Redis version error = %v", err)
	}
}

func TestModuleExposesScalewayPlanAdmission(t *testing.T) {
	if platform.ModulePlanAdmission(Module{}) == nil {
		t.Fatal("Scaleway module does not expose plan admission")
	}
}

func validCapabilityAPI() *fakeCapabilityAPI {
	return &fakeCapabilityAPI{
		project:            scwprovider.ProjectIdentity{ID: "project-1", OrganizationID: "organization-1"},
		kubernetesVersions: []scwprovider.KubernetesVersion{{Name: "1.36.1"}},
		kubernetesClusterTypes: []scwprovider.KubernetesClusterType{{
			Name: "kapsule", Availability: "available", MaxNodes: 150, Region: "fr-par",
		}},
		instanceTypes:        map[string]scwprovider.InstanceType{"DEV1-M": {Name: "DEV1-M"}},
		instanceAvailability: map[string]string{"DEV1-M": "available"},
		databaseEngines: []scwprovider.DatabaseEngine{{
			Name: "MySQL", Versions: []scwprovider.DatabaseEngineVersion{{Name: "MySQL-8", Version: "8", EndOfLife: futureTime()}},
		}},
		databaseNodeTypes: []scwprovider.DatabaseNodeType{{
			Name: "db-dev-s", StockStatus: "available", Region: "fr-par",
		}},
		redisVersions:  []scwprovider.RedisVersion{{Version: "8.6.3", EndOfLifeAt: futureTime()}},
		redisNodeTypes: []scwprovider.RedisNodeType{{Name: "RED1-micro", StockStatus: "available", Zone: "fr-par-1"}},
	}
}

func validAdmissionSpec() Spec {
	return Spec{
		Identity: Identity{
			Project: "shop", ScalewayProject: "project-1", Environment: "preview",
			Region: "fr-par", Zone: "fr-par-1", EnvironmentClass: "preview", Preset: sdk.PresetPreview,
		},
		Artifact: Artifact{ImageDigest: "ghcr.io/magelift/magento@sha256:abcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcdefabcd"},
		Policy:   NetworkPolicy{NetworkCIDR: "10.30.0.0/16", Zones: []string{"fr-par-1"}},
		Catalog: CatalogSelection{
			DatabaseNodeType: "DB-DEV-S", RedisNodeType: "RED1-MICRO", RedisVersion: "8.6.3",
			CacheMode: "redis", KapsuleVersion: "1.36.1", NodeType: "DEV1-M", NodeCount: 1, DesiredWebReplicas: 1,
		},
		Dependencies: Dependencies{DatabaseName: "magento", MasterUsername: "magento", EncryptionKeySecret: "magento-key"},
	}
}

func futureTime() *time.Time {
	value := time.Date(2027, time.August, 11, 0, 0, 0, 0, time.UTC)
	return &value
}

var _ platform.PlannedStack = Planned{}
