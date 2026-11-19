package paasimport

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

type Source struct {
	AppRoot                                  string
	appFile, serviceFile, routeFile, envFile string
	app                                      accAppRaw
	multiApp                                 bool
	services                                 map[string]any
	serviceFiles, routeFiles                 map[string]string
	routes                                   map[string]accRoute
	env                                      accEnvDocument
	magentoEnv                               accEnvDocument
	magentoEnvPath                           string
	unmapped                                 []UnmappedKey
}

func readSourceFile(root, path string, dest any) error {
	data, err := readUnder(root, path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, dest); err != nil {
		return fmt.Errorf("invalid configuration in %s", path)
	}
	return nil
}

func Read(root string, kind SourceKind, selected string) (*Source, error) {
	s := &Source{}
	switch kind {
	case SourceACC:
		s.appFile, s.serviceFile, s.routeFile, s.envFile = accAppFile, accServicesFile, accRoutesFile, accEnvFile
	case SourceUpsun:
		if err := DetectUpsun(root); err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(root, ".upsun")); err == nil {
			if err := s.readModernUpsun(root, selected); err != nil {
				return nil, err
			}
			if err := s.readMagentoStageFile(root); err != nil {
				return nil, err
			}
			return s, nil
		}
		s.appFile, s.serviceFile, s.routeFile, s.envFile = upsunAppFile, upsunServicesFile, upsunRoutesFile, upsunEnvFile
		const global = ".platform/applications.yaml"
		var applications yaml.Node
		if err := readSourceFile(root, global, &applications); err == nil {
			named, err := fixedApplications(global, applications)
			if err != nil {
				return nil, err
			}
			if err := s.selectApp(named, selected); err != nil {
				return nil, err
			}
			s.appFile = global
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown PaaS source %q", kind)
	}
	if s.app == nil {
		if err := readSourceFile(root, s.appFile, &s.app); err != nil {
			return nil, err
		}
		if selected != "" && s.app["name"] != selected {
			return nil, fmt.Errorf("application %q not found in %s", selected, s.appFile)
		}
	}
	for _, input := range []struct {
		path  string
		value any
	}{{s.serviceFile, &s.services}, {s.routeFile, &s.routes}, {s.envFile, &s.env}} {
		if err := readSourceFile(root, input.path, input.value); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if kind == SourceUpsun {
		if err := s.readMagentoStageFile(root); err != nil {
			return nil, err
		}
		return s, nil
	}
	return s, nil
}

func (s *Source) readMagentoStageFile(root string) error {
	s.magentoEnvPath = filepath.Join(s.AppRoot, accEnvFile)
	err := readSourceFile(root, s.magentoEnvPath, &s.magentoEnv)
	if errors.Is(err, os.ErrNotExist) {
		s.magentoEnvPath = ""
		return nil
	}
	return err
}

func fixedApplications(file string, document yaml.Node) (map[string]accAppRaw, error) {
	named := map[string]accAppRaw{}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("%s requires named applications", file)
	}
	node := document.Content[0]
	if node.Kind == yaml.SequenceNode {
		var apps []accAppRaw
		if err := node.Decode(&apps); err != nil {
			return nil, fmt.Errorf("invalid applications in %s", file)
		}
		for _, app := range apps {
			name, _ := app["name"].(string)
			if name == "" || named[name] != nil {
				return nil, fmt.Errorf("%s requires unique application names", file)
			}
			named[name] = app
		}
	} else if node.Kind != yaml.MappingNode || node.Decode(&named) != nil {
		return nil, fmt.Errorf("invalid applications in %s", file)
	}
	return named, nil
}

func (s *Source) selectApp(apps map[string]accAppRaw, selected string) error {
	if selected == "" && len(apps) == 1 {
		for name := range apps {
			selected = name
		}
	}
	app, exists := apps[selected]
	if !exists || app == nil {
		return fmt.Errorf("select a Magento application with --app (available: %s)", strings.Join(slices.Sorted(maps.Keys(apps)), ", "))
	}
	app["name"] = selected
	s.app = app
	s.multiApp = len(apps) > 1
	if source, ok := asStringMap(app["source"]); ok {
		if root, ok := source["root"].(string); ok && root != "" {
			if !filepath.IsLocal(root) {
				return errors.New("application source.root must be relative to the repository")
			}
			s.AppRoot = root
			delete(source, "root")
		}
		if len(source) == 0 {
			delete(app, "source")
		}
	}
	return nil
}

