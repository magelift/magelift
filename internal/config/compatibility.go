package config

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/magelift/magelift/internal/webruntime"
)

type CompatibilityStatus string

const (
	CompatibilitySupported          CompatibilityStatus = "supported"
	CompatibilityUnsupportedAllowed CompatibilityStatus = "unsupported-allowed"
)

// CompatibilityServiceStatus describes why a service choice is present in the
// catalog. Adobe requirements and MageLift implementation support are separate
// facts, so the catalog does not collapse them into one boolean.
type CompatibilityServiceStatus string

const (
	CompatibilityAdobeSupported     CompatibilityServiceStatus = "adobe-supported"
	CompatibilityMageLift           CompatibilityServiceStatus = "magelift-compatible"
	CompatibilityServiceUnsupported CompatibilityServiceStatus = "unsupported"
	CompatibilityServiceUnavailable CompatibilityServiceStatus = "unavailable"
)

type CompatibilityComponent string

const (
	CompatibilityComposer  CompatibilityComponent = "composer"
	CompatibilityDatabase  CompatibilityComponent = "database"
	CompatibilitySearch    CompatibilityComponent = "search"
	CompatibilityQueue     CompatibilityComponent = "queue"
	CompatibilityCache     CompatibilityComponent = "cache"
	CompatibilityWebCache  CompatibilityComponent = "web-cache"
	CompatibilityWebServer CompatibilityComponent = "web-server"
	CompatibilityPHP       CompatibilityComponent = "php"
	CompatibilityEdge      CompatibilityComponent = "edge"
)

type CompatibilitySource struct {
	Name        string `json:"name" yaml:"name"`
	URL         string `json:"url" yaml:"url"`
	RetrievedAt string `json:"retrievedAt" yaml:"retrievedAt"`
}

type CompatibilityMetadata struct {
	UpdatedAt       string                `json:"updatedAt" yaml:"updatedAt"`
	AdobeSnapshotAt string                `json:"adobeSnapshotAt" yaml:"adobeSnapshotAt"`
	Scope           string                `json:"scope" yaml:"scope"`
	Sources         []CompatibilitySource `json:"sources" yaml:"sources"`
}

type CompatibilityReleaseLine struct {
	Line               string   `json:"line" yaml:"line"`
	RecommendedRelease string   `json:"recommendedRelease" yaml:"recommendedRelease"`
	PHPBranches        []string `json:"phpBranches" yaml:"phpBranches"`
	MaximumPatch       int      `json:"maximumPatch" yaml:"maximumPatch"`
	Available          bool     `json:"available" yaml:"available"`
	UnavailableReason  string   `json:"unavailableReason,omitempty" yaml:"unavailableReason,omitempty"`
}

