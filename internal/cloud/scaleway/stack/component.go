package stack

import (
	"errors"
	"fmt"

	"github.com/acourtiol/magelift/internal/cloud/scaleway/cache"
	"github.com/acourtiol/magelift/internal/cloud/scaleway/database"
	"github.com/acourtiol/magelift/internal/cloud/scaleway/naming"
	"github.com/acourtiol/magelift/internal/cloud/scaleway/network"
	"github.com/acourtiol/magelift/internal/cloud/scaleway/runtime"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-scaleway/sdk/go/scaleway"
)

type Component struct {
	pulumi.ResourceState
	Network  *network.Component
	Database *database.Component
	Cache    *cache.Component
	Runtime  *runtime.Component
}

func New(ctx *pulumi.Context, name string, spec Spec, provider *scaleway.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("validate Scaleway stack plan: %w", err)
	}
	if name == "" {
		return nil, errors.New("Scaleway stack name is required")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2("magelift:scaleway:Stack", name, pulumi.Map{
		"project": pulumi.String(spec.Identity.Project), "environment": pulumi.String(spec.Identity.Environment),
		"region": pulumi.String(spec.Identity.Region), "preset": pulumi.String(string(spec.Identity.Preset)),
	}, component, opts...); err != nil {
		return nil, err
	}
	childOpts := []pulumi.ResourceOption{pulumi.Parent(component)}
	if provider != nil {
		childOpts = append(childOpts, pulumi.Provider(provider))
	}

	var err error
	component.Network, err = network.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "net"), network.Args{
		Preset: spec.Identity.Preset, ProjectID: spec.Identity.ScalewayProject, Region: spec.Identity.Region,
		NetworkCIDR: spec.Policy.NetworkCIDR, Zones: spec.Policy.Zones, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway network: %w", err)
	}
	component.Database, err = database.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "rdb"), database.Args{
		ProjectID: spec.Identity.ScalewayProject, Region: spec.Identity.Region,
		PrivateNetworkID: component.Network.PrivateNetworkID,
		DatabaseName:     spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		NodeType: spec.Catalog.DatabaseNodeType, Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway database: %w", err)
	}
	component.Cache, err = cache.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "redis"), cache.Args{
		ProjectID: spec.Identity.ScalewayProject, Zone: spec.Identity.Zone,
		PrivateNetworkID: component.Network.PrivateNetworkID,
		NodeType:         spec.Catalog.RedisNodeType, Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway cache: %w", err)
	}
	component.Runtime, err = runtime.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "app"), runtime.Args{
		ProjectID: spec.Identity.ScalewayProject, MagentoProject: spec.Identity.Project, Environment: spec.Identity.Environment,
		Region:           spec.Identity.Region,
		PrivateNetworkID: component.Network.PrivateNetworkID,
		Image:            spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, WebRuntime: spec.Application.WebRuntime,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		CacheEndpoint: component.Cache.PrimaryEndpoint, SessionEndpoint: component.Cache.PrimaryEndpoint,
		// EncryptionKeySecret is plumbed for day-2 secret injection; CoreEnvBindings does not
		// emit MAGENTO_DC_CRYPT__KEY yet (shared K8s ceiling with GCP — wire via SecretKeyRef).
		EncryptionKeySecret: spec.Dependencies.EncryptionKeySecret,
		KapsuleVersion:      spec.Catalog.KapsuleVersion, NodeType: spec.Catalog.NodeType, NodeCount: spec.Catalog.NodeCount,
		CPURequest: spec.Catalog.CPURequest, MemoryRequest: spec.Catalog.MemoryRequest,
		DesiredWebReplicas: spec.Catalog.DesiredWebReplicas, QueueConsumerCount: spec.Catalog.QueueConsumerCount,
		Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database, component.Cache}))...)
	if err != nil {
		return nil, fmt.Errorf("create Scaleway runtime: %w", err)
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
		platform.OutputCacheEndpoint:    c.Cache.PrimaryEndpoint,
		platform.OutputNetworkVpcID:     c.Network.NetworkID,
		platform.OutputClusterName:      c.Runtime.ClusterName,
		platform.OutputServiceName:      c.Runtime.ServiceName,
		platform.OutputPrivateSubnetIDs: c.Network.PrivateSubnetIDs,
	}
}