func (s *Source) readModernUpsun(root, selected string) error {
	files, err := os.ReadDir(filepath.Join(root, ".upsun"))
	if err != nil {
		return err
	}
	apps := map[string]accAppRaw{}
	appFiles := map[string]string{}
	s.services = map[string]any{}
	s.serviceFiles = map[string]string{}
	s.routeFiles = map[string]string{}
	s.routes = map[string]accRoute{}
	for _, file := range files {
		if file.IsDir() || (filepath.Ext(file.Name()) != ".yaml" && filepath.Ext(file.Name()) != ".yml") {
			continue
		}
		path := filepath.Join(".upsun", file.Name())
		var doc map[string]yaml.Node
		if err := readSourceFile(root, path, &doc); err != nil {
			return err
		}
		for section, node := range doc {
			switch section {
			case "applications":
				var entries map[string]accAppRaw
				if err := node.Decode(&entries); err != nil {
					return fmt.Errorf("invalid applications in %s", path)
				}
				for name, app := range entries {
					if app == nil || apps[name] != nil {
						return fmt.Errorf("empty or duplicate application %q in %s", name, path)
					}
					apps[name], appFiles[name] = app, path
				}
			case "services":
				var entries map[string]any
				if err := node.Decode(&entries); err != nil {
					return fmt.Errorf("invalid services in %s", path)
				}
				for name, v := range entries {
					if _, ok := s.services[name]; ok {
						return fmt.Errorf("duplicate service %q in %s", name, path)
					}
					s.services[name], s.serviceFiles[name] = v, path
				}
			case "routes":
				var entries map[string]accRoute
				if err := node.Decode(&entries); err != nil {
					return fmt.Errorf("invalid routes in %s", path)
				}
				for name, v := range entries {
					if _, ok := s.routes[name]; ok {
						return fmt.Errorf("duplicate route in %s", path)
					}
					s.routes[name], s.routeFiles[name] = v, path
				}
			default:
				s.unmapped = append(s.unmapped, UnmappedKey{Source: path, Path: section})
			}
		}
	}
	if err := s.selectApp(apps, selected); err != nil {
		return err
	}
	s.appFile = appFiles[s.app["name"].(string)]
	return nil
}

