package config

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type CompatibilityStatus string

const (
	CompatibilitySupported          CompatibilityStatus = "supported"
	CompatibilityUnsupportedAllowed CompatibilityStatus = "unsupported-allowed"
)

type CompatibilitySource struct {
	Name string `json:"name" yaml:"name"`
	URL  string `json:"url" yaml:"url"`
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

type PHPBranchSupport struct {
	Branch               string `json:"branch" yaml:"branch"`
	Status               string `json:"status" yaml:"status"`
	ActiveSupportUntil   string `json:"activeSupportUntil" yaml:"activeSupportUntil"`
	SecuritySupportUntil string `json:"securitySupportUntil" yaml:"securitySupportUntil"`
}

type CompatibilityCatalog struct {
	Metadata CompatibilityMetadata      `json:"metadata" yaml:"metadata"`
	Lines    []CompatibilityReleaseLine `json:"lines" yaml:"lines"`
	PHP      []PHPBranchSupport         `json:"php" yaml:"php"`
}

type CompatibilityAssessment struct {
	Status             CompatibilityStatus `json:"status" yaml:"status"`
	MagentoVersion     string              `json:"magentoVersion" yaml:"magentoVersion"`
	PHPBranch          string              `json:"phpBranch" yaml:"phpBranch"`
	RecommendedRelease string              `json:"recommendedRelease,omitempty" yaml:"recommendedRelease,omitempty"`
	Reasons            []string            `json:"reasons,omitempty" yaml:"reasons,omitempty"`
	CatalogUpdatedAt   string              `json:"catalogUpdatedAt" yaml:"catalogUpdatedAt"`
}

var currentCompatibilityCatalog = CompatibilityCatalog{
	Metadata: CompatibilityMetadata{
		UpdatedAt:       "2026-07-18",
		AdobeSnapshotAt: "2026-06-01",
		Scope:           "Magento, PHP, and the AWS service combinations used by the v1 stack planner.",
		Sources: []CompatibilitySource{
			{Name: "Adobe Commerce system requirements", URL: "https://experienceleague.adobe.com/en/docs/commerce-operations/installation-guide/system-requirements"},
			{Name: "PHP supported versions", URL: "https://www.php.net/supported-versions.php"},
			{Name: "Amazon MQ RabbitMQ engine versions", URL: "https://docs.aws.amazon.com/amazon-mq/latest/developer-guide/rabbitmq-version-management.html"},
			{Name: "Amazon ElastiCache engine versions", URL: "https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/engine-versions.html"},
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
}

func CurrentCompatibilityCatalog() CompatibilityCatalog {
	catalog := currentCompatibilityCatalog
	catalog.Lines = append([]CompatibilityReleaseLine(nil), catalog.Lines...)
	catalog.PHP = append([]PHPBranchSupport(nil), catalog.PHP...)
	catalog.Metadata.Sources = append([]CompatibilitySource(nil), catalog.Metadata.Sources...)
	for i := range catalog.Lines {
		catalog.Lines[i].PHPBranches = append([]string(nil), catalog.Lines[i].PHPBranches...)
	}
	return catalog
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
		return assessment, ""
	}
	assessment.Reasons = reasons
	if c.Compatibility.AllowUnsupported {
		assessment.Status = CompatibilityUnsupportedAllowed
		return assessment, ""
	}
	return assessment, strings.Join(reasons, "; ") + "; commit compatibility.allowUnsupported: true to accept this combination"
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
