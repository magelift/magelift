package certification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

const ArchitectureProfileVersion = "v1"

// ArchitectureProfile wraps the public, provider-neutral intent with the
// evidence identity needed by certification. Provider adapters may retain
// opaque implementation state separately; it is never part of the core
// semantic contract unless it changes the declared boundary.
type ArchitectureProfile struct {
	Version           string                 `json:"version" yaml:"version"`
	Intent            sdk.ArchitectureIntent `json:"intent" yaml:"intent"`
	CapabilityCatalog string                 `json:"capabilityCatalog" yaml:"capabilityCatalog"`
	DeclaredStatus    CapabilityStatus       `json:"declaredStatus" yaml:"declaredStatus"`
}

func (p ArchitectureProfile) Validate(catalog CapabilityCatalog) error {
	if p.Version != ArchitectureProfileVersion {
		return fmt.Errorf("unsupported architecture profile version %q", p.Version)
	}
	if err := p.Intent.Validate(); err != nil {
		return fmt.Errorf("validate architecture intent: %w", err)
	}
	if strings.TrimSpace(p.CapabilityCatalog) == "" {
		return errors.New("architecture profile capability catalog version is required")
	}
	if p.CapabilityCatalog != catalog.Version {
		return fmt.Errorf("architecture profile uses capability catalog %q, current catalog is %q", p.CapabilityCatalog, catalog.Version)
	}
	if p.DeclaredStatus == "" || !validCapabilityStatus(p.DeclaredStatus) {
		return fmt.Errorf("invalid architecture profile status %q", p.DeclaredStatus)
	}
	if err := catalog.ValidateAtProfile(p.Intent); err != nil {
		return err
	}
	if err := ValidateResilienceArchitecture(p.Intent); err != nil {
		return fmt.Errorf("validate architecture resilience policy: %w", err)
	}
	return nil
}

// ValidateAtProfile validates only the capabilities explicitly named by a
// profile. It prevents a profile from silently replacing an unavailable
// provider service with a wire-compatible service from another provider.
func (c CapabilityCatalog) ValidateAtProfile(intent sdk.ArchitectureIntent) error {
	for _, boundary := range intent.Boundaries {
		if strings.TrimSpace(boundary.CapabilityID) == "" {
			continue
		}
		record, ok := c.FindID(boundary.CapabilityID)
		if !ok {
			return fmt.Errorf("capability %q is not in the current catalog", boundary.CapabilityID)
		}
		if !profileCapabilityProviderAllowed(record, boundary, intent.Provider) {
			return fmt.Errorf("capability %q belongs to provider %q, profile provider is %q", boundary.CapabilityID, record.Provider, intent.Provider)
		}
		if record.Role != boundary.Role && !(boundary.Role == "edge" && strings.HasPrefix(record.Role, "edge")) {
			return fmt.Errorf("capability %q has role %q, profile boundary requires %q", boundary.CapabilityID, record.Role, boundary.Role)
		}
		if !capabilityMajorMatches(boundary.Major, record.ServiceMajor) {
			return fmt.Errorf("capability %q has service major %q, profile requires %q", boundary.CapabilityID, record.ServiceMajor, boundary.Major)
		}
		if record.MageLiftStatus == CapabilityUnsupported || record.MageLiftStatus == CapabilityUnavailable || record.MageLiftStatus == CapabilityBlocked {
			return fmt.Errorf("capability %q is %s: %s", boundary.CapabilityID, record.MageLiftStatus, record.Reason)
		}
	}
	return nil
}

func profileCapabilityProviderAllowed(record CapabilityRecord, boundary sdk.ServiceBoundaryIntent, provider sdk.ProviderID) bool {
	if record.Provider == string(provider) {
		return true
	}
	// External capabilities are deliberately open to independently maintained
	// provider implementations. The catalog record and external ownership
	// boundary are the compatibility gate; the core must not hardcode today's
	// Fastly/New Relic implementations into the extension contract.
	return boundary.Ownership == sdk.ServiceExternal
}

