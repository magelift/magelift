package database

import (
	"reflect"
	"sort"
	"sync"
	"testing"

	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type node struct {
	typeToken, name string
	inputs          resource.PropertyMap
}
type mocks struct {
	mu    sync.Mutex
	nodes []node
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	state := args.Inputs.Copy()
	if args.TypeToken == "aws:rds/cluster:Cluster" {
		state["arn"] = resource.NewStringProperty("arn:aws:rds:eu-west-3:123456789012:cluster:" + args.Name)
		state["endpoint"] = resource.NewStringProperty(args.Name + ".writer")
		state["readerEndpoint"] = resource.NewStringProperty(args.Name + ".reader")
		state["masterUserSecrets"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{"secretArn": resource.NewStringProperty("arn:aws:secretsmanager:eu-west-3:123456789012:secret:managed")})})
	}
	m.mu.Lock()
	m.nodes = append(m.nodes, node{args.TypeToken, args.Name, args.Inputs.Copy()})
	m.mu.Unlock()
	return args.Name + "-id", state, nil
}
func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestPreviewGraphAndServerlessPolicy(t *testing.T) {
	t.Parallel()
	m := deploy(t, previewArgs())
	want := []string{"aws:rds/cluster:Cluster:shop-cluster", "aws:rds/clusterInstance:ClusterInstance:shop-instance-01", "aws:rds/subnetGroup:SubnetGroup:shop-subnets", TypeToken + ":shop"}
	if got := m.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("graph = %v", got)
	}
	cluster := m.one(t, "aws:rds/cluster:Cluster").inputs
	assertBool(t, cluster, "storageEncrypted", true)
	assertBool(t, cluster, "manageMasterUserPassword", true)
	assertBool(t, cluster, "deletionProtection", false)
	assertBool(t, cluster, "skipFinalSnapshot", true)
	if _, exists := cluster["masterPassword"]; exists {
		t.Fatal("cluster contains a plaintext password input")
	}
	scaling := cluster["serverlessv2ScalingConfiguration"].ObjectValue()
	if scaling["minCapacity"].NumberValue() != 0 || scaling["maxCapacity"].NumberValue() != 4 || scaling["secondsUntilAutoPause"].NumberValue() != 600 {
		t.Fatalf("serverless policy = %v", scaling)
	}
	instance := m.one(t, "aws:rds/clusterInstance:ClusterInstance").inputs
	if instance["instanceClass"].StringValue() != "db.serverless" || instance["publiclyAccessible"].BoolValue() {
		t.Fatalf("preview instance = %v", instance)
	}
	assertBool(t, instance, "performanceInsightsEnabled", false)
}

func TestProductionPresetGraphsAndProtection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset sdk.PresetID
		zones  []string
		count  int
	}{
		{sdk.PresetStandard, []string{"eu-west-3a", "eu-west-3b"}, 2},
		{sdk.PresetHighAvailability, []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}, 3},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.preset), func(t *testing.T) {
			t.Parallel()
			args := provisionedArgs(test.preset, test.zones, test.count)
			m := deploy(t, args)
			if got := m.count("aws:rds/clusterInstance:ClusterInstance"); got != test.count {
				t.Fatalf("instances = %d", got)
			}
			cluster := m.one(t, "aws:rds/cluster:Cluster").inputs
			assertBool(t, cluster, "deletionProtection", true)
			assertBool(t, cluster, "skipFinalSnapshot", false)
			assertBool(t, cluster, "deleteAutomatedBackups", false)
			if cluster["backupRetentionPeriod"].NumberValue() != 14 || cluster["finalSnapshotIdentifier"].StringValue() != "shop-final" {
				t.Fatalf("backup policy = %v", cluster)
			}
			zones := m.values("aws:rds/clusterInstance:ClusterInstance", "availabilityZone")
			wantZones := append([]string(nil), test.zones...)
			sort.Strings(wantZones)
			if !reflect.DeepEqual(zones, wantZones) {
				t.Fatalf("instance zones = %v", zones)
			}
			instance := m.one(t, "aws:rds/clusterInstance:ClusterInstance").inputs
			assertBool(t, instance, "performanceInsightsEnabled", true)
			if instance["performanceInsightsKmsKeyId"].StringValue() != "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555" || instance["performanceInsightsRetentionPeriod"].NumberValue() != 7 {
				t.Fatalf("performance insights policy = %v", instance)
			}
		})
	}
}

