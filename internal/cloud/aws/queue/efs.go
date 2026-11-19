package queue

import (
	"fmt"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/efs"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// brokerStorage provisions one regional filesystem with a mount target in each
// configured private subnet. Those subnets must be in distinct AZs.
func brokerStorage(ctx *pulumi.Context, name string, args Args, opts []pulumi.ResourceOption) (*efs.FileSystem, []pulumi.Resource, error) {
	filesystem, err := efs.NewFileSystem(ctx, name+"-broker-data", &efs.FileSystemArgs{Encrypted: pulumi.Bool(true), Region: pulumi.String(args.Region), Tags: pulumi.ToStringMap(tags(args.Tags, name))}, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create encrypted broker filesystem: %w", err)
	}
	group, err := ec2.NewSecurityGroup(ctx, name+"-broker-efs", &ec2.SecurityGroupArgs{VpcId: args.VpcID, Description: pulumi.String("Private NFS access for Magento RabbitMQ"), Region: pulumi.String(args.Region), Tags: pulumi.ToStringMap(tags(args.Tags, name))}, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("create broker filesystem security group: %w", err)
	}
	groups := securityGroupInputs(args).ToStringArrayOutput()
	count := len(args.SecurityGroupIDs)
	if args.SecurityGroupIDInputs != nil {
		count = args.SecurityGroupCount
	}
	deps := []pulumi.Resource{}
	for i := 0; i < count; i++ {
		brokerGroup := groups.Index(pulumi.Int(i))
		for _, rule := range []struct {
			name, kind string
			subject    pulumi.StringInput
			peer       pulumi.StringPtrInput
		}{{"ingress", "ingress", group.ID().ToStringOutput(), brokerGroup.ToStringPtrOutput()}, {"egress", "egress", brokerGroup, group.ID().ToStringOutput().ToStringPtrOutput()}} {
			r, e := ec2.NewSecurityGroupRule(ctx, fmt.Sprintf("%s-broker-efs-%s-%d", name, rule.name, i), &ec2.SecurityGroupRuleArgs{Type: pulumi.String(rule.kind), SecurityGroupId: rule.subject, SourceSecurityGroupId: rule.peer, FromPort: pulumi.Int(2049), ToPort: pulumi.Int(2049), Protocol: pulumi.String("tcp"), Region: pulumi.String(args.Region)}, opts...)
			if e != nil {
				return nil, nil, fmt.Errorf("create broker NFS security rule: %w", e)
			}
			deps = append(deps, r)
		}
	}
	subnets := subnetInputs(args).ToStringArrayOutput()
	count = len(args.SubnetIDs)
	if args.SubnetIDInputs != nil {
		count = args.PrivateSubnetCount
		if count == 0 {
			count = args.SubnetCount
		}
	}
	for i := 0; i < count; i++ {
		mount, e := efs.NewMountTarget(ctx, fmt.Sprintf("%s-broker-data-%d", name, i), &efs.MountTargetArgs{FileSystemId: filesystem.ID(), SubnetId: subnets.Index(pulumi.Int(i)), SecurityGroups: pulumi.StringArray{group.ID()}, Region: pulumi.String(args.Region)}, opts...)
		if e != nil {
			return nil, nil, fmt.Errorf("create broker filesystem mount target: %w", e)
		}
		deps = append(deps, mount)
	}
	return filesystem, deps, nil
}
