package stack

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	ovhprovider "github.com/magelift/magelift/internal/cloud/ovh/provider"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	ovhapi "github.com/ovh/go-ovh/ovh"
)

// RegionAdmission performs the OVH-specific read-only checks that cannot be
// proven from YAML alone. It is intentionally a provider implementation of
// the platform admission port, so the CLI never needs OVH SDK types.
type RegionAdmission struct {
	NewClient func(string) (ovhprovider.API, error)
}

var _ platform.PlanAdmission = RegionAdmission{}

func (a RegionAdmission) Admit(ctx context.Context, planned platform.PlannedStack) (platform.PlannedStack, error) {
	if ctx == nil {
		return nil, errors.New("OVH plan admission context is required")
	}
	value, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("OVH plan admission received unexpected planned type %T", planned)
	}
	clientFactory := a.NewClient
	if clientFactory == nil {
		clientFactory = func(endpoint string) (ovhprovider.API, error) {
			return ovhapi.NewEndpointClient(endpoint)
		}
	}
	client, err := clientFactory(value.Spec.Identity.APIEndpoint)
	if err != nil {
		return nil, errors.New("create authenticated OVH capability client")
	}
	capabilities, err := ovhprovider.NewCapabilityFromClient(client)
	if err != nil {
		return nil, err
	}
	project, err := capabilities.Project(ctx, value.Spec.Identity.ServiceName)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(project.ID), strings.TrimSpace(value.Spec.Identity.ServiceName)) {
		return nil, fmt.Errorf("OVH credentials resolved to project %q, but the plan targets %q", project.ID, value.Spec.Identity.ServiceName)
	}
	region, err := capabilities.Region(ctx, value.Spec.Identity.ServiceName, value.Spec.Identity.Region)
	if err != nil {
		return nil, err
	}
	if !ovhRegionIsOperational(region.Status) {
		return nil, fmt.Errorf("OVH region %q is not available for deployment (status %q)", value.Spec.Identity.Region, region.Status)
	}
	zones, err := resolveOVHZones(value.Spec, region.AvailabilityZones)
	if err != nil {
		return nil, err
	}
	if err := validateOVHMKSPlanRegion(value.Spec.Identity.Region, value.Spec.Catalog.MKSPlan); err != nil {
		return nil, err
	}
	availability, err := capabilities.DatabaseAvailability(ctx, value.Spec.Identity.ServiceName)
	if err != nil {
		return nil, err
	}
	if err := validateOVHManagedServiceAvailability(value.Spec, availability); err != nil {
		return nil, err
	}

	next := value
	next.Spec.Policy.Zones = zones
	if !next.Spec.Catalog.NodeCountExplicit && next.Spec.Catalog.NodeCount < len(zones) {
		next.Spec.Catalog.NodeCount = len(zones)
	}
	if err := next.Spec.Validate(); err != nil {
		return nil, fmt.Errorf("OVH plan after capability resolution is invalid: %w", err)
	}
	return next, nil
}

func ovhRegionIsOperational(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "UP", "ENABLED":
		return true
	default:
		return false
	}
}

func resolveOVHZones(spec Spec, available []string) ([]string, error) {
	canonical := canonicalOVHZones(available)
	if len(canonical) == 0 {
		return nil, fmt.Errorf("OVH region %q returned no availability zones", spec.Identity.Region)
	}
	if !spec.Policy.ZonesExplicit {
		if spec.Identity.Preset == sdk.PresetHighAvailability {
			return canonical, nil
		}
		return canonical[:1], nil
	}
	byName := make(map[string]string, len(canonical))
	for _, zone := range canonical {
		byName[strings.ToLower(zone)] = zone
	}
	resolved := make([]string, 0, len(spec.Policy.Zones))
	for _, requested := range spec.Policy.Zones {
		canonicalZone, found := byName[strings.ToLower(strings.TrimSpace(requested))]
		if !found {
			return nil, fmt.Errorf("OVH availability zone %q is not available in region %q", requested, spec.Identity.Region)
		}
		resolved = append(resolved, canonicalZone)
	}
	return resolved, nil
}

