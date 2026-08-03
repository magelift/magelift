package target

import (
	"context"
	"testing"

	"github.com/magelift/magelift/internal/infra"
	"github.com/magelift/magelift/internal/topology"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestRegisterAndValidate(t *testing.T) {
	registry := infra.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatal(err)
	}
	target, found := registry.Target(ProviderID, RuntimeID)
	if !found {
		t.Fatal("target not registered")
	}
	topology, err := topology.ForPreset(sdk.PresetPreview)
	if err != nil {
		t.Fatal(err)
	}
	if err := target.Validate(context.Background(), sdk.TargetRequest{
		EnvironmentClass: "preview", Topology: topology,
	}); err != nil {
		t.Fatal(err)
	}
}
