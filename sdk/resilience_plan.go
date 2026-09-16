package sdk

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var resilienceStageSuffixPattern = regexp.MustCompile(`[^a-z0-9]+`)

// CompileResiliencePlan is the provider-neutral recovery graph compiler. It
// owns ordering, idempotency, approval boundaries, and required proof names;
// an adapter supplies only the capabilities and destination identities it can
// actually execute.
func CompileResiliencePlan(descriptor ResilienceAdapterDescriptor, request ResiliencePlanRequest) (ResiliencePlan, error) {
	if err := ValidateResilienceAdapterDescriptor(descriptor); err != nil {
		return ResiliencePlan{}, fmt.Errorf("validate resilience adapter descriptor: %w", err)
	}
	if err := ValidateResiliencePlanRequest(request); err != nil {
		return ResiliencePlan{}, fmt.Errorf("validate resilience plan request: %w", err)
	}
	if descriptor.Provider != request.Architecture.Provider {
		return ResiliencePlan{}, fmt.Errorf("resilience adapter provider %q does not match architecture provider %q", descriptor.Provider, request.Architecture.Provider)
	}

	dataClasses := append([]DataClassIntent(nil), request.Architecture.Resilience.DataClasses...)
	sort.Slice(dataClasses, func(i, j int) bool { return dataClasses[i].Name < dataClasses[j].Name })
	capabilities := make(map[string]ResilienceDataClassCapability, len(descriptor.DataClasses))
	for _, capability := range descriptor.DataClasses {
		capabilities[capability.Name] = capability
	}
	for _, dataClass := range dataClasses {
		capability, ok := capabilities[dataClass.Name]
		if !ok {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceRestore, dataClass.Name, "data class is not declared by the provider adapter")
		}
		if capability.Status == ResilienceCapabilityUnavailable || capability.Status == ResilienceCapabilityUnsupported || capability.Status == ResilienceCapabilityBlocked {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceRestore, dataClass.Name, capability.Reason)
		}
	}

	destination, err := commonRecoveryDestination(request.Architecture.Resilience.RecoveryDestinations, dataClasses, capabilities)
	if err != nil {
		return ResiliencePlan{}, fmt.Errorf("select resilience recovery destination: %w", err)
	}
	allClasses := make([]string, 0, len(dataClasses))
	backupStages := make([]string, 0, len(dataClasses))
	finalStages := make([]string, 0, len(dataClasses))
	plan := ResiliencePlan{
		AdapterID:       descriptor.ID,
		FixtureID:       request.FixtureID,
		OwnershipMarker: request.OwnershipMarker,
	}
	for _, dataClass := range dataClasses {
		capability := capabilities[dataClass.Name]
		suffix := resilienceStageSuffix(dataClass.Name)
		if suffix == "" {
			return ResiliencePlan{}, fmt.Errorf("resilience data class %q cannot produce a stable stage ID", dataClass.Name)
		}
		allClasses = append(allClasses, dataClass.Name)

		if !resilienceRebuildOnly(dataClass.BackupMethod) {
			if !resilienceHasAction(descriptor, capability, ResilienceBackup) {
				return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceBackup, dataClass.Name, "selected data class requires a backup operation")
			}
			backupID := "backup-" + suffix
			plan.Stages = append(plan.Stages, ResilienceStage{
				ID: backupID, Action: ResilienceBackup, DataClasses: []string{dataClass.Name},
				Destination: destination, Idempotent: true,
			})
			backupStages = append(backupStages, backupID)
			plan.RequiredProofs = append(plan.RequiredProofs, "backup:"+dataClass.Name)
		}
	}

	needsFence := resilienceRequiresFence(destination)
	fenceID := ""
	if needsFence {
		if !resilienceHasGlobalAction(descriptor, ResilienceFence) {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceFence, "runtime", "selected recovery destination requires fencing")
		}
		fenceID = "fence-runtime"
		plan.Stages = append(plan.Stages, ResilienceStage{
			ID: fenceID, Action: ResilienceFence, DataClasses: append([]string(nil), allClasses...),
			Destination: destination, DependsOn: append([]string(nil), backupStages...), Idempotent: true, RequiresApproval: true,
			RequiresSingleWriter: true, RequiresSplitBrainCheck: true,
		})
		plan.RequiredProofs = append(plan.RequiredProofs, "fencing", "single-writer", "split-brain")
	}

	for _, dataClass := range dataClasses {
		capability := capabilities[dataClass.Name]
		suffix := resilienceStageSuffix(dataClass.Name)
		dependencies := make([]string, 0, 2)
		if !resilienceRebuildOnly(dataClass.BackupMethod) {
			dependencies = append(dependencies, "backup-"+suffix)
		}
		if fenceID != "" {
			dependencies = append(dependencies, fenceID)
		}
		if !resilienceHasAction(descriptor, capability, ResilienceRestore) {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceRestore, dataClass.Name, "selected data class has no restore or reconstruction operation")
		}
		restoreID := "restore-" + suffix
		plan.Stages = append(plan.Stages, ResilienceStage{
			ID: restoreID, Action: ResilienceRestore, DataClasses: []string{dataClass.Name},
			Destination: destination, DependsOn: dependencies, Idempotent: true,
			RequiresApproval: destination == RecoverySameRegion || dataClass.Name == "queue",
		})
		plan.RequiredProofs = append(plan.RequiredProofs, "restore:"+dataClass.Name)

		if !resilienceHasAction(descriptor, capability, ResilienceIntegrityCheck) {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceIntegrityCheck, dataClass.Name, "selected data class has no integrity verification operation")
		}
		integrityID := "integrity-" + suffix
		plan.Stages = append(plan.Stages, ResilienceStage{
			ID: integrityID, Action: ResilienceIntegrityCheck, DataClasses: []string{dataClass.Name},
			Destination: destination, DependsOn: []string{restoreID}, Idempotent: true,
		})
		plan.RequiredProofs = append(plan.RequiredProofs, "integrity:"+dataClass.Name)
		finalStages = append(finalStages, integrityID)
	}

	if resilienceRequiresFailover(destination) {
		if !resilienceHasGlobalAction(descriptor, ResilienceFailover) {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceFailover, "runtime", "selected recovery destination requires traffic failover")
		}
		failoverID := "failover-runtime"
		plan.Stages = append(plan.Stages, ResilienceStage{
			ID: failoverID, Action: ResilienceFailover, DataClasses: append([]string(nil), allClasses...),
			Destination: destination, DependsOn: append([]string(nil), finalStages...), Idempotent: true, RequiresApproval: true,
			RequiresSingleWriter: true, RequiresSplitBrainCheck: true, RequiresStaleOriginCheck: true,
		})
		plan.RequiredProofs = append(plan.RequiredProofs, "failover", "single-writer", "split-brain", "stale-origin")
		finalStages = []string{failoverID}
	}
	if _, requested := request.ApprovalReferences["failback-runtime"]; requested {
		if !resilienceRequiresFailover(destination) {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceFailback, "runtime", "failback requires an alternate-region or alternate-provider recovery destination")
		}
		if !resilienceHasGlobalAction(descriptor, ResilienceFailback) {
			return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceFailback, "runtime", "provider adapter has not opted into controlled failback")
		}
		if strings.TrimSpace(request.ReconciliationReferences["failback-runtime"]) == "" {
			return ResiliencePlan{}, fmt.Errorf("resilience failback requires a reconciliation reference")
		}
		failbackID := "failback-runtime"
		plan.Stages = append(plan.Stages, ResilienceStage{
			ID: failbackID, Action: ResilienceFailback, DataClasses: append([]string(nil), allClasses...),
			Destination: destination, DependsOn: append([]string(nil), finalStages...), Idempotent: true,
			RequiresApproval: true, RequiresSingleWriter: true, RequiresSplitBrainCheck: true,
			RequiresStaleOriginCheck: true, RequiresReconciliation: true,
		})
		plan.RequiredProofs = append(plan.RequiredProofs, "failback", "reconciliation", "single-writer", "split-brain", "stale-origin")
		finalStages = []string{failbackID}
	}
	if !resilienceHasGlobalAction(descriptor, ResilienceCleanup) {
		return ResiliencePlan{}, resilienceCapabilityUnavailable(descriptor.ID, ResilienceCleanup, "runtime", "recovery cleanup is not implemented by the provider adapter")
	}
	plan.Stages = append(plan.Stages, ResilienceStage{
		ID: "cleanup-runtime", Action: ResilienceCleanup, DataClasses: append([]string(nil), allClasses...),
		Destination: destination, DependsOn: append([]string(nil), finalStages...), Idempotent: true,
	})
	plan.RequiredProofs = append(plan.RequiredProofs, "cleanup")
	if err := bindResilienceAdmissionReferences(&plan, request); err != nil {
		return ResiliencePlan{}, err
	}
	plan.RequiredProofs = uniqueResilienceStrings(plan.RequiredProofs)
	if err := ValidateResiliencePlan(plan, request.Architecture.Resilience); err != nil {
		return ResiliencePlan{}, fmt.Errorf("validate compiled resilience plan: %w", err)
	}
	return plan, nil
}

