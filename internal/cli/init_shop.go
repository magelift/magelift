package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/usererr"
	"github.com/magelift/magelift/sdk"
)

type starterLocation struct{ account, region, domain string }

var (
	starterAccounts = map[string]*regexp.Regexp{
		"aws":   regexp.MustCompile(`^[0-9]{12}$`),
		"gcp":   regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`),
		"azure": regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
	}
	starterRegion   = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	starterDNSLabel = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)
)

func (v starterLocation) validate(provider string) error {
	if v.account != "" && !starterAccounts[provider].MatchString(v.account) {
		return fmt.Errorf("--cloud-account must be a valid %s account, project or subscription identifier", provider)
	}
	if v.region != "" && !starterRegion.MatchString(v.region) {
		return errors.New("--region requires a cloud region identifier")
	}
	if v.domain != "" {
		if !strings.Contains(v.domain, ".") || len("preview."+v.domain) > 253 {
			return errors.New("--domain requires a fully qualified DNS hostname")
		}
		for label := range strings.SplitSeq(v.domain, ".") {
			if !starterDNSLabel.MatchString(label) {
				return errors.New("--domain requires a lowercase DNS hostname")
			}
		}
	}
	return nil
}

func (v starterLocation) apply(project map[string]any, provider string) {
	target := project["target"].(map[string]any)[provider].(map[string]any)
	if v.region != "" {
		project["defaults"].(map[string]any)["region"] = v.region
		if provider == "gcp" {
			target["region"] = v.region
		}
	}
	if v.account != "" {
		switch provider {
		case "gcp":
			target["project"] = v.account
		case "azure":
			target["subscriptionId"] = v.account
		}
	}
	for name, item := range project["environments"].(map[string]any) {
		env := item.(map[string]any)
		if v.account != "" && provider == "aws" {
			env["account"] = v.account
		}
		if v.domain != "" {
			host := v.domain
			if name != "production" {
				host = name + "." + host
			}
			env["domain"] = host
		} else if provider == "azure" && strings.HasSuffix(fmt.Sprint(env["domain"]), ".example.invalid") {
			delete(env, "domain")
		}
	}
}

const defaultInitRecipePHP = "8.5"

var (
	magentoInitVersion  = regexp.MustCompile(`^(?:v)?(2\.\d+\.\d+)(?:-p([1-9][0-9]*))?$`)
	errNoMagentoPackage = errors.New("no Magento metapackage or magento/magento2-base entry in composer.lock")
	editionMetapackage  = map[string]string{
		"magento/product-community-edition":  "open-source",
		"magento/product-enterprise-edition": "commerce",
		"magento/magento-cloud-metapackage":  "commerce",
	}
)

type inspectedShop struct {
	PHP         string
	ProjectName string
	Edition     string
	Version     string
}

type initWriteResult struct {
	Path        string `json:"path" yaml:"path"`
	Provider    string `json:"provider" yaml:"provider"`
	Environment string `json:"environment" yaml:"environment"`
	Edition     string `json:"edition" yaml:"edition"`
	Release     string `json:"release" yaml:"release"`
	Next        string `json:"next" yaml:"next"`
}

func shopSourceRoot(configPath string) string {
	return filepath.Clean(filepath.Dir(filepath.Clean(configPath)))
}

func inspectShopSource(root string) (inspectedShop, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return inspectedShop{}, err
	}
	root = absolute
	for _, rel := range []string{"composer.json", "composer.lock", "bin/magento"} {
		path := filepath.Join(root, rel)
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				return inspectedShop{}, usererr.New(
					fmt.Sprintf("Magento shop source at %s is missing %s", root, rel),
					"run magelift init from the shop repository root that contains composer.json, composer.lock, and bin/magento",
					"docs/onboarding.md",
				)
			}
			return inspectedShop{}, fmt.Errorf("inspect %s: %w", rel, err)
		}
		if !info.Mode().IsRegular() {
			return inspectedShop{}, usererr.New(
				fmt.Sprintf("Magento shop source at %s: %s must be a regular file", root, rel),
				"restore a normal Composer project layout, then rerun magelift init",
				"docs/onboarding.md",
			)
		}
	}

	lockBytes, err := os.ReadFile(filepath.Join(root, "composer.lock"))
	if err != nil {
		return inspectedShop{}, fmt.Errorf("read composer.lock: %w", err)
	}
	edition, version, err := magentoReleaseFromLock(lockBytes)
	if errors.Is(err, errNoMagentoPackage) {
		data, readErr := os.ReadFile(filepath.Join(root, "composer.json"))
		var project composerProject
		if readErr == nil && json.Unmarshal(data, &project) == nil && (project.Name == "magento/magento2ce" || project.Name == "magento/magento2-base") {
			version, err = normalizeMagentoVersion(project.Version)
			edition = "open-source"
		}
	}
	if err != nil {
		return inspectedShop{}, usererr.Wrap(err,
			"could not infer the locked Magento edition and release from composer.lock",
			"ensure composer.lock lists magento/product-community-edition, magento/product-enterprise-edition, or magento/magento2-base, then rerun magelift init",
			"docs/onboarding.md",
		)
	}

	projectName, err := projectNameFromComposer(filepath.Join(root, "composer.json"))
	if err != nil {
		return inspectedShop{}, err
	}

	var locked composerLock
	_ = json.Unmarshal(lockBytes, &locked)
	php := locked.PlatformOverrides["php"]
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`).MatchString(php) {
		php = ""
	}
	return inspectedShop{ProjectName: projectName, Edition: edition, Version: version, PHP: php}, nil
}

