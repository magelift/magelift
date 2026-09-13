package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	scwprovider "github.com/magelift/magelift/internal/cloud/scaleway/provider"
	"github.com/magelift/magelift/internal/platform"
)

// RegionAdmission performs the Scaleway-specific, read-only checks that
// cannot be proven from YAML alone. The capability interface contains no
// mutation methods, so a successful admission cannot create or alter a
// provider resource by construction.
type RegionAdmission struct {
	NewClient func(context.Context, string, string, string) (scwprovider.CapabilityAPI, error)
	Now       func() time.Time
}

var _ platform.PlanAdmission = RegionAdmission{}

type capabilitySnapshot struct {
	kubernetesVersions     []scwprovider.KubernetesVersion
	kubernetesClusterTypes []scwprovider.KubernetesClusterType
	instanceTypes          map[string]scwprovider.InstanceType
	instanceAvailability   map[string]string
	databaseEngines        []scwprovider.DatabaseEngine
	databaseNodeTypes      []scwprovider.DatabaseNodeType
	redisVersions          []scwprovider.RedisVersion
	redisNodeTypes         []scwprovider.RedisNodeType
}

func (a RegionAdmission) Admit(ctx context.Context, planned platform.PlannedStack) (platform.PlannedStack, error) {
	if ctx == nil {
		return nil, errors.New("Scaleway plan admission context is required")
	}
	value, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("Scaleway plan admission received unexpected planned type %T", planned)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	clientFactory := a.NewClient
	if clientFactory == nil {
		clientFactory = func(ctx context.Context, projectID, region, zone string) (scwprovider.CapabilityAPI, error) {
			return scwprovider.NewCapabilityClient(ctx, projectID, region, zone)
		}
	}
	client, err := clientFactory(ctx, value.Spec.Identity.ScalewayProject, value.Spec.Identity.Region, value.Spec.Identity.Zone)
	if err != nil {
		return nil, errors.New("create authenticated Scaleway capability client")
	}
	project, err := client.Project(ctx, value.Spec.Identity.ScalewayProject)
	if err != nil {
		return nil, fmt.Errorf("read Scaleway project identity: %w", err)
	}
	if strings.TrimSpace(project.ID) != strings.TrimSpace(value.Spec.Identity.ScalewayProject) {
		return nil, fmt.Errorf("Scaleway credentials resolved to project %q, but the plan targets %q", project.ID, value.Spec.Identity.ScalewayProject)
	}
	snapshot, err := readCapabilities(ctx, client, value.Spec.Identity.Region, value.Spec.Identity.Zone)
	if err != nil {
		return nil, err
	}
	now := a.Now
	if now == nil {
		now = time.Now
	}
	if err := validateCapabilities(value.Spec, snapshot, now()); err != nil {
		return nil, err
	}
	return value, nil
}

func readCapabilities(ctx context.Context, client scwprovider.CapabilityAPI, region, zone string) (capabilitySnapshot, error) {
	if client == nil {
		return capabilitySnapshot{}, errors.New("Scaleway capability client is required")
	}
	readContext, cancel := context.WithCancel(ctx)
	defer cancel()

	var snapshot capabilitySnapshot
	errs := make(chan error, 8)
	var waitGroup sync.WaitGroup
	launch := func(read func(context.Context) error) {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			if err := read(readContext); err != nil {
				errs <- err
				cancel()
			}
		}()
	}
	launch(func(ctx context.Context) error {
		var err error
		snapshot.kubernetesVersions, err = client.KubernetesVersions(ctx, region)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.kubernetesClusterTypes, err = client.KubernetesClusterTypes(ctx, region)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.instanceTypes, err = client.InstanceTypes(ctx, zone)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.instanceAvailability, err = client.InstanceTypeAvailability(ctx, zone)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.databaseEngines, err = client.DatabaseEngines(ctx, region)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.databaseNodeTypes, err = client.DatabaseNodeTypes(ctx, region)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.redisVersions, err = client.RedisVersions(ctx, zone)
		return err
	})
	launch(func(ctx context.Context) error {
		var err error
		snapshot.redisNodeTypes, err = client.RedisNodeTypes(ctx, zone)
		return err
	})
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		return capabilitySnapshot{}, fmt.Errorf("read Scaleway capability catalog: %w", err)
	}
	return snapshot, nil
}

