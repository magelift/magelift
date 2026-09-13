package awsprovider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ecs"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/elasticache"
	"github.com/aws/aws-sdk-go-v2/service/mq"
	mqtypes "github.com/aws/aws-sdk-go-v2/service/mq/types"
	"github.com/aws/aws-sdk-go-v2/service/opensearch"
	opensearchtypes "github.com/aws/aws-sdk-go-v2/service/opensearch/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	awsendpoint "github.com/magelift/magelift/internal/cloud/aws/endpoint"
)

// CapabilityAPI is the provider-owned read-only surface used by plan
// admission. It intentionally has no create, update, or delete method.
// Service-specific SDK types stop at this package.
type CapabilityAPI interface {
	CallerAccount(context.Context) (string, error)
	AvailabilityZones(context.Context) ([]AvailabilityZone, error)
	InstanceType(context.Context, string) (bool, error)
	InstanceTypeOfferings(context.Context, string, []string) ([]string, error)
	Image(context.Context, string) (bool, error)
	FckNatAMI(context.Context, string, string, string) (bool, error)
	DatabaseEngineVersion(context.Context, string, string) (DatabaseEngineVersion, error)
	DatabaseInstanceClass(context.Context, string, string, string) (DatabaseInstanceClass, error)
	ValkeyEngineVersion(context.Context, string) (bool, error)
	ValkeyNodeType(context.Context, string) (bool, error)
	OpenSearchVersion(context.Context, string) (bool, error)
	OpenSearchInstanceType(context.Context, string, string) (bool, error)
	MQEngineVersion(context.Context, string, string) (bool, error)
	MQInstanceType(context.Context, string, string, string) (bool, error)
	ECSCapacityProvider(context.Context, string) (bool, error)
	EKSVersion(context.Context, string) (EKSVersion, error)
}

type AvailabilityZone struct {
	Name  string
	State string
}

type DatabaseEngineVersion struct {
	EngineVersion           string
	Status                  string
	ServerlessV2MinCapacity *float64
	ServerlessV2MaxCapacity *float64
}

type DatabaseInstanceClass struct {
	Available        bool
	AvailabilityZone []string
}

type EKSVersion struct {
	Version              string
	Status               string
	EndOfStandardSupport string
	EndOfExtendedSupport string
}

// SDKCapabilityClient adapts official AWS SDK read-only APIs. It is safe to
// use concurrently for one plan; admission fans out independent catalog
// requests to keep certification runs short.
type SDKCapabilityClient struct {
	sts         *sts.Client
	ec2         *ec2.Client
	ecs         *ecs.Client
	eks         *eks.Client
	elasticache *elasticache.Client
	mq          *mq.Client
	opensearch  *opensearch.Client
	rds         *rds.Client
}

var _ CapabilityAPI = (*SDKCapabilityClient)(nil)

// NewCapabilityClient loads the AWS SDK default credential chain and creates
// read-only service clients. The optional loopback endpoint is shared with the
// existing emulator-safe AWS identity path.
func NewCapabilityClient(ctx context.Context, region string) (*SDKCapabilityClient, error) {
	if ctx == nil {
		return nil, errors.New("AWS capability context is required")
	}
	if strings.TrimSpace(region) == "" {
		return nil, errors.New("AWS capability region is required")
	}
	endpoint, err := awsendpoint.FromEnv()
	if err != nil {
		return nil, err
	}
	configuration, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region))
	if err != nil {
		return nil, errors.New("load AWS capability configuration failed")
	}
	return &SDKCapabilityClient{
		sts:         sts.NewFromConfig(configuration, stsEndpoint(endpoint)),
		ec2:         ec2.NewFromConfig(configuration, ec2Endpoint(endpoint)),
		ecs:         ecs.NewFromConfig(configuration, ecsEndpoint(endpoint)),
		eks:         eks.NewFromConfig(configuration, eksEndpoint(endpoint)),
		elasticache: elasticache.NewFromConfig(configuration, elasticacheEndpoint(endpoint)),
		mq:          mq.NewFromConfig(configuration, mqEndpoint(endpoint)),
		opensearch:  opensearch.NewFromConfig(configuration, opensearchEndpoint(endpoint)),
		rds:         rds.NewFromConfig(configuration, rdsEndpoint(endpoint)),
	}, nil
}

