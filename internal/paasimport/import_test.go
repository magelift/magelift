package paasimport

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v4"
)

// These tests inspect the normalized document before the CLI renders PHP.
type Result struct {
	YAML     []byte
	Unmapped []UnmappedKey
}

func (r Result) HasUnmapped() bool         { return len(r.Unmapped) > 0 }
func MapACC(root string) (Result, error)   { return mapFixture(root, SourceACC) }
func MapUpsun(root string) (Result, error) { return mapFixture(root, SourceUpsun) }
func mapFixture(root string, kind SourceKind) (Result, error) {
	source, err := Read(root, kind, "")
	if err != nil {
		return Result{}, err
	}
	doc := map[string]any{"schemaVersion": 1, "project": map[string]any{"name": source.app["name"]}, "application": map[string]any{"edition": "open-source", "version": "2.4.9", "mode": "integrated"}, "build": map[string]any{}, "target": map[string]any{"provider": "aws", "runtime": "ecs-fargate", "aws": map[string]any{}}, "defaults": map[string]any{"preset": "preview", "region": "eu-west-3"}, "environments": map[string]any{"staging": map[string]any{"domain": "staging.example.invalid", "account": "123456789012", "class": "staging"}}}
	keys, err := source.Apply(doc)
	if err != nil {
		return Result{}, err
	}
	data, err := yaml.Marshal(doc)
	return Result{data, keys}, err
}

func TestModernUpsunSelectsMagentoAndPreservesServiceVersions(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".upsun")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for file, contents := range map[string]string{
		"apps.yaml":     "applications:\n  frontend:\n    type: nodejs:22\n  shop:\n    type: php:8.4\n    source: {root: commerce}\n    relationships: {primary: {service: db, endpoint: mysql}}\n",
		"routes.yaml":   "routes:\n  'http://a-redirect.example/': {type: redirect, to: 'https://frontend.example/'}\n  'https://shop.example/': {type: upstream, upstream: 'shop:http'}\n",
		"services.yaml": "services:\n  db: {type: 'mariadb:11.4'}\n  frontendcache: {type: 'redis:6'}\n  frontend-fastly: {type: fastly}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, file), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := Read(root, SourceUpsun, ""); err == nil || !strings.Contains(err.Error(), "--app") {
		t.Fatalf("ambiguous application: %v", err)
	}
	source, err := Read(root, SourceUpsun, "shop")
	if err != nil || source.AppRoot != "commerce" {
		t.Fatalf("source %v, %v", source, err)
	}
	project := map[string]any{"build": map[string]any{}, "application": map[string]any{"version": "2.4.8-p5"}, "target": map[string]any{"provider": "gcp", "gcp": map[string]any{}}, "environments": map[string]any{"staging": map[string]any{}}}
	keys, err := source.Apply(project)
	if err != nil || len(keys) != 0 {
		t.Fatalf("residuals %v, error %v", keys, err)
	}
	services := project["services"].(map[string]any)
	staging := project["environments"].(map[string]any)["staging"].(map[string]any)
	if staging["domain"] != "shop.example" || len(services) != 1 || services["database"].(map[string]any)["version"] != "11.4" || project["edge"] != nil || project["application"].(map[string]any)["version"] != "2.4.8-p5" {
		t.Fatal(project)
	}
}

func TestFixedUpsunImportsOnlySelectedAppWithDefaultEndpoints(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".platform")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"applications.yaml": "shop:\n  type: php:8.4\n  relationships: {database: null, cache: null}\nweb:\n  type: nodejs:22\n",
		"services.yaml":     "database: {type: 'oracle-mysql:8.4'}\ncache: {type: 'redis-persistent:7.2'}\nweb-fastly: {type: fastly}\n",
		"routes.yaml":       "'https://web.example/': {type: upstream, upstream: 'web:http'}\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	source, err := Read(root, SourceUpsun, "shop")
	if err != nil {
		t.Fatal(err)
	}
	project := importProjectWithoutDomain()
	residuals, err := source.Apply(project)
	if err != nil || len(residuals) != 0 {
		t.Fatalf("residuals %v, error %v", residuals, err)
	}
	services := project["services"].(map[string]any)
	database, cache := services["database"].(map[string]any), services["cache"].(map[string]any)
	staging := project["environments"].(map[string]any)["staging"].(map[string]any)
	if database["family"] != "mysql" || database["version"] != "8.4" || cache["family"] != "redis" || cache["version"] != "7.2" || project["edge"] != nil || staging["domain"] != nil {
		t.Fatal(project)
	}
}

func importProjectWithoutDomain() map[string]any {
	return map[string]any{
		"application":  map[string]any{},
		"build":        map[string]any{},
		"target":       map[string]any{"provider": "aws", "aws": map[string]any{}},
		"environments": map[string]any{"staging": map[string]any{}},
	}
}

func TestFastlyWithoutSourceDomainLeavesAManualFollowUp(t *testing.T) {
	source := Source{
		app:      accAppRaw{"name": "shop", "type": "php:8.4"},
		services: map[string]any{"edge": "fastly"}, serviceFile: ".magento/services.yaml",
	}
	project := importProjectWithoutDomain()
	residuals, err := source.Apply(project)
	if err != nil || !slices.Contains(residuals, UnmappedKey{Source: source.serviceFile, Path: "edge.domains"}) {
		t.Fatalf("residuals %v, error %v", residuals, err)
	}
	edge := project["edge"].(map[string]any)
	if edge["domains"] != nil || edge["externalProvider"] != "fastly" {
		t.Fatal(edge)
	}
}