func validateCapabilities(spec Spec, snapshot capabilitySnapshot, now time.Time) error {
	if !strings.HasPrefix(spec.Identity.Zone, spec.Identity.Region+"-") {
		return fmt.Errorf("Scaleway zone %q is not in region %q", spec.Identity.Zone, spec.Identity.Region)
	}
	for _, zone := range spec.Policy.Zones {
		if !strings.HasPrefix(zone, spec.Identity.Region+"-") {
			return fmt.Errorf("Scaleway availability zone %q is not in region %q", zone, spec.Identity.Region)
		}
	}
	if spec.Catalog.NodeCount < len(spec.Policy.Zones) {
		return fmt.Errorf("Scaleway node count %d cannot place one node in each of %d availability zones", spec.Catalog.NodeCount, len(spec.Policy.Zones))
	}

	if err := validateKubernetes(spec, snapshot, now); err != nil {
		return err
	}
	if err := validateInstanceType(spec, snapshot); err != nil {
		return err
	}
	if err := validateDatabase(spec, snapshot, now); err != nil {
		return err
	}
	return validateRedis(spec, snapshot, now)
}

func validateKubernetes(spec Spec, snapshot capabilitySnapshot, now time.Time) error {
	version, found := findKubernetesVersion(snapshot.kubernetesVersions, spec.Catalog.KapsuleVersion)
	if !found {
		return fmt.Errorf("Scaleway Kapsule version %q is not available in region %q", spec.Catalog.KapsuleVersion, spec.Identity.Region)
	}
	if isPast(version.DeprecatedAt, now) || isPast(version.EndOfLifeAt, now) {
		return fmt.Errorf("Scaleway Kapsule version %q is deprecated or past end of life", spec.Catalog.KapsuleVersion)
	}
	clusterType, found := findClusterType(snapshot.kubernetesClusterTypes, "kapsule")
	if !found {
		return errors.New("Scaleway Kapsule cluster type is not available in the selected region")
	}
	if !isAvailable(clusterType.Availability) {
		return fmt.Errorf("Scaleway Kapsule cluster type is unavailable (status %q)", clusterType.Availability)
	}
	if clusterType.Region != "" && !strings.EqualFold(clusterType.Region, spec.Identity.Region) {
		return fmt.Errorf("Scaleway Kapsule cluster type is not available in region %q", spec.Identity.Region)
	}
	if clusterType.MaxNodes == 0 || uint32(spec.Catalog.NodeCount) > clusterType.MaxNodes {
		return fmt.Errorf("Scaleway Kapsule node count %d exceeds the admitted maximum of %d", spec.Catalog.NodeCount, clusterType.MaxNodes)
	}
	return nil
}

func validateInstanceType(spec Spec, snapshot capabilitySnapshot) error {
	nodeType, found := findInstanceType(snapshot.instanceTypes, spec.Catalog.NodeType)
	if !found {
		return fmt.Errorf("Scaleway Instance node type %q is not available in zone %q", spec.Catalog.NodeType, spec.Identity.Zone)
	}
	if nodeType.EndOfService {
		return fmt.Errorf("Scaleway Instance node type %q is past end of service", spec.Catalog.NodeType)
	}
	status, found := findFoldedValue(snapshot.instanceAvailability, spec.Catalog.NodeType)
	if !found || !isAvailable(status) {
		return fmt.Errorf("Scaleway Instance node type %q is unavailable (status %q)", spec.Catalog.NodeType, status)
	}
	return nil
}