func stsEndpoint(endpoint string) func(*sts.Options) {
	return func(options *sts.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func ec2Endpoint(endpoint string) func(*ec2.Options) {
	return func(options *ec2.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func ecsEndpoint(endpoint string) func(*ecs.Options) {
	return func(options *ecs.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func eksEndpoint(endpoint string) func(*eks.Options) {
	return func(options *eks.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func elasticacheEndpoint(endpoint string) func(*elasticache.Options) {
	return func(options *elasticache.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func mqEndpoint(endpoint string) func(*mq.Options) {
	return func(options *mq.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func opensearchEndpoint(endpoint string) func(*opensearch.Options) {
	return func(options *opensearch.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func rdsEndpoint(endpoint string) func(*rds.Options) {
	return func(options *rds.Options) {
		if endpoint != "" {
			options.BaseEndpoint = awssdk.String(endpoint)
		}
	}
}

func (c *SDKCapabilityClient) CallerAccount(ctx context.Context) (string, error) {
	if c == nil || c.sts == nil {
		return "", errors.New("AWS STS capability client is required")
	}
	response, err := c.sts.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("read AWS caller identity: %w", err)
	}
	if response == nil || strings.TrimSpace(awssdk.ToString(response.Account)) == "" {
		return "", errors.New("AWS caller identity has no account")
	}
	return awssdk.ToString(response.Account), nil
}

func (c *SDKCapabilityClient) AvailabilityZones(ctx context.Context) ([]AvailabilityZone, error) {
	if c == nil || c.ec2 == nil {
		return nil, errors.New("AWS EC2 capability client is required")
	}
	response, err := c.ec2.DescribeAvailabilityZones(ctx, &ec2.DescribeAvailabilityZonesInput{
		Filters: []ec2types.Filter{{Name: awssdk.String("zone-type"), Values: []string{"availability-zone"}}},
	})
	if err != nil {
		return nil, fmt.Errorf("list AWS availability zones: %w", err)
	}
	result := make([]AvailabilityZone, 0, len(response.AvailabilityZones))
	for _, zone := range response.AvailabilityZones {
		if strings.TrimSpace(awssdk.ToString(zone.ZoneName)) == "" {
			continue
		}
		result = append(result, AvailabilityZone{Name: awssdk.ToString(zone.ZoneName), State: string(zone.State)})
	}
	return result, nil
}

func (c *SDKCapabilityClient) InstanceType(ctx context.Context, name string) (bool, error) {
	if c == nil || c.ec2 == nil {
		return false, errors.New("AWS EC2 capability client is required")
	}
	response, err := c.ec2.DescribeInstanceTypes(ctx, &ec2.DescribeInstanceTypesInput{
		InstanceTypes: []ec2types.InstanceType{ec2types.InstanceType(name)},
	})
	if err != nil {
		return false, fmt.Errorf("describe AWS EC2 instance type: %w", err)
	}
	return len(response.InstanceTypes) > 0, nil
}

func (c *SDKCapabilityClient) InstanceTypeOfferings(ctx context.Context, name string, zones []string) ([]string, error) {
	if c == nil || c.ec2 == nil {
		return nil, errors.New("AWS EC2 capability client is required")
	}
	if len(zones) == 0 {
		return nil, errors.New("AWS EC2 availability zones are required for an instance offering check")
	}
	paginator := ec2.NewDescribeInstanceTypeOfferingsPaginator(c.ec2, &ec2.DescribeInstanceTypeOfferingsInput{
		Filters: []ec2types.Filter{
			{Name: awssdk.String("instance-type"), Values: []string{name}},
			{Name: awssdk.String("location"), Values: append([]string(nil), zones...)},
		},
		LocationType: ec2types.LocationTypeAvailabilityZone,
		MaxResults:   awssdk.Int32(1000),
	}, func(options *ec2.DescribeInstanceTypeOfferingsPaginatorOptions) {
		options.StopOnDuplicateToken = true
	})
	result := make([]string, 0, len(zones))
	seen := make(map[string]struct{}, len(zones))
	for paginator.HasMorePages() {
		response, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("describe AWS EC2 instance offerings: %w", err)
		}
		for _, offering := range response.InstanceTypeOfferings {
			location := strings.TrimSpace(awssdk.ToString(offering.Location))
			if location == "" {
				continue
			}
			if _, ok := seen[location]; ok {
				continue
			}
			seen[location] = struct{}{}
			result = append(result, location)
		}
	}
	return result, nil
}

func (c *SDKCapabilityClient) Image(ctx context.Context, imageID string) (bool, error) {
	if c == nil || c.ec2 == nil {
		return false, errors.New("AWS EC2 capability client is required")
	}
	response, err := c.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		ImageIds:          []string{imageID},
		IncludeDeprecated: awssdk.Bool(true),
	})
	if err != nil {
		return false, fmt.Errorf("describe AWS AMI: %w", err)
	}
	for _, image := range response.Images {
		if awssdk.ToString(image.ImageId) != imageID || string(image.State) != "available" || (image.ImageAllowed != nil && !*image.ImageAllowed) {
			continue
		}
		if image.DeprecationTime != nil && strings.TrimSpace(*image.DeprecationTime) != "" {
			if deprecatedAt, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(*image.DeprecationTime)); parseErr != nil || !deprecatedAt.After(time.Now().UTC()) {
				continue
			}
		}
		return true, nil
	}
	return false, nil
}

// FckNatAMI proves that the dynamic fck-nat lookup will resolve to a current
// ARM64 image owned by the documented fck-nat publisher. It is deliberately a
// separate capability from Image because the component selects the AMI by
// owner/name pattern rather than by a user-supplied ID.
func (c *SDKCapabilityClient) FckNatAMI(ctx context.Context, ownerID, namePattern, architecture string) (bool, error) {
	if c == nil || c.ec2 == nil {
		return false, errors.New("AWS EC2 capability client is required")
	}
	ownerID, namePattern, architecture = strings.TrimSpace(ownerID), strings.TrimSpace(namePattern), strings.TrimSpace(architecture)
	if ownerID == "" || namePattern == "" || architecture == "" {
		return false, errors.New("AWS fck-nat AMI owner, name pattern, and architecture are required")
	}
	response, err := c.ec2.DescribeImages(ctx, &ec2.DescribeImagesInput{
		Owners: []string{ownerID},
		Filters: []ec2types.Filter{
			{Name: awssdk.String("name"), Values: []string{namePattern}},
			{Name: awssdk.String("architecture"), Values: []string{architecture}},
			{Name: awssdk.String("state"), Values: []string{"available"}},
		},
		IncludeDeprecated: awssdk.Bool(true),
	})
	if err != nil {
		return false, fmt.Errorf("describe AWS fck-nat AMI: %w", err)
	}
	for _, image := range response.Images {
		if awssdk.ToString(image.OwnerId) != ownerID || string(image.Architecture) != architecture || string(image.State) != "available" || (image.ImageAllowed != nil && !*image.ImageAllowed) {
			continue
		}
		if image.DeprecationTime != nil && strings.TrimSpace(*image.DeprecationTime) != "" {
			if deprecatedAt, parseErr := time.Parse(time.RFC3339, strings.TrimSpace(*image.DeprecationTime)); parseErr != nil || !deprecatedAt.After(time.Now().UTC()) {
				continue
			}
		}
		return true, nil
	}
	return false, nil
}

func (c *SDKCapabilityClient) DatabaseEngineVersion(ctx context.Context, engine, version string) (DatabaseEngineVersion, error) {
	if c == nil || c.rds == nil {
		return DatabaseEngineVersion{}, errors.New("AWS RDS capability client is required")
	}
	marker := (*string)(nil)
	for {
		response, err := c.rds.DescribeDBEngineVersions(ctx, &rds.DescribeDBEngineVersionsInput{
			Engine: awssdk.String(engine), EngineVersion: awssdk.String(version), Marker: marker, MaxRecords: awssdk.Int32(100),
		})
		if err != nil {
			return DatabaseEngineVersion{}, fmt.Errorf("describe AWS RDS engine version: %w", err)
		}
		for _, candidate := range response.DBEngineVersions {
			if awssdk.ToString(candidate.EngineVersion) != version {
				continue
			}
			capability := DatabaseEngineVersion{EngineVersion: awssdk.ToString(candidate.EngineVersion), Status: awssdk.ToString(candidate.Status)}
			if support := candidate.ServerlessV2FeaturesSupport; support != nil {
				capability.ServerlessV2MinCapacity = support.MinCapacity
				capability.ServerlessV2MaxCapacity = support.MaxCapacity
			}
			return capability, nil
		}
		if response.Marker == nil || *response.Marker == "" {
			return DatabaseEngineVersion{}, nil
		}
		marker = response.Marker
	}
}

func (c *SDKCapabilityClient) DatabaseInstanceClass(ctx context.Context, engine, version, class string) (DatabaseInstanceClass, error) {
	if c == nil || c.rds == nil {
		return DatabaseInstanceClass{}, errors.New("AWS RDS capability client is required")
	}
	marker := (*string)(nil)
	for {
		response, err := c.rds.DescribeOrderableDBInstanceOptions(ctx, &rds.DescribeOrderableDBInstanceOptionsInput{
			Engine: awssdk.String(engine), EngineVersion: awssdk.String(version), DBInstanceClass: awssdk.String(class), Vpc: awssdk.Bool(true), Marker: marker, MaxRecords: awssdk.Int32(100),
		})
		if err != nil {
			return DatabaseInstanceClass{}, fmt.Errorf("describe AWS RDS orderable instance class: %w", err)
		}
		for _, candidate := range response.OrderableDBInstanceOptions {
			if awssdk.ToString(candidate.DBInstanceClass) != class {
				continue
			}
			zones := make([]string, 0, len(candidate.AvailabilityZones))
			for _, zone := range candidate.AvailabilityZones {
				if zone.Name != nil {
					zones = append(zones, *zone.Name)
				}
			}
			return DatabaseInstanceClass{Available: true, AvailabilityZone: zones}, nil
		}
		if response.Marker == nil || *response.Marker == "" {
			return DatabaseInstanceClass{}, nil
		}
		marker = response.Marker
	}
}

func (c *SDKCapabilityClient) ValkeyEngineVersion(ctx context.Context, version string) (bool, error) {
	if c == nil || c.elasticache == nil {
		return false, errors.New("AWS ElastiCache capability client is required")
	}
	marker := (*string)(nil)
	for {
		response, err := c.elasticache.DescribeCacheEngineVersions(ctx, &elasticache.DescribeCacheEngineVersionsInput{
			Engine: awssdk.String("valkey"), EngineVersion: awssdk.String(version), Marker: marker, MaxRecords: awssdk.Int32(100),
		})
		if err != nil {
			return false, fmt.Errorf("describe AWS Valkey engine version: %w", err)
		}
		for _, candidate := range response.CacheEngineVersions {
			if awssdk.ToString(candidate.EngineVersion) == version && strings.EqualFold(strings.TrimSpace(awssdk.ToString(candidate.Engine)), "valkey") {
				return true, nil
			}
		}
		if response.Marker == nil || *response.Marker == "" {
			return false, nil
		}
		marker = response.Marker
	}
}

func (c *SDKCapabilityClient) ValkeyNodeType(ctx context.Context, nodeType string) (bool, error) {
	if c == nil || c.elasticache == nil {
		return false, errors.New("AWS ElastiCache capability client is required")
	}
	// Reserved offerings are the smallest current read-only AWS catalog surface
	// available here. They are an engine-specific node-type proxy, not proof of
	// on-demand capacity or a requested replication topology.
	marker := (*string)(nil)
	for {
		response, err := c.elasticache.DescribeReservedCacheNodesOfferings(ctx, &elasticache.DescribeReservedCacheNodesOfferingsInput{
			CacheNodeType: awssdk.String(nodeType), ProductDescription: awssdk.String("valkey"), Marker: marker, MaxRecords: awssdk.Int32(100),
		})
		if err != nil {
			return false, fmt.Errorf("describe AWS Valkey node type offering: %w", err)
		}
		for _, candidate := range response.ReservedCacheNodesOfferings {
			if awssdk.ToString(candidate.CacheNodeType) == nodeType && strings.EqualFold(strings.TrimSpace(awssdk.ToString(candidate.ProductDescription)), "valkey") {
				return true, nil
			}
		}
		if response.Marker == nil || *response.Marker == "" {
			return false, nil
		}
		marker = response.Marker
	}
}

func (c *SDKCapabilityClient) OpenSearchVersion(ctx context.Context, version string) (bool, error) {
	if c == nil || c.opensearch == nil {
		return false, errors.New("AWS OpenSearch capability client is required")
	}
	paginator := opensearch.NewListVersionsPaginator(c.opensearch, &opensearch.ListVersionsInput{MaxResults: 100})
	for paginator.HasMorePages() {
		response, err := paginator.NextPage(ctx)
		if err != nil {
			return false, fmt.Errorf("list AWS OpenSearch versions: %w", err)
		}
		for _, candidate := range response.Versions {
			if candidate == version {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *SDKCapabilityClient) OpenSearchInstanceType(ctx context.Context, version, instanceType string) (bool, error) {
	if c == nil || c.opensearch == nil {
		return false, errors.New("AWS OpenSearch capability client is required")
	}
	response, err := c.opensearch.DescribeInstanceTypeLimits(ctx, &opensearch.DescribeInstanceTypeLimitsInput{
		EngineVersion: awssdk.String(version), InstanceType: opensearchtypes.OpenSearchPartitionInstanceType(instanceType),
	})
	if err != nil {
		return false, fmt.Errorf("describe AWS OpenSearch instance type: %w", err)
	}
	return len(response.LimitsByRole) > 0, nil
}

func (c *SDKCapabilityClient) MQEngineVersion(ctx context.Context, engine, version string) (bool, error) {
	if c == nil || c.mq == nil {
		return false, errors.New("AWS MQ capability client is required")
	}
	canonicalEngine, err := canonicalMQEngine(engine)
	if err != nil {
		return false, err
	}
	nextToken := (*string)(nil)
	for {
		response, err := c.mq.DescribeBrokerEngineTypes(ctx, &mq.DescribeBrokerEngineTypesInput{EngineType: awssdk.String(canonicalEngine), MaxResults: awssdk.Int32(100), NextToken: nextToken})
		if err != nil {
			return false, fmt.Errorf("describe AWS MQ engine versions: %w", err)
		}
		for _, candidate := range response.BrokerEngineTypes {
			if candidate.EngineType != mqtypes.EngineType(canonicalEngine) {
				continue
			}
			for _, candidateVersion := range candidate.EngineVersions {
				if awssdk.ToString(candidateVersion.Name) == version {
					return true, nil
				}
			}
		}
		if response.NextToken == nil || *response.NextToken == "" {
			return false, nil
		}
		nextToken = response.NextToken
	}
}

func (c *SDKCapabilityClient) MQInstanceType(ctx context.Context, engine, version, instanceType string) (bool, error) {
	if c == nil || c.mq == nil {
		return false, errors.New("AWS MQ capability client is required")
	}
	canonicalEngine, err := canonicalMQEngine(engine)
	if err != nil {
		return false, err
	}
	nextToken := (*string)(nil)
	for {
		response, err := c.mq.DescribeBrokerInstanceOptions(ctx, &mq.DescribeBrokerInstanceOptionsInput{
			EngineType:       awssdk.String(canonicalEngine),
			HostInstanceType: awssdk.String(instanceType),
			MaxResults:       awssdk.Int32(100),
			NextToken:        nextToken,
		})
		if err != nil {
			return false, fmt.Errorf("describe AWS MQ instance type: %w", err)
		}
		for _, candidate := range response.BrokerInstanceOptions {
			if candidate.EngineType != mqtypes.EngineType(canonicalEngine) || awssdk.ToString(candidate.HostInstanceType) != instanceType {
				continue
			}
			for _, candidateVersion := range candidate.SupportedEngineVersions {
				if candidateVersion == version {
					return true, nil
				}
			}
		}
		if response.NextToken == nil || *response.NextToken == "" {
			return false, nil
		}
		nextToken = response.NextToken
	}
}

func (c *SDKCapabilityClient) ECSCapacityProvider(ctx context.Context, name string) (bool, error) {
	if c == nil || c.ecs == nil {
		return false, errors.New("AWS ECS capability client is required")
	}
	response, err := c.ecs.DescribeCapacityProviders(ctx, &ecs.DescribeCapacityProvidersInput{CapacityProviders: []string{name}})
	if err != nil {
		return false, fmt.Errorf("describe AWS ECS capacity provider: %w", err)
	}
	for _, provider := range response.CapacityProviders {
		if awssdk.ToString(provider.Name) == name && string(provider.Status) == "ACTIVE" {
			return true, nil
		}
	}
	return false, nil
}

func (c *SDKCapabilityClient) EKSVersion(ctx context.Context, version string) (EKSVersion, error) {
	if c == nil || c.eks == nil {
		return EKSVersion{}, errors.New("AWS EKS capability client is required")
	}
	paginator := eks.NewDescribeClusterVersionsPaginator(c.eks, &eks.DescribeClusterVersionsInput{
		ClusterVersions: []string{version}, MaxResults: awssdk.Int32(100),
	})
	for paginator.HasMorePages() {
		response, err := paginator.NextPage(ctx)
		if err != nil {
			return EKSVersion{}, fmt.Errorf("describe AWS EKS versions: %w", err)
		}
		for _, candidate := range response.ClusterVersions {
			if awssdk.ToString(candidate.ClusterVersion) != version {
				continue
			}
			return EKSVersion{
				Version:              awssdk.ToString(candidate.ClusterVersion),
				Status:               string(candidate.VersionStatus),
				EndOfStandardSupport: formatTime(candidate.EndOfStandardSupportDate),
				EndOfExtendedSupport: formatTime(candidate.EndOfExtendedSupportDate),
			}, nil
		}
	}
	return EKSVersion{}, nil
}

func formatTime(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
