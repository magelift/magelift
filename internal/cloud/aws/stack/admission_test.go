package stack

import (
	"testing"

	"github.com/magelift/magelift/internal/cloud/aws/network"
	"github.com/magelift/magelift/internal/cloud/aws/runtime"
	"github.com/magelift/magelift/sdk"
)

func TestSelectionFromSpecCoversIndependentAWSInstanceShapes(t *testing.T) {
	spec := Spec{
		Identity: Identity{AccountID: "123456789012", Preset: sdk.PresetStandard},
		Policy: NetworkPolicy{
			AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, NatMode: NatModeFckNat, NatInstanceType: "t4g.nano",
		},
		Catalog: CatalogSelection{
			DatabaseEngine: DatabaseEngineAuroraMySQL, SearchMode: SearchModeProvisioned, QueueMode: QueueModeAmazonMQ,
			Versions:          ServiceVersions{AuroraMySQL: "8.0.mysql_aurora.3.08.2", Valkey: "8.0", OpenSearch: "OpenSearch_2.19", RabbitMQ: "3.13"},
			Valkey:            ValkeyPreviewProfile{NodeType: "cache.r7g.large"},
			AuroraProvisioned: AuroraProvisionedProfile{InstanceClass: "db.r7g.large"},
			SearchProvisioned: SearchProvisionedProfile{InstanceType: "r7g.large.search", DedicatedMasterType: "r6g.large.search"},
			RabbitMQ:          RabbitMQProfile{InstanceType: "mq.m7g.large"},
			Fargate:           FargatePreviewProfile{ComputeMode: runtime.ComputeModeEC2AutoScaling, InstanceType: "m7i.large", InstanceAMI: "ami-1234"},
		},
	}
	selection := selectionFromSpec(spec)
	if len(selection.InstanceTypes) != 2 {
		t.Fatalf("instance selection count = %d, want fck-nat and ECS host selections", len(selection.InstanceTypes))
	}
	if selection.InstanceTypes[0].Name != "t4g.nano" || selection.InstanceTypes[1].Name != "m7i.large" {
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
	if selection.Database == nil || selection.Database.ServerlessV2 || selection.Database.InstanceClass != "db.r7g.large" {
		t.Fatalf("database selection = %#v", selection.Database)
	}
	if selection.Search == nil || len(selection.Search.InstanceTypes) != 2 {
		t.Fatalf("search selection = %#v", selection.Search)
	}
	if selection.MQ == nil || selection.MQ.InstanceType != "mq.m7g.large" {
		t.Fatalf("MQ selection = %#v", selection.MQ)
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
			DatabaseEngine:    DatabaseEngineAuroraMySQL,
			Versions:          ServiceVersions{AuroraMySQL: "8.0.mysql_aurora.3.08.2", Valkey: "8.0"},
			Valkey:            ValkeyPreviewProfile{NodeType: "cache.t4g.micro"},
			AuroraProvisioned: AuroraProvisionedProfile{InstanceClass: "db.r7g.large"},
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
		t.Fatalf("PlanAdmission() = %T, want stack.RegionAdmission", admission)
	}
}
