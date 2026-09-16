package database

import (
	"errors"
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/compute"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/secretmanager"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/servicenetworking"
	"github.com/pulumi/pulumi-gcp/sdk/v9/go/gcp/sql"
	"github.com/pulumi/pulumi-random/sdk/v4/go/random"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const TypeToken = "magelift:gcp:CloudSQL"

const AdminUsername = "magelift-admin"

const logBinTrustFunctionCreatorsFlag = "log_bin_trust_function_creators"

type Args struct {
	Project                 string
	Region                  string
	NetworkID               pulumi.StringInput
	NetworkSelfLink         pulumi.StringInput
	DatabaseName            string
	MasterUsername          string
	DatabaseVersion         string // MYSQL_8_0 | MYSQL_8_4
	Tier                    string
	AvailabilityType        string // ZONAL | REGIONAL
	BackupEnabled           bool
	BinaryLogEnabled        bool
	BackupRetentionCount    int
	TransactionLogRetention int
	BackupStartTime         string
	BackupLocation          string
	DeletionProtection      bool
	Labels                  map[string]string
}

type Component struct {
	pulumi.ResourceState
	WriterEndpoint   pulumi.StringOutput
	DatabaseName     pulumi.StringOutput
	ConnectionName   pulumi.StringOutput
	Password         pulumi.StringOutput
	AdminPassword    pulumi.StringOutput
	PasswordSecretID pulumi.StringOutput
	InstanceName     pulumi.StringOutput
	UserReady        pulumi.Resource
	AdminReady       pulumi.Resource
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("database name is required")
	}
	if strings.TrimSpace(args.Project) == "" || strings.TrimSpace(args.Region) == "" {
		return nil, errors.New("GCP project and region are required")
	}
	if strings.TrimSpace(args.DatabaseName) == "" || strings.TrimSpace(args.MasterUsername) == "" {
		return nil, errors.New("database name and master username are required")
	}
	databaseVersion := strings.TrimSpace(args.DatabaseVersion)
	if !isSupportedDatabaseVersion(databaseVersion) {
		return nil, fmt.Errorf("unsupported Cloud SQL database version %q", args.DatabaseVersion)
	}
	if strings.TrimSpace(args.Tier) == "" {
		args.Tier = DefaultTier(databaseVersion, "preview")
	}
	if err := ValidateTier(databaseVersion, args.Tier); err != nil {
		return nil, err
	}
	edition, err := EditionForDatabaseVersion(databaseVersion)
	if err != nil {
		return nil, err
	}
	availability := args.AvailabilityType
	if availability == "" {
		availability = "ZONAL"
	}
	backupEnabled := args.BackupEnabled
	binaryLogEnabled := args.BinaryLogEnabled
	// REGIONAL MySQL requires automated backups and binary logging. The config
	// layer rejects an explicit false value; this fallback also keeps directly
	// constructed provider specs safe.
	if availability == "REGIONAL" {
		backupEnabled = true
		binaryLogEnabled = true
	}
	if args.BackupRetentionCount < 0 || args.TransactionLogRetention < 0 {
		return nil, errors.New("Cloud SQL backup retention settings cannot be negative")
	}
	if args.TransactionLogRetention > 7 {
		return nil, errors.New("Cloud SQL transaction log retention must be between 1 and 7 days")
	}
	if args.BackupRetentionCount > 0 && args.TransactionLogRetention > args.BackupRetentionCount {
		return nil, errors.New("Cloud SQL transaction log retention cannot exceed retained automated backups")
	}
	if !backupEnabled && (args.BackupRetentionCount > 0 || args.TransactionLogRetention > 0 || args.BackupStartTime != "" || args.BackupLocation != "") {
		return nil, errors.New("Cloud SQL backup retention, schedule, and location settings require automated backups")
	}
	if !binaryLogEnabled && args.TransactionLogRetention > 0 {
		return nil, errors.New("Cloud SQL transaction log retention requires binary logging")
	}
	if args.BackupStartTime != "" && (len(args.BackupStartTime) != 5 || args.BackupStartTime[2] != ':') {
		return nil, errors.New("Cloud SQL backup start time must use HH:MM")
	}
	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"project": pulumi.String(args.Project),
	}, component, opts...); err != nil {
		return nil, err
	}
	parent := pulumi.Parent(component)

	address, err := compute.NewGlobalAddress(ctx, name+"-psa", &compute.GlobalAddressArgs{
		Project:      pulumi.String(args.Project),
		Name:         pulumi.String(name + "-psa"),
		Purpose:      pulumi.String("VPC_PEERING"),
		AddressType:  pulumi.String("INTERNAL"),
		PrefixLength: pulumi.Int(16),
		Network:      args.NetworkID,
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create private service access address: %w", err)
	}
	connection, err := servicenetworking.NewConnection(ctx, name+"-psa", &servicenetworking.ConnectionArgs{
		Network: args.NetworkSelfLink,
		Service: pulumi.String("servicenetworking.googleapis.com"),
		ReservedPeeringRanges: pulumi.StringArray{
			address.Name,
		},
	}, parent, pulumi.DependsOn([]pulumi.Resource{address}),
		// Cloud SQL can keep the producer allocation briefly after instance delete;
		// give the provider time before surfacing FLOW_SN_DC_* to the stack.
		pulumi.Timeouts(&pulumi.CustomTimeouts{Delete: "45m"}))
	if err != nil {
		return nil, fmt.Errorf("create private service connection: %w", err)
	}

	password, err := random.NewRandomPassword(ctx, name+"-password", &random.RandomPasswordArgs{
		Length:          pulumi.Int(32),
		Special:         pulumi.Bool(false),
		OverrideSpecial: pulumi.String(""),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate database password: %w", err)
	}
	adminPassword, err := random.NewRandomPassword(ctx, name+"-admin-password", &random.RandomPasswordArgs{
		Length:          pulumi.Int(32),
		Special:         pulumi.Bool(false),
		OverrideSpecial: pulumi.String(""),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("generate Cloud SQL admin password: %w", err)
	}

	secret, err := secretmanager.NewSecret(ctx, name+"-db-secret", &secretmanager.SecretArgs{
		Project:  pulumi.String(args.Project),
		SecretId: pulumi.String(name + "-db"),
		Replication: &secretmanager.SecretReplicationArgs{
			Auto: &secretmanager.SecretReplicationAutoArgs{},
		},
		Labels: pulumi.ToStringMap(args.Labels),
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create database password secret: %w", err)
	}
	_, err = secretmanager.NewSecretVersion(ctx, name+"-db-secret-version", &secretmanager.SecretVersionArgs{
		Secret:     secret.ID(),
		SecretData: password.Result,
	}, parent, pulumi.DependsOn([]pulumi.Resource{secret, password}))
	if err != nil {
		return nil, fmt.Errorf("create database password secret version: %w", err)
	}
	backupConfiguration := &sql.DatabaseInstanceSettingsBackupConfigurationArgs{
		Enabled:          pulumi.Bool(backupEnabled),
		BinaryLogEnabled: pulumi.Bool(binaryLogEnabled),
	}
	if backupEnabled && args.BackupStartTime != "" {
		backupConfiguration.StartTime = pulumi.StringPtr(args.BackupStartTime)
	}
	if backupEnabled && args.BackupLocation != "" {
		backupConfiguration.Location = pulumi.StringPtr(args.BackupLocation)
	}
	if binaryLogEnabled && args.TransactionLogRetention > 0 {
		backupConfiguration.TransactionLogRetentionDays = pulumi.IntPtr(args.TransactionLogRetention)
	}
	if backupEnabled && args.BackupRetentionCount > 0 {
		backupConfiguration.BackupRetentionSettings = &sql.DatabaseInstanceSettingsBackupConfigurationBackupRetentionSettingsArgs{
			RetainedBackups: pulumi.Int(args.BackupRetentionCount),
			RetentionUnit:   pulumi.StringPtr("COUNT"),
		}
	}

	var finalBackup *sql.DatabaseInstanceSettingsFinalBackupConfigArgs
	if days := finalBackupRetentionDays(backupEnabled, args.TransactionLogRetention, args.BackupRetentionCount); days > 0 {
		finalBackup = &sql.DatabaseInstanceSettingsFinalBackupConfigArgs{
			Enabled:       pulumi.BoolPtr(true),
			RetentionDays: pulumi.IntPtr(days),
		}
	}

	instance, err := sql.NewDatabaseInstance(ctx, name, &sql.DatabaseInstanceArgs{
		Project:         pulumi.String(args.Project),
		Name:            pulumi.String(name),
		DatabaseVersion: pulumi.String(databaseVersion),
		Region:          pulumi.String(args.Region),
		Settings: &sql.DatabaseInstanceSettingsArgs{
			Tier:             pulumi.String(args.Tier),
			Edition:          pulumi.String(edition),
			AvailabilityType: pulumi.String(availability),
			DatabaseFlags: sql.DatabaseInstanceSettingsDatabaseFlagArray{
				&sql.DatabaseInstanceSettingsDatabaseFlagArgs{
					Name:  pulumi.String(logBinTrustFunctionCreatorsFlag),
					Value: pulumi.String("on"),
				},
			},
			BackupConfiguration: backupConfiguration,
			FinalBackupConfig:   finalBackup,
			IpConfiguration: &sql.DatabaseInstanceSettingsIpConfigurationArgs{
				Ipv4Enabled:    pulumi.Bool(false),
				PrivateNetwork: args.NetworkSelfLink,
			},
			UserLabels:                pulumi.ToStringMap(args.Labels),
			DeletionProtectionEnabled: pulumi.BoolPtr(args.DeletionProtection),
		},
		DeletionProtection: pulumi.Bool(args.DeletionProtection),
	}, parent, pulumi.DependsOn([]pulumi.Resource{connection}))
	if err != nil {
		return nil, fmt.Errorf("create Cloud SQL instance: %w", err)
	}
	database, err := sql.NewDatabase(ctx, name+"-db", &sql.DatabaseArgs{
		Project:  pulumi.String(args.Project),
		Name:     pulumi.String(args.DatabaseName),
		Instance: instance.Name,
	}, parent, pulumi.DependsOn([]pulumi.Resource{instance}))
	if err != nil {
		return nil, fmt.Errorf("create Magento database: %w", err)
	}
	user, err := sql.NewUser(ctx, name+"-user", &sql.UserArgs{
		Project:  pulumi.String(args.Project),
		Name:     pulumi.String(args.MasterUsername),
		Instance: instance.Name,
		Password: password.Result,
		Host:     pulumi.String("%"),
	}, parent, pulumi.DependsOn([]pulumi.Resource{instance, database, password}))
	if err != nil {
		return nil, fmt.Errorf("create database user: %w", err)
	}
	adminUser, err := sql.NewUser(ctx, name+"-admin-user", &sql.UserArgs{
		Project:  pulumi.String(args.Project),
		Name:     pulumi.String(AdminUsername),
		Instance: instance.Name,
		Password: adminPassword.Result,
		Host:     pulumi.String("%"),
	}, parent, pulumi.DependsOn([]pulumi.Resource{instance, database, adminPassword}))
	if err != nil {
		return nil, fmt.Errorf("create database admin user: %w", err)
	}

	component.WriterEndpoint = instance.PrivateIpAddress
	component.DatabaseName = pulumi.String(args.DatabaseName).ToStringOutput()
	component.ConnectionName = instance.ConnectionName
	component.Password = pulumi.ToSecret(password.Result).(pulumi.StringOutput)
	component.AdminPassword = pulumi.ToSecret(adminPassword.Result).(pulumi.StringOutput)
	component.PasswordSecretID = secret.SecretId
	component.InstanceName = instance.Name
	component.UserReady = user
	component.AdminReady = adminUser
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"writerEndpoint": component.WriterEndpoint, "databaseName": component.DatabaseName,
		"passwordSecretId": component.PasswordSecretID,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

// finalBackupRetentionDays is the Cloud SQL final-backup window (1–365) used
// when automated backups are on, so instance destroy keeps a backup inside the
// configured retention instead of deleting it. Preview cells with backups off
// return 0 and omit FinalBackupConfig.
func finalBackupRetentionDays(backupEnabled bool, transactionLogRetention, backupRetentionCount int) int {
	if !backupEnabled {
		return 0
	}
	days := transactionLogRetention
	if backupRetentionCount > days {
		days = backupRetentionCount
	}
	if days < 1 {
		days = 7
	}
	if days > 365 {
		return 365
	}
	return days
}
