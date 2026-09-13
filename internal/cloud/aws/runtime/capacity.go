package runtime

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/autoscaling"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ecs"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	ComputeModeFargate         = "fargate"
	ComputeModeFargateSpot     = "fargate-spot"
	ComputeModeEC2AutoScaling  = "ec2-asg"
	ComputeModeManagedInstance = "managed-instances"
)

var amiID = regexp.MustCompile(`^ami-[A-Za-z0-9]+$`)

type capacityConfiguration struct {
	taskCompatibility string
	launchType        string
	serviceStrategy   ecs.ServiceCapacityProviderStrategyArrayInput
	dependencies      []pulumi.Resource
	ready             pulumi.StringInput
}

func (c capacityConfiguration) applyService(args *ecs.ServiceArgs) {
	if c.launchType != "" {
		args.LaunchType = pulumi.String(c.launchType)
		return
	}
	args.CapacityProviderStrategies = c.serviceStrategy
	if c.ready != nil {
		if m, ok := args.Tags.(pulumi.StringMap); ok {
			m["magelift:capacity-ready"] = c.ready
			args.Tags = m
		}
	}
}

func (c capacityConfiguration) serviceOptions(parent pulumi.Resource) []pulumi.ResourceOption {
	options := []pulumi.ResourceOption{pulumi.Parent(parent)}
	if len(c.dependencies) > 0 {
		options = append(options, pulumi.DependsOn(c.dependencies))
	}
	return options
}

func configureCapacity(ctx *pulumi.Context, name string, args Args, cluster *ecs.Cluster, securityGroupID pulumi.StringOutput, parent pulumi.Resource) (capacityConfiguration, error) {
	mode := strings.TrimSpace(args.ComputeMode)
	if mode == "" {
		mode = ComputeModeFargate
	}
	base := capacityConfiguration{taskCompatibility: "FARGATE", launchType: "FARGATE"}
	switch mode {
	case ComputeModeFargate:
		return base, nil
	case ComputeModeFargateSpot:
		association, err := associateCapacityProviders(ctx, name, cluster.Name, pulumi.StringArray{pulumi.String("FARGATE"), pulumi.String("FARGATE_SPOT")}, pulumi.String("FARGATE_SPOT"), parent)
		if err != nil {
			return capacityConfiguration{}, err
		}
		base.launchType = ""
		base.serviceStrategy = ecs.ServiceCapacityProviderStrategyArray{
			&ecs.ServiceCapacityProviderStrategyArgs{CapacityProvider: pulumi.String("FARGATE_SPOT"), Weight: pulumi.Int(1)},
		}
		base.dependencies = []pulumi.Resource{association}
		return base, nil
	case ComputeModeEC2AutoScaling:
		return configureEC2Capacity(ctx, name, args, cluster, securityGroupID, parent)
	case ComputeModeManagedInstance:
		return configureManagedInstanceCapacity(ctx, name, args, cluster, securityGroupID, parent)
	default:
		return capacityConfiguration{}, fmt.Errorf("unsupported ECS compute mode %q", mode)
	}
}

