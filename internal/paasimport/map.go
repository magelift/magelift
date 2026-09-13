package paasimport

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// mageliftDocument is the subset of MageLift YAML emitted by the importer.
// Fields must already exist on starterConfig / schema (IMPORT-04).
type mageliftDocument struct {
	SchemaVersion int                    `yaml:"schemaVersion"`
	Project       mageliftProject        `yaml:"project"`
	Application   mageliftApplication    `yaml:"application"`
	Build         mageliftBuild          `yaml:"build"`
	Target        mageliftTarget         `yaml:"target"`
	Edge          *mageliftEdge          `yaml:"edge,omitempty"`
	Observability *mageliftObservability `yaml:"observability,omitempty"`
	Defaults      mageliftDefaults       `yaml:"defaults"`
	Environments  map[string]mageliftEnv `yaml:"environments"`
	Extensions    map[string]any         `yaml:"extensions"`
}

type mageliftProject struct {
	Name string `yaml:"name"`
}

type mageliftApplication struct {
	Edition string              `yaml:"edition"`
	Version string              `yaml:"version"`
	Mode    string              `yaml:"mode"`
	Magento *mageliftMagento    `yaml:"magento,omitempty"`
	Cron    []mageliftCronEntry `yaml:"cron,omitempty"`
}

// mageliftMagento mirrors the importable fields of config.MagentoRuntime.
type mageliftMagento struct {
	FrontName    string                   `yaml:"frontName,omitempty"`
	CookieDomain string                   `yaml:"cookieDomain,omitempty"`
	CORSOrigins  []string                 `yaml:"corsOrigins,omitempty"`
	Consumers    mageliftMagentoConsumers `yaml:"consumers,omitempty"`
}

type mageliftMagentoConsumers struct {
	Mode  string   `yaml:"mode,omitempty"`
	Names []string `yaml:"names,omitempty"`
}

type mageliftCronEntry struct {
	Schedule string `yaml:"schedule"`
	Command  string `yaml:"command"`
}

type mageliftBuild struct {
	PHP            string            `yaml:"php"`
	Extensions     []string          `yaml:"extensions,omitempty"`
	Composer       *mageliftComposer `yaml:"composer,omitempty"`
	StaticContent  map[string]any    `yaml:"staticContent,omitempty"`
	QualityPatches []string          `yaml:"qualityPatches,omitempty"`
}

type mageliftComposer struct {
	Version string `yaml:"version,omitempty"`
}

type mageliftTarget struct {
	Provider string       `yaml:"provider"`
	Runtime  string       `yaml:"runtime"`
	AWS      *mageliftAWS `yaml:"aws,omitempty"`
}

type mageliftEdge struct {
	ExternalProvider string   `yaml:"externalProvider"`
	Domains          []string `yaml:"domains,omitempty"`
	TLS              bool     `yaml:"tls,omitempty"`
}

type mageliftObservability struct {
	ExternalProvider string            `yaml:"externalProvider"`
	Endpoint         string            `yaml:"endpoint,omitempty"`
	ServiceName      string            `yaml:"serviceName,omitempty"`
	Environment      string            `yaml:"environment,omitempty"`
	Logs             bool              `yaml:"logs,omitempty"`
	Metrics          bool              `yaml:"metrics,omitempty"`
	Traces           bool              `yaml:"traces,omitempty"`
	Labels           map[string]string `yaml:"labels,omitempty"`
}

type mageliftAWS struct {
	EncryptionKeySecretARN string         `yaml:"encryptionKeySecretArn,omitempty"`
	Catalog                map[string]any `yaml:"catalog,omitempty"`
}

type mageliftDefaults struct {
	Region string `yaml:"region"`
	Preset string `yaml:"preset"`
}

type mageliftEnv struct {
	Account string `yaml:"account"`
	Domain  string `yaml:"domain,omitempty"`
}

// defaultEncryptionKeyPlaceholder is a secret-ref-shaped ARN placeholder.
// Crypt plaintext from PaaS env must never be written into magelift.yaml (T-04-02 / T-04-06).
const defaultEncryptionKeyPlaceholder = "arn:aws:secretsmanager:eu-west-3:123456789012:secret:magelift/encryption-key"

func emitMagelift(doc mageliftDocument) ([]byte, error) {
	data, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("encode magelift.yaml: %w", err)
	}
	return data, nil
}

func parsePHPType(typeValue string) (string, error) {
	typeValue = strings.TrimSpace(typeValue)
	if typeValue == "" {
		return "", fmt.Errorf("app type is required")
	}
	branch, ok := strings.CutPrefix(typeValue, "php:")
	if !ok || branch == "" {
		return "", fmt.Errorf("unsupported app type %q (want php:<version>)", typeValue)
	}
	return branch, nil
}

func domainFromRoutes(routes map[string]accRoute) string {
	for pattern := range routes {
		if strings.Contains(pattern, "{default}") {
			continue
		}
		u, err := url.Parse(pattern)
		if err != nil || u.Host == "" {
			continue
		}
		return u.Hostname()
	}
	return "staging.example.com"
}

