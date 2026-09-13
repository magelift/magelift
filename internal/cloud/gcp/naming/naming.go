package naming

import (
	"fmt"
	"regexp"
	"strings"
)

var nonName = regexp.MustCompile(`[^a-z0-9-]+`)

// CloudSQLInstance is the Cloud SQL instance name used by the GCP stack.
func CloudSQLInstance(project, environment string) string {
	return Resource(project, environment, "sql")
}

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

// ClusterName returns the legacy Autopilot cluster name (GKE limit: 40
// characters). Existing Autopilot deployments keep this name for upgrades.
func ClusterName(project, environment string) string {
	return ClusterNameForRuntime(project, environment, "gke-autopilot")
}

// ClusterNameForRuntime keeps GKE runtime modes isolated when an operator
// switches between Autopilot and Standard. The short Autopilot form is kept
// stable for existing state and resource adoption.
func ClusterNameForRuntime(project, environment, runtime string) string {
	suffix := "gke"
	if runtime == "gke-standard" {
		suffix = "gke-standard"
	}
	base := Resource(project, environment, suffix)
	if len(base) > 40 {
		base = strings.TrimRight(base[:40], "-")
	}
	return base
}

// NodePoolName returns a GKE Standard node-pool name. GKE requires this name
// to be shorter than 40 characters, independently of the cluster limit.
func NodePoolName(project, environment string) string {
	base := Resource(project, environment, "nodes")
	if len(base) > 39 {
		base = strings.TrimRight(base[:39], "-")
	}
	return base
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
