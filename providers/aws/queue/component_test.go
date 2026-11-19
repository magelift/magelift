package queue

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
	invokes   []pulumi.MockCallArgs
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	m.resources = append(m.resources, args)
	m.mu.Unlock()
	state := args.Inputs.Copy()
	if args.TypeToken == "aws:mq/broker:Broker" {
		state["arn"] = resource.NewStringProperty("arn:aws:mq:eu-west-3:123456789012:broker:" + args.Name)
		state["instances"] = resource.NewArrayProperty([]resource.PropertyValue{resource.NewObjectProperty(resource.PropertyMap{
			"endpoints":  resource.NewArrayProperty([]resource.PropertyValue{resource.NewStringProperty("amqps://" + args.Name + ".mq.eu-west-3.amazonaws.com:5671")}),
			"consoleUrl": resource.NewStringProperty("https://" + args.Name + ".mq.eu-west-3.amazonaws.com:15671"),
		})})
	}
	return args.Name + "-id", state, nil
}

func (m *mocks) Call(args pulumi.MockCallArgs) (resource.PropertyMap, error) {
	m.mu.Lock()
	m.invokes = append(m.invokes, args)
	m.mu.Unlock()
	return resource.PropertyMap{"secretString": resource.MakeSecret(resource.NewStringProperty("rabbit-password"))}, nil
}

func TestPreviewUsesDatabaseBackedQueue(t *testing.T) {
	m := run(t, Args{Topology: TopologyPreview, Region: "eu-west-3"})
	if got := m.count("aws:mq/broker:Broker"); got != 0 {
		t.Fatalf("preview broker count = %d", got)
	}
	if len(m.invokes) != 0 {
		t.Fatalf("preview secret lookups = %d", len(m.invokes))
	}
}

func TestProductionUsesEncryptedThreeNodeCluster(t *testing.T) {
	m := run(t, productionArgs())
	brokers := m.resourcesOf("aws:mq/broker:Broker")
	if len(brokers) != 1 {
		t.Fatalf("broker count = %d", len(brokers))
	}
	encoded, err := json.Marshal(brokers[0].Inputs.Mappable())
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, required := range []string{`"engineType":"RabbitMQ"`, `"deploymentMode":"CLUSTER_MULTI_AZ"`, `"publiclyAccessible":false`, `"storageType":"ebs"`, `"general":true`, `"useAwsOwnedKey":false`, `"kmsKeyId":"arn:aws:kms:`} {
		if !strings.Contains(text, required) {
			t.Fatalf("broker lacks %s: %s", required, text)
		}
	}
	users := brokers[0].Inputs[resource.PropertyKey("users")].ArrayValue()
	if len(users) != 1 || !users[0].ObjectValue()[resource.PropertyKey("password")].IsSecret() {
		t.Fatalf("broker user password was not secret: %s", text)
	}
	if len(m.invokes) != 1 || m.invokes[0].Args[resource.PropertyKey("secretId")].StringValue() != productionArgs().Credentials.SecretARN {
		t.Fatalf("secret lookup did not use the configured ARN: %#v", m.invokes)
	}
}

func TestRejectsUnsafeProductionInputsBeforeRegistration(t *testing.T) {
	base := productionArgs()
	tests := []struct {
		name   string
		mutate func(*Args)
	}{
		{name: "two zones", mutate: func(args *Args) {
			args.AvailabilityZones = args.AvailabilityZones[:2]
			args.SubnetIDs = args.SubnetIDs[:2]
		}},
		{name: "plaintext password", mutate: func(args *Args) { args.Credentials.SecretARN = "password" }},
		{name: "aws managed key", mutate: func(args *Args) { args.KMSKeyARN = "alias/aws/mq" }},
		{name: "invalid username", mutate: func(args *Args) { args.Credentials.Username = "-root" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			args := base
			args.AvailabilityZones = append([]string(nil), base.AvailabilityZones...)
			args.SubnetIDs = append([]string(nil), base.SubnetIDs...)
			test.mutate(&args)
			m := &mocks{}
			err := pulumi.RunErr(func(ctx *pulumi.Context) error { _, err := New(ctx, "shop", args); return err }, pulumi.WithMocks("magelift", "test", m))
			if err == nil {
				t.Fatal("unsafe queue arguments were accepted")
			}
			if len(m.resources) != 0 {
				t.Fatal("resources registered before validation")
			}
		})
	}
}