type composerLock struct {
	PlatformOverrides map[string]string `json:"platform-overrides"`
	Packages          []composerPackage `json:"packages"`
}

type composerPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func magentoReleaseFromLock(data []byte) (edition, version string, err error) {
	var lock *composerLock
	if err := json.Unmarshal(data, &lock); err != nil {
		return "", "", fmt.Errorf("composer.lock is not valid JSON: %w", err)
	}
	// Commerce locks may also contain the community metapackage.
	if lock == nil {
		return "", "", fmt.Errorf("composer.lock must be a JSON object")
	}
	for _, name := range []string{"magento/product-enterprise-edition", "magento/magento-cloud-metapackage", "magento/product-community-edition"} {
		for _, pkg := range lock.Packages {
			if pkg.Name != name {
				continue
			}
			version, err := normalizeMagentoVersion(pkg.Version)
			if err != nil {
				return "", "", fmt.Errorf("%s: %w", pkg.Name, err)
			}
			return editionMetapackage[name], version, nil
		}
	}
	for _, pkg := range lock.Packages {
		if pkg.Name != "magento/magento2-base" {
			continue
		}
		version, err := normalizeMagentoVersion(pkg.Version)
		if err != nil {
			return "", "", fmt.Errorf("magento/magento2-base: %w", err)
		}
		return "open-source", version, nil
	}
	return "", "", errNoMagentoPackage
}

func normalizeMagentoVersion(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	match := magentoInitVersion.FindStringSubmatch(raw)
	if match == nil {
		return "", fmt.Errorf("unsupported version %q", raw)
	}
	if match[2] == "" {
		return match[1], nil
	}
	return match[1] + "-p" + match[2], nil
}

type composerProject struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func projectNameFromComposer(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read composer.json: %w", err)
	}
	var project *composerProject
	if err := json.Unmarshal(data, &project); err != nil {
		return "", usererr.Wrap(err,
			"composer.json is not valid JSON",
			"fix composer.json, then rerun magelift init",
			"docs/onboarding.md",
		)
	}
	if project == nil {
		return "", usererr.New("composer.json must be a JSON object", "restore composer.json before rerunning magelift init", "docs/onboarding.md")
	}
	name := strings.TrimSpace(project.Name)
	if strings.HasPrefix(name, "magento/") {
		name = filepath.Base(filepath.Dir(path))
	}
	if name == "" {
		return "example-shop", nil
	}
	if idx := strings.LastIndex(name, "/"); idx >= 0 && idx < len(name)-1 {
		name = name[idx+1:]
	}
	name = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		case r >= 'A' && r <= 'Z':
			return r + ('a' - 'A')
		default:
			return '-'
		}
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "example-shop", nil
	}
	if name[0] < 'a' || name[0] > 'z' {
		name = "shop-" + name
	}
	// GCP resource planning requires a letter-led name of at most 31 bytes.
	// Keep a digest when shortening so sibling shops with a shared prefix differ.
	if len(name) > 31 {
		digest := sha256.Sum256([]byte(name))
		name = strings.TrimRight(name[:22], "-") + fmt.Sprintf("-%x", digest[:4])
	}
	return name, nil
}

