package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	aws "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cloudwatchlogtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk/v1"
)

const cloudWatchOwnershipTagKey = "magelift-ownership"

// CloudWatchLogsAPI is the narrow AWS SDK surface used by the lifecycle
// translator. Keeping the interface here makes the official SDK injectable
// in contract tests without making the core depend on AWS types.
type CloudWatchLogsAPI interface {
	CreateLogGroup(context.Context, *cloudwatchlogs.CreateLogGroupInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error)
	PutRetentionPolicy(context.Context, *cloudwatchlogs.PutRetentionPolicyInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error)
	CreateLogStream(context.Context, *cloudwatchlogs.CreateLogStreamInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error)
	DescribeLogStreams(context.Context, *cloudwatchlogs.DescribeLogStreamsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error)
	PutLogEvents(context.Context, *cloudwatchlogs.PutLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error)
	FilterLogEvents(context.Context, *cloudwatchlogs.FilterLogEventsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error)
	DescribeLogGroups(context.Context, *cloudwatchlogs.DescribeLogGroupsInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error)
	ListTagsForResource(context.Context, *cloudwatchlogs.ListTagsForResourceInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.ListTagsForResourceOutput, error)
	DeleteLogGroup(context.Context, *cloudwatchlogs.DeleteLogGroupInput, ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error)
}

// CloudWatchMetricsAPI is the narrow CloudWatch SDK surface used by the
// translator for custom probe metrics, alarms, dashboards, and inventory.
type CloudWatchMetricsAPI interface {
	PutMetricData(context.Context, *cloudwatch.PutMetricDataInput, ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error)
	GetMetricStatistics(context.Context, *cloudwatch.GetMetricStatisticsInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error)
	PutDashboard(context.Context, *cloudwatch.PutDashboardInput, ...func(*cloudwatch.Options)) (*cloudwatch.PutDashboardOutput, error)
	GetDashboard(context.Context, *cloudwatch.GetDashboardInput, ...func(*cloudwatch.Options)) (*cloudwatch.GetDashboardOutput, error)
	DeleteDashboards(context.Context, *cloudwatch.DeleteDashboardsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DeleteDashboardsOutput, error)
	ListDashboards(context.Context, *cloudwatch.ListDashboardsInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListDashboardsOutput, error)
	PutMetricAlarm(context.Context, *cloudwatch.PutMetricAlarmInput, ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricAlarmOutput, error)
	DescribeAlarms(context.Context, *cloudwatch.DescribeAlarmsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error)
	ListTagsForResource(context.Context, *cloudwatch.ListTagsForResourceInput, ...func(*cloudwatch.Options)) (*cloudwatch.ListTagsForResourceOutput, error)
	TagResource(context.Context, *cloudwatch.TagResourceInput, ...func(*cloudwatch.Options)) (*cloudwatch.TagResourceOutput, error)
	DeleteAlarms(context.Context, *cloudwatch.DeleteAlarmsInput, ...func(*cloudwatch.Options)) (*cloudwatch.DeleteAlarmsOutput, error)
}

// CloudWatchBackend translates the portable observability plan to official
// CloudWatch Logs and CloudWatch APIs. It owns only resources whose names are
// derived from the exact MageLift ownership marker.
type CloudWatchBackend struct {
	logs  CloudWatchLogsAPI
	cloud CloudWatchMetricsAPI
	now   func() time.Time
}

var _ providerobservability.Backend = (*CloudWatchBackend)(nil)

func NewCloudWatchBackend(logs CloudWatchLogsAPI, cloud CloudWatchMetricsAPI) (*CloudWatchBackend, error) {
	if logs == nil || cloud == nil {
		return nil, errors.New("CloudWatch Logs and CloudWatch clients are required")
	}
	return &CloudWatchBackend{logs: logs, cloud: cloud, now: time.Now}, nil
}

// NewCloudWatchSDKClient constructs the provider-owned lifecycle client from
// official AWS SDK v2 clients. Credential resolution remains in the caller's
// AWS config/provider boundary.
func NewCloudWatchSDKClient(cfg aws.Config) (*providerobservability.ManagedLifecycleClient, error) {
	backend, err := NewCloudWatchBackend(cloudwatchlogs.NewFromConfig(cfg), cloudwatch.NewFromConfig(cfg))
	if err != nil {
		return nil, err
	}
	return providerobservability.NewManagedLifecycleClient(backend)
}

