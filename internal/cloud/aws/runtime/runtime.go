package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
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
	Tags               map[string]string
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

func NewIdentity(ctx *pulumi.Context, name string, args IdentityArgs, opts ...pulumi.ResourceOption) (*Identity, error) {
	secrets, err := validateSecretReferences(args.Secrets)
	if err != nil {
		return nil, err
	}
	for _, secret := range secrets {
		if secret.Name == "MAGELIFT_DATABASE_CREDENTIALS" {
			return nil, errors.New("MAGELIFT_DATABASE_CREDENTIALS is reserved for the managed database secret")
		}
	}
	if !hasSecretReference(secrets, "MAGENTO_DC_CRYPT__KEY") {
		return nil, errors.New("runtime secrets must include MAGENTO_DC_CRYPT__KEY")
	}
	for _, secret := range secrets {
		if secret.Name == "MAGENTO_DC_CRYPT__KEY" && secret.JSONKey != "" {
			return nil, errors.New("MAGENTO_DC_CRYPT__KEY must reference the full secret value")
		}
	}
	identity := &Identity{}
	if err := ctx.RegisterComponentResourceV2(IdentityTypeToken, name, pulumi.Map{"secretCount": pulumi.Int(len(secrets))}, identity, opts...); err != nil {
		return nil, err
	}
	if err := provisionIdentity(ctx, name, IdentityArgs{Secrets: secrets, Tags: args.Tags}, identity, identity); err != nil {
		return nil, err
	}
	if err := ctx.RegisterResourceOutputs(identity, pulumi.Map{
		"executionRoleArn": identity.ExecutionRoleARN, "executionRoleName": identity.ExecutionRoleName,
		"taskRoleArn": identity.TaskRoleARN, "taskRoleName": identity.TaskRoleName,
		"deploymentRoleArn": identity.DeploymentRoleARN, "deploymentRoleName": identity.DeploymentRoleName,
	}); err != nil {
		return nil, err
	}
	return identity, nil
}

func createIdentity(ctx *pulumi.Context, name string, args IdentityArgs, parent pulumi.Resource) (*Identity, error) {
	identity := &Identity{}
	if err := provisionIdentity(ctx, name, args, identity, parent); err != nil {
		return nil, err
	}
	return identity, nil
}

