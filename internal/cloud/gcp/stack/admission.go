package stack

import (
	"context"
	"errors"
	"fmt"

	gcpprovider "github.com/magelift/magelift/internal/cloud/gcp/provider"
	gcptarget "github.com/magelift/magelift/internal/cloud/gcp/target"
	"github.com/magelift/magelift/internal/platform"
)

type RegionAdmission struct {
	NewClient func(context.Context) (gcpprovider.CapabilityAPI, error)
}

var _ platform.PlanAdmission = RegionAdmission{}

func (a RegionAdmission) Admit(ctx context.Context, planned platform.PlannedStack) (platform.PlannedStack, error) {
	if ctx == nil {
		return nil, errors.New("GCP plan admission context is required")
	}
	value, ok := planned.(Planned)
	if !ok {
		return nil, fmt.Errorf("GCP plan admission received unexpected planned type %T", planned)
	}
	clientFactory := a.NewClient
	if clientFactory == nil {
		clientFactory = func(clientContext context.Context) (gcpprovider.CapabilityAPI, error) {
			return gcpprovider.NewCapabilityClient(clientContext)
		}
	}
	client, err := clientFactory(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCP read-only capability client: %w", err)
	}
	if err := gcpprovider.ValidateSelection(ctx, client, selectionFromSpec(value.Spec)); err != nil {
		return nil, err
	}
	return value, nil
}

func selectionFromSpec(spec Spec) gcpprovider.AdmissionSelection {
	runtimeID := spec.Identity.Runtime
	if runtimeID == "" {
		runtimeID = gcptarget.RuntimeAutopilotID
	}
	selection := gcpprovider.AdmissionSelection{
		ProjectID:          spec.Identity.GCPProject,
		Region:             spec.Identity.Region,
		Zones:              append([]string(nil), spec.Policy.Zones...),
		RequiredServices:   requiredServices(spec),
		CloudSQLTier:       spec.Catalog.CloudSQLTier,
		MemorystoreVersion: spec.Catalog.MemorystoreEngineVersion,
		MemorystoreNode:    spec.Catalog.MemorystoreNodeType,
		MemorystoreMode:    spec.Catalog.MemorystoreMode,
		MemorystoreZone:    spec.Catalog.MemorystoreZone,
		KubernetesVersion:  spec.Catalog.KubernetesVersion,
		ReleaseChannel:     spec.Catalog.ReleaseChannel,
		MachineType:        spec.Catalog.StandardNodeType,
		StandardNodeCount:  spec.Catalog.StandardNodeCount,
		StandardNodeMax:    spec.Catalog.StandardNodeMaxCount,
		Runtime:            string(runtimeID),
	}
	if version, err := spec.CloudSQLDatabaseVersion(); err == nil {
		selection.CloudSQLVersion = version
	}
	return selection
}

func requiredServices(spec Spec) []string {
	services := []string{
		"compute.googleapis.com",
		"container.googleapis.com",
		"cloudbilling.googleapis.com",
		"memorystore.googleapis.com",
		"secretmanager.googleapis.com",
		"serviceusage.googleapis.com",
		"sqladmin.googleapis.com",
		"storage.googleapis.com",
	}
	if spec.Observability.NativeProvider == "google-cloud-operations" {
		services = append(services, "logging.googleapis.com", "monitoring.googleapis.com")
	}
	return services
}
