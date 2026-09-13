package certification

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/magelift/magelift/internal/config"
)

type CapabilityStatus string

const (
	CapabilityCompatible   CapabilityStatus = "magelift-compatible"
	CapabilityExperimental CapabilityStatus = "experimental"
	CapabilityUnsupported  CapabilityStatus = "unsupported"
	CapabilityUnavailable  CapabilityStatus = "unavailable"
)

type CapabilityOption struct {
	ID     string           `json:"id" yaml:"id"`
	Status CapabilityStatus `json:"status" yaml:"status"`
	Notes  string           `json:"notes,omitempty" yaml:"notes,omitempty"`
}

// TargetDescriptor describes a provider architecture without turning its
// options into a certification claim. PASS evidence is the authority for that
// claim.
type TargetDescriptor struct {
	ID               string                        `json:"id" yaml:"id"`
	Provider         string                        `json:"provider" yaml:"provider"`
	Runtime          string                        `json:"runtime" yaml:"runtime"`
	Architecture     string                        `json:"architecture" yaml:"architecture"`
	ClaimTier        string                        `json:"claimTier" yaml:"claimTier"`
	RequiredReleases []string                      `json:"requiredReleases" yaml:"requiredReleases"`
	ComputeModes     []CapabilityOption            `json:"computeModes" yaml:"computeModes"`
	KubernetesModes  []CapabilityOption            `json:"kubernetesModes,omitempty" yaml:"kubernetesModes,omitempty"`
	Services         map[string][]CapabilityOption `json:"services" yaml:"services"`
	Notes            string                        `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type ArchitectureCell struct {
	ID                   string           `json:"id" yaml:"id"`
	Dimensions           Dimensions       `json:"dimensions" yaml:"dimensions"`
	Status               CapabilityStatus `json:"status" yaml:"status"`
	Required             bool             `json:"required" yaml:"required"`
	ExecutionFingerprint string           `json:"executionFingerprint,omitempty" yaml:"executionFingerprint,omitempty"`
	WarmBoundary         string           `json:"warmBoundary,omitempty" yaml:"warmBoundary,omitempty"`
	WarmFrom             string           `json:"warmFrom,omitempty" yaml:"warmFrom,omitempty"`
	Notes                string           `json:"notes,omitempty" yaml:"notes,omitempty"`
}

var matrixServices = []string{"database", "search", "queue", "cache", "webCache", "edge"}

var currentTargetMatrix = []TargetDescriptor{
	{
		ID: "aws/ecs-fargate", Provider: "aws", Runtime: "ecs-fargate", Architecture: "managed-ecs",
		ClaimTier: "certified", RequiredReleases: latestReleases(),
		ComputeModes: options(
			option("ec2-asg", CapabilityExperimental, "ECS EC2 Auto Scaling capacity; host lifecycle and capacity-provider evidence are independent from Fargate."),
			option("fargate", CapabilityCompatible, "ECS Fargate capacity."),
			option("fargate-spot", CapabilityExperimental, "ECS Fargate Spot interruption semantics require a separate resilience profile."),
			option("managed-instances", CapabilityExperimental, "ECS Managed Instances capacity; IAM, placement, and teardown evidence remain separate."),
		),
		KubernetesModes: options(option("none", CapabilityCompatible, "Not a Kubernetes runtime.")),
		Services: map[string][]CapabilityOption{
			"database": options(option("aurora-mysql", CapabilityCompatible, "Aurora MySQL"), option("rds-mysql", CapabilityCompatible, "RDS MySQL for preview"), option("rds-mariadb", CapabilityExperimental, "RDS MariaDB preview adapter; live certification pending")),
			"search":   options(option("serverless", CapabilityCompatible, "Amazon OpenSearch Serverless"), option("provisioned", CapabilityCompatible, "Amazon OpenSearch provisioned"), option("disabled", CapabilityCompatible, "No search service")),
			"queue":    options(option("db", CapabilityCompatible, "Database-backed messaging"), option("amazon-mq", CapabilityCompatible, "Amazon MQ RabbitMQ"), option("ecs-rabbitmq", CapabilityCompatible, "RabbitMQ on ECS"), option("ecs-artemis", CapabilityExperimental, "ActiveMQ Artemis on ECS")),
			"cache":    options(option("valkey", CapabilityCompatible, "Managed Valkey")),
			"webCache": options(option("varnish", CapabilityCompatible, "Pinned Varnish sidecar for integrated mode"), option("none", CapabilityCompatible, "Headless mode")),
			"edge":     options(option("none", CapabilityCompatible, "AWS CloudFront and WAF path"), option("fastly", CapabilityExperimental, "Registered Fastly lifecycle is experimental; full routed production evidence remains pending")),
		},
	},
	{
		ID: "aws/eks", Provider: "aws", Runtime: "eks", Architecture: "managed-kubernetes",
		ClaimTier: "experimental", RequiredReleases: latestReleases(),
		ComputeModes: options(
			option("auto-mode", CapabilityExperimental, "EKS Auto Mode."),
			option("fargate", CapabilityExperimental, "EKS Fargate pods; persistent workload boundaries are explicit."),
			option("managed-node-groups", CapabilityExperimental, "EKS managed node groups."),
			option("self-managed", CapabilityExperimental, "EKS self-managed nodes with pinned bootstrap inputs."),
		),
		KubernetesModes: options(
			option("managed-control-plane", CapabilityCompatible, "AWS-managed EKS control plane."),
			option("multi-zone", CapabilityExperimental, "Multi-zone control-plane and workload placement evidence."),
		),
		Services: map[string][]CapabilityOption{
			"database": options(option("aurora-mysql", CapabilityCompatible, "Aurora MySQL"), option("rds-mysql", CapabilityCompatible, "RDS MySQL for preview"), option("rds-mariadb", CapabilityExperimental, "RDS MariaDB preview adapter; live certification pending")),
			"search":   options(option("opensearch", CapabilityExperimental, "OpenSearch workload or provider adapter"), option("disabled", CapabilityCompatible, "No search service")),
			"queue":    options(option("database", CapabilityCompatible, "Database-backed messaging"), option("rabbitmq", CapabilityExperimental, "RabbitMQ workload"), option("artemis", CapabilityUnavailable, "No EKS ActiveMQ Artemis adapter yet")),
			"cache":    options(option("valkey", CapabilityCompatible, "Managed Valkey")),
			"webCache": options(option("varnish", CapabilityExperimental, "Requires Kubernetes Varnish deployment evidence"), option("none", CapabilityCompatible, "Headless mode")),
			"edge":     options(option("none", CapabilityCompatible, "Provider edge path"), option("fastly", CapabilityExperimental, "Registered Fastly lifecycle is experimental; full routed production evidence remains pending")),
		},
		Notes: "The target graph includes Auto Mode, shared Kubernetes deploy/observe ports, OpenSearch, and RabbitMQ; live Magento acceptance and cleanup evidence are still pending.",
	},
	{
		ID: "gcp/gke-autopilot", Provider: "gcp", Runtime: "gke-autopilot", Architecture: "managed-kubernetes",
		ClaimTier: "certified", RequiredReleases: latestReleases(),
		ComputeModes: options(option("autopilot", CapabilityCompatible, "GKE Autopilot workload management.")),
		KubernetesModes: options(
			option("zonal", CapabilityCompatible, "Zonal GKE control plane."),
			option("regional", CapabilityExperimental, "Regional GKE control plane; live cell evidence remains separate."),
		),
		Services: map[string][]CapabilityOption{
			"database": options(option("cloud-sql-mysql", CapabilityCompatible, "Cloud SQL for MySQL")),
			"search":   options(option("opensearch", CapabilityExperimental, "OpenSearch workload"), option("disabled", CapabilityCompatible, "No search service")),
			"queue":    options(option("database", CapabilityCompatible, "Database-backed messaging"), option("rabbitmq", CapabilityExperimental, "RabbitMQ workload")),
			"cache":    options(option("valkey", CapabilityCompatible, "Memorystore for Valkey 9.0 GA; this is the default for Adobe Commerce 2.4.9"), option("valkey-9.1-preview", CapabilityExperimental, "Memorystore for Valkey 9.1 Preview; selectable only as an experimental profile")),
			"webCache": options(option("none", CapabilityCompatible, "No first-party Varnish layer on the current GKE runtime"), option("varnish", CapabilityExperimental, "Requires Kubernetes Varnish deployment evidence")),
			"edge":     options(option("cloud-armor", CapabilityCompatible, "Google Cloud Armor"), option("none", CapabilityCompatible, "No managed edge policy"), option("fastly", CapabilityExperimental, "Registered Fastly lifecycle is experimental; full routed production evidence remains pending")),
		},
	},
	{
		ID: "gcp/gke-standard", Provider: "gcp", Runtime: "gke-standard", Architecture: "managed-kubernetes",
		ClaimTier: "experimental", RequiredReleases: latestReleases(),
		ComputeModes: options(option("standard", CapabilityExperimental, "GKE Standard node-managed workload runtime.")),
		KubernetesModes: options(
			option("zonal", CapabilityExperimental, "Zonal GKE Standard control plane."),
			option("regional", CapabilityExperimental, "Regional GKE Standard control plane."),
		),
		Services: map[string][]CapabilityOption{
			"database": options(option("cloud-sql-mysql", CapabilityCompatible, "Cloud SQL for MySQL")),
			"search":   options(option("opensearch", CapabilityExperimental, "OpenSearch 3 StatefulSet with node-level kernel tuning"), option("disabled", CapabilityCompatible, "No search service")),
			"queue":    options(option("database", CapabilityCompatible, "Database-backed messaging"), option("rabbitmq", CapabilityExperimental, "RabbitMQ workload")),
			"cache":    options(option("valkey", CapabilityCompatible, "Memorystore for Valkey 9.0 GA; this is the default for Adobe Commerce 2.4.9"), option("valkey-9.1-preview", CapabilityExperimental, "Memorystore for Valkey 9.1 Preview; selectable only as an experimental profile")),
			"webCache": options(option("none", CapabilityCompatible, "No first-party Varnish layer on the current GKE runtime"), option("varnish", CapabilityExperimental, "Requires Kubernetes Varnish deployment evidence")),
			"edge":     options(option("cloud-armor", CapabilityCompatible, "Google Cloud Armor"), option("none", CapabilityCompatible, "No managed edge policy"), option("fastly", CapabilityExperimental, "Registered Fastly lifecycle is experimental; full routed production evidence remains pending")),
		},
		Notes: "Use this runtime for GKE workloads that need node-level settings, including multi-node OpenSearch. The target remains experimental until a current Adobe release line passes live acceptance.",
	},
	{
		ID: "ovh/mks", Provider: "ovh", Runtime: "mks", Architecture: "managed-kubernetes",
		ClaimTier: "experimental", RequiredReleases: latestReleases(),
		ComputeModes: options(
			option("managed-node-pools", CapabilityExperimental, "OVHcloud managed Kubernetes node pools."),
			option("self-managed-workloads", CapabilityExperimental, "Self-managed workload stateful services on MKS."),
		),
		KubernetesModes: options(option("multi-zone", CapabilityExperimental, "Multi-zone MKS workload placement.")),
		Services: map[string][]CapabilityOption{
			"database": options(option("mysql", CapabilityCompatible, "OVH managed MySQL")),
			"search":   options(option("opensearch", CapabilityUnavailable, "No first-party managed or workload adapter is certified yet"), option("disabled", CapabilityCompatible, "No search service")),
			"queue":    options(option("database", CapabilityCompatible, "Database-backed messaging"), option("rabbitmq", CapabilityExperimental, "Requires workload deployment evidence"), option("artemis", CapabilityUnavailable, "No first-party adapter yet")),
			"cache":    options(option("valkey", CapabilityCompatible, "OVH managed Valkey")),
			"webCache": options(option("none", CapabilityCompatible, "No first-party Varnish layer on the current MKS runtime"), option("varnish", CapabilityExperimental, "Requires Kubernetes Varnish deployment evidence")),
			"edge":     options(option("none", CapabilityCompatible, "Provider edge path"), option("fastly", CapabilityExperimental, "Registered Fastly lifecycle is experimental; full routed production evidence remains pending")),
		},
		Notes: "The target is intentionally experimental until a real-account Magento acceptance run passes.",
	},
	{
		ID: "scaleway/kapsule", Provider: "scaleway", Runtime: "kapsule", Architecture: "managed-kubernetes",
		ClaimTier: "experimental", RequiredReleases: latestReleases(),
		ComputeModes: options(
			option("managed-node-pools", CapabilityExperimental, "Scaleway managed Kapsule node pools."),
			option("self-managed-workloads", CapabilityExperimental, "Self-managed workload stateful services on Kapsule."),
		),
		KubernetesModes: options(option("multi-zone", CapabilityExperimental, "Multi-zone Kapsule workload placement.")),
		Services: map[string][]CapabilityOption{
			"database": options(option("mysql", CapabilityCompatible, "Scaleway managed MySQL")),
			"search":   options(option("opensearch", CapabilityUnavailable, "No first-party search adapter is certified yet"), option("disabled", CapabilityCompatible, "No search service")),
			"queue":    options(option("database", CapabilityCompatible, "Database-backed messaging"), option("rabbitmq", CapabilityExperimental, "Requires workload deployment evidence"), option("artemis", CapabilityUnavailable, "No first-party adapter yet")),
			"cache":    options(option("redis", CapabilityCompatible, "Scaleway managed Redis; Adobe latest-patch Valkey rows remain separate"), option("valkey", CapabilityUnavailable, "Scaleway managed Valkey is not available in the current adapter")),
			"webCache": options(option("none", CapabilityCompatible, "No first-party Varnish layer on the current Kapsule runtime"), option("varnish", CapabilityExperimental, "Requires Kubernetes Varnish deployment evidence")),
			"edge":     options(option("none", CapabilityCompatible, "Provider edge path"), option("fastly", CapabilityExperimental, "Registered Fastly lifecycle is experimental; full routed production evidence remains pending")),
		},
		Notes: "The target is intentionally experimental until a real-account Magento acceptance run passes.",
	},
}

func CurrentTargetMatrix() []TargetDescriptor {
	result := make([]TargetDescriptor, 0, len(currentTargetMatrix))
	for _, target := range currentTargetMatrix {
		copyTarget := target
		copyTarget.RequiredReleases = append([]string(nil), target.RequiredReleases...)
		copyTarget.ComputeModes = append([]CapabilityOption(nil), target.ComputeModes...)
		copyTarget.KubernetesModes = append([]CapabilityOption(nil), target.KubernetesModes...)
		copyTarget.Services = make(map[string][]CapabilityOption, len(target.Services))
		for service, values := range target.Services {
			copyTarget.Services[service] = append([]CapabilityOption(nil), values...)
		}
		result = append(result, copyTarget)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

// CellsForTarget expands a target's service options into stable, inspectable
// architecture cells. Unsupported and unavailable options remain in the
// output so the matrix is explicit about what was not certified.
func CellsForTarget(targetID, release, edition, preset string) ([]ArchitectureCell, error) {
	target, found := targetByID(targetID)
	if !found {
		return nil, fmt.Errorf("unknown certification target %q", targetID)
	}
	if !contains(target.RequiredReleases, release) {
		return nil, fmt.Errorf("release %q is not in the target catalog", release)
	}
	if edition != "open-source" && edition != "commerce" {
		return nil, fmt.Errorf("unsupported edition %q", edition)
	}
	if strings.TrimSpace(preset) == "" {
		return nil, fmt.Errorf("preset is required")
	}

	computeModes := target.ComputeModes
	if len(computeModes) == 0 {
		computeModes = options(option("none", CapabilityCompatible, "No separate compute mode was declared by this target."))
	}
	kubernetesModes := target.KubernetesModes
	if len(kubernetesModes) == 0 {
		kubernetesModes = options(option("none", CapabilityCompatible, "No Kubernetes topology was declared by this target."))
	}

	var cells []ArchitectureCell
	var expandServices func(int, map[string]string, []CapabilityOption, CapabilityOption, CapabilityOption)
	expandServices = func(index int, selected map[string]string, choices []CapabilityOption, computeMode, kubernetesMode CapabilityOption) {
		if index == len(matrixServices) {
			dimensions := Dimensions{
				Provider: target.Provider, Runtime: target.Runtime, ComputeMode: computeMode.ID, KubernetesMode: kubernetesMode.ID, Release: release,
				Edition: edition, Preset: preset, Database: selected["database"],
				Search: selected["search"], Queue: selected["queue"], Cache: selected["cache"],
				WebCache: selected["webCache"], Edge: selected["edge"], Scenario: "architecture",
			}
			status := targetStatus(target, append(append([]CapabilityOption{}, computeMode, kubernetesMode), choices...))
			var notes []string
			for _, choice := range []CapabilityOption{computeMode, kubernetesMode} {
				if choice.Notes != "" {
					notes = append(notes, choice.Notes)
				}
			}
			for _, choice := range choices {
				if choice.Notes != "" {
					notes = append(notes, choice.Notes)
				}
			}
			cells = append(cells, ArchitectureCell{ID: dimensions.ID(), Dimensions: dimensions, Status: status, Required: status == CapabilityCompatible, Notes: strings.Join(notes, "; ")})
			return
		}
		service := matrixServices[index]
		for _, choice := range target.Services[service] {
			selected[service] = choice.ID
			checkedChoice := applyReleaseCompatibility(release, service, choice)
			expandServices(index+1, selected, append(choices, checkedChoice), computeMode, kubernetesMode)
		}
	}
	for _, computeMode := range computeModes {
		for _, kubernetesMode := range kubernetesModes {
			expandServices(0, make(map[string]string, len(matrixServices)), nil, computeMode, kubernetesMode)
		}
	}
	annotateArchitectureCells(cells, target, release, edition, preset)
	sort.Slice(cells, func(i, j int) bool { return cells[i].ID < cells[j].ID })
	return cells, nil
}

func annotateArchitectureCells(cells []ArchitectureCell, target TargetDescriptor, release, edition, preset string) {
	groups := make(map[string][]int, len(cells))
	for index := range cells {
		cell := &cells[index]
		cell.WarmBoundary = warmGroupKey(cell.Dimensions)
		cell.ExecutionFingerprint = architectureCellFingerprint(cell, target, release, edition, preset)
		groups[cell.WarmBoundary] = append(groups[cell.WarmBoundary], index)
	}
	for _, indexes := range groups {
		sort.Slice(indexes, func(i, j int) bool { return cells[indexes[i]].ID < cells[indexes[j]].ID })
		for position, index := range indexes {
			if position > 0 {
				cells[index].WarmFrom = cells[indexes[0]].ID
			}
		}
	}
}

func architectureCellFingerprint(cell *ArchitectureCell, target TargetDescriptor, release, edition, preset string) string {
	data, err := json.Marshal(struct {
		Schema        string     `json:"schema"`
		Catalog       string     `json:"catalog"`
		Target        string     `json:"target"`
		Provider      string     `json:"provider"`
		Runtime       string     `json:"runtime"`
		Release       string     `json:"release"`
		Edition       string     `json:"edition"`
		Preset        string     `json:"preset"`
		Dimensions    Dimensions `json:"dimensions"`
		Resilience    string     `json:"resilience"`
		Observability string     `json:"observability"`
		Edge          string     `json:"edge"`
		Artifact      string     `json:"artifact"`
		Migration     string     `json:"migration"`
		Ownership     string     `json:"ownership"`
	}{
		Schema: "magelift-certification-schema-v1", Catalog: CurrentCapabilityCatalog().Version,
		Target: string(target.ID), Provider: target.Provider, Runtime: target.Runtime,
		Release: release, Edition: edition, Preset: preset, Dimensions: cell.Dimensions,
		Resilience: "declared-policy-and-recovery-fixture", Observability: "native-plus-newrelic-otlp",
		Edge: "native-plus-fastly", Artifact: "immutable-digest", Migration: "explicit-owner", Ownership: "cell-scoped",
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func targetByID(id string) (TargetDescriptor, bool) {
	for _, target := range currentTargetMatrix {
		if target.ID == id {
			return target, true
		}
	}
	return TargetDescriptor{}, false
}

func targetStatus(target TargetDescriptor, choices []CapabilityOption) CapabilityStatus {
	status := CapabilityCompatible
	for _, choice := range choices {
		switch choice.Status {
		case CapabilityUnavailable:
			return CapabilityUnavailable
		case CapabilityUnsupported:
			status = CapabilityUnsupported
		case CapabilityExperimental:
			if status == CapabilityCompatible {
				status = CapabilityExperimental
			}
		}
	}
	if status == CapabilityCompatible && target.ClaimTier == "experimental" {
		return CapabilityExperimental
	}
	return status
}

// applyReleaseCompatibility folds the Adobe latest-patch catalog into a
// provider cell. Provider support and Adobe support are separate facts, but a
// cell cannot be a required architecture when Adobe explicitly rejects one of
// its service choices for that release.
func applyReleaseCompatibility(release, service string, choice CapabilityOption) CapabilityOption {
	component, catalogOption, ok := catalogChoice(service, choice.ID)
	if !ok {
		return choice
	}
	requirement, found := compatibilityRequirement(release, component, catalogOption)
	if !found {
		return choice
	}

	switch requirement.Status {
	case config.CompatibilityServiceUnsupported:
		choice.Status = mergeCapabilityStatus(choice.Status, CapabilityUnsupported)
	case config.CompatibilityServiceUnavailable:
		choice.Status = mergeCapabilityStatus(choice.Status, CapabilityUnavailable)
	default:
		return choice
	}

	note := fmt.Sprintf("Adobe %s marks %s/%s %s", release, component, catalogOption, requirement.Status)
	if requirement.Notes != "" {
		note += ": " + requirement.Notes
	}
	choice.Notes = joinNotes(choice.Notes, note)
	return choice
}

func catalogChoice(service, optionID string) (config.CompatibilityComponent, string, bool) {
	switch service {
	case "database":
		if strings.Contains(optionID, "mariadb") {
			return config.CompatibilityDatabase, "mariadb", true
		}
		if strings.Contains(optionID, "mysql") {
			return config.CompatibilityDatabase, "mysql", true
		}
	case "search":
		if optionID == "disabled" {
			return "", "", false
		}
		if strings.Contains(optionID, "elasticsearch") {
			return config.CompatibilitySearch, "elasticsearch", true
		}
		if strings.Contains(optionID, "opensearch") || optionID == "serverless" || optionID == "provisioned" {
			return config.CompatibilitySearch, "opensearch", true
		}
	case "queue":
		switch optionID {
		case "db", "database":
			return config.CompatibilityQueue, "database", true
		case "rabbitmq", "ecs-rabbitmq", "amazon-mq":
			return config.CompatibilityQueue, "rabbitmq", true
		case "artemis", "ecs-artemis":
			return config.CompatibilityQueue, "artemis", true
		}
	case "cache":
		if optionID == "redis" || strings.Contains(optionID, "redis") {
			return config.CompatibilityCache, "redis", true
		}
		if optionID == "valkey" || strings.Contains(optionID, "valkey") {
			return config.CompatibilityCache, "valkey", true
		}
	case "webCache":
		switch optionID {
		case "varnish":
			return config.CompatibilityWebCache, "varnish", true
		case "none":
			return config.CompatibilityWebCache, "none", true
		}
	}
	return "", "", false
}

func compatibilityRequirement(release string, component config.CompatibilityComponent, option string) (config.CompatibilityRequirement, bool) {
	for _, requirement := range config.RequirementsForRelease(release) {
		if requirement.Component == component && requirement.Option == option {
			return requirement, true
		}
	}
	return config.CompatibilityRequirement{}, false
}

func mergeCapabilityStatus(left, right CapabilityStatus) CapabilityStatus {
	if capabilityStatusRank(right) > capabilityStatusRank(left) {
		return right
	}
	return left
}

func capabilityStatusRank(status CapabilityStatus) int {
	switch status {
	case CapabilityUnavailable:
		return 4
	case CapabilityUnsupported:
		return 3
	case CapabilityExperimental:
		return 2
	case CapabilityCompatible:
		return 1
	default:
		return 0
	}
}

func joinNotes(existing, addition string) string {
	if existing == "" {
		return addition
	}
	if addition == "" {
		return existing
	}
	return existing + "; " + addition
}

func latestReleases() []string {
	wanted := map[string]bool{"2.4.6": true, "2.4.7": true, "2.4.8": true, "2.4.9": true}
	result := make([]string, 0, len(wanted))
	for _, line := range config.CurrentCompatibilityCatalog().Lines {
		if line.Available && wanted[line.Line] {
			result = append(result, line.RecommendedRelease)
		}
	}
	sort.Strings(result)
	return result
}

func option(id string, status CapabilityStatus, notes string) CapabilityOption {
	return CapabilityOption{ID: id, Status: status, Notes: notes}
}

func options(values ...CapabilityOption) []CapabilityOption {
	return values
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
