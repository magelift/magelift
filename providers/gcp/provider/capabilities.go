package gcpprovider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	container "cloud.google.com/go/container/apiv1"
	containerpb "cloud.google.com/go/container/apiv1/containerpb"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	cloudbilling "google.golang.org/api/cloudbilling/v1"
	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
	sqladmin "google.golang.org/api/sqladmin/v1beta4"
)

// CapabilityAPI is the GCP read-only surface used by plan admission. It has
// no resource mutation method; the core can therefore call it before any
// Pulumi update without giving admission a write path.
type CapabilityAPI interface {
	Project(context.Context, string) (ProjectIdentity, error)
	BillingEnabled(context.Context, string) (bool, error)
	EnabledServices(context.Context, ProjectIdentity, []string) (map[string]bool, error)
	RegionZones(context.Context, string, string) ([]string, error)
	RegionQuota(context.Context, string, string, string) (float64, error)
	CloudSQLDatabaseVersion(context.Context, string, string) (bool, error)
	CloudSQLTier(context.Context, string, string, string) (bool, error)
	MemorystoreValkey(context.Context, string, string, string, string, string) (bool, error)
	KubernetesVersion(context.Context, string, string, string, string) (bool, error)
	MachineType(context.Context, string, string, string) (bool, error)
	MachineTypeCPUs(context.Context, string, string, string) (int64, error)
}

// SDKCapabilityClient adapts official Google clients and the provider's
// current documented Memorystore catalog. GCP exposes Cloud SQL tiers and GKE
// versions through read APIs. Memorystore for Valkey has a separate REST
// surface from the legacy Redis API, so the current read-only instances list
// is translated explicitly at this boundary.
type SDKCapabilityClient struct {
	projects   *cloudresourcemanager.Service
	billing    *cloudbilling.APIService
	services   *serviceusage.Service
	regions    *compute.Service
	sql        *sqladmin.Service
	valkey     *ValkeyRESTClient
	containers *container.ClusterManagerClient
}

var _ CapabilityAPI = (*SDKCapabilityClient)(nil)

func NewCapabilityClient(ctx context.Context) (*SDKCapabilityClient, error) {
	if ctx == nil {
		return nil, errors.New("GCP capability context is required")
	}
	scopes := option.WithScopes("https://www.googleapis.com/auth/cloud-platform")
	projects, err := cloudresourcemanager.NewService(ctx, scopes)
	if err != nil {
		return nil, errors.New("create GCP Resource Manager capability client failed")
	}
	regions, err := compute.NewService(ctx, scopes)
	if err != nil {
		return nil, errors.New("create GCP Compute capability client failed")
	}
	sql, err := sqladmin.NewService(ctx, scopes)
	if err != nil {
		return nil, errors.New("create GCP Cloud SQL capability client failed")
	}
	tokenSource, err := google.DefaultTokenSource(ctx, "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		return nil, errors.New("create GCP capability token source failed")
	}
	valkey := NewValkeyRESTClient(oauth2.NewClient(ctx, tokenSource), defaultValkeyBaseURL)
	billing, err := cloudbilling.NewService(ctx, scopes)
	if err != nil {
		return nil, errors.New("create GCP Cloud Billing capability client failed")
	}
	services, err := serviceusage.NewService(ctx, scopes)
	if err != nil {
		return nil, errors.New("create GCP Service Usage capability client failed")
	}
	containers, err := container.NewClusterManagerClient(ctx)
	if err != nil {
		return nil, errors.New("create GCP GKE capability client failed")
	}
	return &SDKCapabilityClient{projects: projects, billing: billing, services: services, regions: regions, sql: sql, valkey: valkey, containers: containers}, nil
}