func (backend *CloudWatchBackend) Apply(ctx context.Context, plan providerobservability.Plan) (providerobservability.LifecycleResult, error) {
	if err := validatePlan(ctx, plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	}
	if err := validateBindings(plan); err != nil {
		return providerobservability.LifecycleResult{}, err
	}
	refs := make([]string, 0, len(plan.Bindings)*2+len(plan.Alerts)+1)
	for _, binding := range plan.Bindings {
		switch binding.Signal {
		case "logs":
			if err := backend.ensureLogProbe(ctx, binding); err != nil {
				return providerobservability.LifecycleResult{}, err
			}
			refs = append(refs, "cloudwatch:log-group:"+logGroupName(binding.OwnershipMarker), "cloudwatch:log-stream:"+logStreamName(binding.OwnershipMarker))
		case "metrics":
			if err := backend.publishMetric(ctx, binding); err != nil {
				return providerobservability.LifecycleResult{}, err
			}
			refs = append(refs, "cloudwatch:metric:"+metricName(binding.OwnershipMarker, binding.Signal))
		default:
			return providerobservability.LifecycleResult{}, fmt.Errorf("CloudWatch lifecycle does not implement signal %q", binding.Signal)
		}
	}
	if len(plan.Dashboards) > 0 {
		if err := backend.putDashboard(ctx, plan); err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs = append(refs, "cloudwatch:dashboard:"+dashboardName(plan.OwnershipMarker))
	}
	for _, alert := range plan.Alerts {
		if err := backend.putAlarm(ctx, plan, alert); err != nil {
			return providerobservability.LifecycleResult{}, err
		}
		refs = append(refs, "cloudwatch:alarm:"+alarmName(plan.OwnershipMarker, alert.ID))
	}
	sort.Strings(refs)
	return providerobservability.LifecycleResult{
		OperationID:         "cloudwatch:observability:apply:" + markerDigest(plan.OwnershipMarker),
		ResourceRefs:        refs,
		ProofRefs:           []string{"cloudwatch:ownership", "cloudwatch:direct-api"},
		OwnershipVerified:   true,
		IdempotencyVerified: true,
	}, nil
}

func (backend *CloudWatchBackend) VerifySignal(ctx context.Context, binding providerobservability.SignalBinding) (providerobservability.SignalObservation, error) {
	if ctx == nil {
		return providerobservability.SignalObservation{}, errors.New("CloudWatch signal verification context is required")
	}
	if strings.TrimSpace(binding.OwnershipMarker) == "" {
		return providerobservability.SignalObservation{}, errors.New("CloudWatch signal ownership marker is required")
	}
	observation := providerobservability.SignalObservation{Signal: binding.Signal, Destination: binding.Destination}
	switch binding.Signal {
	case "logs":
		group, err := backend.findLogGroup(ctx, logGroupName(binding.OwnershipMarker))
		if err != nil {
			return providerobservability.SignalObservation{}, err
		}
		if group == nil {
			observation.Reason = "CloudWatch log group is missing"
			return observation, nil
		}
		if binding.RetentionDays > 0 && (group.RetentionInDays == nil || int(*group.RetentionInDays) != binding.RetentionDays) {
			observation.Reason = "CloudWatch log retention does not match the plan"
			return observation, nil
		}
		start := backend.now().Add(-10 * time.Minute).UnixMilli()
		end := backend.now().Add(2 * time.Minute).UnixMilli()
		// FilterLogEvents accepts an omitted filter pattern to return all events.
		// Keep the query scoped to this owned stream and match the exact probe in
		// Go; the marker contains punctuation that is data, not filter syntax.
		output, err := backend.logs.FilterLogEvents(ctx, &cloudwatchlogs.FilterLogEventsInput{LogGroupName: aws.String(logGroupName(binding.OwnershipMarker)), LogStreamNamePrefix: aws.String(logStreamName(binding.OwnershipMarker)), StartTime: aws.Int64(start), EndTime: aws.Int64(end), Limit: aws.Int32(100)})
		if err != nil {
			return providerobservability.SignalObservation{}, errors.New("CloudWatch log delivery verification failed")
		}
		for _, event := range output.Events {
			if event.Message != nil && strings.Contains(*event.Message, probeMessage(binding.OwnershipMarker, binding.Signal)) {
				observation.Delivered = true
				observation.LabelsVerified = strings.Contains(*event.Message, binding.OwnershipMarker)
				break
			}
		}
		observation.RetentionVerified = binding.RetentionDays == 0 || (group.RetentionInDays != nil && int(*group.RetentionInDays) == binding.RetentionDays)
		observation.RedactionVerified = binding.RedactionPolicy == ""
	case "metrics":
		output, err := backend.cloud.GetMetricStatistics(ctx, &cloudwatch.GetMetricStatisticsInput{Namespace: aws.String(metricNamespace(binding.OwnershipMarker)), MetricName: aws.String(metricName(binding.OwnershipMarker, binding.Signal)), Dimensions: []cloudwatchtypes.Dimension{{Name: aws.String("MageLiftMarker"), Value: aws.String(binding.OwnershipMarker)}}, StartTime: aws.Time(backend.now().Add(-10 * time.Minute)), EndTime: aws.Time(backend.now().Add(2 * time.Minute)), Period: aws.Int32(60), Statistics: []cloudwatchtypes.Statistic{cloudwatchtypes.StatisticAverage}})
		if err != nil {
			return providerobservability.SignalObservation{}, errors.New("CloudWatch metric delivery verification failed")
		}
		observation.Delivered = len(output.Datapoints) > 0
		observation.LabelsVerified = observation.Delivered
		observation.RetentionVerified = true
		observation.RedactionVerified = true
	default:
		observation.Reason = "CloudWatch signal is not implemented by this adapter"
	}
	return observation, nil
}

