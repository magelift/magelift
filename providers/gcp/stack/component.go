package stack

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/secretref"
	"github.com/magelift/magelift/providers/gcp/cache"
	"github.com/magelift/magelift/providers/gcp/database"
	"github.com/magelift/magelift/providers/gcp/edge"
	"github.com/magelift/magelift/providers/gcp/naming"
	"github.com/magelift/magelift/providers/gcp/network"
	"github.com/magelift/magelift/providers/gcp/observability"
	"github.com/magelift/magelift/providers/gcp/runtime"
	"github.com/magelift/magelift/providers/gcp/storage"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/secretmanager"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Component struct {
	pulumi.ResourceState
	Network       *network.Component
	Database      *database.Component
	Cache         *cache.Component
	Storage       *storage.Component
	Edge          *edge.Component
	Runtime       *runtime.Component
	Observability *observability.Component
	queueReplicas int
}

func New(ctx *pulumi.Context, name string, spec Spec, provider *gcp.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	validate := spec.Validate
	if spec.AllowExpiredPreview {
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return nil, fmt.Errorf("validate GCP stack plan: %w", err)
	}
	databaseVersion, err := spec.CloudSQLDatabaseVersion()
	if err != nil {
		return nil, fmt.Errorf("resolve GCP Cloud SQL database version: %w", err)
	}
	memorystoreEngineVersion, err := spec.MemorystoreEngineVersion()
	if err != nil {
		return nil, fmt.Errorf("resolve GCP Memorystore engine version: %w", err)
	}
	if name == "" {
		return nil, errors.New("GCP stack name is required")
	}
	component := &Component{queueReplicas: spec.Catalog.QueueReplicas}
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
	encryptionKey, err := magentoEncryptionKey(ctx, name, spec.Identity.GCPProject, spec.Dependencies.EncryptionKeySecret, provider, component)
	if err != nil {
		return nil, fmt.Errorf("resolve Magento encryption key: %w", err)
	}
	smtpPassword, err := resolveSmtpPassword(ctx, name, spec.Email, provider)
	if err != nil {
		return nil, fmt.Errorf("resolve SMTP relay password: %w", err)
	}

	component.Network, err = network.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "net"), network.Args{
		Preset: spec.Identity.Preset, Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkCIDR: spec.Policy.NetworkCIDR, Zones: spec.Policy.Zones, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create GCP network: %w", err)
	}
	component.Database, err = database.New(ctx, naming.CloudSQLInstance(spec.Identity.Project, spec.Identity.Environment), database.Args{
		Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID.ToStringOutput(), NetworkSelfLink: component.Network.NetworkSelfLink,
		DatabaseName: spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		DatabaseVersion: databaseVersion, Tier: spec.Catalog.CloudSQLTier, AvailabilityType: spec.Catalog.CloudSQLAvailability,
		BackupEnabled: spec.Catalog.CloudSQLBackupEnabled, BinaryLogEnabled: spec.Catalog.CloudSQLBinaryLogEnabled,
		BackupRetentionCount: spec.Catalog.CloudSQLBackupRetentionCount, TransactionLogRetention: spec.Catalog.CloudSQLTransactionLogRetention,
		BackupStartTime: spec.Catalog.CloudSQLBackupStartTime, BackupLocation: spec.Catalog.CloudSQLBackupLocation,
		DeletionProtection: spec.Catalog.CloudSQLDeletionProtection,
		Labels:             spec.Identity.Labels,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP database: %w", err)
	}
	// Depend on Database so destroy deletes Memorystore before Cloud SQL's PSA
	// connection; parallel teardown races FLOW_SN_DC_RESOURCE_PREVENTING_DELETE_CONNECTION.
	component.Cache, err = cache.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "valkey"), cache.Args{
		Project: spec.Identity.GCPProject, Region: spec.Identity.Region,
		NetworkID: component.Network.NetworkID.ToStringOutput(), PrivateSubnetIDs: component.Network.PrivateSubnetIDs,
		NodeType: spec.Catalog.MemorystoreNodeType, ShardCount: spec.Catalog.MemorystoreShardCount,
		EngineVersion: memorystoreEngineVersion, ReplicaCount: spec.Catalog.MemorystoreReplicas,
		Mode: spec.Catalog.MemorystoreMode, ZoneDistributionMode: spec.Catalog.MemorystoreZoneDistributionMode,
		Zone: spec.Catalog.MemorystoreZone, DeletionProtection: spec.Catalog.MemorystoreDeletionProtection,
		PSCConnectionLimit: spec.Catalog.MemorystorePSCConnectionLimit,
		Labels:             spec.Identity.Labels,
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
	edgeName := naming.Resource(spec.Identity.Project, spec.Identity.Environment, "edge")
	component.Edge, err = edge.New(ctx, edgeName, edge.Args{
		Project: spec.Identity.GCPProject, Enabled: spec.Catalog.EnableCloudArmor,
		NativeProvider: spec.Edge.NativeProvider, DomainName: spec.Policy.ApplicationDomain,
		TLS: spec.Edge.TLS, TLSMode: spec.Edge.TLSMode, DNSMode: spec.Edge.DNSMode,
		OwnershipMarker: spec.Edge.OwnershipMarker, Labels: spec.Identity.Labels,
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create GCP edge: %w", err)
	}
	component.Runtime, err = runtime.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "app"), runtime.Args{
		Project: spec.Identity.GCPProject, MagentoProject: spec.Identity.Project, Environment: spec.Identity.Environment,
		Region:  spec.Identity.Region,
		Runtime: string(spec.Identity.Runtime), Zones: spec.Policy.Zones,
		KubernetesVersion: spec.Catalog.KubernetesVersion, ReleaseChannel: spec.Catalog.ReleaseChannel,
		ClusterIPv4CIDR: spec.Catalog.ClusterIPv4CIDR, ServicesIPv4CIDR: spec.Catalog.ServicesIPv4CIDR,
		StandardNodeType: spec.Catalog.StandardNodeType, StandardNodeCount: spec.Catalog.StandardNodeCount,
		StandardNodeMinCount: spec.Catalog.StandardNodeMinCount, StandardNodeMaxCount: spec.Catalog.StandardNodeMaxCount,
		StandardNodeDiskType: spec.Catalog.StandardNodeDiskType, StandardNodeDiskSizeGiB: spec.Catalog.StandardNodeDiskSizeGiB,
		StandardNodeImageType: spec.Catalog.StandardNodeImageType, StandardNodeSpot: spec.Catalog.StandardNodeSpot,
		NetworkSelfLink: component.Network.NetworkSelfLink, PrivateSubnetNames: component.Network.PrivateSubnetNames,
		Image: spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, ApplicationVersion: spec.Application.Version, WebRuntime: spec.Application.WebRuntime, Magento: spec.Application.Magento,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		DatabaseUsername: spec.Dependencies.MasterUsername, DatabasePassword: component.Database.Password,
		DatabaseAdminUsername: database.AdminUsername, DatabaseAdminPassword: component.Database.AdminPassword,
		DatabaseUsersReady: []pulumi.Resource{component.Database.UserReady, component.Database.AdminReady},
		CacheEndpoint:      component.Cache.PrimaryEndpoint, SessionEndpoint: component.Cache.PrimaryEndpoint,
		SearchMode: spec.Catalog.SearchMode, SearchReplicas: spec.Catalog.SearchReplicas,
		SearchImage: spec.Catalog.OpenSearchImage,
		QueueMode:   spec.Catalog.QueueMode, QueueReplicas: spec.Catalog.QueueReplicas,
		QueueImage:        spec.Catalog.RabbitMQImage,
		MediaBucket:       component.Storage.BucketName,
		MediaHmacAccessID: component.Storage.HmacAccessID, MediaHmacSecret: component.Storage.HmacSecret,
		MediaS3Prefix: "",
		EncryptionKey: encryptionKey,
		SmtpHost:      spec.Email.Host, SmtpPort: spec.Email.Port, SmtpUsername: spec.Email.Username,
		SmtpFrom: spec.Email.From, SmtpPassword: smtpPassword,
		CPURequest: spec.Catalog.AutopilotCPURequest, MemoryRequest: spec.Catalog.AutopilotMemoryRequest,
		DesiredWebReplicas: spec.Catalog.DesiredWebReplicas, QueueConsumerCount: spec.Catalog.QueueConsumerCount,
		Labels:                      spec.Identity.Labels,
		NativeObservability:         spec.Observability.NativeProvider == "google-cloud-operations",
		NativeEdge:                  component.Edge.NativeEdgeEnabled,
		NativeEdgeBackendConfigName: component.Edge.BackendConfigName,
	}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Network, component.Database, component.Storage}))...)
	if err != nil {
		return nil, fmt.Errorf("create GCP runtime: %w", err)
	}
	if err := edge.Attach(ctx, edgeName, component.Edge, edge.AttachArgs{
		KubernetesProvider: component.Runtime.KubernetesProvider,
		ServiceName:        component.Runtime.ServiceName,
	}, pulumi.DependsOn([]pulumi.Resource{component.Runtime})); err != nil {
		return nil, fmt.Errorf("attach GCP native edge: %w", err)
	}
	if spec.Observability.NativeProvider == "google-cloud-operations" {
		component.Observability, err = observability.New(ctx, naming.Resource(spec.Identity.Project, spec.Identity.Environment, "observability"), observability.Args{
			Project: spec.Identity.GCPProject, Region: spec.Identity.Region, ClusterName: component.Runtime.ClusterName, Intent: spec.Observability,
		}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{component.Runtime}))...)
		if err != nil {
			return nil, fmt.Errorf("create GCP observability: %w", err)
		}
	}
	if err := ctx.RegisterResourceOutputs(component, component.Outputs()); err != nil {
		return nil, err
	}
	return component, nil
}