func (c *SDKCapabilityClient) Project(ctx context.Context, projectID string) (ProjectIdentity, error) {
	if c == nil || c.projects == nil {
		return ProjectIdentity{}, errors.New("GCP Resource Manager capability client is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return ProjectIdentity{}, errors.New("GCP project ID is required")
	}
	project, err := c.projects.Projects.Get(projectID).Context(ctx).Do()
	if err != nil {
		return ProjectIdentity{}, fmt.Errorf("read GCP project %q: %w", projectID, err)
	}
	if project == nil || strings.TrimSpace(project.ProjectId) != projectID {
		return ProjectIdentity{}, errors.New("GCP project identity does not match the requested project")
	}
	return ProjectIdentity{ID: project.ProjectId, Number: project.ProjectNumber}, nil
}

func (c *SDKCapabilityClient) BillingEnabled(ctx context.Context, projectID string) (bool, error) {
	if c == nil || c.billing == nil {
		return false, errors.New("GCP Cloud Billing capability client is required")
	}
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return false, errors.New("GCP project ID is required for billing admission")
	}
	info, err := c.billing.Projects.GetBillingInfo("projects/" + projectID).Context(ctx).Do()
	if err != nil {
		return false, fmt.Errorf("read GCP billing info: %w", err)
	}
	if info == nil || strings.TrimSpace(info.ProjectId) != projectID {
		return false, errors.New("GCP billing info does not match the requested project")
	}
	return info.BillingEnabled, nil
}

func (c *SDKCapabilityClient) EnabledServices(ctx context.Context, identity ProjectIdentity, serviceNames []string) (map[string]bool, error) {
	if c == nil || c.services == nil {
		return nil, errors.New("GCP Service Usage capability client is required")
	}
	if identity.Number <= 0 {
		return nil, errors.New("GCP project number is required for service admission")
	}
	parent := "projects/" + strconv.FormatInt(identity.Number, 10)
	result := make(map[string]bool, len(serviceNames))
	unique := make([]string, 0, len(serviceNames))
	seen := make(map[string]struct{}, len(serviceNames))
	for _, serviceName := range serviceNames {
		serviceName = strings.TrimSpace(serviceName)
		if serviceName == "" || strings.ContainsAny(serviceName, "/?#") {
			return nil, errors.New("GCP service name is required for service admission")
		}
		if _, ok := seen[serviceName]; ok {
			continue
		}
		seen[serviceName] = struct{}{}
		result[serviceName] = false
		unique = append(unique, serviceName)
	}
	for start := 0; start < len(unique); start += 30 {
		end := start + 30
		if end > len(unique) {
			end = len(unique)
		}
		names := make([]string, 0, end-start)
		for _, serviceName := range unique[start:end] {
			names = append(names, parent+"/services/"+serviceName)
		}
		services, err := c.services.Services.BatchGet(parent).Names(names...).Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("read GCP service states: %w", err)
		}
		if services == nil {
			return nil, errors.New("GCP Service Usage returned no service states")
		}
		for _, service := range services.Services {
			if service == nil {
				continue
			}
			name := strings.TrimSpace(service.Name)
			if !strings.HasPrefix(name, parent+"/services/") {
				return nil, errors.New("GCP Service Usage returned a service for another project")
			}
			serviceName := strings.TrimPrefix(name, parent+"/services/")
			if serviceName == "" || strings.Contains(serviceName, "/") {
				return nil, errors.New("GCP Service Usage returned an invalid service name")
			}
			if _, ok := result[serviceName]; !ok {
				return nil, fmt.Errorf("GCP Service Usage returned unexpected service %q", serviceName)
			}
			result[serviceName] = strings.EqualFold(strings.TrimSpace(service.State), "ENABLED")
		}
	}
	return result, nil
}

func (c *SDKCapabilityClient) RegionZones(ctx context.Context, projectID, region string) ([]string, error) {
	if c == nil || c.regions == nil {
		return nil, errors.New("GCP Compute capability client is required")
	}
	region = strings.TrimSpace(region)
	if strings.TrimSpace(projectID) == "" || region == "" {
		return nil, errors.New("GCP project and region are required for zone admission")
	}
	value, err := c.regions.Regions.Get(projectID, region).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read GCP region %q: %w", region, err)
	}
	if value == nil || !strings.EqualFold(value.Status, "UP") {
		return nil, fmt.Errorf("GCP region %q is not up", region)
	}
	result := make([]string, 0, len(value.Zones))
	for _, zoneURL := range value.Zones {
		zone := path.Base(strings.TrimSpace(zoneURL))
		if zone != "." && zone != "/" && zone != "" {
			result = append(result, zone)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("GCP region %q returned no zones", region)
	}
	return result, nil
}

