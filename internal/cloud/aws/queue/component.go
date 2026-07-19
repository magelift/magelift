package queue

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/mq"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:RabbitMqQueue"

type Topology string

const (
	TopologyPreview          Topology = "preview"
	TopologyStandard         Topology = "standard"
	TopologyHighAvailability Topology = "high-availability"
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
}

type Component struct {
	pulumi.ResourceState
	QueueMode          pulumi.StringOutput `pulumi:"queueMode"`
	BrokerARN          pulumi.StringOutput `pulumi:"brokerArn"`
	AMQPEndpoint       pulumi.StringOutput `pulumi:"amqpEndpoint"`
	ManagementEndpoint pulumi.StringOutput `pulumi:"managementEndpoint"`
}

func New(ctx *pulumi.Context, name string, args Args, options ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, options...); err != nil {
		return nil, err
	}
	mode := "rabbitmq"
	if args.Topology == TopologyPreview {
		mode = "database"
	}
	component.QueueMode = pulumi.String(mode).ToStringOutput()
	component.BrokerARN = pulumi.String("").ToStringOutput()
	component.AMQPEndpoint = pulumi.String("").ToStringOutput()
	component.ManagementEndpoint = pulumi.String("").ToStringOutput()
	if args.Topology != TopologyPreview {
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
			return nil, fmt.Errorf("create RabbitMQ broker: %w", err)
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
	}
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"queueMode": component.QueueMode, "brokerArn": component.BrokerARN,
		"amqpEndpoint": component.AMQPEndpoint, "managementEndpoint": component.ManagementEndpoint,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func lookupPassword(ctx *pulumi.Context, region, arn string, provider *awsprovider.Provider) pulumi.StringInput {
	secret := secretsmanager.LookupSecretVersionOutput(ctx, secretsmanager.LookupSecretVersionOutputArgs{
		Region: pulumi.String(region), SecretId: pulumi.String(arn), VersionStage: pulumi.String("AWSCURRENT"),
	}, pulumi.Provider(provider))
	return pulumi.ToSecret(secret.SecretString()).(pulumi.StringOutput)
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" {
		return errors.New("RabbitMQ name and AWS region are required")
	}
	if args.Topology == TopologyPreview {
		return nil
	}
	if args.Provider == nil || !versionPattern.MatchString(args.EngineVersion) || strings.TrimSpace(args.InstanceType) == "" {
		return errors.New("production RabbitMQ requires an AWS provider, engine version, and broker instance type")
	}
	if len(args.AvailabilityZones) != 3 || hasDuplicateOrBlank(args.AvailabilityZones) || !validSubnets(args) {
		return errors.New("RabbitMQ cluster deployment requires exactly three unique availability zones and private subnets")
	}
	if !validSecurityGroups(args) || !kmsARNPattern.MatchString(args.KMSKeyARN) {
		return errors.New("RabbitMQ requires private security groups and a customer-managed KMS key ARN")
	}
	if !secretARNPattern.MatchString(args.Credentials.SecretARN) || !usernamePattern.MatchString(args.Credentials.Username) {
		return errors.New("RabbitMQ credentials require a Secrets Manager ARN and a valid username")
	}
	switch args.Topology {
	case TopologyStandard, TopologyHighAvailability:
		return nil
	default:
		return errors.New("RabbitMQ topology is invalid")
	}
}

func subnetInputs(args Args) pulumi.StringArrayInput {
	if args.SubnetIDInputs != nil {
		return args.SubnetIDInputs
	}
	return pulumi.ToStringArray(args.SubnetIDs)
}

func validSubnets(args Args) bool {
	if args.SubnetIDInputs != nil {
		return args.SubnetCount == 3
	}
	return len(args.SubnetIDs) == 3 && !hasDuplicateOrBlank(args.SubnetIDs)
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
