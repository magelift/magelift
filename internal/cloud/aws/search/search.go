package search

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	sdk "github.com/acourtiol/magelift/sdk/v1"
	"github.com/pulumi/pulumi-aws/sdk/v7/go/aws/opensearch"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	TypeToken         = "magelift:aws:OpenSearch"
	minimumTLSPolicy  = "Policy-Min-TLS-1-2-PFS-2023-10"
	serverlessClassic = "CLASSIC"
)

var (
	resourceName = regexp.MustCompile(`^[a-z][a-z0-9-]{1,24}[a-z0-9]$`)
	kmsARN       = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):kms:[a-z0-9-]+:[0-9]{12}:key/[0-9a-fA-F-]+$`)
	identityARN  = regexp.MustCompile(`^arn:(?:aws|aws-us-gov|aws-cn):iam::[0-9]{12}:(?:role|user)/[A-Za-z0-9+=,.@_/-]+$`)
)

type ServerlessCapacity struct {
	MinimumIndexingOCU float64
	MaximumIndexingOCU float64
	MinimumSearchOCU   float64
	MaximumSearchOCU   float64
}

type Serverless struct {
	AcceptColdStarts bool
	Capacity         ServerlessCapacity
}

type Provisioned struct {
	EngineVersion        string
	InstanceType         string
	InstanceCount        int
	DedicatedMasterType  string
	DedicatedMasterCount int
	EBSVolumeType        string
	EBSVolumeSizeGiB     int
}

type Args struct {
	Preset              sdk.PresetID
	Region              string
	VPCID               pulumi.StringInput
	SubnetIDs           pulumi.StringArray
	SecurityGroupIDs    pulumi.StringArray
	KMSKeyARN           string
	AccessIdentityARN   string
	AccessIdentityInput pulumi.StringInput
	Serverless          *Serverless
	Provisioned         *Provisioned
	Tags                map[string]string
}

type Component struct {
	pulumi.ResourceState
	ARN               pulumi.StringOutput
	Endpoint          pulumi.StringOutput
	DashboardEndpoint pulumi.StringOutput
}

func New(ctx *pulumi.Context, name string, args Args, opts ...pulumi.ResourceOption) (*Component, error) {
	if err := validate(name, args); err != nil {
		return nil, err
	}

	component := &Component{}
	if err := ctx.RegisterComponentResourceV2(TypeToken, name, pulumi.Map{
		"preset": pulumi.String(args.Preset), "region": pulumi.String(args.Region), "subnetIds": args.SubnetIDs,
	}, component, opts...); err != nil {
		return nil, err
	}

	if args.Preset == sdk.PresetPreview {
		if err := createServerless(ctx, name, args, component); err != nil {
			return nil, err
		}
	} else if err := createProvisioned(ctx, name, args, component); err != nil {
		return nil, err
	}

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{
		"arn": component.ARN, "endpoint": component.Endpoint, "dashboardEndpoint": component.DashboardEndpoint,
	}); err != nil {
		return nil, err
	}
	return component, nil
}

func createServerless(ctx *pulumi.Context, name string, args Args, component *Component) error {
	child := pulumi.Parent(component)
	capacity := args.Serverless.Capacity
	group, err := opensearch.NewServerlessCollectionGroup(ctx, name+"-group", &opensearch.ServerlessCollectionGroupArgs{
		Name: pulumi.String(name + "-group"), Region: pulumi.String(args.Region), Generation: pulumi.String(serverlessClassic),
		StandbyReplicas: pulumi.String("DISABLED"), Tags: tags(args.Tags, name, "collection-group"),
		CapacityLimits: opensearch.ServerlessCollectionGroupCapacityLimitArray{
			&opensearch.ServerlessCollectionGroupCapacityLimitArgs{
				MinIndexingCapacityInOcu: pulumi.Float64(capacity.MinimumIndexingOCU), MaxIndexingCapacityInOcu: pulumi.Float64(capacity.MaximumIndexingOCU),
				MinSearchCapacityInOcu: pulumi.Float64(capacity.MinimumSearchOCU), MaxSearchCapacityInOcu: pulumi.Float64(capacity.MaximumSearchOCU),
			},
		},
	}, child)
	if err != nil {
		return err
	}
	endpoint, err := opensearch.NewServerlessVpcEndpoint(ctx, name+"-endpoint", &opensearch.ServerlessVpcEndpointArgs{
		Name: pulumi.String(name + "-endpoint"), Region: pulumi.String(args.Region), VpcId: args.VPCID,
		SubnetIds: args.SubnetIDs, SecurityGroupIds: args.SecurityGroupIDs,
	}, child)
	if err != nil {
		return err
	}
	networkPolicy := endpoint.ID().ApplyT(func(endpointID pulumi.ID) (string, error) {
		return marshalPolicy([]networkPolicyDocument{{
			Description: "Private collection access", AllowFromPublic: false, SourceVPCEs: []string{string(endpointID)},
			Rules: []networkRule{
				{ResourceType: "collection", Resource: []string{"collection/" + name}},
				{ResourceType: "dashboard", Resource: []string{"collection/" + name}},
			},
		}})
	}).(pulumi.StringOutput)
	network, err := opensearch.NewServerlessSecurityPolicy(ctx, name+"-network", &opensearch.ServerlessSecurityPolicyArgs{
		Name: pulumi.String(name + "-network"), Region: pulumi.String(args.Region), Type: pulumi.String("network"), Policy: networkPolicy,
	}, child)
	if err != nil {
		return err
	}
	accessJSON := identityInput(args).ApplyT(func(identity string) (string, error) {
		return marshalPolicy([]accessPolicyDocument{{
			Description: "Magento search access", Principal: []string{identity},
			Rules: []accessRule{
				{ResourceType: "collection", Resource: []string{"collection/" + name}, Permission: []string{"aoss:DescribeCollectionItems"}},
				{ResourceType: "index", Resource: []string{"index/" + name + "/*"}, Permission: []string{"aoss:CreateIndex", "aoss:DeleteIndex", "aoss:UpdateIndex", "aoss:DescribeIndex", "aoss:ReadDocument", "aoss:WriteDocument"}},
			},
		}})
	}).(pulumi.StringOutput)
	access, err := opensearch.NewServerlessAccessPolicy(ctx, name+"-access", &opensearch.ServerlessAccessPolicyArgs{
		Name: pulumi.String(name + "-access"), Region: pulumi.String(args.Region), Type: pulumi.String("data"), Policy: accessJSON,
	}, child)
	if err != nil {
		return err
	}
	collection, err := opensearch.NewServerlessCollection(ctx, name, &opensearch.ServerlessCollectionArgs{
		Name: pulumi.String(name), Region: pulumi.String(args.Region), Type: pulumi.String("SEARCH"),
		CollectionGroupName: group.Name, StandbyReplicas: pulumi.String("DISABLED"), Tags: tags(args.Tags, name, "collection"),
		EncryptionConfigs: opensearch.ServerlessCollectionEncryptionConfigArray{
			&opensearch.ServerlessCollectionEncryptionConfigArgs{KmsKeyArn: pulumi.String(args.KMSKeyARN)},
		},
	}, child, pulumi.DependsOn([]pulumi.Resource{network, access}))
	if err != nil {
		return err
	}
	component.ARN, component.Endpoint, component.DashboardEndpoint = collection.Arn, collection.CollectionEndpoint, collection.DashboardEndpoint
	return nil
}

func createProvisioned(ctx *pulumi.Context, name string, args Args, component *Component) error {
	provisioned := args.Provisioned
	zones := len(args.SubnetIDs)
	accessPolicy := provisionedAccessPolicyInput(name, args.Region, identityInput(args))
	clusterConfig := &opensearch.DomainClusterConfigArgs{
		InstanceType: pulumi.String(provisioned.InstanceType), InstanceCount: pulumi.Int(provisioned.InstanceCount),
		ZoneAwarenessEnabled: pulumi.Bool(true), MultiAzWithStandbyEnabled: pulumi.Bool(args.Preset == sdk.PresetHighAvailability),
		ZoneAwarenessConfig: &opensearch.DomainClusterConfigZoneAwarenessConfigArgs{AvailabilityZoneCount: pulumi.Int(zones)},
	}
	if args.Preset == sdk.PresetHighAvailability {
		clusterConfig.DedicatedMasterEnabled = pulumi.Bool(true)
		clusterConfig.DedicatedMasterType = pulumi.String(provisioned.DedicatedMasterType)
		clusterConfig.DedicatedMasterCount = pulumi.Int(provisioned.DedicatedMasterCount)
	}
	domain, err := opensearch.NewDomain(ctx, name, &opensearch.DomainArgs{
		DomainName: pulumi.String(name), Region: pulumi.String(args.Region), EngineVersion: pulumi.String(provisioned.EngineVersion),
		AccessPolicies:        accessPolicy,
		ClusterConfig:         clusterConfig,
		EbsOptions:            &opensearch.DomainEbsOptionsArgs{EbsEnabled: pulumi.Bool(true), VolumeType: pulumi.String(provisioned.EBSVolumeType), VolumeSize: pulumi.Int(provisioned.EBSVolumeSizeGiB)},
		EncryptAtRest:         &opensearch.DomainEncryptAtRestArgs{Enabled: pulumi.Bool(true), KmsKeyId: pulumi.String(args.KMSKeyARN)},
		NodeToNodeEncryption:  &opensearch.DomainNodeToNodeEncryptionArgs{Enabled: pulumi.Bool(true)},
		DomainEndpointOptions: &opensearch.DomainDomainEndpointOptionsArgs{EnforceHttps: pulumi.Bool(true), TlsSecurityPolicy: pulumi.String(minimumTLSPolicy)},
		AdvancedSecurityOptions: &opensearch.DomainAdvancedSecurityOptionsArgs{
			Enabled: pulumi.Bool(true), InternalUserDatabaseEnabled: pulumi.Bool(false),
			MasterUserOptions: &opensearch.DomainAdvancedSecurityOptionsMasterUserOptionsArgs{MasterUserArn: identityInput(args)},
		},
		VpcOptions: &opensearch.DomainVpcOptionsArgs{SubnetIds: args.SubnetIDs, SecurityGroupIds: args.SecurityGroupIDs, VpcId: args.VPCID},
		Tags:       tags(args.Tags, name, "domain"),
	}, pulumi.Parent(component))
	if err != nil {
		return err
	}
	component.ARN, component.Endpoint = domain.Arn, domain.Endpoint
	component.DashboardEndpoint = domain.DashboardEndpoint
	return nil
}

func validate(name string, args Args) error {
	if !resourceName.MatchString(name) {
		return errors.New("search name must be 3 to 26 lowercase letters, numbers, or hyphens")
	}
	if strings.TrimSpace(args.Region) == "" || args.VPCID == nil || len(args.SubnetIDs) == 0 || len(args.SecurityGroupIDs) == 0 {
		return errors.New("search region, VPC, subnets, and security groups are required")
	}
	if !kmsARN.MatchString(args.KMSKeyARN) {
		return errors.New("search encryption requires a KMS key ARN")
	}
	if args.AccessIdentityInput == nil && !identityARN.MatchString(args.AccessIdentityARN) {
		return errors.New("search access identity must be an IAM role or user ARN")
	}
	if args.Preset == sdk.PresetPreview {
		if args.Serverless == nil || args.Provisioned != nil {
			return errors.New("preview search requires Serverless settings and no provisioned capacity")
		}
		if !args.Serverless.AcceptColdStarts {
			return errors.New("preview search must explicitly accept Serverless cold starts")
		}
		capacity := args.Serverless.Capacity
		if capacity.MinimumIndexingOCU != 0 || capacity.MinimumSearchOCU != 0 {
			return errors.New("preview search must use zero minimum indexing and search capacity")
		}
		if !validCapacityRange(capacity.MinimumIndexingOCU, capacity.MaximumIndexingOCU) || !validCapacityRange(capacity.MinimumSearchOCU, capacity.MaximumSearchOCU) {
			return errors.New("Serverless indexing and search capacity must have finite positive maxima at least as large as their minima")
		}
		if len(args.SubnetIDs) > 6 {
			return errors.New("Serverless VPC endpoints support at most six subnets")
		}
		return nil
	}

	if args.Preset != sdk.PresetStandard && args.Preset != sdk.PresetHighAvailability {
		return fmt.Errorf("unsupported search preset %q", args.Preset)
	}
	if args.Provisioned == nil || args.Serverless != nil {
		return fmt.Errorf("preset %q requires provisioned settings and no Serverless capacity", args.Preset)
	}
	wantZones, minimumInstances := 2, 2
	if args.Preset == sdk.PresetHighAvailability {
		wantZones, minimumInstances = 3, 3
	}
	if len(args.SubnetIDs) != wantZones {
		return fmt.Errorf("preset %q requires exactly %d search subnets", args.Preset, wantZones)
	}
	provisioned := args.Provisioned
	if strings.TrimSpace(provisioned.EngineVersion) == "" || strings.TrimSpace(provisioned.InstanceType) == "" || strings.TrimSpace(provisioned.EBSVolumeType) == "" || provisioned.EBSVolumeSizeGiB <= 0 {
		return errors.New("provisioned search requires caller-selected engine, instance, and EBS capacity")
	}
	if provisioned.InstanceCount < minimumInstances {
		return fmt.Errorf("preset %q requires at least %d data nodes", args.Preset, minimumInstances)
	}
	if args.Preset == sdk.PresetHighAvailability && provisioned.InstanceCount%3 != 0 {
		return errors.New("high-availability search data node count must be a multiple of three")
	}
	if args.Preset == sdk.PresetHighAvailability && (strings.TrimSpace(provisioned.DedicatedMasterType) == "" || provisioned.DedicatedMasterCount != 3) {
		return errors.New("high-availability search requires a caller-selected dedicated master type and three dedicated masters")
	}
	if args.Preset == sdk.PresetStandard && (provisioned.DedicatedMasterType != "" || provisioned.DedicatedMasterCount != 0) {
		return errors.New("standard search does not accept dedicated master settings")
	}
	return nil
}

func validCapacityRange(minimum, maximum float64) bool {
	return !math.IsNaN(minimum) && !math.IsInf(minimum, 0) && !math.IsNaN(maximum) && !math.IsInf(maximum, 0) && minimum >= 0 && maximum > 0 && maximum >= minimum
}

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

func marshalPolicy(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode OpenSearch policy: %w", err)
	}
	return string(encoded), nil
}

func provisionedAccessPolicy(name, region, principalARN string) (string, error) {
	parts := strings.SplitN(principalARN, ":", 6)
	return marshalPolicy(iamPolicyDocument{
		Version: "2012-10-17",
		Statement: []iamStatement{{
			Effect: "Allow", Principal: map[string]string{"AWS": principalARN}, Action: []string{"es:ESHttp*"},
			Resource: fmt.Sprintf("arn:%s:es:%s:%s:domain/%s/*", parts[1], region, parts[4], name),
		}},
	})
}

func provisionedAccessPolicyInput(name, region string, principal pulumi.StringInput) pulumi.StringOutput {
	return principal.ToStringOutput().ApplyT(func(principalARN string) (string, error) {
		return provisionedAccessPolicy(name, region, principalARN)
	}).(pulumi.StringOutput)
}

func identityInput(args Args) pulumi.StringOutput {
	if args.AccessIdentityInput != nil {
		return args.AccessIdentityInput.ToStringOutput()
	}
	return pulumi.String(args.AccessIdentityARN).ToStringOutput()
}

type iamPolicyDocument struct {
	Version   string         `json:"Version"`
	Statement []iamStatement `json:"Statement"`
}

type iamStatement struct {
	Effect    string            `json:"Effect"`
	Principal map[string]string `json:"Principal"`
	Action    []string          `json:"Action"`
	Resource  string            `json:"Resource"`
}

type networkPolicyDocument struct {
	Description     string        `json:"Description"`
	Rules           []networkRule `json:"Rules"`
	AllowFromPublic bool          `json:"AllowFromPublic"`
	SourceVPCEs     []string      `json:"SourceVPCEs"`
}

type networkRule struct {
	ResourceType string   `json:"ResourceType"`
	Resource     []string `json:"Resource"`
}

type accessPolicyDocument struct {
	Description string       `json:"Description"`
	Rules       []accessRule `json:"Rules"`
	Principal   []string     `json:"Principal"`
}

type accessRule struct {
	ResourceType string   `json:"ResourceType"`
	Resource     []string `json:"Resource"`
	Permission   []string `json:"Permission"`
}
