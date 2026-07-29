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
		Build  map[string]string `yaml:"build"`
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
	var unmapped []UnmappedKey

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
		case "name", "type", "relationships":
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
			// Magento cron:run is consumed; schema destination lands in Task 2.
			_ = doc
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
	default:
		return false
	}
}

func serviceMapped(name, typeValue string) bool {
	return relationshipMapped(name, typeValue)
}

func sortUnmapped(keys []UnmappedKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Source != keys[j].Source {
			return keys[i].Source < keys[j].Source
		}
		return keys[i].Path < keys[j].Path
	})
}
