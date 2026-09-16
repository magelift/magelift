package certification

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/magelift/magelift/sdk"
)

// ArchitectureFamily describes a family of deployable shapes without
// embedding a provider SDK type. A provider adapter translates one selected
// mode into its SDK request; the family remains useful for planning, coverage,
// and community implementations.
type ArchitectureFamily struct {
	ID                    string           `json:"id" yaml:"id"`
	Provider              string           `json:"provider" yaml:"provider"`
	Runtime               string           `json:"runtime" yaml:"runtime"`
	ControlPlaneOwnership string           `json:"controlPlaneOwnership" yaml:"controlPlaneOwnership"`
	ComputeModes          []string         `json:"computeModes" yaml:"computeModes"`
	KubernetesTopologies  []string         `json:"kubernetesTopologies,omitempty" yaml:"kubernetesTopologies,omitempty"`
	NetworkModes          []string         `json:"networkModes" yaml:"networkModes"`
	IngressModes          []string         `json:"ingressModes" yaml:"ingressModes"`
	NativeObservability   []string         `json:"nativeObservability,omitempty" yaml:"nativeObservability,omitempty"`
	NativeEdge            []string         `json:"nativeEdge,omitempty" yaml:"nativeEdge,omitempty"`
	CapabilityIDs         []string         `json:"capabilityIds" yaml:"capabilityIds"`
	Status                CapabilityStatus `json:"status" yaml:"status"`
	Reason                string           `json:"reason" yaml:"reason"`
}

// ArchitectureProfileInput contains the semantic choices shared by every
// provider. It intentionally has no AWS, GCP, Scaleway, OVHcloud, Fastly, or
// New Relic API fields.
type ArchitectureProfileInput struct {
	AccountOrProjectRef      string
	Region                   string
	Regions                  []string
	Zones                    []string
	ComputeMode              string
	KubernetesTopology       string
	NetworkMode              string
	NetworkProfile           string
	IngressMode              string
	ConfigurationFingerprint string
	Boundaries               []sdk.ServiceBoundaryIntent
	Edge                     sdk.EdgeIntent
	Observability            sdk.ObservabilityIntent
	Resilience               sdk.ResilienceIntent
	ArtifactDigest           string
	SchemaFingerprint        string
	MigrationFingerprint     string
	OwnershipMarker          string
}

// ArchitectureCoverageRow is the generated, source-linked view of one
// architecture family. A non-certified family always carries both a reason
// and the provider records that led to the classification.
type ArchitectureCoverageRow struct {
	FamilyID      string             `json:"familyId" yaml:"familyId"`
	Provider      string             `json:"provider" yaml:"provider"`
	Runtime       string             `json:"runtime" yaml:"runtime"`
	Status        CapabilityStatus   `json:"status" yaml:"status"`
	ComputeModes  []string           `json:"computeModes" yaml:"computeModes"`
	CapabilityIDs []string           `json:"capabilityIds" yaml:"capabilityIds"`
	Sources       []CapabilitySource `json:"sources" yaml:"sources"`
	Reasons       []string           `json:"reasons" yaml:"reasons"`
}

// ArchitectureCoverageReport is intentionally a value object. It can be
// serialized by a CLI or documentation generator without importing any cloud
// SDK and remains useful to community provider implementations.
type ArchitectureCoverageReport struct {
	CatalogVersion string                    `json:"catalogVersion" yaml:"catalogVersion"`
	GeneratedAt    string                    `json:"generatedAt" yaml:"generatedAt"`
	Counts         map[CapabilityStatus]int  `json:"counts" yaml:"counts"`
	Rows           []ArchitectureCoverageRow `json:"rows" yaml:"rows"`
}