// magentoEncryptionKey returns the Magento crypt key. An empty secret ID
// generates one random value, stores it in Secret Manager, and keeps that
// value for the life of the stack. A set ID reads an existing secret so a
// restore can keep the key that already encrypted shop data.
func magentoEncryptionKey(ctx *pulumi.Context, name, project, secretID string, provider *gcp.Provider, parent pulumi.Resource) (pulumi.StringInput, error) {
	if strings.TrimSpace(secretID) != "" {
		return resolveEncryptionKey(ctx, name, project, secretID, provider)
	}
	if strings.TrimSpace(project) == "" {
		return nil, errors.New("GCP project is required to store the generated Magento encryption key")
	}
	opts := []pulumi.ResourceOption{pulumi.Parent(parent)}
	if provider != nil {
		opts = append(opts, pulumi.Provider(provider))
	}
	generated, err := random.NewRandomPassword(ctx, name+"-encryption-key", &random.RandomPasswordArgs{
		Length:  pulumi.Int(32),
		Special: pulumi.Bool(false),
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("generate Magento encryption key: %w", err)
	}
	secret, err := secretmanager.NewSecret(ctx, name+"-encryption-key-secret", &secretmanager.SecretArgs{
		Project:  pulumi.String(project),
		SecretId: pulumi.String(name + "-crypt"),
		Replication: &secretmanager.SecretReplicationArgs{
			Auto: &secretmanager.SecretReplicationAutoArgs{},
		},
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("create Magento encryption key secret: %w", err)
	}
	if _, err := secretmanager.NewSecretVersion(ctx, name+"-encryption-key-version", &secretmanager.SecretVersionArgs{
		Secret:     secret.ID(),
		SecretData: generated.Result,
	}, append(opts, pulumi.DependsOn([]pulumi.Resource{secret, generated}))...); err != nil {
		return nil, fmt.Errorf("store Magento encryption key: %w", err)
	}
	return generated.Result, nil
}

func resolveEncryptionKey(ctx *pulumi.Context, name, project, secretID string, provider *gcp.Provider) (pulumi.StringInput, error) {
	if strings.TrimSpace(project) == "" || strings.TrimSpace(secretID) == "" {
		return nil, errors.New("GCP project and Secret Manager secret ID are required")
	}
	readOpts := make([]pulumi.ResourceOption, 0, 1)
	if provider != nil {
		readOpts = append(readOpts, pulumi.Provider(provider).(pulumi.ResourceOption))
	}
	version, err := secretmanager.GetSecretVersion(ctx, name+"-encryption-key-version",
		pulumi.ID(fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, secretID)), nil, readOpts...)
	if err != nil {
		return nil, fmt.Errorf("read Secret Manager secret %q: %w", secretID, err)
	}
	key := version.SecretData.ApplyT(func(value *string) (string, error) {
		if value == nil || strings.TrimSpace(*value) == "" {
			return "", errors.New("Secret Manager encryption key is empty")
		}
		return *value, nil
	}).(pulumi.StringOutput)
	return pulumi.ToSecret(key).(pulumi.StringOutput), nil
}

// resolveSmtpPassword reads the relay password from Secret Manager when
// email is enabled. It mirrors resolveEncryptionKey: the reference was
// validated at plan time, the value stays a Pulumi secret end to end.
func resolveSmtpPassword(ctx *pulumi.Context, name string, email EmailSelection, provider *gcp.Provider) (pulumi.StringInput, error) {
	if !email.Enabled() {
		return nil, nil
	}
	reference, err := secretref.Parse(strings.TrimSpace(email.Credential))
	if err != nil {
		return nil, err
	}
	readOpts := make([]pulumi.ResourceOption, 0, 1)
	if provider != nil {
		readOpts = append(readOpts, pulumi.Provider(provider).(pulumi.ResourceOption))
	}
	version, err := secretmanager.GetSecretVersion(ctx, name+"-smtp-password-version",
		pulumi.ID(reference.ID), nil, readOpts...)
	if err != nil {
		return nil, fmt.Errorf("read Secret Manager secret %q: %w", reference.ID, err)
	}
	password := version.SecretData.ApplyT(func(value *string) (string, error) {
		if value == nil || strings.TrimSpace(*value) == "" {
			return "", errors.New("Secret Manager SMTP password is empty")
		}
		return *value, nil
	}).(pulumi.StringOutput)
	return pulumi.ToSecret(password).(pulumi.StringOutput), nil
}

// mediaAppURL is the storefront media base operators verify delivery
// through. Image URLs stay app-relative and materialize via get.php;
// the bucket itself is private and never addressed directly.
func mediaAppURL(domain string) pulumi.StringOutput {
	return pulumi.String(mediaAppURLValue(domain)).ToStringOutput()
}

func mediaAppURLValue(domain string) string {
	trimmed := strings.TrimSpace(domain)
	if trimmed == "" {
		return ""
	}
	return "https://" + trimmed + "/media/"
}

func (c *Component) Outputs() pulumi.Map {
	applicationURL := c.Runtime.ApplicationURL
	if c.Edge.NativeEdgeEnabled {
		applicationURL = c.Edge.ApplicationURL
	}
	outputs := pulumi.Map{
		platform.OutputApplicationURL:          applicationURL,
		platform.OutputDatabaseWriter:          c.Database.WriterEndpoint,
		platform.OutputCacheEndpoint:           c.Cache.PrimaryEndpoint,
		platform.OutputNetworkVpcID:            c.Network.NetworkID,
		platform.OutputClusterName:             c.Runtime.ClusterName,
		platform.OutputServiceName:             c.Runtime.ServiceName,
		platform.OutputPrivateSubnetIDs:        c.Network.PrivateSubnetIDs,
		platform.OutputKubeconfig:              c.Runtime.Kubeconfig,
		platform.OutputDatabaseConnectionName:  c.Database.ConnectionName,
		platform.OutputDatabaseSecretName:      c.Runtime.DatabaseSecretName,
		platform.OutputEncryptionKeySecretName: c.Runtime.EncryptionKeySecretName,
		platform.OutputQueuePasswordSecretName: c.Runtime.QueuePasswordSecretName,
		platform.OutputMediaURL:                mediaAppURL(c.Edge.DomainName),
		"mediaBucket":                          c.Storage.BucketName,
		"searchEndpoint":                       c.Runtime.SearchEndpoint,
		"queueMode":                            c.Runtime.QueueMode,
		"queueHost":                            c.Runtime.QueueHost,
		platform.OutputQueueReplicas:           pulumi.Int(c.queueReplicas),
		"securityPolicyName":                   c.Edge.SecurityPolicyName,
		"nativeEdgeBackendConfigName":          c.Edge.BackendConfigName,
		"nativeEdgeIngressName":                c.Edge.IngressName,
	}
	if c.Observability != nil {
		outputs["observabilityDashboardId"] = c.Observability.DashboardID
		outputs["observabilityDashboardIds"] = c.Observability.DashboardIDs
		outputs["observabilityAlertPolicyNames"] = c.Observability.AlertPolicyNames
		outputs["observabilityUnavailableSignals"] = c.Observability.UnavailableSignals
		outputs["observabilityUnavailableOperations"] = c.Observability.UnavailableOperations
	}
	return outputs
}
