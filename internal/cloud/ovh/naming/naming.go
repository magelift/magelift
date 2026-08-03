package naming

import (
	"fmt"
	"regexp"
	"strings"
)

var nonName = regexp.MustCompile(`[^a-z0-9-]+`)

// Resource builds a stable OVH resource name from project and environment.
func Resource(project, environment, suffix string) string {
	base := sanitize(project) + "-" + sanitize(environment)
	if suffix != "" {
		base += "-" + sanitize(suffix)
	}
	if len(base) > 63 {
		base = base[:63]
		base = strings.TrimRight(base, "-")
	}
	return base
}

func LabelValue(value string) string {
	return sanitize(value)
}

func sanitize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = nonName.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "magelift"
	}
	return value
}

func ClusterName(project, environment string) string {
	return Resource(project, environment, "mks")
}

func FormatURL(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if strings.HasPrefix(host, "http://") || strings.HasPrefix(host, "https://") {
		return host
	}
	return fmt.Sprintf("https://%s", host)
}
