package search

import (
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type node struct {
	typeToken string
	name      string
	inputs    resource.PropertyMap
}

type mocks struct {
	mu    sync.Mutex
	nodes []node
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	state := args.Inputs.Copy()
	switch args.TypeToken {
	case "aws:opensearch/serverlessCollection:ServerlessCollection":
		state["arn"] = resource.NewStringProperty("arn:aws:aoss:eu-west-3:123456789012:collection/abc")
		state["collectionEndpoint"] = resource.NewStringProperty("https://abc.eu-west-3.aoss.amazonaws.com")
		state["dashboardEndpoint"] = resource.NewStringProperty("https://abc.eu-west-3.aoss.amazonaws.com/_dashboards")
	case "aws:opensearch/domain:Domain":
		state["arn"] = resource.NewStringProperty("arn:aws:es:eu-west-3:123456789012:domain/shop")
		state["endpoint"] = resource.NewStringProperty("vpc-shop.eu-west-3.es.amazonaws.com")
		state["dashboardEndpoint"] = resource.NewStringProperty("vpc-shop.eu-west-3.es.amazonaws.com/_dashboards")
	}
	m.mu.Lock()
	m.nodes = append(m.nodes, node{typeToken: args.TypeToken, name: args.Name, inputs: args.Inputs.Copy()})
	m.mu.Unlock()
	return args.Name + "-id", state, nil
}

func (*mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) { return args.Args, nil }

func TestPreviewCreatesPrivateBoundedServerlessSearch(t *testing.T) {
	t.Parallel()
	m := deploy(t, previewArgs())
	want := []string{
		"aws:opensearch/serverlessAccessPolicy:ServerlessAccessPolicy:shop-access",
		"aws:opensearch/serverlessCollection:ServerlessCollection:shop",
		"aws:opensearch/serverlessCollectionGroup:ServerlessCollectionGroup:shop-group",
		"aws:opensearch/serverlessSecurityPolicy:ServerlessSecurityPolicy:shop-network",
		"aws:opensearch/serverlessVpcEndpoint:ServerlessVpcEndpoint:shop-endpoint",
		TypeToken + ":shop",
	}
	if got := m.snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("graph = %v", got)
	}

	group := m.one(t, "aws:opensearch/serverlessCollectionGroup:ServerlessCollectionGroup").inputs
	if group["generation"].StringValue() != "CLASSIC" || group["standbyReplicas"].StringValue() != "DISABLED" {
		t.Fatalf("collection group policy = %v", group)
	}
	limits := group["capacityLimits"].ArrayValue()[0].ObjectValue()
	assertNumber(t, limits, "minIndexingCapacityInOcu", 1)
	assertNumber(t, limits, "maxIndexingCapacityInOcu", 4)
	assertNumber(t, limits, "minSearchCapacityInOcu", 1)
	assertNumber(t, limits, "maxSearchCapacityInOcu", 4)

	collection := m.one(t, "aws:opensearch/serverlessCollection:ServerlessCollection").inputs
	if collection["type"].StringValue() != "SEARCH" || collection["standbyReplicas"].StringValue() != "DISABLED" {
		t.Fatalf("collection policy = %v", collection)
	}
	encryption := collection["encryptionConfigs"].ArrayValue()[0].ObjectValue()
	if encryption["kmsKeyArn"].StringValue() != testKMSKeyARN {
		t.Fatalf("encryption config = %v", encryption)
	}

	network := decodePolicy(t, m.one(t, "aws:opensearch/serverlessSecurityPolicy:ServerlessSecurityPolicy").inputs["policy"].StringValue())
	networkDocument := network[0].(map[string]any)
	if networkDocument["AllowFromPublic"].(bool) || networkDocument["SourceVPCEs"].([]any)[0].(string) != "shop-endpoint-id" {
		t.Fatalf("network policy = %v", networkDocument)
	}
	accessText := m.one(t, "aws:opensearch/serverlessAccessPolicy:ServerlessAccessPolicy").inputs["policy"].StringValue()
	if !strings.Contains(accessText, testIdentityARN) || strings.Contains(strings.ToLower(accessText), "password") || strings.Contains(accessText, "aoss:*") {
		t.Fatalf("access policy is not scoped to the configured ARN: %s", accessText)
	}
}

