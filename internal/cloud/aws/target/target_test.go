package target

import (
	"context"
	"errors"
	"testing"

	"github.com/acourtiol/magelift/internal/infra"
	"github.com/acourtiol/magelift/internal/topology"
	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func TestRegisterMakesAWSTargetSelectable(t *testing.T) {
	registry := infra.NewRegistry()
	if err := Register(registry); err != nil {
		t.Fatal(err)
	}
	target, found := registry.Target(ProviderID, RuntimeID)
	if !found || target.Descriptor() != (sdk.TargetDescriptor{ID: TargetID, Provider: ProviderID, Runtime: RuntimeID}) {
		t.Fatalf("target = %#v, found = %v", target, found)
	}
}

func TestTargetValidationPreservesCancellation(t *testing.T) {
	cause := errors.New("preview canceled")
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(cause)
	if err := (Target{}).Validate(ctx, sdk.TargetRequest{}); !errors.Is(err, cause) {
		t.Fatalf("error = %v", err)
	}
}

func TestRegisterRejectsNilRegistry(t *testing.T) {
	if err := Register(nil); err == nil {
		t.Fatal("nil registry was accepted")
	}
}

func TestTargetAcceptsEachPresetTopology(t *testing.T) {
	t.Parallel()
	for _, preset := range []sdk.PresetID{sdk.PresetPreview, sdk.PresetStandard, sdk.PresetHighAvailability} {
		preset := preset
		t.Run(string(preset), func(t *testing.T) {
			t.Parallel()
			desired, err := topology.ForPreset(preset)
			if err != nil {
				t.Fatal(err)
			}
			if err := (Target{}).Validate(context.Background(), sdk.TargetRequest{Topology: desired}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTargetRejectsTopologyThatWeakensPresetConstraints(t *testing.T) {
	t.Parallel()
	desired, err := topology.ForPreset(sdk.PresetHighAvailability)
	if err != nil {
		t.Fatal(err)
	}
	desired.Availability.MinimumZones = 2
	if err := (Target{}).Validate(context.Background(), sdk.TargetRequest{Topology: desired}); err == nil {
		t.Fatal("weakened high-availability topology was accepted")
	}
}

func TestTargetRejectsPreviewForProduction(t *testing.T) {
	t.Parallel()
	desired, err := topology.ForPreset(sdk.PresetPreview)
	if err != nil {
		t.Fatal(err)
	}
	err = (Target{}).Validate(context.Background(), sdk.TargetRequest{EnvironmentClass: "production", Topology: desired})
	if err == nil || err.Error() != "production environments cannot use the preview preset" {
		t.Fatalf("error = %v", err)
	}
}
