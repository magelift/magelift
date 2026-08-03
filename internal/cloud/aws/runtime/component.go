package runtime

import (
	"sort"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ecs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

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
	if err := attachExecutionLogPolicy(ctx, name, args, identity, component); err != nil {
		return nil, err
	}

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
		loadBalancerContainer := "web"
		if args.ApplicationMode == "integrated" {
			// Integrated traffic enters Varnish; nginx/frankenphp stay internal.
			loadBalancerContainer = "varnish"
		}
		serviceArgs.LoadBalancers = ecs.ServiceLoadBalancerArray{ecs.ServiceLoadBalancerArgs{ContainerName: pulumi.String(loadBalancerContainer), ContainerPort: pulumi.Int(args.ContainerPort), TargetGroupArn: targetGroupInput(args.TargetGroupARN)}}
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