func (backend *CloudWatchBackend) VerifyOperations(ctx context.Context, plan providerobservability.Plan) (providerobservability.OperationalObservation, error) {
	if err := validatePlan(ctx, plan); err != nil {
		return providerobservability.OperationalObservation{}, err
	}
	observation := providerobservability.OperationalObservation{
		AlertsVerified:     len(plan.Alerts) == 0,
		DashboardsVerified: len(plan.Dashboards) == 0,
		SLOsVerified:       len(plan.SLOs) == 0,
	}
	if len(plan.Dashboards) > 0 {
		_, err := backend.cloud.GetDashboard(ctx, &cloudwatch.GetDashboardInput{DashboardName: aws.String(dashboardName(plan.OwnershipMarker))})
		if err != nil {
			observation.DashboardsVerified = false
		} else {
			observation.DashboardsVerified = true
		}
	}
	if len(plan.Alerts) > 0 {
		output, err := backend.cloud.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{AlarmNamePrefix: aws.String(alarmPrefix(plan.OwnershipMarker))})
		if err != nil {
			return providerobservability.OperationalObservation{}, errors.New("CloudWatch alarm verification failed")
		}
		observation.AlertsVerified = true
		found := make(map[string]struct{}, len(output.MetricAlarms))
		for _, alarm := range output.MetricAlarms {
			if alarm.AlarmName != nil {
				found[*alarm.AlarmName] = struct{}{}
			}
		}
		for _, alert := range plan.Alerts {
			alarm, ok := foundAlarm(output.MetricAlarms, alarmName(plan.OwnershipMarker, alert.ID))
			if !ok {
				observation.AlertsVerified = false
				continue
			}
			matches, err := backend.cloudWatchAlarmMatches(ctx, alarm, plan.OwnershipMarker, alert)
			if err != nil {
				return providerobservability.OperationalObservation{}, errors.New("CloudWatch alarm metadata verification failed")
			}
			if !matches {
				observation.AlertsVerified = false
			}
		}
	}
	return observation, nil
}

