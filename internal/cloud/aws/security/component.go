package security

import (
	"errors"
	"sort"
	"strings"

	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/ec2"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/vpc"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:SecurityGroups"

type Args struct {
	Region        string
	VPCID         pulumi.StringInput
	WebTargetPort int
	Tags          map[string]string
}

type Component struct {
	pulumi.ResourceState
	EdgeSecurityGroupID   pulumi.StringOutput
	WebSecurityGroupID    pulumi.StringOutput
	DataSecurityGroupID   pulumi.StringOutput
	CacheSecurityGroupID  pulumi.StringOutput
	SearchSecurityGroupID pulumi.StringOutput
	QueueSecurityGroupID  pulumi.StringOutput
}

type groupSet struct {
	edge, web, data, cache, search, queue *ec2.SecurityGroup
}

type rule struct {
	name        string
	description string
	port        int
	from        *ec2.SecurityGroup
	to          *ec2.SecurityGroup
	cidr        string
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"region": pulumi.String(args.Region), "vpcId": args.VPCID, "webTargetPort": pulumi.Int(targetPort(args)),
	}, component, opts...); err != nil {
		return nil, err
	}

	groups, err := createGroups(ctx, name, args, component)
	if err != nil {
		return nil, err
	}
	if err := createRules(ctx, name, args, groups, component); err != nil {
		return nil, err
	}

	component.EdgeSecurityGroupID = groups.edge.ID().ToStringOutput()
	component.WebSecurityGroupID = groups.web.ID().ToStringOutput()
	component.DataSecurityGroupID = groups.data.ID().ToStringOutput()
	component.CacheSecurityGroupID = groups.cache.ID().ToStringOutput()
	component.SearchSecurityGroupID = groups.search.ID().ToStringOutput()
	component.QueueSecurityGroupID = groups.queue.ID().ToStringOutput()
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"edgeSecurityGroupId": component.EdgeSecurityGroupID, "webSecurityGroupId": component.WebSecurityGroupID,
		"dataSecurityGroupId": component.DataSecurityGroupID, "cacheSecurityGroupId": component.CacheSecurityGroupID,
		"searchSecurityGroupId": component.SearchSecurityGroupID, "queueSecurityGroupId": component.QueueSecurityGroupID,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func createGroups(ctx *pulumi.Context, name string, args Args, parent pulumi.Resource) (groupSet, error) {
	groups := groupSet{}
	targets := []struct {
		role        string
		description string
		result      **ec2.SecurityGroup
	}{
		{"edge", "Public application load balancer", &groups.edge},
		{"web", "Magento web tasks", &groups.web},
		{"data", "Magento database", &groups.data},
		{"cache", "Magento cache and sessions", &groups.cache},
		{"search", "Magento search", &groups.search},
		{"queue", "Magento message queue", &groups.queue},
	}
	for _, target := range targets {
		group, err := ec2.NewSecurityGroup(ctx, name+"-"+target.role, &ec2.SecurityGroupArgs{
			Name: pulumi.String(name + "-" + target.role), Description: pulumi.String(target.description), VpcId: args.VPCID,
			Ingress: ec2.SecurityGroupIngressArray{}, Egress: ec2.SecurityGroupEgressArray{}, Region: pulumi.String(args.Region),
			Tags: tags(args.Tags, name, target.role),
		}, pulumi.Parent(parent))
		if err != nil {
			return groupSet{}, err
		}
		*target.result = group
	}
	return groups, nil
}

func createRules(ctx *pulumi.Context, name string, args Args, groups groupSet, parent pulumi.Resource) error {
	ingress := []rule{
		{name: "edge-https", description: "Public HTTPS to the load balancer", port: 443, to: groups.edge, cidr: "0.0.0.0/0"},
		{name: "web-target", description: "Load balancer traffic to web tasks", port: targetPort(args), from: groups.edge, to: groups.web},
		{name: "data-mysql", description: "Web tasks to Aurora MySQL", port: 3306, from: groups.web, to: groups.data},
		{name: "cache-valkey", description: "Web tasks to Valkey", port: 6379, from: groups.web, to: groups.cache},
		{name: "search-https", description: "Web tasks to OpenSearch", port: 443, from: groups.web, to: groups.search},
		{name: "queue-amqps", description: "Web tasks to RabbitMQ over TLS", port: 5671, from: groups.web, to: groups.queue},
	}
	for _, spec := range ingress {
		ruleArgs := &vpc.SecurityGroupIngressRuleArgs{
			SecurityGroupId: spec.to.ID(), Description: pulumi.String(spec.description), IpProtocol: pulumi.String("tcp"),
			FromPort: pulumi.Int(spec.port), ToPort: pulumi.Int(spec.port), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, spec.name),
		}
		if spec.from != nil {
			ruleArgs.ReferencedSecurityGroupId = spec.from.ID()
		} else {
			ruleArgs.CidrIpv4 = pulumi.String(spec.cidr)
		}
		if _, err := vpc.NewSecurityGroupIngressRule(ctx, name+"-ingress-"+spec.name, ruleArgs, pulumi.Parent(parent)); err != nil {
			return err
		}
	}

	egress := []rule{
		{name: "edge-target", description: "Load balancer traffic to web tasks", port: targetPort(args), from: groups.edge, to: groups.web},
		{name: "web-data", description: "Web tasks to Aurora MySQL", port: 3306, from: groups.web, to: groups.data},
		{name: "web-cache", description: "Web tasks to Valkey", port: 6379, from: groups.web, to: groups.cache},
		{name: "web-search", description: "Web tasks to OpenSearch", port: 443, from: groups.web, to: groups.search},
		{name: "web-queue", description: "Web tasks to RabbitMQ over TLS", port: 5671, from: groups.web, to: groups.queue},
		{name: "web-https", description: "Web tasks to HTTPS services", port: 443, from: groups.web, cidr: "0.0.0.0/0"},
	}
	for _, spec := range egress {
		ruleArgs := &vpc.SecurityGroupEgressRuleArgs{
			SecurityGroupId: spec.from.ID(), Description: pulumi.String(spec.description), IpProtocol: pulumi.String("tcp"),
			FromPort: pulumi.Int(spec.port), ToPort: pulumi.Int(spec.port), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, spec.name),
		}
		if spec.to != nil {
			ruleArgs.ReferencedSecurityGroupId = spec.to.ID()
		} else {
			ruleArgs.CidrIpv4 = pulumi.String(spec.cidr)
		}
		if _, err := vpc.NewSecurityGroupEgressRule(ctx, name+"-egress-"+spec.name, ruleArgs, pulumi.Parent(parent)); err != nil {
			return err
		}
	}
	return nil
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("security component name is required")
	}
	if strings.TrimSpace(args.Region) == "" || args.VPCID == nil {
		return errors.New("security groups require an AWS region and VPC")
	}
	if args.WebTargetPort < 0 || args.WebTargetPort > 65535 {
		return errors.New("web target port is invalid")
	}
	return nil
}

func targetPort(args Args) int {
	if args.WebTargetPort == 0 {
		return 8080
	}
	return args.WebTargetPort
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
