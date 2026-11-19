package cli

import (
	"github.com/magelift/magelift/internal/config"
	"net/url"
)

// envConsole points to the provider's own account-scoped operational surfaces.
// A link does not assert that a resource has been deployed or is healthy.
func envConsole(c config.Config) map[string]string {
	region := url.QueryEscape(c.Defaults.Region)
	switch c.Target.Provider {
	case "aws":
		return map[string]string{"workloads": "https://console.aws.amazon.com/ecs/v2/clusters?region=" + region, "logs": "https://console.aws.amazon.com/cloudwatch/home?region=" + region + "#logsV2:log-groups", "metrics": "https://console.aws.amazon.com/cloudwatch/home?region=" + region + "#metricsV2:"}
	case "gcp":
		if c.Target.GCP != nil {
			project := url.QueryEscape(c.Target.GCP.Project)
			return map[string]string{"workloads": "https://console.cloud.google.com/kubernetes/workload?project=" + project, "logs": "https://console.cloud.google.com/logs/query?project=" + project, "metrics": "https://console.cloud.google.com/monitoring?project=" + project}
		}
	case "azure":
		if a := c.Target.Azure; a != nil {
			base := "https://portal.azure.com/#resource/subscriptions/" + url.PathEscape(a.SubscriptionID) + "/resourceGroups/" + url.PathEscape(a.ResourceGroup)
			return map[string]string{"resources": base + "/overview", "activity": base + "/activitylog", "metrics": "https://portal.azure.com/#view/Microsoft_Azure_Monitoring/AzureMonitoringBrowseBlade"}
		}
	}
	return nil
}