func (c *SDKCapabilityClient) RegionQuota(ctx context.Context, projectID, region, metric string) (float64, error) {
	if c == nil || c.regions == nil {
		return 0, errors.New("GCP Compute capability client is required")
	}
	projectID, region, metric = strings.TrimSpace(projectID), strings.TrimSpace(region), strings.TrimSpace(metric)
	if projectID == "" || region == "" || metric == "" {
		return 0, errors.New("GCP project, region, and quota metric are required")
	}
	value, err := c.regions.Regions.Get(projectID, region).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("read GCP region quota %q: %w", metric, err)
	}
	if value == nil {
		return 0, fmt.Errorf("GCP region %q returned no quota catalog", region)
	}
	for _, quota := range value.Quotas {
		if quota != nil && strings.EqualFold(strings.TrimSpace(quota.Metric), metric) {
			available := quota.Limit - quota.Usage
			if available < 0 {
				return 0, nil
			}
			return available, nil
		}
	}
	return 0, fmt.Errorf("GCP region quota metric %q was not returned", metric)
}

func (c *SDKCapabilityClient) CloudSQLDatabaseVersion(_ context.Context, _ string, version string) (bool, error) {
	switch strings.TrimSpace(version) {
	case "MYSQL_8_0", "MYSQL_8_4":
		return true, nil
	default:
		return false, nil
	}
}

