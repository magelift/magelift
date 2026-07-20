package eksops

import (
	"errors"
	"fmt"

	"github.com/acourtiol/magelift/internal/cloud/aws/cache"
	"github.com/acourtiol/magelift/internal/cloud/aws/database"
	"github.com/acourtiol/magelift/internal/cloud/aws/eks"
	"github.com/acourtiol/magelift/internal/cloud/aws/network"
	"github.com/acourtiol/magelift/internal/cloud/aws/security"
	"github.com/acourtiol/magelift/internal/platform"
	sdk "github.com/acourtiol/magelift/sdk/v1"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Component struct {
	pulumi.ResourceState
	Network  *network.Component
	Security *security.Component
	Database *database.Component
	Cache    *cache.Component
	Runtime  *eks.Component
}

func New(ctx *pulumi.Context, name string, spec Spec, provider *awsprovider.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("validate AWS EKS stack plan: %w", err)
	}
	if name == "" {
		return nil, errors.New("AWS EKS stack name is required")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2("magelift:aws:EKSStack", name, pulumi.Map{
		"project": pulumi.String(spec.Identity.Project), "environment": pulumi.String(spec.Identity.Environment),
		"region": pulumi.String(spec.Identity.Region), "preset": pulumi.String(string(spec.Identity.Preset)),
	}, component, opts...); err != nil {
		return nil, err
	}
	regional := []pulumi.ResourceOption{pulumi.Parent(component)}
	if provider != nil {
		regional = append(regional, pulumi.Provider(provider))
	}
	tags := spec.Identity.Tags

	var err error
	component.Network, err = network.New(ctx, name+"-network", network.Args{
		Preset: spec.Identity.Preset, Region: spec.Identity.Region, VPCCIDR: spec.Policy.VPCCIDR.String(),
		AvailabilityZones: spec.Policy.AvailabilityZones, NatMode: spec.Policy.NatMode,
		GatewayEndpoints: []network.GatewayEndpoint{{Service: network.GatewayEndpointS3, Rationale: network.ReduceNATCostAndExposure}},
		Tags:             tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS network: %w", err)
	}
	component.Security, err = security.New(ctx, name+"-security", security.Args{
		Region: spec.Identity.Region, VPCID: component.Network.VpcID.ToStringOutput(), WebTargetPort: eks.ApplicationPort, Tags: tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS security groups: %w", err)
	}

	dataSubnets := stringInputs(component.Network.DataSubnetIDs)
	privateSubnets := stringInputs(component.Network.PrivateSubnetIDs)
	databaseCount := 2
	if spec.Identity.Preset == sdk.PresetHighAvailability {
		databaseCount = 3
	}
	databaseSubnets, databaseZones := firstN(dataSubnets, spec.Policy.AvailabilityZones, databaseCount)

	engineVersion := spec.Catalog.AuroraMySQLVersion
	if spec.Catalog.DatabaseEngine == DatabaseEngineRDSMySQL {
		engineVersion = spec.Catalog.MySQLVersion
	}
	component.Database, err = database.New(ctx, name+"-database", database.Args{
		Preset: spec.Identity.Preset, EnvironmentClass: spec.Identity.EnvironmentClass, Region: spec.Identity.Region,
		AvailabilityZones: databaseZones, DataSubnetIDs: databaseSubnets,
		VpcSecurityGroupIDs: pulumi.StringArray{component.Security.DataSecurityGroupID},
		Engine:              spec.Catalog.DatabaseEngine, EngineVersion: engineVersion,
		DatabaseName: spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		KMSKeyARN: spec.Dependencies.KMSKeyARN, BackupRetentionDays: spec.Catalog.BackupDays,
		FinalSnapshotIdentifier: name + "-final", ProvisionedInstanceClass: spec.Catalog.InstanceClass,
		InstanceCount: spec.Catalog.InstanceCount, ServerlessV2: serverlessDatabase(spec), Tags: tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS database: %w", err)
	}

	component.Cache, err = cache.New(ctx, name+"-cache", cache.Args{
		Topology: cache.Topology(spec.Identity.Preset), Region: spec.Identity.Region, EngineVersion: spec.Catalog.ValkeyVersion,
		NodeType: spec.Catalog.ValkeyNodeType, ReplicaCount: spec.Catalog.ValkeyReplicaCount,
		SubnetIDInputs: dataSubnets, SubnetCount: len(dataSubnets), SecurityGroupInput: component.Security.CacheSecurityGroupID,
		KMSKeyARN:  spec.Dependencies.KMSKeyARN,
		AuthTokens: cache.AuthTokens{CacheSecretARN: spec.Dependencies.CacheSecretARN, SessionSecretARN: spec.Dependencies.SessionSecretARN},
		Provider:   provider, Tags: tags,
	})
	if err != nil {
		return nil, fmt.Errorf("create AWS Valkey cache: %w", err)
	}

	component.Runtime, err = eks.New(ctx, name+"-runtime", eks.Args{
		Region: spec.Identity.Region, PrivateSubnetIDs: privateSubnets,
		Image: spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, WebRuntime: spec.Application.WebRuntime,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		CacheEndpoint: component.Cache.CachePrimaryEndpoint, SessionEndpoint: component.Cache.SessionPrimaryEndpoint,
		CPURequest: spec.Catalog.CPURequest, MemoryRequest: spec.Catalog.MemoryRequest,
		DesiredWebReplicas: spec.Catalog.DesiredWebReplicas, QueueConsumerCount: spec.Catalog.QueueConsumerCount,
		Tags: tags,
	}, append(regional, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database, component.Cache}))...)
	if err != nil {
		return nil, fmt.Errorf("create AWS EKS runtime: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(component, component.Outputs()); err != nil {
		return nil, err
	}
	return component, nil
}

func (c *Component) Outputs() pulumi.Map {
	return pulumi.Map{
		platform.OutputApplicationURL:   c.Runtime.ApplicationURL,
		platform.OutputDatabaseWriter:   c.Database.WriterEndpoint,
		platform.OutputCacheEndpoint:    c.Cache.CachePrimaryEndpoint,
		platform.OutputNetworkVpcID:     c.Network.VpcID,
		platform.OutputClusterName:      c.Runtime.ClusterName,
		platform.OutputServiceName:      c.Runtime.ServiceName,
		platform.OutputPrivateSubnetIDs: stringInputs(c.Network.PrivateSubnetIDs),
	}
}

func stringInputs(ids []pulumi.IDOutput) pulumi.StringArray {
	result := make(pulumi.StringArray, 0, len(ids))
	for _, id := range ids {
		result = append(result, id.ToStringOutput())
	}
	return result
}

func firstN(subnets pulumi.StringArray, zones []string, count int) (pulumi.StringArray, []string) {
	if count > len(subnets) {
		count = len(subnets)
	}
	if count > len(zones) {
		count = len(zones)
	}
	return subnets[:count], zones[:count]
}

func serverlessDatabase(spec Spec) *database.ServerlessV2 {
	if spec.Catalog.DatabaseEngine != DatabaseEngineAuroraMySQL || spec.Identity.Preset != sdk.PresetPreview {
		return nil
	}
	return &database.ServerlessV2{
		MinimumACU: spec.Catalog.AuroraMinACU, MaximumACU: spec.Catalog.AuroraMaxACU,
		AutoPauseSeconds: spec.Catalog.AuroraAutoPause, EngineSupportsAutoPause: spec.Catalog.AuroraAutoPauseOK,
	}
}
