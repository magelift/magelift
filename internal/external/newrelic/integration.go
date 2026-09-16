package newrelic

import (
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/sdk"
)

// Workload identifies the instrumentation boundary. ECS, Kubernetes, and a
// generic workload intentionally remain separate because their collectors,
// permissions, and ownership evidence differ.
type Workload string

const (
	WorkloadECS        Workload = "ecs"
	WorkloadKubernetes Workload = "kubernetes"
	WorkloadGeneric    Workload = "generic"
)

// IntegrationMode selects a documented New Relic collector deployment or the
// portable OTLP path. Native here describes the collector integration
// boundary; it does not mean that New Relic resources become part of the
// MageLift core schema.
type IntegrationMode string

const (
	IntegrationNative IntegrationMode = "native"
	IntegrationOTLP   IntegrationMode = "otlp"
)

type IntegrationStatus string

const (
	IntegrationConfigured  IntegrationStatus = "configured"
	IntegrationUnsupported IntegrationStatus = "unsupported"
	IntegrationUnavailable IntegrationStatus = "unavailable"
)

// IntegrationRequest is provider-owned input for selecting a documented New
// Relic path. CredentialRef and NativeReference are opaque references; no
// credential value or New Relic object is represented here.
type IntegrationRequest struct {
	TargetProvider  string
	TargetRuntime   string
	Workload        Workload
	Mode            IntegrationMode
	CredentialRef   string
	Endpoint        string
	NativeReference string
	OwnershipMarker string
}

// IntegrationPlan is safe to pass to a core lifecycle runner. Unsupported
// combinations are represented as data so certification can record them
// without treating a neighboring architecture as a PASS.
type IntegrationPlan struct {
	Provider              string
	TargetProvider        string
	TargetRuntime         string
	Workload              Workload
	Mode                  IntegrationMode
	Status                IntegrationStatus
	Reason                string
	CredentialRef         string
	Endpoint              string
	NativeReference       string
	OwnershipMarker       string
	CollectorDistribution string
}

// PlanIntegration selects the source-documented New Relic path without
// creating resources. The actual collector/OTLP lifecycle is still executed
// by an injected adapter and must produce independent delivery and cleanup
// proof.
func PlanIntegration(request IntegrationRequest) (IntegrationPlan, error) {
	if err := validateNewRelicTarget(request.TargetProvider, request.TargetRuntime); err != nil {
		return IntegrationPlan{}, err
	}
	if err := validateWorkload(request.Workload); err != nil {
		return IntegrationPlan{}, err
	}
	if request.Mode != IntegrationNative && request.Mode != IntegrationOTLP {
		return IntegrationPlan{}, fmt.Errorf("unsupported New Relic integration mode %q", request.Mode)
	}
	if err := sdk.ValidateCredentialReference(request.CredentialRef); err != nil {
		return IntegrationPlan{}, fmt.Errorf("New Relic credential reference: %w", err)
	}
	if err := validateOwnershipMarker(request.OwnershipMarker); err != nil {
		return IntegrationPlan{}, err
	}
	if request.NativeReference != "" && strings.ContainsAny(request.NativeReference, "\r\n\x00") {
		return IntegrationPlan{}, errors.New("New Relic native reference must be single-line and NUL-free")
	}
	plan := IntegrationPlan{
		Provider:        "newrelic",
		TargetProvider:  request.TargetProvider,
		TargetRuntime:   request.TargetRuntime,
		Workload:        request.Workload,
		Mode:            request.Mode,
		Status:          IntegrationConfigured,
		CredentialRef:   request.CredentialRef,
		NativeReference: request.NativeReference,
		OwnershipMarker: request.OwnershipMarker,
	}
	if strings.TrimSpace(request.Endpoint) != "" {
		endpoint, err := normalizeEndpoint(request.Endpoint)
		if err != nil {
			return IntegrationPlan{}, err
		}
		plan.Endpoint = endpoint
	}
	if request.Mode == IntegrationOTLP {
		if plan.Endpoint == "" {
			return IntegrationPlan{}, errors.New("New Relic OTLP integration endpoint is required")
		}
		plan.CollectorDistribution = "opentelemetry"
		return plan, nil
	}

	switch request.Workload {
	case WorkloadECS:
		if request.TargetProvider != "aws" || !strings.HasPrefix(request.TargetRuntime, "ecs") {
			return unsupportedPlan(plan, "New Relic ECS collector integration requires an AWS ECS target")
		}
		if strings.TrimSpace(request.NativeReference) == "" {
			return unavailablePlan(plan, "New Relic ECS collector integration requires an opaque ECS service or task-definition reference")
		}
		plan.CollectorDistribution = "opentelemetry-collector-contrib"
	case WorkloadKubernetes:
		if !isNRDOTKubernetesTarget(request.TargetProvider, request.TargetRuntime) {
			return unsupportedPlan(plan, "New Relic NRDOT Kubernetes integration requires a supported MageLift Kubernetes target; use OTLP or a separately verified custom collector for this target")
		}
		if strings.TrimSpace(request.NativeReference) == "" {
			return unavailablePlan(plan, "New Relic Kubernetes integration requires an opaque cluster reference")
		}
		plan.CollectorDistribution = NRDOTKubernetesDistribution
	case WorkloadGeneric:
		return unsupportedPlan(plan, "New Relic has no generic native infrastructure integration; use OTLP")
	}
	if plan.Endpoint == "" {
		return unavailablePlan(plan, "New Relic collector integration endpoint is required for the selected data region")
	}
	return plan, nil
}

func unavailablePlan(plan IntegrationPlan, reason string) (IntegrationPlan, error) {
	plan.Status = IntegrationUnavailable
	plan.Reason = reason
	return plan, nil
}

func unsupportedPlan(plan IntegrationPlan, reason string) (IntegrationPlan, error) {
	plan.Status = IntegrationUnsupported
	plan.Reason = reason
	return plan, nil
}

func validateWorkload(workload Workload) error {
	switch workload {
	case WorkloadECS, WorkloadKubernetes, WorkloadGeneric:
		return nil
	default:
		return fmt.Errorf("unsupported New Relic workload %q", workload)
	}
}

// isNRDOTKubernetesTarget identifies first-party Kubernetes runtimes for
// which the provider-neutral Helm lifecycle can install the standard NRDOT
// chart. New Relic documents the chart as a Kubernetes installation and lists
// managed-cloud examples such as EKS and GKE; Kapsule and MKS use the same
// Kubernetes/Helm boundary, but their compatibility remains a separate live
// evidence gate rather than an implicit certification claim.
func isNRDOTKubernetesTarget(provider, runtime string) bool {
	switch provider {
	case "aws":
		return runtime == "eks" || strings.HasPrefix(runtime, "eks-")
	case "gcp":
		return runtime == "gke" || strings.HasPrefix(runtime, "gke-")
	case "scaleway":
		return runtime == "kapsule" || strings.HasPrefix(runtime, "kapsule-")
	case "ovh":
		return runtime == "mks" || strings.HasPrefix(runtime, "mks-")
	default:
		return false
	}
}

func validateNewRelicTarget(provider, runtime string) error {
	if err := sdk.ValidateTargetDescriptor(sdk.TargetDescriptor{
		ID:       "newrelic.integration",
		Provider: sdk.ProviderID(provider),
		Runtime:  sdk.RuntimeID(runtime),
	}); err != nil {
		return fmt.Errorf("New Relic target identity: %w", err)
	}
	return nil
}
