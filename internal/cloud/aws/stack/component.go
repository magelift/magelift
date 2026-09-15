package stack

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/magelift/magelift/internal/cloud/aws/cache"
	"github.com/magelift/magelift/internal/cloud/aws/database"
	"github.com/magelift/magelift/internal/cloud/aws/edge"
	"github.com/magelift/magelift/internal/cloud/aws/ingress"
	"github.com/magelift/magelift/internal/cloud/aws/network"
	"github.com/magelift/magelift/internal/cloud/aws/observability"
	"github.com/magelift/magelift/internal/cloud/aws/queue"
	"github.com/magelift/magelift/internal/cloud/aws/runtime"
	"github.com/magelift/magelift/internal/cloud/aws/search"
	"github.com/magelift/magelift/internal/cloud/aws/security"
	"github.com/magelift/magelift/internal/cloud/aws/storage"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/secretref"
	sdk "github.com/magelift/magelift/sdk/v1"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/iam"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const varnishImage = "docker.io/library/varnish:8.0.2@sha256:4b595728592a5b9709c9aac15368ca492e9742fb269ed12466b434a62b2c1b63"

type Component struct {
	pulumi.ResourceState
	Network         *network.Component
	Security        *security.Component
	Ingress         *ingress.Component
	RuntimeIdentity *runtime.Identity
	Runtime         *runtime.Component
	Database        *database.Component
	Cache           *cache.Component
	Search          *search.Component
	Queue           *queue.Component
	Storage         *storage.Component
	Edge            *edge.Component
	Observability   *observability.Component
}