func productionArgs() Args {
	return Args{
		Mode: ModeAmazonMQ, Topology: TopologyStandard, Region: "eu-west-3", EngineVersion: "3.13", InstanceType: "mq.m7g.large",
		AvailabilityZones: []string{"eu-west-3a", "eu-west-3b", "eu-west-3c"}, SubnetIDs: []string{"subnet-a", "subnet-b", "subnet-c"},
		SecurityGroupIDs: []string{"sg-queue"}, KMSKeyARN: "arn:aws:kms:eu-west-3:123456789012:key/00000000-0000-0000-0000-000000000000",
		Credentials: Credentials{SecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop/rabbit-auth-AbCd", Username: "magelift"},
	}
}

func TestECSRabbitMQCreatesBrokerService(t *testing.T) {
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop", Args{
			Mode: ModeECSRabbitMQ, Topology: TopologyStandard, Region: "eu-west-3",
			SubnetIDs: []string{"subnet-a"}, SecurityGroupIDs: []string{"sg-queue"},
			Credentials:      Credentials{SecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop/rabbit-auth-AbCd", Username: "magelift"},
			Provider:         provider,
			VpcID:            pulumi.String("vpc-123"),
			ExecutionRoleARN: pulumi.String("arn:aws:iam::123456789012:role/exec"),
			TaskRoleARN:      pulumi.String("arn:aws:iam::123456789012:role/task"),
			LogGroupPrefix:   "/magelift/shop/staging", LogRetentionDays: 30,
		})
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if got := m.count("aws:mq/broker:Broker"); got != 0 {
		t.Fatalf("ecs-rabbitmq must not create Amazon MQ broker, got %d", got)
	}
	if got := m.count("aws:ecs/service:Service"); got != 1 {
		t.Fatalf("broker service count = %d", got)
	}
	if got := m.count("aws:servicediscovery/privateDnsNamespace:PrivateDnsNamespace"); got != 1 {
		t.Fatalf("discovery namespace count = %d", got)
	}
	logGroups := m.resourcesOf("aws:cloudwatch/logGroup:LogGroup")
	if len(logGroups) != 1 || logGroups[0].Inputs["retentionInDays"].NumberValue() != 30 {
		t.Fatalf("ECS broker log retention = %v", logGroups)
	}
}

func TestRabbitImageHonorsRequestedVersion(t *testing.T) {
	if got := rabbitImageForVersion("4.3"); got != rabbitImage {
		t.Fatalf("recommended image = %q", got)
	}
	if got := rabbitImageForVersion("4.2"); got != "docker.io/library/rabbitmq:4.2-management-alpine" {
		t.Fatalf("custom image = %q", got)
	}
}

func run(t *testing.T, args Args) *mocks {
	t.Helper()
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		if args.Topology != TopologyPreview {
			provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String(args.Region)})
			if err != nil {
				return err
			}
			args.Provider = provider
		}
		_, err := New(ctx, "shop", args)
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestNormalizeBrokerPassword(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "openssl hex trailing newline", input: "abcdef0123456789\n", want: "abcdef0123456789"},
		{name: "crlf", input: "BrokerPassw0rd!\r\n", want: "BrokerPassw0rd!"},
		{name: "surrounding space", input: "  BrokerPassw0rd!  ", want: "BrokerPassw0rd!"},
		{name: "already clean", input: "BrokerPassw0rd!", want: "BrokerPassw0rd!"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeBrokerPassword(tc.input); got != tc.want {
				t.Fatalf("normalizeBrokerPassword() = %q, want %q", got, tc.want)
			}
		})
	}
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

func (m *mocks) resourcesOf(token string) []pulumi.MockResourceArgs {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := []pulumi.MockResourceArgs{}
	for _, resource := range m.resources {
		if resource.TypeToken == token {
			result = append(result, resource)
		}
	}
	return result
}

