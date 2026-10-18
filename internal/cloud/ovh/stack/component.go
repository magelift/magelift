package stack

import (
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/cloud/ovh/cache"
	"github.com/magelift/magelift/internal/cloud/ovh/database"
	"github.com/magelift/magelift/internal/cloud/ovh/naming"
	"github.com/magelift/magelift/internal/cloud/ovh/network"
	"github.com/magelift/magelift/internal/cloud/ovh/observability"
	"github.com/magelift/magelift/internal/cloud/ovh/runtime"
	"github.com/magelift/magelift/internal/platform"
	"github.com/ovh/pulumi-ovh/sdk/v2/go/ovh"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Component struct {
	pulumi.ResourceState
	Network       *network.Component
	Database      *database.Component
	Cache         *cache.Component
	Runtime       *runtime.Component
	Observability *observability.Component
}

func New(ctx *pulumi.Context, name string, spec Spec, provider *ovh.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	validate := spec.Validate
	if spec.AllowExpiredPreview {
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return nil, fmt.Errorf("validate OVH stack plan: %w", err)
	}
	if name == "" {
		return nil, errors.New("OVH stack name is required")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2("magelift:ovh:Stack", name, pulumi.Map{
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
		Preset: spec.Identity.Preset, ServiceName: spec.Identity.ServiceName, Region: spec.Identity.Region,
		NetworkCIDR: spec.Policy.NetworkCIDR, Zones: spec.Policy.Zones, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create OVH network: %w", err)
	}

	firstSubnet := component.Network.PrivateSubnetIDs.Index(pulumi.Int(0))
	component.Database, err = database.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "sql"), database.Args{
		ServiceName: spec.Identity.ServiceName, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID, SubnetID: firstSubnet,
		DatabaseName: spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		Flavor: spec.Catalog.DatabaseFlavor, Plan: spec.Catalog.DatabasePlan, Version: spec.Catalog.DatabaseVersion, NodeCount: spec.Catalog.DatabaseNodeCount,
		BackupTime: spec.Catalog.DatabaseBackupTime, BackupRegions: spec.Catalog.DatabaseBackupRegions, DeletionProtection: spec.Catalog.DatabaseDeletionProtection,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create OVH database: %w", err)
	}
	component.Cache, err = cache.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "valkey"), cache.Args{
		ServiceName: spec.Identity.ServiceName, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID, SubnetID: firstSubnet,
		Flavor: spec.Catalog.ValkeyFlavor, Plan: spec.Catalog.ValkeyPlan, Version: spec.Catalog.ValkeyVersion, NodeCount: spec.Catalog.ValkeyNodeCount,
		BackupTime: spec.Catalog.ValkeyBackupTime, BackupRegions: spec.Catalog.ValkeyBackupRegions, DeletionProtection: spec.Catalog.ValkeyDeletionProtection,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create OVH cache: %w", err)
	}
	component.Runtime, err = runtime.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "app"), runtime.Args{
		ServiceName: spec.Identity.ServiceName, Region: spec.Identity.Region,
		ProjectName: spec.Identity.Project, Environment: spec.Identity.Environment,
		NetworkID: component.Network.NetworkID, SubnetID: firstSubnet,
		AttachFloatingIPs:              spec.Catalog.AttachFloatingIPs,
		PrivateNetworkRoutingAsDefault: spec.Catalog.PrivateNetworkRoutingAsDefault,
		Image:                          spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, ApplicationVersion: spec.Application.Version, WebRuntime: spec.Application.WebRuntime, Magento: spec.Application.Magento,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		DatabaseUsername: spec.Dependencies.MasterUsername, DatabasePassword: component.Database.Password,
		CacheEndpoint: component.Cache.PrimaryEndpoint, SessionEndpoint: component.Cache.PrimaryEndpoint,
		EncryptionKeySecret: spec.Dependencies.EncryptionKeySecret,
		CPURequest:          spec.Catalog.CPURequest, MemoryRequest: spec.Catalog.MemoryRequest,
		DesiredWebReplicas: spec.Catalog.DesiredWebReplicas, QueueConsumerCount: spec.Catalog.QueueConsumerCount,
		MKSPlan: spec.Catalog.MKSPlan, NodeFlavor: spec.Catalog.NodeFlavor, NodeCount: spec.Catalog.NodeCount,
		AvailabilityZones: spec.Policy.Zones,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database, component.Cache}))...)
	if err != nil {
		return nil, fmt.Errorf("create OVH runtime: %w", err)
	}
	if spec.Observability.NativeProvider == "ovh-logs-data-platform" {
		component.Observability, err = observability.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "observability"), observability.Args{
			ServiceName: spec.Identity.ServiceName, ClusterID: component.Runtime.ClusterIdentifier, Intent: spec.Observability,
		}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Runtime}))...)
		if err != nil {
			return nil, fmt.Errorf("create OVH observability: %w", err)
		}
	}
	if err := ctx.RegisterResourceOutputs(component, component.Outputs()); err != nil {
		return nil, err
	}
	return component, nil
}

func (c *Component) Outputs() pulumi.Map {
	outputs := pulumi.Map{
		platform.OutputApplicationURL:          c.Runtime.ApplicationURL,
		platform.OutputDatabaseWriter:          c.Database.WriterEndpoint,
		platform.OutputCacheEndpoint:           c.Cache.PrimaryEndpoint,
		platform.OutputNetworkVpcID:            c.Network.NetworkID,
		platform.OutputClusterName:             c.Runtime.ClusterName,
		platform.OutputServiceName:             c.Runtime.ServiceName,
		platform.OutputPrivateSubnetIDs:        c.Network.PrivateSubnetIDs,
		platform.OutputKubeconfig:              c.Runtime.Kubeconfig,
		platform.OutputDatabaseSecretName:      c.Runtime.DatabaseSecretName,
		platform.OutputEncryptionKeySecretName: c.Runtime.EncryptionKeySecretName,
	}
	if c.Observability != nil {
		outputs["observabilityAuditSubscriptionId"] = c.Observability.AuditSubscriptionID
		outputs["observabilityUnavailableSignals"] = c.Observability.UnavailableSignals
		outputs["observabilityUnavailableOperations"] = c.Observability.UnavailableOperations
	}
	return outputs
}
