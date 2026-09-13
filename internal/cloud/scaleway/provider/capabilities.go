package scalewayprovider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	account "github.com/scaleway/scaleway-sdk-go/api/account/v3"
	instance "github.com/scaleway/scaleway-sdk-go/api/instance/v1"
	k8s "github.com/scaleway/scaleway-sdk-go/api/k8s/v1"
	rdb "github.com/scaleway/scaleway-sdk-go/api/rdb/v1"
	redis "github.com/scaleway/scaleway-sdk-go/api/redis/v1"
	"github.com/scaleway/scaleway-sdk-go/scw"
)

// CapabilityAPI is the read-only Scaleway surface used by plan admission.
// The provider SDK types stop at this package; stack admission consumes the
// small value objects below and therefore remains deterministic to test.
type CapabilityAPI interface {
	Project(context.Context, string) (ProjectIdentity, error)
	KubernetesVersions(context.Context, string) ([]KubernetesVersion, error)
	KubernetesClusterTypes(context.Context, string) ([]KubernetesClusterType, error)
	InstanceTypes(context.Context, string) (map[string]InstanceType, error)
	InstanceTypeAvailability(context.Context, string) (map[string]string, error)
	DatabaseEngines(context.Context, string) ([]DatabaseEngine, error)
	DatabaseNodeTypes(context.Context, string) ([]DatabaseNodeType, error)
	RedisVersions(context.Context, string) ([]RedisVersion, error)
	RedisNodeTypes(context.Context, string) ([]RedisNodeType, error)
}

type KubernetesVersion struct {
	Name         string
	DeprecatedAt *time.Time
	EndOfLifeAt  *time.Time
}

type KubernetesClusterType struct {
	Name         string
	Availability string
	MaxNodes     uint32
	Region       string
}

type InstanceType struct {
	Name         string
	EndOfService bool
	Architecture string
	CPUs         uint32
	MemoryBytes  uint64
}

type DatabaseEngine struct {
	Name     string
	Versions []DatabaseEngineVersion
}

type DatabaseEngineVersion struct {
	Name      string
	Version   string
	Disabled  bool
	Beta      bool
	EndOfLife *time.Time
}

type DatabaseNodeType struct {
	Name        string
	StockStatus string
	Disabled    bool
	HARequired  bool
	Region      string
}

type RedisVersion struct {
	Version     string
	EndOfLifeAt *time.Time
}

type RedisNodeType struct {
	Name        string
	StockStatus string
	Disabled    bool
	Zone        string
}

// SDKCapabilityClient adapts the official Scaleway SDK read-only endpoints.
// It deliberately exposes no create, update, or delete operation.
type SDKCapabilityClient struct {
	projects *account.ProjectAPI
	k8s      *k8s.API
	instance *instance.API
	rdb      *rdb.API
	redis    *redis.API
}

var _ CapabilityAPI = (*SDKCapabilityClient)(nil)

// NewCapabilityClient loads the authenticated Scaleway SDK client used by
// admission. It does not provision or mutate a provider resource.
func NewCapabilityClient(ctx context.Context, projectID, region, zone string) (*SDKCapabilityClient, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway capability context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectID) == "" || strings.TrimSpace(region) == "" || strings.TrimSpace(zone) == "" {
		return nil, errors.New("Scaleway capability project, region, and zone are required")
	}
	options := []scw.ClientOption{
		scw.WithDefaultProjectID(projectID),
		scw.WithDefaultRegion(scw.Region(region)),
		scw.WithDefaultZone(scw.Zone(zone)),
		scw.WithEnv(),
	}
	// The CLI and the SDK share the Scaleway profile file, but the SDK does
	// not infer the CLI's selected profile from `scw --profile`. Honor the
	// standard SCW_PROFILE bridge when a caller selected one, while keeping
	// environment credentials available for CI and community adapters.
	if profile := strings.TrimSpace(os.Getenv("SCW_PROFILE")); profile != "" {
		config, err := scw.LoadConfig()
		if err != nil {
			return nil, errors.New("load Scaleway SDK profile")
		}
		profileConfig, err := config.GetProfile(profile)
		if err != nil {
			return nil, errors.New("load selected Scaleway SDK profile")
		}
		options = append(options, scw.WithProfile(profileConfig))
	}
	client, err := scw.NewClient(options...)
	if err != nil {
		return nil, errors.New("create Scaleway capability client")
	}
	return &SDKCapabilityClient{
		projects: account.NewProjectAPI(client),
		k8s:      k8s.NewAPI(client),
		instance: instance.NewAPI(client),
		rdb:      rdb.NewAPI(client),
		redis:    redis.NewAPI(client),
	}, nil
}