func validateShopForInitRecipe(shop inspectedShop) error {
	if magentoInitVersion.FindStringSubmatch(shop.Version) == nil {
		return usererr.New(fmt.Sprintf("locked Magento release %q is not an exact version", shop.Version), "pin an exact release in composer.lock, then rerun magelift init", "docs/configuration.md")
	}
	return nil
}

func recommendedInitPHP(version string) string {
	match := magentoInitVersion.FindStringSubmatch(version)
	if match != nil {
		for _, line := range config.CurrentCompatibilityCatalog().Lines {
			if line.Line == match[1] && len(line.PHPBranches) != 0 {
				return line.PHPBranches[len(line.PHPBranches)-1]
			}
		}
	}
	return defaultInitRecipePHP
}

func validateStarterProvider(provider string) error {
	switch provider {
	case "aws", "gcp", "azure":
		return nil
	default:
		return fmt.Errorf("unknown starter provider %q (choose aws, gcp or azure)", provider)
	}
}

// starterDocument is shared by new shops, existing shops and PaaS imports.
// Service choices live at the root; environments vary capacity and protection.
func starterDocument(provider string, shop inspectedShop) (map[string]any, error) {
	if err := validateStarterProvider(provider); err != nil {
		return nil, err
	}
	php := shop.PHP
	if php == "" {
		php = recommendedInitPHP(shop.Version)
	}
	build := map[string]any{"php": php, "staticContent": map[string]any{"locales": []string{"en_US"}, "themes": []string{"Magento/luma"}}}
	if shop.Edition == "commerce" {
		build["composer"] = map[string]any{"credentials": "env://COMPOSER_AUTH"}
	}
	target := map[string]any{"provider": provider}
	region := "us-central1"
	switch provider {
	case "gcp":
		target["runtime"] = "gke-autopilot"
		target["gcp"] = map[string]any{"project": "example-gcp-project", "region": region, "openSearchMode": "opensearch", "queueMode": "database"}
	case "aws":
		region = "eu-west-3"
		target["runtime"] = "ecs-fargate"
		target["aws"] = map[string]any{"catalog": map[string]any{"databaseEngine": "rds-mysql", "searchMode": "provisioned", "queueMode": "db"}}
	case "azure":
		region = "eastus"
		target["runtime"] = "aks"
		target["azure"] = map[string]any{"subscriptionId": "00000000-0000-0000-0000-000000000000", "resourceGroup": shop.ProjectName}
	}
	environments := map[string]any{
		"staging":    map[string]any{"class": "staging", "preset": "preview", "domain": "staging.example.invalid"},
		"uat":        map[string]any{"class": "staging", "preset": "standard", "domain": "uat.example.invalid"},
		"production": map[string]any{"class": "production", "preset": "high-availability", "domain": "shop.example.invalid", "protection": true},
		"preview":    map[string]any{"class": "preview", "preset": "preview", "domain": "preview.example.invalid", "previewTtl": "168h", "monthlyBudgetCents": 2500},
	}
	if provider == "aws" {
		for _, env := range environments {
			env.(map[string]any)["account"] = "123456789012"
		}
	}
	// Managed database and search APIs accept different version spellings
	// from upstream container registries. These are editable starting values.
	v, _ := config.RecommendedServiceVersions(shop.Version)
	database, search := v.MySQL, strings.TrimPrefix(v.OpenSearch, "OpenSearch_")
	if provider != "aws" {
		database = database[:strings.LastIndexByte(database, '.')]
		search = sdk.OpenSearchContainerTag(search)
	}
	cache := v.Valkey
	if provider == "gcp" && cache == "8.1" {
		// Memorystore offers 8.0 in the Valkey 8 series.
		cache = "8.0"
	}
	services := map[string]any{
		"database": map[string]any{"family": "mysql", "version": database},
		"cache":    map[string]any{"family": "valkey", "version": cache},
		"search":   map[string]any{"family": "opensearch", "version": search},
		"queue":    map[string]any{"family": "database"},
	}
	return map[string]any{
		"schemaVersion": 1, "project": map[string]any{"name": shop.ProjectName},
		"application": map[string]any{"edition": shop.Edition, "version": shop.Version, "mode": "integrated", "webRuntime": "nginx-fpm", "magento": map[string]any{"frontName": "admin"}},
		"build":       build, "target": target, "defaults": map[string]any{"region": region, "preset": "standard"},
		"environments": environments, "services": services,
	}, nil
}
