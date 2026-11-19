package paasimport

import (
	"encoding/json"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Stage files can contain live credentials. Import known settings and describe the rest by path.
func stageResiduals(source, path string, v any) []UnmappedKey {
	m, ok := asStringMap(v)
	if !ok || len(m) == 0 {
		return []UnmappedKey{{Source: source, Path: path}}
	}
	var keys []UnmappedKey
	for _, key := range slices.Sorted(maps.Keys(m)) {
		keys = append(keys, stageResiduals(source, path+"."+key, m[key])...)
	}
	return keys
}

func mapStageValue(doc *mageliftDocument, stage, key string, v any) (bool, []string) {
	if stage == "build" {
		switch key {
		case "QUALITY_PATCHES":
			ids, ok := qualityPatchIDs(v)
			if !ok {
				return true, []string{""}
			}
			doc.Build.QualityPatches = ids
			return true, nil
		case "SCD_MATRIX":
			matrix, ok := staticMatrix(v)
			if !ok {
				return true, []string{""}
			}
			if doc.Build.StaticContent == nil {
				doc.Build.StaticContent = map[string]any{}
			}
			doc.Build.StaticContent["matrix"] = matrix
			return true, nil
		}
	}
	if stage != "deploy" && stage != "global" {
		return false, nil
	}
	switch key {
	case "CRON_CONSUMERS_RUNNER":
		fields, ok := asStringMap(v)
		if !ok {
			return true, []string{""}
		}
		var left []string
		for name, v := range fields {
			switch name {
			case "cron_run":
				if b, ok := v.(bool); ok {
					if b {
						magentoOverlay(doc).Consumers.Mode = "cron"
					} else {
						magentoOverlay(doc).Consumers.Mode = "disabled"
					}
					continue
				}
			case "max_messages":
				if n, ok := v.(int); ok && n >= 0 {
					addRuntimeOption(doc, []string{"cron_consumers_runner", "max_messages"}, n)
					continue
				}
			case "consumers":
				if names, ok := stringList(v); ok && len(names) == 0 {
					magentoOverlay(doc).Consumers.Names = nil
					continue
				}
				if names, ok := stringList(v); ok {
					if cleaned, valid := magentoConsumerNames(strings.Join(names, ",")); valid {
						magentoOverlay(doc).Consumers.Names = cleaned
						continue
					}
				}
			}
			left = append(left, "."+name)
		}
		return true, left
	case "CONSUMERS_WAIT_FOR_MAX_MESSAGES":
		if b, ok := v.(bool); ok {
			n := 0
			if b {
				n = 1
			}
			addRuntimeOption(doc, []string{"queue", "consumers_wait_for_messages"}, n)
			return true, nil
		}
		return true, []string{""}
	case "SESSION_CONFIGURATION", "CACHE_CONFIGURATION", "QUEUE_CONFIGURATION", "DATABASE_CONFIGURATION":
		fields, ok := asStringMap(v)
		if !ok {
			return true, []string{""}
		}
		if merge, exists := fields["_merge"]; exists && merge != true {
			return true, []string{""}
		}
		var left []string
		for _, k := range slices.Sorted(maps.Keys(fields)) {
			if k == "_merge" {
				continue
			}
			for _, item := range stageResiduals("", "."+k, fields[k]) {
				path := strings.Split(strings.TrimPrefix(item.Path, "."), ".")
				var leaf any = fields
				for _, p := range path {
					m, _ := asStringMap(leaf)
					leaf = m[p]
				}
				if key == "SESSION_CONFIGURATION" && len(path) == 2 && path[0] == "redis" && sessionOption(path[1], leaf) {
					addRuntimeOption(doc, append([]string{"session"}, path...), leaf)
					continue
				}
				left = append(left, item.Path)
			}
		}
		return true, left
	}
	return false, nil
}

var themeCode = regexp.MustCompile(`^[A-Za-z0-9_-]+/[A-Za-z0-9_-]+$`)
var localeCode = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_@-]*$`)

func staticMatrix(v any) (map[string][]string, bool) {
	fields, ok := asStringMap(v)
	if !ok || len(fields) == 0 {
		return nil, false
	}
	out := map[string][]string{}
	for theme, raw := range fields {
		if !themeCode.MatchString(theme) {
			return nil, false
		}
		if m, ok := asStringMap(raw); ok {
			if len(m) != 1 {
				return nil, false
			}
			raw = m["language"]
		}
		locales, ok := stringList(raw)
		if !ok {
			return nil, false
		}
		for _, locale := range locales {
			if !localeCode.MatchString(locale) {
				return nil, false
			}
		}
		out[theme] = locales
	}
	return out, true
}

func sessionOption(k string, v any) bool {
	switch k {
	case "compression_library":
		s, ok := v.(string)
		return ok && slices.Contains([]string{"gzip", "lzf", "lz4", "snappy", "zstd", "none"}, s)
	case "timeout", "compression_threshold", "log_level", "max_concurrency", "break_after_frontend", "break_after_adminhtml", "first_lifetime", "bot_first_lifetime", "bot_lifetime", "disable_locking", "min_lifetime", "max_lifetime":
		switch n := v.(type) {
		case int:
			return n >= 0
		case float64:
			return n >= 0
		case bool:
			return k == "disable_locking"
		case string:
			return numericOption.MatchString(n)
		}
	}
	return false
}

var numericOption = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

func addRuntimeOption(doc *mageliftDocument, path []string, value any) {
	overlay := magentoOverlay(doc)
	if overlay.Variables == nil {
		overlay.Variables = map[string]string{}
	}
	const key = "MAGENTO_DC__OVERRIDE"
	base := map[string]any{}
	if prior := overlay.Variables[key]; prior != "" {
		_ = json.Unmarshal([]byte(prior), &base)
	}
	child := base
	for _, name := range path[:len(path)-1] {
		child = ensureMap(child, name)
	}
	child[path[len(path)-1]] = value
	encoded, _ := json.Marshal(base)
	overlay.Variables[key] = string(encoded)
}
