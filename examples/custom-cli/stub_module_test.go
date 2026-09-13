package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	magecli "github.com/magelift/magelift/cli"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestStubModuleRegistersAlongsideFirstParty(t *testing.T) {
	t.Parallel()
	command, err := magecli.NewWithExtensions(stubModule{})
	if err != nil {
		t.Fatalf("NewWithExtensions(stub): %v", err)
	}
	if command == nil {
		t.Fatal("expected command")
	}
	if (stubModule{}).Descriptor().Targets[0].ID != "example.community-stub" {
		t.Fatalf("Descriptor target ID changed")
	}
	if (stubModule{}).Descriptor().Tier != sdk.ExtensionTierExperimental {
		t.Fatalf("Tier = %q, want experimental", (stubModule{}).Descriptor().Tier)
	}
}

func TestStubModulePlanRefusesDeploy(t *testing.T) {
	t.Parallel()
	_, err := stubModule{}.Plan(context.Background(), sdk.ModulePlanRequest{})
	if err == nil {
		t.Fatal("Plan() succeeded; want clear refusal")
	}
	if !errors.Is(err, errCommunityStub) && !strings.Contains(err.Error(), "registration demo") {
		t.Fatalf("Plan() error = %v, want clear refusal", err)
	}
}

func TestStubModuleDescriptorDoesNotCollideWithFirstParty(t *testing.T) {
	t.Parallel()
	d := stubModule{}.Descriptor().Targets[0]
	for _, banned := range []string{"aws", "gcp", "ovh", "scaleway"} {
		if string(d.Provider) == banned {
			t.Fatalf("provider %q collides with first-party id", d.Provider)
		}
	}
	for _, banned := range []sdk.TargetID{
		"aws.ecs-fargate", "aws.eks", "gcp.gke-autopilot", "ovh.mks", "scaleway.kapsule",
	} {
		if d.ID == banned {
			t.Fatalf("target ID %q collides with first-party id", d.ID)
		}
	}
}
