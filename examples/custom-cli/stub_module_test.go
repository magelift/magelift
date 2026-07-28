package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/acourtiol/magelift/internal/config"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func TestStubModuleRegistersAlongsideFirstParty(t *testing.T) {
	t.Parallel()
	modules := platform.NewModuleRegistry()
	for _, module := range []platform.StackModule{
		stubModule{},
	} {
		if err := modules.RegisterModule(module); err != nil {
			t.Fatalf("RegisterModule(stub): %v", err)
		}
	}
	got, ok := modules.Module(sdk.ProviderID("example"), sdk.RuntimeID("community-stub"))
	if !ok {
		t.Fatal("expected stub module registered for example/community-stub")
	}
	if got.Descriptor().ID != "example.community-stub" {
		t.Fatalf("Descriptor().ID = %q, want example.community-stub", got.Descriptor().ID)
	}
	if got.CertificationTier() != platform.TierExperimental {
		t.Fatalf("CertificationTier() = %q, want experimental (community-labeled target)", got.CertificationTier())
	}
}

func TestStubModulePlanRefusesDeploy(t *testing.T) {
	t.Parallel()
	_, err := stubModule{}.Plan(config.Config{}, "staging", platform.PlanOptions{})
	if err == nil {
		t.Fatal("Plan() succeeded; want clear refusal")
	}
	if !errors.Is(err, platform.ErrNotSupported) && !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("Plan() error = %v, want ErrNotSupported-shaped message", err)
	}
}

func TestStubModuleDescriptorDoesNotCollideWithFirstParty(t *testing.T) {
	t.Parallel()
	d := stubModule{}.Descriptor()
	for _, banned := range []string{"aws", "gcp", "ovh", "scaleway"} {
		if string(d.Provider) == banned {
			t.Fatalf("provider %q collides with first-party id", d.Provider)
		}
	}
	for _, banned := range []sdk.TargetID{
		"aws.ecs-fargate", "aws.eks-autopilot", "gcp.gke-autopilot", "ovh.mks", "scaleway.kapsule",
	} {
		if d.ID == banned {
			t.Fatalf("target ID %q collides with first-party id", d.ID)
		}
	}
}
