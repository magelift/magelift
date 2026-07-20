package target

import (
	"context"
	"testing"

	"github.com/acourtiol/magelift/internal/infra"
	"github.com/acourtiol/magelift/internal/topology"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func TestRegisterAndValidate(t *testing.T) {
	registry := infra.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatal(err)
	}
	got, ok := registry.Target(ProviderID, RuntimeID)
	if !ok {
		t.Fatal("expected OVH target registration")
	}
	desired, err := topology.ForPreset(sdk.PresetPreview)
	if err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(context.Background(), sdk.TargetRequest{
		EnvironmentClass: "preview",
		Topology:         desired,
	}); err != nil {
		t.Fatal(err)
	}
}
