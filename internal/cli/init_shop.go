package cli

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/usererr"
)

const defaultInitRecipePHP = "8.5"

var (
	magentoInitVersion  = regexp.MustCompile(`^(?:v)?(2\.4\.\d+)(?:-p([1-9][0-9]*))?$`)
	errNoMagentoPackage = errors.New("no Magento metapackage or magento/magento2-base entry in composer.lock")
	editionMetapackage  = map[string]string{
		"magento/product-community-edition":  "open-source",
		"magento/product-enterprise-edition": "commerce",
		"magento/magento-cloud-metapackage":  "commerce",
	}
)

type inspectedShop struct {
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
	root = filepath.Clean(root)
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

	return inspectedShop{ProjectName: projectName, Edition: edition, Version: version}, nil
}

type composerLock struct {
	Packages []composerPackage `json:"packages"`
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
	catalog := config.CurrentCompatibilityCatalog()
	match := magentoInitVersion.FindStringSubmatch(shop.Version)
	if match == nil {
		return usererr.New(
			fmt.Sprintf("locked Magento release %q is not a supported exact version", shop.Version),
			"pin an exact 2.4.x or 2.4.x-pN release in composer.lock, then rerun magelift init",
			"docs/configuration.md",
		)
	}
	var line *config.CompatibilityReleaseLine
	for i := range catalog.Lines {
		if catalog.Lines[i].Line == match[1] {
			line = &catalog.Lines[i]
			break
		}
	}
	if line == nil {
		return usererr.New(
			fmt.Sprintf("Magento release line %s is not in the MageLift compatibility catalog", match[1]),
			"run magelift compatibility catalog and align the shop to a listed line, or author magelift.yaml manually",
			"docs/configuration.md",
		)
	}
	if !line.Available {
		reason := strings.TrimSpace(line.UnavailableReason)
		if reason == "" {
			reason = fmt.Sprintf("Magento %s is not available in the current catalog", line.Line)
		}
		return usererr.New(
			reason,
			"choose a supported Magento line or author magelift.yaml manually with compatibility.allowUnsupported if deliberate",
			"docs/configuration.md",
		)
	}
	if match[2] != "" {
		patch, err := strconv.Atoi(match[2])
		if err != nil {
			return usererr.New(
				fmt.Sprintf("Magento patch in %q is invalid", shop.Version),
				"pin a catalogued patch such as "+line.RecommendedRelease+", then rerun magelift init",
				"docs/configuration.md",
			)
		}
		if patch > line.MaximumPatch {
			return usererr.New(
				fmt.Sprintf("Magento %s is newer than the latest patch in the compatibility catalog (%s)", shop.Version, line.RecommendedRelease),
				"pin "+line.RecommendedRelease+" or author magelift.yaml manually for this shop",
				"docs/configuration.md",
			)
		}
	}
	if !containsString(line.PHPBranches, defaultInitRecipePHP) {
		branches := append([]string(nil), line.PHPBranches...)
		return usererr.New(
			fmt.Sprintf("the community alpha init recipe uses PHP %s; Magento %s supports PHP %s in the compatibility catalog, not PHP %s", defaultInitRecipePHP, line.Line, strings.Join(branches, " or "), defaultInitRecipePHP),
			"keep the shop version and author magelift.yaml with its supported PHP branch and explicit compatible builder/runtime overrides",
			"docs/onboarding.md",
		)
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func initNextAction() string {
	return "review project.name, cloud project or account placeholders, and environments.test.domain, then commit magelift.yaml and run magelift bootstrap --env test"
}

func validateStarterProvider(provider string) error {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "aws", "gcp":
		return nil
	default:
		return fmt.Errorf("MageLift supports starter providers \"aws\" and \"gcp\" only, got %q", provider)
	}
}

func renderMinimalStarter(provider string, shop inspectedShop) (string, error) {
	if err := validateStarterProvider(provider); err != nil {
		return "", err
	}
	template := minimalAWSStarterTemplate
	if strings.EqualFold(strings.TrimSpace(provider), "gcp") {
		template = minimalGCPStarterTemplate
	}
	starter := fmt.Sprintf(template, strconv.Quote(shop.ProjectName), shop.Edition, shop.Version)
	if shop.Edition == "commerce" {
		starter = strings.Replace(starter, "build:\n", "build:\n  composer:\n    credentials: env://COMPOSER_AUTH\n", 1)
	}
	return starter, nil
}

const minimalGCPStarterTemplate = `schemaVersion: 1
project:
  name: %s
application:
  edition: %s
  version: %s
  mode: integrated
  webRuntime: nginx-fpm
  magento:
    frontName: admin
build:
  php: "8.5"
  staticContent:
    locales: [en_US]
    themes: [Magento/luma]
target:
  provider: gcp
  runtime: gke-autopilot
  gcp:
    project: example-gcp-project
    region: us-central1
defaults:
  region: us-central1
  preset: preview
environments:
  test:
    class: staging
    preset: preview
    domain: test.example.invalid
extensions: {}
`

const minimalAWSStarterTemplate = `schemaVersion: 1
project:
  name: %s
application:
  edition: %s
  version: %s
  mode: integrated
  webRuntime: nginx-fpm
  magento:
    frontName: admin
build:
  php: "8.5"
target:
  provider: aws
  runtime: ecs-fargate
  aws:
    catalog:
      searchMode: disabled
defaults:
  region: eu-west-3
  preset: preview
environments:
  test:
    account: "123456789012"
    class: staging
    preset: preview
    domain: test.example.invalid
extensions: {}
`

func validateInitStarter(data []byte) error {
	file, err := config.Load(data)
	if err != nil {
		return fmt.Errorf("validate generated shop configuration: %w", err)
	}
	if _, err := file.ResolveBuild(); err != nil {
		return fmt.Errorf("validate generated build configuration: %w", err)
	}
	if _, err := file.Resolve("test", config.ResolveOptions{}); err != nil {
		return fmt.Errorf("validate generated test environment: %w", err)
	}
	return nil
}
