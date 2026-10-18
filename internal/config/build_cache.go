package config

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	registryCacheRepository = regexp.MustCompile(`^[a-z0-9][a-z0-9.:-]*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+$`)
	registryCacheTag        = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
)

// ValidateRegistryCacheReference checks a user-owned registry cache reference.
// It is exported for early CLI validation before a BuildKit invocation.
func ValidateRegistryCacheReference(field, reference string) error {
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return fmt.Errorf("%s is required", field)
	}
	if ref != reference {
		return fmt.Errorf("%s must not contain leading or trailing whitespace", field)
	}
	if strings.Contains(ref, "://") || strings.Contains(ref, "?") || strings.Contains(ref, "#") {
		return fmt.Errorf("%s must not be a URL", field)
	}
	if strings.Contains(ref, "type=") || strings.Contains(ref, "ref=") || strings.Contains(ref, "mode=") {
		return fmt.Errorf("%s must not contain BuildKit cache options", field)
	}
	if strings.Contains(ref, "@") {
		return fmt.Errorf("%s must use an explicit tag, not a digest reference", field)
	}
	if strings.ContainsAny(ref, "\x00\r\n\t ") || strings.Contains(ref, ",") {
		return fmt.Errorf("%s must be a registry reference with an explicit tag", field)
	}
	lastSlash := strings.LastIndexByte(ref, '/')
	lastColon := strings.LastIndexByte(ref, ':')
	if lastSlash < 1 || lastColon <= lastSlash || lastColon == len(ref)-1 {
		return fmt.Errorf("%s must name a registry-qualified repository with an explicit tag", field)
	}
	repository, tag := ref[:lastColon], ref[lastColon+1:]
	host := ref[:strings.IndexByte(ref, '/')]
	if host != "localhost" && !strings.ContainsAny(host, ".:") {
		return fmt.Errorf("%s must name a registry host", field)
	}
	if !registryCacheRepository.MatchString(repository) || !registryCacheTag.MatchString(tag) {
		return fmt.Errorf("%s has an invalid repository or tag", field)
	}
	return nil
}

func validateBuildCache(cache *BuildCache) []string {
	if cache == nil {
		return nil
	}
	var problems []string
	export := strings.TrimSpace(cache.To)
	mode := strings.TrimSpace(cache.Mode)
	if mode != "" && export == "" {
		problems = append(problems, "build.cache.mode requires build.cache.to")
	}
	if cache.IgnoreExportErrors && export == "" {
		problems = append(problems, "build.cache.ignoreExportErrors requires build.cache.to")
	}
	if mode != "" && mode != "min" && mode != "max" {
		problems = append(problems, "build.cache.mode must be min or max")
	}
	seen := make(map[string]struct{}, len(cache.From))
	for index, reference := range cache.From {
		field := fmt.Sprintf("build.cache.from[%d]", index)
		if err := ValidateRegistryCacheReference(field, reference); err != nil {
			problems = append(problems, err.Error())
			continue
		}
		if _, exists := seen[reference]; exists {
			problems = append(problems, fmt.Sprintf("build.cache.from cannot contain duplicate %q", reference))
			continue
		}
		seen[reference] = struct{}{}
	}
	if export != "" {
		if err := ValidateRegistryCacheReference("build.cache.to", export); err != nil {
			problems = append(problems, err.Error())
		}
	}
	sort.Strings(problems)
	return problems
}
