package stack

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

var (
	stableName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)
	digest     = regexp.MustCompile(`^[^@\s]+@sha256:[a-f0-9]{64}$`)
	// Accept both legacy single-datacenter IDs (GRA9) and current regional
	// slugs (EU-WEST-PAR). The provider remains the authority for availability.
	ovhRegion = regexp.MustCompile(`^[A-Z][A-Z0-9-]{1,30}$`)
)

const (
	ovhDefaultAPIEndpoint = "ovh-eu"
	// OVH's current Public Cloud Database guidance uses the Gen 3 b3-8 flavor
	// for current regional examples. Older db1/db2 flavors can exist in the
	// global reference catalog while still being unavailable in a region.
	ovhDefaultDatabaseFlavor = "b3-8"
	ovhDefaultValkeyVersion  = "8.1"
)

type Spec struct {
	Identity      Identity
	Application   Application
	Artifact      Artifact
	Lifecycle     Lifecycle
	Policy        NetworkPolicy
	Catalog       CatalogSelection
	Dependencies  Dependencies
	Edge          sdk.EdgeIntent
	Observability sdk.ObservabilityIntent
	// AllowExpiredPreview records that CLI planning already accepted an
	// expired preview for teardown. The Pulumi program reads it to skip
	// only the expiry check; deploy planning never sets it.
	AllowExpiredPreview bool
}

type Identity struct {
	Project          string
	ServiceName      string
	APIEndpoint      string
	Environment      string
	Region           string
	EnvironmentClass string
	Preset           sdk.PresetID
	Labels           map[string]string
}

type Application struct {
	Edition    string
	Version    string
	Mode       string
	WebRuntime string
	Magento    platform.MagentoOverlays
}

type Artifact struct {
	ImageDigest string
}

type Lifecycle struct {
	ExpiresAt  time.Time
	Protection bool
}

type NetworkPolicy struct {
	NetworkCIDR   string
	Zones         []string
	ZonesExplicit bool
}

type CatalogSelection struct {
	DatabaseFlavor                 string
	DatabasePlan                   string
	DatabaseVersion                string
	DatabaseNodeCount              int
	DatabaseBackupTime             string
	DatabaseBackupRegions          []string
	DatabaseDeletionProtection     *bool
	ValkeyFlavor                   string
	ValkeyPlan                     string
	ValkeyVersion                  string
	ValkeyNodeCount                int
	ValkeyBackupTime               string
	ValkeyBackupRegions            []string
	ValkeyDeletionProtection       *bool
	MKSPlan                        string
	AttachFloatingIPs              bool
	PrivateNetworkRoutingAsDefault bool
	NodeFlavor                     string
	NodeCount                      int
	NodeCountExplicit              bool
	CPURequest                     string
	MemoryRequest                  string
	DesiredWebReplicas             int
	QueueConsumerCount             int
}

type Dependencies struct {
	DatabaseName        string
	MasterUsername      string
	EncryptionKeySecret string
	StateBucket         string
	StateEndpoint       string
	StateRegion         string
}

func (s Spec) Validate() error {
	return s.validate(false)
}

// ValidateAllowExpiredPreview is restricted to teardown planning. It keeps
// every structural and security check while allowing an already expired
// preview to be converted into a destroy request.
func (s Spec) ValidateAllowExpiredPreview() error {
	return s.validate(true)
}

