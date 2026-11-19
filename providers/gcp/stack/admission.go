package stack

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gcpprovider "github.com/magelift/magelift/providers/gcp/provider"
	gcptarget "github.com/magelift/magelift/providers/gcp/target"
)

type RegionAdmission struct {
	NewClient func(context.Context) (gcpprovider.CapabilityAPI, error)
}

// AdmitSpec resolves current GCP project, region, managed-service, and GKE
// catalog availability before any plan is stored. The protocol server calls
// it during the plan operation; it replaces the platform PlanAdmission hook.
func (a RegionAdmission) AdmitSpec(ctx context.Context, spec Spec) (Spec, error) {
	if ctx == nil {
		return Spec{}, errors.New("GCP plan admission context is required")
	}
	// Teardown reads an existing state snapshot and must remain possible when
	// current service versions or admission APIs are unavailable.
	if spec.AllowExpiredPreview {
		return spec, nil
	}
	clientFactory := a.NewClient
	if clientFactory == nil {
		clientFactory = func(clientContext context.Context) (gcpprovider.CapabilityAPI, error) {
			return gcpprovider.NewCapabilityClient(clientContext)
		}
	}
	client, err := clientFactory(ctx)
	if err != nil {
		return Spec{}, fmt.Errorf("create GCP read-only capability client: %w", err)
	}
	selection := selectionFromSpec(spec)
	if spec.AccountOnly {
		if err := gcpprovider.ValidateAccountPrep(ctx, client, selection); err != nil {
			return Spec{}, err
		}
		return spec, nil
	}
	if err := gcpprovider.ValidateSelection(ctx, client, selection); err != nil {
		return Spec{}, err
	}
	session := spec.Services["session"]
	if session.Family != "" && !serviceSelfOrAttached(session) && session.Version != "" {
		version := "VALKEY_" + strings.ReplaceAll(session.ManagedBranch(), ".", "_")
		if version != selection.MemorystoreVersion {
			selection.MemorystoreVersion = version
			if err := gcpprovider.ValidateSelection(ctx, client, selection); err != nil {
				return Spec{}, fmt.Errorf("session store: %w", err)
			}
		}
	}
	return spec, nil
}

func selectionFromSpec(spec Spec) gcpprovider.AdmissionSelection {
	runtimeID := spec.Identity.Runtime
	if runtimeID == "" {
		runtimeID = gcptarget.RuntimeAutopilotID
	}
	selection := gcpprovider.AdmissionSelection{
		ProjectID:          spec.Identity.GCPProject,
		SkipCloudSQL:       serviceSelfOrAttached(spec.Services["database"]),
		SkipMemorystore:    serviceSelfOrAttached(spec.Services["cache"]) && (spec.Services["session"].Family == "" || serviceSelfOrAttached(spec.Services["session"])),
		ExistingCluster:    spec.PreviewPool != nil,
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
	session := spec.Services["session"]
	if serviceSelfOrAttached(spec.Services["cache"]) && session.Family != "" && !serviceSelfOrAttached(session) && session.Version != "" {
		selection.MemorystoreVersion = "VALKEY_" + strings.ReplaceAll(session.ManagedBranch(), ".", "_")
	}
	if version, err := spec.CloudSQLDatabaseVersion(); err == nil {
		selection.CloudSQLVersion = version
	}
	return selection
}

func requiredServices(spec Spec) []string {
	services := []string{
		"artifactregistry.googleapis.com",
		"compute.googleapis.com",
		"container.googleapis.com",
		"cloudbilling.googleapis.com",
		"secretmanager.googleapis.com",
		"serviceusage.googleapis.com",
		"storage.googleapis.com",
	}
	if !serviceSelfOrAttached(spec.Services["database"]) {
		services = append(services, "sqladmin.googleapis.com")
	}
	if !serviceSelfOrAttached(spec.Services["cache"]) || (spec.Services["session"].Family != "" && !serviceSelfOrAttached(spec.Services["session"])) {
		services = append(services, "memorystore.googleapis.com")
	}
	if spec.Observability.NativeProvider == "google-cloud-operations" {
		services = append(services, "logging.googleapis.com", "monitoring.googleapis.com")
	}
	return services
}
