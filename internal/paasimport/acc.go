package paasimport

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"go.yaml.in/yaml/v4"
)

type accAppRaw map[string]any

type accRoute struct {
	Type     string `yaml:"type"`
	Upstream string `yaml:"upstream"`
	To       string `yaml:"to"`
}

type accEnvDocument struct {
	Stage struct {
		Global map[string]string `yaml:"global"`
		Deploy map[string]string `yaml:"deploy"`
		Build  map[string]any    `yaml:"build"`
	} `yaml:"stage"`
	Variables struct {
		Env map[string]string `yaml:"env"`
	} `yaml:"variables"`
}

type accCron struct {
	Spec string `yaml:"spec"`
	Cmd  string `yaml:"cmd"`
}

// MapACC reads an ACC config root and returns MageLift YAML plus residual keys.
func MapACC(root string) (Result, error) {
	if err := DetectACC(root); err != nil {
		return Result{}, err
	}

	appData, err := readUnder(root, accAppFile)
	if err != nil {
		return Result{}, err
	}
	var raw accAppRaw
	if err := yaml.Unmarshal(appData, &raw); err != nil {
		return Result{}, fmt.Errorf("parse %s: %w", accAppFile, err)
	}

	doc, unmapped, err := mapAppTree(accAppFile, raw)
	if err != nil {
		return Result{}, err
	}

	if servicesData, err := readUnder(root, accServicesFile); err == nil {
		var services map[string]any
		if err := yaml.Unmarshal(servicesData, &services); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", accServicesFile, err)
		}
		var fastlyUnmapped []UnmappedKey
		doc.Edge, fastlyUnmapped = fastlyEdgeIntent(accServicesFile, services)
		unmapped = append(unmapped, fastlyUnmapped...)
		if observability := observabilityServiceIntent(services); observability != nil {
			doc.Observability = observability
		}
		unmapped = append(unmapped, mapServices(accServicesFile, services)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	if routesData, err := readUnder(root, accRoutesFile); err == nil {
		var routes map[string]accRoute
		if err := yaml.Unmarshal(routesData, &routes); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", accRoutesFile, err)
		}
		staging := doc.Environments["staging"]
		if staging.Domain == "" {
			staging.Domain = domainFromRoutes(routes)
		}
		doc.Environments["staging"] = staging
		if doc.Edge != nil && staging.Domain != "" {
			doc.Edge.Domains = []string{staging.Domain}
		}
		// All route patterns are structural / consumed.
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	if envData, err := readUnder(root, accEnvFile); err == nil {
		var env accEnvDocument
		if err := yaml.Unmarshal(envData, &env); err != nil {
			return Result{}, fmt.Errorf("parse %s: %w", accEnvFile, err)
		}
		unmapped = append(unmapped, applyAllowlistEnv(&doc, accEnvFile, env)...)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Result{}, err
	}

	sortUnmapped(unmapped)
	yamlBytes, err := emitMagelift(doc)
	if err != nil {
		return Result{}, err
	}
	return Result{YAML: yamlBytes, Unmapped: unmapped}, nil
}

func mapAppTree(source string, raw accAppRaw) (mageliftDocument, []UnmappedKey, error) {
	name, _ := raw["name"].(string)
	typeValue, _ := raw["type"].(string)
	if strings.TrimSpace(name) == "" {
		return mageliftDocument{}, nil, fmt.Errorf("%s: name is required", source)
	}
	php, err := parsePHPType(typeValue)
	if err != nil {
		return mageliftDocument{}, nil, fmt.Errorf("%s: %w", source, err)
	}

	doc := baseDocument(name, php)
	unmapped := mapBuildRequirements(source, raw, &doc)

	if relsRaw, ok := raw["relationships"]; ok {
		rels, ok := asStringMap(relsRaw)
		if !ok {
			unmapped = append(unmapped, UnmappedKey{Source: source, Path: "relationships"})
		} else {
			relStrings := map[string]string{}
			for k, v := range rels {
				s, ok := v.(string)
				if !ok {
					unmapped = append(unmapped, UnmappedKey{Source: source, Path: "relationships." + k})
					continue
				}
				relStrings[k] = s
				if !relationshipMapped(k, s) {
					unmapped = append(unmapped, UnmappedKey{Source: source, Path: "relationships." + k})
				}
			}
			if catalog := catalogFromRelationships(relStrings); catalog != nil {
				if doc.Target.AWS == nil {
					doc.Target.AWS = &mageliftAWS{}
				}
				doc.Target.AWS.Catalog = catalog
			}
		}
	}

	for key, value := range raw {
		switch key {
		case "name", "type", "relationships", "runtime", "dependencies":
			continue
		case "crons":
			unmapped = append(unmapped, mapCrons(source, value, &doc)...)
		case "hooks":
			unmapped = append(unmapped, mapHooks(source, value)...)
		default:
			unmapped = append(unmapped, UnmappedKey{Source: source, Path: key})
		}
	}
	return doc, unmapped, nil
}

func mapBuildRequirements(source string, raw accAppRaw, doc *mageliftDocument) []UnmappedKey {
	var unmapped []UnmappedKey
	if value, ok := raw["runtime"]; ok {
		runtime, ok := asStringMap(value)
		if !ok {
			unmapped = append(unmapped, UnmappedKey{Source: source, Path: "runtime"})
		} else {
			for key, value := range runtime {
				switch key {
				case "extensions":
					extensions, ok := stringList(value)
					if !ok {
						unmapped = append(unmapped, UnmappedKey{Source: source, Path: "runtime.extensions"})
						continue
					}
					doc.Build.Extensions = extensions
				case "disabled_extensions":
					unmapped = append(unmapped, UnmappedKey{Source: source, Path: "runtime.disabled_extensions"})
				default:
					unmapped = append(unmapped, UnmappedKey{Source: source, Path: "runtime." + key})
				}
			}
		}
	}
	if value, ok := raw["dependencies"]; ok {
		dependencies, ok := asStringMap(value)
		if !ok {
			unmapped = append(unmapped, UnmappedKey{Source: source, Path: "dependencies"})
			return unmapped
		}
		for name, value := range dependencies {
			if name != "php" {
				unmapped = append(unmapped, UnmappedKey{Source: source, Path: "dependencies." + name})
				continue
			}
			phpDependencies, ok := asStringMap(value)
			if !ok {
				unmapped = append(unmapped, UnmappedKey{Source: source, Path: "dependencies.php"})
				continue
			}
			for packageName, packageVersion := range phpDependencies {
				if packageName != "composer/composer" {
					unmapped = append(unmapped, UnmappedKey{Source: source, Path: "dependencies.php." + packageName})
					continue
				}
				version, ok := packageVersion.(string)
				if !ok || !validComposerVersion(version) {
					unmapped = append(unmapped, UnmappedKey{Source: source, Path: "dependencies.php.composer/composer"})
					continue
				}
				doc.Build.Composer = &mageliftComposer{Version: strings.TrimSpace(version)}
			}
		}
	}

	return unmapped
}

func mapServices(source string, services map[string]any) []UnmappedKey {
	var unmapped []UnmappedKey
	for name, raw := range services {
		svcType := serviceType(raw)
		if serviceMapped(name, svcType) {
			continue
		}
		unmapped = append(unmapped, UnmappedKey{Source: source, Path: name})
	}
	return unmapped
}

func serviceType(raw any) string {
	switch v := raw.(type) {
	case string:
		return v
	case map[string]any:
		if t, ok := v["type"].(string); ok {
			return t
		}
	}
	return ""
}

func mapHooks(source string, value any) []UnmappedKey {
	hooks, ok := asStringMap(value)
	if !ok {
		return []UnmappedKey{{Source: source, Path: "hooks"}}
	}
	var out []UnmappedKey
	for name := range hooks {
		out = append(out, UnmappedKey{Source: source, Path: "hooks." + name})
	}
	return out
}

func mapCrons(source string, value any, doc *mageliftDocument) []UnmappedKey {
	crons, ok := asStringMap(value)
	if !ok {
		return []UnmappedKey{{Source: source, Path: "crons"}}
	}
	var out []UnmappedKey
	for name, raw := range crons {
		entry, ok := parseCron(raw)
		if !ok {
			out = append(out, UnmappedKey{Source: source, Path: "crons." + name})
			continue
		}
		if isMagentoCron(entry.Cmd) {
			doc.Application.Cron = append(doc.Application.Cron, mageliftCronEntry{
				Schedule: entry.Spec,
				Command:  strings.TrimSpace(entry.Cmd),
			})
			continue
		}
		out = append(out, UnmappedKey{Source: source, Path: "crons." + name})
	}
	return out
}

func parseCron(raw any) (accCron, bool) {
	m, ok := asStringMap(raw)
	if !ok {
		return accCron{}, false
	}
	spec, _ := m["spec"].(string)
	cmd, _ := m["cmd"].(string)
	if cmd == "" {
		cmd, _ = m["command"].(string)
	}
	return accCron{Spec: spec, Cmd: cmd}, cmd != ""
}

// asStringMap normalizes YAML map shapes (named accAppRaw, map[string]any, map[any]any).
func asStringMap(value any) (map[string]any, bool) {
	switch m := value.(type) {
	case accAppRaw:
		return map[string]any(m), true
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, v := range m {
			ks, ok := k.(string)
			if !ok {
				continue
			}
			out[ks] = v
		}
		return out, true
	default:
		return nil, false
	}
}

func stringList(value any) ([]string, bool) {
	var values []any
	switch typed := value.(type) {
	case []any:
		values = typed
	case []string:
		values = make([]any, len(typed))
		for index, item := range typed {
			values[index] = item
		}
	default:
		return nil, false
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		item, ok := value.(string)
		if !ok {
			return nil, false
		}
		item = strings.ToLower(strings.TrimSpace(item))
		if !validPHPExtensionName(item) {
			return nil, false
		}
		if _, exists := seen[item]; exists {
			return nil, false
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	sort.Strings(result)
	return result, true
}

func validPHPExtensionName(value string) bool {
	if value == "" || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if character != '_' && character != '-' && (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func validComposerVersion(value string) bool {
	value = strings.TrimSuffix(strings.TrimSpace(value), "+")
	parts := strings.Split(value, ".")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "2" {
		return false
	}
	for _, part := range parts[1:] {
		if part == "" {
			return false
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return false
			}
		}
	}
	return true
}

func isMagentoCron(cmd string) bool {
	c := strings.ToLower(strings.TrimSpace(cmd))
	return strings.Contains(c, "bin/magento") && strings.Contains(c, "cron:run")
}

func relationshipMapped(name, value string) bool {
	service := strings.ToLower(value)
	key := strings.ToLower(name)
	switch {
	case strings.Contains(service, "mysql") || strings.Contains(service, "mariadb") ||
		key == "database" || key == "db":
		return true
	case strings.Contains(service, "redis") || key == "redis" || key == "cache" || key == "session":
		return true
	case strings.Contains(service, "opensearch") || strings.Contains(service, "elasticsearch") ||
		key == "opensearch" || key == "elasticsearch" || key == "search":
		return true
	case strings.Contains(service, "rabbitmq") || strings.Contains(service, "amqp") ||
		key == "rabbitmq" || key == "amqp" || key == "queue":
		return true
	case strings.Contains(service, "fastly") || strings.Contains(key, "fastly"):
		return true
	case observabilityProvider(key, service) != "":
		return true
	default:
		return false
	}
}

func observabilityProvider(name, typeValue string) string {
	value := strings.ToLower(name + " " + typeValue)
	switch {
	case strings.Contains(value, "newrelic") || strings.Contains(value, "new-relic"):
		return "newrelic"
	case strings.Contains(value, "datadog"):
		return "datadog"
	case strings.Contains(value, "opentelemetry") || strings.Contains(value, "otel"):
		return "otlp"
	default:
		return ""
	}
}

func observabilityServiceIntent(services map[string]any) *mageliftObservability {
	for name, raw := range services {
		provider := observabilityProvider(name, serviceType(raw))
		if provider == "" {
			continue
		}
		return &mageliftObservability{
			ExternalProvider: provider,
			Logs:             true,
			Metrics:          provider == "newrelic" || provider == "datadog",
			Traces:           provider == "newrelic" || provider == "datadog",
		}
	}
	return nil
}

func serviceMapped(name, typeValue string) bool {
	return relationshipMapped(name, typeValue)
}

func fastlyEdgeIntent(source string, services map[string]any) (*mageliftEdge, []UnmappedKey) {
	for name, raw := range services {
		typeValue := strings.ToLower(serviceType(raw))
		key := strings.ToLower(name)
		if !strings.Contains(typeValue, "fastly") && !strings.Contains(key, "fastly") {
			continue
		}
		return &mageliftEdge{ExternalProvider: "fastly", TLS: true}, []UnmappedKey{
			{Source: source, Path: name + ".serviceId"},
			{Source: source, Path: name + ".tokenSecret"},
		}
	}
	return nil, nil
}

func sortUnmapped(keys []UnmappedKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Source != keys[j].Source {
			return keys[i].Source < keys[j].Source
		}
		return keys[i].Path < keys[j].Path
	})
}
