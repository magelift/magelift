package cache

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type cacheMocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
	calls     []pulumi.MockCallArgs
}

func (m *cacheMocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	outputs := args.Inputs.Copy()
	if args.TypeToken == "aws:elasticache/replicationGroup:ReplicationGroup" {
		outputs["primaryEndpointAddress"] = resource.NewStringProperty(args.Name + ".cache.amazonaws.com")
	}
	return args.Name + "-id", outputs, nil
}

func (m *cacheMocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	m.mu.Lock()
	m.calls = append(m.calls, args)
	m.mu.Unlock()
	return resource.PropertyMap{
		"secretString": resource.MakeSecret(resource.NewStringProperty("mock-auth-token-value")),
		"secretId":     args.Args["secretId"],
		"versionId":    resource.NewStringProperty("version-1"),
	}, nil
}

func TestPreviewCreatesOneBoundedEncryptedGroup(t *testing.T) {
	mocks := runComponent(t, TopologyPreview, 0)
	groups := resourcesOfType(mocks, "aws:elasticache/replicationGroup:ReplicationGroup")
	if len(groups) != 1 {
		t.Fatalf("replication groups = %d, want 1", len(groups))
	}
	assertSecureGroup(t, groups[0], false, 1)
	assertSecretReferencesOnly(t, mocks, 1)
}

func TestStandardCreatesSeparateCacheAndSessionGroups(t *testing.T) {
	mocks := runComponent(t, TopologyStandard, 1)
	groups := resourcesOfType(mocks, "aws:elasticache/replicationGroup:ReplicationGroup")
	if len(groups) != 2 {
		t.Fatalf("replication groups = %d, want 2", len(groups))
	}
	seen := map[string]bool{}
	for _, group := range groups {
		seen[group.Name] = true
		assertSecureGroup(t, group, true, 2)
	}
	if !seen["shop-cache"] || !seen["shop-sessions"] {
		t.Fatalf("cache and session groups are not separate: %#v", seen)
	}
	assertSecretReferencesOnly(t, mocks, 2)
}

func TestHighAvailabilityRequiresTwoReplicasPerGroup(t *testing.T) {
	mocks := runComponent(t, TopologyHighAvailability, 2)
	for _, group := range resourcesOfType(mocks, "aws:elasticache/replicationGroup:ReplicationGroup") {
		assertSecureGroup(t, group, true, 3)
	}
}

func TestRejectsCapacityDefaultsAndUnsafeSecrets(t *testing.T) {
	valid := baseArgs(TopologyStandard, 1)
	tests := []struct {
		name   string
		mutate func(*Args)
	}{
		{name: "missing engine version", mutate: func(args *Args) { args.EngineVersion = "" }},
		{name: "missing node type", mutate: func(args *Args) { args.NodeType = "" }},
		{name: "plaintext token", mutate: func(args *Args) { args.AuthTokens.CacheSecretARN = "plaintext-password" }},
		{name: "shared tokens", mutate: func(args *Args) { args.AuthTokens.SessionSecretARN = args.AuthTokens.CacheSecretARN }},
		{name: "missing replica", mutate: func(args *Args) { args.ReplicaCount = 0 }},
		{name: "single subnet", mutate: func(args *Args) { args.SubnetIDs = []string{"subnet-a"} }},
		{name: "missing KMS", mutate: func(args *Args) { args.KMSKeyARN = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := valid
			args.SubnetIDs = append([]string(nil), valid.SubnetIDs...)
			test.mutate(&args)
			if err := validateArgs("shop", args); err == nil {
				t.Fatal("unsafe Valkey arguments were accepted")
			}
		})
	}
}

func runComponent(t *testing.T, topology Topology, replicas int) *cacheMocks {
	t.Helper()
	mocks := &cacheMocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
		if err != nil {
			return err
		}
		args := baseArgs(topology, replicas)
		args.Provider = provider
		_, err = New(ctx, "shop", args)
		return err
	}, pulumi.WithMocks("magelift", "test", mocks))
	if err != nil {
		t.Fatal(err)
	}
	return mocks
}

func baseArgs(topology Topology, replicas int) Args {
	args := Args{
		Topology: topology, Region: "eu-west-3", EngineVersion: "8.1", NodeType: "cache.r7g.large", ReplicaCount: replicas,
		SubnetIDs: []string{"subnet-a", "subnet-b"}, SecurityGroup: "sg-cache", KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
		AuthTokens: AuthTokens{CacheSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop/cache-auth-AbCd", SessionSecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop/session-auth-EfGh"},
		Tags:       map[string]string{"magelift:managed-by": "magelift"},
	}
	if topology == TopologyPreview {
		args.AuthTokens.SessionSecretARN = ""
	}
	return args
}

func resourcesOfType(mocks *cacheMocks, token string) []pulumi.MockResourceArgs {
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	var result []pulumi.MockResourceArgs
	for _, registered := range mocks.resources {
		if registered.TypeToken == token {
			result = append(result, registered)
		}
	}
	return result
}

func assertSecureGroup(t *testing.T, group pulumi.MockResourceArgs, highAvailability bool, clusters float64) {
	t.Helper()
	encoded, err := json.Marshal(group.Inputs.Mappable())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{`"engine":"valkey"`, `"atRestEncryptionEnabled":true`, `"transitEncryptionEnabled":true`, `"transitEncryptionMode":"required"`, `"kmsKeyId":"arn:aws:kms:`, `"securityGroupIds":["sg-cache"]`} {
		if !strings.Contains(text, required) {
			t.Fatalf("replication group lacks %s: %s", required, text)
		}
	}
	if group.Inputs["automaticFailoverEnabled"].BoolValue() != highAvailability || group.Inputs["multiAzEnabled"].BoolValue() != highAvailability {
		t.Fatalf("high availability flags are incorrect: %s", text)
	}
	if group.Inputs["numCacheClusters"].NumberValue() != clusters {
		t.Fatalf("numCacheClusters = %v, want %v", group.Inputs["numCacheClusters"], clusters)
	}
	if !group.Inputs["authToken"].IsSecret() {
		t.Fatal("resolved auth token is not marked secret")
	}
}

func assertSecretReferencesOnly(t *testing.T, mocks *cacheMocks, want int) {
	t.Helper()
	mocks.mu.Lock()
	defer mocks.mu.Unlock()
	if len(mocks.calls) != want {
		t.Fatalf("secret lookups = %d, want %d", len(mocks.calls), want)
	}
	for _, call := range mocks.calls {
		if call.Token != "aws:secretsmanager/getSecretVersion:getSecretVersion" {
			t.Fatalf("unexpected invoke: %s", call.Token)
		}
		secretID := call.Args["secretId"].StringValue()
		if !secretARNPattern.MatchString(secretID) || strings.Contains(secretID, "mock-auth-token-value") {
			t.Fatalf("unsafe secret lookup input: %q", secretID)
		}
	}
}
