package paasimport

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/searchplugins"
	"github.com/magelift/magelift/sdk"
)

func importService(id string, raw any, app accAppRaw, provider string, project map[string]any) (map[string]any, string, []string) {
	family, version, _ := strings.Cut(serviceType(raw), ":")
	persistent := family == "redis-persistent" || family == "valkey-persistent"
	family = strings.TrimSuffix(family, "-persistent")
	if family == "oracle-mysql" {
		family = "mysql"
	}
	role := serviceRole(family)
	if role == "" || version == "" {
		return nil, "", nil
	}
	if role == "cache" && storeRole(id, app, persistent) == "session" {
		role = "session"
	}
	binding := map[string]any{"family": family, "version": version}
	kube := provider == "gcp" || provider == "azure"
	if kube && (role == "cache" || role == "session" || role == "search" || (role == "queue" && family == "rabbitmq") || (role == "database" && family == "mariadb")) {
		binding["ownership"] = "self-hosted"
	}
	if role == "search" {
		// Composer metadata can identify analyzers needed in addition to the source declarations.
		existing, _ := ensureMap(project, "services")[role].(map[string]any)
		if family == "opensearch" && existing["plugins"] != nil {
			binding["plugins"] = existing["plugins"]
		}
		if !strings.Contains(version, ".") {
			branch, _ := existing["version"].(string)
			if strings.HasPrefix(branch, version+".") {
				binding["version"] = branch
			}
		}
	}
	var residual []string
	fields, _ := asStringMap(raw)
	for _, key := range slices.Sorted(maps.Keys(fields)) {
		value := fields[key]
		switch key {
		case "type":
		case "disk":
			// Upsun specifies disk in MiB. Kubernetes requests whole GiB.
			if size, ok := value.(int); ok && size > 0 && binding["ownership"] == "self-hosted" {
				binding["storageGiB"] = 1 + (size-1)/1024
			} else {
				residual = append(residual, key)
			}
		case "configuration":
			settings, ok := asStringMap(value)
			if !ok {
				residual = append(residual, key)
				continue
			}
			for _, option := range slices.Sorted(maps.Keys(settings)) {
				if option == "properties" && role == "database" && (provider == "aws" || binding["ownership"] == "self-hosted") {
					if props, ok := asStringMap(settings[option]); ok {
						parameters := map[string]string{}
						for _, original := range slices.Sorted(maps.Keys(props)) {
							param := map[string]string{"default_charset": "character_set_server", "default_collation": "collation_server", "max_allowed_packet": "max_allowed_packet"}[original]
							value := fmt.Sprint(props[original])
							if param == "max_allowed_packet" {
								value = packetBytes(value)
							}
							if param == "" || sdk.ValidateDatabaseParameters(map[string]string{param: value}) != nil {
								residual = append(residual, key+"."+option+"."+original)
								continue
							}
							parameters[param] = value
						}
						if len(parameters) > 0 {
							binding["parameters"] = parameters
						}
						continue
					}
				}
				if option == "maxmemory_policy" && (role == "cache" || role == "session") {
					if v, ok := settings[option].(string); ok && slices.Contains([]string{"noeviction", "allkeys-lru", "allkeys-lfu", "allkeys-random", "volatile-lru", "volatile-lfu", "volatile-random", "volatile-ttl"}, v) {
						binding["evictionPolicy"] = v
						continue
					}
				}
				if option == "plugins" && family == "opensearch" {
					if plugins, ok := stringList(settings[option]); ok && searchplugins.Validate(plugins) == nil {
						current, _ := binding["plugins"].([]string)
						for _, name := range plugins {
							if !slices.Contains(current, name) {
								current = append(current, name)
							}
						}
						binding["plugins"] = current
						continue
					}
				}
				residual = append(residual, key+"."+option)
			}
		default:
			residual = append(residual, key)
		}
	}
	return binding, role, residual
}

func storeRole(id string, app accAppRaw, persistent bool) string {
	relationships, _ := asStringMap(app["relationships"])
	var explicitCache, explicitSession bool
	for alias, raw := range relationships {
		v, _ := raw.(string)
		name, _, _ := strings.Cut(v, ":")
		if name != id {
			continue
		}
		switch strings.ToLower(alias) {
		case "session", "sessions", "redis-session", "valkey-session", "redis_session", "valkey_session":
			explicitSession = true
		case "cache", "redis", "valkey":
			explicitCache = true
		}
	}
	if (persistent || explicitSession) && !explicitCache {
		return "session"
	}
	return "cache"
}

func packetBytes(value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	for _, suffix := range []struct {
		unit       string
		multiplier uint64
	}{{"K", 1 << 10}, {"M", 1 << 20}, {"G", 1 << 30}} {
		if digits, ok := strings.CutSuffix(strings.TrimSuffix(v, "B"), suffix.unit); ok {
			n, err := strconv.ParseUint(digits, 10, 32)
			if err != nil || n > (1<<30)/suffix.multiplier {
				return ""
			}
			return strconv.FormatUint(n*suffix.multiplier, 10)
		}
	}
	return v
}
