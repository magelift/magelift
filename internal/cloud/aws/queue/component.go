package queue

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/cloudwatch"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/mq"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/servicediscovery"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:RabbitMqQueue"

// Queue modes exposed on target.aws.catalog.queueMode.
const (
	ModeDB          = "db"
	ModeAmazonMQ    = "amazon-mq"
	ModeECSRabbitMQ = "ecs-rabbitmq"
	ModeECSArtemis  = "ecs-artemis"
)

type Topology string

const (
	TopologyPreview          Topology = "preview"
	TopologyStandard         Topology = "standard"
	TopologyHighAvailability Topology = "high-availability"
)

const (
	ecsBrokerCPU       = "512"
	ecsBrokerMemoryMiB = "1024"
	amqpPort           = 5672
	rabbitImage        = "docker.io/library/rabbitmq:3.13.7-management-alpine"
	artemisImage       = "docker.io/apache/activemq-artemis:2.39.0-alpine"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)
var secretARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):secretsmanager:[a-z0-9-]+:[0-9]{12}:secret:[A-Za-z0-9/_+=.@-]+$`)
var kmsARNPattern = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
var usernamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

type Credentials struct {
	SecretARN string
	Username  string
}

type Args struct {
	Mode                  string
	Topology              Topology
	Region                string
	EngineVersion         string
	InstanceType          string
	AvailabilityZones     []string
	SubnetIDs             []string
	SubnetIDInputs        pulumi.StringArrayInput
	SubnetCount           int
	SecurityGroupIDs      []string
	SecurityGroupIDInputs pulumi.StringArrayInput
	SecurityGroupCount    int
	KMSKeyARN             string
	Credentials           Credentials
	Provider              *awsprovider.Provider
	Tags                  map[string]string
	// ECS broker path (ecs-rabbitmq / ecs-artemis).
	VpcID              pulumi.StringInput
	ExecutionRoleARN   pulumi.StringInput
	TaskRoleARN        pulumi.StringInput
	LogGroupPrefix     string
	PrivateSubnetCount int
}

type Component struct {
	pulumi.ResourceState
	QueueMode          pulumi.StringOutput `pulumi:"queueMode"`
	BrokerARN          pulumi.StringOutput `pulumi:"brokerArn"`
	AMQPEndpoint       pulumi.StringOutput `pulumi:"amqpEndpoint"`
	ManagementEndpoint pulumi.StringOutput `pulumi:"managementEndpoint"`
}

func New(ctx *pulumi.Context, name string, args Args, options ...pulumi.ResourceOption) (*Component, error) {
	mode := resolveMode(args)
	if err := validate(name, mode, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, options...); err != nil {
		return nil, err
	}
	component.QueueMode = pulumi.String(mode).ToStringOutput()
	component.BrokerARN = pulumi.String("").ToStringOutput()
	component.AMQPEndpoint = pulumi.String("").ToStringOutput()
	component.ManagementEndpoint = pulumi.String("").ToStringOutput()

	switch mode {
	case ModeDB:
		// Magento database-backed messaging; no broker resources.
	case ModeAmazonMQ:
		if err := provisionAmazonMQ(ctx, name, args, component, options...); err != nil {
			return nil, err
		}
	case ModeECSRabbitMQ, ModeECSArtemis:
		if err := provisionECSBroker(ctx, name, mode, args, component, options...); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported queue mode %q", mode)
	}

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"queueMode": component.QueueMode, "brokerArn": component.BrokerARN,
		"amqpEndpoint": component.AMQPEndpoint, "managementEndpoint": component.ManagementEndpoint,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func resolveMode(args Args) string {
	if mode := strings.TrimSpace(args.Mode); mode != "" {
		return mode
	}
	if args.Topology == TopologyPreview {
		return ModeDB
	}
	return ModeAmazonMQ
}

func provisionAmazonMQ(ctx *pulumi.Context, name string, args Args, component *Component, options ...pulumi.ResourceOption) error {
	password := lookupPassword(ctx, args.Region, args.Credentials.SecretARN, args.Provider)
	brokerOptions := append(append([]pulumi.ResourceOption{}, options...), pulumi.Parent(component), pulumi.Provider(args.Provider))
	broker, err := mq.NewBroker(ctx, name+"-broker", &mq.BrokerArgs{
		AuthenticationStrategy: pulumi.String("simple"), AutoMinorVersionUpgrade: pulumi.Bool(true),
		BrokerName: pulumi.String(name), DeploymentMode: pulumi.String("CLUSTER_MULTI_AZ"),
		EncryptionOptions: &mq.BrokerEncryptionOptionsArgs{KmsKeyId: pulumi.String(args.KMSKeyARN), UseAwsOwnedKey: pulumi.Bool(false)},
		EngineType:        pulumi.String("RabbitMQ"), EngineVersion: pulumi.String(args.EngineVersion), HostInstanceType: pulumi.String(args.InstanceType),
		Logs: &mq.BrokerLogsArgs{General: pulumi.Bool(true)}, PubliclyAccessible: pulumi.Bool(false),
		Region: pulumi.String(args.Region), SecurityGroups: securityGroupInputs(args), StorageType: pulumi.String("ebs"),
		SubnetIds: subnetInputs(args), Tags: pulumi.ToStringMap(tags(args.Tags, name)),
		Users: mq.BrokerUserArray{mq.BrokerUserArgs{Username: pulumi.String(args.Credentials.Username), Password: password}},
	}, brokerOptions...)
	if err != nil {
		return fmt.Errorf("create RabbitMQ broker: %w", err)
	}
	component.BrokerARN = broker.Arn
	instance := broker.Instances.Index(pulumi.Int(0))
	component.AMQPEndpoint = instance.Endpoints().Index(pulumi.Int(0))
	component.ManagementEndpoint = instance.ConsoleUrl().ApplyT(func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}).(pulumi.StringOutput)
	return nil
}

func provisionECSBroker(ctx *pulumi.Context, name, mode string, args Args, component *Component, options ...pulumi.ResourceOption) error {
	child := append(append([]pulumi.ResourceOption{}, options...), pulumi.Parent(component), pulumi.Provider(args.Provider))

	namespace, err := servicediscovery.NewPrivateDnsNamespace(ctx, name+"-ns", &servicediscovery.PrivateDnsNamespaceArgs{
		Name:        pulumi.String(sanitizeDNSLabel(name) + ".magelift.local"),
		Description: pulumi.String("MageLift AMQP broker discovery"),
		Vpc:         args.VpcID,
		Region:      pulumi.String(args.Region),
		Tags:        pulumi.ToStringMap(tags(args.Tags, name)),
	}, child...)
	if err != nil {
		return fmt.Errorf("create broker discovery namespace: %w", err)
	}
	// SRV records require containerName+containerPort on the ECS service registry.
	// A records reject containerPort (AWS InvalidParameterException).
	discovery, err := servicediscovery.NewService(ctx, name+"-discovery", &servicediscovery.ServiceArgs{
		Name:        pulumi.String("amqp"),
		NamespaceId: namespace.ID(),
		Region:      pulumi.String(args.Region),
		DnsConfig: &servicediscovery.ServiceDnsConfigArgs{
			NamespaceId:   namespace.ID(),
			RoutingPolicy: pulumi.String("MULTIVALUE"),
			DnsRecords: servicediscovery.ServiceDnsConfigDnsRecordArray{
				&servicediscovery.ServiceDnsConfigDnsRecordArgs{Ttl: pulumi.Int(10), Type: pulumi.String("SRV")},
			},
		},
		Tags: pulumi.ToStringMap(tags(args.Tags, name)),
	}, child...)
	if err != nil {
		return fmt.Errorf("create broker discovery service: %w", err)
	}

	cluster, err := ecs.NewCluster(ctx, name+"-broker-cluster", &ecs.ClusterArgs{
		Region: pulumi.String(args.Region),
		Tags:   pulumi.ToStringMap(tags(args.Tags, name)),
	}, child...)
	if err != nil {
		return fmt.Errorf("create broker ECS cluster: %w", err)
	}
	logGroup, err := cloudwatch.NewLogGroup(ctx, name+"-broker-logs", &cloudwatch.LogGroupArgs{
		Name:            pulumi.String(args.LogGroupPrefix + "/broker"),
		RetentionInDays: pulumi.Int(14),
		Region:          pulumi.String(args.Region),
		Tags:            pulumi.ToStringMap(tags(args.Tags, name)),
	}, child...)
	if err != nil {
		return fmt.Errorf("create broker log group: %w", err)
	}

	image := rabbitImage
	env := []map[string]any{
		{"name": "RABBITMQ_DEFAULT_USER", "value": args.Credentials.Username},
	}
	secrets := []map[string]any{
		{"name": "RABBITMQ_DEFAULT_PASS", "valueFrom": args.Credentials.SecretARN},
	}
	if mode == ModeECSArtemis {
		image = artemisImage
		env = []map[string]any{
			{"name": "AMQ_USER", "value": args.Credentials.Username},
			{"name": "AMQ_EXTRA_ARGS", "value": "--nio"},
		}
		secrets = []map[string]any{
			{"name": "AMQ_PASSWORD", "valueFrom": args.Credentials.SecretARN},
		}
	}

	container := map[string]any{
		"name":      "broker",
		"image":     image,
		"essential": true,
		"portMappings": []map[string]any{
			{"containerPort": amqpPort, "protocol": "tcp"},
		},
		"environment": env,
		"secrets":     secrets,
		"logConfiguration": map[string]any{
			"logDriver": "awslogs",
			"options": map[string]string{
				"awslogs-group":         args.LogGroupPrefix + "/broker",
				"awslogs-region":        args.Region,
				"awslogs-stream-prefix": "broker",
			},
		},
	}
	encoded, err := json.Marshal([]map[string]any{container})
	if err != nil {
		return fmt.Errorf("encode broker container definitions: %w", err)
	}

	task, err := ecs.NewTaskDefinition(ctx, name+"-broker-task", &ecs.TaskDefinitionArgs{
		Family:                  pulumi.String(name + "-broker"),
		RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		NetworkMode:             pulumi.String("awsvpc"),
		Cpu:                     pulumi.String(ecsBrokerCPU),
		Memory:                  pulumi.String(ecsBrokerMemoryMiB),
		ExecutionRoleArn:        args.ExecutionRoleARN,
		TaskRoleArn:             args.TaskRoleARN,
		ContainerDefinitions:    pulumi.String(string(encoded)),
		Region:                  pulumi.String(args.Region),
		Tags:                    pulumi.ToStringMap(tags(args.Tags, name)),
	}, append(child, pulumi.DependsOn([]pulumi.Resource{logGroup}))...)
	if err != nil {
		return fmt.Errorf("create broker task definition: %w", err)
	}

	_, err = ecs.NewService(ctx, name+"-broker-service", &ecs.ServiceArgs{
		Name:           pulumi.String(name + "-broker"),
		Cluster:        cluster.Arn,
		TaskDefinition: task.Arn,
		DesiredCount:   pulumi.Int(1),
		LaunchType:     pulumi.String("FARGATE"),
		NetworkConfiguration: &ecs.ServiceNetworkConfigurationArgs{
			Subnets:        subnetInputs(args),
			SecurityGroups: securityGroupInputs(args),
			AssignPublicIp: pulumi.Bool(false),
		},
		ServiceRegistries: &ecs.ServiceServiceRegistriesArgs{
			RegistryArn:   discovery.Arn,
			ContainerName: pulumi.String("broker"),
			ContainerPort: pulumi.Int(amqpPort),
		},
		Region: pulumi.String(args.Region),
		Tags:   pulumi.ToStringMap(tags(args.Tags, name)),
	}, child...)
	if err != nil {
		return fmt.Errorf("create broker ECS service: %w", err)
	}

	component.AMQPEndpoint = pulumi.All(discovery.Name, namespace.Name).ApplyT(func(values []any) string {
		svc, _ := values[0].(string)
		ns, _ := values[1].(string)
		return fmt.Sprintf("amqp://%s.%s:%d", svc, ns, amqpPort)
	}).(pulumi.StringOutput)
	return nil
}

func lookupPassword(ctx *pulumi.Context, region, arn string, provider *awsprovider.Provider) pulumi.StringInput {
	secret := secretsmanager.LookupSecretVersionOutput(ctx, secretsmanager.LookupSecretVersionOutputArgs{
		Region: pulumi.String(region), SecretId: pulumi.String(arn), VersionStage: pulumi.String("AWSCURRENT"),
	}, pulumi.Provider(provider))
	return pulumi.ToSecret(secret.SecretString()).(pulumi.StringOutput)
}

func validate(name, mode string, args Args) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" {
		return errors.New("queue name and AWS region are required")
	}
	switch mode {
	case ModeDB:
		return nil
	case ModeAmazonMQ:
		return validateAmazonMQ(args)
	case ModeECSRabbitMQ, ModeECSArtemis:
		return validateECSBroker(args)
	default:
		return fmt.Errorf("queueMode must be db, amazon-mq, ecs-rabbitmq, or ecs-artemis (got %q)", mode)
	}
}

func validateAmazonMQ(args Args) error {
	if args.Provider == nil || !versionPattern.MatchString(args.EngineVersion) || strings.TrimSpace(args.InstanceType) == "" {
		return errors.New("amazon-mq requires an AWS provider, engine version, and broker instance type")
	}
	if len(args.AvailabilityZones) != 3 || hasDuplicateOrBlank(args.AvailabilityZones) || !validSubnets(args) {
		return errors.New("amazon-mq cluster deployment requires exactly three unique availability zones and private subnets")
	}
	if !validSecurityGroups(args) || !kmsARNPattern.MatchString(args.KMSKeyARN) {
		return errors.New("amazon-mq requires private security groups and a customer-managed KMS key ARN")
	}
	if !secretARNPattern.MatchString(args.Credentials.SecretARN) || !usernamePattern.MatchString(args.Credentials.Username) {
		return errors.New("amazon-mq credentials require a Secrets Manager ARN and a valid username")
	}
	switch args.Topology {
	case TopologyStandard, TopologyHighAvailability, TopologyPreview:
		return nil
	default:
		return errors.New("amazon-mq topology is invalid")
	}
}

func validateECSBroker(args Args) error {
	if args.Provider == nil || args.VpcID == nil || args.ExecutionRoleARN == nil || args.TaskRoleARN == nil {
		return errors.New("ecs broker requires an AWS provider, VPC id, and ECS execution/task role ARNs")
	}
	if strings.TrimSpace(args.LogGroupPrefix) == "" {
		return errors.New("ecs broker requires a CloudWatch log group prefix")
	}
	if !validSubnets(args) || args.PrivateSubnetCount < 1 && args.SubnetIDInputs == nil && len(args.SubnetIDs) < 1 {
		return errors.New("ecs broker requires private subnets")
	}
	if args.SubnetIDInputs != nil && args.SubnetCount < 1 {
		return errors.New("ecs broker requires private subnets")
	}
	if !validSecurityGroups(args) {
		return errors.New("ecs broker requires a private security group")
	}
	if !secretARNPattern.MatchString(args.Credentials.SecretARN) || !usernamePattern.MatchString(args.Credentials.Username) {
		return errors.New("ecs broker credentials require a Secrets Manager ARN and a valid username")
	}
	return nil
}

func sanitizeDNSLabel(name string) string {
	label := strings.ToLower(name)
	var b strings.Builder
	for _, r := range label {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "magelift"
	}
	if len(out) > 40 {
		return out[:40]
	}
	return out
}

func subnetInputs(args Args) pulumi.StringArrayInput {
	if args.SubnetIDInputs != nil {
		return args.SubnetIDInputs
	}
	return pulumi.ToStringArray(args.SubnetIDs)
}

func validSubnets(args Args) bool {
	if args.SubnetIDInputs != nil {
		return args.SubnetCount > 0
	}
	return len(args.SubnetIDs) > 0 && !hasDuplicateOrBlank(args.SubnetIDs)
}

func securityGroupInputs(args Args) pulumi.StringArrayInput {
	if args.SecurityGroupIDInputs != nil {
		return args.SecurityGroupIDInputs
	}
	return pulumi.ToStringArray(args.SecurityGroupIDs)
}

func validSecurityGroups(args Args) bool {
	if args.SecurityGroupIDInputs != nil {
		return args.SecurityGroupCount > 0
	}
	return len(args.SecurityGroupIDs) > 0 && !hasDuplicateOrBlank(args.SecurityGroupIDs)
}

func hasDuplicateOrBlank(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func tags(input map[string]string, component string) map[string]string {
	result := make(map[string]string, len(input)+2)
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = input[key]
	}
	result["Name"] = component + "-broker"
	result["magelift:component"] = component
	result["magelift:role"] = "queue"
	return result
}

// NeedsBrokerSecret reports whether Magento AMQP password injection is required.
func NeedsBrokerSecret(mode string) bool {
	switch strings.TrimSpace(mode) {
	case ModeAmazonMQ, ModeECSRabbitMQ, ModeECSArtemis:
		return true
	default:
		return false
	}
}

// AMQPPortForMode returns the security-group port for Magento → broker traffic.
func AMQPPortForMode(mode string) int {
	if mode == ModeAmazonMQ {
		return 5671
	}
	return amqpPort
}