func validateDatabase(spec Spec, snapshot capabilitySnapshot, now time.Time) error {
	engine, found := findDatabaseEngine(snapshot.databaseEngines, "MySQL")
	if !found {
		return errors.New("Scaleway MySQL-8 database engine is not available in the selected region")
	}
	version, found := findDatabaseEngineVersion(engine.Versions, "MySQL-8")
	if !found {
		return errors.New("Scaleway MySQL-8 database engine is not available in the selected region")
	}
	if version.Disabled || isPast(version.EndOfLife, now) {
		return errors.New("Scaleway MySQL-8 database engine has no active version in the selected region")
	}
	nodeType, found := findDatabaseNodeType(snapshot.databaseNodeTypes, spec.Catalog.DatabaseNodeType)
	if !found {
		return fmt.Errorf("Scaleway database node type %q is not available in region %q", spec.Catalog.DatabaseNodeType, spec.Identity.Region)
	}
	if nodeType.Disabled || !isAvailable(nodeType.StockStatus) {
		return fmt.Errorf("Scaleway database node type %q is unavailable (status %q)", spec.Catalog.DatabaseNodeType, nodeType.StockStatus)
	}
	if nodeType.Region != "" && !strings.EqualFold(nodeType.Region, spec.Identity.Region) {
		return fmt.Errorf("Scaleway database node type %q is not available in region %q", spec.Catalog.DatabaseNodeType, spec.Identity.Region)
	}
	if !spec.Catalog.DatabaseHighAvailability && nodeType.HARequired {
		return fmt.Errorf("Scaleway database node type %q requires high availability", spec.Catalog.DatabaseNodeType)
	}
	return nil
}

func validateRedis(spec Spec, snapshot capabilitySnapshot, now time.Time) error {
	version, found := findRedisVersion(snapshot.redisVersions, spec.Catalog.RedisVersion)
	if !found {
		return fmt.Errorf("Scaleway Redis version %q is not available in zone %q", spec.Catalog.RedisVersion, spec.Identity.Zone)
	}
	if isPast(version.EndOfLifeAt, now) {
		return fmt.Errorf("Scaleway Redis version %q is past end of life", spec.Catalog.RedisVersion)
	}
	nodeType, found := findRedisNodeType(snapshot.redisNodeTypes, spec.Catalog.RedisNodeType)
	if !found {
		return fmt.Errorf("Scaleway Redis node type %q is not available in zone %q", spec.Catalog.RedisNodeType, spec.Identity.Zone)
	}
	if nodeType.Disabled || !isAvailable(nodeType.StockStatus) {
		return fmt.Errorf("Scaleway Redis node type %q is unavailable (status %q)", spec.Catalog.RedisNodeType, nodeType.StockStatus)
	}
	if nodeType.Zone != "" && !strings.EqualFold(nodeType.Zone, spec.Identity.Zone) {
		return fmt.Errorf("Scaleway Redis node type %q is not available in zone %q", spec.Catalog.RedisNodeType, spec.Identity.Zone)
	}
	return nil
}

func findKubernetesVersion(values []scwprovider.KubernetesVersion, name string) (scwprovider.KubernetesVersion, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.KubernetesVersion{}, false
}

func findClusterType(values []scwprovider.KubernetesClusterType, name string) (scwprovider.KubernetesClusterType, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.KubernetesClusterType{}, false
}

func findInstanceType(values map[string]scwprovider.InstanceType, name string) (scwprovider.InstanceType, bool) {
	for key, value := range values {
		if strings.EqualFold(key, name) || strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.InstanceType{}, false
}

func findFoldedValue(values map[string]string, name string) (string, bool) {
	for key, value := range values {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return "", false
}

func findDatabaseEngine(values []scwprovider.DatabaseEngine, name string) (scwprovider.DatabaseEngine, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.DatabaseEngine{}, false
}

func findDatabaseEngineVersion(values []scwprovider.DatabaseEngineVersion, name string) (scwprovider.DatabaseEngineVersion, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.DatabaseEngineVersion{}, false
}

func findDatabaseNodeType(values []scwprovider.DatabaseNodeType, name string) (scwprovider.DatabaseNodeType, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.DatabaseNodeType{}, false
}

func findRedisVersion(values []scwprovider.RedisVersion, name string) (scwprovider.RedisVersion, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Version, name) {
			return value, true
		}
	}
	return scwprovider.RedisVersion{}, false
}

func findRedisNodeType(values []scwprovider.RedisNodeType, name string) (scwprovider.RedisNodeType, bool) {
	for _, value := range values {
		if strings.EqualFold(value.Name, name) {
			return value, true
		}
	}
	return scwprovider.RedisNodeType{}, false
}

func isAvailable(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "available":
		return true
	default:
		return false
	}
}

func isPast(value *time.Time, now time.Time) bool {
	return value != nil && now.After(*value)
}
