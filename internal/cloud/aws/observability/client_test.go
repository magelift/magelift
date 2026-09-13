package observability

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	cloudwatchtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cloudwatchlogtypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	providerobservability "github.com/magelift/magelift/internal/external/observability"
	"github.com/magelift/magelift/sdk/v1"
)

type fakeCloudWatchLogs struct {
	groups  map[string]cloudwatchlogtypes.LogGroup
	tags    map[string]map[string]string
	streams map[string]map[string]bool
	events  map[string][]cloudwatchlogtypes.FilteredLogEvent
}

func newFakeCloudWatchLogs() *fakeCloudWatchLogs {
	return &fakeCloudWatchLogs{groups: map[string]cloudwatchlogtypes.LogGroup{}, tags: map[string]map[string]string{}, streams: map[string]map[string]bool{}, events: map[string][]cloudwatchlogtypes.FilteredLogEvent{}}
}

func (f *fakeCloudWatchLogs) CreateLogGroup(_ context.Context, input *cloudwatchlogs.CreateLogGroupInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogGroupOutput, error) {
	name := aws.ToString(input.LogGroupName)
	if _, exists := f.groups[name]; exists {
		return nil, errors.New("already exists")
	}
	arn := "arn:aws:logs:eu-west-3:123456789012:log-group:" + name
	f.groups[name] = cloudwatchlogtypes.LogGroup{LogGroupName: aws.String(name), LogGroupArn: aws.String(arn)}
	f.tags[arn] = input.Tags
	return &cloudwatchlogs.CreateLogGroupOutput{}, nil
}

func (f *fakeCloudWatchLogs) PutRetentionPolicy(_ context.Context, input *cloudwatchlogs.PutRetentionPolicyInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutRetentionPolicyOutput, error) {
	name := aws.ToString(input.LogGroupName)
	group, exists := f.groups[name]
	if !exists {
		return nil, errors.New("group missing")
	}
	group.RetentionInDays = input.RetentionInDays
	f.groups[name] = group
	return &cloudwatchlogs.PutRetentionPolicyOutput{}, nil
}

func (f *fakeCloudWatchLogs) CreateLogStream(_ context.Context, input *cloudwatchlogs.CreateLogStreamInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.CreateLogStreamOutput, error) {
	group := aws.ToString(input.LogGroupName)
	stream := aws.ToString(input.LogStreamName)
	if f.streams[group] == nil {
		f.streams[group] = map[string]bool{}
	}
	if f.streams[group][stream] {
		return nil, errors.New("already exists")
	}
	f.streams[group][stream] = true
	return &cloudwatchlogs.CreateLogStreamOutput{}, nil
}

func (f *fakeCloudWatchLogs) DescribeLogStreams(_ context.Context, input *cloudwatchlogs.DescribeLogStreamsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogStreamsOutput, error) {
	group := aws.ToString(input.LogGroupName)
	output := &cloudwatchlogs.DescribeLogStreamsOutput{}
	for stream := range f.streams[group] {
		output.LogStreams = append(output.LogStreams, cloudwatchlogtypes.LogStream{LogStreamName: aws.String(stream)})
	}
	return output, nil
}

func (f *fakeCloudWatchLogs) PutLogEvents(_ context.Context, input *cloudwatchlogs.PutLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.PutLogEventsOutput, error) {
	group := aws.ToString(input.LogGroupName)
	stream := aws.ToString(input.LogStreamName)
	if !f.streams[group][stream] {
		return nil, errors.New("stream missing")
	}
	for _, event := range input.LogEvents {
		f.events[group] = append(f.events[group], cloudwatchlogtypes.FilteredLogEvent{Message: event.Message})
	}
	return &cloudwatchlogs.PutLogEventsOutput{}, nil
}

func (f *fakeCloudWatchLogs) FilterLogEvents(_ context.Context, input *cloudwatchlogs.FilterLogEventsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.FilterLogEventsOutput, error) {
	return &cloudwatchlogs.FilterLogEventsOutput{Events: f.events[aws.ToString(input.LogGroupName)]}, nil
}