// CompatibilityRequirement is one service choice for one exact Adobe release.
// An empty Versions slice means the option is version-independent in the
// catalog, such as database-backed messaging or no Varnish layer.
type CompatibilityRequirement struct {
	Release   string                     `json:"release" yaml:"release"`
	Component CompatibilityComponent     `json:"component" yaml:"component"`
	Option    string                     `json:"option" yaml:"option"`
	Versions  []string                   `json:"versions,omitempty" yaml:"versions,omitempty"`
	Status    CompatibilityServiceStatus `json:"status" yaml:"status"`
	Notes     string                     `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type PHPBranchSupport struct {
	Branch               string `json:"branch" yaml:"branch"`
	Status               string `json:"status" yaml:"status"`
	ActiveSupportUntil   string `json:"activeSupportUntil" yaml:"activeSupportUntil"`
	SecuritySupportUntil string `json:"securitySupportUntil" yaml:"securitySupportUntil"`
}

type CompatibilityCatalog struct {
	Metadata     CompatibilityMetadata      `json:"metadata" yaml:"metadata"`
	Lines        []CompatibilityReleaseLine `json:"lines" yaml:"lines"`
	PHP          []PHPBranchSupport         `json:"php" yaml:"php"`
	Requirements []CompatibilityRequirement `json:"requirements" yaml:"requirements"`
}

// ServiceRequirementForRelease returns the provider-neutral Adobe/MageLift
// requirement for an exact release or its catalogued release line. Provider
// adapters translate the returned family/version into their API enum; this
// core function must not know names such as VALKEY_9_0.
func ServiceRequirementForRelease(release string, component CompatibilityComponent, option string) (CompatibilityRequirement, error) {
	if err := currentCompatibilityCatalog.Validate(time.Now().UTC()); err != nil {
		return CompatibilityRequirement{}, fmt.Errorf("validate compatibility catalog: %w", err)
	}
	requested := strings.TrimSpace(release)
	for _, requirement := range currentCompatibilityCatalog.Requirements {
		if requirement.Release == requested && requirement.Component == component && requirement.Option == option {
			return requirement, nil
		}
	}
	for _, line := range currentCompatibilityCatalog.Lines {
		if line.Line != requested {
			continue
		}
		for _, requirement := range currentCompatibilityCatalog.Requirements {
			if requirement.Release == line.RecommendedRelease && requirement.Component == component && requirement.Option == option {
				return requirement, nil
			}
		}
	}
	return CompatibilityRequirement{}, fmt.Errorf("%s/%s is not in the compatibility catalog for %s", component, option, release)
}

type CompatibilityAssessment struct {
	Status             CompatibilityStatus `json:"status" yaml:"status"`
	MagentoVersion     string              `json:"magentoVersion" yaml:"magentoVersion"`
	PHPBranch          string              `json:"phpBranch" yaml:"phpBranch"`
	RecommendedRelease string              `json:"recommendedRelease,omitempty" yaml:"recommendedRelease,omitempty"`
	Reasons            []string            `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	Warnings           []string            `json:"warnings,omitempty" yaml:"warnings,omitempty"`
	CatalogUpdatedAt   string              `json:"catalogUpdatedAt" yaml:"catalogUpdatedAt"`
}

