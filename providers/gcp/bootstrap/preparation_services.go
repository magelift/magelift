package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/api/cloudresourcemanager/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	serviceusage "google.golang.org/api/serviceusage/v1"
)

const (
	serviceEnableAttempts = 6
	serviceEnableDelay    = 2 * time.Second
	serviceOperationWait  = 5 * time.Minute
	serviceUsageAPIName   = "serviceusage.googleapis.com"
)

// PreparationRequiredServices returns the APIs reference account prep enables.
func PreparationRequiredServices() []string {
	return []string{
		"artifactregistry.googleapis.com",
		"cloudbilling.googleapis.com",
		"cloudresourcemanager.googleapis.com",
		"compute.googleapis.com",
		"container.googleapis.com",
		"iam.googleapis.com",
		"iamcredentials.googleapis.com",
		"logging.googleapis.com",
		"memorystore.googleapis.com",
		"monitoring.googleapis.com",
		"networkconnectivity.googleapis.com",
		"secretmanager.googleapis.com",
		"servicenetworking.googleapis.com",
		"serviceusage.googleapis.com",
		"sqladmin.googleapis.com",
		"storage.googleapis.com",
		"sts.googleapis.com",
	}
}

// ServiceEnablementAPI enables missing project services idempotently.
type ServiceEnablementAPI interface {
	ProjectNumber(ctx context.Context, projectID string) (int64, error)
	EnabledServices(ctx context.Context, projectNumber int64, services []string) (map[string]bool, error)
	EnableService(ctx context.Context, projectNumber int64, service string) error
	Close() error
}

// NewServiceEnablementClient builds a live Service Usage + CRM client.
func NewServiceEnablementClient(ctx context.Context, opts ...option.ClientOption) (ServiceEnablementAPI, error) {
	scopes := append([]option.ClientOption{option.WithScopes("https://www.googleapis.com/auth/cloud-platform")}, opts...)
	services, err := serviceusage.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create Service Usage client: %w", err)
	}
	crm, err := cloudresourcemanager.NewService(ctx, scopes...)
	if err != nil {
		return nil, fmt.Errorf("create Resource Manager client: %w", err)
	}
	return &sdkServiceEnablement{services: services, crm: crm}, nil
}

type sdkServiceEnablement struct {
	services *serviceusage.Service
	crm      *cloudresourcemanager.Service
}

func (c *sdkServiceEnablement) Close() error {
	return nil
}

func (c *sdkServiceEnablement) ProjectNumber(ctx context.Context, projectID string) (int64, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return 0, errors.New("GCP project ID is required")
	}
	project, err := c.crm.Projects.Get(projectID).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("read GCP project %q: %w", projectID, err)
	}
	if project == nil || project.ProjectNumber == 0 {
		return 0, fmt.Errorf("GCP project number is missing for project %q", projectID)
	}
	return project.ProjectNumber, nil
}

func (c *sdkServiceEnablement) EnabledServices(ctx context.Context, projectNumber int64, serviceNames []string) (map[string]bool, error) {
	if c == nil || c.services == nil {
		return nil, errors.New("Service Usage client is required")
	}
	if projectNumber <= 0 {
		return nil, errors.New("GCP project number is required")
	}
	parent := "projects/" + strconv.FormatInt(projectNumber, 10)
	result := make(map[string]bool, len(serviceNames))
	unique := make([]string, 0, len(serviceNames))
	seen := make(map[string]struct{}, len(serviceNames))
	for _, serviceName := range serviceNames {
		serviceName = strings.TrimSpace(serviceName)
		if serviceName == "" || strings.ContainsAny(serviceName, "/?#") {
			return nil, fmt.Errorf("invalid GCP service name %q", serviceName)
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
			return nil, fmt.Errorf("read enabled services for project number %d: %w", projectNumber, serviceUsageListActionableError(err))
		}
		if services == nil {
			return nil, errors.New("Service Usage returned no service states")
		}
		for _, service := range services.Services {
			if service == nil {
				continue
			}
			name := strings.TrimSpace(service.Name)
			if !strings.HasPrefix(name, parent+"/services/") {
				return nil, fmt.Errorf("Service Usage returned a service for another project")
			}
			serviceName := strings.TrimPrefix(name, parent+"/services/")
			if serviceName == "" || strings.Contains(serviceName, "/") {
				return nil, errors.New("Service Usage returned an invalid service name")
			}
			if _, ok := result[serviceName]; !ok {
				return nil, fmt.Errorf("Service Usage returned unexpected service %q", serviceName)
			}
			result[serviceName] = strings.EqualFold(strings.TrimSpace(service.State), "ENABLED")
		}
	}
	return result, nil
}