func (c *SDKCapabilityClient) Project(ctx context.Context, projectID string) (ProjectIdentity, error) {
	if ctx == nil {
		return ProjectIdentity{}, errors.New("Scaleway capability context is required")
	}
	if c == nil || c.projects == nil {
		return ProjectIdentity{}, errors.New("Scaleway project capability client is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectIdentity{}, errors.New("Scaleway project ID is required")
	}
	project, err := c.projects.GetProject(&account.ProjectAPIGetProjectRequest{ProjectID: projectID}, scw.WithContext(ctx))
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("read Scaleway project identity: %w", err)
	}
	if project == nil || strings.TrimSpace(project.ID) == "" {
		return ProjectIdentity{}, errors.New("Scaleway project identity response is empty")
	}
	return ProjectIdentity{ID: project.ID, OrganizationID: project.OrganizationID}, nil
}

func requestOptions(ctx context.Context, allPages bool) []scw.RequestOption {
	options := []scw.RequestOption{scw.WithContext(ctx)}
	if allPages {
		options = append(options, scw.WithAllPages())
	}
	return options
}

func (c *SDKCapabilityClient) KubernetesVersions(ctx context.Context, region string) ([]KubernetesVersion, error) {
	if c == nil || c.k8s == nil {
		return nil, errors.New("Scaleway Kubernetes capability client is required")
	}
	// The Kapsule ListVersions response is a complete catalog and does not
	// implement the SDK pagination interface. Passing WithAllPages here makes
	// the SDK reject an otherwise valid read-only admission request.
	response, err := c.k8s.ListVersions(&k8s.ListVersionsRequest{Region: scw.Region(region)}, requestOptions(ctx, false)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Kubernetes versions: %w", err)
	}
	result := make([]KubernetesVersion, 0, len(response.Versions))
	for _, version := range response.Versions {
		if version == nil {
			continue
		}
		result = append(result, KubernetesVersion{Name: version.Name, DeprecatedAt: version.DeprecatedAt, EndOfLifeAt: version.EndOfLifeAt})
	}
	return result, nil
}