// ArchitectureCoverage generates the complete family report from the
// source-dated capability catalog. It fails rather than omitting a family or
// silently downgrading an unavailable capability to a neighboring provider.
func ArchitectureCoverage(catalog CapabilityCatalog, now time.Time) (ArchitectureCoverageReport, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if err := catalog.Validate(now); err != nil {
		return ArchitectureCoverageReport{}, err
	}
	report := ArchitectureCoverageReport{CatalogVersion: catalog.Version, GeneratedAt: now.UTC().Format(time.RFC3339), Counts: make(map[CapabilityStatus]int)}
	for _, family := range CurrentArchitectureFamilies() {
		if err := family.Validate(catalog); err != nil {
			return ArchitectureCoverageReport{}, err
		}
		row := ArchitectureCoverageRow{
			FamilyID: family.ID, Provider: family.Provider, Runtime: family.Runtime,
			Status: family.Status, ComputeModes: append([]string(nil), family.ComputeModes...),
			CapabilityIDs: append([]string(nil), family.CapabilityIDs...), Reasons: []string{family.Reason},
		}
		for _, capabilityID := range family.CapabilityIDs {
			record, _ := catalog.FindID(capabilityID)
			row.Sources = append(row.Sources, record.Source)
			if record.Reason != "" && record.Reason != family.Reason {
				row.Reasons = append(row.Reasons, record.ID+": "+record.Reason)
			}
			row.Status = mergeArchitectureStatus(row.Status, record.effectiveStatus())
		}
		row.Reasons = uniqueStrings(row.Reasons)
		report.Counts[row.Status]++
		report.Rows = append(report.Rows, row)
	}
	sort.Slice(report.Rows, func(i, j int) bool { return report.Rows[i].FamilyID < report.Rows[j].FamilyID })
	return report, nil
}

func mergeArchitectureStatus(current, candidate CapabilityStatus) CapabilityStatus {
	priority := map[CapabilityStatus]int{
		CapabilityCertified: 0, CapabilitySupported: 1, CapabilityCompatible: 1,
		CapabilityExperimental: 2, CapabilityNotRun: 2, CapabilityUnsupported: 3,
		CapabilityUnavailable: 4, CapabilityBlocked: 5,
	}
	if priority[candidate] > priority[current] {
		return candidate
	}
	return current
}