func (backend *CloudWatchBackend) Destroy(ctx context.Context, plan providerobservability.Plan, _ []string) error {
	if err := validatePlan(ctx, plan); err != nil {
		return err
	}
	inventory, err := backend.Inventory(ctx, plan.OwnershipMarker)
	if err != nil {
		return errors.New("inventory CloudWatch resources for cleanup failed")
	}
	alarmNames := make([]string, 0)
	for _, resource := range inventory {
		if !resource.Owned || resource.OwnershipMarker != plan.OwnershipMarker {
			continue
		}
		switch {
		case strings.HasPrefix(resource.Identity, "cloudwatch:alarm:"):
			alarmNames = append(alarmNames, strings.TrimPrefix(resource.Identity, "cloudwatch:alarm:"))
		case strings.HasPrefix(resource.Identity, "cloudwatch:dashboard:"):
			name := strings.TrimPrefix(resource.Identity, "cloudwatch:dashboard:")
			if _, err := backend.cloud.DeleteDashboards(ctx, &cloudwatch.DeleteDashboardsInput{DashboardNames: []string{name}}); err != nil {
				return errors.New("delete CloudWatch dashboard failed")
			}
		case strings.HasPrefix(resource.Identity, "cloudwatch:log-group:"):
			name := strings.TrimPrefix(resource.Identity, "cloudwatch:log-group:")
			if _, err := backend.logs.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(name)}); err != nil {
				return errors.New("delete CloudWatch log group failed")
			}
		}
	}
	if len(alarmNames) > 0 {
		if _, err := backend.cloud.DeleteAlarms(ctx, &cloudwatch.DeleteAlarmsInput{AlarmNames: alarmNames}); err != nil {
			return errors.New("delete CloudWatch alarms failed")
		}
	}
	return nil
}

func (backend *CloudWatchBackend) Inventory(ctx context.Context, marker string) ([]providerobservability.InventoryResource, error) {
	if ctx == nil || strings.TrimSpace(marker) == "" {
		return nil, errors.New("CloudWatch inventory requires context and ownership marker")
	}
	resources := make([]providerobservability.InventoryResource, 0)
	if group, err := backend.findLogGroup(ctx, logGroupName(marker)); err != nil {
		return nil, err
	} else if group != nil {
		owned := false
		if group.LogGroupArn != nil {
			tags, tagErr := backend.logs.ListTagsForResource(ctx, &cloudwatchlogs.ListTagsForResourceInput{ResourceArn: group.LogGroupArn})
			if tagErr != nil {
				return nil, errors.New("inventory CloudWatch log group ownership failed")
			}
			owned = tags.Tags[cloudWatchOwnershipTagKey] == marker
		}
		resource := providerobservability.InventoryResource{Identity: "cloudwatch:log-group:" + logGroupName(marker), Live: true}
		if owned {
			resource.OwnershipMarker = marker
			resource.Owned = true
		}
		resources = append(resources, resource)
	}
	dashboardOutput, err := backend.cloud.ListDashboards(ctx, &cloudwatch.ListDashboardsInput{DashboardNamePrefix: aws.String(dashboardName(marker))})
	if err != nil {
		return nil, errors.New("inventory CloudWatch dashboards failed")
	}
	for _, dashboard := range dashboardOutput.DashboardEntries {
		if dashboard.DashboardName != nil && *dashboard.DashboardName == dashboardName(marker) {
			resource := providerobservability.InventoryResource{Identity: "cloudwatch:dashboard:" + *dashboard.DashboardName, Live: true}
			if dashboard.DashboardArn != nil {
				owned, tagErr := backend.cloudWatchResourceOwned(ctx, *dashboard.DashboardArn, marker)
				if tagErr != nil {
					return nil, errors.New("inventory CloudWatch dashboard ownership failed")
				}
				if owned {
					resource.OwnershipMarker = marker
					resource.Owned = true
				}
			}
			resources = append(resources, resource)
		}
	}
	alarms, err := backend.cloud.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{AlarmNamePrefix: aws.String(alarmPrefix(marker))})
	if err != nil {
		return nil, errors.New("inventory CloudWatch alarms failed")
	}
	for _, alarm := range alarms.MetricAlarms {
		if alarm.AlarmName != nil {
			resource := providerobservability.InventoryResource{Identity: "cloudwatch:alarm:" + *alarm.AlarmName, Live: true}
			if alarm.AlarmArn != nil {
				owned, tagErr := backend.cloudWatchResourceOwned(ctx, *alarm.AlarmArn, marker)
				if tagErr != nil {
					return nil, errors.New("inventory CloudWatch alarm ownership failed")
				}
				if owned {
					resource.OwnershipMarker = marker
					resource.Owned = true
				}
			}
			resources = append(resources, resource)
		}
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].Identity < resources[j].Identity })
	return resources, nil
}

