// Package observability provisions Scaleway Cockpit data sources. Cockpit
// owns metrics, logs, and traces; unsupported audit and alert semantics stay
// visible in the provider-neutral signal plan.
package observability

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway/observability"
)

const ComponentToken = "magelift:scaleway:Observability"

var resourceNamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

type Args struct {
	ProjectID string
	Region    string
	Intent    sdk.ObservabilityIntent
}

type Component struct {
	pulumi.ResourceState
	SourceIDs             pulumi.StringArrayOutput
	AlertManagerID        pulumi.StringOutput
	UnavailableSignals    pulumi.StringArrayOutput
	UnavailableOperations pulumi.StringArrayOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("Scaleway observability name is required")
	}
	if strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("Scaleway observability project and region are required")
	}
	if args.Intent.NativeProvider != "scaleway-cockpit" {
		return nil, fmt.Errorf("Scaleway observability requires native provider scaleway-cockpit, got %q", args.Intent.NativeProvider)
	}
	if err := sdk.ValidateObservabilityIntent(args.Intent); err != nil {
		return nil, fmt.Errorf("validate Scaleway observability intent: %w", err)
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(ComponentToken, name, pulumi.Map{
		"projectId": pulumi.String(args.ProjectID), "region": pulumi.String(args.Region),
		"ownershipMarker": pulumi.String(args.Intent.OwnershipMarker),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	retention := args.Intent.RetentionDays
	if retention == 0 {
		retention = 7
	}
	kinds := make(map[string]struct{})
	for _, signal := range args.Intent.Signals {
		switch signal {
		case "logs", "metrics", "traces":
			kinds[signal] = struct{}{}
		}
	}
	sortedKinds := make([]string, 0, len(kinds))
	for kind := range kinds {
		sortedKinds = append(sortedKinds, kind)
	}
	sort.Strings(sortedKinds)
	sourceIDs := make(pulumi.StringArray, 0, len(sortedKinds))
	for _, kind := range sortedKinds {
		source, err := observability.NewSource(ctx, name+"-"+kind, &observability.SourceArgs{
			Name:          pulumi.String(sourceName(name, kind)),
			ProjectId:     pulumi.String(args.ProjectID),
			Region:        pulumi.String(args.Region),
			RetentionDays: pulumi.Int(retention),
			Type:          pulumi.String(kind),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create Scaleway Cockpit %s source: %w", kind, err)
		}
		sourceIDs = append(sourceIDs, source.ID().ToStringOutput())
	}
	var alertManager *observability.AlertManager
	var err error
	if len(args.Intent.AlertRefs) > 0 {
		alertRefs := make(pulumi.StringArray, 0, len(args.Intent.AlertRefs))
		for _, ref := range args.Intent.AlertRefs {
			alertRefs = append(alertRefs, pulumi.String(ref))
		}
		alertManager, err = observability.NewAlertManager(ctx, name+"-alerts", &observability.AlertManagerArgs{
			ProjectId:             pulumi.String(args.ProjectID),
			Region:                pulumi.String(args.Region),
			PreconfiguredAlertIds: alertRefs,
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create Scaleway Cockpit alert manager: %w", err)
		}
		component.AlertManagerID = alertManager.ID().ToStringOutput()
	}
	unavailable := make([]pulumi.StringInput, 0)
	for _, signal := range args.Intent.Signals {
		if signal != "logs" && signal != "metrics" && signal != "traces" {
			unavailable = append(unavailable, pulumi.String(signal))
		}
	}
	component.SourceIDs = sourceIDs.ToStringArrayOutput()
	component.UnavailableSignals = pulumi.StringArray(unavailable).ToStringArrayOutput()
	operations := make(pulumi.StringArray, 0, len(toUnavailableOperations(args.Intent)))
	for _, operation := range toUnavailableOperations(args.Intent) {
		operations = append(operations, pulumi.String(operation))
	}
	component.UnavailableOperations = operations.ToStringArrayOutput()
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"sourceIds": component.SourceIDs, "alertManagerId": component.AlertManagerID,
		"unavailableSignals": component.UnavailableSignals, "unavailableOperations": component.UnavailableOperations,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func toUnavailableOperations(intent sdk.ObservabilityIntent) []string {
	operations := make([]string, 0, len(intent.Alerts)+len(intent.Dashboards)+len(intent.SLOs))
	for _, alert := range intent.Alerts {
		operations = append(operations, "alert:"+alert.ID)
	}
	for _, dashboard := range intent.Dashboards {
		operations = append(operations, "dashboard:"+dashboard.ID)
	}
	for _, slo := range intent.SLOs {
		operations = append(operations, "slo:"+slo.ID)
	}
	sort.Strings(operations)
	return operations
}

func sourceName(name, kind string) string {
	clean := resourceNamePattern.ReplaceAllString(strings.ToLower(name), "-")
	clean = strings.Trim(clean, "-")
	if clean == "" {
		clean = "magelift"
	}
	return clean + "-" + kind
}