func New(ctx *pulumi.Context, name string, spec Spec, providers Providers, opts ...pulumi.ResourceOption) (*Component, error) {
	validate := spec.Validate
	if spec.AllowExpiredPreview {
		validate = spec.ValidateAllowExpiredPreview
	}
	if err := validate(); err != nil {
		return nil, fmt.Errorf("validate AWS stack plan: %w", err)
	}
	if err := providers.Validate(); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errors.New("AWS stack name is required")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2("magelift:aws:Stack", name, pulumi.Map{
		"project": pulumi.String(spec.Identity.Project), "environment": pulumi.String(spec.Identity.Environment),
		"region": pulumi.String(spec.Identity.Region), "preset": pulumi.String(spec.Identity.Preset),
		"imageDigest": pulumi.String(spec.Artifact.ImageDigest), "catalogVersion": pulumi.String(spec.Catalog.Version),
	}, component, opts...); err != nil {
		return nil, err
	}
	regional := []pulumi.ResourceOption{pulumi.Parent(component), pulumi.Provider(providers.Regional)}
	global := []pulumi.ResourceOption{pulumi.Parent(component), pulumi.Provider(providers.Global)}
	tags := spec.Identity.Tags

	var err error
	frontendPort := runtime.FrontendPort(spec.Application.Mode)
	interfaceEndpoints := []network.InterfaceEndpoint(nil)
	if spec.Identity.Preset != sdk.PresetPreview {
		interfaceEndpoints = []network.InterfaceEndpoint{
			{Service: network.InterfaceEndpointECRAPI, Rationale: network.ReduceNATCostAndExposure},
			{Service: network.InterfaceEndpointECRDKR, Rationale: network.ReduceNATCostAndExposure},
			{Service: network.InterfaceEndpointLogs, Rationale: network.ReduceNATCostAndExposure},
			{Service: network.InterfaceEndpointSecrets, Rationale: network.ReduceNATCostAndExposure},
			{Service: network.InterfaceEndpointSSM, Rationale: network.ReduceNATCostAndExposure},
			{Service: network.InterfaceEndpointSSMMsg, Rationale: network.ReduceNATCostAndExposure},
			{Service: network.InterfaceEndpointEC2Msg, Rationale: network.ReduceNATCostAndExposure},
		}
	}
	networkArgs := network.Args{
		Preset: spec.Identity.Preset, Region: spec.Identity.Region, VPCCIDR: spec.Policy.VPCCIDR.String(),
		AvailabilityZones: spec.Policy.AvailabilityZones, NatMode: spec.Policy.NatMode, NatTopology: spec.Policy.NatTopology, NatReplacementMode: spec.Policy.NatReplacementMode, NatInstanceType: spec.Policy.NatInstanceType,
		GatewayEndpoints: []network.GatewayEndpoint{{Service: network.GatewayEndpointS3, Rationale: network.ReduceNATCostAndExposure}}, InterfaceEndpoints: interfaceEndpoints, Tags: tags,
	}
	if spec.Existing.Network != nil {
		networkArgs.Existing = &network.ExistingNetwork{VPCID: spec.Existing.Network.ExternalID, PublicSubnetIDs: spec.Existing.PublicSubnetIDs, PrivateSubnetIDs: spec.Existing.PrivateSubnetIDs, DataSubnetIDs: spec.Existing.DataSubnetIDs}
		networkArgs.GatewayEndpoints = nil
		networkArgs.InterfaceEndpoints = nil
	}
	component.Network, err = network.New(ctx, name+"-network", networkArgs, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS network: %w", err)
	}
	component.Security, err = security.New(ctx, name+"-security", security.Args{
		Region: spec.Identity.Region, VPCID: component.Network.VpcID.ToStringOutput(), WebTargetPort: frontendPort,
		QueuePort: queue.AMQPPortForMode(spec.Catalog.QueueMode), Tags: tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS security groups: %w", err)
	}
	publicSubnets := stringInputs(component.Network.PublicSubnetIDs)
	privateSubnets := stringInputs(component.Network.PrivateSubnetIDs)
	dataSubnets := stringInputs(component.Network.DataSubnetIDs)
	databaseCount := 2
	if spec.Identity.Preset == sdk.PresetHighAvailability {
		databaseCount = 3
	}
	databaseSubnets, databaseZones := firstN(dataSubnets, spec.Policy.AvailabilityZones, databaseCount)
	searchCount := 2
	switch {
	case spec.Catalog.SearchMode == SearchModeProvisioned && spec.Identity.Preset == sdk.PresetPreview:
		searchCount = 1
	case spec.Identity.Preset == sdk.PresetHighAvailability:
		searchCount = 3
	}
	searchSubnets, _ := firstN(dataSubnets, spec.Policy.AvailabilityZones, searchCount)

	component.Ingress, err = ingress.New(ctx, name+"-ingress", ingress.Args{
		Region: spec.Identity.Region, VPCID: component.Network.VpcID.ToStringOutput(), PublicSubnetIDs: publicSubnets,
		SecurityGroupID: component.Security.EdgeSecurityGroupID, CertificateARN: pulumi.String(spec.Existing.ALBCertificate.ExternalID),
		TargetPort: frontendPort, HealthCheckPath: "/health", Provider: providers.Regional, Tags: tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS ingress: %w", err)
	}
	component.RuntimeIdentity, err = runtime.NewIdentity(ctx, name+"-runtime", runtime.IdentityArgs{Secrets: runtimeSecrets(spec), Tags: tags}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS ECS runtime identity: %w", err)
	}

	engineVersion := spec.Catalog.Versions.AuroraMySQL
	switch spec.Catalog.DatabaseEngine {
	case DatabaseEngineRDSMySQL:
		engineVersion = spec.Catalog.Versions.MySQL
	case DatabaseEngineRDSMariaDB:
		engineVersion = spec.Catalog.Versions.MariaDB
	}
	serverless := serverlessDatabase(spec)
	provisionedClass := spec.Catalog.AuroraProvisioned.InstanceClass
	provisionedCount := spec.Catalog.AuroraProvisioned.InstanceCount
	if serverless != nil {
		provisionedClass = ""
		provisionedCount = 0
	}
	databaseArgs := database.Args{
		Preset: spec.Identity.Preset, EnvironmentClass: spec.Identity.EnvironmentClass, Region: spec.Identity.Region, AvailabilityZones: databaseZones, DataSubnetIDs: databaseSubnets,
		VpcSecurityGroupIDs: pulumi.StringArray{component.Security.DataSecurityGroupID}, Engine: spec.Catalog.DatabaseEngine, EngineVersion: engineVersion, DatabaseName: spec.Dependencies.DatabaseName, MasterUsername: spec.Dependencies.MasterUsername,
		KMSKeyARN: spec.Dependencies.KMSKeyARN, BackupRetentionDays: spec.Catalog.Retention.BackupDays, BackupWindow: spec.Catalog.DatabaseBackupWindow, MaintenanceWindow: spec.Catalog.DatabaseMaintenanceWindow, DeletionProtection: spec.Catalog.DatabaseDeletionProtection, DeleteAutomatedBackups: spec.Catalog.DatabaseDeleteAutomatedBackups, FinalSnapshotIdentifier: name + "-final", ProvisionedInstanceClass: provisionedClass, InstanceCount: provisionedCount,
		ServerlessV2: serverless, Tags: tags,
	}
	if spec.Existing.Database != nil {
		databaseArgs.Existing = &database.ExistingDatabase{
			Identifier: spec.Existing.Database.ExternalID,
			Endpoint:   spec.Existing.DatabaseEndpoint,
			SecretARN:  spec.Existing.DatabaseSecretARN,
		}
	}
	component.Database, err = database.New(ctx, name+"-database", databaseArgs, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS database: %w", err)
	}
	if _, err := iam.NewRolePolicy(ctx, name+"-execution-database-policy", &iam.RolePolicyArgs{
		Role:   component.RuntimeIdentity.ExecutionRoleName,
		Policy: databaseExecutionPolicy(component.Database.MasterSecretARN, spec.Dependencies.KMSKeyARN, spec.Identity.Region),
	}, regional...); err != nil {
		return nil, fmt.Errorf("grant ECS execution access to managed database secret: %w", err)
	}

	component.Cache, err = cache.New(ctx, name+"-cache", cache.Args{
		Topology: cache.Topology(spec.Identity.Preset), Region: spec.Identity.Region, EngineVersion: spec.Catalog.Versions.Valkey, NodeType: spec.Catalog.Valkey.NodeType, ReplicaCount: spec.Catalog.Valkey.ReplicaCount,
		SnapshotRetentionLimit: spec.Catalog.CacheSnapshotRetentionLimit, SnapshotWindow: spec.Catalog.CacheSnapshotWindow, SubnetIDInputs: dataSubnets, SubnetCount: len(dataSubnets), SecurityGroupInput: component.Security.CacheSecurityGroupID, KMSKeyARN: spec.Dependencies.KMSKeyARN,
		AuthTokens: cache.AuthTokens{CacheSecretARN: spec.Dependencies.CacheSecretARN, SessionSecretARN: spec.Dependencies.SessionSecretARN}, Provider: providers.Regional, Tags: tags,
	})
	if err != nil {
		return nil, fmt.Errorf("create AWS Valkey cache: %w", err)
	}

	searchEndpoint := pulumi.String("").ToStringOutput()
	searchARN := pulumi.String("").ToStringOutput()
	if spec.Catalog.SearchMode != SearchModeDisabled {
		component.Search, err = search.New(ctx, name+"-search", search.Args{
			Preset: spec.Identity.Preset, Region: spec.Identity.Region, VPCID: component.Network.VpcID.ToStringOutput(), SubnetIDs: searchSubnets,
			SecurityGroupIDs: pulumi.StringArray{component.Security.SearchSecurityGroupID}, KMSKeyARN: spec.Dependencies.KMSKeyARN, AccessIdentityInput: component.RuntimeIdentity.TaskRoleARN,
			Serverless: serverlessSearch(spec), Provisioned: provisionedSearch(spec), Tags: tags,
		}, regional...)
		if err != nil {
			return nil, fmt.Errorf("create AWS OpenSearch: %w", err)
		}
		searchEndpoint = component.Search.Endpoint
		searchARN = component.Search.ARN
	}

	logGroupPrefix := "/magelift/" + spec.Identity.Project + "/" + spec.Identity.Environment
	component.Queue, err = queue.New(ctx, name+"-queue", queue.Args{
		Mode: spec.Catalog.QueueMode, Topology: queue.Topology(spec.Identity.Preset), Region: spec.Identity.Region, EngineVersion: spec.Catalog.Versions.RabbitMQ, InstanceType: spec.Catalog.RabbitMQ.InstanceType,
		AvailabilityZones: spec.Policy.AvailabilityZones, SubnetIDInputs: privateSubnets, SubnetCount: len(privateSubnets), SecurityGroupIDInputs: pulumi.StringArray{component.Security.QueueSecurityGroupID}, SecurityGroupCount: 1,
		KMSKeyARN: spec.Dependencies.KMSKeyARN, Credentials: queue.Credentials{SecretARN: spec.Dependencies.QueueSecretARN, Username: spec.Dependencies.MasterUsername}, Provider: providers.Regional, Tags: tags,
		VpcID: component.Network.VpcID.ToStringOutput(), ExecutionRoleARN: component.RuntimeIdentity.ExecutionRoleARN, TaskRoleARN: component.RuntimeIdentity.TaskRoleARN, LogGroupPrefix: logGroupPrefix, LogRetentionDays: spec.Catalog.Retention.LogDays,
	})
	if err != nil {
		return nil, fmt.Errorf("create AWS queue: %w", err)
	}

	component.Storage, err = storage.New(ctx, name+"-storage", storage.Args{
		Region: spec.Identity.Region, BucketName: name + "-media", KMSKeyARN: spec.Dependencies.KMSKeyARN, NoncurrentVersionRetentionDays: spec.Catalog.Retention.ArtifactDays, AbortMultipartUploadDays: 7,
		Domain: spec.Policy.MediaDomain, CertificateARN: spec.Existing.Certificate.ExternalID, RegionalProvider: providers.Regional, GlobalProvider: providers.Global, Tags: tags,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS media storage: %w", err)
	}
	capabilityConfig := &runtime.CapabilityConfig{
		DatabaseWriterEndpoint: component.Database.WriterEndpoint, DatabaseName: spec.Dependencies.DatabaseName, DatabaseSecretARN: databaseSecretARN(component.Database.MasterSecretARN),
		CacheEndpoint: component.Cache.CachePrimaryEndpoint, SessionEndpoint: component.Cache.SessionPrimaryEndpoint,
		SearchEndpoint: searchEndpoint, QueueMode: component.Queue.QueueMode, QueueEndpoint: component.Queue.AMQPEndpoint,
		QueueUsername: pulumi.String(spec.Dependencies.MasterUsername),
		MediaBucket:   component.Storage.BucketName,
	}
	logGroups, err := observability.NewLogGroups(ctx, name+"-observability", observability.Args{
		Region: spec.Identity.Region, EnvironmentClass: spec.Identity.EnvironmentClass, LogGroupPrefix: logGroupPrefix,
		KMSKeyARN: spec.Dependencies.KMSKeyARN, RetentionInDays: spec.Catalog.Retention.LogDays, Tags: tags,
		Intent: spec.Observability,
	}, regional...)
	if err != nil {
		return nil, fmt.Errorf("create AWS workload log groups: %w", err)
	}
	runtimeOpts := append([]pulumi.ResourceOption{}, regional...)
	runtimeOpts = append(runtimeOpts, pulumi.DependsOn([]pulumi.Resource{logGroups}))
	component.Runtime, err = runtime.New(ctx, name+"-runtime", runtime.Args{
		ComputeMode: spec.Catalog.Fargate.ComputeMode, InstanceType: spec.Catalog.Fargate.InstanceType, InstanceAMI: spec.Catalog.Fargate.InstanceAMI, MinCapacity: spec.Catalog.Fargate.MinCapacity, MaxCapacity: spec.Catalog.Fargate.MaxCapacity,
		ApplicationMode: spec.Application.Mode, ApplicationVersion: spec.Application.Version, WebRuntime: spec.Application.WebRuntime, Magento: spec.Application.Magento,
		Region: spec.Identity.Region, VpcID: component.Network.VpcID.ToStringOutput(), PrivateSubnetIDs: privateSubnets,
		Image: spec.Artifact.ImageDigest, DatabaseSecretARN: databaseSecretARN(component.Database.MasterSecretARN), ContainerPort: frontendPort, VarnishImage: varnishImageFor(spec.Application.Mode), TaskCPU: strconv.Itoa(spec.Catalog.Fargate.CPU), TaskMemory: strconv.Itoa(spec.Catalog.Fargate.MemoryMiB), DesiredCount: spec.Catalog.Fargate.DesiredCount, QueueConsumerCount: queueConsumerCount(spec),
		WebSecurityGroupID: component.Security.WebSecurityGroupID, TargetGroupARN: component.Ingress.TargetGroupARN,
		Secrets: runtimeSecrets(spec), Identity: component.RuntimeIdentity,
		Capabilities:     capabilityConfig,
		SearchProxyImage: searchSigningProxyImage(spec),
		EncryptionKeyARN: pulumi.String(spec.Dependencies.EncryptionKeyARN),
		LogGroupPrefix:   logGroupPrefix,
		Tags:             tags,
	}, runtimeOpts...)
	if err != nil {
		return nil, fmt.Errorf("create AWS ECS runtime: %w", err)
	}
	if nativeEdgeEnabled(spec.Edge) {
		component.Edge, err = edge.New(ctx, name+"-edge", edge.Args{
			DomainName: spec.Policy.ApplicationDomain, FrontName: spec.Application.Magento.FrontName, HostedZone: *spec.Existing.HostedZone, Certificate: *spec.Existing.Certificate,
			Origin: edge.ALBOrigin{DNSName: component.Ingress.DNSName, ARNInput: component.Ingress.LoadBalancerARN}, GlobalAWS: providers.Global, Security: edge.DefaultSecurityPolicy(), Tags: tags,
		}, global...)
		if err != nil {
			return nil, fmt.Errorf("create AWS edge: %w", err)
		}
	}
	if spec.Observability.NativeProvider == "cloudwatch" {
		component.Observability, err = observability.New(ctx, name+"-observability", observability.Args{
			Region: spec.Identity.Region, EnvironmentClass: spec.Identity.EnvironmentClass, LogGroupPrefix: logGroupPrefix,
			KMSKeyARN: spec.Dependencies.KMSKeyARN, RetentionInDays: spec.Catalog.Retention.LogDays, ECSClusterNameInput: component.Runtime.ClusterName, ECSServiceNameInput: component.Runtime.ServiceName,
			DesiredTaskCount: spec.Catalog.Fargate.DesiredCount, LoadBalancerDimensionInput: component.Ingress.LoadBalancerDimension, NotificationTopicARN: spec.Existing.SNSTopicARN,
			SyntheticEnabled: spec.Identity.EnvironmentClass == "production", SyntheticURL: "https://" + spec.Policy.ApplicationDomain + "/health", SyntheticArtifactRetentionDays: spec.Catalog.Retention.ArtifactDays, Tags: tags,
			ExistingLogGroups: logGroups, Intent: spec.Observability,
		}, regional...)
		if err != nil {
			return nil, fmt.Errorf("create AWS observability: %w", err)
		}
	}
	capabilityPolicy := taskPolicy(spec, component, searchARN)
	if _, err := iam.NewRolePolicy(ctx, name+"-task-policy", &iam.RolePolicyArgs{
		Role: component.Runtime.TaskRoleName, Policy: capabilityPolicy,
	}, regional...); err != nil {
		return nil, fmt.Errorf("grant Magento task capabilities: %w", err)
	}
	if _, err := iam.NewRolePolicy(ctx, name+"-deployment-policy", &iam.RolePolicyArgs{
		Role: component.Runtime.DeploymentRoleName, Policy: capabilityPolicy,
	}, regional...); err != nil {
		return nil, fmt.Errorf("grant Magento deployment capabilities: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(component, component.Outputs()); err != nil {
		return nil, err
	}
	return component, nil
}

// Outputs returns the stack-level values deploy, health, exec, and outputs
// commands read via Pulumi Automation API. Keep this map and Program exports
// in sync; RegisterResourceOutputs alone does not surface stack outputs.
func (c *Component) Outputs() pulumi.Map {
	searchEndpoint := pulumi.String("").ToStringOutput()
	if c.Search != nil {
		searchEndpoint = c.Search.Endpoint
	}
	edgeDistributionID := pulumi.ID("").ToIDOutput()
	applicationURL := pulumi.Sprintf("https://%s", c.Ingress.DNSName)
	if c.Edge != nil {
		edgeDistributionID = c.Edge.DistributionID
		applicationURL = pulumi.Sprintf("https://%s", c.Edge.DistributionDomainName)
	}
	outputs := pulumi.Map{
		"networkVpcId": c.Network.VpcID, "edgeDistributionId": edgeDistributionID,
		"applicationURL": applicationURL, "mediaURL": c.Storage.DistributionURL, "mediaBucket": c.Storage.BucketName,
		"databaseWriter": c.Database.WriterEndpoint, "cacheEndpoint": c.Cache.CachePrimaryEndpoint, "searchEndpoint": searchEndpoint, "queueMode": c.Queue.QueueMode,
		"clusterName": c.Runtime.ClusterName, "clusterArn": c.Runtime.ClusterARN, "serviceName": c.Runtime.ServiceName,
		"taskDefinitionArn": c.Runtime.TaskDefinitionARN, "deployTaskDefinitionArn": c.Runtime.DeployTaskDefinitionARN,
		"cronServiceName": c.Runtime.CronServiceName, "cronTaskDefinitionArn": c.Runtime.CronTaskDefinitionARN,
		"queueServiceName": c.Runtime.QueueServiceName, "queueTaskDefinitionArn": c.Runtime.QueueTaskDefinitionARN,
		"taskRoleArn": c.Runtime.TaskRoleARN, "deploymentRoleArn": c.Runtime.DeploymentRoleARN, "securityGroupId": c.Runtime.SecurityGroupID, "privateSubnetIds": stringInputs(c.Network.PrivateSubnetIDs),
	}
	if c.Observability != nil {
		outputs["observabilityUnavailableSignals"] = c.Observability.UnavailableSignals
		outputs["observabilityUnavailableOperations"] = c.Observability.UnavailableOperations
	}
	return outputs
}

func databaseSecretARN(input pulumi.StringPtrOutput) pulumi.StringOutput {
	return input.ApplyT(func(value *string) string {
		if value == nil {
			return ""
		}
		return *value
	}).(pulumi.StringOutput)
}

func taskPolicy(spec Spec, component *Component, searchARN pulumi.StringOutput) pulumi.StringOutput {
	return pulumi.All(component.Storage.BucketARN, searchARN, component.Database.MasterSecretARN).ApplyT(func(values []interface{}) (string, error) {
		bucketARN, _ := values[0].(string)
		searchARNValue, _ := values[1].(string)
		databaseSecretARN := ""
		switch value := values[2].(type) {
		case *string:
			if value != nil {
				databaseSecretARN = *value
			}
		case string:
			databaseSecretARN = value
		}
		secrets := []string{spec.Dependencies.CacheSecretARN, spec.Dependencies.EncryptionKeyARN}
		if spec.Identity.Preset != sdk.PresetPreview {
			secrets = append(secrets, spec.Dependencies.SessionSecretARN)
		}
		if queue.NeedsBrokerSecret(effectiveQueueMode(spec)) {
			secrets = append(secrets, spec.Dependencies.QueueSecretARN)
		}
		if databaseSecretARN != "" {
			secrets = append(secrets, databaseSecretARN)
		}
		statements := []map[string]interface{}{
			{"Effect": "Allow", "Action": []string{"s3:ListBucket"}, "Resource": bucketARN},
			{"Effect": "Allow", "Action": []string{"s3:GetObject", "s3:PutObject", "s3:DeleteObject"}, "Resource": bucketARN + "/*"},
			{"Effect": "Allow", "Action": []string{"kms:Decrypt", "kms:Encrypt", "kms:GenerateDataKey"}, "Resource": spec.Dependencies.KMSKeyARN},
			{"Effect": "Allow", "Action": []string{"ssmmessages:CreateControlChannel", "ssmmessages:CreateDataChannel", "ssmmessages:OpenControlChannel", "ssmmessages:OpenDataChannel"}, "Resource": "*"},
			{"Effect": "Allow", "Action": []string{"secretsmanager:GetSecretValue"}, "Resource": secrets},
		}
		if searchARNValue != "" {
			searchAction, searchResource := "es:ESHttp*", searchARNValue+"/*"
			if spec.Catalog.SearchMode == SearchModeServerless {
				searchAction, searchResource = "aoss:APIAccessAll", searchARNValue
			}
			statements = append(statements, map[string]interface{}{"Effect": "Allow", "Action": []string{searchAction}, "Resource": searchResource})
		}
		document := struct {
			Version   string                   `json:"Version"`
			Statement []map[string]interface{} `json:"Statement"`
		}{Version: "2012-10-17", Statement: statements}
		encoded, err := json.Marshal(document)
		return string(encoded), err
	}).(pulumi.StringOutput)
}

func databaseExecutionPolicy(secretARN pulumi.StringPtrOutput, kmsKeyARN, region string) pulumi.StringOutput {
	return secretARN.ApplyT(func(value *string) (string, error) {
		if value == nil || *value == "" {
			return "", errors.New("managed database secret ARN is required")
		}
		if kmsKeyARN == "" || region == "" {
			return "", errors.New("managed database secret KMS key ARN is required")
		}
		domainSuffix := "amazonaws.com"
		if strings.HasPrefix(region, "cn-") {
			domainSuffix = "amazonaws.com.cn"
		}
		document := struct {
			Version   string           `json:"Version"`
			Statement []map[string]any `json:"Statement"`
		}{
			Version: "2012-10-17",
			Statement: []map[string]any{
				{"Effect": "Allow", "Action": []string{"secretsmanager:GetSecretValue"}, "Resource": *value},
				{"Effect": "Allow", "Action": []string{"kms:Decrypt"}, "Resource": kmsKeyARN, "Condition": map[string]any{"StringEquals": map[string]string{"kms:ViaService": "secretsmanager." + region + "." + domainSuffix}}},
			},
		}
		encoded, err := json.Marshal(document)
		return string(encoded), err
	}).(pulumi.StringOutput)
}

func stringInputs(ids []pulumi.IDOutput) pulumi.StringArray {
	result := make(pulumi.StringArray, len(ids))
	for index, id := range ids {
		result[index] = id.ToStringOutput()
	}
	return result
}

func firstN(subnets pulumi.StringArray, zones []string, count int) (pulumi.StringArray, []string) {
	if count > len(subnets) {
		count = len(subnets)
	}
	return subnets[:count], zones[:count]
}

func serverlessDatabase(spec Spec) *database.ServerlessV2 {
	if spec.Identity.Preset != sdk.PresetPreview || spec.Catalog.DatabaseEngine != DatabaseEngineAuroraMySQL {
		return nil
	}
	profile := spec.Catalog.Aurora
	return &database.ServerlessV2{MinimumACU: profile.MinimumACU, MaximumACU: profile.MaximumACU, AutoPauseSeconds: profile.AutoPauseSeconds, EngineSupportsAutoPause: profile.EngineSupportsAutoPause}
}

func queueConsumerCount(spec Spec) int {
	mode := spec.Catalog.QueueMode
	if mode == "" {
		mode = defaultQueueMode(spec.Identity.Preset)
	}
	count := 0
	if mode != QueueModeDB {
		count = 2
	}
	return platform.MagentoConsumerProcessCount(spec.Application.Magento.ConsumersMode, count)
}

func searchSigningProxyImage(spec Spec) string {
	if spec.Catalog.SearchMode != SearchModeServerless {
		return ""
	}
	return runtime.DefaultSearchProxyImage
}

func varnishImageFor(applicationMode string) string {
	if applicationMode == "integrated" {
		return varnishImage
	}
	return ""
}

func serverlessSearch(spec Spec) *search.Serverless {
	if spec.Catalog.SearchMode != SearchModeServerless {
		return nil
	}
	profile := spec.Catalog.Search
	return &search.Serverless{AcceptColdStarts: profile.AcceptColdStarts, Capacity: search.ServerlessCapacity{MinimumIndexingOCU: 1, MaximumIndexingOCU: profile.MaximumIndexingOCU, MinimumSearchOCU: 1, MaximumSearchOCU: profile.MaximumSearchOCU}}
}

func provisionedSearch(spec Spec) *search.Provisioned {
	if spec.Catalog.SearchMode != SearchModeProvisioned {
		return nil
	}
	profile := spec.Catalog.SearchProvisioned
	return &search.Provisioned{EngineVersion: spec.Catalog.Versions.OpenSearch, InstanceType: profile.InstanceType, InstanceCount: profile.InstanceCount, DedicatedMasterType: profile.DedicatedMasterType, DedicatedMasterCount: profile.DedicatedMasterCount, EBSVolumeType: profile.EBSVolumeType, EBSVolumeSizeGiB: profile.EBSVolumeSizeGiB}
}

func runtimeSecrets(spec Spec) []runtime.SecretReference {
	secrets := []runtime.SecretReference{
		{Name: "MAGELIFT_CACHE_TOKEN", ARN: spec.Dependencies.CacheSecretARN},
		{Name: "MAGENTO_DC_CACHE__FRONTEND__DEFAULT__BACKEND_OPTIONS__PASSWORD", ARN: spec.Dependencies.CacheSecretARN},
		{Name: "MAGENTO_DC_CACHE__FRONTEND__PAGE_CACHE__BACKEND_OPTIONS__PASSWORD", ARN: spec.Dependencies.CacheSecretARN},
	}
	sessionSecret := spec.Dependencies.CacheSecretARN
	if spec.Identity.Preset != sdk.PresetPreview {
		sessionSecret = spec.Dependencies.SessionSecretARN
		secrets = append(secrets,
			runtime.SecretReference{Name: "MAGELIFT_SESSION_TOKEN", ARN: spec.Dependencies.SessionSecretARN},
		)
	}
	if queue.NeedsBrokerSecret(effectiveQueueMode(spec)) {
		secrets = append(secrets,
			runtime.SecretReference{Name: "MAGELIFT_QUEUE_PASSWORD", ARN: spec.Dependencies.QueueSecretARN},
			runtime.SecretReference{Name: "MAGENTO_DC_QUEUE__AMQP__PASSWORD", ARN: spec.Dependencies.QueueSecretARN},
		)
	}
	secrets = append(secrets,
		runtime.SecretReference{Name: "MAGENTO_DC_SESSION__REDIS_PASSWORD", ARN: sessionSecret},
		runtime.SecretReference{Name: "MAGENTO_DC_CRYPT__KEY", ARN: spec.Dependencies.EncryptionKeyARN},
	)
	secrets = append(secrets, magentoAWSSecretOverlays(spec.Application.Magento)...)
	return secrets
}

func magentoAWSSecretOverlays(overlays platform.MagentoOverlays) []runtime.SecretReference {
	keys := make([]string, 0, len(overlays.Variables))
	for key := range overlays.Variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var secrets []runtime.SecretReference
	for _, key := range keys {
		reference, err := secretref.Parse(overlays.Variables[key])
		if err != nil || reference.Kind != secretref.SecretsManager {
			continue
		}
		secrets = append(secrets, runtime.SecretReference{Name: key, ARN: reference.ID, JSONKey: reference.JSONField})
	}
	return secrets
}

func effectiveQueueMode(spec Spec) string {
	if mode := strings.TrimSpace(spec.Catalog.QueueMode); mode != "" {
		return mode
	}
	return defaultQueueMode(spec.Identity.Preset)
}