func capabilityMajorMatches(required, actual string) bool {
	required = strings.TrimSpace(required)
	actual = strings.TrimSpace(actual)
	if required == "" || actual == "" || required == "current" || actual == "current" {
		return true
	}
	return strings.Contains(actual, required) || strings.Contains(required, actual)
}

func (c CapabilityCatalog) FindID(id string) (CapabilityRecord, bool) {
	for _, record := range c.Records {
		if record.ID == id {
			return record, true
		}
	}
	return CapabilityRecord{}, false
}

// Fingerprint returns a deterministic SHA-256 identity for semantic profile
// inputs. It sorts all unordered collections before encoding and therefore
// remains stable across YAML/map ordering and provider adapter internals.
func (p ArchitectureProfile) Fingerprint() (string, error) {
	if err := p.Intent.Validate(); err != nil {
		return "", err
	}
	intent := normalizeArchitectureIntent(p.Intent)
	data, err := json.Marshal(struct {
		Version           string                 `json:"version"`
		CapabilityCatalog string                 `json:"capabilityCatalog"`
		Intent            sdk.ArchitectureIntent `json:"intent"`
	}{p.Version, p.CapabilityCatalog, intent})
	if err != nil {
		return "", fmt.Errorf("encode architecture fingerprint: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func normalizeArchitectureIntent(intent sdk.ArchitectureIntent) sdk.ArchitectureIntent {
	intent.Regions = sdk.SortedStrings(intent.Regions)
	intent.Zones = sdk.SortedStrings(intent.Zones)
	intent.Boundaries = append([]sdk.ServiceBoundaryIntent(nil), intent.Boundaries...)
	sort.Slice(intent.Boundaries, func(i, j int) bool { return intent.Boundaries[i].Role < intent.Boundaries[j].Role })
	for i := range intent.Boundaries {
		intent.Boundaries[i].FailureDomains = sdk.SortedStrings(intent.Boundaries[i].FailureDomains)
	}
	intent.Resilience.RecoveryDestinations = append([]sdk.RecoveryDestination(nil), intent.Resilience.RecoveryDestinations...)
	sort.Slice(intent.Resilience.RecoveryDestinations, func(i, j int) bool {
		return intent.Resilience.RecoveryDestinations[i] < intent.Resilience.RecoveryDestinations[j]
	})
	intent.Resilience.DataClasses = append([]sdk.DataClassIntent(nil), intent.Resilience.DataClasses...)
	sort.Slice(intent.Resilience.DataClasses, func(i, j int) bool {
		return intent.Resilience.DataClasses[i].Name < intent.Resilience.DataClasses[j].Name
	})
	for i := range intent.Resilience.DataClasses {
		intent.Resilience.DataClasses[i].FailureDomains = sdk.SortedStrings(intent.Resilience.DataClasses[i].FailureDomains)
	}
	intent.Edge.Domains = sdk.SortedStrings(intent.Edge.Domains)
	intent.Observability.Signals = sdk.SortedStrings(intent.Observability.Signals)
	intent.Observability.AlertRefs = sdk.SortedStrings(intent.Observability.AlertRefs)
	intent.Observability.Alerts = append([]sdk.AlertIntent(nil), intent.Observability.Alerts...)
	sort.Slice(intent.Observability.Alerts, func(i, j int) bool { return intent.Observability.Alerts[i].ID < intent.Observability.Alerts[j].ID })
	intent.Observability.Dashboards = append([]sdk.DashboardIntent(nil), intent.Observability.Dashboards...)
	sort.Slice(intent.Observability.Dashboards, func(i, j int) bool {
		return intent.Observability.Dashboards[i].ID < intent.Observability.Dashboards[j].ID
	})
	for i := range intent.Observability.Dashboards {
		intent.Observability.Dashboards[i].Signals = sdk.SortedStrings(intent.Observability.Dashboards[i].Signals)
	}
	intent.Observability.SLOs = append([]sdk.SLOIntent(nil), intent.Observability.SLOs...)
	sort.Slice(intent.Observability.SLOs, func(i, j int) bool { return intent.Observability.SLOs[i].ID < intent.Observability.SLOs[j].ID })
	return intent
}
