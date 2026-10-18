// Package observability provisions the GKE-facing part of Google Cloud
// Observability. The portable signal intent stays in sdk; this package is
// the provider adapter for GKE collection and Google Cloud Observability
// resources.
package observability

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/logging"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/monitoring"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:gcp:Observability"

var labelPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

type Args struct {
	Project     string
	Region      string
	ClusterName pulumi.StringInput
	Intent      sdk.ObservabilityIntent
}

type Component struct {
	pulumi.ResourceState
	DashboardID           pulumi.StringOutput
	DashboardIDs          pulumi.StringArrayOutput
	LogBucketID           pulumi.StringOutput
	LogSinkID             pulumi.StringOutput
	AlertPolicyNames      pulumi.StringArrayOutput
	SLONames              pulumi.StringArrayOutput
	UnavailableSignals    pulumi.StringArrayOutput
	UnavailableOperations pulumi.StringArrayOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("GCP observability name is required")
	}
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("GCP observability project and region are required")
	}
	if args.Intent.NativeProvider != "google-cloud-operations" {
		return nil, fmt.Errorf("GCP observability requires native provider google-cloud-operations, got %q", args.Intent.NativeProvider)
	}
	if args.Intent.RetentionDays != 0 && (args.Intent.RetentionDays < 1 || args.Intent.RetentionDays > 3650) {
		return nil, errors.New("GCP observability retention must be between 1 and 3650 days when configured")
	}
	if strings.TrimSpace(args.Intent.RedactionPolicyRef) != "" {
		return nil, errors.New("GCP observability redaction requires a managed data-protection policy, which this component does not provide")
	}
	if err := sdk.ValidateObservabilityIntent(args.Intent); err != nil {
		return nil, fmt.Errorf("validate GCP observability intent: %w", err)
	}
	labels, err := googleLabels(args.Intent.Labels)
	if err != nil {
		return nil, err
	}
	for _, alert := range args.Intent.Alerts {
		if err := validateAlert(alert); err != nil {
			return nil, err
		}
	}
	serviceID := ""
	if len(args.Intent.SLOs) > 0 {
		var err error
		serviceID, err = googleSLOServiceID(args.Project, args.Intent.NativeReference)
		if err != nil {
			return nil, err
		}
		for _, slo := range args.Intent.SLOs {
			if err := validateGoogleSLO(slo); err != nil {
				return nil, err
			}
		}
	}
	labels[ownershipLabel] = pulumi.String(markerDigest(args.Intent.OwnershipMarker))
	component := &Component{
		LogBucketID: pulumi.String("").ToStringOutput(),
		LogSinkID:   pulumi.String("").ToStringOutput(),
	}
	if err := ctx.RegisterComponentResourceV2(ComponentToken, name, pulumi.Map{
		"project": pulumi.String(args.Project), "region": pulumi.String(args.Region),
		"ownershipMarker": pulumi.String(args.Intent.OwnershipMarker),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	dashboardIDs := make(pulumi.StringArray, 0, max(1, len(args.Intent.Dashboards)))
	if len(args.Intent.Dashboards) == 0 {
		dashboard, err := monitoring.NewDashboard(ctx, name+"-dashboard", &monitoring.DashboardArgs{
			Project:       pulumi.String(args.Project),
			DashboardJson: dashboardJSON(name, args.ClusterName),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create Google Cloud Monitoring dashboard: %w", err)
		}
		component.DashboardID = dashboard.ID().ToStringOutput()
		dashboardIDs = append(dashboardIDs, component.DashboardID)
	} else {
		for index, dashboardIntent := range args.Intent.Dashboards {
			dashboard, err := monitoring.NewDashboard(ctx, name+"-dashboard-"+dashboardIntent.ID, &monitoring.DashboardArgs{
				Project:       pulumi.String(args.Project),
				DashboardJson: requestedDashboardJSON(name, args.ClusterName, dashboardIntent),
			}, parent)
			if err != nil {
				return nil, fmt.Errorf("create Google Cloud Monitoring dashboard %q: %w", dashboardIntent.ID, err)
			}
			if index == 0 {
				component.DashboardID = dashboard.ID().ToStringOutput()
			}
			dashboardIDs = append(dashboardIDs, dashboard.ID().ToStringOutput())
		}
	}
	component.DashboardIDs = dashboardIDs.ToStringArrayOutput()
	if args.Intent.RetentionDays > 0 {
		bucket, err := logging.NewProjectBucketConfig(ctx, name+"-log-retention-bucket", &logging.ProjectBucketConfigArgs{
			Project:       pulumi.String(args.Project),
			Location:      pulumi.String(googleCloudOperationsDefaultLogLocation),
			BucketId:      pulumi.String(googleLogBucketID(args.Intent.OwnershipMarker)),
			Description:   pulumi.String(googleLogBucketDescription(args.Intent.OwnershipMarker)),
			RetentionDays: pulumi.Int(args.Intent.RetentionDays),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create Google Cloud log retention bucket: %w", err)
		}
		component.LogBucketID = bucket.BucketId

		sink, err := logging.NewProjectSink(ctx, name+"-log-retention-sink", &logging.ProjectSinkArgs{
			Project:     pulumi.String(args.Project),
			Name:        pulumi.String(googleLogSinkID(args.Intent.OwnershipMarker)),
			Description: pulumi.String(googleLogSinkDescription(args.Intent.OwnershipMarker)),
			Destination: pulumi.String("logging.googleapis.com/" + googleLogBucketName(args.Project, googleCloudOperationsDefaultLogLocation, args.Intent.OwnershipMarker)),
			Filter:      pulumi.String(`logName = ` + quote(logName(args.Project, args.Intent.OwnershipMarker))),
		}, parent, pulumi.DependsOn([]pulumi.Resource{bucket}))
		if err != nil {
			return nil, fmt.Errorf("create Google Cloud log retention sink: %w", err)
		}
		component.LogSinkID = sink.Name
	}

	policyNames := make(pulumi.StringArray, 0, len(args.Intent.Alerts))
	for _, alert := range args.Intent.Alerts {
		policy, err := newAlertPolicy(ctx, name, args, alert, labels, parent)
		if err != nil {
			return nil, err
		}
		policyNames = append(policyNames, policy.Name)
	}
	component.AlertPolicyNames = policyNames.ToStringArrayOutput()
	sloNames := make(pulumi.StringArray, 0, len(args.Intent.SLOs))
	for _, slo := range args.Intent.SLOs {
		resource, err := monitoring.NewSlo(ctx, name+"-slo-"+slo.ID, &monitoring.SloArgs{
			Project:           pulumi.String(args.Project),
			Service:           pulumi.String(serviceID),
			SloId:             pulumi.String(googleSLOID(args.Intent.OwnershipMarker, slo.ID)),
			DisplayName:       pulumi.String(googleSLODisplayName(args.Intent.OwnershipMarker, slo.ID)),
			Goal:              pulumi.Float64(slo.Target),
			RollingPeriodDays: pulumi.Int(int(slo.WindowSeconds / int64(24*time.Hour/time.Second))),
			BasicSli:          &monitoring.SloBasicSliArgs{Availability: &monitoring.SloBasicSliAvailabilityArgs{Enabled: pulumi.Bool(true)}},
			UserLabels:        labels,
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create Google Cloud SLO %q: %w", slo.ID, err)
		}
		sloNames = append(sloNames, resource.Name)
	}
	component.SLONames = sloNames.ToStringArrayOutput()
	component.UnavailableSignals = pulumiStringArray(unavailableSignals(args.Intent))
	component.UnavailableOperations = pulumiStringArray(unavailableOperations(args.Intent))
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"dashboardId": component.DashboardID, "dashboardIds": component.DashboardIDs,
		"logBucketId": component.LogBucketID, "logSinkId": component.LogSinkID,
		"alertPolicyNames": component.AlertPolicyNames, "sloNames": component.SLONames,
		"unavailableSignals": component.UnavailableSignals, "unavailableOperations": component.UnavailableOperations,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func unavailableSignals(intent sdk.ObservabilityIntent) []string {
	supported := map[string]bool{"logs": true, "metrics": true}
	requested := make([]string, 0, len(intent.Signals))
	requested = append(requested, intent.Signals...)
	for _, dashboard := range intent.Dashboards {
		requested = append(requested, dashboard.Signals...)
	}
	seen := make(map[string]struct{}, len(requested))
	result := make([]string, 0, len(requested))
	for _, signal := range requested {
		if _, exists := seen[signal]; exists {
			continue
		}
		seen[signal] = struct{}{}
		if !supported[signal] {
			result = append(result, signal)
		}
	}
	sort.Strings(result)
	return result
}

func unavailableOperations(intent sdk.ObservabilityIntent) []string {
	result := make([]string, 0, len(intent.SLOs)+1)
	if len(intent.Alerts) > 0 {
		result = append(result, "alert.notification-channel")
	}
	for _, slo := range intent.SLOs {
		if strings.TrimSpace(intent.NativeReference) == "" {
			result = append(result, "slo:"+slo.ID)
			continue
		}
		if _, err := googleSLOServiceIDFromParent(intent.NativeReference); err != nil || validateGoogleSLO(slo) != nil {
			result = append(result, "slo:"+slo.ID)
		}
	}
	sort.Strings(result)
	return result
}

func pulumiStringArray(values []string) pulumi.StringArrayOutput {
	inputs := make(pulumi.StringArray, 0, len(values))
	for _, value := range values {
		inputs = append(inputs, pulumi.String(value))
	}
	return inputs.ToStringArrayOutput()
}

func googleLabels(input map[string]string) (pulumi.StringMap, error) {
	labels := pulumi.StringMap{"magelift_managed": pulumi.String("true")}
	for key, value := range input {
		if !labelPattern.MatchString(key) || strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n") {
			return nil, fmt.Errorf("invalid Google Cloud observability label %q", key)
		}
		labels[key] = pulumi.String(value)
	}
	return labels, nil
}

func dashboardJSON(name string, clusterName pulumi.StringInput) pulumi.StringOutput {
	return renderDashboardJSON(name+" Google Cloud Observability", clusterName, []string{"logs", "metrics"})
}

func requestedDashboardJSON(name string, clusterName pulumi.StringInput, intent sdk.DashboardIntent) pulumi.StringOutput {
	return renderDashboardJSON(name+" "+intent.ID+" Google Cloud Observability", clusterName, intent.Signals)
}

func renderDashboardJSON(displayName string, clusterName pulumi.StringInput, signals []string) pulumi.StringOutput {
	requested := sdk.SortedStrings(signals)
	return pulumi.All(clusterName).ApplyT(func(values []interface{}) (string, error) {
		cluster, ok := values[0].(string)
		if !ok || strings.TrimSpace(cluster) == "" {
			return "", errors.New("GCP observability cluster name is required")
		}
		widgets := make([]map[string]any, 0, len(requested))
		for _, signal := range requested {
			switch signal {
			case "logs":
				widgets = append(widgets, map[string]any{
					"title":     "GKE workload logs",
					"logsPanel": map[string]any{"filter": fmt.Sprintf(`resource.type="k8s_container" resource.labels.cluster_name=%q`, cluster)},
				})
			case "metrics":
				widgets = append(widgets, map[string]any{
					"title": "GKE workload CPU",
					"xyChart": map[string]any{"dataSets": []any{
						map[string]any{"timeSeriesQuery": map[string]any{"timeSeriesFilter": map[string]any{
							"filter": fmt.Sprintf(`metric.type="kubernetes.io/container/cpu/core_usage_time" resource.labels.cluster_name=%q`, cluster),
						}}},
					}},
				})
			default:
				widgets = append(widgets, map[string]any{
					"title": signal + " is unavailable natively",
					"text": map[string]any{
						"content": fmt.Sprintf("MageLift requested signal `%s` is not delivered by this native GCP component. Configure an external observability adapter for it.", signal),
						"format":  "MARKDOWN",
					},
				})
			}
		}
		body, err := json.Marshal(map[string]any{
			"displayName": displayName,
			"gridLayout":  map[string]any{"columns": "2", "widgets": widgets},
		})
		if err != nil {
			return "", fmt.Errorf("encode Google Cloud dashboard: %w", err)
		}
		return string(body), nil
	}).(pulumi.StringOutput)
}

func newAlertPolicy(ctx *pulumi.Context, name string, args Args, alert sdk.AlertIntent, labels pulumi.StringMap, parent pulumi.ResourceOption) (*monitoring.AlertPolicy, error) {
	if err := validateAlert(alert); err != nil {
		return nil, err
	}
	filter := fmt.Sprintf(`resource.type="k8s_container" jsonPayload.magelift.signal=%q`, alert.Signal)
	condition := monitoring.AlertPolicyConditionArgs{
		DisplayName: pulumi.String(alert.ID),
		ConditionMatchedLog: monitoring.AlertPolicyConditionConditionMatchedLogArgs{
			Filter: pulumi.String(filter),
		},
	}
	documentation := &monitoring.AlertPolicyDocumentationArgs{Content: pulumi.String(alert.ID + " requires operator action."), MimeType: pulumi.String("text/markdown")}
	if alert.RunbookURL != "" {
		documentation.Links = monitoring.AlertPolicyDocumentationLinkArray{monitoring.AlertPolicyDocumentationLinkArgs{DisplayName: pulumi.String("runbook"), Url: pulumi.String(alert.RunbookURL)}}
	}
	policy, err := monitoring.NewAlertPolicy(ctx, name+"-alert-"+alert.ID, &monitoring.AlertPolicyArgs{
		Project:       pulumi.String(args.Project),
		DisplayName:   pulumi.String(name + " " + alert.ID),
		Combiner:      pulumi.String("OR"),
		Conditions:    monitoring.AlertPolicyConditionArray{condition},
		Documentation: *documentation,
		Severity:      pulumi.String(alertSeverity(alert.Severity)),
		UserLabels:    pulumiAlertLabels(labels, alert),
		// NotificationChannels omitted: paging delivery is not managed here.
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Google Cloud alert policy %q: %w", alert.ID, err)
	}
	return policy, nil
}

func pulumiAlertLabels(base pulumi.StringMap, intent sdk.AlertIntent) pulumi.StringMap {
	labels := make(pulumi.StringMap, len(base)+2)
	for key, value := range base {
		labels[key] = value
	}
	labels["magelift_owner"] = pulumi.String(googleLabelValue(intent.Owner))
	labels["magelift_deduplication"] = pulumi.String(googleLabelValue(intent.DeduplicationKey))
	return labels
}

func validateAlert(alert sdk.AlertIntent) error {
	// Matched-log policies are intentionally limited to presence alerts. A
	// portable threshold alert needs a provider metric mapping; silently
	// discarding Operator and Threshold would produce a false certification.
	if alert.Signal != "logs" && alert.Signal != "provider-operations" && alert.Signal != "cleanup-failure" {
		return fmt.Errorf("GCP native alert %q uses unsupported signal %q; provide a provider metric adapter or use New Relic", alert.ID, alert.Signal)
	}
	if alert.Operator != "gt" || alert.Threshold != 0 {
		return fmt.Errorf("GCP native log alert %q requires operator gt and threshold 0", alert.ID)
	}
	if alert.WindowSeconds%60 != 0 {
		return fmt.Errorf("GCP native alert %q window must be a whole number of minutes", alert.ID)
	}
	return nil
}

func alertSeverity(severity string) string {
	switch severity {
	case "critical":
		return "CRITICAL"
	case "warning":
		return "WARNING"
	default:
		return "ERROR"
	}
}