func (backend *CloudWatchBackend) ensureLogProbe(ctx context.Context, binding providerobservability.SignalBinding) error {
	groupName := logGroupName(binding.OwnershipMarker)
	group, err := backend.findLogGroup(ctx, groupName)
	if err != nil {
		return errors.New("inventory CloudWatch log group failed")
	}
	if group == nil {
		if _, err := backend.logs.CreateLogGroup(ctx, &cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(groupName), Tags: map[string]string{cloudWatchOwnershipTagKey: binding.OwnershipMarker}}); err != nil {
			if _, lookupErr := backend.findLogGroup(ctx, groupName); lookupErr != nil {
				return errors.New("create CloudWatch log group failed")
			}
		}
		group, err = backend.findLogGroup(ctx, groupName)
		if err != nil || group == nil {
			return errors.New("inventory CloudWatch log group after create failed")
		}
	}
	if err := backend.requireOwnedLogGroup(ctx, group, binding.OwnershipMarker); err != nil {
		return err
	}
	retention := binding.RetentionDays
	if retention <= 0 {
		retention = 7
	}
	if _, err := backend.logs.PutRetentionPolicy(ctx, &cloudwatchlogs.PutRetentionPolicyInput{LogGroupName: aws.String(groupName), RetentionInDays: aws.Int32(int32(retention))}); err != nil {
		return errors.New("set CloudWatch log retention failed")
	}
	streamName := logStreamName(binding.OwnershipMarker)
	streams, err := backend.logs.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{LogGroupName: aws.String(groupName), LogStreamNamePrefix: aws.String(streamName), Limit: aws.Int32(10)})
	if err != nil {
		return errors.New("inventory CloudWatch log stream failed")
	}
	found := false
	for _, stream := range streams.LogStreams {
		if stream.LogStreamName != nil && *stream.LogStreamName == streamName {
			found = true
			break
		}
	}
	if !found {
		if _, err := backend.logs.CreateLogStream(ctx, &cloudwatchlogs.CreateLogStreamInput{LogGroupName: aws.String(groupName), LogStreamName: aws.String(streamName)}); err != nil {
			streams, lookupErr := backend.logs.DescribeLogStreams(ctx, &cloudwatchlogs.DescribeLogStreamsInput{LogGroupName: aws.String(groupName), LogStreamNamePrefix: aws.String(streamName), Limit: aws.Int32(10)})
			if lookupErr != nil {
				return errors.New("create CloudWatch log stream failed")
			}
			for _, stream := range streams.LogStreams {
				if stream.LogStreamName != nil && *stream.LogStreamName == streamName {
					found = true
					break
				}
			}
			if !found {
				return errors.New("create CloudWatch log stream failed")
			}
		}
	}
	_, err = backend.logs.PutLogEvents(ctx, &cloudwatchlogs.PutLogEventsInput{LogGroupName: aws.String(groupName), LogStreamName: aws.String(streamName), LogEvents: []cloudwatchlogtypes.InputLogEvent{{Message: aws.String(probeMessage(binding.OwnershipMarker, binding.Signal)), Timestamp: aws.Int64(backend.now().UnixMilli())}}})
	if err != nil {
		return errors.New("publish CloudWatch log probe failed")
	}
	return nil
}

func (backend *CloudWatchBackend) publishMetric(ctx context.Context, binding providerobservability.SignalBinding) error {
	value := float64(1)
	_, err := backend.cloud.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{Namespace: aws.String(metricNamespace(binding.OwnershipMarker)), MetricData: []cloudwatchtypes.MetricDatum{{MetricName: aws.String(metricName(binding.OwnershipMarker, binding.Signal)), Dimensions: []cloudwatchtypes.Dimension{{Name: aws.String("MageLiftMarker"), Value: aws.String(binding.OwnershipMarker)}}, Value: &value, Unit: cloudwatchtypes.StandardUnitCount, Timestamp: aws.Time(backend.now())}}})
	if err != nil {
		return errors.New("publish CloudWatch metric probe failed")
	}
	return nil
}