// CurrentArchitectureFamilies is the source-of-truth list for the supported
// architecture axes. It deliberately includes modes that are provider-
// available but not yet MageLift-certified.
func CurrentArchitectureFamilies() []ArchitectureFamily {
	families := []ArchitectureFamily{
		{
			ID: "aws.ecs", Provider: "aws", Runtime: "ecs-fargate", ControlPlaneOwnership: "managed",
			ComputeModes: []string{"ec2-asg", "fargate", "fargate-spot", "managed-instances"},
			NetworkModes: []string{"private"}, IngressModes: []string{"load-balancer"},
			NativeObservability: []string{"cloudwatch"}, NativeEdge: []string{"cloudfront-waf"},
			CapabilityIDs: []string{"aws.ecs.fargate", "aws.ecs.fargate-spot", "aws.ecs.ec2-asg", "aws.ecs.managed-instances", "aws.rds.mysql", "aws.elasticache.valkey", "aws.opensearch", "aws.amazon-mq.rabbitmq", "aws.s3", "aws.secrets-manager", "aws.cloudwatch", "aws.cloudfront", "aws.waf"},
			Status:        CapabilityExperimental, Reason: "ECS capacity modes share the portable contract but require independent placement, interruption, and teardown evidence.",
		},
		{
			ID: "aws.eks", Provider: "aws", Runtime: "eks", ControlPlaneOwnership: "managed",
			ComputeModes:         []string{"auto-mode", "fargate", "managed-node-groups", "self-managed"},
			KubernetesTopologies: []string{"managed-control-plane", "multi-zone"},
			NetworkModes:         []string{"private"}, IngressModes: []string{"load-balancer", "ingress"},
			NativeObservability: []string{"cloudwatch"}, NativeEdge: []string{"cloudfront-waf"},
			CapabilityIDs: []string{"aws.eks.managed-control-plane", "aws.eks.managed-node-groups", "aws.eks.auto-mode", "aws.eks.fargate", "aws.eks.self-managed", "aws.eks.cloudwatch-observability", "aws.rds.mysql", "aws.elasticache.valkey", "aws.opensearch", "aws.amazon-mq.rabbitmq", "aws.s3", "aws.secrets-manager", "aws.cloudwatch", "aws.cloudfront", "aws.waf"},
			Status:        CapabilityExperimental, Reason: "EKS compute and workload scheduling modes are separate architecture boundaries; no mode inherits another mode's certification.",
		},
		{
			ID: "gcp.gke", Provider: "gcp", Runtime: "gke", ControlPlaneOwnership: "managed",
			ComputeModes: []string{"autopilot", "standard"}, KubernetesTopologies: []string{"regional", "zonal"},
			NetworkModes: []string{"private"}, IngressModes: []string{"load-balancer", "ingress"},
			NativeObservability: []string{"google-cloud-operations"}, NativeEdge: []string{"google-cloud-load-balancing", "cloud-cdn", "cloud-armor"},
			CapabilityIDs: []string{"gcp.gke.autopilot", "gcp.gke.standard", "gcp.cloud-sql.mysql", "gcp.memorystore.valkey-9.0", "gcp.memorystore.valkey-9.1", "gcp.cloud-storage", "gcp.secret-manager", "gcp.cloud-observability", "gcp.load-balancing-cdn-armor"},
			Status:        CapabilityExperimental, Reason: "GKE Autopilot and Standard expose different workload, node, storage, and disruption controls.",
		},
		{
			ID: "scaleway.kapsule", Provider: "scaleway", Runtime: "kapsule", ControlPlaneOwnership: "managed",
			ComputeModes: []string{"managed-node-pools", "self-managed-workloads"}, KubernetesTopologies: []string{"multi-zone"},
			NetworkModes: []string{"private"}, IngressModes: []string{"load-balancer", "ingress"},
			NativeObservability: []string{"scaleway-cockpit"}, NativeEdge: []string{"scaleway-edge-services", "scaleway-load-balancer"},
			CapabilityIDs: []string{"scaleway.kapsule", "scaleway.managed-mysql", "scaleway.managed-redis", "scaleway.object-storage", "scaleway.secret-manager", "scaleway.search", "scaleway.queue", "scaleway.cockpit", "scaleway.load-balancer", "scaleway.edge-services"},
			Status:        CapabilityExperimental, Reason: "Kapsule is managed Kubernetes; missing native search and queue products remain explicit unavailable boundaries.",
		},
		{
			ID: "ovh.mks", Provider: "ovh", Runtime: "mks", ControlPlaneOwnership: "managed",
			ComputeModes: []string{"managed-node-pools", "self-managed-workloads"}, KubernetesTopologies: []string{"multi-zone"},
			NetworkModes: []string{"private"}, IngressModes: []string{"load-balancer", "ingress"},
			NativeObservability: []string{"ovh-logs-data-platform"}, NativeEdge: []string{"ovh-public-cloud-load-balancer", "ovh-cdn"},
			CapabilityIDs: []string{"ovh.mks", "ovh.managed-database", "ovh.managed-valkey", "ovh.object-storage", "ovh.secret-manager", "ovh.kms", "ovh.search", "ovh.queue", "ovh.logs-data-platform", "ovh.public-cloud-load-balancer", "ovh.cdn"},
			Status:        CapabilityExperimental, Reason: "MKS control-plane ownership and Public Cloud service boundaries require provider-specific recovery and network evidence.",
		},
	}
	for i := range families {
		families[i] = copyArchitectureFamily(families[i])
	}
	sort.Slice(families, func(i, j int) bool { return families[i].ID < families[j].ID })
	return families
}

func copyArchitectureFamily(family ArchitectureFamily) ArchitectureFamily {
	family.ComputeModes = sdk.SortedStrings(family.ComputeModes)
	family.KubernetesTopologies = sdk.SortedStrings(family.KubernetesTopologies)
	family.NetworkModes = sdk.SortedStrings(family.NetworkModes)
	family.IngressModes = sdk.SortedStrings(family.IngressModes)
	family.NativeObservability = sdk.SortedStrings(family.NativeObservability)
	family.NativeEdge = sdk.SortedStrings(family.NativeEdge)
	family.CapabilityIDs = sdk.SortedStrings(family.CapabilityIDs)
	return family
}

func (f ArchitectureFamily) Validate(catalog CapabilityCatalog) error {
	if strings.TrimSpace(f.ID) == "" || strings.TrimSpace(f.Provider) == "" || strings.TrimSpace(f.Runtime) == "" || strings.TrimSpace(f.ControlPlaneOwnership) == "" {
		return errors.New("architecture family ID, provider, runtime, and control-plane ownership are required")
	}
	if len(f.ComputeModes) == 0 || len(f.NetworkModes) == 0 || len(f.IngressModes) == 0 {
		return fmt.Errorf("architecture family %q must declare compute, network, and ingress modes", f.ID)
	}
	if !validCapabilityStatus(f.Status) {
		return fmt.Errorf("architecture family %q has invalid status %q", f.ID, f.Status)
	}
	if f.Status != CapabilityCertified && strings.TrimSpace(f.Reason) == "" {
		return fmt.Errorf("architecture family %q requires a reason for its non-certified status", f.ID)
	}
	if err := catalog.Validate(catalogTime(catalog)); err != nil {
		return err
	}
	for _, capabilityID := range f.CapabilityIDs {
		record, ok := catalog.FindID(capabilityID)
		if !ok {
			return fmt.Errorf("architecture family %q references missing capability %q", f.ID, capabilityID)
		}
		if record.Provider != f.Provider && record.Provider != "fastly" && record.Provider != "newrelic" {
			return fmt.Errorf("architecture family %q references capability %q from provider %q", f.ID, capabilityID, record.Provider)
		}
	}
	return nil
}