func TestProvisionedSearchEnforcesTransportEncryptionAndFineGrainedAccess(t *testing.T) {
	t.Parallel()
	tests := []struct {
		preset      sdk.PresetID
		subnets     pulumi.StringArray
		instances   int
		zones       float64
		withStandby bool
	}{
		{sdk.PresetStandard, stringsInput("data-a", "data-b"), 2, 2, false},
		{sdk.PresetHighAvailability, stringsInput("data-a", "data-b", "data-c"), 3, 3, true},
	}
	for _, test := range tests {
		test := test
		t.Run(string(test.preset), func(t *testing.T) {
			t.Parallel()
			args := provisionedArgs(test.preset, test.subnets, test.instances)
			m := deploy(t, args)
			want := []string{"aws:opensearch/domain:Domain:shop", TypeToken + ":shop"}
			if got := m.snapshot(); !reflect.DeepEqual(got, want) {
				t.Fatalf("graph = %v", got)
			}
			domain := m.one(t, "aws:opensearch/domain:Domain").inputs
			assertBoolObject(t, domain, "encryptAtRest", "enabled", true)
			assertBoolObject(t, domain, "nodeToNodeEncryption", "enabled", true)
			endpoint := domain["domainEndpointOptions"].ObjectValue()
			if !endpoint["enforceHttps"].BoolValue() || endpoint["tlsSecurityPolicy"].StringValue() != minimumTLSPolicy {
				t.Fatalf("endpoint policy = %v", endpoint)
			}
			security := domain["advancedSecurityOptions"].ObjectValue()
			if !security["enabled"].BoolValue() || security["internalUserDatabaseEnabled"].BoolValue() {
				t.Fatalf("fine-grained access = %v", security)
			}
			master := security["masterUserOptions"].ObjectValue()
			if master["masterUserArn"].StringValue() != testIdentityARN || master.HasValue("masterUserPassword") || master.HasValue("masterUserName") {
				t.Fatalf("master identity = %v", master)
			}
			accessPolicy := domain["accessPolicies"].StringValue()
			if !strings.Contains(accessPolicy, testIdentityARN) || !strings.Contains(accessPolicy, "arn:aws:es:eu-west-3:123456789012:domain/shop/*") || strings.Contains(strings.ToLower(accessPolicy), "password") {
				t.Fatalf("domain access policy = %s", accessPolicy)
			}
			cluster := domain["clusterConfig"].ObjectValue()
			if cluster["zoneAwarenessConfig"].ObjectValue()["availabilityZoneCount"].NumberValue() != test.zones || cluster["multiAzWithStandbyEnabled"].BoolValue() != test.withStandby {
				t.Fatalf("zone policy = %v", cluster)
			}
			if test.withStandby && (!cluster["dedicatedMasterEnabled"].BoolValue() || cluster["dedicatedMasterCount"].NumberValue() != 3 || cluster["dedicatedMasterType"].StringValue() != "m7g.master-selected.search") {
				t.Fatalf("dedicated master policy = %v", cluster)
			}
		})
	}
}

func TestSearchAcceptsOutputIdentityInput(t *testing.T) {
	t.Parallel()
	args := previewArgs()
	args.AccessIdentityARN = ""
	args.AccessIdentityInput = pulumi.String(testIdentityARN)
	m := deploy(t, args)
	accessText := m.one(t, "aws:opensearch/serverlessAccessPolicy:ServerlessAccessPolicy").inputs["policy"].StringValue()
	if !strings.Contains(accessText, testIdentityARN) {
		t.Fatalf("output identity was not propagated into access policy: %s", accessText)
	}
}