func provisionIdentity(ctx *pulumi.Context, name string, args IdentityArgs, identity *Identity, parent pulumi.Resource) error {
	identity.secretReferences = append([]SecretReference(nil), args.Secrets...)
	assumeRole := `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ecs-tasks.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	executionRole, err := role(ctx, name+"-execution-role", "ECS task execution", assumeRole, args.Tags, name, parent)
	if err != nil {
		return err
	}
	taskRole, err := role(ctx, name+"-task-role", "Magento application task", assumeRole, args.Tags, name, parent)
	if err != nil {
		return err
	}
	deploymentRole, err := role(ctx, name+"-deployment-role", "Magento deployment task", assumeRole, args.Tags, name, parent)
	if err != nil {
		return err
	}
	identity.ExecutionRoleARN, identity.ExecutionRoleName = executionRole.Arn, executionRole.Name
	identity.TaskRoleARN, identity.TaskRoleName = taskRole.Arn, taskRole.Name
	identity.DeploymentRoleARN, identity.DeploymentRoleName = deploymentRole.Arn, deploymentRole.Name
	policy, err := executionPolicy(args.Secrets)
	if err != nil {
		return err
	}
	_, err = iam.NewRolePolicy(ctx, name+"-execution-policy", &iam.RolePolicyArgs{Role: executionRole.Name, Policy: pulumi.String(policy)}, pulumi.Parent(parent))
	return err
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	secrets, err := validate(name, args)
	if err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"region": pulumi.String(args.Region), "applicationMode": pulumi.String(args.ApplicationMode), "webRuntime": pulumi.String(args.WebRuntime), "image": pulumi.String(args.Image), "containerPort": pulumi.Int(args.ContainerPort),
		"taskCpu": pulumi.String(args.TaskCPU), "taskMemory": pulumi.String(args.TaskMemory), "desiredCount": pulumi.Int(args.DesiredCount),
		"privateSubnetIds": args.PrivateSubnetIDs,
	}, component, opts...); err != nil {
		return nil, err
	}
	child := pulumi.Parent(component)

	cluster, err := ecs.NewCluster(ctx, name+"-cluster", &ecs.ClusterArgs{Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "cluster")}, child)
	if err != nil {
		return nil, err
	}
	component.ClusterARN = cluster.Arn
	component.ClusterName = cluster.Name

	identity := args.Identity
	if identity == nil {
		identity, err = createIdentity(ctx, name, IdentityArgs{Secrets: secrets, Tags: args.Tags}, component)
		if err != nil {
			return nil, err
		}
	}
	component.ExecutionRoleARN, component.TaskRoleARN, component.TaskRoleName = identity.ExecutionRoleARN, identity.TaskRoleARN, identity.TaskRoleName
	component.DeploymentRoleARN, component.DeploymentRoleName = identity.DeploymentRoleARN, identity.DeploymentRoleName

	var securityGroupID pulumi.StringOutput
	if args.WebSecurityGroupID != nil {
		securityGroupID = args.WebSecurityGroupID.ToStringOutput()
	} else {
		securityGroup, err := ec2.NewSecurityGroup(ctx, name+"-web-sg", &ec2.SecurityGroupArgs{
			VpcId: args.VpcID, Description: pulumi.String("MageLift web tasks without inbound rules"), Ingress: ec2.SecurityGroupIngressArray{},
			Egress: ec2.SecurityGroupEgressArray{ec2.SecurityGroupEgressArgs{Protocol: pulumi.String("-1"), FromPort: pulumi.Int(0), ToPort: pulumi.Int(0), CidrBlocks: pulumi.StringArray{pulumi.String("0.0.0.0/0")}}},
			Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "web-security-group"),
		}, child)
		if err != nil {
			return nil, err
		}
		securityGroupID = securityGroup.ID().ToStringOutput()
	}
	component.SecurityGroupID = securityGroupID

	definitions, err := containerDefinitionsInput(args, secrets)
	if err != nil {
		return nil, err
	}
	taskDefinition, err := ecs.NewTaskDefinition(ctx, name+"-web-task", &ecs.TaskDefinitionArgs{
		Family: pulumi.String(name + "-web"), ContainerDefinitions: definitions, Cpu: pulumi.String(args.TaskCPU), Memory: pulumi.String(args.TaskMemory),
		ExecutionRoleArn: identity.ExecutionRoleARN, TaskRoleArn: identity.TaskRoleARN, NetworkMode: pulumi.String("awsvpc"), RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		RuntimePlatform: &ecs.TaskDefinitionRuntimePlatformArgs{CpuArchitecture: pulumi.String("X86_64"), OperatingSystemFamily: pulumi.String("LINUX")},
		Volumes:         ecs.TaskDefinitionVolumeArray{ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("tmp")}, ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("var")}},
		Region:          pulumi.String(args.Region), Tags: tags(args.Tags, name, "web-task"),
	}, child)
	if err != nil {
		return nil, err
	}
	component.TaskDefinitionARN = taskDefinition.Arn

	deployDefinitions, err := containerDefinitionsForInput(args, secrets, "deploy", deploymentCommand(), false)
	if err != nil {
		return nil, err
	}
	deployTaskDefinition, err := ecs.NewTaskDefinition(ctx, name+"-deploy-task", &ecs.TaskDefinitionArgs{
		Family: pulumi.String(name + "-deploy"), ContainerDefinitions: deployDefinitions, Cpu: pulumi.String(args.TaskCPU), Memory: pulumi.String(args.TaskMemory),
		ExecutionRoleArn: identity.ExecutionRoleARN, TaskRoleArn: identity.DeploymentRoleARN, NetworkMode: pulumi.String("awsvpc"), RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		RuntimePlatform: &ecs.TaskDefinitionRuntimePlatformArgs{CpuArchitecture: pulumi.String("X86_64"), OperatingSystemFamily: pulumi.String("LINUX")},
		Volumes:         ecs.TaskDefinitionVolumeArray{ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("tmp")}, ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("var")}},
		Region:          pulumi.String(args.Region), Tags: tags(args.Tags, name, "deploy-task"),
	}, child)
	if err != nil {
		return nil, err
	}
	component.DeployTaskDefinitionARN = deployTaskDefinition.Arn

	serviceArgs := &ecs.ServiceArgs{
		Cluster: cluster.Arn, TaskDefinition: taskDefinition.Arn, DesiredCount: pulumi.Int(args.DesiredCount), LaunchType: pulumi.String("FARGATE"),
		AvailabilityZoneRebalancing: pulumi.String("ENABLED"), EnableEcsManagedTags: pulumi.Bool(true), EnableExecuteCommand: pulumi.Bool(true),
		DeploymentCircuitBreaker: &ecs.ServiceDeploymentCircuitBreakerArgs{Enable: pulumi.Bool(true), Rollback: pulumi.Bool(true)},
		NetworkConfiguration:     &ecs.ServiceNetworkConfigurationArgs{AssignPublicIp: pulumi.Bool(false), Subnets: args.PrivateSubnetIDs, SecurityGroups: pulumi.StringArray{securityGroupID}},
		PropagateTags:            pulumi.String("TASK_DEFINITION"), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "web-service"),
	}
	if args.TargetGroupARN != nil {
		serviceArgs.LoadBalancers = ecs.ServiceLoadBalancerArray{ecs.ServiceLoadBalancerArgs{ContainerName: pulumi.String("web"), ContainerPort: pulumi.Int(args.ContainerPort), TargetGroupArn: targetGroupInput(args.TargetGroupARN)}}
	}
	service, err := ecs.NewService(ctx, name+"-web-service", serviceArgs, child)
	if err != nil {
		return nil, err
	}
	component.ServiceID = service.ID()
	component.ServiceName = service.Name

	cronDefinitions, err := containerDefinitionsForInput(args, secrets, "cron", []string{"/bin/sh", "-c", "while true; do bin/magento cron:run; sleep 60; done"}, false)
	if err != nil {
		return nil, err
	}
	cronTaskDefinition, err := ecs.NewTaskDefinition(ctx, name+"-cron-task", &ecs.TaskDefinitionArgs{
		Family: pulumi.String(name + "-cron"), ContainerDefinitions: cronDefinitions, Cpu: pulumi.String(args.TaskCPU), Memory: pulumi.String(args.TaskMemory),
		ExecutionRoleArn: identity.ExecutionRoleARN, TaskRoleArn: identity.TaskRoleARN, NetworkMode: pulumi.String("awsvpc"), RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
		RuntimePlatform: &ecs.TaskDefinitionRuntimePlatformArgs{CpuArchitecture: pulumi.String("X86_64"), OperatingSystemFamily: pulumi.String("LINUX")},
		Volumes:         ecs.TaskDefinitionVolumeArray{ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("tmp")}, ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("var")}},
		Region:          pulumi.String(args.Region), Tags: tags(args.Tags, name, "cron-task"),
	}, child)
	if err != nil {
		return nil, err
	}
	component.CronTaskDefinitionARN = cronTaskDefinition.Arn
	cronService, err := ecs.NewService(ctx, name+"-cron-service", &ecs.ServiceArgs{
		Cluster: cluster.Arn, TaskDefinition: cronTaskDefinition.Arn, DesiredCount: pulumi.Int(1), LaunchType: pulumi.String("FARGATE"),
		AvailabilityZoneRebalancing: pulumi.String("ENABLED"), EnableEcsManagedTags: pulumi.Bool(true), EnableExecuteCommand: pulumi.Bool(true),
		DeploymentCircuitBreaker: &ecs.ServiceDeploymentCircuitBreakerArgs{Enable: pulumi.Bool(true), Rollback: pulumi.Bool(true)},
		NetworkConfiguration:     &ecs.ServiceNetworkConfigurationArgs{AssignPublicIp: pulumi.Bool(false), Subnets: args.PrivateSubnetIDs, SecurityGroups: pulumi.StringArray{securityGroupID}},
		PropagateTags:            pulumi.String("TASK_DEFINITION"), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "cron-service"),
	}, child)
	if err != nil {
		return nil, err
	}
	component.CronServiceName = cronService.Name

	if args.QueueConsumerCount > 0 {
		queueDefinitions, err := containerDefinitionsForInput(args, secrets, "queue", []string{"bin/magento", "queue:consumers:start", "async.operations.all"}, false)
		if err != nil {
			return nil, err
		}
		queueTaskDefinition, err := ecs.NewTaskDefinition(ctx, name+"-queue-task", &ecs.TaskDefinitionArgs{
			Family: pulumi.String(name + "-queue"), ContainerDefinitions: queueDefinitions, Cpu: pulumi.String(args.TaskCPU), Memory: pulumi.String(args.TaskMemory),
			ExecutionRoleArn: identity.ExecutionRoleARN, TaskRoleArn: identity.TaskRoleARN, NetworkMode: pulumi.String("awsvpc"), RequiresCompatibilities: pulumi.StringArray{pulumi.String("FARGATE")},
			RuntimePlatform: &ecs.TaskDefinitionRuntimePlatformArgs{CpuArchitecture: pulumi.String("X86_64"), OperatingSystemFamily: pulumi.String("LINUX")},
			Volumes:         ecs.TaskDefinitionVolumeArray{ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("tmp")}, ecs.TaskDefinitionVolumeArgs{Name: pulumi.String("var")}},
			Region:          pulumi.String(args.Region), Tags: tags(args.Tags, name, "queue-task"),
		}, child)
		if err != nil {
			return nil, err
		}
		component.QueueTaskDefinitionARN = queueTaskDefinition.Arn
		queueService, err := ecs.NewService(ctx, name+"-queue-service", &ecs.ServiceArgs{
			Cluster: cluster.Arn, TaskDefinition: queueTaskDefinition.Arn, DesiredCount: pulumi.Int(args.QueueConsumerCount), LaunchType: pulumi.String("FARGATE"),
			AvailabilityZoneRebalancing: pulumi.String("ENABLED"), EnableEcsManagedTags: pulumi.Bool(true), EnableExecuteCommand: pulumi.Bool(true),
			DeploymentCircuitBreaker: &ecs.ServiceDeploymentCircuitBreakerArgs{Enable: pulumi.Bool(true), Rollback: pulumi.Bool(true)},
			NetworkConfiguration:     &ecs.ServiceNetworkConfigurationArgs{AssignPublicIp: pulumi.Bool(false), Subnets: args.PrivateSubnetIDs, SecurityGroups: pulumi.StringArray{securityGroupID}},
			PropagateTags:            pulumi.String("TASK_DEFINITION"), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "queue-service"),
		}, child)
		if err != nil {
			return nil, err
		}
		component.QueueServiceName = queueService.Name
	}
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"clusterName": cluster.Name, "clusterArn": cluster.Arn, "serviceName": service.Name, "executionRoleArn": identity.ExecutionRoleARN, "taskRoleArn": identity.TaskRoleARN, "taskRoleName": identity.TaskRoleName, "deploymentRoleArn": identity.DeploymentRoleARN,
		"taskDefinitionArn": taskDefinition.Arn, "deployTaskDefinitionArn": component.DeployTaskDefinitionARN, "cronTaskDefinitionArn": component.CronTaskDefinitionARN, "cronServiceName": component.CronServiceName, "queueTaskDefinitionArn": component.QueueTaskDefinitionARN, "queueServiceName": component.QueueServiceName, "serviceId": service.ID(), "securityGroupId": component.SecurityGroupID, "deploymentRoleName": component.DeploymentRoleName,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func targetGroupInput(input pulumi.StringInput) pulumi.StringPtrInput {
	if value, ok := input.(pulumi.String); ok {
		return pulumi.StringPtr(string(value))
	}
	return input.ToStringOutput().ToStringPtrOutput()
}

func validate(name string, args Args) ([]SecretReference, error) {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" || args.VpcID == nil || len(args.PrivateSubnetIDs) == 0 {
		return nil, errors.New("runtime name, region, VPC, and private subnets are required")
	}
	if args.ApplicationMode != "integrated" && args.ApplicationMode != "headless" {
		return nil, errors.New("runtime application mode must be integrated or headless")
	}
	if args.WebRuntime != "nginx-fpm" && args.WebRuntime != "frankenphp-classic" {
		return nil, errors.New("runtime web runtime must be nginx-fpm or frankenphp-classic")
	}
	if args.ApplicationMode == "integrated" {
		if !imageDigest.MatchString(args.VarnishImage) {
			return nil, errors.New("integrated runtime requires a Varnish image pinned by a lowercase SHA-256 digest")
		}
	} else if args.VarnishImage != "" {
		return nil, errors.New("headless runtime cannot include a Varnish image")
	}
	if (args.WebSecurityGroupID == nil) != (args.TargetGroupARN == nil) {
		return nil, errors.New("runtime external web security group and target group must be provided together")
	}
	if !imageDigest.MatchString(args.Image) {
		return nil, errors.New("runtime image must be pinned by a lowercase SHA-256 digest")
	}
	if args.SearchProxyImage != "" && !imageDigest.MatchString(args.SearchProxyImage) {
		return nil, errors.New("search proxy image must be pinned by a lowercase SHA-256 digest")
	}
	if args.SearchProxyImage != "" && (args.Capabilities == nil || args.Capabilities.SearchEndpoint == nil) {
		return nil, errors.New("search proxy requires a search endpoint")
	}
	if args.ContainerPort < 1 || args.ContainerPort > 65535 {
		return nil, errors.New("container port must be between 1 and 65535")
	}
	cpu, cpuErr := strconv.Atoi(args.TaskCPU)
	memory, memoryErr := strconv.Atoi(args.TaskMemory)
	if cpuErr != nil || memoryErr != nil || cpu < 1 || memory < 1 || args.DesiredCount < 1 {
		return nil, errors.New("benchmark-selected task CPU, memory, and desired count are required")
	}
	if args.Capabilities != nil {
		values := []pulumi.StringInput{
			args.Capabilities.DatabaseWriterEndpoint, args.Capabilities.DatabaseSecretARN, args.Capabilities.CacheEndpoint,
			args.Capabilities.SessionEndpoint, args.Capabilities.SearchEndpoint, args.Capabilities.QueueMode,
			args.Capabilities.QueueEndpoint, args.Capabilities.QueueUsername, args.Capabilities.MediaBucket,
		}
		for _, value := range values {
			if value == nil {
				return nil, errors.New("runtime capability references must be complete")
			}
		}
	}
	if args.EncryptionKeyARN == nil {
		return nil, errors.New("runtime Magento encryption key secret is required")
	}
	if args.DatabaseSecretARN == nil {
		return nil, errors.New("runtime managed database secret is required")
	}
	secrets, err := validateSecretReferences(args.Secrets)
	if err != nil {
		return nil, err
	}
	for _, secret := range secrets {
		if secret.Name == "MAGELIFT_DATABASE_CREDENTIALS" {
			return nil, errors.New("MAGELIFT_DATABASE_CREDENTIALS is reserved for the managed database secret")
		}
	}
	if !hasSecretReference(secrets, "MAGENTO_DC_CRYPT__KEY") {
		return nil, errors.New("runtime secrets must include MAGENTO_DC_CRYPT__KEY")
	}
	for _, secret := range secrets {
		if secret.Name == "MAGENTO_DC_CRYPT__KEY" && secret.JSONKey != "" {
			return nil, errors.New("MAGENTO_DC_CRYPT__KEY must reference the full secret value")
		}
	}
	if encryptionInput, ok := args.EncryptionKeyARN.(pulumi.String); ok {
		for _, secret := range secrets {
			if secret.Name == "MAGENTO_DC_CRYPT__KEY" && secret.ARN != string(encryptionInput) {
				return nil, errors.New("runtime encryption key secret reference does not match the task secret")
			}
		}
	}
	if args.Identity != nil && !sameSecretReferences(secrets, args.Identity.secretReferences) {
		return nil, errors.New("runtime identity and task definitions must use the same secret references")
	}
	return secrets, nil
}

func validateSecretReferences(input []SecretReference) ([]SecretReference, error) {
	result := append([]SecretReference(nil), input...)
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	for index, secret := range result {
		if !secretName.MatchString(secret.Name) || !secretARN.MatchString(secret.ARN) {
			return nil, errors.New("task secrets require a stable environment name and Secrets Manager or SSM ARN")
		}
		if secret.JSONKey != "" && (!strings.Contains(secret.ARN, ":secret:") || !secretJSONKey.MatchString(secret.JSONKey)) {
			return nil, errors.New("JSON key selectors require a Secrets Manager ARN and a safe key name")
		}
		if index > 0 && result[index-1].Name == secret.Name {
			return nil, fmt.Errorf("duplicate task secret name %q", secret.Name)
		}
	}
	return result, nil
}

func sameSecretReferences(left, right []SecretReference) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func hasSecretReference(secrets []SecretReference, name string) bool {
	for _, secret := range secrets {
		if secret.Name == name {
			return true
		}
	}
	return false
}

func role(ctx *pulumi.Context, name, description, assume string, inputTags map[string]string, component string, parent pulumi.Resource) (*iam.Role, error) {
	return iam.NewRole(ctx, name, &iam.RoleArgs{AssumeRolePolicy: pulumi.String(assume), Description: pulumi.String(description), Tags: tags(inputTags, component, name)}, pulumi.Parent(parent))
}

func executionPolicy(secrets []SecretReference) (string, error) {
	type statement struct {
		Effect   string   `json:"Effect"`
		Action   []string `json:"Action"`
		Resource any      `json:"Resource"`
	}
	document := struct {
		Version   string      `json:"Version"`
		Statement []statement `json:"Statement"`
	}{Version: "2012-10-17", Statement: []statement{
		{Effect: "Allow", Action: []string{"ecr:GetAuthorizationToken"}, Resource: "*"},
		{Effect: "Allow", Action: []string{"ecr:BatchCheckLayerAvailability", "ecr:BatchGetImage", "ecr:GetDownloadUrlForLayer"}, Resource: "*"},
	}}
	byService := map[string][]string{}
	seen := map[string]map[string]bool{}
	for _, secret := range secrets {
		match := secretARN.FindStringSubmatch(secret.ARN)
		if seen[match[1]] == nil {
			seen[match[1]] = map[string]bool{}
		}
		if !seen[match[1]][secret.ARN] {
			byService[match[1]] = append(byService[match[1]], secret.ARN)
			seen[match[1]][secret.ARN] = true
		}
	}
	if values := byService["secretsmanager"]; len(values) > 0 {
		document.Statement = append(document.Statement, statement{Effect: "Allow", Action: []string{"secretsmanager:GetSecretValue"}, Resource: values})
	}
	if values := byService["ssm"]; len(values) > 0 {
		document.Statement = append(document.Statement, statement{Effect: "Allow", Action: []string{"ssm:GetParameters"}, Resource: values})
	}
	encoded, err := json.Marshal(document)
	return string(encoded), err
}

func containerDefinitionsInput(args Args, secrets []SecretReference) (pulumi.StringInput, error) {
	return pulumi.All(capabilityEnvironment(args), args.DatabaseSecretARN, args.EncryptionKeyARN, searchEndpointInput(args)).ApplyT(func(values []interface{}) (string, error) {
		environment, ok := values[0].([]containerEnvironment)
		if !ok {
			return "", errors.New("resolve runtime capability environment")
		}
		databaseARN, ok := values[1].(string)
		if !ok || strings.TrimSpace(databaseARN) == "" {
			return "", errors.New("resolve database secret reference")
		}
		encryptionARN, ok := values[2].(string)
		if !ok || strings.TrimSpace(encryptionARN) == "" {
			return "", errors.New("resolve Magento encryption key secret reference")
		}
		searchEndpoint, ok := values[3].(string)
		if !ok {
			return "", errors.New("resolve OpenSearch endpoint for signing proxy")
		}
		return containerDefinitions(args, appendEncryptionSecret(secrets, encryptionARN), environment, databaseARN, searchEndpoint)
	}).(pulumi.StringOutput), nil
}

func searchEndpointInput(args Args) pulumi.StringInput {
	if args.Capabilities == nil || args.Capabilities.SearchEndpoint == nil {
		return pulumi.String("")
	}
	return args.Capabilities.SearchEndpoint
}

// FrontendPort returns the task port reached by the load balancer. Integrated
// Magento requests enter through Varnish; headless requests enter the
// application listener directly.
func FrontendPort(applicationMode string) int {
	if applicationMode == "integrated" {
		return VarnishPort
	}
	return ApplicationPort
}

func containerDefinitions(args Args, secrets []SecretReference, environment []containerEnvironment, databaseARN, searchEndpoint string) (string, error) {
	if args.WebRuntime == "nginx-fpm" {
		php := baseContainer(args, appendDatabaseSecret(secrets, databaseARN), environment, "php-fpm", nil, false)
		web := baseContainer(args, nil, nil, "web", []string{"nginx", "-g", "daemon off;"}, args.ApplicationMode != "integrated")
		containers, err := appendSearchProxy(args, []containerDefinition{php, web}, searchEndpoint)
		if err != nil {
			return "", err
		}
		containers, err = appendVarnish(args, containers)
		if err != nil {
			return "", err
		}
		encoded, err := json.Marshal(containers)
		return string(encoded), err
	}
	return containerDefinitionsFor(args, appendDatabaseSecret(secrets, databaseARN), environment, "web", nil, true, searchEndpoint)
}

type containerSecret struct {
	Name      string `json:"name"`
	ValueFrom string `json:"valueFrom"`
}

type containerMount struct {
	SourceVolume  string `json:"sourceVolume"`
	ContainerPath string `json:"containerPath"`
	ReadOnly      bool   `json:"readOnly"`
}

type containerPort struct {
	ContainerPort int    `json:"containerPort"`
	Protocol      string `json:"protocol"`
}

type containerLinux struct {
	InitProcessEnabled bool             `json:"initProcessEnabled"`
	Tmpfs              []containerTmpfs `json:"tmpfs,omitempty"`
}

type containerTmpfs struct {
	ContainerPath string   `json:"containerPath"`
	MountOptions  []string `json:"mountOptions,omitempty"`
	Size          int      `json:"size"`
}

type containerDependency struct {
	ContainerName string `json:"containerName"`
	Condition     string `json:"condition"`
}

type containerEnvironment struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type containerDefinition struct {
	Name                   string                 `json:"name"`
	Image                  string                 `json:"image"`
	Command                []string               `json:"command,omitempty"`
	Essential              bool                   `json:"essential"`
	User                   string                 `json:"user,omitempty"`
	ReadonlyRootFilesystem bool                   `json:"readonlyRootFilesystem"`
	LinuxParameters        containerLinux         `json:"linuxParameters"`
	PortMappings           []containerPort        `json:"portMappings"`
	MountPoints            []containerMount       `json:"mountPoints"`
	DependsOn              []containerDependency  `json:"dependsOn,omitempty"`
	Secrets                []containerSecret      `json:"secrets,omitempty"`
	Environment            []containerEnvironment `json:"environment,omitempty"`
}

func containerDefinitionsForInput(args Args, secrets []SecretReference, name string, command []string, exposePort bool) (pulumi.StringInput, error) {
	return pulumi.All(capabilityEnvironment(args), args.DatabaseSecretARN, args.EncryptionKeyARN, searchEndpointInput(args)).ApplyT(func(values []interface{}) (string, error) {
		environment, ok := values[0].([]containerEnvironment)
		if !ok {
			return "", errors.New("resolve runtime capability environment")
		}
		databaseARN, ok := values[1].(string)
		if !ok || strings.TrimSpace(databaseARN) == "" {
			return "", errors.New("resolve database secret reference")
		}
		encryptionARN, ok := values[2].(string)
		if !ok || strings.TrimSpace(encryptionARN) == "" {
			return "", errors.New("resolve Magento encryption key secret reference")
		}
		searchEndpoint, ok := values[3].(string)
		if !ok {
			return "", errors.New("resolve OpenSearch endpoint for signing proxy")
		}
		return containerDefinitionsFor(args, appendDatabaseSecret(appendEncryptionSecret(secrets, encryptionARN), databaseARN), environment, name, command, exposePort, searchEndpoint)
	}).(pulumi.StringOutput), nil
}

func containerDefinitionsFor(args Args, secrets []SecretReference, environment []containerEnvironment, name string, command []string, exposePort bool, searchEndpoint string) (string, error) {
	containers, err := appendSearchProxy(args, []containerDefinition{baseContainer(args, secrets, environment, name, command, exposePort && args.ApplicationMode != "integrated")}, searchEndpoint)
	if err != nil {
		return "", err
	}
	if name == "web" {
		containers, err = appendVarnish(args, containers)
		if err != nil {
			return "", err
		}
	}
	encoded, err := json.Marshal(containers)
	return string(encoded), err
}

func appendSearchProxy(args Args, containers []containerDefinition, endpoint string) ([]containerDefinition, error) {
	if args.SearchProxyImage == "" {
		return containers, nil
	}
	host, service := searchProxyTarget(endpoint)
	if host == "" {
		return nil, errors.New("OpenSearch endpoint is required for the signing proxy")
	}
	proxy := containerDefinition{
		Name:                   "search-proxy",
		Image:                  args.SearchProxyImage,
		Command:                []string{"--port", strconv.Itoa(searchProxyPort), "--name", service, "--region", args.Region, "--host", host, "--sign-host", host, "--upstream-url-scheme", "https"},
		Essential:              true,
		ReadonlyRootFilesystem: true,
		LinuxParameters:        containerLinux{InitProcessEnabled: false},
	}
	for index := range containers {
		if args.WebRuntime != "nginx-fpm" || containers[index].Name == "php-fpm" {
			containers[index].DependsOn = []containerDependency{{ContainerName: "search-proxy", Condition: "START"}}
		}
	}
	return append(containers, proxy), nil
}

func appendVarnish(args Args, containers []containerDefinition) ([]containerDefinition, error) {
	if args.ApplicationMode != "integrated" {
		return containers, nil
	}
	if !imageDigest.MatchString(args.VarnishImage) {
		return nil, errors.New("integrated runtime requires a Varnish image pinned by a lowercase SHA-256 digest")
	}
	return append(containers, containerDefinition{
		Name:      "varnish",
		Image:     args.VarnishImage,
		Essential: true,
		User:      "varnish",
		// Varnish creates its VSM working directory under /var/lib/varnish.
		// Keep the image root read-only and provide only an executable,
		// task-scoped cache filesystem for VSM and the transient cache.
		ReadonlyRootFilesystem: true,
		LinuxParameters: containerLinux{
			InitProcessEnabled: false,
			Tmpfs: []containerTmpfs{{
				ContainerPath: "/var/lib/varnish",
				MountOptions:  []string{"rw", "exec", "uid=1000", "gid=1000", "mode=0750"},
				Size:          384,
			}},
		},
		PortMappings: []containerPort{{ContainerPort: VarnishPort, Protocol: "tcp"}},
		DependsOn:    []containerDependency{{ContainerName: "web", Condition: "START"}},
		Environment: []containerEnvironment{
			{Name: "VARNISH_BACKEND_HOST", Value: "127.0.0.1"},
			{Name: "VARNISH_BACKEND_PORT", Value: strconv.Itoa(ApplicationPort)},
			{Name: "VARNISH_HTTP_PORT", Value: strconv.Itoa(VarnishPort)},
			{Name: "VARNISH_SIZE", Value: "256M"},
		},
	}), nil
}

func searchProxyTarget(endpoint string) (string, string) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		parsed, _ = url.Parse("https://" + endpoint)
	}
	host := parsed.Hostname()
	service := "es"
	if strings.Contains(host, ".aoss.") {
		service = "aoss"
	}
	return host, service
}

func appendDatabaseSecret(secrets []SecretReference, databaseARN string) []SecretReference {
	if strings.TrimSpace(databaseARN) == "" {
		return append([]SecretReference(nil), secrets...)
	}
	result := append([]SecretReference(nil), secrets...)
	result = append(result, SecretReference{Name: "MAGELIFT_DATABASE_CREDENTIALS", ARN: databaseARN})
	for _, field := range []struct {
		name string
		key  string
	}{
		{"MAGENTO_DC_DB__CONNECTION__DEFAULT__HOST", "host"},
		{"MAGENTO_DC_DB__CONNECTION__DEFAULT__PORT", "port"},
		{"MAGENTO_DC_DB__CONNECTION__DEFAULT__DBNAME", "dbname"},
		{"MAGENTO_DC_DB__CONNECTION__DEFAULT__USERNAME", "username"},
		{"MAGENTO_DC_DB__CONNECTION__DEFAULT__PASSWORD", "password"},
	} {
		result = append(result, SecretReference{Name: field.name, ARN: databaseARN, JSONKey: field.key})
	}
	return result
}

func appendEncryptionSecret(secrets []SecretReference, encryptionARN string) []SecretReference {
	result := append([]SecretReference(nil), secrets...)
	for _, secret := range result {
		if secret.Name == "MAGENTO_DC_CRYPT__KEY" {
			return result
		}
	}
	return append(result, SecretReference{Name: "MAGENTO_DC_CRYPT__KEY", ARN: encryptionARN})
}

func deploymentCommand() []string {
	return []string{"/bin/sh", "-ec", "bin/magento app:config:import --no-interaction && bin/magento setup:upgrade --keep-generated --no-interaction && bin/magento cache:clean && bin/magento cache:flush"}
}

func baseContainer(args Args, secrets []SecretReference, environment []containerEnvironment, name string, command []string, exposePort bool) containerDefinition {
	selected := make([]containerSecret, len(secrets))
	for index, value := range secrets {
		selected[index] = containerSecret{Name: value.Name, ValueFrom: value.ValueFrom()}
	}
	container := containerDefinition{Name: name, Image: args.Image, Command: command, Essential: true, User: "10001:10001", ReadonlyRootFilesystem: true, LinuxParameters: containerLinux{InitProcessEnabled: true}, MountPoints: []containerMount{{SourceVolume: "tmp", ContainerPath: "/tmp"}, {SourceVolume: "var", ContainerPath: "/app/var"}}, Secrets: selected, Environment: environment}
	if exposePort {
		container.PortMappings = []containerPort{{ContainerPort: args.ContainerPort, Protocol: "tcp"}}
	}
	return container
}

func capabilityEnvironment(args Args) pulumi.Output {
	if args.Capabilities == nil {
		return pulumi.ToOutput([]containerEnvironment{
			{Name: "MAGELIFT_APPLICATION_MODE", Value: args.ApplicationMode},
			{Name: "MAGELIFT_WEB_RUNTIME", Value: args.WebRuntime},
		})
	}
	capabilities := args.Capabilities
	return pulumi.All(
		capabilities.DatabaseWriterEndpoint, capabilities.DatabaseSecretARN, capabilities.CacheEndpoint, capabilities.SessionEndpoint,
		capabilities.SearchEndpoint, capabilities.QueueMode, capabilities.QueueEndpoint, capabilities.QueueUsername, capabilities.MediaBucket,
	).ApplyT(func(values []interface{}) []containerEnvironment {
		queueMode := values[5].(string)
		queueEndpoint := values[6].(string)
		searchEndpoint := values[4].(string)
		queueConnection := "db"
		if queueMode == "rabbitmq" {
			queueConnection = "amqp"
		}
		queueHost, queuePort, queueSSL := amqpSettings(queueEndpoint)
		environment := []containerEnvironment{
			{Name: "MAGELIFT_APPLICATION_MODE", Value: args.ApplicationMode},
			{Name: "MAGELIFT_WEB_RUNTIME", Value: args.WebRuntime},
			{Name: "MAGELIFT_DATABASE_WRITER", Value: values[0].(string)},
			{Name: "MAGELIFT_DATABASE_SECRET_ARN", Value: values[1].(string)},
			{Name: "MAGELIFT_CACHE_ENDPOINT", Value: values[2].(string)},
			{Name: "MAGELIFT_SESSION_ENDPOINT", Value: values[3].(string)},
			{Name: "MAGELIFT_SEARCH_ENDPOINT", Value: searchEndpoint},
			{Name: "MAGELIFT_QUEUE_MODE", Value: queueMode},
			{Name: "MAGELIFT_QUEUE_ENDPOINT", Value: queueEndpoint},
			{Name: "MAGELIFT_MEDIA_BUCKET", Value: values[8].(string)},
			{Name: "MAGENTO_DC_DB__CONNECTION__DEFAULT__MODEL", Value: "mysql4"},
			{Name: "MAGENTO_DC_DB__CONNECTION__DEFAULT__ENGINE", Value: "innodb"},
			{Name: "MAGENTO_DC_DB__CONNECTION__DEFAULT__INITSTATEMENTS", Value: "SET NAMES utf8;"},
			{Name: "MAGENTO_DC_DB__CONNECTION__DEFAULT__ACTIVE", Value: "1"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND", Value: "Magento\\Framework\\Cache\\Backend\\Redis"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__SERVER", Value: values[2].(string)},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__PORT", Value: "6379"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__DATABASE", Value: "0"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND", Value: "Magento\\Framework\\Cache\\Backend\\Redis"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__SERVER", Value: values[2].(string)},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__PORT", Value: "6379"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__DATABASE", Value: "1"},
			{Name: "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__COMPRESS_DATA", Value: "0"},
			{Name: "MAGENTO_DC_SESSION__SAVE", Value: "redis"},
			{Name: "MAGENTO_DC_SESSION__REDIS_HOST", Value: values[3].(string)},
			{Name: "MAGENTO_DC_SESSION__REDIS_PORT", Value: "6379"},
			{Name: "MAGENTO_DC_SESSION__REDIS_DB", Value: "2"},
			{Name: "MAGENTO_DC_QUEUE__DEFAULT_CONNECTION", Value: queueConnection},
			{Name: "MAGENTO_DC_QUEUE__AMQP__HOST", Value: queueHost},
			{Name: "MAGENTO_DC_QUEUE__AMQP__PORT", Value: queuePort},
			{Name: "MAGENTO_DC_QUEUE__AMQP__SSL", Value: queueSSL},
			{Name: "MAGENTO_DC_QUEUE__AMQP__USERNAME", Value: values[7].(string)},
		}
		if args.SearchProxyImage != "" {
			environment = append(environment,
				containerEnvironment{Name: "MAGENTO_DC_CATALOG__SEARCH__ENGINE", Value: "opensearch"},
				containerEnvironment{Name: "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_HOSTNAME", Value: "127.0.0.1"},
				containerEnvironment{Name: "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_PORT", Value: strconv.Itoa(searchProxyPort)},
				containerEnvironment{Name: "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_INDEX_PREFIX", Value: "magento2"},
				containerEnvironment{Name: "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_ENABLE_AUTH", Value: "0"},
				containerEnvironment{Name: "MAGENTO_DC_CATALOG__SEARCH__OPENSEARCH_SERVER_TIMEOUT", Value: "15"},
			)
		}
		return environment
	})
}

func amqpSettings(endpoint string) (string, string, string) {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Hostname() == "" {
		return "", "", ""
	}
	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "amqps" {
			port = "5671"
		} else {
			port = "5672"
		}
	}
	ssl := "0"
	if parsed.Scheme == "amqps" {
		ssl = "1"
	}
	return parsed.Hostname(), port, ssl
}

func tags(input map[string]string, component, role string) pulumi.StringMap {
	result := pulumi.StringMap{}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = pulumi.String(input[key])
	}
	result["Name"] = pulumi.String(component + "-" + role)
	result["magelift:component"] = pulumi.String(component)
	result["magelift:role"] = pulumi.String(role)
	return result
}
