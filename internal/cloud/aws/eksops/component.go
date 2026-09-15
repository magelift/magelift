package eksops

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/cloud/aws/cache"
	"github.com/magelift/magelift/internal/cloud/aws/database"
	"github.com/magelift/magelift/internal/cloud/aws/eks"
	"github.com/magelift/magelift/internal/cloud/aws/network"
	"github.com/magelift/magelift/internal/cloud/aws/security"
	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type Component struct {
	pulumi.ResourceState
	Network       *network.Component
	Security      *security.Component
	Database      *database.Component
	Cache         *cache.Component
	Runtime       *eks.Component
	queueReplicas int
}

func New(ctx *pulumi.Context, name string, spec Spec, provider *awsprovider.Provider, opts ...pulumi.ResourceOption) (*Component, error) {
	validate := spec.Validate
	if spec.AllowExpiredPreview {
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return nil, fmt.Errorf("validate AWS EKS stack plan: %w", err)
	}
	if name == "" {
		return nil, errors.New("AWS EKS stack name is required")
	}
	component := &Component{queueReplicas: spec.Catalog.QueueReplicas}
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
		AvailabilityZones: spec.Policy.AvailabilityZones, NatMode: spec.Policy.NatMode, NatTopology: spec.Policy.NatTopology, NatReplacementMode: spec.Policy.NatReplacementMode, NatInstanceType: spec.Policy.NatInstanceType,
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
	switch spec.Catalog.DatabaseEngine {
	case DatabaseEngineRDSMySQL:
		engineVersion = spec.Catalog.MySQLVersion
	case DatabaseEngineRDSMariaDB:
		engineVersion = spec.Catalog.MariaDBVersion
	}
	component.Database, err = database.New(ctx, name+"-database", database.Args{
		Preset: spec.Identity.Preset, EnvironmentClass: spec.Identity.EnvironmentClass, Region: spec.Identity.Region,
		AvailabilityZones: databaseZones, DataSubnetIDs: databaseSubnets,
		VpcSecurityGroupIDs: pulumi.StringArray{component.Security.DataSecurityGroupID},
		Engine:              spec.Catalog.DatabaseEngine, EngineVersion: engineVersion,
		DatabaseName: spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		KMSKeyARN: spec.Dependencies.KMSKeyARN, BackupRetentionDays: spec.Catalog.BackupDays, BackupWindow: spec.Catalog.DatabaseBackupWindow, MaintenanceWindow: spec.Catalog.DatabaseMaintenanceWindow,
		DeletionProtection: spec.Catalog.DatabaseDeletionProtection, DeleteAutomatedBackups: spec.Catalog.DatabaseDeleteAutomatedBackups,
		FinalSnapshotIdentifier: name + "-final", ProvisionedInstanceClass: spec.Catalog.InstanceClass,
		InstanceCount: spec.Catalog.InstanceCount, ServerlessV2: serverlessDatabase(spec), Tags: tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS database: %w", err)
	}

	component.Cache, err = cache.New(ctx, name+"-cache", cache.Args{
		Topology: cache.Topology(spec.Identity.Preset), Region: spec.Identity.Region, EngineVersion: spec.Catalog.ValkeyVersion,
		NodeType: spec.Catalog.ValkeyNodeType, ReplicaCount: spec.Catalog.ValkeyReplicaCount,
		SnapshotRetentionLimit: spec.Catalog.CacheSnapshotRetentionLimit, SnapshotWindow: spec.Catalog.CacheSnapshotWindow,
		SubnetIDInputs: dataSubnets, SubnetCount: len(dataSubnets), SecurityGroupInput: component.Security.CacheSecurityGroupID,
		KMSKeyARN: spec.Dependencies.KMSKeyARN,
		// Magento 2.4.9's supplied Valkey adapter does not expose a TLS option;
		// keep this managed cache private while the adapter remains plain-Redis.
		DisableTransitEncryption: true,
		AuthTokens:               cache.AuthTokens{CacheSecretARN: spec.Dependencies.CacheSecretARN, SessionSecretARN: spec.Dependencies.SessionSecretARN},
		Provider:                 provider, Tags: tags,
	})
	if err != nil {
		return nil, fmt.Errorf("create AWS Valkey cache: %w", err)
	}
	encryptionKey, err := resolveEncryptionKey(ctx, spec.Dependencies.EncryptionKeyARN, provider)
	if err != nil {
		return nil, err
	}
	databasePassword, err := resolveDatabasePassword(ctx, component.Database.MasterSecretARN, provider)
	if err != nil {
		return nil, err
	}

	component.Runtime, err = eks.New(ctx, name+"-runtime", eks.Args{
		Region: spec.Identity.Region, AccountID: spec.Identity.AccountID, PrivateSubnetIDs: privateSubnets, KubernetesVersion: spec.Catalog.KubernetesVersion, ComputeMode: spec.Catalog.ComputeMode,
		NodeInstanceType: spec.Catalog.NodeInstanceType, NodeAMI: spec.Catalog.NodeAMI, NodeMinSize: spec.Catalog.NodeMinSize, NodeDesiredSize: spec.Catalog.NodeDesiredSize, NodeMaxSize: spec.Catalog.NodeMaxSize, FargateNamespaces: spec.Catalog.FargateNamespaces,
		Image: spec.Artifact.ImageDigest, ApplicationMode: spec.Application.Mode, ApplicationVersion: spec.Application.Version, WebRuntime: spec.Application.WebRuntime, Magento: spec.Application.Magento,
		EncryptionKey:  encryptionKey,
		DatabaseWriter: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName,
		DatabaseUsername: spec.Dependencies.MasterUsername, DatabasePassword: databasePassword,
		DatabaseAdminUsername: spec.Dependencies.MasterUsername, DatabaseAdminPassword: databasePassword,
		DatabaseReady: []pulumi.Resource{component.Database},
		CacheEndpoint: component.Cache.CachePrimaryEndpoint, SessionEndpoint: component.Cache.SessionPrimaryEndpoint,
		DatabaseSecurityGroupID: component.Security.DataSecurityGroupID, CacheSecurityGroupID: component.Security.CacheSecurityGroupID,
		SearchMode: spec.Catalog.SearchMode, SearchReplicas: spec.Catalog.SearchReplicas,
		QueueMode: spec.Catalog.QueueMode, QueueReplicas: spec.Catalog.QueueReplicas,
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

func resolveEncryptionKey(ctx *pulumi.Context, secretARN string, provider *awsprovider.Provider) (pulumi.StringInput, error) {
	if strings.TrimSpace(secretARN) == "" {
		return nil, errors.New("AWS Secrets Manager encryption key ARN is required")
	}
	readOpts := make([]pulumi.InvokeOption, 0, 1)
	if provider != nil {
		readOpts = append(readOpts, pulumi.Provider(provider))
	}
	version := secretsmanager.LookupSecretVersionOutput(ctx, secretsmanager.LookupSecretVersionOutputArgs{
		SecretId: pulumi.String(secretARN),
	}, readOpts...)
	return pulumi.ToSecret(version.SecretString()).(pulumi.StringOutput), nil
}

func resolveDatabasePassword(ctx *pulumi.Context, secretARN pulumi.StringPtrOutput, provider *awsprovider.Provider) (pulumi.StringInput, error) {
	readOpts := make([]pulumi.InvokeOption, 0, 1)
	if provider != nil {
		readOpts = append(readOpts, pulumi.Provider(provider))
	}
	version := secretsmanager.LookupSecretVersionOutput(ctx, secretsmanager.LookupSecretVersionOutputArgs{
		SecretId: secretARN.Elem(),
	}, readOpts...)
	password := version.SecretString().ApplyT(func(raw string) (string, error) {
		var credentials struct {
			Password string `json:"password"`
		}
		if err := json.Unmarshal([]byte(raw), &credentials); err != nil {
			return "", fmt.Errorf("AWS database master secret is not valid JSON: %w", err)
		}
		if strings.TrimSpace(credentials.Password) == "" {
			return "", errors.New("AWS database master secret has no password")
		}
		return credentials.Password, nil
	}).(pulumi.StringOutput)
	return pulumi.ToSecret(password).(pulumi.StringOutput), nil
}

func (c *Component) Outputs() pulumi.Map {
	return pulumi.Map{
		platform.OutputApplicationURL:          c.Runtime.ApplicationURL,
		platform.OutputDatabaseWriter:          c.Database.WriterEndpoint,
		platform.OutputCacheEndpoint:           c.Cache.CachePrimaryEndpoint,
		platform.OutputNetworkVpcID:            c.Network.VpcID,
		platform.OutputClusterName:             c.Runtime.ClusterName,
		platform.OutputServiceName:             c.Runtime.ServiceName,
		platform.OutputPrivateSubnetIDs:        stringInputs(c.Network.PrivateSubnetIDs),
		platform.OutputKubeconfig:              c.Runtime.Kubeconfig,
		platform.OutputSearchEndpoint:          c.Runtime.SearchEndpoint,
		platform.OutputQueuePasswordSecretName: c.Runtime.QueuePasswordSecretName,
		"queueMode":                            c.Runtime.QueueMode,
		"queueHost":                            c.Runtime.QueueHost,
		platform.OutputQueueReplicas:           pulumi.Int(c.queueReplicas),
		platform.OutputDatabaseSecretName:      c.Runtime.DatabaseSecretName,
		platform.OutputEncryptionKeySecretName: c.Runtime.EncryptionKeySecretName,
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