func (s Spec) validate(allowExpiredPreview bool) error {
	var problems []error
	if !stableName.MatchString(s.Identity.Project) {
		problems = append(problems, errors.New("project name must be a stable lowercase identifier"))
	}
	if strings.TrimSpace(s.Identity.ServiceName) == "" {
		problems = append(problems, errors.New("OVH cloud project service name is required"))
	}
	if s.Identity.APIEndpoint != "" && !validOVHAPIEndpoint(s.Identity.APIEndpoint) {
		problems = append(problems, fmt.Errorf("invalid OVH API endpoint %q; use ovh-eu, ovh-ca, or ovh-us", s.Identity.APIEndpoint))
	}
	if !stableName.MatchString(s.Identity.Environment) {
		problems = append(problems, errors.New("environment name must be a stable lowercase identifier"))
	}
	if !ovhRegion.MatchString(s.Identity.Region) {
		problems = append(problems, fmt.Errorf("invalid OVH region %q", s.Identity.Region))
	}
	if s.Identity.EnvironmentClass == "" {
		problems = append(problems, errors.New("environment class is required"))
	}
	if s.Identity.Preset != sdk.PresetPreview && s.Identity.Preset != sdk.PresetStandard && s.Identity.Preset != sdk.PresetHighAvailability {
		problems = append(problems, fmt.Errorf("invalid preset %q", s.Identity.Preset))
	}
	if s.Catalog.MKSPlan != "free" && s.Catalog.MKSPlan != "standard" {
		problems = append(problems, fmt.Errorf("invalid OVH MKS plan %q", s.Catalog.MKSPlan))
	}
	if s.Catalog.MKSPlan == "free" && len(s.Policy.Zones) > 1 {
		problems = append(problems, errors.New("OVH MKS free plan supports one availability zone; use the standard plan for multi-zone workers"))
	}
	problems = append(problems, validateAvailabilityZones(s.Policy.Zones)...)
	if len(s.Policy.Zones) > 1 && s.Catalog.NodeCount < len(s.Policy.Zones) {
		problems = append(problems, fmt.Errorf("OVH MKS node count %d is too small for %d availability zones; configure at least one node per zone", s.Catalog.NodeCount, len(s.Policy.Zones)))
	}
	if s.Catalog.DatabaseNodeCount < 0 || s.Catalog.ValkeyNodeCount < 0 {
		problems = append(problems, errors.New("OVH managed database node counts cannot be negative"))
	}
	problems = append(problems, validateManagedServiceCatalog(s.Catalog)...)
	if !digest.MatchString(s.Artifact.ImageDigest) {
		problems = append(problems, errors.New("artifact image digest must be repository@sha256:..."))
	}
	if _, err := netip.ParsePrefix(s.Policy.NetworkCIDR); err != nil {
		problems = append(problems, fmt.Errorf("network CIDR: %w", err))
	}
	if err := sdk.ValidateEdgeIntent(s.Edge); err != nil {
		problems = append(problems, fmt.Errorf("edge intent: %w", err))
	}
	if native := strings.TrimSpace(s.Edge.NativeProvider); native != "" && native != "none" {
		switch native {
		case "ovh-public-cloud-load-balancer":
			if s.Edge.TLS {
				problems = append(problems, errors.New("OVH MKS public-cloud Load Balancer is L4-only; terminate TLS on Fastly or another external edge"))
			}
			if s.Edge.CachePolicyRef != "" || s.Edge.PurgeOnDeploy || s.Edge.PurgePolicyRef != "" {
				problems = append(problems, errors.New("OVH MKS public-cloud Load Balancer has no native cache or purge lifecycle"))
			}
			if s.Edge.WAFPolicyRef != "" {
				problems = append(problems, errors.New("OVH MKS public-cloud Load Balancer has no native WAF lifecycle"))
			}
		case "ovh-cdn":
			problems = append(problems, errors.New("OVH CDN is not an MKS public-cloud edge adapter; use Fastly or another external edge"))
		default:
			problems = append(problems, fmt.Errorf("OVH native edge provider %q has no MageLift edge adapter", native))
		}
	}
	if len(s.Policy.Zones) == 0 {
		problems = append(problems, errors.New("at least one zone is required"))
	}
	if s.Dependencies.DatabaseName == "" || s.Dependencies.MasterUsername == "" {
		problems = append(problems, errors.New("database name and master username are required"))
	}
	if strings.TrimSpace(s.Dependencies.EncryptionKeySecret) == "" {
		problems = append(problems, errors.New("Kubernetes Secret name for Magento encryption key is required"))
	}
	if err := sdk.ValidateObservabilityIntent(s.Observability); err != nil {
		problems = append(problems, fmt.Errorf("observability intent: %w", err))
	}
	if s.Catalog.DesiredWebReplicas < 1 {
		problems = append(problems, errors.New("desired web replicas must be at least 1"))
	}
	if !allowExpiredPreview && !s.Lifecycle.ExpiresAt.IsZero() && time.Now().After(s.Lifecycle.ExpiresAt) {
		problems = append(problems, errors.New("preview environment has expired"))
	}
	return errors.Join(problems...)
}

