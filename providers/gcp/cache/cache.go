package cache

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/memorystore"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/networkconnectivity"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// memorystoreServiceClass is the producer service class for PSC automation to
// Memorystore for Valkey. See https://cloud.google.com/memorystore/docs/valkey/instance-provisioning-vpc
const memorystoreServiceClass = "gcp-memorystore"

const TypeToken = "magelift:gcp:MemorystoreValkey"

type Args struct {
	Project              string
	Region               string
	NetworkID            pulumi.StringInput
	PrivateSubnetIDs     pulumi.StringArrayInput
	NodeType             string
	ShardCount           int
	EngineVersion        string
	ReplicaCount         int
	Mode                 string
	ZoneDistributionMode string
	Zone                 string
	DeletionProtection   bool
	PSCConnectionLimit   int
	Labels               map[string]string
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
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	if strings.TrimSpace(args.NodeType) == "" {
		args.NodeType = "SHARED_CORE_NANO"
	}
	if strings.TrimSpace(args.EngineVersion) == "" {
		return nil, errors.New("Memorystore Valkey engine version is required")
	}
	if args.ShardCount == 0 {
		args.ShardCount = 1
	}
	if args.ShardCount < 1 {
		return nil, errors.New("Memorystore shard count must be at least 1")
	}
	if args.ReplicaCount < 0 || args.ReplicaCount > 5 {
		return nil, errors.New("Memorystore replica count must be between 0 and 5")
	}
	if strings.TrimSpace(args.Mode) == "" {
		args.Mode = "CLUSTER_DISABLED"
	}
	if args.Mode != "CLUSTER" && args.Mode != "CLUSTER_DISABLED" {
		return nil, fmt.Errorf("invalid Memorystore mode %q", args.Mode)
	}
	if args.Mode == "CLUSTER_DISABLED" && args.ShardCount != 1 {
		return nil, errors.New("Cluster Mode Disabled Memorystore instances support only one shard")
	}
	if strings.TrimSpace(args.ZoneDistributionMode) == "" {
		args.ZoneDistributionMode = "MULTI_ZONE"
	}
	if args.ZoneDistributionMode != "MULTI_ZONE" && args.ZoneDistributionMode != "SINGLE_ZONE" {
		return nil, fmt.Errorf("invalid Memorystore zone distribution mode %q", args.ZoneDistributionMode)
	}
	if args.ZoneDistributionMode == "SINGLE_ZONE" && strings.TrimSpace(args.Zone) == "" {
		return nil, errors.New("single-zone Memorystore placement requires a zone")
	}
	if args.PSCConnectionLimit < 0 {
		return nil, errors.New("Memorystore PSC connection limit cannot be negative")
	}
	if args.PSCConnectionLimit == 0 {
		args.PSCConnectionLimit = 2
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	instanceID := strings.Trim(strings.ToLower(name), "-")
	if len(instanceID) < 4 {
		instanceID = instanceID + "-valkey"
	}
	// DesiredAutoCreatedEndpoints requires a regional Service Connection Policy
	// for gcp-memorystore on the consumer VPC before the instance can be created.
	scp, err := networkconnectivity.NewServiceConnectionPolicy(ctx, name+"-scp", &networkconnectivity.ServiceConnectionPolicyArgs{
		Name:         pulumi.String(name + "-memorystore"),
		Location:     pulumi.String(args.Region),
		ServiceClass: pulumi.String(memorystoreServiceClass),
		Description:  pulumi.String("MageLift Memorystore for Valkey PSC automation"),
		Network:      args.NetworkID,
		PscConfig: &networkconnectivity.ServiceConnectionPolicyPscConfigArgs{
			Subnetworks: args.PrivateSubnetIDs,
			Limit:       pulumi.String(fmt.Sprintf("%d", args.PSCConnectionLimit)),
		},
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Memorystore service connection policy: %w", err)
	}
	instance, err := memorystore.NewInstance(ctx, name, &memorystore.InstanceArgs{
		Project:                   pulumi.String(args.Project),
		InstanceId:                pulumi.String(instanceID),
		Location:                  pulumi.String(args.Region),
		ShardCount:                pulumi.Int(args.ShardCount),
		ReplicaCount:              pulumi.Int(args.ReplicaCount),
		NodeType:                  pulumi.String(args.NodeType),
		Mode:                      pulumi.String(args.Mode),
		EngineVersion:             pulumi.String(args.EngineVersion),
		AuthorizationMode:         pulumi.String("AUTH_DISABLED"),
		TransitEncryptionMode:     pulumi.String("TRANSIT_ENCRYPTION_DISABLED"),
		DeletionProtectionEnabled: pulumi.Bool(args.DeletionProtection),
		DesiredAutoCreatedEndpoints: memorystore.InstanceDesiredAutoCreatedEndpointArray{
			&memorystore.InstanceDesiredAutoCreatedEndpointArgs{
				Network:   args.NetworkID,
				ProjectId: pulumi.String(args.Project),
			},
		},
		ZoneDistributionConfig: &memorystore.InstanceZoneDistributionConfigArgs{
			Mode: pulumi.String(args.ZoneDistributionMode),
			Zone: pulumi.String(args.Zone),
		},
		Labels: pulumi.ToStringMap(args.Labels),
	}, parent, pulumi.DependsOn([]pulumi.Resource{scp}))
	if err != nil {
		return nil, fmt.Errorf("create Memorystore Valkey instance: %w", err)
	}

	component.PrimaryEndpoint = instance.Endpoints.ApplyT(func(endpoints []memorystore.InstanceEndpoint) string {
		preferredConnectionType := "CONNECTION_TYPE_PRIMARY"
		if args.Mode == "CLUSTER" {
			preferredConnectionType = "CONNECTION_TYPE_DISCOVERY"
		}
		var fallback string
		for _, endpoint := range endpoints {
			for _, connection := range endpoint.Connections {
				if connection.PscAutoConnection != nil && connection.PscAutoConnection.IpAddress != nil {
					if fallback == "" {
						fallback = *connection.PscAutoConnection.IpAddress
					}
					if connection.PscAutoConnection.ConnectionType != nil && *connection.PscAutoConnection.ConnectionType == preferredConnectionType {
						return *connection.PscAutoConnection.IpAddress
					}
				}
			}
		}
		return fallback
	}).(pulumi.StringOutput)
	component.InstanceID = instance.InstanceId
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"primaryEndpoint": component.PrimaryEndpoint, "instanceId": component.InstanceID,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
