package database

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/rds"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:aws:AuroraMysql"

var kmsARN = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[0-9a-fA-F-]+$`)
var databaseName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var username = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,15}$`)

type ServerlessV2 struct {
	MinimumACU              float64
	MaximumACU              float64
	AutoPauseSeconds        int
	EngineSupportsAutoPause bool
}

type Args struct {
	Preset                   sdk.PresetID
	EnvironmentClass         string
	Region                   string
	AvailabilityZones        []string
	DataSubnetIDs            pulumi.StringArray
	VpcSecurityGroupIDs      pulumi.StringArray
	EngineVersion            string
	DatabaseName             string
	MasterUsername           string
	KMSKeyARN                string
	BackupRetentionDays      int
	FinalSnapshotIdentifier  string
	ProvisionedInstanceClass string
	InstanceCount            int
	ServerlessV2             *ServerlessV2
	Tags                     map[string]string
}

type Component struct {
	pulumi.ResourceState
	ClusterARN      pulumi.StringOutput
	WriterEndpoint  pulumi.StringOutput
	ReaderEndpoint  pulumi.StringOutput
	MasterSecretARN pulumi.StringPtrOutput
	InstanceIDs     []pulumi.IDOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"preset": pulumi.String(args.Preset), "environmentClass": pulumi.String(args.EnvironmentClass), "region": pulumi.String(args.Region),
		"engineVersion": pulumi.String(args.EngineVersion), "backupRetentionDays": pulumi.Int(args.BackupRetentionDays), "dataSubnetIds": args.DataSubnetIDs,
	}, component, opts...); err != nil {
		return nil, err
	}
	child := pulumi.Parent(component)
	subnets, err := rds.NewSubnetGroup(ctx, name+"-subnets", &rds.SubnetGroupArgs{
		SubnetIds: args.DataSubnetIDs, Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "subnet-group"),
	}, child)
	if err != nil {
		return nil, err
	}
	production := args.EnvironmentClass == "production"
	clusterArgs := &rds.ClusterArgs{
		Engine: pulumi.String("aurora-mysql"), EngineMode: pulumi.String("provisioned"), EngineVersion: pulumi.String(args.EngineVersion),
		DatabaseName: pulumi.String(args.DatabaseName), MasterUsername: pulumi.String(args.MasterUsername), ManageMasterUserPassword: pulumi.Bool(true),
		MasterUserSecretKmsKeyId: pulumi.String(args.KMSKeyARN), StorageEncrypted: pulumi.Bool(true), KmsKeyId: pulumi.String(args.KMSKeyARN),
		DbSubnetGroupName: subnets.Name, VpcSecurityGroupIds: args.VpcSecurityGroupIDs, BackupRetentionPeriod: pulumi.Int(args.BackupRetentionDays),
		DeletionProtection: pulumi.Bool(production), SkipFinalSnapshot: pulumi.Bool(!production), DeleteAutomatedBackups: pulumi.Bool(!production),
		CopyTagsToSnapshot: pulumi.Bool(true), EnabledCloudwatchLogsExports: pulumi.StringArray{pulumi.String("error"), pulumi.String("slowquery")},
		Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "cluster"),
	}
	if production {
		clusterArgs.FinalSnapshotIdentifier = pulumi.String(args.FinalSnapshotIdentifier)
	}
	instanceClass := args.ProvisionedInstanceClass
	instanceCount := args.InstanceCount
	if args.Preset == sdk.PresetPreview {
		instanceClass, instanceCount = "db.serverless", 1
		clusterArgs.Serverlessv2ScalingConfiguration = &rds.ClusterServerlessv2ScalingConfigurationArgs{
			MinCapacity: pulumi.Float64(args.ServerlessV2.MinimumACU), MaxCapacity: pulumi.Float64(args.ServerlessV2.MaximumACU),
		}
		if args.ServerlessV2.AutoPauseSeconds != 0 {
			clusterArgs.Serverlessv2ScalingConfiguration = &rds.ClusterServerlessv2ScalingConfigurationArgs{MinCapacity: pulumi.Float64(args.ServerlessV2.MinimumACU), MaxCapacity: pulumi.Float64(args.ServerlessV2.MaximumACU), SecondsUntilAutoPause: pulumi.Int(args.ServerlessV2.AutoPauseSeconds)}
		}
	}
	cluster, err := rds.NewCluster(ctx, name+"-cluster", clusterArgs, child)
	if err != nil {
		return nil, err
	}
	component.ClusterARN, component.WriterEndpoint, component.ReaderEndpoint = cluster.Arn, cluster.Endpoint, cluster.ReaderEndpoint
	component.MasterSecretARN = cluster.MasterUserSecrets.Index(pulumi.Int(0)).SecretArn()
	for index := 0; index < instanceCount; index++ {
		suffix := fmt.Sprintf("%02d", index+1)
		instanceArgs := &rds.ClusterInstanceArgs{
			ClusterIdentifier: cluster.ID(), InstanceClass: pulumi.String(instanceClass), Engine: rds.EngineTypeAuroraMysql,
			DbSubnetGroupName: subnets.Name, AvailabilityZone: pulumi.String(args.AvailabilityZones[index%len(args.AvailabilityZones)]), PubliclyAccessible: pulumi.Bool(false),
			PerformanceInsightsEnabled: pulumi.Bool(production),
			PromotionTier:              pulumi.Int(index), Region: pulumi.String(args.Region), Tags: tags(args.Tags, name, "instance-"+suffix),
		}
		if production {
			// Keep the retention and encryption policy explicit for production. Seven
			// days is the minimum supported Performance Insights retention period.
			instanceArgs.PerformanceInsightsKmsKeyId = pulumi.String(args.KMSKeyARN)
			instanceArgs.PerformanceInsightsRetentionPeriod = pulumi.Int(7)
		}
		instance, err := rds.NewClusterInstance(ctx, name+"-instance-"+suffix, instanceArgs, child)
		if err != nil {
			return nil, err
		}
		component.InstanceIDs = append(component.InstanceIDs, instance.ID())
	}
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"clusterArn": cluster.Arn, "writerEndpoint": cluster.Endpoint, "readerEndpoint": cluster.ReaderEndpoint,
		"masterSecretArn": component.MasterSecretARN, "instanceIds": idArray(component.InstanceIDs),
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func validate(name string, args Args) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(args.Region) == "" || strings.TrimSpace(args.EngineVersion) == "" {
		return errors.New("database name, region, and Aurora MySQL engine version are required")
	}
	if !databaseName.MatchString(args.DatabaseName) || !username.MatchString(args.MasterUsername) {
		return errors.New("database and master user names are invalid")
	}
	if !kmsARN.MatchString(args.KMSKeyARN) {
		return errors.New("database storage and managed credentials require a KMS key ARN")
	}
	if args.BackupRetentionDays < 1 || args.BackupRetentionDays > 35 {
		return errors.New("backup retention must be between 1 and 35 days to provide PITR")
	}
	wantZones := map[sdk.PresetID]int{sdk.PresetPreview: 2, sdk.PresetStandard: 2, sdk.PresetHighAvailability: 3}[args.Preset]
	if wantZones == 0 || len(args.AvailabilityZones) != wantZones || len(args.DataSubnetIDs) != wantZones {
		return fmt.Errorf("preset %q requires exactly %d database availability zones and data subnets", args.Preset, wantZones)
	}
	seen := map[string]bool{}
	for _, zone := range args.AvailabilityZones {
		if strings.TrimSpace(zone) == "" || seen[zone] {
			return errors.New("database availability zones must be non-empty and unique")
		}
		seen[zone] = true
	}
	if len(args.VpcSecurityGroupIDs) == 0 {
		return errors.New("database VPC security groups are required")
	}
	production := args.EnvironmentClass == "production"
	if production && args.Preset == sdk.PresetPreview {
		return errors.New("production databases cannot use the preview preset")
	}
	if production && strings.TrimSpace(args.FinalSnapshotIdentifier) == "" {
		return errors.New("production databases require a final snapshot identifier")
	}
	if args.Preset == sdk.PresetPreview {
		if args.ServerlessV2 == nil || args.InstanceCount != 0 || args.ProvisionedInstanceClass != "" {
			return errors.New("preview requires bounded Serverless v2 settings and no provisioned capacity")
		}
		settings := args.ServerlessV2
		if settings.MinimumACU < 0 || settings.MaximumACU <= 0 || settings.MaximumACU > 256 || settings.MaximumACU < settings.MinimumACU || !halfStep(settings.MinimumACU) || !halfStep(settings.MaximumACU) {
			return errors.New("Serverless v2 capacity bounds must be non-negative half-ACU increments with maximum at least minimum")
		}
		if settings.AutoPauseSeconds != 0 && (!settings.EngineSupportsAutoPause || settings.MinimumACU != 0 || settings.AutoPauseSeconds < 300 || settings.AutoPauseSeconds > 86400) {
			return errors.New("Serverless v2 auto-pause requires engine support, zero minimum ACU, and 300 to 86400 seconds")
		}
	} else {
		minimumInstances := 2
		if args.Preset == sdk.PresetHighAvailability {
			minimumInstances = 3
		}
		if strings.TrimSpace(args.ProvisionedInstanceClass) == "" || args.InstanceCount < minimumInstances || args.ServerlessV2 != nil {
			return fmt.Errorf("preset %q requires a caller-selected instance class and at least %d instances", args.Preset, minimumInstances)
		}
	}
	return nil
}

func halfStep(value float64) bool { return value*2 == float64(int(value*2)) }
func tags(input map[string]string, component, role string) pulumi.StringMap {
	result := pulumi.StringMap{}
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = pulumi.String(input[key])
	}
	result["Name"] = pulumi.String(component + "-" + role)
	result["magelift:component"] = pulumi.String(component)
	result["magelift:role"] = pulumi.String(role)
	return result
}
func idArray(ids []pulumi.IDOutput) pulumi.Array {
	result := make(pulumi.Array, len(ids))
	for index := range ids {
		result[index] = ids[index]
	}
	return result
}
