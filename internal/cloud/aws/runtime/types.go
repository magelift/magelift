// Package runtime provisions Magento ECS task definitions, services, and identity on AWS.
package runtime

import (
	"regexp"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:EcsRuntime"

const IdentityTypeToken = "magelift:aws:EcsRuntimeIdentity"

const (
	ApplicationPort = 8080
	VarnishPort     = 6081
	searchProxyPort = 8081
)

var imageDigest = regexp.MustCompile(`^[^\s@]+@sha256:[a-f0-9]{64}$`)

var secretName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

var secretARN = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):(secretsmanager|ssm):[a-z0-9-]+:[0-9]{12}:(?:secret:[A-Za-z0-9/_+=.@-]+|parameter/[A-Za-z0-9_.+=/@-]+)$`)

var secretJSONKey = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

type SecretReference struct {
	Name    string
	ARN     string
	JSONKey string
}

func (s SecretReference) ValueFrom() string {
	if s.JSONKey == "" {
		return s.ARN
	}
	return s.ARN + ":" + s.JSONKey + "::"
}

type CapabilityConfig struct {
	DatabaseWriterEndpoint pulumi.StringInput
	DatabaseName           string
	DatabaseSecretARN      pulumi.StringInput
	CacheEndpoint          pulumi.StringInput
	SessionEndpoint        pulumi.StringInput
	SearchEndpoint         pulumi.StringInput
	QueueMode              pulumi.StringInput
	QueueEndpoint          pulumi.StringInput
	QueueUsername          pulumi.StringInput
	MediaBucket            pulumi.StringInput
}

type IdentityArgs struct {
	Secrets []SecretReference
	Tags    map[string]string
}

type Identity struct {
	pulumi.ResourceState
	ExecutionRoleARN   pulumi.StringOutput
	ExecutionRoleName  pulumi.StringOutput
	TaskRoleARN        pulumi.StringOutput
	TaskRoleName       pulumi.StringOutput
	DeploymentRoleARN  pulumi.StringOutput
	DeploymentRoleName pulumi.StringOutput
	secretReferences   []SecretReference
}

type Args struct {
	Region             string
	ApplicationMode    string
	WebRuntime         string
	VpcID              pulumi.StringInput
	PrivateSubnetIDs   pulumi.StringArray
	Image              string
	SearchProxyImage   string
	DatabaseSecretARN  pulumi.StringInput
	EncryptionKeyARN   pulumi.StringInput
	ContainerPort      int
	VarnishImage       string
	TaskCPU            string
	TaskMemory         string
	DesiredCount       int
	QueueConsumerCount int
	WebSecurityGroupID pulumi.StringInput
	TargetGroupARN     pulumi.StringInput
	Secrets            []SecretReference
	Identity           *Identity
	Capabilities       *CapabilityConfig
	// LogGroupPrefix is /magelift/<project>/<env>; containers append /web|/deploy|/cron.
	LogGroupPrefix string
	Tags           map[string]string
}

type Component struct {
	pulumi.ResourceState
	ClusterName             pulumi.StringOutput
	ClusterARN              pulumi.StringOutput
	ServiceName             pulumi.StringOutput
	ExecutionRoleARN        pulumi.StringOutput
	TaskRoleARN             pulumi.StringOutput
	TaskRoleName            pulumi.StringOutput
	DeploymentRoleARN       pulumi.StringOutput
	DeploymentRoleName      pulumi.StringOutput
	TaskDefinitionARN       pulumi.StringOutput
	DeployTaskDefinitionARN pulumi.StringOutput
	CronTaskDefinitionARN   pulumi.StringOutput
	CronServiceName         pulumi.StringOutput
	QueueTaskDefinitionARN  pulumi.StringOutput
	QueueServiceName        pulumi.StringOutput
	ServiceID               pulumi.IDOutput
	SecurityGroupID         pulumi.StringOutput
}
