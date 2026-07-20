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

type Args struct {
	Project         string
	Region          string
	NetworkID       pulumi.StringInput
	NetworkSelfLink pulumi.StringInput
	DatabaseName    string
	MasterUsername  string
	Tier            string
	AvailabilityType string // ZONAL | REGIONAL
	Labels          map[string]string
}

type Component struct {
	pulumi.ResourceState
	WriterEndpoint   pulumi.StringOutput
	DatabaseName     pulumi.StringOutput
	ConnectionName   pulumi.StringOutput
	PasswordSecretID pulumi.StringOutput
	InstanceName     pulumi.StringOutput
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
	if strings.TrimSpace(args.Tier) == "" {
		args.Tier = "db-custom-1-3840"
	}
	availability := args.AvailabilityType
	if availability == "" {
		availability = "ZONAL"
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

	instance, err := sql.NewDatabaseInstance(ctx, name, &sql.DatabaseInstanceArgs{
		Project:         pulumi.String(args.Project),
		Name:            pulumi.String(name),
		DatabaseVersion: pulumi.String("MYSQL_8_0"),
		Region:          pulumi.String(args.Region),
		Settings: &sql.DatabaseInstanceSettingsArgs{
			Tier:             pulumi.String(args.Tier),
			AvailabilityType: pulumi.String(availability),
			BackupConfiguration: &sql.DatabaseInstanceSettingsBackupConfigurationArgs{
				Enabled: pulumi.Bool(availability == "REGIONAL"),
			},
			IpConfiguration: &sql.DatabaseInstanceSettingsIpConfigurationArgs{
				Ipv4Enabled:    pulumi.Bool(false),
				PrivateNetwork: args.NetworkSelfLink,
			},
			UserLabels: pulumi.ToStringMap(args.Labels),
		},
		DeletionProtection: pulumi.Bool(false),
	}, parent, pulumi.DependsOn([]pulumi.Resource{connection}))
	if err != nil {
		return nil, fmt.Errorf("create Cloud SQL instance: %w", err)
	}
	_, err = sql.NewDatabase(ctx, name+"-db", &sql.DatabaseArgs{
		Project:  pulumi.String(args.Project),
		Name:     pulumi.String(args.DatabaseName),
		Instance: instance.Name,
	}, parent, pulumi.DependsOn([]pulumi.Resource{instance}))
	if err != nil {
		return nil, fmt.Errorf("create Magento database: %w", err)
	}
	_, err = sql.NewUser(ctx, name+"-user", &sql.UserArgs{
		Project:  pulumi.String(args.Project),
		Name:     pulumi.String(args.MasterUsername),
		Instance: instance.Name,
		Password: password.Result,
		Host:     pulumi.String("%"),
	}, parent, pulumi.DependsOn([]pulumi.Resource{instance, password}))
	if err != nil {
		return nil, fmt.Errorf("create database user: %w", err)
	}

	component.WriterEndpoint = instance.PrivateIpAddress
	component.DatabaseName = pulumi.String(args.DatabaseName).ToStringOutput()
	component.ConnectionName = instance.ConnectionName
	component.PasswordSecretID = secret.SecretId
	component.InstanceName = instance.Name
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"writerEndpoint": component.WriterEndpoint, "databaseName": component.DatabaseName,
		"passwordSecretId": component.PasswordSecretID,
	}); err != nil {
		return nil, err
	}
	return component, nil
}
