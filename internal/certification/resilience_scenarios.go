package certification

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// ResilienceScenarioMode states who can safely execute a failure exercise.
// Unsupported is a successful classification of a provider gap, not a
// passing resilience result.
type ResilienceScenarioMode string

const (
	ScenarioAutomated        ResilienceScenarioMode = "automated"
	ScenarioOperatorAssisted ResilienceScenarioMode = "operator-assisted"
	ScenarioUnsupported      ResilienceScenarioMode = "unsupported"
)

type ResilienceScenarioKind string

const (
	ScenarioProcessLoss           ResilienceScenarioKind = "process-loss"
	ScenarioTaskLoss              ResilienceScenarioKind = "task-loss"
	ScenarioPodLoss               ResilienceScenarioKind = "pod-loss"
	ScenarioNodeLoss              ResilienceScenarioKind = "node-loss"
	ScenarioZoneLoss              ResilienceScenarioKind = "zone-loss"
	ScenarioRegionLoss            ResilienceScenarioKind = "region-loss"
	ScenarioCorruptedData         ResilienceScenarioKind = "corrupted-data"
	ScenarioFailedDeployment      ResilienceScenarioKind = "failed-deployment"
	ScenarioLostCredentials       ResilienceScenarioKind = "lost-credentials"
	ScenarioProviderOutage        ResilienceScenarioKind = "provider-outage"
	ScenarioPartialRestore        ResilienceScenarioKind = "partial-restore"
	ScenarioInterruptedTeardown   ResilienceScenarioKind = "interrupted-teardown"
	ScenarioAlternateProviderLoss ResilienceScenarioKind = "alternate-provider-loss"
)