func (c *sdkServiceEnablement) EnableService(ctx context.Context, projectNumber int64, service string) error {
	service = strings.TrimSpace(service)
	if service == "" {
		return errors.New("GCP service name is required")
	}
	name := "projects/" + strconv.FormatInt(projectNumber, 10) + "/services/" + service
	var lastErr error
	for attempt := 1; attempt <= serviceEnableAttempts; attempt++ {
		op, err := c.services.Services.Enable(name, &serviceusage.EnableServiceRequest{}).Context(ctx).Do()
		if err != nil {
			lastErr = err
			if !isTransientServiceError(err) || attempt == serviceEnableAttempts {
				return fmt.Errorf("enable GCP service %q for project number %d: %w; ensure the caller has serviceusage.services.enable", service, projectNumber, err)
			}
			if err := waitPreparation(ctx, serviceEnableDelay); err != nil {
				return err
			}
			continue
		}
		if err := waitServiceUsageOperation(ctx, c.services, op); err != nil {
			lastErr = err
			if !isTransientServiceError(err) || attempt == serviceEnableAttempts {
				return fmt.Errorf("enable GCP service %q for project number %d: %w", service, projectNumber, err)
			}
			if err := waitPreparation(ctx, serviceEnableDelay); err != nil {
				return err
			}
			continue
		}
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("enable GCP service %q for project number %d: %w", service, projectNumber, lastErr)
	}
	return errors.New("enable GCP service retry loop exhausted")
}

// EnsureAPIs enables only missing services from the reference recipe.
func (p *AccountPreparer) EnsureAPIs(ctx context.Context, plan PreparationPlan, spec Spec) error {
	if p == nil || p.apis == nil {
		return errors.New("account preparer is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validatePreparationPlanScope(plan, spec); err != nil {
		return err
	}
	projectID := strings.TrimSpace(plan.Base.GCPProject)
	if projectID == "" {
		return errors.New("preparation plan GCP project is required")
	}
	projectNumber, err := p.apis.ProjectNumber(ctx, projectID)
	if err != nil {
		return err
	}
	services := PreparationRequiredServices()
	enabled, err := p.apis.EnabledServices(ctx, projectNumber, services)
	if err != nil {
		return err
	}
	for _, service := range services {
		if enabled[service] {
			continue
		}
		if err := p.apis.EnableService(ctx, projectNumber, service); err != nil {
			return err
		}
	}
	return nil
}

func waitServiceUsageOperation(ctx context.Context, svc *serviceusage.Service, op *serviceusage.Operation) error {
	if op == nil {
		return errors.New("Service Usage operation is required")
	}
	name := strings.TrimSpace(op.Name)
	if name == "" {
		if op.Done {
			return serviceUsageOperationError(op)
		}
		return errors.New("Service Usage operation name is missing")
	}
	deadline := time.Now().Add(serviceOperationWait)
	current := op
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if current.Done {
			return serviceUsageOperationError(current)
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timed out waiting for Service Usage operation %q", name)
		}
		next, err := svc.Operations.Get(name).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("poll Service Usage operation %q: %w", name, err)
		}
		current = next
		if err := waitPreparation(ctx, 2*time.Second); err != nil {
			return err
		}
	}
}

func serviceUsageOperationError(op *serviceusage.Operation) error {
	if op == nil || !op.Done {
		return nil
	}
	if op.Error != nil && strings.TrimSpace(op.Error.Message) != "" {
		return fmt.Errorf("Service Usage operation failed: %s", op.Error.Message)
	}
	return nil
}

func serviceUsageListActionableError(err error) error {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	if apiErr.Code != http.StatusForbidden && apiErr.Code != http.StatusServiceUnavailable {
		return err
	}
	for _, detail := range apiErr.Errors {
		reason := strings.TrimSpace(detail.Reason)
		if reason == "SERVICE_DISABLED" || reason == "accessNotConfigured" {
			return fmt.Errorf(
				"enable %s on the GCP project (or grant serviceusage.services.get) before account preparation: %w",
				serviceUsageAPIName, err,
			)
		}
	}
	return err
}

func isTransientServiceError(err error) bool {
	var apiErr *googleapi.Error
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.Code {
	case http.StatusTooManyRequests, http.StatusInternalServerError,
		http.StatusBadGateway, http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		return false
	}
}

func waitPreparation(ctx context.Context, delay time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}
