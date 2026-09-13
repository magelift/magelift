package cache

import (
	"errors"
	"fmt"
	"strings"

	"github.com/ovh/pulumi-ovh/sdk/v2/go/ovh/cloudproject"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:ovh:Cache"

type Args struct {
	ServiceName        string
	Region             string
	NetworkID          pulumi.StringInput
	SubnetID           pulumi.StringInput
	Flavor             string
	Plan               string
	Version            string
	NodeCount          int
	BackupTime         string
	BackupRegions      []string
	DeletionProtection *bool
}

type Component struct {
	pulumi.ResourceState
	PrimaryEndpoint pulumi.StringOutput
	InstanceID      pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("cache name is required")
	}
	if strings.TrimSpace(args.ServiceName) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("OVH service name and region are required")
	}
	if args.Flavor == "" {
		args.Flavor = "b3-8"
	}
	if args.Plan == "" {
		args.Plan = "essential"
	}
	if args.Version == "" {
		args.Version = "8.1"
	}
	if args.NodeCount == 0 {
		args.NodeCount = 1
	}
	if args.NodeCount < 1 {
		return nil, errors.New("OVH Valkey node count must be at least 1")
	}

	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"serviceName": pulumi.String(args.ServiceName),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	// Password is generated so Valkey user creation can attach later; engine still boots without it.
	if _, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{
		Length:  pulumi.Int(32),
		Special: pulumi.Bool(false),
	}, parent); err != nil {
		return nil, fmt.Errorf("generate Valkey password: %w", err)
	}

	nodes := make(cloudproject.DatabaseNodeArray, 0, args.NodeCount)
	for range args.NodeCount {
		node := &cloudproject.DatabaseNodeArgs{Region: pulumi.String(args.Region)}
		if args.NetworkID != nil && args.SubnetID != nil {
			node.NetworkId = args.NetworkID.ToStringOutput().ToStringPtrOutput()
			node.SubnetId = args.SubnetID.ToStringOutput().ToStringPtrOutput()
		}
		nodes = append(nodes, node)
	}

	instanceArgs := &cloudproject.DatabaseArgs{
		ServiceName: pulumi.String(args.ServiceName),
		Description: pulumi.String(name),
		Engine:      pulumi.String("valkey"),
		Version:     pulumi.String(args.Version),
		Plan:        pulumi.String(args.Plan),
		Flavor:      pulumi.String(args.Flavor),
		Nodes:       nodes,
	}
	if args.BackupTime != "" {
		instanceArgs.BackupTime = pulumi.StringPtr(args.BackupTime)
	}
	if len(args.BackupRegions) > 0 {
		instanceArgs.BackupRegions = pulumi.ToStringArray(args.BackupRegions)
	}
	if args.DeletionProtection != nil {
		instanceArgs.DeletionProtection = pulumi.BoolPtr(*args.DeletionProtection)
	}
	instance, err := cloudproject.NewDatabase(ctx, name, instanceArgs, parent)
	if err != nil {
		return nil, fmt.Errorf("create OVH Valkey cache: %w", err)
	}

	endpoint := instance.Endpoints.ApplyT(func(endpoints []cloudproject.DatabaseEndpoint) (string, error) {
		if len(endpoints) == 0 || endpoints[0].Domain == nil || *endpoints[0].Domain == "" {
			return "", errors.New("OVH Valkey endpoint domain is empty")
		}
		return *endpoints[0].Domain, nil
	}).(pulumi.StringOutput)

	component.PrimaryEndpoint = endpoint
	component.InstanceID = instance.ID().ToStringOutput()

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"primaryEndpoint": component.PrimaryEndpoint,
		"instanceId":      component.InstanceID,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