func TestDatabaseRejectsUnsafePolicyBeforeRegistration(t *testing.T) {
	t.Parallel()
	tests := []func(*Args){
		func(args *Args) { args.KMSKeyARN = "alias/aws/rds" },
		func(args *Args) { args.BackupRetentionDays = 0 },
		func(args *Args) { args.DataSubnetIDs = args.DataSubnetIDs[:1] },
		func(args *Args) { args.AvailabilityZones[1] = args.AvailabilityZones[0] },
		func(args *Args) { args.EnvironmentClass = "production" },
		func(args *Args) { args.ServerlessV2.EngineSupportsAutoPause = false },
		func(args *Args) { args.ServerlessV2.MinimumACU = .3 },
	}
	for index, mutate := range tests {
		args := previewArgs()
		mutate(&args)
		m := &mocks{}
		err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m))
		if err == nil {
			t.Fatalf("case %d was accepted", index)
		}
		if len(m.snapshot()) != 0 {
			t.Fatalf("case %d registered resources", index)
		}
	}
}

func previewArgs() Args {
	return Args{Preset: sdk.PresetPreview, Region: "eu-west-3", AvailabilityZones: []string{"eu-west-3a", "eu-west-3b"}, DataSubnetIDs: pulumi.StringArray{pulumi.String("data-a"), pulumi.String("data-b")}, VpcSecurityGroupIDs: pulumi.StringArray{pulumi.String("sg-db")}, EngineVersion: "8.0.mysql_aurora.3.10.0", DatabaseName: "magento", MasterUsername: "magelift", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555", BackupRetentionDays: 1, ServerlessV2: &ServerlessV2{MinimumACU: 0, MaximumACU: 4, AutoPauseSeconds: 600, EngineSupportsAutoPause: true}}
}
func provisionedArgs(preset sdk.PresetID, zones []string, count int) Args {
	ids := make(pulumi.StringArray, len(zones))
	for index := range ids {
		ids[index] = pulumi.String("data-" + zones[index])
	}
	return Args{Preset: preset, EnvironmentClass: "production", Region: "eu-west-3", AvailabilityZones: zones, DataSubnetIDs: ids, VpcSecurityGroupIDs: pulumi.StringArray{pulumi.String("sg-db")}, EngineVersion: "8.0.mysql_aurora.3.10.0", DatabaseName: "magento", MasterUsername: "magelift", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555", BackupRetentionDays: 14, FinalSnapshotIdentifier: "shop-final", ProvisionedInstanceClass: "db.benchmark-selected", InstanceCount: count}
}
func deploy(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("project", "stack", m)); err != nil {
		t.Fatal(err)
	}
	return m
}
func (m *mocks) snapshot() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.nodes))
	for index, node := range m.nodes {
		result[index] = node.typeToken + ":" + node.name
	}
	sort.Strings(result)
	return result
}
func (m *mocks) count(token string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := 0
	for _, node := range m.nodes {
		if node.typeToken == token {
			result++
		}
	}
	return result
}
func (m *mocks) one(t *testing.T, token string) node {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, node := range m.nodes {
		if node.typeToken == token {
			return node
		}
	}
	t.Fatalf("resource %s not found", token)
	return node{}
}
func (m *mocks) values(token, key string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []string{}
	for _, node := range m.nodes {
		if node.typeToken == token {
			result = append(result, node.inputs[resource.PropertyKey(key)].StringValue())
		}
	}
	sort.Strings(result)
	return result
}
func assertBool(t *testing.T, values resource.PropertyMap, key string, want bool) {
	t.Helper()
	if got := values[resource.PropertyKey(key)].BoolValue(); got != want {
		t.Fatalf("%s = %t, want %t", key, got, want)
	}
}