func (c *SDKCapabilityClient) KubernetesClusterTypes(ctx context.Context, region string) ([]KubernetesClusterType, error) {
	if c == nil || c.k8s == nil {
		return nil, errors.New("Scaleway Kubernetes capability client is required")
	}
	response, err := c.k8s.ListClusterTypes(&k8s.ListClusterTypesRequest{Region: scw.Region(region)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Kubernetes cluster types: %w", err)
	}
	result := make([]KubernetesClusterType, 0, len(response.ClusterTypes))
	for _, clusterType := range response.ClusterTypes {
		if clusterType == nil {
			continue
		}
		result = append(result, KubernetesClusterType{Name: clusterType.Name, Availability: clusterType.Availability.String(), MaxNodes: clusterType.MaxNodes, Region: string(clusterType.Region)})
	}
	return result, nil
}

func (c *SDKCapabilityClient) InstanceTypes(ctx context.Context, zone string) (map[string]InstanceType, error) {
	if c == nil || c.instance == nil {
		return nil, errors.New("Scaleway Instance capability client is required")
	}
	response, err := c.instance.ListServersTypes(&instance.ListServersTypesRequest{Zone: scw.Zone(zone)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Instance types: %w", err)
	}
	result := make(map[string]InstanceType, len(response.Servers))
	for name, serverType := range response.Servers {
		if serverType == nil {
			continue
		}
		result[name] = InstanceType{Name: name, EndOfService: serverType.EndOfService, Architecture: string(serverType.Arch), CPUs: serverType.Ncpus, MemoryBytes: serverType.RAM}
	}
	return result, nil
}

func (c *SDKCapabilityClient) InstanceTypeAvailability(ctx context.Context, zone string) (map[string]string, error) {
	if c == nil || c.instance == nil {
		return nil, errors.New("Scaleway Instance capability client is required")
	}
	response, err := c.instance.GetServerTypesAvailability(&instance.GetServerTypesAvailabilityRequest{Zone: scw.Zone(zone)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("read Scaleway Instance type availability: %w", err)
	}
	result := make(map[string]string, len(response.Servers))
	for name, availability := range response.Servers {
		if availability == nil {
			continue
		}
		result[name] = availability.Availability.String()
	}
	return result, nil
}

func (c *SDKCapabilityClient) DatabaseEngines(ctx context.Context, region string) ([]DatabaseEngine, error) {
	if c == nil || c.rdb == nil {
		return nil, errors.New("Scaleway RDB capability client is required")
	}
	response, err := c.rdb.ListDatabaseEngines(&rdb.ListDatabaseEnginesRequest{Region: scw.Region(region)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway database engines: %w", err)
	}
	result := make([]DatabaseEngine, 0, len(response.Engines))
	for _, engine := range response.Engines {
		if engine == nil {
			continue
		}
		versions := make([]DatabaseEngineVersion, 0, len(engine.Versions))
		for _, version := range engine.Versions {
			if version == nil {
				continue
			}
			versions = append(versions, DatabaseEngineVersion{Name: version.Name, Version: version.Version, Disabled: version.Disabled, Beta: version.Beta, EndOfLife: version.EndOfLife})
		}
		result = append(result, DatabaseEngine{Name: engine.Name, Versions: versions})
	}
	return result, nil
}

func (c *SDKCapabilityClient) DatabaseNodeTypes(ctx context.Context, region string) ([]DatabaseNodeType, error) {
	if c == nil || c.rdb == nil {
		return nil, errors.New("Scaleway RDB capability client is required")
	}
	response, err := c.rdb.ListNodeTypes(&rdb.ListNodeTypesRequest{Region: scw.Region(region)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway database node types: %w", err)
	}
	result := make([]DatabaseNodeType, 0, len(response.NodeTypes))
	for _, nodeType := range response.NodeTypes {
		if nodeType == nil {
			continue
		}
		result = append(result, DatabaseNodeType{Name: nodeType.Name, StockStatus: nodeType.StockStatus.String(), Disabled: nodeType.Disabled, HARequired: nodeType.IsHaRequired, Region: string(nodeType.Region)})
	}
	return result, nil
}

func (c *SDKCapabilityClient) RedisVersions(ctx context.Context, zone string) ([]RedisVersion, error) {
	if c == nil || c.redis == nil {
		return nil, errors.New("Scaleway Redis capability client is required")
	}
	response, err := c.redis.ListClusterVersions(&redis.ListClusterVersionsRequest{Zone: scw.Zone(zone)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Redis versions: %w", err)
	}
	result := make([]RedisVersion, 0, len(response.Versions))
	for _, version := range response.Versions {
		if version == nil {
			continue
		}
		result = append(result, RedisVersion{Version: version.Version, EndOfLifeAt: version.EndOfLifeAt})
	}
	return result, nil
}

func (c *SDKCapabilityClient) RedisNodeTypes(ctx context.Context, zone string) ([]RedisNodeType, error) {
	if c == nil || c.redis == nil {
		return nil, errors.New("Scaleway Redis capability client is required")
	}
	response, err := c.redis.ListNodeTypes(&redis.ListNodeTypesRequest{Zone: scw.Zone(zone)}, requestOptions(ctx, true)...)
	if err != nil {
		return nil, fmt.Errorf("list Scaleway Redis node types: %w", err)
	}
	result := make([]RedisNodeType, 0, len(response.NodeTypes))
	for _, nodeType := range response.NodeTypes {
		if nodeType == nil {
			continue
		}
		result = append(result, RedisNodeType{Name: nodeType.Name, StockStatus: nodeType.StockStatus.String(), Disabled: nodeType.Disabled, Zone: string(nodeType.Zone)})
	}
	return result, nil
}