func TestSearchRejectsUnsafeOrGuessedInputsBeforeRegistration(t *testing.T) {
	t.Parallel()
	previewCases := []func(*Args){
		func(args *Args) { args.KMSKeyARN = "alias/aws/aoss" },
		func(args *Args) { args.AccessIdentityARN = "magelift-admin" },
		func(args *Args) { args.Serverless.AcceptColdStarts = false },
		func(args *Args) { args.Serverless.Capacity.MinimumSearchOCU = 0 },
		func(args *Args) { args.Serverless.Capacity.MinimumIndexingOCU = 3 },
		func(args *Args) { args.Serverless.Capacity.MaximumIndexingOCU = 0 },
		func(args *Args) { args.Serverless.Capacity.MaximumSearchOCU = math.Inf(1) },
		func(args *Args) { args.Serverless.Capacity.MaximumIndexingOCU = 3 },
		func(args *Args) { args.Provisioned = &Provisioned{} },
	}
	for index, mutate := range previewCases {
		args := previewArgs()
		mutate(&args)
		assertRejectedBeforeRegistration(t, index, args)
	}

	provisionedCases := []func(*Args){
		func(args *Args) { args.Provisioned.EngineVersion = "" },
		func(args *Args) { args.Provisioned.InstanceType = "" },
		func(args *Args) { args.Provisioned.EBSVolumeSizeGiB = 0 },
		func(args *Args) { args.Provisioned.InstanceCount = 1 },
		func(args *Args) { args.SubnetIDs = args.SubnetIDs[:1] },
		func(args *Args) { args.Serverless = &Serverless{} },
	}
	for index, mutate := range provisionedCases {
		args := provisionedArgs(sdk.PresetStandard, stringsInput("data-a", "data-b"), 2)
		mutate(&args)
		assertRejectedBeforeRegistration(t, index+len(previewCases), args)
	}

	ha := provisionedArgs(sdk.PresetHighAvailability, stringsInput("data-a", "data-b", "data-c"), 4)
	assertRejectedBeforeRegistration(t, len(previewCases)+len(provisionedCases), ha)
}

const (
	testKMSKeyARN   = "arn:aws:kms:eu-west-3:123456789012:key/11111111-2222-3333-4444-555555555555"
	testIdentityARN = "arn:aws:iam::123456789012:role/magelift-search"
)

func previewArgs() Args {
	return Args{
		Preset: sdk.PresetPreview, Region: "eu-west-3", VPCID: pulumi.String("vpc-123"),
		SubnetIDs: stringsInput("data-a", "data-b"), SecurityGroupIDs: stringsInput("sg-search"),
		KMSKeyARN: testKMSKeyARN, AccessIdentityARN: testIdentityARN,
		Serverless: &Serverless{AcceptColdStarts: true, Capacity: ServerlessCapacity{MinimumIndexingOCU: 1, MaximumIndexingOCU: 4, MinimumSearchOCU: 1, MaximumSearchOCU: 4}},
	}
}

func provisionedArgs(preset sdk.PresetID, subnetIDs pulumi.StringArray, instances int) Args {
	provisioned := &Provisioned{EngineVersion: "OpenSearch_3.3", InstanceType: "m7g.benchmark-selected.search", InstanceCount: instances, EBSVolumeType: "gp3", EBSVolumeSizeGiB: 200}
	if preset == sdk.PresetHighAvailability {
		provisioned.DedicatedMasterType = "m7g.master-selected.search"
		provisioned.DedicatedMasterCount = 3
	}
	return Args{
		Preset: preset, Region: "eu-west-3", VPCID: pulumi.String("vpc-123"), SubnetIDs: subnetIDs,
		SecurityGroupIDs: stringsInput("sg-search"), KMSKeyARN: testKMSKeyARN, AccessIdentityARN: testIdentityARN,
		Provisioned: provisioned,
	}
}

func stringsInput(values ...string) pulumi.StringArray {
	result := make(pulumi.StringArray, len(values))
	for index, value := range values {
		result[index] = pulumi.String(value)
	}
	return result
}

func deploy(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	if err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop", args)
		return err
	}, pulumi.WithMocks("project", "stack", m)); err != nil {
		t.Fatal(err)
	}
	return m
}

