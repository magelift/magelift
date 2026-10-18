package stack

import (
	"slices"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func TestProgramCloudSQLUsesFirstPolicyZone(t *testing.T) {
	in := gcpDeploymentInputs()
	in.Target.Zones = []string{"us-central1-c", "us-central1-f"}
	spec, err := PlanFromInputs(in)
	if err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
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
			t.Fatalf("zone = %q, want us-central1-c", pref["zone"].StringValue())
		}
	}
	if !found {
		t.Fatal("expected Cloud SQL database instance in mock graph")
	}
}

func TestProgramCloudSQLIgnoresZoneChangesAfterCreate(t *testing.T) {
	spec, err := PlanFromInputs(gcpDeploymentInputs())
	if err != nil {
		t.Fatal(err)
	}
	mocks := &stackMocks{}
	if err := pulumi.RunErr(Program(spec), pulumi.WithMocks("magelift", "shop-staging", mocks)); err != nil {
		t.Fatal(err)
	}
	for _, res := range mocks.resources {
		if res.TypeToken != "gcp:sql/databaseInstance:DatabaseInstance" {
			continue
		}
		if res.RegisterRPC == nil {
			t.Fatal("missing resource options")
		}
		if !slices.Contains(res.RegisterRPC.GetIgnoreChanges(), "settings.locationPreference.zone") {
			t.Fatalf("legacy Cloud SQL zone could be replaced: %v", res.RegisterRPC.GetIgnoreChanges())
		}
		return
	}
	t.Fatal("expected Cloud SQL database instance in mock graph")
}
