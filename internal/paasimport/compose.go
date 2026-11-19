package paasimport

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
)

var composeImageVersion = regexp.MustCompile(`^([0-9]+(\.[0-9]+)*)(-[a-zA-Z][a-zA-Z0-9.-]*)?$`)

// ApplyCompose reads service image tags without evaluating Compose interpolation,
// build instructions, environment files, or commands from the shop.
func ApplyCompose(root string, project map[string]any) ([]UnmappedKey, error) {
	var compose struct {
		Services map[string]struct {
			Image   string         `yaml:"image"`
			Options map[string]any `yaml:",inline"`
		} `yaml:"services"`
	}
	var file string
	for _, path := range []string{"compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"} {
		err := readSourceFile(root, path, &compose)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		file = path
		break
	}
	if file == "" || len(compose.Services) == 0 {
		return nil, fmt.Errorf("no Compose services found in the shop root")
	}
	seen := map[string]bool{}
	services := ensureMap(project, "services")
	provider := project["target"].(map[string]any)["provider"].(string)
	var left []UnmappedKey
	for _, name := range slices.Sorted(maps.Keys(compose.Services)) {
		service := compose.Services[name]
		image, _, _ := strings.Cut(service.Image, "@")
		image = strings.TrimPrefix(strings.TrimPrefix(image, "docker.io/"), "library/")
		repo, tag, _ := strings.Cut(image, ":")
		family := map[string]string{"mysql": "mysql", "mariadb": "mariadb", "redis": "redis", "valkey/valkey": "valkey", "rabbitmq": "rabbitmq", "opensearchproject/opensearch": "opensearch"}[repo]
		knownRole := family != "" || serviceRole(name) != "" || slices.Contains([]string{"search", "database", "db", "cache", "session", "queue"}, name)
		if knownRole {
			for _, key := range []string{"build", "command", "entrypoint", "env_file", "environment", "volumes", "configs", "secrets", "deploy"} {
				if _, ok := service.Options[key]; ok {
					left = append(left, UnmappedKey{Source: file, Path: filePath(name, key)})
				}
			}
		}
		if family == "" {
			// A local Dockerfile or private image may carry shop-specific plugins.
			if knownRole {
				left = append(left, UnmappedKey{filePath(name, "image"), file})
			}
			continue
		}
		m := composeImageVersion.FindStringSubmatch(tag)
		if m == nil {
			left = append(left, UnmappedKey{filePath(name, "image"), file})
			continue
		}
		source := map[string]any{"type": family + ":" + m[1]}
		app := accAppRaw{"relationships": map[string]any{name: name}}
		b, role, _ := importService(name, source, app, provider, project)
		if seen[role] {
			left = append(left, UnmappedKey{filePath(name, "image"), file})
			continue
		}
		seen[role] = true
		if b["ownership"] == "self-hosted" {
			b["image"] = service.Image
		}
		services[role] = b
	}
	sortUnmapped(left)
	return left, nil
}

func filePath(service, key string) string { return "services." + service + "." + key }