func TestECSRabbitMQUsesEncryptedPersistentStorageAndResolvableDNS(t *testing.T) {
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop", Args{Mode: ModeECSRabbitMQ, Topology: TopologyStandard, Region: "eu-west-3", SubnetIDInputs: pulumi.StringArray{pulumi.String("subnet-a"), pulumi.String("subnet-b")}, SubnetCount: 2, PrivateSubnetCount: 2, SecurityGroupIDInputs: pulumi.StringArray{pulumi.String("sg-broker")}, SecurityGroupCount: 1, Credentials: Credentials{SecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop/rabbit-auth-AbCd", Username: "magelift"}, Provider: provider, VpcID: pulumi.String("vpc-123"), ExecutionRoleARN: pulumi.String("arn:aws:iam::123456789012:role/exec"), TaskRoleARN: pulumi.String("arn:aws:iam::123456789012:role/task"), LogGroupPrefix: "/magelift/shop/staging"})
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	for token, count := range map[string]int{"aws:efs/fileSystem:FileSystem": 1, "aws:efs/mountTarget:MountTarget": 2, "aws:ec2/securityGroup:SecurityGroup": 1, "aws:ec2/securityGroupRule:SecurityGroupRule": 2} {
		if got := m.count(token); got != count {
			t.Errorf("%s count=%d want %d", token, got, count)
		}
	}
	for token, required := range map[string][]string{
		"aws:efs/fileSystem:FileSystem":         {`"encrypted":true`},
		"aws:ecs/taskDefinition:TaskDefinition": {`"transitEncryption":"ENABLED"`, `broker-data`, `/var/lib/rabbitmq`, `RABBITMQ_NODENAME`, `rabbit@localhost`},
		"aws:ecs/service:Service":               {`"deploymentMinimumHealthyPercent":0`, `"deploymentMaximumPercent":100`, `"desiredCount":1`},
		"aws:servicediscovery/service:Service":  {`"type":"A"`},
	} {
		list := m.resourcesOf(token)
		if len(list) != 1 {
			t.Errorf("%s has %d resources", token, len(list))
			continue
		}
		data, err := json.Marshal(list[0].Inputs.Mappable())
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range required {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s lacks %q: %s", token, want, data)
			}
		}
	}
	for _, rule := range m.resourcesOf("aws:ec2/securityGroupRule:SecurityGroupRule") {
		data, _ := json.Marshal(rule.Inputs.Mappable())
		for _, want := range []string{`"fromPort":2049`, `"toPort":2049`, `"sourceSecurityGroupId"`} {
			if !strings.Contains(string(data), want) {
				t.Errorf("NFS rule missing %s: %s", want, data)
			}
		}
		if strings.Contains(string(data), "0.0.0.0/0") {
			t.Errorf("NFS rule open to world: %s", data)
		}
	}
	services := m.resourcesOf("aws:ecs/service:Service")
	if len(services) == 1 {
		data, _ := json.Marshal(services[0].Inputs.Mappable())
		if strings.Contains(string(data), "containerPort") {
			t.Errorf("A record registry must not have container port: %s", data)
		}
	}
}

func TestECSArtemisDoesNotProvisionRabbitStorage(t *testing.T) {
	m := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		provider, err := awsprovider.NewProvider(ctx, "regional", &awsprovider.ProviderArgs{Region: pulumi.String("eu-west-3")})
		if err != nil {
			return err
		}
		_, err = New(ctx, "shop", Args{Mode: ModeECSArtemis, Topology: TopologyStandard, Region: "eu-west-3", SubnetIDs: []string{"subnet-a"}, SecurityGroupIDs: []string{"sg-broker"}, Credentials: Credentials{SecretARN: "arn:aws:secretsmanager:eu-west-3:123456789012:secret:shop/rabbit-auth-AbCd", Username: "magelift"}, Provider: provider, VpcID: pulumi.String("vpc-123"), ExecutionRoleARN: pulumi.String("arn:aws:iam::123456789012:role/exec"), TaskRoleARN: pulumi.String("arn:aws:iam::123456789012:role/task"), LogGroupPrefix: "/magelift/shop/staging"})
		return err
	}, pulumi.WithMocks("magelift", "test", m))
	if err != nil {
		t.Fatal(err)
	}
	if m.count("aws:efs/fileSystem:FileSystem") != 0 {
		t.Fatal("Artemis unexpectedly acquired RabbitMQ storage")
	}
	tasks := m.resourcesOf("aws:ecs/taskDefinition:TaskDefinition")
	data, _ := json.Marshal(tasks[0].Inputs.Mappable())
	if !strings.Contains(string(data), "apache/activemq-artemis") || strings.Contains(string(data), "RABBITMQ_NODENAME") {
		t.Fatalf("task: %s", data)
	}
}