func validOVHAPIEndpoint(endpoint string) bool {
	switch endpoint {
	case "ovh-eu", "ovh-ca", "ovh-us":
		return true
	default:
		return false
	}
}

func validateAvailabilityZones(zones []string) []error {
	var problems []error
	seen := make(map[string]struct{}, len(zones))
	for index, zone := range zones {
		trimmed := strings.TrimSpace(zone)
		if trimmed == "" {
			problems = append(problems, fmt.Errorf("OVH MKS availability zone %d is empty", index))
			continue
		}
		if trimmed != zone {
			problems = append(problems, fmt.Errorf("OVH MKS availability zone %d %q must not have surrounding whitespace", index, zone))
		}
		key := strings.ToLower(trimmed)
		if _, exists := seen[key]; exists {
			problems = append(problems, fmt.Errorf("OVH MKS availability zone %q is duplicated", trimmed))
			continue
		}
		seen[key] = struct{}{}
	}
	return problems
}

func validateManagedServiceCatalog(catalog CatalogSelection) []error {
	var problems []error
	if catalog.DatabaseVersion != "" && catalog.DatabaseVersion != "8.0" && catalog.DatabaseVersion != "8.4" {
		problems = append(problems, fmt.Errorf("unsupported OVH MySQL version %q; current documented versions are 8.0 and 8.4", catalog.DatabaseVersion))
	}
	if catalog.ValkeyVersion != "" && catalog.ValkeyVersion != "7.2" && catalog.ValkeyVersion != "8.0" && catalog.ValkeyVersion != "8.1" && catalog.ValkeyVersion != "9.0" && catalog.ValkeyVersion != "9.1" {
		problems = append(problems, fmt.Errorf("unsupported OVH Valkey version %q; current documented or catalog-admitted versions are 7.2, 8.0, 8.1, 9.0, and 9.1", catalog.ValkeyVersion))
	}
	if catalog.DatabasePlan != "" {
		minNodes, maxNodes, ok := ovhMySQLPlanBounds(catalog.DatabasePlan)
		if !ok {
			problems = append(problems, fmt.Errorf("unsupported OVH MySQL plan %q; use discovery, essential, business, production, enterprise, or advanced", catalog.DatabasePlan))
		} else if catalog.DatabaseNodeCount > 0 && catalog.DatabaseNodeCount < minNodes {
			problems = append(problems, fmt.Errorf("OVH MySQL plan %q requires at least %d node(s), got %d", catalog.DatabasePlan, minNodes, catalog.DatabaseNodeCount))
		} else if catalog.DatabaseNodeCount > maxNodes {
			problems = append(problems, fmt.Errorf("OVH MySQL plan %q supports at most %d node(s), got %d", catalog.DatabasePlan, maxNodes, catalog.DatabaseNodeCount))
		}
	}
	if catalog.ValkeyPlan != "" {
		minNodes, maxNodes, ok := ovhValkeyPlanBounds(catalog.ValkeyPlan)
		if !ok {
			problems = append(problems, fmt.Errorf("unsupported OVH Valkey plan %q; use discovery, essential, business, or production", catalog.ValkeyPlan))
		} else if catalog.ValkeyNodeCount > 0 && catalog.ValkeyNodeCount < minNodes {
			problems = append(problems, fmt.Errorf("OVH Valkey plan %q requires at least %d node(s), got %d", catalog.ValkeyPlan, minNodes, catalog.ValkeyNodeCount))
		} else if catalog.ValkeyNodeCount > maxNodes {
			problems = append(problems, fmt.Errorf("OVH Valkey plan %q supports at most %d node(s), got %d", catalog.ValkeyPlan, maxNodes, catalog.ValkeyNodeCount))
		}
	}
	return problems
}

func ovhMySQLPlanBounds(plan string) (int, int, bool) {
	switch plan {
	case "discovery", "essential":
		return 1, 1, true
	case "business", "production":
		return 2, 2, true
	case "enterprise", "advanced":
		return 3, 3, true
	default:
		return 0, 0, false
	}
}

func ovhValkeyPlanBounds(plan string) (int, int, bool) {
	switch plan {
	case "discovery", "essential":
		return 1, 1, true
	case "business", "production":
		return 2, 2, true
	default:
		return 0, 0, false
	}
}
