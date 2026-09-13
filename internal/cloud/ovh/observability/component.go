// Package observability provisions the OVHcloud-native Kubernetes audit-log
// subscription when an existing Logs Data Platform stream is supplied. OVH's
// documented Pulumi surface does not make generic workload log shipping or
// metrics/traces a native stack resource, so those signals remain explicit
// unavailable rows in the portable plan.
package observability

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/ovh/pulumi-ovh/sdk/v2/go/ovh"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:ovh:Observability"

type Args struct {
	ServiceName string
	ClusterID   pulumi.StringInput
	Intent      sdk.ObservabilityIntent
}

type Component struct {
	pulumi.ResourceState
	AuditSubscriptionID   pulumi.StringOutput
	UnavailableSignals    pulumi.StringArrayOutput
	UnavailableOperations pulumi.StringArrayOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("OVH observability name is required")
	}
	if strings.TrimSpace(args.ServiceName) == "" || args.ClusterID == nil {
		return nil, errors.New("OVH observability service and MKS cluster are required")
	}
	if args.Intent.NativeProvider != "ovh-logs-data-platform" {
		return nil, fmt.Errorf("OVH observability requires native provider ovh-logs-data-platform, got %q", args.Intent.NativeProvider)
	}
	if err := sdk.ValidateObservabilityIntent(args.Intent); err != nil {
		return nil, fmt.Errorf("validate OVH observability intent: %w", err)
	}
	wantsAudit := false
	for _, signal := range args.Intent.Signals {
		if signal == "audit-events" || signal == "provider-operations" {
			wantsAudit = true
			break
		}
	}
	if wantsAudit && strings.TrimSpace(args.Intent.NativeReference) == "" {
		return nil, errors.New("OVH native audit observability requires nativeReference to identify a Logs Data Platform stream")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(ComponentToken, name, pulumi.Map{
		"serviceName":     pulumi.String(args.ServiceName),
		"ownershipMarker": pulumi.String(args.Intent.OwnershipMarker),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)
	if wantsAudit {
		subscription, err := ovh.NewCloudProjectKubeLogSubscription(ctx, name+"-audit", &ovh.CloudProjectKubeLogSubscriptionArgs{
			ServiceName: pulumi.String(args.ServiceName),
			KubeId:      args.ClusterID,
			StreamId:    pulumi.String(args.Intent.NativeReference),
			Kind:        pulumi.String("audit"),
		}, parent)
		if err != nil {
			return nil, fmt.Errorf("create OVH Kubernetes audit log subscription: %w", err)
		}
		component.AuditSubscriptionID = subscription.SubscriptionId
	}
	component.UnavailableSignals = pulumi.StringArray(toPulumiStrings(unavailableSignals(args.Intent))).ToStringArrayOutput()
	component.UnavailableOperations = pulumi.StringArray(toPulumiStrings(unavailableOperations(args.Intent))).ToStringArrayOutput()
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"auditSubscriptionId": component.AuditSubscriptionID,
		"unavailableSignals":  component.UnavailableSignals, "unavailableOperations": component.UnavailableOperations,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func unavailableSignals(intent sdk.ObservabilityIntent) []string {
	result := make([]string, 0, len(intent.Signals))
	for _, signal := range intent.Signals {
		if signal != "audit-events" && signal != "provider-operations" {
			result = append(result, signal)
		}
	}
	sort.Strings(result)
	return result
}

func unavailableOperations(intent sdk.ObservabilityIntent) []string {
	result := make([]string, 0, len(intent.Alerts)+len(intent.Dashboards)+len(intent.SLOs))
	for _, alert := range intent.Alerts {
		result = append(result, "alert:"+alert.ID)
	}
	for _, dashboard := range intent.Dashboards {
		result = append(result, "dashboard:"+dashboard.ID)
	}
	for _, slo := range intent.SLOs {
		result = append(result, "slo:"+slo.ID)
	}
	sort.Strings(result)
	return result
}

func toPulumiStrings(values []string) []pulumi.StringInput {
	result := make([]pulumi.StringInput, 0, len(values))
	for _, value := range values {
		result = append(result, pulumi.String(value))
	}
	return result
}