func (f *fakeCloudWatchLogs) DescribeLogGroups(_ context.Context, input *cloudwatchlogs.DescribeLogGroupsInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DescribeLogGroupsOutput, error) {
	output := &cloudwatchlogs.DescribeLogGroupsOutput{}
	for name, group := range f.groups {
		if len(aws.ToString(input.LogGroupNamePrefix)) == 0 || len(name) >= len(aws.ToString(input.LogGroupNamePrefix)) && name[:len(aws.ToString(input.LogGroupNamePrefix))] == aws.ToString(input.LogGroupNamePrefix) {
			group.LogGroupName = aws.String(name)
			output.LogGroups = append(output.LogGroups, group)
		}
	}
	return output, nil
}

func (f *fakeCloudWatchLogs) ListTagsForResource(_ context.Context, input *cloudwatchlogs.ListTagsForResourceInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.ListTagsForResourceOutput, error) {
	tags := f.tags[aws.ToString(input.ResourceArn)]
	return &cloudwatchlogs.ListTagsForResourceOutput{Tags: tags}, nil
}

func (f *fakeCloudWatchLogs) DeleteLogGroup(_ context.Context, input *cloudwatchlogs.DeleteLogGroupInput, _ ...func(*cloudwatchlogs.Options)) (*cloudwatchlogs.DeleteLogGroupOutput, error) {
	delete(f.groups, aws.ToString(input.LogGroupName))
	return &cloudwatchlogs.DeleteLogGroupOutput{}, nil
}

type fakeCloudWatch struct {
	dashboards    map[string]bool
	dashboardARNs map[string]string
	alarms        map[string]bool
	alarmARNs     map[string]string
	tags          map[string]map[string]string
	metrics       map[string]bool
}

func newFakeCloudWatch() *fakeCloudWatch {
	return &fakeCloudWatch{dashboards: map[string]bool{}, dashboardARNs: map[string]string{}, alarms: map[string]bool{}, alarmARNs: map[string]string{}, tags: map[string]map[string]string{}, metrics: map[string]bool{}}
}

func (f *fakeCloudWatch) PutMetricData(_ context.Context, input *cloudwatch.PutMetricDataInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricDataOutput, error) {
	for _, datum := range input.MetricData {
		f.metrics[aws.ToString(datum.MetricName)] = true
	}
	return &cloudwatch.PutMetricDataOutput{}, nil
}

func (f *fakeCloudWatch) GetMetricStatistics(_ context.Context, input *cloudwatch.GetMetricStatisticsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetMetricStatisticsOutput, error) {
	if !f.metrics[aws.ToString(input.MetricName)] {
		return &cloudwatch.GetMetricStatisticsOutput{}, nil
	}
	return &cloudwatch.GetMetricStatisticsOutput{Datapoints: []cloudwatchtypes.Datapoint{{Average: aws.Float64(1)}}}, nil
}

func (f *fakeCloudWatch) PutDashboard(_ context.Context, input *cloudwatch.PutDashboardInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutDashboardOutput, error) {
	name := aws.ToString(input.DashboardName)
	f.dashboards[name] = true
	f.dashboardARNs[name] = "arn:aws:cloudwatch::123456789012:dashboard/" + name
	return &cloudwatch.PutDashboardOutput{}, nil
}

func (f *fakeCloudWatch) GetDashboard(_ context.Context, input *cloudwatch.GetDashboardInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.GetDashboardOutput, error) {
	if !f.dashboards[aws.ToString(input.DashboardName)] {
		return nil, errors.New("dashboard missing")
	}
	return &cloudwatch.GetDashboardOutput{}, nil
}

func (f *fakeCloudWatch) DeleteDashboards(_ context.Context, input *cloudwatch.DeleteDashboardsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DeleteDashboardsOutput, error) {
	for _, name := range input.DashboardNames {
		delete(f.dashboards, name)
		delete(f.dashboardARNs, name)
	}
	return &cloudwatch.DeleteDashboardsOutput{}, nil
}