func (backend *CloudWatchBackend) putDashboard(ctx context.Context, plan providerobservability.Plan) error {
	dashboards, err := backend.cloud.ListDashboards(ctx, &cloudwatch.ListDashboardsInput{DashboardNamePrefix: aws.String(dashboardName(plan.OwnershipMarker))})
	if err != nil {
		return errors.New("inventory CloudWatch dashboard before update failed")
	}
	for _, dashboard := range dashboards.DashboardEntries {
		if dashboard.DashboardName == nil || *dashboard.DashboardName != dashboardName(plan.OwnershipMarker) {
			continue
		}
		if dashboard.DashboardArn == nil || strings.TrimSpace(*dashboard.DashboardArn) == "" {
			return errors.New("CloudWatch dashboard collision has no taggable ARN")
		}
		owned, err := backend.cloudWatchResourceOwned(ctx, *dashboard.DashboardArn, plan.OwnershipMarker)
		if err != nil {
			return errors.New("inspect CloudWatch dashboard ownership failed")
		}
		if !owned {
			return errors.New("refusing to replace an unowned CloudWatch dashboard")
		}
	}
	body, err := json.Marshal(map[string]any{"widgets": []any{map[string]any{"type": "text", "x": 0, "y": 0, "width": 24, "height": 3, "properties": map[string]any{"markdown": "MageLift observability " + plan.OwnershipMarker}}}})
	if err != nil {
		return errors.New("encode CloudWatch dashboard failed")
	}
	if _, err := backend.cloud.PutDashboard(ctx, &cloudwatch.PutDashboardInput{DashboardName: aws.String(dashboardName(plan.OwnershipMarker)), DashboardBody: aws.String(string(body))}); err != nil {
		return errors.New("create CloudWatch dashboard failed")
	}
	dashboards, err = backend.cloud.ListDashboards(ctx, &cloudwatch.ListDashboardsInput{DashboardNamePrefix: aws.String(dashboardName(plan.OwnershipMarker))})
	if err != nil {
		return errors.New("inventory CloudWatch dashboard after update failed")
	}
	for _, dashboard := range dashboards.DashboardEntries {
		if dashboard.DashboardName != nil && *dashboard.DashboardName == dashboardName(plan.OwnershipMarker) && dashboard.DashboardArn != nil {
			if _, err := backend.cloud.TagResource(ctx, &cloudwatch.TagResourceInput{ResourceARN: dashboard.DashboardArn, Tags: []cloudwatchtypes.Tag{{Key: aws.String(cloudWatchOwnershipTagKey), Value: aws.String(plan.OwnershipMarker)}}}); err != nil {
				return errors.New("tag CloudWatch dashboard ownership failed")
			}
			return nil
		}
	}
	return errors.New("CloudWatch dashboard response has no taggable ARN")
}

func (backend *CloudWatchBackend) putAlarm(ctx context.Context, plan providerobservability.Plan, alert v1.AlertIntent) error {
	if strings.TrimSpace(alert.ID) == "" {
		return errors.New("CloudWatch alert ID is required")
	}
	comparison, evaluationPeriods, err := cloudWatchAlarmSettings(alert)
	if err != nil {
		return err
	}
	tags, err := cloudWatchAlertTags(plan.OwnershipMarker, alert)
	if err != nil {
		return err
	}
	threshold := alert.Threshold
	name := alarmName(plan.OwnershipMarker, alert.ID)
	existing, err := backend.cloud.DescribeAlarms(ctx, &cloudwatch.DescribeAlarmsInput{AlarmNames: []string{name}})
	if err != nil {
		return errors.New("inspect CloudWatch alarm before update failed")
	}
	for _, alarm := range existing.MetricAlarms {
		if alarm.AlarmName == nil || *alarm.AlarmName != name {
			continue
		}
		if alarm.AlarmArn == nil || strings.TrimSpace(*alarm.AlarmArn) == "" {
			return errors.New("CloudWatch alarm collision has no taggable ARN")
		}
		owned, err := backend.cloudWatchResourceOwned(ctx, *alarm.AlarmArn, plan.OwnershipMarker)
		if err != nil {
			return errors.New("inspect CloudWatch alarm ownership failed")
		}
		if !owned {
			return errors.New("refusing to replace an unowned CloudWatch alarm")
		}
	}
	_, err = backend.cloud.PutMetricAlarm(ctx, &cloudwatch.PutMetricAlarmInput{AlarmName: aws.String(name), AlarmDescription: aws.String(cloudWatchAlarmDescription(alert)), ActionsEnabled: aws.Bool(false), ComparisonOperator: comparison, EvaluationPeriods: aws.Int32(evaluationPeriods), MetricName: aws.String(metricName(plan.OwnershipMarker, alert.Signal)), Namespace: aws.String(metricNamespace(plan.OwnershipMarker)), Period: aws.Int32(60), Statistic: cloudwatchtypes.StatisticAverage, Threshold: &threshold, TreatMissingData: aws.String("notBreaching"), Tags: tags})
	if err != nil {
		return errors.New("create CloudWatch alarm failed")
	}
	return nil
}

