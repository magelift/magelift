package observability

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/s3"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/synthetics"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:Observability"

var kmsARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
var topicARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):sns:[a-z0-9-]+:[0-9]{12}:[A-Za-z0-9._-]+$`)

type Args struct {
	Region                         string
	EnvironmentClass               string
	LogGroupPrefix                 string
	KMSKeyARN                      string
	RetentionInDays                int
	ECSClusterName                 string
	ECSClusterNameInput            pulumi.StringInput
	ECSServiceName                 string
	ECSServiceNameInput            pulumi.StringInput
	DesiredTaskCount               int
	LoadBalancerDimension          string
	LoadBalancerDimensionInput     pulumi.StringInput
	NotificationTopicARN           string
	SyntheticEnabled               bool
	SyntheticURL                   string
	SyntheticArtifactRetentionDays int
	Tags                           map[string]string
}

type Component struct {
	pulumi.ResourceState
	WebLogGroupARN     pulumi.StringOutput `pulumi:"webLogGroupArn"`
	DeployLogGroupARN  pulumi.StringOutput `pulumi:"deployLogGroupArn"`
	CronLogGroupARN    pulumi.StringOutput `pulumi:"cronLogGroupArn"`
	DashboardName      pulumi.StringOutput `pulumi:"dashboardName"`
	AlarmARNs          pulumi.ArrayOutput  `pulumi:"alarmArns"`
	SyntheticCanaryARN pulumi.StringOutput `pulumi:"syntheticCanaryArn"`
	SyntheticBucketARN pulumi.StringOutput `pulumi:"syntheticBucketArn"`
}

type syntheticArtifactStore struct {
	bucket     *s3.Bucket
	versioning *s3.BucketVersioning
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, opts...); err != nil {
		return nil, err
	}
	child := pulumi.Parent(component)
	logGroups := make([]*cloudwatch.LogGroup, 0, 3)
	for _, role := range []string{"web", "deploy", "cron"} {
		group, err := cloudwatch.NewLogGroup(ctx, name+"-"+role+"-logs", &cloudwatch.LogGroupArgs{
			Name: pulumi.String(args.LogGroupPrefix + "/" + role), Region: pulumi.String(args.Region), KmsKeyId: pulumi.String(args.KMSKeyARN),
			RetentionInDays: pulumi.Int(args.RetentionInDays), DeletionProtectionEnabled: pulumi.Bool(args.EnvironmentClass == "production"), SkipDestroy: pulumi.Bool(args.EnvironmentClass == "production"), Tags: pulumi.ToStringMap(tags(args.Tags, name, role)),
		}, child)
		if err != nil {
			return nil, fmt.Errorf("create %s log group: %w", role, err)
		}
		logGroups = append(logGroups, group)
	}
	dashboard, err := cloudwatch.NewDashboard(ctx, name+"-dashboard", &cloudwatch.DashboardArgs{DashboardName: pulumi.String(name), DashboardBody: dashboardBodyInput(args), Region: pulumi.String(args.Region)}, child)
	if err != nil {
		return nil, fmt.Errorf("create observability dashboard: %w", err)
	}
	actions := pulumi.Array{}
	if args.NotificationTopicARN != "" {
		actions = pulumi.Array{pulumi.String(args.NotificationTopicARN)}
	}
	alarms := []*cloudwatch.MetricAlarm{}
	ecsDimensions := ecsDimensions(args)
	cpu, err := newAlarm(ctx, name+"-ecs-cpu", "ECS CPU is high", args, cloudwatch.MetricAlarmArgs{
		MetricName: pulumi.String("CPUUtilization"), Namespace: pulumi.String("AWS/ECS"), Statistic: pulumi.String("Average"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(5), DatapointsToAlarm: pulumi.Int(3), ComparisonOperator: pulumi.String("GreaterThanThreshold"), Threshold: pulumi.Float64(80), Dimensions: ecsDimensions, AlarmActions: actions,
	}, child)
	if err != nil {
		return nil, err
	}
	alarms = append(alarms, cpu)
	running, err := newAlarm(ctx, name+"-ecs-running", "ECS service has too few tasks", args, cloudwatch.MetricAlarmArgs{
		MetricName: pulumi.String("RunningTaskCount"), Namespace: pulumi.String("ECS/ContainerInsights"), Statistic: pulumi.String("Minimum"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(5), DatapointsToAlarm: pulumi.Int(3), ComparisonOperator: pulumi.String("LessThanThreshold"), Threshold: pulumi.Float64(float64(args.DesiredTaskCount)), Dimensions: ecsDimensions, AlarmActions: actions,
	}, child)
	if err != nil {
		return nil, err
	}
	alarms = append(alarms, running)
	if hasLoadBalancerDimension(args) {
		target5xx, err := newAlarm(ctx, name+"-alb-5xx", "ALB target errors are high", args, cloudwatch.MetricAlarmArgs{
			MetricName: pulumi.String("HTTPCode_Target_5XX_Count"), Namespace: pulumi.String("AWS/ApplicationELB"), Statistic: pulumi.String("Sum"), Period: pulumi.Int(60), EvaluationPeriods: pulumi.Int(5), DatapointsToAlarm: pulumi.Int(3), ComparisonOperator: pulumi.String("GreaterThanThreshold"), Threshold: pulumi.Float64(5), Dimensions: loadBalancerDimensions(args), AlarmActions: actions,
		}, child)
		if err != nil {
			return nil, err
		}
		alarms = append(alarms, target5xx)
	}
	var syntheticCanary *synthetics.Canary
	var syntheticBucket *s3.Bucket
	if args.SyntheticEnabled {
		artifactStore, err := newSyntheticArtifactBucket(ctx, name, args, child)
		if err != nil {
			return nil, err
		}
		syntheticBucket = artifactStore.bucket
		syntheticCanary, err = newSyntheticCanary(ctx, name, args, artifactStore, child)
		if err != nil {
			return nil, err
		}
		syntheticAlarm, err := newAlarm(ctx, name+"-synthetic", "Synthetic health check is failing", args, cloudwatch.MetricAlarmArgs{
			MetricName: pulumi.String("SuccessPercent"), Namespace: pulumi.String("CloudWatchSynthetics"), Statistic: pulumi.String("Average"), Period: pulumi.Int(300), EvaluationPeriods: pulumi.Int(3), DatapointsToAlarm: pulumi.Int(2), ComparisonOperator: pulumi.String("LessThanThreshold"), Threshold: pulumi.Float64(100), Dimensions: pulumi.StringMap{"CanaryName": pulumi.String(name + "-synthetic")}, AlarmActions: actions, TreatMissingData: pulumi.String("breaching"),
		}, child)
		if err != nil {
			return nil, err
		}
		alarms = append(alarms, syntheticAlarm)
	}
	component.WebLogGroupARN, component.DeployLogGroupARN, component.CronLogGroupARN = logGroups[0].Arn, logGroups[1].Arn, logGroups[2].Arn
	component.DashboardName = dashboard.DashboardName
	component.AlarmARNs = alarmIDs(alarms)
	if syntheticCanary != nil {
		component.SyntheticCanaryARN = syntheticCanary.Arn
		component.SyntheticBucketARN = syntheticBucket.Arn
	}
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"webLogGroupArn": component.WebLogGroupARN, "deployLogGroupArn": component.DeployLogGroupARN, "cronLogGroupArn": component.CronLogGroupARN, "dashboardName": component.DashboardName, "alarmArns": component.AlarmARNs, "syntheticCanaryArn": component.SyntheticCanaryARN, "syntheticBucketArn": component.SyntheticBucketARN}); err != nil {
		return nil, err
	}
	return component, nil
}

func newAlarm(ctx *pulumi.Context, name, description string, args Args, alarmArgs cloudwatch.MetricAlarmArgs, parent pulumi.ResourceOption) (*cloudwatch.MetricAlarm, error) {
	alarmArgs.Name, alarmArgs.AlarmDescription, alarmArgs.Region, alarmArgs.Tags = pulumi.String(name), pulumi.String(description), pulumi.String(args.Region), pulumi.ToStringMap(tags(args.Tags, name, "alarm"))
	if alarmArgs.TreatMissingData == nil {
		alarmArgs.TreatMissingData = pulumi.String("notBreaching")
	}
	alarm, err := cloudwatch.NewMetricAlarm(ctx, name, &alarmArgs, parent)
	if err != nil {
		return nil, fmt.Errorf("create alarm %s: %w", name, err)
	}
	return alarm, nil
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" || strings.TrimSpace(args.LogGroupPrefix) == "" || !hasECSNames(args) {
		return errors.New("observability requires a name, region, log group prefix, ECS cluster, and ECS service")
	}
	if !kmsARNPattern.MatchString(args.KMSKeyARN) || args.DesiredTaskCount < 1 {
		return errors.New("observability requires a KMS key ARN and a positive desired task count")
	}
	allowedRetention := map[int]bool{1: true, 3: true, 5: true, 7: true, 14: true, 30: true, 60: true, 90: true, 120: true, 150: true, 180: true, 365: true, 400: true, 545: true, 731: true, 1096: true, 1827: true, 2192: true, 2557: true, 2922: true, 3288: true, 3653: true}
	if !allowedRetention[args.RetentionInDays] {
		return errors.New("observability log retention must use an AWS-supported retention period")
	}
	if args.NotificationTopicARN != "" && !topicARNPattern.MatchString(args.NotificationTopicARN) {
		return errors.New("observability notification topic must be an SNS ARN")
	}
	if args.SyntheticEnabled {
		if args.EnvironmentClass != "production" {
			return errors.New("synthetic health checks are enabled only for production environments")
		}
		target, err := url.ParseRequestURI(strings.TrimSpace(args.SyntheticURL))
		if err != nil || target.Scheme != "https" || target.Host == "" {
			return errors.New("synthetic health check URL must be an HTTPS URL")
		}
		if args.SyntheticArtifactRetentionDays < 1 {
			return errors.New("synthetic artifact retention must be positive")
		}
	}
	return nil
}

func newSyntheticArtifactBucket(ctx *pulumi.Context, name string, args Args, parent pulumi.ResourceOption) (*syntheticArtifactStore, error) {
	bucket, err := s3.NewBucket(ctx, name+"-synthetic-artifacts", &s3.BucketArgs{
		Region: pulumi.String(args.Region), ForceDestroy: pulumi.Bool(true), Tags: pulumi.ToStringMap(tags(args.Tags, name, "synthetic-artifacts")),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create synthetic artifact bucket: %w", err)
	}
	if _, err := s3.NewBucketOwnershipControls(ctx, name+"-synthetic-ownership", &s3.BucketOwnershipControlsArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), Rule: &s3.BucketOwnershipControlsRuleArgs{ObjectOwnership: pulumi.String("BucketOwnerEnforced")}}, parent); err != nil {
		return nil, fmt.Errorf("configure synthetic artifact bucket ownership: %w", err)
	}
	if _, err := s3.NewBucketPublicAccessBlock(ctx, name+"-synthetic-public-access", &s3.BucketPublicAccessBlockArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), BlockPublicAcls: pulumi.Bool(true), BlockPublicPolicy: pulumi.Bool(true), IgnorePublicAcls: pulumi.Bool(true), RestrictPublicBuckets: pulumi.Bool(true)}, parent); err != nil {
		return nil, fmt.Errorf("configure synthetic artifact bucket public access: %w", err)
	}
	versioning, err := s3.NewBucketVersioning(ctx, name+"-synthetic-versioning", &s3.BucketVersioningArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), VersioningConfiguration: &s3.BucketVersioningVersioningConfigurationArgs{Status: pulumi.String("Enabled")}}, parent)
	if err != nil {
		return nil, fmt.Errorf("configure synthetic artifact bucket versioning: %w", err)
	}
	if _, err := s3.NewBucketServerSideEncryptionConfiguration(ctx, name+"-synthetic-encryption", &s3.BucketServerSideEncryptionConfigurationArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), Rules: s3.BucketServerSideEncryptionConfigurationRuleArray{s3.BucketServerSideEncryptionConfigurationRuleArgs{ApplyServerSideEncryptionByDefault: &s3.BucketServerSideEncryptionConfigurationRuleApplyServerSideEncryptionByDefaultArgs{SseAlgorithm: pulumi.String("aws:kms"), KmsMasterKeyId: pulumi.String(args.KMSKeyARN)}, BucketKeyEnabled: pulumi.Bool(true), BlockedEncryptionTypes: pulumi.StringArray{pulumi.String("SSE-C")}}}}, parent); err != nil {
		return nil, fmt.Errorf("configure synthetic artifact bucket encryption: %w", err)
	}
	if _, err := s3.NewBucketLifecycleConfiguration(ctx, name+"-synthetic-lifecycle", &s3.BucketLifecycleConfigurationArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), Rules: s3.BucketLifecycleConfigurationRuleArray{s3.BucketLifecycleConfigurationRuleArgs{Id: pulumi.String("synthetic-retention"), Status: pulumi.String("Enabled"), Filter: &s3.BucketLifecycleConfigurationRuleFilterArgs{Prefix: pulumi.String("synthetics/")}, Expiration: &s3.BucketLifecycleConfigurationRuleExpirationArgs{Days: pulumi.Int(args.SyntheticArtifactRetentionDays)}, NoncurrentVersionExpiration: &s3.BucketLifecycleConfigurationRuleNoncurrentVersionExpirationArgs{NoncurrentDays: pulumi.Int(args.SyntheticArtifactRetentionDays)}, AbortIncompleteMultipartUpload: &s3.BucketLifecycleConfigurationRuleAbortIncompleteMultipartUploadArgs{DaysAfterInitiation: pulumi.Int(7)}}}}, parent, pulumi.DependsOn([]pulumi.Resource{versioning})); err != nil {
		return nil, fmt.Errorf("configure synthetic artifact bucket lifecycle: %w", err)
	}
	policy := bucket.Arn.ApplyT(func(bucketARN string) (string, error) {
		return syntheticBucketPolicy(bucketARN)
	}).(pulumi.StringOutput)
	if _, err := s3.NewBucketPolicy(ctx, name+"-synthetic-policy", &s3.BucketPolicyArgs{Bucket: bucket.ID(), Region: pulumi.String(args.Region), Policy: policy}, parent); err != nil {
		return nil, fmt.Errorf("restrict synthetic artifact bucket transport: %w", err)
	}
	return &syntheticArtifactStore{bucket: bucket, versioning: versioning}, nil
}

func syntheticBucketPolicy(bucketARN string) (string, error) {
	policy := map[string]any{"Version": "2012-10-17", "Statement": []any{map[string]any{
		"Sid": "DenyInsecureTransport", "Effect": "Deny", "Principal": "*", "Action": "s3:*", "Resource": []string{bucketARN, bucketARN + "/*"},
		"Condition": map[string]any{"Bool": map[string]bool{"aws:SecureTransport": false}},
	}}}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func newSyntheticCanary(ctx *pulumi.Context, name string, args Args, store *syntheticArtifactStore, parent pulumi.ResourceOption) (*synthetics.Canary, error) {
	bucket := store.bucket
	zipBase64, sourceHash, err := syntheticScript(args.SyntheticURL)
	if err != nil {
		return nil, fmt.Errorf("package synthetic health check: %w", err)
	}
	role, err := iam.NewRole(ctx, name+"-synthetic-role", &iam.RoleArgs{AssumeRolePolicy: pulumi.String(`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`), Tags: pulumi.ToStringMap(tags(args.Tags, name, "synthetic-role"))}, parent)
	if err != nil {
		return nil, fmt.Errorf("create synthetic execution role: %w", err)
	}
	policy := pulumi.All(bucket.Arn, bucket.ID()).ApplyT(func(values []interface{}) (string, error) {
		bucketARN, _ := values[0].(string)
		return syntheticExecutionPolicy(args, name, bucketARN)
	}).(pulumi.StringOutput)
	rolePolicy, err := iam.NewRolePolicy(ctx, name+"-synthetic-policy", &iam.RolePolicyArgs{Role: role.Name, Policy: policy}, parent)
	if err != nil {
		return nil, fmt.Errorf("grant synthetic execution permissions: %w", err)
	}
	script, err := s3.NewBucketObject(ctx, name+"-synthetic-script", &s3.BucketObjectArgs{Bucket: bucket.ID(), Key: pulumi.String("synthetics-scripts/" + name + "/health.zip"), ContentBase64: pulumi.String(zipBase64), ContentType: pulumi.String("application/zip"), ServerSideEncryption: pulumi.String("aws:kms"), KmsKeyId: pulumi.String(args.KMSKeyARN), SourceHash: pulumi.String(sourceHash), Tags: pulumi.ToStringMap(tags(args.Tags, name, "synthetic-script"))}, parent, pulumi.DependsOn([]pulumi.Resource{rolePolicy, store.versioning}))
	if err != nil {
		return nil, fmt.Errorf("upload synthetic health check: %w", err)
	}
	canary, err := synthetics.NewCanary(ctx, name+"-synthetic", &synthetics.CanaryArgs{ArtifactS3Location: pulumi.Sprintf("s3://%s/synthetics/", bucket.ID()), ExecutionRoleArn: role.Arn, Handler: pulumi.String("index.handler"), RuntimeVersion: pulumi.String("syn-nodejs-puppeteer-11.0"), S3Bucket: bucket.ID(), S3Key: script.Key, S3Version: script.VersionId, Schedule: &synthetics.CanaryScheduleArgs{Expression: pulumi.String("rate(5 minutes)")}, RunConfig: &synthetics.CanaryRunConfigArgs{MemoryInMb: pulumi.Int(1024), TimeoutInSeconds: pulumi.Int(60)}, StartCanary: pulumi.Bool(true), DeleteLambda: pulumi.Bool(true), SuccessRetentionPeriod: pulumi.Int(args.SyntheticArtifactRetentionDays), FailureRetentionPeriod: pulumi.Int(args.SyntheticArtifactRetentionDays), Tags: pulumi.ToStringMap(tags(args.Tags, name, "synthetic"))}, parent, pulumi.DependsOn([]pulumi.Resource{script, rolePolicy}))
	if err != nil {
		return nil, fmt.Errorf("create synthetic health check: %w", err)
	}
	return canary, nil
}

func syntheticScript(target string) (string, string, error) {
	script := "const synthetics = require('@aws/synthetics-puppeteer');\n\nexports.handler = async () => {\n  const page = await synthetics.getPage();\n  await synthetics.executeStep('health', async () => {\n    const response = await page.goto(" + strconv.Quote(target) + ", { waitUntil: 'networkidle0', timeout: 30000 });\n    if (!response || !response.ok()) {\n      throw new Error('health endpoint returned a non-success response');\n    }\n  });\n};\n"
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	header := &zip.FileHeader{Name: "index.js", Method: zip.Deflate, Modified: time.Unix(0, 0).UTC()}
	file, err := writer.CreateHeader(header)
	if err != nil {
		return "", "", err
	}
	if _, err := file.Write([]byte(script)); err != nil {
		return "", "", err
	}
	if err := writer.Close(); err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(archive.Bytes())
	return base64.StdEncoding.EncodeToString(archive.Bytes()), fmt.Sprintf("%x", digest[:]), nil
}

func syntheticExecutionPolicy(args Args, name, bucketARN string) (string, error) {
	partition := "aws"
	serviceSuffix := "amazonaws.com"
	if strings.HasPrefix(args.Region, "us-gov-") {
		partition = "aws-us-gov"
	} else if strings.HasPrefix(args.Region, "cn-") {
		partition = "aws-cn"
		serviceSuffix = "amazonaws.com.cn"
	}
	policy := map[string]any{"Version": "2012-10-17", "Statement": []any{
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetBucketLocation"}, "Resource": bucketARN},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:PutObject"}, "Resource": bucketARN + "/synthetics/*"},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:GetObjectVersion"}, "Resource": bucketARN + "/synthetics-scripts/*"},
		map[string]any{"Effect": "Allow", "Action": []string{"s3:ListAllMyBuckets"}, "Resource": "*"},
		map[string]any{"Effect": "Allow", "Action": []string{"kms:Decrypt", "kms:GenerateDataKey"}, "Resource": args.KMSKeyARN, "Condition": map[string]any{"StringEquals": map[string]string{"kms:ViaService": "s3." + args.Region + "." + serviceSuffix}}},
		map[string]any{"Effect": "Allow", "Action": []string{"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"}, "Resource": "arn:" + partition + ":logs:" + args.Region + ":*:log-group:/aws/lambda/cwsyn-" + name + "*"},
		map[string]any{"Effect": "Allow", "Action": []string{"cloudwatch:PutMetricData"}, "Resource": "*", "Condition": map[string]any{"StringEquals": map[string]string{"cloudwatch:namespace": "CloudWatchSynthetics"}}},
	}}
	encoded, err := json.Marshal(policy)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func dashboardBody(args Args) (string, error) {
	metric := []any{"AWS/ECS", "CPUUtilization", "ClusterName", args.ECSClusterName, "ServiceName", args.ECSServiceName, map[string]any{"label": "ECS CPU", "stat": "Average", "period": 60}}
	widgets := []any{
		map[string]any{"type": "metric", "x": 0, "y": 0, "width": 12, "height": 6, "properties": map[string]any{"view": "timeSeries", "title": "ECS CPU", "region": args.Region, "metrics": []any{metric}, "period": 60, "stat": "Average"}},
		map[string]any{"type": "log", "x": 12, "y": 0, "width": 12, "height": 6, "properties": map[string]any{"query": fmt.Sprintf("SOURCE '%s/web' | fields @timestamp, @message | sort @timestamp desc | limit 50", args.LogGroupPrefix), "region": args.Region, "title": "Recent web logs", "view": "table"}},
	}
	encoded, err := json.Marshal(map[string]any{"widgets": widgets})
	return string(encoded), err
}

func dashboardBodyInput(args Args) pulumi.StringOutput {
	return pulumi.All(clusterNameInput(args), serviceNameInput(args)).ApplyT(func(values []interface{}) (string, error) {
		copy := args
		copy.ECSClusterName, _ = values[0].(string)
		copy.ECSServiceName, _ = values[1].(string)
		return dashboardBody(copy)
	}).(pulumi.StringOutput)
}

func ecsDimensions(args Args) pulumi.StringMapOutput {
	return pulumi.All(clusterNameInput(args), serviceNameInput(args)).ApplyT(func(values []interface{}) map[string]string {
		cluster, _ := values[0].(string)
		service, _ := values[1].(string)
		return map[string]string{"ClusterName": cluster, "ServiceName": service}
	}).(pulumi.StringMapOutput)
}

func loadBalancerDimensions(args Args) pulumi.StringMapOutput {
	input := args.LoadBalancerDimensionInput
	if input == nil {
		input = pulumi.String(args.LoadBalancerDimension)
	}
	return input.ToStringOutput().ApplyT(func(value string) map[string]string {
		return map[string]string{"LoadBalancer": value}
	}).(pulumi.StringMapOutput)
}

func clusterNameInput(args Args) pulumi.StringOutput {
	if args.ECSClusterNameInput != nil {
		return args.ECSClusterNameInput.ToStringOutput()
	}
	return pulumi.String(args.ECSClusterName).ToStringOutput()
}

func serviceNameInput(args Args) pulumi.StringOutput {
	if args.ECSServiceNameInput != nil {
		return args.ECSServiceNameInput.ToStringOutput()
	}
	return pulumi.String(args.ECSServiceName).ToStringOutput()
}

func hasECSNames(args Args) bool {
	cluster := args.ECSClusterNameInput != nil || strings.TrimSpace(args.ECSClusterName) != ""
	service := args.ECSServiceNameInput != nil || strings.TrimSpace(args.ECSServiceName) != ""
	return cluster && service
}

func hasLoadBalancerDimension(args Args) bool {
	return args.LoadBalancerDimensionInput != nil || strings.TrimSpace(args.LoadBalancerDimension) != ""
}

func alarmIDs(alarms []*cloudwatch.MetricAlarm) pulumi.ArrayOutput {
	values := make([]any, len(alarms))
	for index, alarm := range alarms {
		values[index] = alarm.Arn
	}
	return pulumi.All(values...).ApplyT(func(values []interface{}) []interface{} { return values }).(pulumi.ArrayOutput)
}

func tags(input map[string]string, component, role string) map[string]string {
	result := make(map[string]string, len(input)+3)
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = input[key]
	}
	result["Name"] = component + "-" + role
	result["magelift:component"] = component
	result["magelift:role"] = role
	return result
}
