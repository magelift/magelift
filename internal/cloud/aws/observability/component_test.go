package observability

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:s3/bucket:Bucket":
		state["arn"] = resource.NewStringProperty("arn:aws:s3:::shop-synthetic-artifacts")
		state["bucket"] = resource.NewStringProperty("shop-synthetic-artifacts")
	case "aws:iam/role:Role":
		state["arn"] = resource.NewStringProperty("arn:aws:iam::123456789012:role/" + args.Name)
		state["name"] = resource.NewStringProperty(args.Name)
	case "aws:s3/bucketObject:BucketObject":
		state["versionId"] = resource.NewStringProperty("version-1")
	}
	if args.TypeToken == "aws:cloudwatch/metricAlarm:MetricAlarm" {
		state["arn"] = resource.NewStringProperty("arn:aws:cloudwatch:eu-west-3:123456789012:alarm:" + args.Name)
	}
	return args.Name + "-id", state, nil
}

func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestCreatesEncryptedLogsDashboardAndAlarms(t *testing.T) {
	m := deploy(t, validArgs())
	if m.count("aws:cloudwatch/logGroup:LogGroup") != 3 || m.count("aws:cloudwatch/metricAlarm:MetricAlarm") != 3 {
		t.Fatalf("unexpected observability resource counts: %#v", m.resources)
	}
	group := m.one(t, "aws:cloudwatch/logGroup:LogGroup").Inputs
	if group[resource.PropertyKey("kmsKeyId")].StringValue() != validArgs().KMSKeyARN || group[resource.PropertyKey("retentionInDays")].NumberValue() != 30 {
		t.Fatalf("log group retention or encryption is incorrect: %#v", group)
	}
	if !group[resource.PropertyKey("deletionProtectionEnabled")].BoolValue() || !group[resource.PropertyKey("skipDestroy")].BoolValue() {
		t.Fatal("production log groups are not protected")
	}
	body := m.one(t, "aws:cloudwatch/dashboard:Dashboard").Inputs[resource.PropertyKey("dashboardBody")].StringValue()
	if !strings.Contains(body, "CPUUtilization") || !strings.Contains(body, "/magelift/shop/web") {
		t.Fatalf("dashboard is missing ECS and log widgets: %s", body)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatal(err)
	}
}

func TestRejectsInvalidRetentionAndNotificationBeforeRegistration(t *testing.T) {
	for _, mutate := range []func(*Args){
		func(args *Args) { args.RetentionInDays = 2 },
		func(args *Args) { args.KMSKeyARN = "alias/aws/logs" },
		func(args *Args) { args.NotificationTopicARN = "topic" },
		func(args *Args) {
			args.SyntheticEnabled = true
			args.SyntheticURL = "http://shop.example.com/health"
			args.SyntheticArtifactRetentionDays = 30
		},
	} {
		args := validArgs()
		mutate(&args)
		m := &mocks{}
		err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("magelift", "test", m))
		if err == nil {
			t.Fatal("invalid observability settings were accepted")
		}
		if len(m.resources) != 0 {
			t.Fatal("resources registered before validation")
		}
	}
}

func TestAcceptsOutputMetricDimensions(t *testing.T) {
	args := validArgs()
	args.ECSClusterName = ""
	args.ECSServiceName = ""
	args.LoadBalancerDimension = ""
	args.ECSClusterNameInput = pulumi.String("shop-cluster")
	args.ECSServiceNameInput = pulumi.String("shop-web")
	args.LoadBalancerDimensionInput = pulumi.String("app/shop/123")
	m := deploy(t, args)
	if m.count("aws:cloudwatch/metricAlarm:MetricAlarm") != 3 {
		t.Fatalf("output metric dimensions changed alarm count: %#v", m.resources)
	}
}