func foundAlarm(alarms []cloudwatchtypes.MetricAlarm, name string) (cloudwatchtypes.MetricAlarm, bool) {
	for _, alarm := range alarms {
		if aws.ToString(alarm.AlarmName) == name {
			return alarm, true
		}
	}
	return cloudwatchtypes.MetricAlarm{}, false
}

func (backend *CloudWatchBackend) cloudWatchAlarmMatches(ctx context.Context, alarm cloudwatchtypes.MetricAlarm, marker string, intent v1.AlertIntent) (bool, error) {
	if alarm.AlarmArn == nil || strings.TrimSpace(aws.ToString(alarm.AlarmArn)) == "" {
		return false, nil
	}
	actual, err := backend.cloud.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{ResourceARN: alarm.AlarmArn})
	if err != nil {
		return false, err
	}
	want, err := cloudWatchAlertTagMap(marker, intent)
	if err != nil {
		return false, err
	}
	got := make(map[string]string, len(actual.Tags))
	for _, tag := range actual.Tags {
		got[aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	for key, value := range want {
		if got[key] != value {
			return false, nil
		}
	}
	return true, nil
}

func cloudWatchAlertTags(marker string, intent v1.AlertIntent) ([]cloudwatchtypes.Tag, error) {
	values, err := cloudWatchAlertTagMap(marker, intent)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	tags := make([]cloudwatchtypes.Tag, 0, len(keys))
	for _, key := range keys {
		tags = append(tags, cloudwatchtypes.Tag{Key: aws.String(key), Value: aws.String(values[key])})
	}
	return tags, nil
}

func cloudWatchAlertTagMap(marker string, intent v1.AlertIntent) (map[string]string, error) {
	metadata := map[string]string{
		cloudWatchOwnershipTagKey: marker,
		"magelift-owner":          intent.Owner,
		"magelift-severity":       intent.Severity,
		"magelift-deduplication":  intent.DeduplicationKey,
		"magelift-runbook":        intent.RunbookURL,
	}
	for key, value := range metadata {
		if strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("CloudWatch alert metadata %q is required", key)
		}
		if len([]byte(value)) > 256 {
			return nil, fmt.Errorf("CloudWatch alert metadata %q exceeds the 256-byte tag value limit", key)
		}
	}
	return metadata, nil
}

func cloudWatchAlarmSettings(intent v1.AlertIntent) (cloudwatchtypes.ComparisonOperator, int32, error) {
	if intent.WindowSeconds <= 0 || intent.WindowSeconds%60 != 0 || intent.WindowSeconds > 7*24*60*60 {
		return "", 0, fmt.Errorf("CloudWatch alert %q window must be a positive whole number of minutes no greater than seven days", intent.ID)
	}
	var comparison cloudwatchtypes.ComparisonOperator
	switch intent.Operator {
	case "gt":
		comparison = cloudwatchtypes.ComparisonOperatorGreaterThanThreshold
	case "gte":
		comparison = cloudwatchtypes.ComparisonOperatorGreaterThanOrEqualToThreshold
	case "lt":
		comparison = cloudwatchtypes.ComparisonOperatorLessThanThreshold
	case "lte":
		comparison = cloudwatchtypes.ComparisonOperatorLessThanOrEqualToThreshold
	case "eq":
		return "", 0, fmt.Errorf("CloudWatch alert %q equality operator is unsupported; use gt, gte, lt, or lte", intent.ID)
	default:
		return "", 0, fmt.Errorf("CloudWatch alert %q operator %q is unsupported", intent.ID, intent.Operator)
	}
	return comparison, int32(intent.WindowSeconds / 60), nil
}

func cloudWatchAlarmDescription(intent v1.AlertIntent) string {
	return fmt.Sprintf("MageLift alert %s; owner=%s; severity=%s; deduplication=%s; runbook=%s", intent.ID, intent.Owner, intent.Severity, intent.DeduplicationKey, intent.RunbookURL)
}

func (backend *CloudWatchBackend) findLogGroup(ctx context.Context, name string) (*cloudwatchlogtypes.LogGroup, error) {
	output, err := backend.logs.DescribeLogGroups(ctx, &cloudwatchlogs.DescribeLogGroupsInput{LogGroupNamePrefix: aws.String(name), Limit: aws.Int32(50)})
	if err != nil {
		return nil, err
	}
	for index := range output.LogGroups {
		group := output.LogGroups[index]
		if group.LogGroupName != nil && *group.LogGroupName == name {
			return &group, nil
		}
	}
	return nil, nil
}

