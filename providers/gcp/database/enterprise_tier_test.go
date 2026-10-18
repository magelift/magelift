package database

import (
	"slices"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestNewCloudSQLMySQL84DedicatedCustomUsesEnterpriseEdition(t *testing.T) {
	mocks := &sqlInstanceMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-staging-sql", Args{
			Project: "example-gcp-project", Region: "us-central1",
			NetworkID:       pulumi.String("projects/p/global/networks/n"),
			NetworkSelfLink: pulumi.String("https://www.googleapis.com/compute/v1/projects/p/global/networks/n"),
			DatabaseName:    "magento", MasterUsername: "magento",
			DatabaseVersion: DatabaseVersionMySQL84,
			Tier:            "db-custom-2-7680",
			Zone:            "us-central1-c",
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
		if got := res.Inputs["databaseVersion"].StringValue(); got != DatabaseVersionMySQL84 {
			t.Fatalf("databaseVersion = %q", got)
		}
		settings := res.Inputs["settings"].ObjectValue()
		if got := settings["tier"].StringValue(); got != "db-custom-2-7680" {
			t.Fatalf("tier = %q", got)
		}
		if got := settings["edition"].StringValue(); got != DatabaseEditionEnterprise {
			t.Fatalf("edition = %q, want ENTERPRISE", got)
		}
		if res.RegisterRPC == nil || !slices.Equal(res.RegisterRPC.GetIgnoreChanges(), []string{"settings.locationPreference.zone"}) {
			t.Fatalf("unexpected ignore list; edition and tier must remain visible: %v", res.RegisterRPC)
		}
	}
	if !found {
		t.Fatal("expected Cloud SQL database instance in mock graph")
	}
}
