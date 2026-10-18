package database

import (
	"slices"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type sqlInstanceMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *sqlInstanceMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "gcp:compute/globalAddress:GlobalAddress":
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:servicenetworking/connection:Connection":
	case "gcp:sql/databaseInstance:DatabaseInstance":
		state["privateIpAddress"] = resource.NewStringProperty("10.0.0.5")
		state["connectionName"] = resource.NewStringProperty("p:us-central1:sql")
		state["name"] = resource.NewStringProperty(args.Name)
	case "gcp:sql/database:Database", "gcp:sql/user:User":
	case "gcp:secretmanager/secret:Secret":
		state["secretId"] = resource.NewStringProperty(args.Name)
	default:
	}
	return args.Name + "-id", state, nil
}

func (*sqlInstanceMocks) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func TestNewCloudSQLInstanceSetsLocationPreferenceZone(t *testing.T) {
	mocks := &sqlInstanceMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-staging-sql", Args{
			Project: "example-gcp-project", Region: "us-central1",
			NetworkID:       pulumi.String("projects/p/global/networks/n"),
			NetworkSelfLink: pulumi.String("https://www.googleapis.com/compute/v1/projects/p/global/networks/n"),
			DatabaseName:    "magento", MasterUsername: "magento",
			DatabaseVersion: "MYSQL_8_4", Tier: "db-perf-optimized-N-2",
			Zone: "us-central1-c",
		})
		return err
	}, pulumi.WithMocks("magelift", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, res := range mocks.resources {
		if res.TypeToken != "gcp:sql/databaseInstance:DatabaseInstance" {
			continue
		}
		found = true
		pref := res.Inputs["settings"].ObjectValue()["locationPreference"].ObjectValue()
		if pref["zone"].StringValue() != "us-central1-c" {
			t.Fatalf("zone = %q", pref["zone"].StringValue())
		}
		if res.RegisterRPC == nil || !slices.Contains(res.RegisterRPC.GetIgnoreChanges(), "settings.locationPreference.zone") {
			t.Fatalf("missing creation-only zone ignore: %v", res.RegisterRPC)
		}
	}
	if !found {
		t.Fatal("expected Cloud SQL database instance in mock graph")
	}
}

func TestNewCloudSQLOmitsLocationPreferenceWhenZoneUnset(t *testing.T) {
	mocks := &sqlInstanceMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-staging-sql", Args{
			Project: "example-gcp-project", Region: "us-central1",
			NetworkID:       pulumi.String("projects/p/global/networks/n"),
			NetworkSelfLink: pulumi.String("https://www.googleapis.com/compute/v1/projects/p/global/networks/n"),
			DatabaseName:    "magento", MasterUsername: "magento",
			DatabaseVersion: "MYSQL_8_4", Tier: "db-perf-optimized-N-2",
		})
		return err
	}, pulumi.WithMocks("magelift", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range mocks.resources {
		if res.TypeToken != "gcp:sql/databaseInstance:DatabaseInstance" {
			continue
		}
		settings := res.Inputs["settings"].ObjectValue()
		if _, ok := settings["locationPreference"]; ok {
			t.Fatalf("unexpected locationPreference: %v", settings["locationPreference"])
		}
		return
	}
	t.Fatal("expected Cloud SQL database instance in mock graph")
}
