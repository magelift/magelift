package stack

import (
	"errors"
	"fmt"

	"github.com/magelift/magelift/internal/cloud/gcp/cache"
	"github.com/magelift/magelift/internal/cloud/gcp/database"
	"github.com/magelift/magelift/internal/cloud/gcp/edge"
	"github.com/magelift/magelift/internal/cloud/gcp/naming"
	"github.com/magelift/magelift/internal/cloud/gcp/network"
	"github.com/magelift/magelift/internal/cloud/gcp/runtime"
	"github.com/magelift/magelift/internal/cloud/gcp/storage"
	"github.com/magelift/magelift/internal/platform"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Component struct {
	pulumi.ResourceState
	Network  *network.Component
	Database *database.Component
	Cache    *cache.Component
	Storage  *storage.Component
	Edge     *edge.Component
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
		Tier: spec.Catalog.CloudSQLTier, AvailabilityType: spec.Catalog.CloudSQLAvailability,
		Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP database: %w", err)
	}
	// Depend on Database so destroy deletes Memorystore before Cloud SQL's PSA
	// connection — parallel teardown races FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION.
	component.Cache, err = cache.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "valkey"), cache.Args{
		Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID.ToStringOutput(), PrivateSubnetIDs: component.Network.PrivateSubnetIDs,
		NodeType: spec.Catalog.MemorystoreNodeType, ReplicaCount: spec.Catalog.MemorystoreReplicas,
		Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP cache: %w", err)
	}
	component.Storage, err = storage.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "media"), storage.Args{
		Project: spec.Identity.GCPProject, Location: spec.Identity.Region, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create GCP media storage: %w", err)
	}
	component.Edge, err = edge.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "edge"), edge.Args{
		Project: spec.Identity.GCPProject, Enabled: spec.Catalog.EnableCloudArmor, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create GCP edge: %w", err)
	}
	component.Runtime, err = runtime.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "app"), runtime.Args{
		Project: spec.Identity.GCPProject, MagentoProject: spec.Identity.Project, Environment: spec.Identity.Environment,
		Region:          spec.Identity.Region,
		NetworkSelfLink: component.Network.NetworkSelfLink, PrivateSubnetNames: component.Network.PrivateSubnetNames,
		Image: spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, WebRuntime: spec.Application.WebRuntime,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		CacheEndpoint: component.Cache.PrimaryEndpoint, SessionEndpoint: component.Cache.PrimaryEndpoint,
		SearchMode: spec.Catalog.SearchMode, SearchReplicas: spec.Catalog.SearchReplicas,
		QueueMode: spec.Catalog.QueueMode, QueueReplicas: spec.Catalog.QueueReplicas,
		MediaBucket: component.Storage.BucketName, MediaURL: component.Storage.MediaURL,
		// EncryptionKeySecret is plumbed for day-2 secret injection; CoreEnvBindings does not
		// emit MAGENTO_DC_CRYPT__KEY yet (shared K8s ceiling with OVH/Scaleway — wire via SecretKeyRef).
		EncryptionKeySecret: spec.Dependencies.EncryptionKeySecret,
		CPURequest:          spec.Catalog.AutopilotCPURequest, MemoryRequest: spec.Catalog.AutopilotMemoryRequest,
		DesiredWebReplicas: spec.Catalog.DesiredWebReplicas, QueueConsumerCount: spec.Catalog.QueueConsumerCount,
		Labels: spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database, component.Cache, component.Storage}))...)
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
		platform.OutputKubeconfig:       c.Runtime.Kubeconfig,
		"mediaURL":                      c.Storage.MediaURL,
		"mediaBucket":                   c.Storage.BucketName,
		"searchEndpoint":                c.Runtime.SearchEndpoint,
		"queueMode":                     c.Runtime.QueueMode,
		"queueHost":                     c.Runtime.QueueHost,
		"securityPolicyName":            c.Edge.SecurityPolicyName,
	}
}