func (c *SDKCapabilityClient) CloudSQLTier(ctx context.Context, projectID, region, tier string) (bool, error) {
	if c == nil || c.sql == nil {
		return false, errors.New("GCP Cloud SQL capability client is required")
	}
	projectID, region, tier = strings.TrimSpace(projectID), strings.TrimSpace(region), strings.TrimSpace(tier)
	if projectID == "" || region == "" || tier == "" {
		return false, errors.New("GCP Cloud SQL project, region, and tier are required")
	}
	result, err := c.sql.Tiers.List(projectID).Context(ctx).Do()
	if err != nil {
		return false, fmt.Errorf("list GCP Cloud SQL tiers: %w", err)
	}
	if result == nil {
		return false, errors.New("GCP Cloud SQL returned no tier catalog")
	}
	for _, candidate := range result.Items {
		if candidate == nil || candidate.Tier != tier {
			continue
		}
		for _, candidateRegion := range candidate.Region {
			if candidateRegion == region {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *SDKCapabilityClient) MemorystoreValkey(ctx context.Context, projectID, region, engineVersion, nodeType, mode string) (bool, error) {
	if c == nil || c.valkey == nil {
		return false, errors.New("GCP Memorystore capability client is required")
	}
	projectID, region = strings.TrimSpace(projectID), strings.TrimSpace(region)
	engineVersion, nodeType, mode = strings.TrimSpace(engineVersion), strings.TrimSpace(nodeType), strings.TrimSpace(mode)
	if projectID == "" || region == "" || engineVersion == "" || nodeType == "" {
		return false, errors.New("GCP Memorystore project, region, engine version, and node type are required")
	}
	if !supportedValkeyVersions[engineVersion] {
		return false, nil
	}
	if !supportedValkeyNodeTypes[nodeType] {
		return false, nil
	}
	if mode == "" {
		mode = "CLUSTER_DISABLED"
	}
	if strings.HasPrefix(nodeType, "CUSTOM_") && mode != "CLUSTER_DISABLED" {
		return false, nil
	}
	if err := c.valkey.ListInstances(ctx, projectID, region); err != nil {
		return false, fmt.Errorf("read GCP Memorystore Valkey instances in %q: %w", region, err)
	}
	return true, nil
}

func (c *SDKCapabilityClient) KubernetesVersion(ctx context.Context, projectID, region, releaseChannel, version string) (bool, error) {
	if c == nil || c.containers == nil {
		return false, errors.New("GCP GKE capability client is required")
	}
	projectID, region, releaseChannel, version = strings.TrimSpace(projectID), strings.TrimSpace(region), strings.TrimSpace(releaseChannel), strings.TrimSpace(version)
	if projectID == "" || region == "" {
		return false, errors.New("GCP GKE project and region are required")
	}
	config, err := c.containers.GetServerConfig(ctx, &containerpb.GetServerConfigRequest{Name: "projects/" + projectID + "/locations/" + region})
	if err != nil {
		return false, fmt.Errorf("read GCP GKE server config: %w", err)
	}
	if config == nil {
		return false, errors.New("GCP GKE returned no server config")
	}
	if version == "" {
		if strings.TrimSpace(releaseChannel) != "" && !hasGKEReleaseChannel(config.Channels, releaseChannel) {
			return false, nil
		}
		return strings.TrimSpace(config.DefaultClusterVersion) != "", nil
	}
	if releaseChannel != "" {
		for _, channel := range config.Channels {
			if channel == nil || !strings.EqualFold(channel.Channel.String(), releaseChannel) {
				continue
			}
			return containsGKEVersion(channel.ValidVersions, version), nil
		}
		return false, nil
	}
	return containsGKEVersion(config.ValidMasterVersions, version), nil
}

func hasGKEReleaseChannel(channels []*containerpb.ServerConfig_ReleaseChannelConfig, wanted string) bool {
	for _, channel := range channels {
		if channel != nil && strings.EqualFold(channel.Channel.String(), strings.TrimSpace(wanted)) {
			return true
		}
	}
	return false
}

func containsGKEVersion(candidates []string, requested string) bool {
	requested = strings.TrimSpace(requested)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == requested || strings.HasPrefix(candidate, requested+".") {
			return true
		}
	}
	return false
}

func (c *SDKCapabilityClient) MachineType(ctx context.Context, projectID, zone, machineType string) (bool, error) {
	value, err := c.readMachineType(ctx, projectID, zone, machineType)
	if err != nil {
		return false, err
	}
	// MachineTypes.Get is an existence/readiness check; unlike Region, the
	// Compute Engine MachineType resource has no Status field. A deprecation
	// marker means the provider no longer guarantees that the type can be used
	// for a new VM, so fail closed for admission.
	return value != nil && value.Deprecated == nil, nil
}

func (c *SDKCapabilityClient) MachineTypeCPUs(ctx context.Context, projectID, zone, machineType string) (int64, error) {
	value, err := c.readMachineType(ctx, projectID, zone, machineType)
	if err != nil {
		return 0, err
	}
	if value == nil || value.Deprecated != nil || value.GuestCpus <= 0 {
		return 0, fmt.Errorf("GCP machine type %q in %q has no current CPU capacity", strings.TrimSpace(machineType), strings.TrimSpace(zone))
	}
	return value.GuestCpus, nil
}

func (c *SDKCapabilityClient) readMachineType(ctx context.Context, projectID, zone, machineType string) (*compute.MachineType, error) {
	if c == nil || c.regions == nil {
		return nil, errors.New("GCP Compute capability client is required")
	}
	projectID, zone, machineType = strings.TrimSpace(projectID), strings.TrimSpace(zone), strings.TrimSpace(machineType)
	if projectID == "" || zone == "" || machineType == "" {
		return nil, errors.New("GCP Compute project, zone, and machine type are required")
	}
	value, err := c.regions.MachineTypes.Get(projectID, zone, machineType).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("read GCP machine type %q in %q: %w", machineType, zone, err)
	}
	return value, nil
}

var supportedValkeyVersions = map[string]bool{
	"VALKEY_7_2": true,
	"VALKEY_8_0": true,
	"VALKEY_9_0": true,
	"VALKEY_9_1": true,
}

var supportedValkeyNodeTypes = map[string]bool{
	"SHARED_CORE_NANO": true,
	"CUSTOM_PICO":      true,
	"CUSTOM_MICRO":     true,
	"CUSTOM_MINI":      true,
	"STANDARD_SMALL":   true,
	"HIGHMEM_MEDIUM":   true,
	"HIGHCPU_MEDIUM":   true,
	"STANDARD_LARGE":   true,
	"HIGHMEM_XLARGE":   true,
	"HIGHMEM_2XLARGE":  true,
}

const defaultValkeyBaseURL = "https://memorystore.googleapis.com"

// ValkeyRESTClient is the read-only translator for the current Memorystore
// for Valkey REST API. It intentionally exposes only GET list behavior: plan
// admission must never acquire a resource mutation path.
type ValkeyRESTClient struct {
	httpClient *http.Client
	baseURL    string
}

// NewValkeyRESTClient constructs a read-only Valkey API client. A custom
// endpoint is useful for deterministic tests and community-maintained API
// proxies; production callers should use the documented Memorystore host.
func NewValkeyRESTClient(httpClient *http.Client, baseURL string) *ValkeyRESTClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = defaultValkeyBaseURL
	}
	return &ValkeyRESTClient{httpClient: httpClient, baseURL: strings.TrimRight(baseURL, "/")}
}