func configureEC2Capacity(ctx *pulumi.Context, name string, args Args, cluster *ecs.Cluster, securityGroupID pulumi.StringOutput, parent pulumi.Resource) (capacityConfiguration, error) {
	if !amiID.MatchString(strings.TrimSpace(args.InstanceAMI)) {
		return capacityConfiguration{}, errors.New("ECS EC2 capacity requires a pinned ami-* instanceAMI")
	}
	instanceType := strings.TrimSpace(args.InstanceType)
	if instanceType == "" {
		instanceType = "t3.medium"
	}
	minSize, desiredSize, maxSize := hostCapacity(args)

	hostRole, err := iam.NewRole(ctx, name+"-ecs-host-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(ec2AssumeRolePolicy),
		Description:      pulumi.String("MageLift ECS EC2 host role"),
		Tags:             tags(args.Tags, name, "ecs-host-role"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS host role: %w", err)
	}
	for _, policy := range []struct{ suffix, arn string }{
		{"ecs", "arn:aws:iam::aws:policy/service-role/AmazonEC2ContainerServiceforEC2Role"},
		{"ecr", "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryPullOnly"},
	} {
		if _, err := iam.NewRolePolicyAttachment(ctx, name+"-ecs-host-"+policy.suffix, &iam.RolePolicyAttachmentArgs{
			Role: hostRole.Name, PolicyArn: pulumi.String(policy.arn),
		}, pulumi.Parent(parent)); err != nil {
			return capacityConfiguration{}, fmt.Errorf("attach ECS host %s policy: %w", policy.suffix, err)
		}
	}
	instanceProfile, err := iam.NewInstanceProfile(ctx, name+"-ecs-host-profile", &iam.InstanceProfileArgs{
		Role: hostRole.Name, Tags: tags(args.Tags, name, "ecs-host-profile"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS host instance profile: %w", err)
	}
	launchTemplate, err := ec2.NewLaunchTemplate(ctx, name+"-ecs-host-template", &ec2.LaunchTemplateArgs{
		ImageId: pulumi.String(args.InstanceAMI), InstanceType: pulumi.String(instanceType),
		IamInstanceProfile:  &ec2.LaunchTemplateIamInstanceProfileArgs{Name: instanceProfile.Name},
		VpcSecurityGroupIds: pulumi.StringArray{securityGroupID},
		UserData:            ecsHostUserData(cluster.Name),
		MetadataOptions:     &ec2.LaunchTemplateMetadataOptionsArgs{HttpEndpoint: pulumi.String("enabled"), HttpTokens: pulumi.String("required")},
		TagSpecifications: ec2.LaunchTemplateTagSpecificationArray{
			&ec2.LaunchTemplateTagSpecificationArgs{ResourceType: pulumi.String("instance"), Tags: tags(args.Tags, name, "ecs-host")},
		},
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS host launch template: %w", err)
	}
	group, err := autoscaling.NewGroup(ctx, name+"-ecs-hosts", &autoscaling.GroupArgs{
		VpcZoneIdentifiers: args.PrivateSubnetIDs, MinSize: pulumi.Int(minSize), DesiredCapacity: pulumi.Int(desiredSize), MaxSize: pulumi.Int(maxSize),
		HealthCheckType: pulumi.String("EC2"), ProtectFromScaleIn: pulumi.Bool(true), CapacityRebalance: pulumi.Bool(true),
		LaunchTemplate: &autoscaling.GroupLaunchTemplateArgs{Id: launchTemplate.ID(), Version: pulumi.String("$Latest")},
		Tags: autoscaling.GroupTagArray{
			&autoscaling.GroupTagArgs{Key: pulumi.String("AmazonECSManaged"), Value: pulumi.String("true"), PropagateAtLaunch: pulumi.Bool(true)},
		},
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS host auto scaling group: %w", err)
	}
	provider, err := ecs.NewCapacityProvider(ctx, name+"-ecs-capacity-provider", &ecs.CapacityProviderArgs{
		Name: pulumi.String(ecsCapacityProviderName(name, "ec2")),
		AutoScalingGroupProvider: &ecs.CapacityProviderAutoScalingGroupProviderArgs{
			AutoScalingGroupArn: group.Arn, ManagedDraining: pulumi.String("ENABLED"), ManagedTerminationProtection: pulumi.String("ENABLED"),
			ManagedScaling: &ecs.CapacityProviderAutoScalingGroupProviderManagedScalingArgs{
				Status: pulumi.String("ENABLED"), TargetCapacity: pulumi.Int(80), MinimumScalingStepSize: pulumi.Int(1), MaximumScalingStepSize: pulumi.Int(2), InstanceWarmupPeriod: pulumi.Int(300),
			},
		},
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS EC2 capacity provider: %w", err)
	}
	association, err := associateCapacityProviders(ctx, name, cluster.Name, pulumi.StringArray{provider.Name}, provider.Name, parent)
	if err != nil {
		return capacityConfiguration{}, err
	}
	return capacityConfiguration{
		taskCompatibility: "EC2",
		serviceStrategy: ecs.ServiceCapacityProviderStrategyArray{
			&ecs.ServiceCapacityProviderStrategyArgs{CapacityProvider: provider.Name, Weight: pulumi.Int(1)},
		},
		dependencies: []pulumi.Resource{launchTemplate, group, provider, association},
	}, nil
}

func configureManagedInstanceCapacity(ctx *pulumi.Context, name string, args Args, cluster *ecs.Cluster, securityGroupID pulumi.StringOutput, parent pulumi.Resource) (capacityConfiguration, error) {
	instanceType := strings.TrimSpace(args.InstanceType)
	if instanceType == "" {
		instanceType = "m6i.large"
	}
	instanceRoleName := "ecsInstanceRole-" + name
	instanceRole, err := iam.NewRole(ctx, name+"-ecs-managed-instance-role", &iam.RoleArgs{
		Name: pulumi.String(instanceRoleName), AssumeRolePolicy: pulumi.String(ec2AssumeRolePolicy),
		Description: pulumi.String("MageLift ECS Managed Instance role"), Tags: tags(args.Tags, name, "ecs-managed-instance-role"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS Managed Instance role: %w", err)
	}
	instancePolicy, err := iam.NewRolePolicyAttachment(ctx, name+"-ecs-managed-instance-policy", &iam.RolePolicyAttachmentArgs{
		Role: instanceRole.Name, PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonECSInstanceRolePolicyForManagedInstances"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("attach ECS Managed Instance policy: %w", err)
	}
	instanceProfile, err := iam.NewInstanceProfile(ctx, name+"-ecs-managed-instance-profile", &iam.InstanceProfileArgs{
		Name: pulumi.String(instanceRoleName), Role: instanceRole.Name, Tags: tags(args.Tags, name, "ecs-managed-instance-profile"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS Managed Instance profile: %w", err)
	}
	infrastructureRole, err := iam.NewRole(ctx, name+"-ecs-infrastructure-role", &iam.RoleArgs{
		AssumeRolePolicy: pulumi.String(ecsInfrastructureAssumeRolePolicy), Description: pulumi.String("MageLift ECS infrastructure role"), Tags: tags(args.Tags, name, "ecs-infrastructure-role"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS infrastructure role: %w", err)
	}
	infrastructurePolicy, err := iam.NewRolePolicyAttachment(ctx, name+"-ecs-infrastructure-policy", &iam.RolePolicyAttachmentArgs{
		Role: infrastructureRole.Name, PolicyArn: pulumi.String("arn:aws:iam::aws:policy/AmazonECSInfrastructureRolePolicyForManagedInstances"),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("attach ECS infrastructure policy: %w", err)
	}
	passRole, err := iam.NewRolePolicy(ctx, name+"-ecs-infrastructure-pass-role", &iam.RolePolicyArgs{
		Role: infrastructureRole.Name, Policy: passRolePolicy(instanceRole.Arn),
	}, pulumi.Parent(parent))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("allow ECS infrastructure role to pass instance role: %w", err)
	}
	// RolePolicyAttachment can succeed before STS evaluations see the managed
	// policy. CreateCapacityProvider assumes the infrastructure role immediately
	// and fails with ec2:DescribeInstanceTypeOfferings unless we wait.
	iamReady := pulumi.All(infrastructurePolicy.ID(), instancePolicy.ID(), instanceProfile.ID(), passRole.ID()).ApplyT(func(values []interface{}) string {
		_ = values
		soakIfLive(ctx, managedInstanceIAMPropagation)
		return "ready"
	}).(pulumi.StringOutput)
	providerTags := tags(args.Tags, name, "ecs-managed-capacity")
	providerTags["magelift:iam-propagated"] = iamReady
	vcpu, memoryMiB := managedInstanceCPUMemory(instanceType)
	provider, err := ecs.NewCapacityProvider(ctx, name+"-ecs-managed-capacity-provider", &ecs.CapacityProviderArgs{
		Name: pulumi.String(ecsCapacityProviderName(name, "managed")), Cluster: cluster.Name,
		Tags: providerTags,
		ManagedInstancesProvider: &ecs.CapacityProviderManagedInstancesProviderArgs{
			InfrastructureRoleArn: infrastructureRole.Arn, PropagateTags: pulumi.String("CAPACITY_PROVIDER"),
			InstanceLaunchTemplate: &ecs.CapacityProviderManagedInstancesProviderInstanceLaunchTemplateArgs{
				CapacityOptionType: pulumi.String("ON_DEMAND"), Ec2InstanceProfileArn: instanceProfile.Arn, Monitoring: pulumi.String("DETAILED"),
				NetworkConfiguration: &ecs.CapacityProviderManagedInstancesProviderInstanceLaunchTemplateNetworkConfigurationArgs{
					Subnets: args.PrivateSubnetIDs, SecurityGroups: pulumi.StringArray{securityGroupID},
				},
				StorageConfiguration: &ecs.CapacityProviderManagedInstancesProviderInstanceLaunchTemplateStorageConfigurationArgs{StorageSizeGib: pulumi.Int(30)},
				// Pin a concrete SKU and omit instanceGenerations: AWS rejects
				// current/previous together with generation-specific types
				// (m6i.large). https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_InstanceRequirementsRequest.html
				InstanceRequirements: &ecs.CapacityProviderManagedInstancesProviderInstanceLaunchTemplateInstanceRequirementsArgs{
					AllowedInstanceTypes: pulumi.StringArray{pulumi.String(instanceType)},
					VcpuCount: &ecs.CapacityProviderManagedInstancesProviderInstanceLaunchTemplateInstanceRequirementsVcpuCountArgs{
						Min: pulumi.Int(vcpu), Max: pulumi.Int(vcpu),
					},
					MemoryMib: &ecs.CapacityProviderManagedInstancesProviderInstanceLaunchTemplateInstanceRequirementsMemoryMibArgs{
						Min: pulumi.Int(memoryMiB), Max: pulumi.Int(memoryMiB),
					},
				},
			},
		},
	}, pulumi.Parent(parent), pulumi.DependsOn([]pulumi.Resource{instancePolicy, instanceProfile, infrastructurePolicy, passRole}))
	if err != nil {
		return capacityConfiguration{}, fmt.Errorf("create ECS Managed Instance capacity provider: %w", err)
	}
	// CreateCapacityProvider can return before status=ACTIVE. CreateService
	// then fails with InvalidParameterException. DependsOn the provider is
	// graph order only. https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_CreateCapacityProvider.html
	capacityReady := provider.ID().ApplyT(func(id pulumi.ID) string {
		_ = id
		soakIfLive(ctx, managedInstanceCapacityProviderActive)
		return "ready"
	}).(pulumi.StringOutput)
	// Managed Instances CPs are bound to the cluster at CreateCapacityProvider
	// time. PutClusterCapacityProviders is unsupported and races while the CP
	// is not yet ACTIVE. https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_PutClusterCapacityProviders.html
	return capacityConfiguration{
		taskCompatibility: "MANAGED_INSTANCES",
		serviceStrategy: ecs.ServiceCapacityProviderStrategyArray{
			&ecs.ServiceCapacityProviderStrategyArgs{CapacityProvider: provider.Name, Weight: pulumi.Int(1)},
		},
		dependencies: []pulumi.Resource{instancePolicy, instanceProfile, infrastructurePolicy, passRole, provider},
		ready:        capacityReady,
	}, nil
}

// managedInstanceIAMPropagation is how long CreateCapacityProvider waits after
// IAM attachments return. AWS documents that new role policy attachments can
// take several seconds to become visible to STS-assumed callers.
const managedInstanceIAMPropagation = 30 * time.Second

// managedInstanceCapacityProviderActive is how long CreateService waits after
// CreateCapacityProvider returns. The provider can still be PROVISIONING.
const managedInstanceCapacityProviderActive = 60 * time.Second

func soakIfLive(ctx *pulumi.Context, d time.Duration) {
	if !shouldSoakManagedInstanceIAM(ctx.DryRun(), runningUnderGoTest()) {
		return
	}
	time.Sleep(d)
}

func shouldSoakManagedInstanceIAM(dryRun, underGoTest bool) bool {
	return !dryRun && !underGoTest
}

func runningUnderGoTest() bool {
	return flag.Lookup("test.v") != nil
}

func associateCapacityProviders(ctx *pulumi.Context, name string, clusterName pulumi.StringOutput, providers pulumi.StringArrayInput, defaultProvider pulumi.StringInput, parent pulumi.Resource) (*ecs.ClusterCapacityProviders, error) {
	association, err := ecs.NewClusterCapacityProviders(ctx, name+"-capacity-providers", &ecs.ClusterCapacityProvidersArgs{
		ClusterName: clusterName, CapacityProviders: providers,
		DefaultCapacityProviderStrategies: ecs.ClusterCapacityProvidersDefaultCapacityProviderStrategyArray{
			&ecs.ClusterCapacityProvidersDefaultCapacityProviderStrategyArgs{CapacityProvider: defaultProvider, Weight: pulumi.Int(1)},
		},
	}, pulumi.Parent(parent))
	if err != nil {
		return nil, fmt.Errorf("associate ECS capacity providers: %w", err)
	}
	return association, nil
}

// managedInstanceCPUMemory returns the vCPU count and memory (MiB) that AWS
// CreateCapacityProvider requires on instanceRequirements. AllowedInstanceTypes
// alone is not enough; the Pulumi AWS provider rejects a missing vcpu_count or
// memory_mib. Bounds are pinned to the selected type so the cell cannot pick a
// larger SKU.
func managedInstanceCPUMemory(instanceType string) (vcpu, memoryMiB int) {
	switch strings.ToLower(strings.TrimSpace(instanceType)) {
	case "t3.medium", "c7i.large":
		return 2, 4096
	case "m6i.large", "":
		return 2, 8192
	default:
		return 2, 8192
	}
}

// ecsCapacityProviderName returns an ECS CreateCapacityProvider name.
// AWS rejects names prefixed with "aws", "ecs", or "fargate" (case-insensitive).
// https://docs.aws.amazon.com/AmazonECS/latest/APIReference/API_CreateCapacityProvider.html
func ecsCapacityProviderName(name, suffix string) string {
	base := strings.Trim(strings.TrimSpace(name)+"-"+strings.TrimSpace(suffix), "-")
	lower := strings.ToLower(base)
	if strings.HasPrefix(lower, "aws") || strings.HasPrefix(lower, "ecs") || strings.HasPrefix(lower, "fargate") {
		base = "ml-" + base
	}
	if len(base) > 255 {
		base = base[:255]
	}
	return base
}

func hostCapacity(args Args) (minSize, desiredSize, maxSize int) {
	minSize = args.MinCapacity
	if minSize == 0 {
		minSize = 1
	}
	desiredSize = args.DesiredCount
	if desiredSize < minSize {
		desiredSize = minSize
	}
	maxSize = args.MaxCapacity
	if maxSize == 0 {
		maxSize = desiredSize
		if maxSize < 2 {
			maxSize = 2
		}
	}
	if maxSize < desiredSize {
		maxSize = desiredSize
	}
	return minSize, desiredSize, maxSize
}

const ec2AssumeRolePolicy = `{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"Service":"ec2.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

const ecsInfrastructureAssumeRolePolicy = `{"Version":"2012-10-17","Statement":[{"Sid":"AllowAccessToECSForInfrastructureManagement","Effect":"Allow","Principal":{"Service":"ecs.amazonaws.com"},"Action":"sts:AssumeRole"}]}`

func passRolePolicy(roleARN pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.All(roleARN).ApplyT(func(values []interface{}) string {
		return passRolePolicyJSON(values[0].(string))
	}).(pulumi.StringOutput)
}

func passRolePolicyJSON(roleARN string) string {
	document := map[string]any{
		"Version": "2012-10-17",
		"Statement": []map[string]any{{
			"Effect": "Allow", "Action": "iam:PassRole", "Resource": roleARN,
			"Condition": map[string]any{"StringLike": map[string]string{"iam:PassedToService": "ec2.*"}},
		}},
	}
	encoded, _ := json.Marshal(document)
	return string(encoded)
}

func ecsHostUserData(clusterName pulumi.StringInput) pulumi.StringOutput {
	return clusterName.ToStringOutput().ApplyT(func(name string) string {
		content := fmt.Sprintf("#!/bin/bash\nset -eu\ncat >/etc/ecs/ecs.config <<'EOF'\nECS_CLUSTER=%s\nECS_ENABLE_CONTAINER_METADATA=true\nEOF\n", name)
		return base64.StdEncoding.EncodeToString([]byte(content))
	}).(pulumi.StringOutput)
}