func assertRejectedBeforeRegistration(t *testing.T, index int, args Args) {
	t.Helper()
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop", args)
		return err
	}, pulumi.WithMocks("project", "stack", m))
	if err == nil {
		t.Fatalf("case %d was accepted", index)
	}
	if got := m.snapshot(); len(got) != 0 {
		t.Fatalf("case %d registered resources: %v", index, got)
	}
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

func decodePolicy(t *testing.T, value string) []any {
	t.Helper()
	var decoded []any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}

func assertNumber(t *testing.T, values resource.PropertyMap, key string, want float64) {
	t.Helper()
	if got := values[resource.PropertyKey(key)].NumberValue(); got != want {
		t.Fatalf("%s = %f, want %f", key, got, want)
	}
}

func assertBoolObject(t *testing.T, values resource.PropertyMap, objectKey, key string, want bool) {
	t.Helper()
	if got := values[resource.PropertyKey(objectKey)].ObjectValue()[resource.PropertyKey(key)].BoolValue(); got != want {
		t.Fatalf("%s.%s = %t, want %t", objectKey, key, got, want)
	}
}

// TestValidServerlessOCU locks MageLift's collection-group OCU step rule.
//
// This is MageLift's empirically observed rule (commit b8b957e, 2026-07-19),
// not AWS-documented behaviour. AWS publishes only min/max floors for
// collection-group capacity fields; the step rule is unpublished. Do not
// "fix" the validator against the docs; a relaxed rule fails at create
// time on a live account. Re-verify on the next paid AWS pass.
func TestValidServerlessOCU(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "accept 1", value: 1, want: true},
		{name: "accept 2", value: 2, want: true},
		{name: "accept 4", value: 4, want: true},
		{name: "accept 8", value: 8, want: true},
		{name: "accept 16", value: 16, want: true},
		{name: "accept 32", value: 32, want: true},
		{name: "accept 48", value: 48, want: true},
		{name: "accept 64", value: 64, want: true},
		{name: "reject 0", value: 0, want: false},
		{name: "reject 0.5", value: 0.5, want: false},
		{name: "reject 3", value: 3, want: false},
		{name: "reject 6", value: 6, want: false},
		{name: "reject 12", value: 12, want: false},
		{name: "reject 17", value: 17, want: false},
		{name: "reject 24", value: 24, want: false},
		{name: "reject 33", value: 33, want: false},
		{name: "reject negative", value: -1, want: false},
		{name: "reject NaN", value: math.NaN(), want: false},
		{name: "reject +Inf", value: math.Inf(1), want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validServerlessOCU(tc.value); got != tc.want {
				t.Fatalf("validServerlessOCU(%v) = %t, want %t", tc.value, got, tc.want)
			}
		})
	}
}

func TestValidCapacityRange(t *testing.T) {
	cases := []struct {
		name    string
		minimum float64
		maximum float64
		want    bool
	}{
		{name: "accept equal bounds", minimum: 4, maximum: 4, want: true},
		{name: "accept maximum above minimum", minimum: 1, maximum: 16, want: true},
		{name: "reject maximum below minimum", minimum: 8, maximum: 4, want: false},
		{name: "reject zero minimum", minimum: 0, maximum: 4, want: false},
		{name: "reject negative minimum", minimum: -1, maximum: 4, want: false},
		{name: "reject zero maximum", minimum: 1, maximum: 0, want: false},
		{name: "reject NaN minimum", minimum: math.NaN(), maximum: 4, want: false},
		{name: "reject NaN maximum", minimum: 1, maximum: math.NaN(), want: false},
		{name: "reject +Inf maximum", minimum: 1, maximum: math.Inf(1), want: false},
		{name: "reject -Inf minimum", minimum: math.Inf(-1), maximum: 4, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := validCapacityRange(tc.minimum, tc.maximum); got != tc.want {
				t.Fatalf("validCapacityRange(%v, %v) = %t, want %t", tc.minimum, tc.maximum, got, tc.want)
			}
		})
	}
}