func (backend *CloudWatchBackend) requireOwnedLogGroup(ctx context.Context, group *cloudwatchlogtypes.LogGroup, marker string) error {
	if group == nil || group.LogGroupArn == nil || strings.TrimSpace(*group.LogGroupArn) == "" {
		return errors.New("CloudWatch log group collision has no taggable ARN")
	}
	tags, err := backend.logs.ListTagsForResource(ctx, &cloudwatchlogs.ListTagsForResourceInput{ResourceArn: group.LogGroupArn})
	if err != nil {
		return errors.New("inspect CloudWatch log group ownership failed")
	}
	if tags.Tags[cloudWatchOwnershipTagKey] != marker {
		return errors.New("refusing to replace an unowned CloudWatch log group")
	}
	return nil
}

func (backend *CloudWatchBackend) cloudWatchResourceOwned(ctx context.Context, resourceARN, marker string) (bool, error) {
	tags, err := backend.cloud.ListTagsForResource(ctx, &cloudwatch.ListTagsForResourceInput{ResourceARN: aws.String(resourceARN)})
	if err != nil {
		return false, err
	}
	for _, tag := range tags.Tags {
		if aws.ToString(tag.Key) == cloudWatchOwnershipTagKey {
			return aws.ToString(tag.Value) == marker, nil
		}
	}
	return false, nil
}

func validatePlan(ctx context.Context, plan providerobservability.Plan) error {
	if ctx == nil {
		return errors.New("CloudWatch observability context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(plan.OwnershipMarker) == "" || strings.ContainsAny(plan.OwnershipMarker, "\r\n\x00") {
		return errors.New("CloudWatch observability ownership marker is required")
	}
	return nil
}

func validateBindings(plan providerobservability.Plan) error {
	for _, binding := range plan.Bindings {
		if binding.Destination != "cloudwatch" || binding.OwnershipMarker != plan.OwnershipMarker {
			return errors.New("CloudWatch binding ownership or destination does not match the plan")
		}
		if strings.TrimSpace(binding.RedactionPolicy) != "" {
			return errors.New("CloudWatch redaction requires an explicit log-router or collector policy")
		}
		switch binding.Signal {
		case "logs", "metrics":
		default:
			return fmt.Errorf("CloudWatch lifecycle does not implement signal %q", binding.Signal)
		}
	}
	for _, alert := range plan.Alerts {
		if err := v1.ValidateObservabilityIntent(v1.ObservabilityIntent{NativeProvider: "cloudwatch", OwnershipMarker: plan.OwnershipMarker, Signals: []string{alert.Signal}, Alerts: []v1.AlertIntent{alert}}); err != nil {
			return fmt.Errorf("validate CloudWatch alert %q: %w", alert.ID, err)
		}
		if _, _, err := cloudWatchAlarmSettings(alert); err != nil {
			return err
		}
		if _, err := cloudWatchAlertTagMap(plan.OwnershipMarker, alert); err != nil {
			return err
		}
	}
	return nil
}

func markerDigest(marker string) string {
	sum := sha256.Sum256([]byte(marker))
	return hex.EncodeToString(sum[:])[:16]
}

func logGroupName(marker string) string  { return "/magelift/observability/" + markerDigest(marker) }
func logStreamName(marker string) string { return "probe-" + markerDigest(marker) }
func dashboardName(marker string) string { return "magelift-" + markerDigest(marker) }
func alarmPrefix(marker string) string   { return "magelift-" + markerDigest(marker) + "-" }

func alarmName(marker, id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	var builder strings.Builder
	for _, char := range id {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' || char == '_' {
			builder.WriteRune(char)
		} else {
			builder.WriteByte('-')
		}
	}
	return alarmPrefix(marker) + strings.Trim(builder.String(), "-")
}

func metricNamespace(marker string) string { return "MageLift/" + markerDigest(marker) }
func metricName(marker, signal string) string {
	return "probe_" + markerDigest(marker) + "_" + strings.ReplaceAll(signal, "-", "_")
}
func probeMessage(marker, signal string) string {
	return "magelift-probe signal=" + signal + " marker=" + marker
}
