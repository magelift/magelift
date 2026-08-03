package v1

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var stableID = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[.-][a-z0-9]+)*$`)

func ValidateTargetDescriptor(descriptor TargetDescriptor) error {
	return errors.Join(
		validateID("target ID", string(descriptor.ID)),
		validateID("provider ID", string(descriptor.Provider)),
		validateID("runtime ID", string(descriptor.Runtime)),
	)
}

func ValidateCapabilityDescriptors(descriptors []CapabilityDescriptor) error {
	seen := make(map[CapabilityProviderID]struct{}, len(descriptors))
	var problems []error
	for _, descriptor := range descriptors {
		problems = append(problems,
			validateID("capability provider ID", string(descriptor.ID)),
			validateID("capability ID", string(descriptor.Capability)),
			validateID("provider ID", string(descriptor.Provider)),
		)
		if !validCapabilityKind(descriptor.Kind) {
			problems = append(problems, fmt.Errorf("invalid capability kind %q", descriptor.Kind))
		}
		if _, exists := seen[descriptor.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate capability provider ID %q", descriptor.ID))
		}
		seen[descriptor.ID] = struct{}{}
	}
	return errors.Join(problems...)
}

// ValidateCapabilityRequirements checks an artifact's requirements against a
// target's advertised capabilities without contacting the target provider.
func ValidateCapabilityRequirements(required, available []CapabilityID) error {
	provided := make(map[CapabilityID]struct{}, len(available))
	var problems []error
	for _, capability := range available {
		problems = append(problems, validateID("available capability ID", string(capability)))
		if _, exists := provided[capability]; exists {
			problems = append(problems, fmt.Errorf("duplicate available capability %q", capability))
		}
		provided[capability] = struct{}{}
	}
	seen := make(map[CapabilityID]struct{}, len(required))
	unsupported := make([]string, 0)
	for _, capability := range required {
		problems = append(problems, validateID("required capability ID", string(capability)))
		if _, exists := seen[capability]; exists {
			problems = append(problems, fmt.Errorf("duplicate required capability %q", capability))
		}
		seen[capability] = struct{}{}
		if _, exists := provided[capability]; !exists {
			unsupported = append(unsupported, string(capability))
		}
	}
	if len(unsupported) > 0 {
		sort.Strings(unsupported)
		problems = append(problems, fmt.Errorf("target does not provide required capabilities: %s", strings.Join(unsupported, ", ")))
	}
	return errors.Join(problems...)
}

func ValidateTransformDescriptor(descriptor TransformDescriptor) error {
	var problems []error
	problems = append(problems,
		validateID("transform ID", string(descriptor.ID)),
		validateID("provider ID", string(descriptor.Provider)),
		validateID("runtime ID", string(descriptor.Runtime)),
	)
	seen := make(map[ComponentID]struct{}, len(descriptor.Components))
	for _, component := range descriptor.Components {
		problems = append(problems, validateID("component ID", string(component)))
		if _, exists := seen[component]; exists {
			problems = append(problems, fmt.Errorf("duplicate component ID %q", component))
		}
		seen[component] = struct{}{}
	}
	if len(descriptor.Components) == 0 {
		problems = append(problems, errors.New("transform must select at least one component"))
	}
	return errors.Join(problems...)
}

func ValidateExistingResourceRef(ref ExistingResourceRef) error {
	var problems []error
	problems = append(problems,
		validateID("resource ID", string(ref.ID)),
		validateID("provider ID", string(ref.Provider)),
	)
	if !validExistingResourceKind(ref.Kind) {
		problems = append(problems, fmt.Errorf("invalid existing resource kind %q", ref.Kind))
	}
	if strings.TrimSpace(ref.ExternalID) == "" {
		problems = append(problems, errors.New("existing resource external ID is required"))
	}
	return errors.Join(problems...)
}

// ValidateLifecycleHooks checks descriptor shape and dependency edges. Core
// lifecycle step IDs may be referenced without appearing in this hook set.
func ValidateLifecycleHooks(descriptors []LifecycleHookDescriptor) error {
	known := make(map[HookID]LifecycleHookDescriptor, len(descriptors))
	var problems []error
	for _, descriptor := range descriptors {
		problems = append(problems, validateHookDescriptor(descriptor))
		if _, exists := known[descriptor.ID]; exists {
			problems = append(problems, fmt.Errorf("duplicate lifecycle hook ID %q", descriptor.ID))
		}
		known[descriptor.ID] = descriptor
	}

	for _, descriptor := range descriptors {
		for _, dependency := range descriptor.DependsOn {
			if dependency == descriptor.ID {
				problems = append(problems, fmt.Errorf("lifecycle hook %q cannot depend on itself", descriptor.ID))
			}
		}
	}
	problems = append(problems, lifecycleCycles(known))
	return errors.Join(problems...)
}

func validateHookDescriptor(descriptor LifecycleHookDescriptor) error {
	var problems []error
	problems = append(problems,
		validateID("lifecycle hook ID", string(descriptor.ID)),
		validateID("relative lifecycle step ID", string(descriptor.RelativeTo)),
	)
	if !validLifecyclePhase(descriptor.Phase) {
		problems = append(problems, fmt.Errorf("invalid lifecycle phase %q", descriptor.Phase))
	}
	if !validHookRelationship(descriptor.Relationship) {
		problems = append(problems, fmt.Errorf("invalid hook relationship %q", descriptor.Relationship))
	}
	if descriptor.Timeout < 1 {
		problems = append(problems, errors.New("lifecycle hook timeout must be at least one second"))
	}
	if descriptor.MaxAttempts < 1 {
		problems = append(problems, errors.New("lifecycle hook attempts must be at least one"))
	} else if descriptor.MaxAttempts > 1 && !descriptor.Idempotent {
		problems = append(problems, errors.New("only idempotent lifecycle hooks may be retried"))
	}
	seen := make(map[HookID]struct{}, len(descriptor.DependsOn))
	for _, dependency := range descriptor.DependsOn {
		problems = append(problems, validateID("lifecycle dependency ID", string(dependency)))
		if _, exists := seen[dependency]; exists {
			problems = append(problems, fmt.Errorf("lifecycle hook %q contains duplicate dependency %q", descriptor.ID, dependency))
		}
		seen[dependency] = struct{}{}
	}
	return errors.Join(problems...)
}

func lifecycleCycles(known map[HookID]LifecycleHookDescriptor) error {
	visiting := make(map[HookID]bool, len(known))
	visited := make(map[HookID]bool, len(known))
	var visit func(HookID) error
	visit = func(id HookID) error {
		if visiting[id] {
			return fmt.Errorf("lifecycle hook dependency cycle at %q", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		for _, dependency := range known[id].DependsOn {
			if _, isHook := known[dependency]; isHook {
				if err := visit(dependency); err != nil {
					return err
				}
			}
		}
		delete(visiting, id)
		visited[id] = true
		return nil
	}
	for id := range known {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}

func validateID(name, value string) error {
	if !stableID.MatchString(value) {
		return fmt.Errorf("%s %q is not a stable ID", name, value)
	}
	return nil
}

func validCapabilityKind(kind CapabilityKind) bool {
	switch kind {
	case CapabilityDatabase, CapabilityCache, CapabilitySearch, CapabilityQueue,
		CapabilityObjectStorage, CapabilityEdge, CapabilityObservability:
		return true
	default:
		return false
	}
}

func validExistingResourceKind(kind ExistingResourceKind) bool {
	switch kind {
	case ExistingNetwork, ExistingDatabase, ExistingCache, ExistingSearch,
		ExistingQueue, ExistingObjectStore, ExistingDNSZone, ExistingCertificate:
		return true
	default:
		return false
	}
}

func validLifecyclePhase(phase LifecyclePhase) bool {
	switch phase {
	case PhaseValidate, PhaseBuild, PhasePackage, PhaseDeploy, PhasePostDeploy:
		return true
	default:
		return false
	}
}

func validHookRelationship(relationship HookRelationship) bool {
	switch relationship {
	case HookBefore, HookAfter, HookReplace, HookDisable:
		return true
	default:
		return false
	}
}
