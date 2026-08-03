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
	Project          string
	Region           string
	NetworkID        pulumi.StringInput
	PrivateSubnetIDs pulumi.StringArrayInput
	NodeType         string
	ReplicaCount     int
	Labels           map[string]string
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
			Limit:       pulumi.String("2"),
		},
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create Memorystore service connection policy: %w", err)
	}
	instance, err := memorystore.NewInstance(ctx, name, &memorystore.InstanceArgs{
		Project:                   pulumi.String(args.Project),
		InstanceId:                pulumi.String(instanceID),
		Location:                  pulumi.String(args.Region),
		ShardCount:                pulumi.Int(1),
		ReplicaCount:              pulumi.Int(args.ReplicaCount),
		NodeType:                  pulumi.String(args.NodeType),
		Mode:                      pulumi.String("CLUSTER_DISABLED"),
		EngineVersion:             pulumi.String("VALKEY_8_0"),
		AuthorizationMode:         pulumi.String("AUTH_DISABLED"),
		TransitEncryptionMode:     pulumi.String("TRANSIT_ENCRYPTION_DISABLED"),
		DeletionProtectionEnabled: pulumi.Bool(false),
		DesiredAutoCreatedEndpoints: memorystore.InstanceDesiredAutoCreatedEndpointArray{
			&memorystore.InstanceDesiredAutoCreatedEndpointArgs{
				Network:   args.NetworkID,
				ProjectId: pulumi.String(args.Project),
			},
		},
		Labels: pulumi.ToStringMap(args.Labels),
	}, parent, pulumi.DependsOn([]pulumi.Resource{scp}))
	if err != nil {
		return nil, fmt.Errorf("create Memorystore Valkey instance: %w", err)
	}

	component.PrimaryEndpoint = instance.Endpoints.ApplyT(func(endpoints []memorystore.InstanceEndpoint) string {
		for _, endpoint := range endpoints {
			for _, connection := range endpoint.Connections {
				if connection.PscAutoConnection != nil && connection.PscAutoConnection.IpAddress != nil {
					return *connection.PscAutoConnection.IpAddress
				}
			}
		}
		return ""
	}).(pulumi.StringOutput)
	component.InstanceID = instance.InstanceId
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"primaryEndpoint": component.PrimaryEndpoint, "instanceId": component.InstanceID,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