func catalogFromRelationships(rels map[string]string) map[string]any {
	catalog := map[string]any{}
	for name, value := range rels {
		service := strings.ToLower(value)
		key := strings.ToLower(name)
		switch {
		case strings.Contains(service, "rabbitmq") || strings.Contains(service, "amqp") ||
			key == "rabbitmq" || key == "amqp" || key == "queue":
			catalog["queueMode"] = "ecs-rabbitmq"
		case strings.Contains(service, "opensearch") || strings.Contains(service, "elasticsearch") ||
			key == "opensearch" || key == "elasticsearch" || key == "search":
			catalog["searchMode"] = "serverless"
		}
	}
	if len(catalog) == 0 {
		return nil
	}
	return catalog
}

// applyAllowlistEnv maps D-07 env vars into doc and returns residual env keys.
func applyAllowlistEnv(doc *mageliftDocument, source string, env accEnvDocument) []UnmappedKey {
	type staged struct {
		path string
		key  string
		val  string
	}
	var vars []staged
	var unmapped []UnmappedKey
	for k, v := range env.Stage.Global {
		vars = append(vars, staged{path: "stage.global." + k, key: k, val: v})
	}
	for k, v := range env.Stage.Deploy {
		vars = append(vars, staged{path: "stage.deploy." + k, key: k, val: v})
	}
	for k, v := range env.Stage.Build {
		if k == "QUALITY_PATCHES" {
			ids, ok := qualityPatchIDs(v)
			if !ok {
				unmapped = append(unmapped, UnmappedKey{Source: source, Path: "stage.build.QUALITY_PATCHES"})
				continue
			}
			doc.Build.QualityPatches = ids
			continue
		}
		val, ok := v.(string)
		if !ok {
			unmapped = append(unmapped, UnmappedKey{Source: source, Path: "stage.build." + k})
			continue
		}
		vars = append(vars, staged{path: "stage.build." + k, key: k, val: val})
	}
	for k, v := range env.Variables.Env {
		vars = append(vars, staged{path: "variables.env." + k, key: k, val: v})
	}

	for _, item := range vars {
		if handled, residual := applyObservabilityEnv(doc, item.key, item.val); handled {
			if residual {
				unmapped = append(unmapped, UnmappedKey{Source: source, Path: item.path})
			}
			continue
		}
		if !IsAllowlistedEnv(item.key) {
			unmapped = append(unmapped, UnmappedKey{Source: source, Path: item.path})
			continue
		}
		if handled, valid := applyMagentoEnv(doc, item.key, item.val); handled {
			if !valid {
				unmapped = append(unmapped, UnmappedKey{Source: source, Path: item.path})
			}
			continue
		}
		switch item.key {
		case "SCD_STRATEGY":
			if item.val == "" {
				continue
			}
			if doc.Build.StaticContent == nil {
				doc.Build.StaticContent = map[string]any{}
			}
			doc.Build.StaticContent["strategy"] = item.val
		case "SCD_THREADS":
			n, err := strconv.Atoi(item.val)
			if err != nil || n < 1 {
				unmapped = append(unmapped, UnmappedKey{Source: source, Path: item.path})
				continue
			}
			if doc.Build.StaticContent == nil {
				doc.Build.StaticContent = map[string]any{}
			}
			doc.Build.StaticContent["threads"] = n
		case "CRYPT_KEY":
			if doc.Target.AWS == nil {
				doc.Target.AWS = &mageliftAWS{}
			}
			doc.Target.AWS.EncryptionKeySecretARN = defaultEncryptionKeyPlaceholder
		case "UPDATE_URLS":
			domain := strings.TrimSpace(item.val)
			domain = strings.TrimPrefix(domain, "https://")
			domain = strings.TrimPrefix(domain, "http://")
			domain = strings.TrimSuffix(domain, "/")
			if domain == "" {
				continue
			}
			staging := doc.Environments["staging"]
			staging.Domain = domain
			doc.Environments["staging"] = staging
		}
	}
	return unmapped
}

func applyMagentoEnv(doc *mageliftDocument, key, value string) (handled, valid bool) {
	value = strings.TrimSpace(value)
	switch key {
	case "CONFIG__DEFAULT__ADMIN__URL__FRONT_NAME":
		if value == "" || !magentoPathSegment.MatchString(value) {
			return true, false
		}
		magentoOverlay(doc).FrontName = value
	case "CONFIG__DEFAULT__WEB__COOKIE__COOKIE_DOMAIN":
		if value == "" || strings.ContainsAny(value, " \t\r\n/") {
			return true, false
		}
		magentoOverlay(doc).CookieDomain = value
	case "CONFIG__DEFAULT__WEB__CORS__ORIGINS", "CONFIG__DEFAULT__WEB__GRAPHQL__CORS_ORIGINS":
		origins, ok := magentoOrigins(value)
		if !ok {
			return true, false
		}
		magentoOverlay(doc).CORSOrigins = origins
		doc.Application.Mode = "headless"
	case "CONFIG__DEFAULT__CRON__CONSUMERS_RUNNER__MODE":
		if value != "cron" && value != "processes" && value != "both" {
			return true, false
		}
		magentoOverlay(doc).Consumers.Mode = value
	case "CONFIG__DEFAULT__CRON__CONSUMERS_RUNNER__CONSUMERS":
		names, ok := magentoConsumerNames(value)
		if !ok {
			return true, false
		}
		magentoOverlay(doc).Consumers.Names = names
	default:
		return false, false
	}
	return true, true
}

