package topology

import (
	"reflect"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestPresetTopologiesAreDeterministicAndValid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset       sdk.PresetID
		zones        int
		capabilities int
		disposable   bool
	}{
		{sdk.PresetPreview, 1, 7, true},
		{sdk.PresetStandard, 2, 8, false},
		{sdk.PresetHighAvailability, 3, 8, false},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.preset), func(t *testing.T) {
			t.Parallel()
			first, err := ForPreset(test.preset)
			if err != nil {
				t.Fatal(err)
			}
			second, err := ForPreset(test.preset)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatal("preset topology changed between calls")
			}
			if err := sdk.ValidateDesiredTopology(first); err != nil {
				t.Fatalf("invalid topology: %v", err)
			}
			if first.Availability.MinimumZones != test.zones || len(first.Capabilities) != test.capabilities || first.Disposable != test.disposable {
				t.Fatalf("topology = %#v", first)
			}
		})
	}
}

func TestPresetCapabilityPolicies(t *testing.T) {
	t.Parallel()
	preview, _ := ForPreset(sdk.PresetPreview)
	if !preview.RequiresTTL || !preview.RequiresBudget {
		t.Fatal("preview does not require TTL and budget controls")
	}
	if capabilityByID(t, preview, "database").Availability.MinimumZones != 2 {
		t.Fatal("preview database does not span the minimum Aurora subnet zones")
	}
	if capabilityByID(t, preview, "cache").Purpose != "cache-and-sessions" || !capabilityByID(t, preview, "search").ScaleToZeroAllowed {
		t.Fatal("preview capability policy is incomplete")
	}
	if capabilityByID(t, preview, "sessions").ID != "" {
		t.Fatal("preview unexpectedly has dedicated sessions")
	}

	for _, preset := range []sdk.PresetID{sdk.PresetStandard, sdk.PresetHighAvailability} {
		topology, _ := ForPreset(preset)
		if !capabilityByID(t, topology, "database").PointInTimeRecovery {
			t.Fatalf("%s database does not require PITR", preset)
		}
		if !capabilityByID(t, topology, "cache").Dedicated || !capabilityByID(t, topology, "sessions").Dedicated {
			t.Fatalf("%s does not isolate cache and sessions", preset)
		}
		if preset == sdk.PresetStandard && capabilityByID(t, topology, "queue").Availability.MinimumZones != 3 {
			t.Fatal("standard RabbitMQ queue does not require three availability zones")
		}
	}
}

func TestUnknownPresetFails(t *testing.T) {
	t.Parallel()
	if _, err := ForPreset("custom"); err == nil {
		t.Fatal("unknown preset was accepted")
	}
}

func capabilityByID(t *testing.T, topology sdk.DesiredTopology, id sdk.CapabilityID) sdk.CapabilityRequirement {
	t.Helper()
	for _, capability := range topology.Capabilities {
		if capability.ID == id {
			return capability
		}
	}
	return sdk.CapabilityRequirement{}
}
