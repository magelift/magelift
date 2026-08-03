package v1

import (
	"errors"
	"fmt"
)

type PresetID string
type WorkloadID string

const (
	PresetPreview          PresetID = "preview"
	PresetStandard         PresetID = "standard"
	PresetHighAvailability PresetID = "high-availability"
)

type Availability struct {
	MinimumZones      int
	AutomaticRecovery bool
}

type WorkloadRequirement struct {
	ID                WorkloadID
	Availability      Availability
	HorizontalScaling bool
	Singleton         bool
}

type CapabilityRequirement struct {
	ID                  CapabilityID
	Kind                CapabilityKind
	Purpose             string
	Availability        Availability
	Dedicated           bool
	Durable             bool
	PointInTimeRecovery bool
	ScaleToZeroAllowed  bool
}

type DesiredTopology struct {
	Preset         PresetID
	Availability   Availability
	Disposable     bool
	RequiresTTL    bool
	RequiresBudget bool
	Workloads      []WorkloadRequirement
	Capabilities   []CapabilityRequirement
}

func ValidateDesiredTopology(topology DesiredTopology) error {
	var problems []error
	if topology.Preset != PresetPreview && topology.Preset != PresetStandard && topology.Preset != PresetHighAvailability {
		problems = append(problems, fmt.Errorf("invalid topology preset %q", topology.Preset))
	}
	problems = append(problems, validateAvailability("topology", topology.Availability))

	workloads := make(map[WorkloadID]struct{}, len(topology.Workloads))
	for _, workload := range topology.Workloads {
		problems = append(problems,
			validateID("workload ID", string(workload.ID)),
			validateAvailability("workload "+string(workload.ID), workload.Availability),
		)
		if workload.Singleton && workload.HorizontalScaling {
			problems = append(problems, fmt.Errorf("workload %q cannot be singleton and horizontally scalable", workload.ID))
		}
		if _, exists := workloads[workload.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate workload ID %q", workload.ID))
		}
		workloads[workload.ID] = struct{}{}
	}

	capabilities := make(map[CapabilityID]struct{}, len(topology.Capabilities))
	for _, capability := range topology.Capabilities {
		problems = append(problems,
			validateID("capability ID", string(capability.ID)),
			validateAvailability("capability "+string(capability.ID), capability.Availability),
		)
		if !validCapabilityKind(capability.Kind) {
			problems = append(problems, fmt.Errorf("invalid capability kind %q", capability.Kind))
		}
		if capability.Purpose == "" {
			problems = append(problems, fmt.Errorf("capability %q requires a purpose", capability.ID))
		}
		if capability.ScaleToZeroAllowed && capability.Durable {
			problems = append(problems, fmt.Errorf("durable capability %q cannot scale to zero", capability.ID))
		}
		if capability.PointInTimeRecovery && (capability.Kind != CapabilityDatabase || !capability.Durable) {
			problems = append(problems, fmt.Errorf("capability %q requires a durable database for point-in-time recovery", capability.ID))
		}
		if _, exists := capabilities[capability.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate capability ID %q", capability.ID))
		}
		capabilities[capability.ID] = struct{}{}
	}
	if len(topology.Workloads) == 0 || len(topology.Capabilities) == 0 {
		problems = append(problems, errors.New("topology requires workloads and capabilities"))
	}
	return errors.Join(problems...)
}

func validateAvailability(name string, availability Availability) error {
	if availability.MinimumZones < 1 {
		return fmt.Errorf("%s minimum zones must be at least one", name)
	}
	return nil
}