func TestCreatesProductionSyntheticHealthCheck(t *testing.T) {
	args := validArgs()
	args.SyntheticEnabled = true
	args.SyntheticURL = "https://shop.example.com/health"
	args.SyntheticArtifactRetentionDays = 30
	m := deploy(t, args)
	if m.count("aws:s3/bucket:Bucket") != 1 || m.count("aws:s3/bucketPolicy:BucketPolicy") != 1 || m.count("aws:s3/bucketObject:BucketObject") != 1 || m.count("aws:synthetics/canary:Canary") != 1 || m.count("aws:iam/role:Role") != 1 || m.count("aws:iam/rolePolicy:RolePolicy") != 1 || m.count("aws:cloudwatch/metricAlarm:MetricAlarm") != 4 {
		t.Fatalf("synthetic health check resource graph is incomplete: %#v", m.resources)
	}
	if !m.one(t, "aws:s3/bucket:Bucket").Inputs[resource.PropertyKey("forceDestroy")].BoolValue() {
		t.Fatal("synthetic artifact bucket would block complete environment destruction")
	}
	if !strings.Contains(m.one(t, "aws:s3/bucketPolicy:BucketPolicy").Inputs[resource.PropertyKey("policy")].StringValue(), "aws:SecureTransport") {
		t.Fatal("synthetic artifact bucket does not enforce TLS")
	}
	canary := m.one(t, "aws:synthetics/canary:Canary").Inputs
	if canary[resource.PropertyKey("runtimeVersion")].StringValue() != "syn-nodejs-puppeteer-11.0" || canary[resource.PropertyKey("handler")].StringValue() != "index.handler" {
		t.Fatalf("canary runtime or handler is incorrect: %#v", canary)
	}
	if !strings.Contains(canary[resource.PropertyKey("artifactS3Location")].StringValue(), "s3://") || canary[resource.PropertyKey("schedule")].ObjectValue()[resource.PropertyKey("expression")].StringValue() != "rate(5 minutes)" {
		t.Fatalf("canary storage or schedule is incorrect: %#v", canary)
	}
	alarms := m.resourcesByName("shop-synthetic")
	if len(alarms) != 1 || alarms[0].Inputs[resource.PropertyKey("treatMissingData")].StringValue() != "breaching" {
		t.Fatalf("synthetic alarm missing-data policy = %#v", alarms)
	}
	policy := m.one(t, "aws:iam/rolePolicy:RolePolicy").Inputs[resource.PropertyKey("policy")].StringValue()
	for _, required := range []string{"s3:GetObject", "s3:GetObjectVersion", "s3:PutObject", "s3:ListAllMyBuckets", "kms:GenerateDataKey", "cloudwatch:PutMetricData", "CloudWatchSynthetics"} {
		if !strings.Contains(policy, required) {
			t.Fatalf("synthetic execution policy is missing %q: %s", required, policy)
		}
	}
	encoded := m.one(t, "aws:s3/bucketObject:BucketObject").Inputs[resource.PropertyKey("contentBase64")].StringValue()
	archiveBytes, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		t.Fatal(err)
	}
	file, err := reader.Open("index.js")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	script, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(script), "https://shop.example.com/health") || !strings.Contains(string(script), "@aws/synthetics-puppeteer") {
		t.Fatalf("synthetic script does not target the health endpoint: %s", script)
	}
}

func validArgs() Args {
	return Args{
		Region: "eu-west-3", EnvironmentClass: "production", LogGroupPrefix: "/magelift/shop", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000", RetentionInDays: 30,
		ECSClusterName: "shop-cluster", ECSServiceName: "shop-web", DesiredTaskCount: 2, LoadBalancerDimension: "app/shop/123", NotificationTopicARN: "arn:aws:sns:eu-west-3:123456789012:ops", Tags: map[string]string{"magelift:managed-by": "magelift"},
	}
}

func deploy(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("magelift", "test", m)); err != nil {
		t.Fatal(err)
	}
	return m
}

func (m *mocks) count(token string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, resource := range m.resources {
		if resource.TypeToken == token {
			count++
		}
	}
	return count
}

func (m *mocks) one(t *testing.T, token string) pulumi.MockResourceArgs {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, resource := range m.resources {
		if resource.TypeToken == token {
			return resource
		}
	}
	t.Fatalf("resource %s not found", token)
	return pulumi.MockResourceArgs{}
}

func (m *mocks) resourcesByName(suffix string) []pulumi.MockResourceArgs {
	m.mu.Lock()
	defer m.mu.Unlock()
	var matches []pulumi.MockResourceArgs
	for _, resource := range m.resources {
		if strings.HasSuffix(resource.Name, suffix) && resource.TypeToken == "aws:cloudwatch/metricAlarm:MetricAlarm" {
			matches = append(matches, resource)
		}
	}
	return matches
}