type valkeyInstancesPage struct {
	NextPageToken string   `json:"nextPageToken"`
	Unreachable   []string `json:"unreachable"`
}

// ListInstances verifies that the current project/location Valkey API can be
// read. An empty successful list is valid: it means no instance exists yet;
// it does not claim account-specific create capacity.
func (c *ValkeyRESTClient) ListInstances(ctx context.Context, projectID, region string) error {
	if c == nil || c.httpClient == nil {
		return errors.New("GCP Memorystore Valkey HTTP client is required")
	}
	projectID, region = strings.TrimSpace(projectID), strings.TrimSpace(region)
	if projectID == "" || region == "" || strings.ContainsAny(projectID, "/?#") || strings.ContainsAny(region, "/?#") {
		return errors.New("GCP Memorystore project and region are required resource path segments")
	}
	base, err := url.Parse(strings.TrimRight(c.baseURL, "/"))
	if err != nil || base.Scheme == "" || base.Host == "" {
		return errors.New("GCP Memorystore Valkey base URL is invalid")
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/v1/projects/" + projectID + "/locations/" + region + "/instances"
	var pageToken string
	seenPageTokens := make(map[string]struct{})
	for {
		if pageToken != "" {
			if _, seen := seenPageTokens[pageToken]; seen {
				return errors.New("GCP Memorystore Valkey list returned a repeated page token")
			}
			seenPageTokens[pageToken] = struct{}{}
		}
		requestURL := *base
		query := requestURL.Query()
		query.Set("pageSize", "1000")
		if pageToken != "" {
			query.Set("pageToken", pageToken)
		}
		requestURL.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
		if err != nil {
			return fmt.Errorf("create GCP Memorystore Valkey request: %w", err)
		}
		response, err := c.httpClient.Do(request)
		if err != nil {
			return fmt.Errorf("request GCP Memorystore Valkey instances: %w", err)
		}
		page, decodeErr := decodeValkeyInstancesPage(response)
		if decodeErr != nil {
			return decodeErr
		}
		if len(page.Unreachable) > 0 {
			return fmt.Errorf("GCP Memorystore reported unreachable locations: %s", strings.Join(page.Unreachable, ", "))
		}
		pageToken = strings.TrimSpace(page.NextPageToken)
		if pageToken == "" {
			return nil
		}
	}
}

func decodeValkeyInstancesPage(response *http.Response) (valkeyInstancesPage, error) {
	if response == nil {
		return valkeyInstancesPage{}, errors.New("GCP Memorystore returned no HTTP response")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return valkeyInstancesPage{}, fmt.Errorf("GCP Memorystore Valkey list returned HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var page valkeyInstancesPage
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&page); err != nil {
		return valkeyInstancesPage{}, fmt.Errorf("decode GCP Memorystore Valkey list: %w", err)
	}
	return page, nil
}
