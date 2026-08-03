package topology

import (
	"fmt"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

func ForPreset(preset sdk.PresetID) (sdk.DesiredTopology, error) {
	availability := func(zones int) sdk.Availability {
		return sdk.Availability{MinimumZones: zones, AutomaticRecovery: true}
	}
	capability := func(id sdk.CapabilityID, kind sdk.CapabilityKind, purpose string, zones int) sdk.CapabilityRequirement {
		return sdk.CapabilityRequirement{ID: id, Kind: kind, Purpose: purpose, Availability: availability(zones)}
	}

	switch preset {
	case sdk.PresetPreview:
		capabilities := []sdk.CapabilityRequirement{
			capability("database", sdk.CapabilityDatabase, "application-data", 2),
			capability("cache", sdk.CapabilityCache, "cache-and-sessions", 1),
			capability("search", sdk.CapabilitySearch, "catalog-search", 1),
			capability("queue", sdk.CapabilityQueue, "database-backed-messaging", 1),
			capability("object-storage", sdk.CapabilityObjectStorage, "media", 1),
			capability("edge", sdk.CapabilityEdge, "public-ingress", 1),
			capability("observability", sdk.CapabilityObservability, "operations", 1),
		}
		capabilities[2].ScaleToZeroAllowed = true
		capabilities[0].Durable = true
		capabilities[3].Durable = true
		capabilities[4].Durable = true
		return sdk.DesiredTopology{
			Preset: preset, Availability: availability(1), Disposable: true, RequiresTTL: true, RequiresBudget: true,
			Workloads: []sdk.WorkloadRequirement{
				{ID: "web", Availability: availability(1)},
				{ID: "deploy", Availability: sdk.Availability{MinimumZones: 1}, Singleton: true},
				{ID: "cron", Availability: availability(1), Singleton: true},
				{ID: "queue-consumer", Availability: availability(1)},
			},
			Capabilities: capabilities,
		}, nil
	case sdk.PresetStandard, sdk.PresetHighAvailability:
		zones := 2
		if preset == sdk.PresetHighAvailability {
			zones = 3
		}
		capabilities := []sdk.CapabilityRequirement{
			capability("database", sdk.CapabilityDatabase, "application-data", zones),
			capability("cache", sdk.CapabilityCache, "cache", zones),
			capability("sessions", sdk.CapabilityCache, "sessions", zones),
			capability("search", sdk.CapabilitySearch, "catalog-search", zones),
			capability("queue", sdk.CapabilityQueue, "messaging", zones),
			capability("object-storage", sdk.CapabilityObjectStorage, "media", zones),
			capability("edge", sdk.CapabilityEdge, "public-ingress", zones),
			capability("observability", sdk.CapabilityObservability, "operations", zones),
		}
		capabilities[0].Durable = true
		capabilities[0].PointInTimeRecovery = true
		capabilities[1].Dedicated = true
		capabilities[2].Dedicated = true
		capabilities[4].Durable = true
		capabilities[5].Durable = true
		if preset == sdk.PresetStandard {
			capabilities[4].Availability.MinimumZones = 3
		}
		return sdk.DesiredTopology{
			Preset: preset, Availability: availability(zones),
			Workloads: []sdk.WorkloadRequirement{
				{ID: "web", Availability: availability(zones), HorizontalScaling: true},
				{ID: "deploy", Availability: sdk.Availability{MinimumZones: 1}, Singleton: true},
				{ID: "cron", Availability: availability(zones), Singleton: true},
				{ID: "queue-consumer", Availability: availability(zones), HorizontalScaling: true},
			},
			Capabilities: capabilities,
		}, nil
	default:
		return sdk.DesiredTopology{}, fmt.Errorf("unknown topology preset %q", preset)
	}
}