// Profile builds a validated profile for one selected family mode. It is
// side-effect free and is therefore safe to call before credentials or cloud
// clients are initialized.
func (f ArchitectureFamily) Profile(input ArchitectureProfileInput, catalog CapabilityCatalog) (ArchitectureProfile, error) {
	if err := f.Validate(catalog); err != nil {
		return ArchitectureProfile{}, err
	}
	computeMode := strings.TrimSpace(input.ComputeMode)
	if computeMode == "" {
		if len(f.ComputeModes) != 1 {
			return ArchitectureProfile{}, fmt.Errorf("architecture family %q requires an explicit compute mode", f.ID)
		}
		computeMode = f.ComputeModes[0]
	}
	if !containsString(f.ComputeModes, computeMode) {
		return ArchitectureProfile{}, fmt.Errorf("architecture family %q does not support compute mode %q", f.ID, computeMode)
	}
	if !containsString(f.NetworkModes, input.NetworkMode) {
		return ArchitectureProfile{}, fmt.Errorf("architecture family %q does not support network mode %q", f.ID, input.NetworkMode)
	}
	if !containsString(f.IngressModes, input.IngressMode) {
		return ArchitectureProfile{}, fmt.Errorf("architecture family %q does not support ingress mode %q", f.ID, input.IngressMode)
	}
	if input.KubernetesTopology != "" && !containsString(f.KubernetesTopologies, input.KubernetesTopology) {
		return ArchitectureProfile{}, fmt.Errorf("architecture family %q does not support Kubernetes topology %q", f.ID, input.KubernetesTopology)
	}
	if input.Edge.NativeProvider != "" && !containsString(f.NativeEdge, input.Edge.NativeProvider) {
		return ArchitectureProfile{}, fmt.Errorf("architecture family %q does not support native edge provider %q", f.ID, input.Edge.NativeProvider)
	}
	if input.Observability.NativeProvider != "" && !containsString(f.NativeObservability, input.Observability.NativeProvider) {
		return ArchitectureProfile{}, fmt.Errorf("architecture family %q does not support native observability provider %q", f.ID, input.Observability.NativeProvider)
	}
	profile := ArchitectureProfile{
		Version:           ArchitectureProfileVersion,
		CapabilityCatalog: catalog.Version,
		DeclaredStatus:    f.Status,
		Intent: sdk.ArchitectureIntent{
			ProfileID: f.ID + "." + computeMode,
			Provider:  sdk.ProviderID(f.Provider), Runtime: sdk.RuntimeID(f.Runtime),
			AccountOrProjectRef: input.AccountOrProjectRef, Region: input.Region,
			Regions: append([]string(nil), input.Regions...), Zones: append([]string(nil), input.Zones...),
			ComputeMode: computeMode, KubernetesMode: input.KubernetesTopology,
			NetworkMode: input.NetworkMode, NetworkProfile: input.NetworkProfile, IngressMode: input.IngressMode,
			Boundaries: append([]sdk.ServiceBoundaryIntent(nil), input.Boundaries...),
			Edge:       input.Edge, Observability: input.Observability, Resilience: input.Resilience,
			ConfigurationFingerprint: input.ConfigurationFingerprint, ArtifactDigest: input.ArtifactDigest, SchemaFingerprint: input.SchemaFingerprint,
			MigrationFingerprint: input.MigrationFingerprint, OwnershipMarker: input.OwnershipMarker,
		},
	}
	if err := profile.Validate(catalog); err != nil {
		return ArchitectureProfile{}, err
	}
	return profile, nil
}

func catalogTime(catalog CapabilityCatalog) time.Time {
	parsed, err := parseCatalogDate(catalog.UpdatedAt)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
