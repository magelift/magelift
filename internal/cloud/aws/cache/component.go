package cache

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	awsprovider "github.com/pulumi/pulumi-aws/sdk/v7/go/aws"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/elasticache"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/secretsmanager"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const ComponentToken = "magelift:aws:Valkey"

type Topology string

const (
	TopologyPreview          Topology = "preview"
	TopologyStandard         Topology = "standard"
	TopologyHighAvailability Topology = "high-availability"
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?$`)
var secretARNPattern = regexp.MustCompile(`^arn:aws:secretsmanager:[a-z0-9-]+:[0-9]{12}:secret:[A-Za-z0-9/_+=.@-]+$`)
var kmsARNPattern = regexp.MustCompile(`^arn:aws:kms:[a-z0-9-]+:[0-9]{12}:key/[A-Za-z0-9-]+$`)
var snapshotWindowPattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]-(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)

type AuthTokens struct {
	CacheSecretARN   string
	SessionSecretARN string
}

type Args struct {
	Topology               Topology
	Region                 string
	EngineVersion          string
	NodeType               string
	ReplicaCount           int
	SnapshotRetentionLimit *int
	SnapshotWindow         string
	SubnetIDs              []string
	SubnetIDInputs         pulumi.StringArrayInput
	SubnetCount            int
	SecurityGroup          string
	SecurityGroupInput     pulumi.StringInput
	KMSKeyARN              string
	AuthTokens             AuthTokens
	// DisableTransitEncryption is an explicit compatibility escape hatch for
	// Magento images whose Valkey adapter cannot express TLS. The cache remains
	// private behind the caller's security groups and is not exposed publicly.
	DisableTransitEncryption bool
	Provider                 *awsprovider.Provider
	Tags                     map[string]string
}

type Component struct {
	pulumi.ResourceState
	CachePrimaryEndpoint   pulumi.StringOutput `pulumi:"cachePrimaryEndpoint"`
	SessionPrimaryEndpoint pulumi.StringOutput `pulumi:"sessionPrimaryEndpoint"`
}

func New(ctx *pulumi.Context, name string, args Args, options ...pulumi.ResourceOption) (*Component, error) {
	if err := validateArgs(name, args); err != nil {
		return nil, err
	}
	component := &Component{}
	if err := ctx.RegisterComponentResource(ComponentToken, name, component, options...); err != nil {
		return nil, err
	}
	childOptions := []pulumi.ResourceOption{pulumi.Parent(component), pulumi.Provider(args.Provider)}
	subnetGroup, err := elasticache.NewSubnetGroup(ctx, name, &elasticache.SubnetGroupArgs{
		Name: pulumi.String(name), Region: pulumi.String(args.Region), SubnetIds: subnetInputs(args), Tags: pulumi.ToStringMap(args.Tags),
	}, childOptions...)
	if err != nil {
		return nil, fmt.Errorf("create Valkey subnet group: %w", err)
	}

	var cacheToken pulumi.StringInput
	if !args.DisableTransitEncryption {
		cacheToken = lookupToken(ctx, args.Region, args.AuthTokens.CacheSecretARN, args.Provider)
	}
	cacheGroup, err := newReplicationGroup(ctx, name+"-cache", "Magento cache", args, subnetGroup.Name, cacheToken, childOptions)
	if err != nil {
		return nil, err
	}
	sessionEndpoint := cacheGroup.PrimaryEndpointAddress
	if args.Topology != TopologyPreview {
		var sessionToken pulumi.StringInput
		if !args.DisableTransitEncryption {
			sessionToken = lookupToken(ctx, args.Region, args.AuthTokens.SessionSecretARN, args.Provider)
		}
		sessionGroup, err := newReplicationGroup(ctx, name+"-sessions", "Magento sessions", args, subnetGroup.Name, sessionToken, childOptions)
		if err != nil {
			return nil, err
		}
		sessionEndpoint = sessionGroup.PrimaryEndpointAddress
	}

	component.CachePrimaryEndpoint = cacheGroup.PrimaryEndpointAddress
	component.SessionPrimaryEndpoint = sessionEndpoint
	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{"cachePrimaryEndpoint": component.CachePrimaryEndpoint, "sessionPrimaryEndpoint": component.SessionPrimaryEndpoint}); err != nil {
		return nil, err
	}
	return component, nil
}

func lookupToken(ctx *pulumi.Context, region, arn string, provider *awsprovider.Provider) pulumi.StringOutput {
	secret := secretsmanager.LookupSecretVersionOutput(ctx, secretsmanager.LookupSecretVersionOutputArgs{
		Region: pulumi.String(region), SecretId: pulumi.String(arn), VersionStage: pulumi.String("AWSCURRENT"),
	}, pulumi.Provider(provider))
	return secret.SecretString()
}

func newReplicationGroup(ctx *pulumi.Context, name, description string, args Args, subnetGroup pulumi.StringInput, token pulumi.StringInput, options []pulumi.ResourceOption) (*elasticache.ReplicationGroup, error) {
	highAvailability := args.Topology != TopologyPreview
	groupArgs := &elasticache.ReplicationGroupArgs{
		ReplicationGroupId: pulumi.String(replicationGroupID(name, args)), Description: pulumi.String(description), Region: pulumi.String(args.Region),
		Engine: pulumi.String("valkey"), EngineVersion: pulumi.String(args.EngineVersion), NodeType: pulumi.String(args.NodeType), NumCacheClusters: pulumi.Int(args.ReplicaCount + 1),
		AutomaticFailoverEnabled: pulumi.Bool(highAvailability), MultiAzEnabled: pulumi.Bool(highAvailability), AutoMinorVersionUpgrade: pulumi.Bool(true),
		SubnetGroupName: subnetGroup, SecurityGroupIds: pulumi.StringArray{securityGroupInput(args)}, Port: pulumi.Int(6379),
		AtRestEncryptionEnabled: pulumi.Bool(true), KmsKeyId: pulumi.String(args.KMSKeyARN), Tags: pulumi.ToStringMap(args.Tags),
	}
	if args.SnapshotRetentionLimit != nil {
		groupArgs.SnapshotRetentionLimit = pulumi.IntPtrFromPtr(args.SnapshotRetentionLimit)
	}
	if args.SnapshotWindow != "" {
		groupArgs.SnapshotWindow = pulumi.String(args.SnapshotWindow)
	}
	if !args.DisableTransitEncryption {
		groupArgs.TransitEncryptionEnabled = pulumi.Bool(true)
		groupArgs.TransitEncryptionMode = pulumi.String("required")
		groupArgs.AuthToken = token
		groupArgs.AuthTokenUpdateStrategy = pulumi.String("ROTATE")
	}
	group, err := elasticache.NewReplicationGroup(ctx, name, groupArgs, options...)
	if err != nil {
		return nil, fmt.Errorf("create %s replication group: %w", description, err)
	}
	return group, nil
}

func replicationGroupID(name string, args Args) string {
	if args.DisableTransitEncryption {
		return name + "-plain"
	}
	return name
}

func validateArgs(name string, args Args) error {
	if strings.TrimSpace(name) == "" || args.Provider == nil || strings.TrimSpace(args.Region) == "" {
		return errors.New("Valkey name, region, and AWS provider are required")
	}
	if !versionPattern.MatchString(args.EngineVersion) || strings.TrimSpace(args.NodeType) == "" {
		return errors.New("Valkey engine version and node type must be provided explicitly")
	}
	if args.SnapshotRetentionLimit != nil && (*args.SnapshotRetentionLimit < 0 || *args.SnapshotRetentionLimit > 35) {
		return errors.New("Valkey snapshot retention must be between 0 and 35 days")
	}
	if args.SnapshotWindow != "" {
		if !snapshotWindowPattern.MatchString(args.SnapshotWindow) {
			return errors.New("Valkey snapshot window must use HH:MM-HH:MM in UTC")
		}
		if args.SnapshotRetentionLimit == nil || *args.SnapshotRetentionLimit == 0 {
			return errors.New("Valkey snapshot window requires positive snapshot retention")
		}
	}
	if !validSubnets(args) || !validSecurityGroup(args) || !kmsARNPattern.MatchString(args.KMSKeyARN) {
		return errors.New("Valkey private subnet, security group, and KMS inputs are invalid")
	}
	if !args.DisableTransitEncryption && !secretARNPattern.MatchString(args.AuthTokens.CacheSecretARN) {
		return errors.New("Valkey cache auth token must be a Secrets Manager ARN")
	}
	switch args.Topology {
	case TopologyPreview:
		if args.ReplicaCount < 0 || args.ReplicaCount > 1 || args.AuthTokens.SessionSecretARN != "" {
			return errors.New("preview Valkey must use one bounded group with at most one replica")
		}
	case TopologyStandard:
		if args.ReplicaCount < 1 || (!args.DisableTransitEncryption && !validDistinctSessionToken(args.AuthTokens)) {
			return errors.New("standard Valkey requires separate cache and session groups with at least one replica")
		}
	case TopologyHighAvailability:
		if args.ReplicaCount < 2 || (!args.DisableTransitEncryption && !validDistinctSessionToken(args.AuthTokens)) {
			return errors.New("high-availability Valkey requires separate groups with at least two replicas")
		}
	default:
		return errors.New("Valkey topology is invalid")
	}
	return nil
}

func subnetInputs(args Args) pulumi.StringArrayInput {
	if args.SubnetIDInputs != nil {
		return args.SubnetIDInputs
	}
	return pulumi.ToStringArray(args.SubnetIDs)
}

func validSubnets(args Args) bool {
	if args.SubnetIDInputs != nil {
		return args.SubnetCount >= 2
	}
	return len(args.SubnetIDs) >= 2 && !hasBlankOrDuplicate(args.SubnetIDs)
}

func securityGroupInput(args Args) pulumi.StringInput {
	if args.SecurityGroupInput != nil {
		return args.SecurityGroupInput
	}
	return pulumi.String(args.SecurityGroup)
}

func validSecurityGroup(args Args) bool {
	return args.SecurityGroupInput != nil || strings.TrimSpace(args.SecurityGroup) != ""
}

func validDistinctSessionToken(tokens AuthTokens) bool {
	return secretARNPattern.MatchString(tokens.SessionSecretARN) && tokens.SessionSecretARN != tokens.CacheSecretARN
}

func hasBlankOrDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, found := seen[value]; found {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
