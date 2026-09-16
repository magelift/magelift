package target

import (
	"context"
	"testing"

	"github.com/magelift/magelift/internal/topology"
	"github.com/magelift/magelift/sdk"
)

func TestValidate(t *testing.T) {
	preset, err := topology.ForPreset(sdk.PresetPreview)
	if err != nil {
		t.Fatal(err)
	}
	if err := (Target{}).Validate(context.Background(), sdk.TargetRequest{
		EnvironmentClass: "preview", Topology: preset,
	}); err != nil {
		t.Fatal(err)
	}
	if got := (Target{}).Descriptor(); got.ID != TargetID || got.Provider != ProviderID || got.Runtime != RuntimeID {
		t.Fatalf("descriptor = %#v", got)
	}
}
