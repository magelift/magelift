// Package searchplugins prepares the shop's OpenSearch image for its analyzers.
package searchplugins

import (
	"fmt"
	"regexp"
)

var pluginName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

func Validate(plugins []string) error {
	seen := make(map[string]bool, len(plugins))
	for _, plugin := range plugins {
		if !pluginName.MatchString(plugin) || seen[plugin] {
			return fmt.Errorf("invalid or repeated OpenSearch plugin name %q", plugin)
		}
		seen[plugin] = true
	}
	return nil
}

// Entrypoint installs missing named plugins before starting the upstream container.
// A shop image with the plugins already installed also works without network access.
func Entrypoint(plugins []string) []string {
	if len(plugins) == 0 {
		return nil
	}
	return append([]string{"/bin/sh", "-ec", `cd /usr/share/opensearch
for plugin do
    if [ ! -f "plugins/$plugin/plugin-descriptor.properties" ]; then
        bin/opensearch-plugin install --batch "$plugin"
    fi
done
exec ./opensearch-docker-entrypoint.sh opensearch`, "magelift-search"}, plugins...)
}