func magentoOverlay(doc *mageliftDocument) *mageliftMagento {
	if doc.Application.Magento == nil {
		doc.Application.Magento = &mageliftMagento{}
	}
	return doc.Application.Magento
}

func magentoOrigins(value string) ([]string, bool) {
	var origins []string
	for _, raw := range strings.Split(value, ",") {
		origin := strings.TrimSpace(raw)
		parsed, err := url.ParseRequestURI(origin)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, false
		}
		origins = append(origins, origin)
	}
	return origins, len(origins) > 0
}

var magentoPathSegment = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

func magentoConsumerNames(value string) ([]string, bool) {
	var names []string
	seen := map[string]struct{}{}
	for _, raw := range strings.Split(value, ",") {
		name := strings.TrimSpace(raw)
		if name == "" || strings.ContainsAny(name, " \t\r\n") {
			return nil, false
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	return names, len(names) > 0
}

func applyObservabilityEnv(doc *mageliftDocument, key, value string) (handled, residual bool) {
	key = strings.ToUpper(strings.TrimSpace(key))
	provider := ""
	switch key {
	case "NEW_RELIC_LICENSE_KEY", "NEW_RELIC_API_KEY":
		provider = "newrelic"
	case "DATADOG_API_KEY", "DD_API_KEY":
		provider = "datadog"
	case "NEW_RELIC_APP_NAME":
		provider = "newrelic"
	case "DD_SERVICE":
		provider = "datadog"
	case "NEW_RELIC_ENVIRONMENT", "DD_ENV":
		provider = "newrelic"
	case "OTEL_EXPORTER_OTLP_ENDPOINT":
		provider = "otlp"
	case "OTEL_SERVICE_NAME":
		provider = "otlp"
	default:
		return false, false
	}
	if doc.Observability == nil {
		doc.Observability = &mageliftObservability{ExternalProvider: provider, Logs: true}
	}
	if doc.Observability.ExternalProvider == "" {
		doc.Observability.ExternalProvider = provider
	}
	if doc.Observability.ExternalProvider != provider {
		return true, true
	}
	switch key {
	case "NEW_RELIC_LICENSE_KEY", "NEW_RELIC_API_KEY", "DATADOG_API_KEY", "DD_API_KEY":
		return true, true
	case "NEW_RELIC_APP_NAME", "DD_SERVICE", "OTEL_SERVICE_NAME":
		doc.Observability.ServiceName = strings.TrimSpace(value)
	case "NEW_RELIC_ENVIRONMENT", "DD_ENV":
		doc.Observability.Environment = strings.TrimSpace(value)
	case "OTEL_EXPORTER_OTLP_ENDPOINT":
		doc.Observability.Endpoint = strings.TrimSpace(value)
		doc.Observability.Metrics = true
		doc.Observability.Traces = true
	}
	if provider == "newrelic" || provider == "datadog" {
		doc.Observability.Metrics = true
		doc.Observability.Traces = true
	}
	return true, false
}

func baseDocument(name, php string) mageliftDocument {
	return mageliftDocument{
		SchemaVersion: 1,
		Project:       mageliftProject{Name: name},
		Application: mageliftApplication{
			Edition: "open-source",
			Version: "2.4.9",
			Mode:    "integrated",
		},
		Build: mageliftBuild{PHP: php},
		Target: mageliftTarget{
			Provider: "aws",
			Runtime:  "ecs-fargate",
		},
		Defaults: mageliftDefaults{
			Region: "eu-west-3",
			Preset: "preview",
		},
		Environments: map[string]mageliftEnv{
			"staging": {Account: "123456789012"},
		},
		Extensions: map[string]any{},
	}
}

var qualityPatchID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func qualityPatchIDs(value any) ([]string, bool) {
	var raw []any
	switch typed := value.(type) {
	case []any:
		raw = typed
	case []string:
		ids := make([]string, 0, len(typed))
		seen := map[string]struct{}{}
		for _, id := range typed {
			id = strings.TrimSpace(id)
			if !qualityPatchID.MatchString(id) {
				return nil, false
			}
			if _, exists := seen[id]; exists {
				return nil, false
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
		return ids, true
	default:
		return nil, false
	}
	ids := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, item := range raw {
		id, ok := item.(string)
		if !ok {
			return nil, false
		}
		id = strings.TrimSpace(id)
		if !qualityPatchID.MatchString(id) {
			return nil, false
		}
		if _, exists := seen[id]; exists {
			return nil, false
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, true
}