func (f *fakeCloudWatch) ListDashboards(_ context.Context, input *cloudwatch.ListDashboardsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListDashboardsOutput, error) {
	output := &cloudwatch.ListDashboardsOutput{}
	for name := range f.dashboards {
		output.DashboardEntries = append(output.DashboardEntries, cloudwatchtypes.DashboardEntry{DashboardName: aws.String(name), DashboardArn: aws.String(f.dashboardARNs[name])})
	}
	return output, nil
}

func (f *fakeCloudWatch) PutMetricAlarm(_ context.Context, input *cloudwatch.PutMetricAlarmInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.PutMetricAlarmOutput, error) {
	name := aws.ToString(input.AlarmName)
	f.alarms[name] = true
	arn := "arn:aws:cloudwatch:eu-west-3:123456789012:alarm:" + name
	f.alarmARNs[name] = arn
	if f.tags[arn] == nil {
		f.tags[arn] = map[string]string{}
	}
	for _, tag := range input.Tags {
		f.tags[arn][aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return &cloudwatch.PutMetricAlarmOutput{}, nil
}

func (f *fakeCloudWatch) DescribeAlarms(_ context.Context, input *cloudwatch.DescribeAlarmsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DescribeAlarmsOutput, error) {
	output := &cloudwatch.DescribeAlarmsOutput{}
	prefix := aws.ToString(input.AlarmNamePrefix)
	for name := range f.alarms {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			output.MetricAlarms = append(output.MetricAlarms, cloudwatchtypes.MetricAlarm{AlarmName: aws.String(name), AlarmArn: aws.String(f.alarmARNs[name])})
		}
	}
	return output, nil
}

func (f *fakeCloudWatch) DeleteAlarms(_ context.Context, input *cloudwatch.DeleteAlarmsInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.DeleteAlarmsOutput, error) {
	for _, name := range input.AlarmNames {
		delete(f.alarms, name)
		delete(f.alarmARNs, name)
	}
	return &cloudwatch.DeleteAlarmsOutput{}, nil
}

func (f *fakeCloudWatch) ListTagsForResource(_ context.Context, input *cloudwatch.ListTagsForResourceInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.ListTagsForResourceOutput, error) {
	var tags []cloudwatchtypes.Tag
	for key, value := range f.tags[aws.ToString(input.ResourceARN)] {
		tags = append(tags, cloudwatchtypes.Tag{Key: aws.String(key), Value: aws.String(value)})
	}
	return &cloudwatch.ListTagsForResourceOutput{Tags: tags}, nil
}

func (f *fakeCloudWatch) TagResource(_ context.Context, input *cloudwatch.TagResourceInput, _ ...func(*cloudwatch.Options)) (*cloudwatch.TagResourceOutput, error) {
	arn := aws.ToString(input.ResourceARN)
	if f.tags[arn] == nil {
		f.tags[arn] = map[string]string{}
	}
	for _, tag := range input.Tags {
		f.tags[arn][aws.ToString(tag.Key)] = aws.ToString(tag.Value)
	}
	return &cloudwatch.TagResourceOutput{}, nil
}

func TestCloudWatchBackendUsesOfficialSDKSurfaceAndOwnsOnlyMarkerResources(t *testing.T) {
	logs := newFakeCloudWatchLogs()
	cloud := newFakeCloudWatch()
	backend, err := NewCloudWatchBackend(logs, cloud)
	if err != nil {
		t.Fatal(err)
	}
	backend.now = func() time.Time { return time.Unix(1700000000, 0).UTC() }
	plan := providerobservability.Plan{
		OwnershipMarker: "magelift/aws/observability-test",
		Bindings: []providerobservability.SignalBinding{
			{Signal: "logs", Destination: "cloudwatch", Mode: "native", OwnershipMarker: "magelift/aws/observability-test", RetentionDays: 30},
			{Signal: "metrics", Destination: "cloudwatch", Mode: "native", OwnershipMarker: "magelift/aws/observability-test"},
		},
		Dashboards: []v1.DashboardIntent{{ID: "runtime", Signals: []string{"metrics"}}},
		Alerts:     []v1.AlertIntent{{ID: "runtime-health", Signal: "metrics", Severity: "warning", Operator: "gt", Threshold: 1, WindowSeconds: 60, Owner: "platform-oncall", RunbookURL: "https://runbooks.example/runtime-health", DeduplicationKey: "runtime-health"}},
	}
	result, err := backend.Apply(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !result.OwnershipVerified || !result.IdempotencyVerified || len(result.ResourceRefs) != 5 {
		t.Fatalf("apply result = %#v", result)
	}
	for _, binding := range plan.Bindings {
		observation, err := backend.VerifySignal(context.Background(), binding)
		if err != nil {
			t.Fatal(err)
		}
		if !observation.Delivered || !observation.LabelsVerified || !observation.RetentionVerified || !observation.RedactionVerified {
			t.Fatalf("signal observation = %#v", observation)
		}
	}
	operations, err := backend.VerifyOperations(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	if !operations.AlertsVerified || !operations.DashboardsVerified || !operations.SLOsVerified {
		t.Fatalf("operations = %#v", operations)
	}
	alarmTags := cloud.tags[cloud.alarmARNs[alarmName(plan.OwnershipMarker, "runtime-health")]]
	for key, want := range map[string]string{"magelift-ownership": plan.OwnershipMarker, "magelift-owner": "platform-oncall", "magelift-severity": "warning", "magelift-deduplication": "runtime-health", "magelift-runbook": "https://runbooks.example/runtime-health"} {
		if alarmTags[key] != want {
			t.Fatalf("alarm tag %q = %q, want %q; tags=%v", key, alarmTags[key], want, alarmTags)
		}
	}
	inventory, err := backend.Inventory(context.Background(), plan.OwnershipMarker)
	if err != nil || len(inventory) != 3 {
		t.Fatalf("inventory = %#v, err=%v", inventory, err)
	}
	if err := backend.Destroy(context.Background(), plan, result.ResourceRefs); err != nil {
		t.Fatal(err)
	}
	inventory, err = backend.Inventory(context.Background(), plan.OwnershipMarker)
	if err != nil || len(inventory) != 0 {
		t.Fatalf("post-destroy inventory = %#v, err=%v", inventory, err)
	}
}

func TestCloudWatchAlarmSettingsMapsSupportedOperatorsAndRejectsEquality(t *testing.T) {
	for _, test := range []struct {
		operator string
		want     cloudwatchtypes.ComparisonOperator
	}{
		{operator: "gt", want: cloudwatchtypes.ComparisonOperatorGreaterThanThreshold},
		{operator: "gte", want: cloudwatchtypes.ComparisonOperatorGreaterThanOrEqualToThreshold},
		{operator: "lt", want: cloudwatchtypes.ComparisonOperatorLessThanThreshold},
		{operator: "lte", want: cloudwatchtypes.ComparisonOperatorLessThanOrEqualToThreshold},
	} {
		t.Run(test.operator, func(t *testing.T) {
			got, periods, err := cloudWatchAlarmSettings(v1.AlertIntent{ID: "test", Operator: test.operator, WindowSeconds: 5 * 60})
			if err != nil || got != test.want || periods != 5 {
				t.Fatalf("settings = %q, %d, %v; want %q, 5, nil", got, periods, err, test.want)
			}
		})
	}
	for _, intent := range []v1.AlertIntent{
		{ID: "equality", Operator: "eq", WindowSeconds: 60},
		{ID: "partial-minute", Operator: "gt", WindowSeconds: 61},
		{ID: "too-long", Operator: "gt", WindowSeconds: 7*24*60*60 + 60},
	} {
		if _, _, err := cloudWatchAlarmSettings(intent); err == nil {
			t.Fatalf("unsupported alarm settings accepted: %#v", intent)
		}
	}
}

func TestCloudWatchBackendRejectsBindingWithWrongOwnershipBeforeMutation(t *testing.T) {
	logs := newFakeCloudWatchLogs()
	cloud := newFakeCloudWatch()
	backend, err := NewCloudWatchBackend(logs, cloud)
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: "magelift/test", Bindings: []providerobservability.SignalBinding{{Signal: "metrics", Destination: "cloudwatch", OwnershipMarker: "other"}}})
	if err == nil {
		t.Fatal("wrong ownership was accepted")
	}
	if len(cloud.metrics) != 0 || len(logs.groups) != 0 {
		t.Fatalf("provider was mutated before ownership rejection: metrics=%v groups=%v", cloud.metrics, logs.groups)
	}
}

func TestCloudWatchBackendRefusesSameNameForeignResources(t *testing.T) {
	marker := "magelift/aws/foreign-collision"
	t.Run("log group", func(t *testing.T) {
		logs := newFakeCloudWatchLogs()
		name := logGroupName(marker)
		arn := "arn:aws:logs:eu-west-3:123456789012:log-group:" + name
		logs.groups[name] = cloudwatchlogtypes.LogGroup{LogGroupName: aws.String(name), LogGroupArn: aws.String(arn)}
		logs.tags[arn] = map[string]string{cloudWatchOwnershipTagKey: "other-owner"}
		cloud := newFakeCloudWatch()
		backend, err := NewCloudWatchBackend(logs, cloud)
		if err != nil {
			t.Fatal(err)
		}
		_, err = backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: marker, Bindings: []providerobservability.SignalBinding{{Signal: "logs", Destination: "cloudwatch", OwnershipMarker: marker}}})
		if err == nil || !strings.Contains(err.Error(), "unowned") {
			t.Fatalf("apply error = %v, want foreign log-group refusal", err)
		}
		if len(cloud.metrics) != 0 {
			t.Fatalf("foreign log-group apply continued into later mutations: %#v", cloud.metrics)
		}
	})

	t.Run("dashboard", func(t *testing.T) {
		logs := newFakeCloudWatchLogs()
		cloud := newFakeCloudWatch()
		name := dashboardName(marker)
		arn := "arn:aws:cloudwatch::123456789012:dashboard/" + name
		cloud.dashboards[name] = true
		cloud.dashboardARNs[name] = arn
		cloud.tags[arn] = map[string]string{cloudWatchOwnershipTagKey: "other-owner"}
		backend, err := NewCloudWatchBackend(logs, cloud)
		if err != nil {
			t.Fatal(err)
		}
		_, err = backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: marker, Dashboards: []v1.DashboardIntent{{ID: "runtime", Signals: []string{"metrics"}}}})
		if err == nil || !strings.Contains(err.Error(), "unowned") {
			t.Fatalf("apply error = %v, want foreign dashboard refusal", err)
		}
	})

	t.Run("alarm", func(t *testing.T) {
		logs := newFakeCloudWatchLogs()
		cloud := newFakeCloudWatch()
		name := alarmName(marker, "runtime-health")
		arn := "arn:aws:cloudwatch:eu-west-3:123456789012:alarm:" + name
		cloud.alarms[name] = true
		cloud.alarmARNs[name] = arn
		cloud.tags[arn] = map[string]string{cloudWatchOwnershipTagKey: "other-owner"}
		backend, err := NewCloudWatchBackend(logs, cloud)
		if err != nil {
			t.Fatal(err)
		}
		_, err = backend.Apply(context.Background(), providerobservability.Plan{OwnershipMarker: marker, Alerts: []v1.AlertIntent{{ID: "runtime-health", Signal: "metrics", Severity: "warning", Operator: "gt", Threshold: 1, WindowSeconds: 60, Owner: "platform-oncall", RunbookURL: "https://runbooks.example/runtime-health", DeduplicationKey: "runtime-health"}}})
		if err == nil || !strings.Contains(err.Error(), "unowned") {
			t.Fatalf("apply error = %v, want foreign alarm refusal", err)
		}
	})
}
