package v1

import (
	"strings"
	"testing"
)

func TestValidateDesiredTopologyRejectsInvalidShape(t *testing.T) {
	t.Parallel()
	topology := DesiredTopology{
		Preset:       PresetPreview,
		Availability: Availability{},
		Workloads: []WorkloadRequirement{
			{ID: "web", Availability: Availability{MinimumZones: 1}, HorizontalScaling: true, Singleton: true},
			{ID: "web", Availability: Availability{MinimumZones: 1}},
		},
		Capabilities: []CapabilityRequirement{
			{ID: "search", Kind: CapabilitySearch, Availability: Availability{MinimumZones: 1}, Durable: true, ScaleToZeroAllowed: true},
			{ID: "search", Kind: "invalid", Purpose: "search", Availability: Availability{MinimumZones: 1}, PointInTimeRecovery: true},
		},
	}
	err := ValidateDesiredTopology(topology)
	for _, message := range []string{"minimum zones", "singleton", "duplicate workload", "requires a purpose", "cannot scale to zero", "invalid capability kind", "point-in-time recovery", "duplicate capability"} {
		if err == nil || !strings.Contains(err.Error(), message) {
			t.Fatalf("error %q does not contain %q", err, message)
		}
	}
}
