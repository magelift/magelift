package naming

import (
	"fmt"
	"regexp"
	"strings"
)

var nonName = regexp.MustCompile(`[^a-z0-9-]+`)

// Resource builds a stable GCP resource name from project and environment.
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

// LabelValue sanitizes a label value for GCP resource labels.
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

// ClusterName returns the Autopilot cluster name.
func ClusterName(project, environment string) string {
	return Resource(project, environment, "gke")
}

// FormatURL builds an https URL from a load balancer address.
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