func canonicalOVHZones(available []string) []string {
	result := make([]string, 0, len(available))
	seen := make(map[string]struct{}, len(available))
	for _, zone := range available {
		zone = strings.TrimSpace(zone)
		key := strings.ToLower(zone)
		if zone == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, zone)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

// This catalog is sourced from OVHcloud's current Managed Kubernetes regional
// availability table, last checked 2026-08-11. Admission fails closed for a
// region not listed here instead of allowing a paid create to discover the
// incompatibility later.
var ovhMKSPlanRegions = map[string]map[string]struct{}{
	"free": {
		"GRA5": {}, "GRA7": {}, "GRA9": {}, "GRA11": {}, "SBG5": {}, "DE1": {}, "UK1": {}, "WAW1": {},
		"SGP1": {}, "SYD1": {}, "BHS5": {}, "US-WEST-OR-1": {}, "US-EAST-VA-1": {},
	},
	"standard": {
		"EU-WEST-RBX": {}, "EU-WEST-PAR": {}, "EU-SOUTH-MIL": {}, "AP-SOUTH-MUM": {},
	},
}

func validateOVHMKSPlanRegion(region, plan string) error {
	region = strings.ToUpper(strings.TrimSpace(region))
	plan = strings.ToLower(strings.TrimSpace(plan))
	regions, knownPlan := ovhMKSPlanRegions[plan]
	if !knownPlan {
		return fmt.Errorf("OVH MKS plan %q is not in the admitted plan catalog", plan)
	}
	if _, allowed := regions[region]; !allowed {
		return fmt.Errorf("OVH MKS plan %q is not available in region %q; choose a plan supported by the current provider catalog", plan, region)
	}
	return nil
}

type ovhManagedServiceSelection struct {
	Name      string
	Engine    string
	Version   string
	Plan      string
	Flavor    string
	Region    string
	NodeCount int
}

func validateOVHManagedServiceAvailability(spec Spec, availability []ovhprovider.DatabaseAvailability) error {
	selections := []ovhManagedServiceSelection{
		{
			Name:      "MySQL",
			Engine:    "mysql",
			Version:   spec.Catalog.DatabaseVersion,
			Plan:      spec.Catalog.DatabasePlan,
			Flavor:    spec.Catalog.DatabaseFlavor,
			Region:    spec.Identity.Region,
			NodeCount: spec.Catalog.DatabaseNodeCount,
		},
		{
			Name:      "Valkey",
			Engine:    "valkey",
			Version:   spec.Catalog.ValkeyVersion,
			Plan:      spec.Catalog.ValkeyPlan,
			Flavor:    spec.Catalog.ValkeyFlavor,
			Region:    spec.Identity.Region,
			NodeCount: spec.Catalog.ValkeyNodeCount,
		},
	}
	for _, selection := range selections {
		if !ovhManagedServiceAvailable(selection, availability) {
			return fmt.Errorf(
				"OVH %s selection is not available: engine=%q version=%q plan=%q flavor=%q region=%q network=%q nodes=%d",
				selection.Name,
				selection.Engine,
				selection.Version,
				selection.Plan,
				selection.Flavor,
				selection.Region,
				"private",
				selection.NodeCount,
			)
		}
	}
	return nil
}

func ovhManagedServiceAvailable(selection ovhManagedServiceSelection, availability []ovhprovider.DatabaseAvailability) bool {
	if selection.Engine == "" || selection.Version == "" || selection.Plan == "" || selection.Flavor == "" || selection.Region == "" || selection.NodeCount <= 0 {
		return false
	}
	for _, candidate := range availability {
		matchesIdentity := strings.EqualFold(candidate.Engine, selection.Engine) &&
			strings.EqualFold(candidate.Version, selection.Version) &&
			strings.EqualFold(candidate.Plan, selection.Plan) &&
			strings.EqualFold(candidate.Flavor, selection.Flavor) &&
			strings.EqualFold(candidate.Region, selection.Region)
		matchesNetwork := strings.EqualFold(candidate.Network, "private")
		matchesNodeCount := candidate.MinNodeNumber > 0 &&
			candidate.MaxNodeNumber >= candidate.MinNodeNumber &&
			selection.NodeCount >= candidate.MinNodeNumber &&
			selection.NodeCount <= candidate.MaxNodeNumber
		if matchesIdentity && matchesNetwork && matchesNodeCount {
			return true
		}
	}
	return false
}