// bindResilienceAdmissionReferences carries only the opaque identities that
// were supplied at plan time onto their exact stages. Ignoring an unknown map
// key would make a caller believe an approval was bound when it was not.
func bindResilienceAdmissionReferences(plan *ResiliencePlan, request ResiliencePlanRequest) error {
	for _, stageID := range resilienceReferenceKeys(request.ApprovalReferences) {
		stageIndex := resilienceStageIndex(*plan, stageID)
		if stageIndex < 0 {
			return fmt.Errorf("resilience approval reference targets unknown stage %q", stageID)
		}
		plan.Stages[stageIndex].ApprovalReference = request.ApprovalReferences[stageID]
	}
	for _, stageID := range resilienceReferenceKeys(request.ReconciliationReferences) {
		stageIndex := resilienceStageIndex(*plan, stageID)
		if stageIndex < 0 {
			return fmt.Errorf("resilience reconciliation reference targets unknown stage %q", stageID)
		}
		plan.Stages[stageIndex].ReconciliationReference = request.ReconciliationReferences[stageID]
	}
	return nil
}

func resilienceReferenceKeys(references map[string]string) []string {
	keys := make([]string, 0, len(references))
	for key := range references {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func resilienceStageIndex(plan ResiliencePlan, stageID string) int {
	for index, stage := range plan.Stages {
		if stage.ID == stageID {
			return index
		}
	}
	return -1
}

func commonRecoveryDestination(requested []RecoveryDestination, dataClasses []DataClassIntent, capabilities map[string]ResilienceDataClassCapability) (RecoveryDestination, error) {
	requestedSet := make(map[RecoveryDestination]struct{}, len(requested))
	for _, destination := range requested {
		requestedSet[destination] = struct{}{}
	}
	for _, destination := range []RecoveryDestination{RecoveryAlternateProvider, RecoveryAlternateRegion, RecoverySameRegionIsolated, RecoverySameRegion} {
		if _, requested := requestedSet[destination]; !requested {
			continue
		}
		common := true
		for _, dataClass := range dataClasses {
			if !containsRecoveryDestination(capabilities[dataClass.Name].Destinations, destination) {
				common = false
				break
			}
		}
		if common {
			return destination, nil
		}
	}
	return "", resilienceCapabilityUnavailable("core", ResilienceRestore, "runtime", "no requested recovery destination is supported by every selected data class")
}

func containsRecoveryDestination(destinations []RecoveryDestination, candidate RecoveryDestination) bool {
	for _, destination := range destinations {
		if destination == candidate {
			return true
		}
	}
	return false
}

func resilienceHasAction(descriptor ResilienceAdapterDescriptor, capability ResilienceDataClassCapability, action ResilienceAction) bool {
	if !resilienceHasGlobalAction(descriptor, action) {
		return false
	}
	for _, candidate := range capability.Actions {
		if candidate == action {
			return true
		}
	}
	return false
}

func resilienceHasGlobalAction(descriptor ResilienceAdapterDescriptor, action ResilienceAction) bool {
	for _, candidate := range descriptor.Capabilities {
		if candidate == action {
			return true
		}
	}
	return false
}

func resilienceRebuildOnly(method string) bool {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "none", "rebuild", "recreate":
		return true
	default:
		return false
	}
}

func resilienceRequiresFence(destination RecoveryDestination) bool {
	return destination == RecoverySameRegion || resilienceRequiresFailover(destination)
}

func resilienceRequiresFailover(destination RecoveryDestination) bool {
	return destination == RecoveryAlternateRegion || destination == RecoveryAlternateProvider
}

func resilienceStageSuffix(name string) string {
	suffix := resilienceStageSuffixPattern.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-")
	return strings.Trim(suffix, "-")
}

func uniqueResilienceStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, found := seen[value]; found {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func resilienceCapabilityUnavailable(adapterID string, operation ResilienceAction, dataClass, reason string) error {
	if strings.TrimSpace(reason) == "" {
		reason = "provider adapter does not expose this operation"
	}
	return ResilienceCapabilityError{AdapterID: adapterID, Operation: operation, DataClass: dataClass, Status: ResilienceCapabilityUnavailable, Reason: reason}
}
