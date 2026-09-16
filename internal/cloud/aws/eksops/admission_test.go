package eksops

import (
	"testing"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	"github.com/magelift/magelift/sdk"
)

func TestSelectionFromSpecCoversEKSComputeAndFckNat(t *testing.T) {
	spec := Spec{
		Identity: Identity{AccountID: "123456789012", Preset: sdk.PresetStandard},
		Policy:   NetworkPolicy{AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat, NatInstanceType: "c6gn.medium"},
		Catalog: CatalogSelection{
			DatabaseEngine: DatabaseEngineAuroraMySQL, KubernetesVersion: "1.36", ComputeMode: ComputeModeSelfManaged,
			NodeInstanceType: "m7i.large", NodeAMI: "ami-1234", AuroraMySQLVersion: "8.0.mysql_aurora.3.08.2",
			InstanceClass: "db.r7g.large", ValkeyVersion: "8.0", ValkeyNodeType: "cache.r7g.large",
		},
	}
	selection := selectionFromSpec(spec)
	if selection.EKSVersion != "1.36" {
		t.Fatalf("EKS version = %q, want 1.36", selection.EKSVersion)
	}
	if len(selection.InstanceTypes) != 2 || selection.InstanceTypes[0].Name != "c6gn.medium" || selection.InstanceTypes[1].Name != "m7i.large" {
		t.Fatalf("instance selections = %#v", selection.InstanceTypes)
	}
	for index, zones := range [][]string{{"eu-west-3a", "eu-west-3b"}, {"eu-west-3a", "eu-west-3b"}} {
		if got := selection.InstanceTypes[index].RequiredAvailabilityZones; len(got) != len(zones) || got[0] != zones[0] || got[1] != zones[1] {
			t.Fatalf("instance selection %d required zones = %#v, want %#v", index, got, zones)
		}
	}
	if len(selection.AMIs) != 1 || selection.AMIs[0] != "ami-1234" {
		t.Fatalf("AMIs = %#v", selection.AMIs)
	}
	if selection.Database == nil || selection.Database.InstanceClass != "db.r7g.large" || selection.Database.ServerlessV2 {
		t.Fatalf("database selection = %#v", selection.Database)
	}
	if selection.Valkey == nil || selection.Valkey.NodeType != "cache.r7g.large" {
		t.Fatalf("Valkey selection = %#v", selection.Valkey)
	}
}

func TestSelectionFromSpecUsesSingleAZOnlyForFckNat(t *testing.T) {
	spec := Spec{
		Identity: Identity{AccountID: "123456789012", Preset: sdk.PresetStandard},
		Policy: NetworkPolicy{
			AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat,
			NatTopology: network.NatTopologySingleAZ,
		},
		Catalog: CatalogSelection{
			DatabaseEngine: DatabaseEngineRDSMySQL, MySQLVersion: "8.0", ValkeyVersion: "8.0", ValkeyNodeType: "cache.t4g.micro",
			InstanceClass: "db.r7g.large", KubernetesVersion: "1.36", ComputeMode: ComputeModeAuto,
		},
	}
	selection := selectionFromSpec(spec)
	if len(selection.InstanceTypes) != 1 {
		t.Fatalf("instance selections = %#v, want only fck-nat", selection.InstanceTypes)
	}
	if got := selection.InstanceTypes[0].RequiredAvailabilityZones; len(got) != 1 || got[0] != "eu-west-3a" {
		t.Fatalf("fck-nat required zones = %#v, want [eu-west-3a]", got)
	}
}

func TestModuleExposesReadOnlyPlanAdmission(t *testing.T) {
	admission := Module{}.PlanAdmission()
	if admission == nil {
		t.Fatalf("PlanAdmission() returned nil")
	}
	if _, ok := admission.(RegionAdmission); !ok {
		t.Fatalf("PlanAdmission() = %T, want eksops.RegionAdmission", admission)
	}
}
