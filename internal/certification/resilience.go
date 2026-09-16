package certification

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// ResilienceDataClassCapability describes what a selected architecture must
// prove for one state class. Rebuildable search and cache state are deliberately
// represented separately from durable database and media backups.
type ResilienceDataClassCapability struct {
	Name              string           `json:"name" yaml:"name"`
	Status            CapabilityStatus `json:"status" yaml:"status"`
	Durable           bool             `json:"durable" yaml:"durable"`
	BackupSupported   bool             `json:"backupSupported" yaml:"backupSupported"`
	RestoreSupported  bool             `json:"restoreSupported" yaml:"restoreSupported"`
	IntegrityRequired bool             `json:"integrityRequired" yaml:"integrityRequired"`
	Strategy          string           `json:"strategy" yaml:"strategy"`
	Destinations      []string         `json:"destinations,omitempty" yaml:"destinations,omitempty"`
	Reason            string           `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// ResilienceCapabilityProfile is a conservative, source-dated planning
// profile. It is not live certification: a provider API can be available while
// the corresponding MageLift backup/restore adapter or evidence is still
// experimental.
type ResilienceCapabilityProfile struct {
	ID                  string                          `json:"id" yaml:"id"`
	Provider            string                          `json:"provider" yaml:"provider"`
	Runtime             string                          `json:"runtime" yaml:"runtime"`
	Status              CapabilityStatus                `json:"status" yaml:"status"`
	MaxRPOSeconds       int64                           `json:"maxRpoSeconds" yaml:"maxRpoSeconds"`
	MaxRTOSeconds       int64                           `json:"maxRtoSeconds" yaml:"maxRtoSeconds"`
	AllowedDestinations []sdk.RecoveryDestination       `json:"allowedDestinations" yaml:"allowedDestinations"`
	DataClasses         []ResilienceDataClassCapability `json:"dataClasses" yaml:"dataClasses"`
	SourceCapabilityIDs []string                        `json:"sourceCapabilityIds" yaml:"sourceCapabilityIds"`
	Reason              string                          `json:"reason" yaml:"reason"`
}

// CurrentResilienceProfiles exposes every first-party architecture family to
// the same policy validator. The values are deliberately conservative policy
// ceilings until a live profile records measured RPO/RTO; an adapter must
// tighten them when its selected service or region has a stricter limit.
func CurrentResilienceProfiles() []ResilienceCapabilityProfile {
	profiles := make([]ResilienceCapabilityProfile, 0, 5)
	for _, family := range CurrentArchitectureFamilies() {
		profiles = append(profiles, ResilienceCapabilityProfile{
			ID: family.ID, Provider: family.Provider, Runtime: family.Runtime,
			Status: family.Status, MaxRPOSeconds: 3600, MaxRTOSeconds: 86400,
			AllowedDestinations: allowedRecoveryDestinations(family.ID),
			DataClasses:         resilienceDataClassesForFamily(family), SourceCapabilityIDs: append([]string(nil), family.CapabilityIDs...), Reason: resilienceProfileReason(family.ID),
		})
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	return profiles
}

func resilienceProfileReason(familyID string) string {
	if familyID == "gcp.gke" {
		return "Policy is available for planning. Isolated same-region restore is executable. Writer fencing, traffic failover, controlled failback, and regional DR are typed unsupported until a native translator opts in; in-place and alternate-region plans fail closed. Physical zone outage is not injectable; zone-loss evidence is backing-VM simulation. Failed-deployment has no injector. Partial-restore has no multi-class injector. Magento known-content HA integrity is unproven."
	}
	return "Policy is available for planning; live backup, restore, HA, DR, fencing, and measured RPO/RTO evidence remain profile-specific."
}

func allowedRecoveryDestinations(familyID string) []sdk.RecoveryDestination {
	if familyID == "gcp.gke" {
		return []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated}
	}
	return []sdk.RecoveryDestination{sdk.RecoverySameRegion, sdk.RecoverySameRegionIsolated, sdk.RecoveryAlternateRegion}
}

func defaultResilienceDataClasses() []ResilienceDataClassCapability {
	return []ResilienceDataClassCapability{
		{Name: "database", Status: CapabilitySupported, Durable: true, BackupSupported: true, RestoreSupported: true, IntegrityRequired: true, Strategy: "durable-backup-and-restore", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}},
		{Name: "media", Status: CapabilitySupported, Durable: true, BackupSupported: true, RestoreSupported: true, IntegrityRequired: true, Strategy: "versioned-object-backup-and-manifest-check", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}},
		{Name: "configuration-secrets", Status: CapabilitySupported, Durable: true, BackupSupported: true, RestoreSupported: true, IntegrityRequired: true, Strategy: "versioned-secret-and-config-export", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}},
		{Name: "infrastructure-state", Status: CapabilitySupported, Durable: true, BackupSupported: true, RestoreSupported: true, IntegrityRequired: true, Strategy: "immutable-state-backup", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}},
		{Name: "audit-evidence", Status: CapabilitySupported, Durable: true, BackupSupported: true, RestoreSupported: true, IntegrityRequired: true, Strategy: "append-only-evidence-export", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}},
		{Name: "queue", Status: CapabilityExperimental, Durable: false, BackupSupported: true, RestoreSupported: true, IntegrityRequired: true, Strategy: "provider-or-fixture-specific-message-recovery", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}, Reason: "Message durability, replay, and loss semantics require a provider-specific adapter and independent evidence."},
		{Name: "search-index", Status: CapabilityExperimental, Durable: false, BackupSupported: false, RestoreSupported: true, IntegrityRequired: true, Strategy: "rebuild-from-durable-source", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}, Reason: "Search is a reconstructible projection; rebuild duration and application-query evidence remain independent from durable backups."},
		{Name: "cache", Status: CapabilitySupported, Durable: false, BackupSupported: false, RestoreSupported: true, IntegrityRequired: false, Strategy: "reconstruct-from-source-of-truth", Destinations: []string{"same-region", "same-region-isolated", "alternate-region"}},
	}
}

func resilienceDataClassesForFamily(family ArchitectureFamily) []ResilienceDataClassCapability {
	classes := defaultResilienceDataClasses()
	for index := range classes {
		switch {
		case (family.ID == "aws.ecs" || family.ID == "aws.eks") && classes[index].Name == "queue":
			// The first-party AWS queue translator is the bounded SQS
			// export/replay path. It deliberately cannot claim in-place or
			// alternate-region recovery without a different adapter.
			classes[index].Destinations = []string{"same-region-isolated"}
		case family.ID == "gcp.gke" && classes[index].Name == "queue":
			// Pub/Sub snapshot recovery seeks the source subscription in
			// place or an owned isolated subscription on the same topic.
			// Alternate-region destinations still need a separate topology.
			classes[index].Destinations = []string{"same-region", "same-region-isolated"}
		case family.ID == "gcp.gke":
			// Regional DR stays a typed gap until writer fencing and
			// traffic failover have a native translator. Isolated and
			// same-region restore remain the honest executable ceiling.
			classes[index].Destinations = withoutRecoveryDestination(classes[index].Destinations, "alternate-region")
		case (family.ID == "scaleway.kapsule" || family.ID == "ovh.mks") && (classes[index].Name == "queue" || classes[index].Name == "search-index"):
			classes[index].Status = CapabilityUnavailable
			classes[index].BackupSupported = false
			classes[index].RestoreSupported = false
			classes[index].Strategy = "unsupported-provider-boundary"
			classes[index].Reason = "The source-dated catalog has no native product or MageLift adapter for this data class on the selected provider family."
		}
	}
	return classes
}

func FindResilienceProfile(id string) (ResilienceCapabilityProfile, bool) {
	for _, profile := range CurrentResilienceProfiles() {
		if profile.ID == id {
			return profile, true
		}
	}
	return ResilienceCapabilityProfile{}, false
}

// ValidateIntent checks whether a declared target is within the selected
// architecture policy. It does not turn an experimental profile into a
// certification result; release gates still require independent proof.
func (p ResilienceCapabilityProfile) ValidateIntent(intent sdk.ResilienceIntent) error {
	var problems []error
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Provider) == "" || strings.TrimSpace(p.Runtime) == "" {
		problems = append(problems, errors.New("resilience capability profile identity is required"))
	}
	if p.Status == CapabilityUnavailable || p.Status == CapabilityUnsupported || p.Status == CapabilityBlocked {
		problems = append(problems, fmt.Errorf("resilience profile %q is %s: %s", p.ID, p.Status, p.Reason))
	}
	if p.MaxRPOSeconds <= 0 || p.MaxRTOSeconds <= 0 {
		problems = append(problems, fmt.Errorf("resilience profile %q has invalid policy ceilings", p.ID))
	}
	if intent.RPOSeconds > p.MaxRPOSeconds {
		problems = append(problems, fmt.Errorf("resilience RPO %d seconds exceeds profile ceiling %d seconds", intent.RPOSeconds, p.MaxRPOSeconds))
	}
	if intent.RTOSeconds > p.MaxRTOSeconds {
		problems = append(problems, fmt.Errorf("resilience RTO %d seconds exceeds profile ceiling %d seconds", intent.RTOSeconds, p.MaxRTOSeconds))
	}
	allowedDestinations := make(map[sdk.RecoveryDestination]struct{}, len(p.AllowedDestinations))
	for _, destination := range p.AllowedDestinations {
		allowedDestinations[destination] = struct{}{}
	}
	for _, destination := range intent.RecoveryDestinations {
		if _, ok := allowedDestinations[destination]; !ok {
			problems = append(problems, fmt.Errorf("resilience destination %q is not allowed by profile %q", destination, p.ID))
		}
	}
	capabilities := make(map[string]ResilienceDataClassCapability, len(p.DataClasses))
	for _, dataClass := range p.DataClasses {
		if !validCapabilityStatus(dataClass.Status) {
			problems = append(problems, fmt.Errorf("resilience data class %q has invalid status %q", dataClass.Name, dataClass.Status))
		}
		if dataClass.Status != CapabilitySupported && dataClass.Status != CapabilityCertified && strings.TrimSpace(dataClass.Reason) == "" {
			problems = append(problems, fmt.Errorf("resilience data class %q requires a reason for status %q", dataClass.Name, dataClass.Status))
		}
		if len(dataClass.Destinations) == 0 {
			problems = append(problems, fmt.Errorf("resilience data class %q requires at least one recovery destination", dataClass.Name))
		}
		capabilities[dataClass.Name] = dataClass
	}
	for _, dataClass := range intent.DataClasses {
		capability, ok := capabilities[dataClass.Name]
		if !ok {
			problems = append(problems, fmt.Errorf("resilience data class %q is not supported by profile %q", dataClass.Name, p.ID))
			continue
		}
		if capability.Strategy == "" {
			problems = append(problems, fmt.Errorf("resilience data class %q has no recovery strategy", dataClass.Name))
		}
		if capability.Status == CapabilityUnavailable || capability.Status == CapabilityUnsupported || capability.Status == CapabilityBlocked {
			problems = append(problems, fmt.Errorf("resilience data class %q is %s: %s", dataClass.Name, capability.Status, capability.Reason))
		}
		if capability.Durable && !capability.BackupSupported {
			problems = append(problems, fmt.Errorf("durable data class %q has no backup strategy", dataClass.Name))
		}
		if !capability.RestoreSupported {
			problems = append(problems, fmt.Errorf("resilience data class %q has no restore strategy", dataClass.Name))
		}
		if capability.IntegrityRequired && strings.TrimSpace(dataClass.IntegrityMethod) == "" {
			problems = append(problems, fmt.Errorf("resilience data class %q requires an integrity method", dataClass.Name))
		}
		for _, destination := range intent.RecoveryDestinations {
			if !containsResilienceDestination(capability.Destinations, destination) {
				problems = append(problems, fmt.Errorf("resilience data class %q does not support recovery destination %q in profile %q", dataClass.Name, destination, p.ID))
			}
		}
	}
	return errors.Join(problems...)
}

func containsResilienceDestination(destinations []string, candidate sdk.RecoveryDestination) bool {
	for _, destination := range destinations {
		if destination == string(candidate) {
			return true
		}
	}
	return false
}

func withoutRecoveryDestination(destinations []string, remove string) []string {
	filtered := make([]string, 0, len(destinations))
	for _, destination := range destinations {
		if destination == remove {
			continue
		}
		filtered = append(filtered, destination)
	}
	return filtered
}

// ValidateResilienceArchitecture joins the generic SDK checks with the
// provider-family policy before a module is allowed to mutate infrastructure.
func ValidateResilienceArchitecture(intent sdk.ArchitectureIntent) error {
	if err := intent.Validate(); err != nil {
		return err
	}
	profile, found := FindResilienceProfile(string(intent.Provider) + "." + string(intent.Runtime))
	if !found {
		for _, candidate := range CurrentResilienceProfiles() {
			if candidate.Provider == string(intent.Provider) && candidate.Runtime == string(intent.Runtime) {
				profile = candidate
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("no resilience policy profile for provider %q runtime %q", intent.Provider, intent.Runtime)
	}
	return profile.ValidateIntent(intent.Resilience)
}