var currentCompatibilityCatalog = CompatibilityCatalog{
	Metadata: CompatibilityMetadata{
		UpdatedAt:       "2026-08-12",
		AdobeSnapshotAt: "2026-06-01",
		Scope:           "Adobe Commerce release requirements and MageLift service choices for the RC1 architecture matrix.",
		Sources: []CompatibilitySource{
			{Name: "Adobe Commerce system requirements", URL: "https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements?lang=en", RetrievedAt: "2026-08-08"},
			{Name: "PHP supported versions", URL: "https://www.php.net/supported-versions.php", RetrievedAt: "2026-08-08"},
			{Name: "Amazon MQ RabbitMQ engine versions", URL: "https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-version-management.html", RetrievedAt: "2026-08-08"},
			{Name: "Amazon ElastiCache engine versions", URL: "https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/engine-versions.html", RetrievedAt: "2026-08-08"},
			{Name: "Google Cloud Memorystore for Valkey supported versions", URL: "https://docs.cloud.google.com/memorystore/docs/valkey/supported-versions", RetrievedAt: "2026-08-12"},
		},
	},
	Lines: []CompatibilityReleaseLine{
		{Line: "2.4.9", RecommendedRelease: "2.4.9", PHPBranches: []string{"8.5"}, MaximumPatch: 0, Available: true},
		{Line: "2.4.8", RecommendedRelease: "2.4.8-p5", PHPBranches: []string{"8.3", "8.4"}, MaximumPatch: 5, Available: true},
		{Line: "2.4.7", RecommendedRelease: "2.4.7-p10", PHPBranches: []string{"8.2", "8.3"}, MaximumPatch: 10, Available: true},
		{Line: "2.4.6", RecommendedRelease: "2.4.6-p15", PHPBranches: []string{"8.2"}, MaximumPatch: 15, Available: true},
		{Line: "2.4.5", RecommendedRelease: "2.4.5-p17", PHPBranches: []string{"8.1"}, MaximumPatch: 17, Available: false, UnavailableReason: "Adobe lists this line with PHP 8.1, which is below MageLift's PHP 8.2 minimum."},
		{Line: "2.4.4", RecommendedRelease: "2.4.4-p18", PHPBranches: []string{"8.1"}, MaximumPatch: 18, Available: false, UnavailableReason: "Adobe lists this line with PHP 8.1, which is below MageLift's PHP 8.2 minimum."},
	},
	PHP: []PHPBranchSupport{
		{Branch: "8.2", Status: "security-fixes-only", ActiveSupportUntil: "2024-12-31", SecuritySupportUntil: "2026-12-31"},
		{Branch: "8.3", Status: "security-fixes-only", ActiveSupportUntil: "2025-12-31", SecuritySupportUntil: "2027-12-31"},
		{Branch: "8.4", Status: "active", ActiveSupportUntil: "2026-12-31", SecuritySupportUntil: "2028-12-31"},
		{Branch: "8.5", Status: "active", ActiveSupportUntil: "2027-12-31", SecuritySupportUntil: "2029-12-31"},
	},
	Requirements: []CompatibilityRequirement{
		{Release: "2.4.6-p15", Component: CompatibilityComposer, Option: "composer", Versions: []string{"2.2.26+"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityDatabase, Option: "mariadb", Versions: []string{"10.11"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityDatabase, Option: "mysql", Status: CompatibilityServiceUnsupported, Notes: "Adobe's current latest-patch row does not list MySQL."},
		{Release: "2.4.6-p15", Component: CompatibilitySearch, Option: "opensearch", Versions: []string{"2", "3"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilitySearch, Option: "elasticsearch", Status: CompatibilityServiceUnsupported, Notes: "Elasticsearch 7.17 is outside the current latest-patch row."},
		{Release: "2.4.6-p15", Component: CompatibilityQueue, Option: "rabbitmq", Versions: []string{"4.2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityQueue, Option: "artemis", Versions: []string{"2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityQueue, Option: "database", Status: CompatibilityMageLift, Notes: "Magento database-backed messaging avoids a broker and is a MageLift topology choice."},
		{Release: "2.4.6-p15", Component: CompatibilityCache, Option: "valkey", Versions: []string{"8.1"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityCache, Option: "redis", Status: CompatibilityServiceUnsupported, Notes: "The latest-patch row uses Valkey instead of Redis."},
		{Release: "2.4.6-p15", Component: CompatibilityWebCache, Option: "varnish", Versions: []string{"8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityWebCache, Option: "none", Status: CompatibilityMageLift, Notes: "Headless and API-only deployments may omit the Varnish layer."},
		{Release: "2.4.6-p15", Component: CompatibilityWebServer, Option: "nginx", Versions: []string{"1.30"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityPHP, Option: "php", Versions: []string{"8.2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.6-p15", Component: CompatibilityEdge, Option: "fastly", Status: CompatibilityMageLift, Notes: "Fastly is an experimental edge migration target, not an Adobe system requirement."},

		{Release: "2.4.7-p10", Component: CompatibilityComposer, Option: "composer", Versions: []string{"2.10"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityDatabase, Option: "mariadb", Versions: []string{"10.11", "11.8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityDatabase, Option: "mysql", Status: CompatibilityServiceUnsupported, Notes: "Adobe's current latest-patch row does not list MySQL."},
		{Release: "2.4.7-p10", Component: CompatibilitySearch, Option: "opensearch", Versions: []string{"2", "3"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilitySearch, Option: "elasticsearch", Versions: []string{"8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityQueue, Option: "rabbitmq", Versions: []string{"4.2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityQueue, Option: "artemis", Versions: []string{"2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityQueue, Option: "database", Status: CompatibilityMageLift, Notes: "Magento database-backed messaging avoids a broker and is a MageLift topology choice."},
		{Release: "2.4.7-p10", Component: CompatibilityCache, Option: "valkey", Versions: []string{"8.1"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityCache, Option: "redis", Status: CompatibilityServiceUnsupported, Notes: "The latest-patch row uses Valkey instead of Redis."},
		{Release: "2.4.7-p10", Component: CompatibilityWebCache, Option: "varnish", Versions: []string{"8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityWebCache, Option: "none", Status: CompatibilityMageLift, Notes: "Headless and API-only deployments may omit the Varnish layer."},
		{Release: "2.4.7-p10", Component: CompatibilityWebServer, Option: "nginx", Versions: []string{"1.30"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityPHP, Option: "php", Versions: []string{"8.2", "8.3"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.7-p10", Component: CompatibilityEdge, Option: "fastly", Status: CompatibilityMageLift, Notes: "Fastly is an experimental edge migration target, not an Adobe system requirement."},

		{Release: "2.4.8-p5", Component: CompatibilityComposer, Option: "composer", Versions: []string{"2.10"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityDatabase, Option: "mariadb", Versions: []string{"11.4", "11.8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityDatabase, Option: "mysql", Versions: []string{"8.4"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilitySearch, Option: "opensearch", Versions: []string{"3"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilitySearch, Option: "elasticsearch", Versions: []string{"8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityQueue, Option: "rabbitmq", Versions: []string{"4.2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityQueue, Option: "artemis", Versions: []string{"2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityQueue, Option: "database", Status: CompatibilityMageLift, Notes: "Magento database-backed messaging avoids a broker and is a MageLift topology choice."},
		{Release: "2.4.8-p5", Component: CompatibilityCache, Option: "valkey", Versions: []string{"8.1"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityCache, Option: "redis", Status: CompatibilityServiceUnsupported, Notes: "The latest-patch row uses Valkey instead of Redis."},
		{Release: "2.4.8-p5", Component: CompatibilityWebCache, Option: "varnish", Versions: []string{"8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityWebCache, Option: "none", Status: CompatibilityMageLift, Notes: "Headless and API-only deployments may omit the Varnish layer."},
		{Release: "2.4.8-p5", Component: CompatibilityWebServer, Option: "nginx", Versions: []string{"1.30"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityPHP, Option: "php", Versions: []string{"8.3", "8.4"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.8-p5", Component: CompatibilityEdge, Option: "fastly", Status: CompatibilityMageLift, Notes: "Fastly is an experimental edge migration target, not an Adobe system requirement."},

		{Release: "2.4.9", Component: CompatibilityComposer, Option: "composer", Versions: []string{"2.10"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityDatabase, Option: "mariadb", Versions: []string{"12.3", "11.8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityDatabase, Option: "mysql", Versions: []string{"8.4"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilitySearch, Option: "opensearch", Versions: []string{"3"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilitySearch, Option: "elasticsearch", Status: CompatibilityServiceUnsupported, Notes: "The current latest-patch row lists OpenSearch 3 instead."},
		{Release: "2.4.9", Component: CompatibilityQueue, Option: "rabbitmq", Versions: []string{"4.2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityQueue, Option: "artemis", Versions: []string{"2"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityQueue, Option: "database", Status: CompatibilityMageLift, Notes: "Magento database-backed messaging avoids a broker and is a MageLift topology choice."},
		{Release: "2.4.9", Component: CompatibilityCache, Option: "valkey", Versions: []string{"9"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityCache, Option: "redis", Status: CompatibilityServiceUnsupported, Notes: "The latest-patch row uses Valkey instead of Redis."},
		{Release: "2.4.9", Component: CompatibilityWebCache, Option: "varnish", Versions: []string{"8"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityWebCache, Option: "none", Status: CompatibilityMageLift, Notes: "Headless and API-only deployments may omit the Varnish layer."},
		{Release: "2.4.9", Component: CompatibilityWebServer, Option: "nginx", Versions: []string{"1.30"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityPHP, Option: "php", Versions: []string{"8.5"}, Status: CompatibilityAdobeSupported},
		{Release: "2.4.9", Component: CompatibilityEdge, Option: "fastly", Status: CompatibilityMageLift, Notes: "Fastly is an experimental edge migration target, not an Adobe system requirement."},
	},
}

// Validate rejects a compatibility catalog whose source metadata is missing
// or stale. Provider-specific API enum translation remains in adapters, but
// no adapter may silently select a release mapping from an expired catalog.
func (c CompatibilityCatalog) Validate(now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if strings.TrimSpace(c.Metadata.UpdatedAt) == "" || strings.TrimSpace(c.Metadata.AdobeSnapshotAt) == "" || strings.TrimSpace(c.Metadata.Scope) == "" {
		return fmt.Errorf("compatibility catalog metadata is incomplete")
	}
	for name, value := range map[string]string{"updatedAt": c.Metadata.UpdatedAt, "adobeSnapshotAt": c.Metadata.AdobeSnapshotAt} {
		parsed, err := time.Parse("2006-01-02", value)
		if err != nil {
			return fmt.Errorf("compatibility catalog %s: %w", name, err)
		}
		if parsed.After(now.Add(24 * time.Hour)) {
			return fmt.Errorf("compatibility catalog %s is in the future", name)
		}
	}
	if len(c.Metadata.Sources) == 0 {
		return fmt.Errorf("compatibility catalog requires source records")
	}
	for _, source := range c.Metadata.Sources {
		if strings.TrimSpace(source.Name) == "" || strings.TrimSpace(source.RetrievedAt) == "" {
			return fmt.Errorf("compatibility catalog source metadata is incomplete")
		}
		parsedURL, err := url.Parse(source.URL)
		if err != nil || parsedURL.Scheme != "https" || parsedURL.Host == "" {
			return fmt.Errorf("compatibility catalog source URL must be HTTPS: %q", source.URL)
		}
		retrieved, err := time.Parse("2006-01-02", source.RetrievedAt)
		if err != nil {
			return fmt.Errorf("compatibility catalog source %q retrievedAt: %w", source.Name, err)
		}
		if retrieved.After(now.Add(24*time.Hour)) || now.Sub(retrieved) > 90*24*time.Hour {
			return fmt.Errorf("compatibility catalog source %q is stale", source.Name)
		}
	}
	if len(c.Lines) == 0 || len(c.Requirements) == 0 {
		return fmt.Errorf("compatibility catalog requires release lines and requirements")
	}
	return nil
}

func CurrentCompatibilityCatalog() CompatibilityCatalog {
	catalog := currentCompatibilityCatalog
	catalog.Lines = append([]CompatibilityReleaseLine(nil), catalog.Lines...)
	catalog.PHP = append([]PHPBranchSupport(nil), catalog.PHP...)
	catalog.Metadata.Sources = append([]CompatibilitySource(nil), catalog.Metadata.Sources...)
	catalog.Requirements = append([]CompatibilityRequirement(nil), catalog.Requirements...)
	for i := range catalog.Lines {
		catalog.Lines[i].PHPBranches = append([]string(nil), catalog.Lines[i].PHPBranches...)
	}
	for i := range catalog.Requirements {
		catalog.Requirements[i].Versions = append([]string(nil), catalog.Requirements[i].Versions...)
	}
	return catalog
}

// RequirementsForRelease returns the service choices for one exact release.
func RequirementsForRelease(release string) []CompatibilityRequirement {
	all := CurrentCompatibilityCatalog().Requirements
	filtered := make([]CompatibilityRequirement, 0)
	for _, requirement := range all {
		if requirement.Release == release {
			requirement.Versions = append([]string(nil), requirement.Versions...)
			filtered = append(filtered, requirement)
		}
	}
	return filtered
}

type CompatibilitySelection struct {
	Release   string
	Component CompatibilityComponent
	Option    string
	Version   string
}

// ValidateCompatibilitySelection validates one catalog choice without making
// provider calls. Provider availability is checked separately by the target
// module, so a catalog match is not itself a certification claim.
func ValidateCompatibilitySelection(selection CompatibilitySelection) (CompatibilityRequirement, error) {
	for _, requirement := range currentCompatibilityCatalog.Requirements {
		if requirement.Release != selection.Release || requirement.Component != selection.Component || requirement.Option != selection.Option {
			continue
		}
		if requirement.Status != CompatibilityAdobeSupported && requirement.Status != CompatibilityMageLift {
			return requirement, fmt.Errorf("%s/%s is %s", selection.Component, selection.Option, requirement.Status)
		}
		if len(requirement.Versions) > 0 && !contains(requirement.Versions, selection.Version) {
			return requirement, fmt.Errorf("%s/%s does not list version %q", selection.Component, selection.Option, selection.Version)
		}
		return requirement, nil
	}
	return CompatibilityRequirement{}, fmt.Errorf("%s/%s is not in the compatibility catalog for %s", selection.Component, selection.Option, selection.Release)
}

var magentoExactVersion = regexp.MustCompile(`^(2\.4\.\d+)(?:-p([1-9][0-9]*))?$`)
var phpExactVersion = regexp.MustCompile(`^(8\.\d+)(?:\.\d+)?$`)

func assessCompatibility(c Config) (CompatibilityAssessment, string) {
	assessment := CompatibilityAssessment{
		MagentoVersion:   c.Application.Version,
		CatalogUpdatedAt: currentCompatibilityCatalog.Metadata.UpdatedAt,
	}
	magentoMatch := magentoExactVersion.FindStringSubmatch(c.Application.Version)
	if magentoMatch == nil {
		return assessment, fmt.Sprintf("application.version %q must be an exact Magento release such as 2.4.8-p5; ranges are not resolved", c.Application.Version)
	}
	phpMatch := phpExactVersion.FindStringSubmatch(c.Build.PHP)
	if phpMatch == nil {
		return assessment, fmt.Sprintf("build.php %q must select an exact PHP branch or patch version such as 8.4 or 8.4.1", c.Build.PHP)
	}
	assessment.PHPBranch = phpMatch[1]

	var release *CompatibilityReleaseLine
	for i := range currentCompatibilityCatalog.Lines {
		if currentCompatibilityCatalog.Lines[i].Line == magentoMatch[1] {
			release = &currentCompatibilityCatalog.Lines[i]
			break
		}
	}
	var reasons []string
	if release == nil {
		reasons = append(reasons, fmt.Sprintf("Magento release line %s is not in the current compatibility catalog", magentoMatch[1]))
	} else {
		assessment.RecommendedRelease = release.RecommendedRelease
		patch := 0
		if magentoMatch[2] != "" {
			var err error
			patch, err = strconv.Atoi(magentoMatch[2])
			if err != nil {
				reasons = append(reasons, fmt.Sprintf("Magento patch number in %s is outside the supported range", c.Application.Version))
			}
		}
		if patch > release.MaximumPatch {
			reasons = append(reasons, fmt.Sprintf("Magento %s is newer than the latest patch in the Adobe snapshot (%s)", c.Application.Version, release.RecommendedRelease))
		}
		if !release.Available {
			reasons = append(reasons, release.UnavailableReason)
		}
		if !contains(release.PHPBranches, assessment.PHPBranch) {
			branches := append([]string(nil), release.PHPBranches...)
			sort.Strings(branches)
			reasons = append(reasons, fmt.Sprintf("Magento %s supports PHP %s in this catalog, not PHP %s", release.Line, strings.Join(branches, " or "), assessment.PHPBranch))
		}
	}

	if len(reasons) == 0 {
		assessment.Status = CompatibilitySupported
	} else {
		assessment.Reasons = reasons
		if c.Compatibility.AllowUnsupported {
			assessment.Status = CompatibilityUnsupportedAllowed
		} else {
			return assessment, strings.Join(reasons, "; ") + "; commit compatibility.allowUnsupported: true to accept this combination"
		}
	}
	if msg := assessSelectedServices(c, &assessment); msg != "" {
		return assessment, msg
	}
	return assessment, ""
}

func assessSelectedServices(c Config, assessment *CompatibilityAssessment) string {
	release := assessment.RecommendedRelease
	if release == "" {
		release = c.Application.Version
	}
	assessment.Warnings = append(assessment.Warnings, experimentalTargetWarnings(c)...)
	if msg := providerUnavailableReason(c); msg != "" {
		return msg
	}
	if msg := assessWebRuntime(c, assessment); msg != "" {
		return msg
	}
	family := selectedDatabaseFamily(c)
	if family == "" {
		return ""
	}
	requirement, err := ServiceRequirementForRelease(release, CompatibilityDatabase, family)
	if err != nil {
		return ""
	}
	if requirement.Status != CompatibilityServiceUnsupported {
		return ""
	}
	supported := nearestAdobeDatabaseFamily(release)
	msg := fmt.Sprintf("Adobe: Magento %s does not list %s as a supported database family; nearest Adobe-supported family is %s", release, family, supported)
	if c.Compatibility.AllowUnsupported {
		assessment.Reasons = append(assessment.Reasons, msg)
		assessment.Status = CompatibilityUnsupportedAllowed
		return ""
	}
	return msg + "; commit compatibility.allowUnsupported: true to accept this Adobe-unsupported combination"
}

func assessWebRuntime(c Config, assessment *CompatibilityAssessment) string {
	warning, err := webruntime.Admit(c.Application.WebRuntime, c.Application.Version, c.Compatibility.AllowUnsupported)
	if err != nil {
		return err.Error()
	}
	if warning != "" {
		assessment.Warnings = append(assessment.Warnings, warning)
		assessment.Reasons = append(assessment.Reasons, warning)
		assessment.Status = CompatibilityUnsupportedAllowed
	}
	return ""
}

func selectedDatabaseFamily(c Config) string {
	if family := strings.ToLower(strings.TrimSpace(c.Local.Database.Family)); family != "" {
		return family
	}
	if c.Target.AWS != nil {
		switch strings.TrimSpace(c.Target.AWS.Catalog.DatabaseEngine) {
		case "aurora-mysql", "rds-mysql":
			return "mysql"
		case "rds-mariadb":
			return "mariadb"
		}
	}
	return ""
}

func nearestAdobeDatabaseFamily(release string) string {
	for _, requirement := range RequirementsForRelease(release) {
		if requirement.Component == CompatibilityDatabase && requirement.Status == CompatibilityAdobeSupported {
			return requirement.Option
		}
	}
	return "mariadb"
}

func experimentalTargetWarnings(c Config) []string {
	key := strings.TrimSpace(c.Target.Provider) + "/" + strings.TrimSpace(c.Target.Runtime)
	var warnings []string
	switch key {
	case "aws/eks", "gcp/gke-standard", "ovh/mks", "scaleway/kapsule":
		warnings = append(warnings, fmt.Sprintf("MageLift: target %s is experimental; not certified. See docs/capability-matrix.md", key))
	}
	if c.Target.Provider == "aws" && c.Target.AWS != nil {
		mode := strings.TrimSpace(c.Target.AWS.Catalog.Fargate.ComputeMode)
		switch mode {
		case "managed-instances", "ec2-asg", "fargate-spot":
			warnings = append(warnings, fmt.Sprintf("MageLift: AWS ECS %s is experimental; not certified. See docs/capability-matrix.md", mode))
		}
		catalog := c.Target.AWS.Catalog
		if strings.TrimSpace(catalog.DatabaseEngine) == "aurora-mysql" {
			warnings = append(warnings, "MageLift: AWS Aurora MySQL is experimental; not certified. See docs/capability-matrix.md")
		}
		if strings.TrimSpace(catalog.SearchMode) == "provisioned" {
			warnings = append(warnings, "MageLift: AWS OpenSearch Magento data-plane is experimental; not certified. See docs/capability-matrix.md")
		}
		queueMode := strings.TrimSpace(catalog.QueueMode)
		if queueMode == "" {
			preset := strings.TrimSpace(c.Preset)
			if preset == "" {
				preset = strings.TrimSpace(c.Defaults.Preset)
			}
			if preset != "preview" {
				queueMode = "ecs-rabbitmq"
			}
		}
		if queueMode == "amazon-mq" {
			warnings = append(warnings, "MageLift: AWS Amazon MQ is experimental-warn; not certified. See docs/capability-matrix.md")
		}
		if strings.TrimSpace(catalog.EKS.QueueMode) == "rabbitmq" && catalog.EKS.QueueReplicas >= 3 {
			warnings = append(warnings, "MageLift: AWS EKS RabbitMQ quorum is experimental; not certified. See docs/capability-matrix.md")
		}
	}
	return warnings
}

func providerUnavailableReason(c Config) string {
	if c.Target.Provider == "ovh" && strings.EqualFold(strings.TrimSpace(c.Edge.NativeProvider), "cdn") {
		return "OVH: native CDN is unavailable (no MKS CDN adapter); use Fastly or the MKS load balancer"
	}
	return ""
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
