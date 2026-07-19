package stack

import (
	"errors"
	"fmt"

	"github.com/acourtiol/magelift/internal/cloud/gcp/cache"
	"github.com/acourtiol/magelift/internal/cloud/gcp/database"
	"github.com/acourtiol/magelift/internal/cloud/gcp/naming"
	"github.com/acourtiol/magelift/internal/cloud/gcp/network"
	"github.com/acourtiol/magelift/internal/cloud/gcp/runtime"
	"github.com/acourtiol/magelift/internal/platform"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Component struct {
	pulumi.ResourceState
	Network  *network.Component
	Database *database.Component
	Cache    *cache.Component
	Runtime  *runtime.Component
}

func New(ctx *pulumi.Context, name string, spec Spec, provider *gcp.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := spec.Validate(); err != nil {
		return nil, fmt.Errorf("validate GCP stack plan: %w", err)
	}
	if name == "" {
		return nil, errors.New("GCP stack name is required")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2("magelift:gcp:Stack", name, pulumi.Map{
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
		Preset: spec.Identity.Preset, Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkCIDR: spec.Policy.NetworkCIDR, Zones: spec.Policy.Zones, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create GCP network: %w", err)
	}
	component.Database, err = database.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "sql"), database.Args{
		Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID.ToStringOutput(), NetworkSelfLink: component.Network.NetworkSelfLink,
		DatabaseName: spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		Tier: spec.Catalog.CloudSQLTier, Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP database: %w", err)
	}
	component.Cache, err = cache.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "valkey"), cache.Args{
		Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID.ToStringOutput(), PrivateSubnetIDs: component.Network.PrivateSubnetIDs,
		NodeType: spec.Catalog.MemorystoreNodeType, Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP cache: %w", err)
	}
	component.Runtime, err = runtime.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "app"), runtime.Args{
		Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkSelfLink: component.Network.NetworkSelfLink, PrivateSubnetNames: component.Network.PrivateSubnetNames,
		Image: spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, WebRuntime: spec.Application.WebRuntime,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		CacheEndpoint: component.Cache.PrimaryEndpoint, SessionEndpoint: component.Cache.PrimaryEndpoint,
		EncryptionKeySecret: spec.Dependencies.EncryptionKeySecret,
		CPURequest:          spec.Catalog.AutopilotCPURequest, MemoryRequest: spec.Catalog.AutopilotMemoryRequest,
		DesiredWebReplicas: spec.Catalog.DesiredWebReplicas, QueueConsumerCount: spec.Catalog.QueueConsumerCount,
		Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database, component.Cache}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP runtime: %w", err)
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