func (s *Source) Apply(project map[string]any) ([]UnmappedKey, error) {
	app := maps.Clone(s.app)
	residuals := slices.Clone(s.unmapped)
	// Expanded Upsun relationships identify the service and optional endpoint.
	linked := map[string]bool{}
	if list, ok := app["relationships"].([]any); ok {
		rels := map[string]any{}
		for _, v := range list {
			if id, ok := v.(string); ok {
				rels[id] = id
			} else {
				residuals = append(residuals, UnmappedKey{Source: s.appFile, Path: "relationships"})
			}
		}
		app["relationships"] = rels
	}
	if rels, ok := asStringMap(app["relationships"]); ok {
		normalized := map[string]any{}
		for name, r := range rels {
			if r == nil {
				linked[name] = true
				normalized[name] = name
				continue
			}
			if v, ok := asStringMap(r); ok {
				id, _ := v["service"].(string)
				if id != "" {
					linked[id] = true
					normalized[name] = id
					if endpoint, ok := v["endpoint"].(string); ok && endpoint != "" {
						normalized[name] = id + ":" + endpoint
					}
					for field := range v {
						if field != "service" && field != "endpoint" {
							residuals = append(residuals, UnmappedKey{Source: s.appFile, Path: "relationships." + name + "." + field})
						}
					}
					continue
				}
			} else if v, ok := r.(string); ok {
				id, _, _ := strings.Cut(v, ":")
				linked[id] = true
			}
			normalized[name] = r
		}
		app["relationships"] = normalized
	}
	if vars, ok := asStringMap(app["variables"]); ok {
		if env, ok := asStringMap(vars["env"]); ok {
			if s.env.Variables.Env == nil {
				s.env.Variables.Env = map[string]string{}
			}
			for name, v := range env {
				if value, ok := v.(string); ok {
					s.env.Variables.Env[name] = value
				} else {
					residuals = append(residuals, UnmappedKey{Source: s.appFile, Path: "variables.env." + name})
				}
			}
		}
		for k := range vars {
			if k != "env" {
				residuals = append(residuals, UnmappedKey{Source: s.appFile, Path: "variables." + k})
			}
		}
		delete(app, "variables")
	}
	doc, extra, err := mapAppTree(s.appFile, app)
	if err != nil {
		return nil, err
	}
	residuals = append(residuals, extra...)
	envFile := s.envFile
	if envFile == "" {
		envFile = s.appFile
	}
	residuals = append(residuals, applyAllowlistEnv(&doc, envFile, s.env)...)
	if s.magentoEnvPath != "" {
		residuals = append(residuals, applyAllowlistEnv(&doc, s.magentoEnvPath, s.magentoEnv)...)
	}
	env := project["environments"].(map[string]any)["staging"].(map[string]any)
	// Keep routes pointing at the selected Magento application in multi-app projects.
	routes := map[string]accRoute{}
	for name, route := range s.routes {
		upstream, _, _ := strings.Cut(route.Upstream, ":")
		if upstream == app["name"] || (!s.multiApp && (upstream == "" || len(s.routeFiles) == 0)) {
			routes[name] = route
			file := s.routeFile
			if s.routeFiles[name] != "" {
				file = s.routeFiles[name]
			}
			if strings.ContainsAny(name, "{}*") {
				residuals = append(residuals, UnmappedKey{Source: file, Path: "routes.placeholder"})
			}
			if route.Type == "redirect" {
				residuals = append(residuals, UnmappedKey{Source: file, Path: "routes.redirect"})
			}
		}
	}
	domain := doc.Environments["staging"].Domain
	if domain == "" {
		domain = domainFromRoutes(routes)
	}
	if domain != "" {
		env["domain"] = domain
	}
	encoded, err := emitMagelift(doc)
	if err != nil {
		return nil, err
	}
	var imported map[string]any
	if err := yaml.Unmarshal(encoded, &imported); err != nil {
		return nil, err
	}
	for _, key := range []string{"magento", "cron"} {
		if v, ok := imported["application"].(map[string]any)[key]; ok {
			if key == "magento" {
				maps.Copy(ensureMap(project["application"].(map[string]any), key), v.(map[string]any))
				continue
			}
			project["application"].(map[string]any)[key] = v
		}
	}
	if doc.Application.Mode == "headless" {
		project["application"].(map[string]any)["mode"] = "headless"
	}
	build := project["build"].(map[string]any)
	for key, v := range imported["build"].(map[string]any) {
		if key == "staticContent" && build[key] != nil {
			settings := build[key].(map[string]any)
			incoming := v.(map[string]any)
			if incoming["matrix"] != nil {
				delete(settings, "themes")
				delete(settings, "locales")
			}
			maps.Copy(settings, incoming)
		} else if key == "composer" && build["composer"] != nil {
			maps.Copy(build[key].(map[string]any), v.(map[string]any))
		} else {
			build[key] = v
		}
	}
	if doc.Observability != nil {
		project["observability"] = imported["observability"]
	}
	target := project["target"].(map[string]any)
	provider := target["provider"].(string)
	block := target[provider].(map[string]any)
	if doc.Target.AWS != nil && doc.Target.AWS.EncryptionKeySecretARN != "" {
		switch provider {
		case "aws":
			block["encryptionKeySecretArn"] = defaultEncryptionKeyPlaceholder
		case "gcp":
			block["encryptionKeySecret"] = "magelift-encryption-key"
		case "azure":
			block["encryptionKeySecretName"] = "magelift-source-crypt-key"
		default:
			residuals = append(residuals, UnmappedKey{Source: envFile, Path: "CRYPT_KEY"})
		}
	}
	importedRoles := map[string]bool{}
	for _, id := range slices.Sorted(maps.Keys(s.services)) {
		if (s.multiApp || len(s.serviceFiles) > 0 && len(linked) > 0) && !linked[id] {
			continue
		}
		service := s.services[id]
		file := s.serviceFile
		if s.serviceFiles[id] != "" {
			file = s.serviceFiles[id]
		}
		if edge, unmapped := fastlyEdgeIntent(file, map[string]any{id: service}); edge != nil {
			intent := map[string]any{"externalProvider": "fastly", "tls": true}
			if domain != "" {
				intent["domains"] = []string{domain}
			} else {
				residuals = append(residuals, UnmappedKey{Source: file, Path: id + ".domains"})
			}
			project["edge"] = intent
			residuals = append(residuals, unmapped...)
			continue
		}
		if obs := observabilityServiceIntent(map[string]any{id: service}); obs != nil {
			if project["observability"] == nil {
				project["observability"] = map[string]any{"externalProvider": obs.ExternalProvider, "logs": obs.Logs, "traces": obs.Traces, "metrics": obs.Metrics}
			}
			continue
		}
		binding, role, options := importService(id, service, app, provider, project)
		if role == "" || importedRoles[role] {
			residuals = append(residuals, UnmappedKey{Source: file, Path: id})
			continue
		}
		importedRoles[role] = true
		ensureMap(project, "services")[role] = binding
		for _, path := range options {
			residuals = append(residuals, UnmappedKey{Source: file, Path: id + "." + path})
		}
	}
	sortUnmapped(residuals)
	return residuals, nil
}

func ensureMap(m map[string]any, k string) map[string]any {
	v, ok := m[k].(map[string]any)
	if !ok {
		v = map[string]any{}
		m[k] = v
	}
	return v
}
func serviceRole(family string) string {
	switch family {
	case "mysql", "mariadb":
		return "database"
	case "redis", "valkey":
		return "cache"
	case "opensearch", "elasticsearch":
		return "search"
	case "rabbitmq", "activemq", "artemis":
		return "queue"
	}
	return ""
}