// ResilienceScenario is the provider-neutral declaration of one controlled
// failure drill. The adapter supplies the injection and observation mechanics;
// the core keeps the required proof and ownership semantics identical.
type ResilienceScenario struct {
	ID                     string                 `json:"id" yaml:"id"`
	Kind                   ResilienceScenarioKind `json:"kind" yaml:"kind"`
	Scope                  string                 `json:"scope" yaml:"scope"`
	Mode                   ResilienceScenarioMode `json:"mode" yaml:"mode"`
	RequiredActions        []sdk.ResilienceAction `json:"requiredActions,omitempty" yaml:"requiredActions,omitempty"`
	FencingRequired        bool                   `json:"fencingRequired" yaml:"fencingRequired"`
	TrafficHealthRequired  bool                   `json:"trafficHealthRequired" yaml:"trafficHealthRequired"`
	QueueHealthRequired    bool                   `json:"queueHealthRequired" yaml:"queueHealthRequired"`
	DatabaseHealthRequired bool                   `json:"databaseHealthRequired" yaml:"databaseHealthRequired"`
	CacheLossRequired      bool                   `json:"cacheLossRequired" yaml:"cacheLossRequired"`
	IntegrityRequired      bool                   `json:"integrityRequired" yaml:"integrityRequired"`
	Owner                  string                 `json:"owner" yaml:"owner"`
	RunbookURL             string                 `json:"runbookUrl" yaml:"runbookUrl"`
	Reason                 string                 `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// FailureScenarioProof is safe evidence for one injected failure. It is
// intentionally separate from backup/restore proof so a successful backup
// cannot close an unexecuted HA or DR exercise.
type FailureScenarioProof struct {
	ScenarioID             string `json:"scenarioId" yaml:"scenarioId"`
	Status                 Status `json:"status" yaml:"status"`
	InjectionVerified      bool   `json:"injectionVerified" yaml:"injectionVerified"`
	TrafficHealthVerified  bool   `json:"trafficHealthVerified" yaml:"trafficHealthVerified"`
	QueueHealthVerified    bool   `json:"queueHealthVerified" yaml:"queueHealthVerified"`
	DatabaseHealthVerified bool   `json:"databaseHealthVerified" yaml:"databaseHealthVerified"`
	CacheLossClassified    bool   `json:"cacheLossClassified" yaml:"cacheLossClassified"`
	IntegrityVerified      bool   `json:"integrityVerified" yaml:"integrityVerified"`
	FencingVerified        bool   `json:"fencingVerified" yaml:"fencingVerified"`
	MeasuredRPOSeconds     int64  `json:"measuredRpoSeconds,omitempty" yaml:"measuredRpoSeconds,omitempty"`
	MeasuredRTOSeconds     int64  `json:"measuredRtoSeconds,omitempty" yaml:"measuredRtoSeconds,omitempty"`
	OperatorAction         string `json:"operatorAction,omitempty" yaml:"operatorAction,omitempty"`
	Reason                 string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

// BuildResilienceScenarioMatrix returns every failure boundary relevant to a
// selected architecture. It never claims that a scenario has been executed.
func BuildResilienceScenarioMatrix(intent sdk.ArchitectureIntent) ([]ResilienceScenario, error) {
	if err := intent.Validate(); err != nil {
		return nil, fmt.Errorf("validate resilience scenario architecture: %w", err)
	}
	owner := strings.TrimSpace(intent.Resilience.FailoverOwner)
	scenarios := []ResilienceScenario{
		{Kind: ScenarioCorruptedData, Scope: "durable-data-and-known-content-fixture", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/corrupted-data"},
		{Kind: ScenarioFailedDeployment, Scope: "immutable-artifact-and-pre-deploy-backup", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceBackup, sdk.ResilienceIntegrityCheck}, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/failed-deployment"},
		{Kind: ScenarioLostCredentials, Scope: "secret-reference-and-provider-identity", Mode: ScenarioOperatorAssisted, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceFence, sdk.ResilienceCleanup}, FencingRequired: true, Owner: owner, RunbookURL: "runbook://resilience/lost-credentials", Reason: "Credential revocation and replacement require an operator-approved identity change."},
		{Kind: ScenarioProviderOutage, Scope: "provider-control-plane-and-data-plane", Mode: ScenarioOperatorAssisted, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceFence}, FencingRequired: true, Owner: owner, RunbookURL: "runbook://resilience/provider-outage", Reason: "A provider outage cannot be injected or resolved by the core without provider-specific fault tooling."},
		{Kind: ScenarioPartialRestore, Scope: "database-media-configuration-search-cache", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck}, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/partial-restore"},
		{Kind: ScenarioInterruptedTeardown, Scope: "owned-resources-and-delayed-tombstones", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceCleanup}, Owner: owner, RunbookURL: "runbook://resilience/interrupted-teardown"},
	}

	switch string(intent.Runtime) {
	case "ecs-fargate":
		scenarios = append(scenarios, ResilienceScenario{Kind: ScenarioProcessLoss, Scope: "ECS task replacement", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceIntegrityCheck}, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/ecs-task"})
		scenarios = append(scenarios, ResilienceScenario{Kind: ScenarioTaskLoss, Scope: "ECS service task and queue consumer", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceIntegrityCheck}, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/ecs-task"})
	case "eks", "gke", "kapsule", "mks", "gke-autopilot", "gke-standard":
		scenarios = append(scenarios,
			ResilienceScenario{Kind: ScenarioPodLoss, Scope: "Kubernetes pod and workload rescheduling", Mode: ScenarioAutomated, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceIntegrityCheck}, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/kubernetes-pod"},
			ResilienceScenario{Kind: ScenarioNodeLoss, Scope: "Kubernetes node and persistent workload", Mode: ScenarioOperatorAssisted, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceIntegrityCheck}, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/kubernetes-node", Reason: "Node termination requires provider or test-cluster fault injection and explicit storage observation."},
		)
	default:
		scenarios = append(scenarios, ResilienceScenario{Kind: ScenarioProcessLoss, Scope: "runtime process", Mode: ScenarioOperatorAssisted, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceIntegrityCheck}, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/process", Reason: "The runtime family has no built-in process fault injector."})
	}

	zoneMode := ScenarioAutomated
	zoneReason := ""
	if len(intent.Zones) < 2 {
		zoneMode = ScenarioUnsupported
		zoneReason = "at least two declared zones are required for a zone-loss exercise"
	}
	scenarios = append(scenarios, ResilienceScenario{Kind: ScenarioZoneLoss, Scope: "declared availability-zone boundary", Mode: zoneMode, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceFailover, sdk.ResilienceIntegrityCheck}, FencingRequired: true, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/zone-loss", Reason: zoneReason})

	regionMode := ScenarioAutomated
	regionReason := ""
	if !containsRecoveryDestination(intent.Resilience.RecoveryDestinations, sdk.RecoveryAlternateRegion) {
		regionMode = ScenarioUnsupported
		regionReason = "alternate-region is not declared by the resilience profile"
	}
	scenarios = append(scenarios, ResilienceScenario{Kind: ScenarioRegionLoss, Scope: "alternate-region runtime and durable-data recovery", Mode: regionMode, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceFence, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck, sdk.ResilienceFailover}, FencingRequired: true, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/region-loss", Reason: regionReason})

	alternateProviderReason := "alternate-provider recovery requires an explicitly registered target pair plus compatible identity, durable-data, network, service, edge, and observability adapters"
	if !containsRecoveryDestination(intent.Resilience.RecoveryDestinations, sdk.RecoveryAlternateProvider) {
		alternateProviderReason = "alternate-provider is not declared by the resilience profile"
	}
	scenarios = append(scenarios, ResilienceScenario{Kind: ScenarioAlternateProviderLoss, Scope: "alternate-provider identity, durable-data, network, service, edge, and observability recovery", Mode: ScenarioUnsupported, RequiredActions: []sdk.ResilienceAction{sdk.ResilienceFence, sdk.ResilienceRestore, sdk.ResilienceIntegrityCheck, sdk.ResilienceFailover}, FencingRequired: true, TrafficHealthRequired: true, IntegrityRequired: true, Owner: owner, RunbookURL: "runbook://resilience/alternate-provider", Reason: alternateProviderReason})

	for index := range scenarios {
		applyRuntimeFailureGates(&scenarios[index], intent)
		applyGCPFailureScenarioHonesty(&scenarios[index], intent)
		scenarios[index].ID = "scenario-" + strings.ReplaceAll(string(scenarios[index].Kind), "-", "_")
	}
	sort.Slice(scenarios, func(i, j int) bool { return scenarios[i].ID < scenarios[j].ID })
	if err := ValidateResilienceScenarioMatrix(scenarios); err != nil {
		return nil, err
	}
	return scenarios, nil
}

func ValidateResilienceScenarioMatrix(scenarios []ResilienceScenario) error {
	if len(scenarios) == 0 {
		return errors.New("resilience scenario matrix must not be empty")
	}
	seen := make(map[string]struct{}, len(scenarios))
	for _, scenario := range scenarios {
		if strings.TrimSpace(scenario.ID) == "" || scenario.Kind == "" || strings.TrimSpace(scenario.Scope) == "" || strings.TrimSpace(scenario.Owner) == "" || strings.TrimSpace(scenario.RunbookURL) == "" {
			return fmt.Errorf("resilience scenario %q is incomplete", scenario.ID)
		}
		if _, exists := seen[scenario.ID]; exists {
			return fmt.Errorf("duplicate resilience scenario %q", scenario.ID)
		}
		seen[scenario.ID] = struct{}{}
		switch scenario.Mode {
		case ScenarioAutomated, ScenarioOperatorAssisted:
			if len(scenario.RequiredActions) == 0 {
				return fmt.Errorf("resilience scenario %q requires actions", scenario.ID)
			}
		case ScenarioUnsupported:
			if strings.TrimSpace(scenario.Reason) == "" {
				return fmt.Errorf("unsupported resilience scenario %q requires a reason", scenario.ID)
			}
		default:
			return fmt.Errorf("resilience scenario %q has invalid mode %q", scenario.ID, scenario.Mode)
		}
	}
	return nil
}

// ValidateFailureScenarioProof checks that a failure drill proves the gates
// declared by its scenario. It is intentionally independent from provider
// fault-injection APIs so every adapter reports the same release semantics.
func ValidateFailureScenarioProof(scenario ResilienceScenario, proof FailureScenarioProof) error {
	if scenario.ID == "" || proof.ScenarioID != scenario.ID {
		return fmt.Errorf("failure proof scenario %q does not match %q", proof.ScenarioID, scenario.ID)
	}
	if proof.MeasuredRPOSeconds < 0 || proof.MeasuredRTOSeconds < 0 {
		return fmt.Errorf("failure proof %q contains a negative recovery measurement", scenario.ID)
	}
	switch scenario.Mode {
	case ScenarioUnsupported:
		if proof.Status != StatusSkip && proof.Status != StatusBlocked && proof.Status != StatusNotRun {
			return fmt.Errorf("unsupported failure scenario %q must be skipped, blocked, or not run", scenario.ID)
		}
		if strings.TrimSpace(proof.Reason) == "" {
			return fmt.Errorf("unsupported failure scenario %q requires a reason", scenario.ID)
		}
		return nil
	case ScenarioAutomated, ScenarioOperatorAssisted:
	default:
		return fmt.Errorf("failure scenario %q has invalid mode %q", scenario.ID, scenario.Mode)
	}
	if !validEvidenceStatus(proof.Status) {
		return fmt.Errorf("failure scenario %q has invalid proof status %q", scenario.ID, proof.Status)
	}
	if (proof.Status == StatusPass || proof.Status == StatusFail) && !proof.InjectionVerified {
		return fmt.Errorf("failure scenario %q has an executable result without injection verification", scenario.ID)
	}
	if proof.Status == StatusPass {
		if !proof.InjectionVerified {
			return fmt.Errorf("failure scenario %q passed without injection verification", scenario.ID)
		}
		if scenario.TrafficHealthRequired && !proof.TrafficHealthVerified {
			return fmt.Errorf("failure scenario %q passed without traffic-health verification", scenario.ID)
		}
		if scenario.QueueHealthRequired && !proof.QueueHealthVerified {
			return fmt.Errorf("failure scenario %q passed without queue-health verification", scenario.ID)
		}
		if scenario.DatabaseHealthRequired && !proof.DatabaseHealthVerified {
			return fmt.Errorf("failure scenario %q passed without database-health verification", scenario.ID)
		}
		if scenario.CacheLossRequired && !proof.CacheLossClassified {
			return fmt.Errorf("failure scenario %q passed without cache-loss classification", scenario.ID)
		}
		if scenario.IntegrityRequired && !proof.IntegrityVerified {
			return fmt.Errorf("failure scenario %q passed without integrity verification", scenario.ID)
		}
		if scenario.FencingRequired && !proof.FencingVerified {
			return fmt.Errorf("failure scenario %q passed without fencing verification", scenario.ID)
		}
		if scenario.Mode == ScenarioOperatorAssisted && strings.TrimSpace(proof.OperatorAction) == "" {
			return fmt.Errorf("operator-assisted failure scenario %q passed without an operator action", scenario.ID)
		}
	}
	if (proof.Status == StatusFail || proof.Status == StatusBlocked || proof.Status == StatusSkip) && strings.TrimSpace(proof.Reason) == "" {
		return fmt.Errorf("failure scenario %q requires a reason for status %q", scenario.ID, proof.Status)
	}
	return nil
}

func applyRuntimeFailureGates(scenario *ResilienceScenario, intent sdk.ArchitectureIntent) {
	if scenario == nil {
		return
	}
	switch scenario.Kind {
	case ScenarioProcessLoss, ScenarioTaskLoss, ScenarioPodLoss, ScenarioNodeLoss, ScenarioZoneLoss, ScenarioRegionLoss, ScenarioPartialRestore:
	default:
		return
	}
	for _, dataClass := range intent.Resilience.DataClasses {
		switch dataClass.Name {
		case "queue":
			scenario.QueueHealthRequired = true
		case "database":
			scenario.DatabaseHealthRequired = true
		case "cache":
			scenario.CacheLossRequired = true
		}
	}
}

func applyGCPFailureScenarioHonesty(scenario *ResilienceScenario, intent sdk.ArchitectureIntent) {
	if scenario == nil || intent.Provider != "gcp" {
		return
	}
	switch scenario.Kind {
	case ScenarioFailedDeployment:
		scenario.Mode = ScenarioOperatorAssisted
		scenario.Reason = "no failed-deployment injector; pre-deploy backup exists but the drill requires an operator-approved bad artifact"
		return
	case ScenarioPartialRestore:
		scenario.Mode = ScenarioOperatorAssisted
		scenario.Reason = "no partial-restore injector; single-class restore cells exist but the drill requires restoring a subset of database, media, configuration, search, and cache together"
		return
	}
	if !gcpKubernetesRuntime(intent.Runtime) {
		return
	}
	const integrityCeiling = "Magento known-content application-integrity is not proven (setup:db:status only)"
	switch scenario.Kind {
	case ScenarioZoneLoss:
		if scenario.Mode == ScenarioUnsupported {
			return
		}
		scenario.Reason = "physical zone outage is unsupported; the evidenced ceiling is backing-VM zone-loss simulation without a native fence or failover translator; " + integrityCeiling
	case ScenarioPodLoss:
		scenario.Reason = "evidenced ceiling is kube reschedule plus setup:db:status; Magento known-content application-integrity is not proven"
	case ScenarioNodeLoss:
		if scenario.Reason != "" {
			scenario.Reason = strings.TrimSuffix(scenario.Reason, ".") + "; " + integrityCeiling
		} else {
			scenario.Reason = integrityCeiling
		}
	case ScenarioLostCredentials, ScenarioProviderOutage:
		scenario.Mode = ScenarioUnsupported
		scenario.Reason = "writer fencing is typed unsupported on GCP; this scenario fail-closed until a native fence translator opts in"
	}
}

func gcpKubernetesRuntime(runtime sdk.RuntimeID) bool {
	switch string(runtime) {
	case "gke", "gke-autopilot", "gke-standard":
		return true
	default:
		return false
	}
}

func containsRecoveryDestination(destinations []sdk.RecoveryDestination, wanted sdk.RecoveryDestination) bool {
	for _, destination := range destinations {
		if destination == wanted {
			return true
		}
	}
	return false
}
