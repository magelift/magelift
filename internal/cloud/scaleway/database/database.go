package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway/databases"
)

const TypeToken = "magelift:scaleway:Database"

type Args struct {
	ProjectID        string
	Region           string
	PrivateNetworkID pulumi.StringInput
	DatabaseName     string
	MasterUsername   string
	NodeType         string
	Labels           map[string]string
}

type Component struct {
	pulumi.ResourceState
	WriterEndpoint pulumi.StringOutput
	DatabaseName   pulumi.StringOutput
	InstanceName   pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("database name is required")
	}
	if strings.TrimSpace(args.ProjectID) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("Scaleway project ID and region are required")
	}
	if strings.TrimSpace(args.DatabaseName) == "" || strings.TrimSpace(args.MasterUsername) == "" {
		return nil, errors.New("database name and master username are required")
	}
	if strings.TrimSpace(args.NodeType) == "" {
		args.NodeType = "DB-DEV-S"
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"projectId": pulumi.String(args.ProjectID), "region": pulumi.String(args.Region),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	tags := make(pulumi.StringArray, 0, len(args.Labels))
	for key, value := range args.Labels {
		tags = append(tags, pulumi.String(key+"="+value))
	}

	password, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{
		Length:          pulumi.Int(32),
		Special:         pulumi.Bool(false),
		OverrideSpecial: pulumi.String(""),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate database password: %w", err)
	}

	instance, err := databases.NewInstance(ctx, name, &databases.InstanceArgs{
		Name:      pulumi.String(name),
		NodeType:  pulumi.String(args.NodeType),
		Engine:    pulumi.String("MySQL-8"),
		Region:    pulumi.String(args.Region),
		ProjectId: pulumi.String(args.ProjectID),
		UserName:  pulumi.String(args.MasterUsername),
		Password:  password.Result,
		PrivateNetwork: &databases.InstancePrivateNetworkArgs{
			PnId: args.PrivateNetworkID,
		},
		Tags: tags,
	}, parent, pulumi.DependsOn([]pulumi.Resource{password}))
	if err != nil {
		return nil, fmt.Errorf("create Scaleway Database Instance: %w", err)
	}
	_, err = databases.NewDatabase(ctx, name+"-db", &databases.DatabaseArgs{
		InstanceId: instance.ID(),
		Name:       pulumi.String(args.DatabaseName),
	}, parent, pulumi.DependsOn([]pulumi.Resource{instance}))
	if err != nil {
		return nil, fmt.Errorf("create Magento database: %w", err)
	}

	// Prefer the managed Load Balancer front (private network attachment);
	// fall back to the deprecated direct endpointIp for older/mock states.
	component.WriterEndpoint = pulumi.All(instance.LoadBalancer, instance.EndpointIp).ApplyT(func(values []interface{}) string {
		if lb, ok := values[0].(databases.InstanceLoadBalancer); ok && lb.Ip != nil && *lb.Ip != "" {
			return *lb.Ip
		}
		return values[1].(string)
	}).(pulumi.StringOutput)
	component.DatabaseName = pulumi.String(args.DatabaseName).ToStringOutput()
	component.InstanceName = instance.Name
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"writerEndpoint": component.WriterEndpoint, "databaseName": component.DatabaseName,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
