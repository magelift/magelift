package ovhprovider

import (
	"context"
	"errors"
	"net/url"
	"strings"
)

// RegionCapability is the read-only region shape returned by the OVHcloud
// Public Cloud API. The API has exposed both name and regionName across
// generations, so the client normalizes either field into Name.
type RegionCapability struct {
	Name              string   `json:"name"`
	RegionName        string   `json:"regionName"`
	Status            string   `json:"status"`
	AvailabilityZones []string `json:"availabilityZones"`
}

type ProjectCapability struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	ServiceName string `json:"serviceName"`
}

// DatabaseAvailability is one provider-admitted combination returned by the
// Public Cloud Database availability endpoint. The endpoint returns one row
// per engine, version, plan, flavor, region, and network boundary.
type DatabaseAvailability struct {
	Engine        string `json:"engine"`
	Version       string `json:"version"`
	Plan          string `json:"plan"`
	Flavor        string `json:"flavor"`
	Region        string `json:"region"`
	Network       string `json:"network"`
	MinNodeNumber int    `json:"minNodeNumber"`
	MaxNodeNumber int    `json:"maxNodeNumber"`
}

// CapabilityClient owns read-only OVH capability lookups. It deliberately
// depends on the narrow API interface shared with identity admission so
// provider responses remain easy to fake and community adapters can reuse the
// translator without importing OVH SDK response types.
type CapabilityClient struct {
	api API
}

// NewCapabilityFromClient injects an OVH API client for capability queries.
func NewCapabilityFromClient(client API) (*CapabilityClient, error) {
	if client == nil {
		return nil, errors.New("OVH API client is required")
	}
	return &CapabilityClient{api: client}, nil
}

// Project verifies that the requested Public Cloud project exists before any
// region, managed-service, or network capability is admitted.
func (c *CapabilityClient) Project(ctx context.Context, project string) (ProjectCapability, error) {
	if ctx == nil {
		return ProjectCapability{}, errors.New("OVH capability context is required")
	}
	if c == nil || c.api == nil {
		return ProjectCapability{}, errors.New("OVH capability client is required")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return ProjectCapability{}, errors.New("OVH capability project is required")
	}
	var capability ProjectCapability
	path := "/cloud/project/" + url.PathEscape(project)
	if err := c.api.GetWithContext(ctx, path, &capability); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return ProjectCapability{}, err
		}
		return ProjectCapability{}, errors.New("OVH project capability lookup failed")
	}
	if strings.TrimSpace(capability.ID) == "" {
		capability.ID = strings.TrimSpace(capability.ProjectID)
	}
	if strings.TrimSpace(capability.ID) == "" {
		capability.ID = strings.TrimSpace(capability.ServiceName)
	}
	if capability.ID == "" {
		return ProjectCapability{}, errors.New("OVH project capability response is empty")
	}
	return capability, nil
}

// Region reads one project region and its currently available zones. The
// lookup is read-only and must complete before any Pulumi resource mutation.
func (c *CapabilityClient) Region(ctx context.Context, project, region string) (RegionCapability, error) {
	if ctx == nil {
		return RegionCapability{}, errors.New("OVH capability context is required")
	}
	if c == nil || c.api == nil {
		return RegionCapability{}, errors.New("OVH capability client is required")
	}
	project = strings.TrimSpace(project)
	region = strings.TrimSpace(region)
	if project == "" || region == "" {
		return RegionCapability{}, errors.New("OVH capability project and region are required")
	}
	var capability RegionCapability
	path := "/cloud/project/" + url.PathEscape(project) + "/region/" + url.PathEscape(region)
	if err := c.api.GetWithContext(ctx, path, &capability); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return RegionCapability{}, err
		}
		return RegionCapability{}, errors.New("OVH region capability lookup failed")
	}
	if capability.Name == "" {
		capability.Name = capability.RegionName
	}
	return capability, nil
}

// DatabaseAvailability reads all currently admitted managed-database
// combinations for one Public Cloud project. The lookup is read-only and must
// complete before a database or Valkey resource is registered with Pulumi.
func (c *CapabilityClient) DatabaseAvailability(ctx context.Context, project string) ([]DatabaseAvailability, error) {
	if ctx == nil {
		return nil, errors.New("OVH capability context is required")
	}
	if c == nil || c.api == nil {
		return nil, errors.New("OVH capability client is required")
	}
	project = strings.TrimSpace(project)
	if project == "" {
		return nil, errors.New("OVH capability project is required")
	}
	var availability []DatabaseAvailability
	path := "/cloud/project/" + url.PathEscape(project) + "/database/availability"
	if err := c.api.GetWithContext(ctx, path, &availability); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, errors.New("OVH database availability lookup failed")
	}
	if availability == nil {
		availability = []DatabaseAvailability{}
	}
	return availability, nil
}
