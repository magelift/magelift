package certification

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	sdk "github.com/magelift/magelift/sdk/v1"
)

// CertificationPlan is the side-effect-free execution plan printed before a
// live certification run. It deliberately describes cost and credentials
// without resolving either one, so inspecting the matrix cannot mutate a
// provider account or expose a secret.
type CertificationPlan struct {
	Target                 TargetDescriptor               `json:"target" yaml:"target"`
	Release                string                         `json:"release" yaml:"release"`
	Edition                string                         `json:"edition" yaml:"edition"`
	Preset                 string                         `json:"preset" yaml:"preset"`
	CellCount              int                            `json:"cellCount" yaml:"cellCount"`
	StatusCounts           map[CapabilityStatus]int       `json:"statusCounts" yaml:"statusCounts"`
	RequiredCellIDs        []string                       `json:"requiredCellIds,omitempty" yaml:"requiredCellIds,omitempty"`
	DeferredCellIDs        []string                       `json:"deferredCellIds,omitempty" yaml:"deferredCellIds,omitempty"`
	WarmGroups             []WarmGroup                    `json:"warmGroups" yaml:"warmGroups"`
	Execution              Schedule                       `json:"execution" yaml:"execution"`
	CredentialRequirements []string                       `json:"credentialRequirements" yaml:"credentialRequirements"`
	TeardownScope          []string                       `json:"teardownScope" yaml:"teardownScope"`
	EstimatedCost          CostEstimate                   `json:"estimatedCost" yaml:"estimatedCost"`
	Artifact               *sdk.ImmutableArtifactContract `json:"artifact,omitempty" yaml:"artifact,omitempty"`
}

type WarmGroup struct {
	ID             string   `json:"id" yaml:"id"`
	BaselineCellID string   `json:"baselineCellId" yaml:"baselineCellId"`
	ReusedCellIDs  []string `json:"reusedCellIds,omitempty" yaml:"reusedCellIds,omitempty"`
	CellIDs        []string `json:"cellIds" yaml:"cellIds"`
	Boundary       string   `json:"boundary" yaml:"boundary"`
}

type CostEstimate struct {
	Status string `json:"status" yaml:"status"`
	Note   string `json:"note" yaml:"note"`
}

// ReuseBoundary is the complete, provider-neutral identity needed before a
// certification executor may reuse a provisioned stack. The artifact and all
// stateful/setup identities are explicit because a semantic matrix fingerprint
// alone cannot prove that two runs have compatible state.
type ReuseBoundary struct {
	CompatibilityFingerprint string
	Artifact                 sdk.ImmutableArtifactContract
	Fixture                  string
	BackupSet                string
	Observability            string
	Edge                     string
	SchemaFingerprint        string
	MigrationFingerprint     string
	StateBackend             string
}

// ValidateIdentity validates the reusable setup identities before an artifact
// has been built or looked up. CertificationRun uses this to keep artifact
// publication after read-only admission while still allowing callers to build
// the boundary from one immutable artifact record.
func (boundary ReuseBoundary) ValidateIdentity() error {
	for name, value := range map[string]string{
		"compatibility fingerprint": boundary.CompatibilityFingerprint,
		"fixture":                   boundary.Fixture,
		"backup set":                boundary.BackupSet,
		"observability":             boundary.Observability,
		"edge":                      boundary.Edge,
		"schema fingerprint":        boundary.SchemaFingerprint,
		"migration fingerprint":     boundary.MigrationFingerprint,
		"state backend":             boundary.StateBackend,
	} {
		if strings.TrimSpace(value) == "" || strings.ContainsAny(value, "\r\n\x00") {
			return fmt.Errorf("reuse boundary %s is required and must be single-line", name)
		}
	}
	if err := ValidateSecretSafeValue(boundary); err != nil {
		return fmt.Errorf("reuse boundary contains secret material: %w", err)
	}
	return nil
}

func (boundary ReuseBoundary) Validate() error {
	if err := boundary.ValidateIdentity(); err != nil {
		return err
	}
	if err := boundary.Artifact.Validate(); err != nil {
		return fmt.Errorf("reuse boundary artifact: %w", err)
	}
	if boundary.Artifact.InputFingerprint != boundary.CompatibilityFingerprint {
		return fmt.Errorf("reuse boundary artifact input fingerprint %q does not match %q", boundary.Artifact.InputFingerprint, boundary.CompatibilityFingerprint)
	}
	return nil
}

// PlanForTarget expands a target without contacting a provider. Cells marked
// unsupported or unavailable remain visible in DeferredCellIDs so an operator
// can see exactly why the live run will not attempt them. Because no artifact
// or setup identities are supplied, this inspection-only form deliberately
// does not authorize session reuse.
func PlanForTarget(targetID, release, edition, preset string) (CertificationPlan, error) {
	return planForTarget(targetID, release, edition, preset, nil, nil)
}

// PlanForTargetWithBoundary builds the executable form of a certification
// plan. It carries a complete reuse fingerprint into the scheduler so a
// session registry can reuse only an exactly compatible stack. The caller is
// expected to obtain the artifact with EnsurePipelineArtifact before calling
// this function; planning itself remains side-effect free.
func PlanForTargetWithBoundary(targetID, release, edition, preset string, boundary ReuseBoundary) (CertificationPlan, error) {
	if err := boundary.Validate(); err != nil {
		return CertificationPlan{}, err
	}
	return planForTarget(targetID, release, edition, preset, &boundary, nil)
}

// PlanForTargetWithBoundaryAndAdmission builds an executable plan using the
// already-admitted run snapshot. It is the run-scoped counterpart to
// PlanForTargetWithBoundary: every schedule build uses the same immutable
// provider facts and cannot silently perform a second provider probe.
func PlanForTargetWithBoundaryAndAdmission(admissionContext context.Context, targetID, release, edition, preset string, boundary ReuseBoundary, admissionInput AdmissionInput, admissionSnapshot *AdmissionSnapshot) (CertificationPlan, error) {
	return PlanForTargetWithBoundaryAndAdmissionOptions(admissionContext, targetID, release, edition, preset, boundary, admissionInput, admissionSnapshot, nil)
}

// PlanForTargetWithBoundaryAndAdmissionOptions is the configurable form of
// PlanForTargetWithBoundaryAndAdmission. The supplied scheduler options are
// copied into the plan build, while admission is always replaced with the
// already-admitted run snapshot so a caller cannot accidentally trigger a
// second provider probe or use a different admission input.
func PlanForTargetWithBoundaryAndAdmissionOptions(admissionContext context.Context, targetID, release, edition, preset string, boundary ReuseBoundary, admissionInput AdmissionInput, admissionSnapshot *AdmissionSnapshot, schedulerOptions *SchedulerOptions) (CertificationPlan, error) {
	if admissionSnapshot == nil {
		return CertificationPlan{}, errors.New("certification plan admission snapshot is required")
	}
	if admissionContext == nil {
		return CertificationPlan{}, errors.New("certification plan admission context is required")
	}
	if err := boundary.Validate(); err != nil {
		return CertificationPlan{}, err
	}
	if err := validateAdmissionInput(admissionInput); err != nil {
		return CertificationPlan{}, errors.New("certification plan admission input is invalid")
	}
	options := SchedulerOptions{
		Admission:         &admissionInput,
		AdmissionSnapshot: admissionSnapshot,
		AdmissionContext:  admissionContext,
	}
	if schedulerOptions != nil {
		options = *schedulerOptions
		options.Admission = &admissionInput
		options.AdmissionProbe = nil
		options.AdmissionSnapshot = admissionSnapshot
		options.AdmissionContext = admissionContext
	}
	return planForTarget(targetID, release, edition, preset, &boundary, &options)
}

func planForTarget(targetID, release, edition, preset string, boundary *ReuseBoundary, schedulerOptions *SchedulerOptions) (CertificationPlan, error) {
	values, err := CellsForTarget(targetID, release, edition, preset)
	if err != nil {
		return CertificationPlan{}, err
	}
	target, found := targetByID(targetID)
	if !found {
		return CertificationPlan{}, fmt.Errorf("unknown certification target %q", targetID)
	}

	plan := CertificationPlan{
		Target:                 target,
		Release:                release,
		Edition:                edition,
		Preset:                 preset,
		CellCount:              len(values),
		StatusCounts:           make(map[CapabilityStatus]int),
		CredentialRequirements: credentialRequirements(target.Provider, edition),
		TeardownScope:          teardownScope(target.Provider, target.Runtime),
		EstimatedCost: CostEstimate{
			Status: "not-calculated",
			Note:   "This read-only plan does not call provider pricing APIs; run cost --live with a resolved configuration before apply.",
		},
	}
	if boundary != nil {
		artifact := boundary.Artifact
		plan.Artifact = &artifact
	}
	scheduleCells := make([]ScheduleCell, 0, len(values))
	for _, cell := range values {
		ownershipMarker := "magelift/certification/" + target.ID + "/" + release + "/" + edition + "/" + preset + "/" + warmGroupKey(cell.Dimensions)
		scheduleCell := ScheduleCell{
			ID: cell.ID, Fingerprint: cell.ExecutionFingerprint, WarmBoundary: cell.WarmBoundary, WarmFrom: cell.WarmFrom,
			MutationKeys: []string{"provider/" + target.Provider, "warm/" + cell.WarmBoundary}, OwnershipMarker: ownershipMarker,
			Status: cell.Status, Required: cell.Required,
		}
		if boundary != nil {
			reuseFingerprint := ReuseFingerprint{
				Architecture:         cell.Dimensions.ID(),
				Fixture:              boundary.Fixture,
				BackupSet:            boundary.BackupSet,
				Observability:        boundary.Observability,
				Edge:                 boundary.Edge,
				ArtifactDigest:       boundary.Artifact.ImageDigest,
				SchemaFingerprint:    boundary.SchemaFingerprint,
				MigrationFingerprint: boundary.MigrationFingerprint,
				OwnershipMarker:      ownershipMarker,
				StateBackend:         boundary.StateBackend,
			}
			digest, digestErr := reuseFingerprint.Digest()
			if digestErr != nil {
				return CertificationPlan{}, fmt.Errorf("build reuse fingerprint for cell %q: %w", cell.ID, digestErr)
			}
			scheduleCell.Fingerprint = digest
			scheduleCell.ReuseFingerprint = &reuseFingerprint
		}
		scheduleCells = append(scheduleCells, scheduleCell)
	}
	options := SchedulerOptions{MaxParallel: 2, QuotaByKey: map[string]int{"provider/" + target.Provider: 1}}
	if schedulerOptions != nil {
		options = *schedulerOptions
		if options.MaxParallel == 0 {
			options.MaxParallel = 2
		}
		if options.QuotaByKey == nil {
			options.QuotaByKey = map[string]int{"provider/" + target.Provider: 1}
		}
	}
	schedule, err := BuildSchedule(scheduleCells, options)
	if err != nil {
		return CertificationPlan{}, fmt.Errorf("build certification execution schedule: %w", err)
	}
	plan.Execution = schedule

	groups := make(map[string]*WarmGroup)
	for _, cell := range values {
		plan.StatusCounts[cell.Status]++
		if cell.Status == CapabilityUnsupported || cell.Status == CapabilityUnavailable {
			plan.DeferredCellIDs = append(plan.DeferredCellIDs, cell.ID)
		} else if cell.Required {
			plan.RequiredCellIDs = append(plan.RequiredCellIDs, cell.ID)
		}

		key := warmGroupKey(cell.Dimensions)
		group := groups[key]
		if group == nil {
			group = &WarmGroup{
				ID:       key,
				Boundary: "same provider, runtime, release, edition, preset, database, cache, web-cache, and edge; search and queue transitions are candidates for reuse",
			}
			groups[key] = group
		}
		group.CellIDs = append(group.CellIDs, cell.ID)
	}
	for _, group := range groups {
		sort.Strings(group.CellIDs)
		if len(group.CellIDs) == 0 {
			continue
		}
		group.BaselineCellID = group.CellIDs[0]
		if len(group.CellIDs) > 1 {
			group.ReusedCellIDs = append([]string(nil), group.CellIDs[1:]...)
		}
		plan.WarmGroups = append(plan.WarmGroups, *group)
	}
	sort.Slice(plan.WarmGroups, func(i, j int) bool { return plan.WarmGroups[i].ID < plan.WarmGroups[j].ID })
	sort.Strings(plan.RequiredCellIDs)
	sort.Strings(plan.DeferredCellIDs)
	return plan, nil
}

func warmGroupKey(dimensions Dimensions) string {
	return strings.Join([]string{
		valueOrNone(dimensions.Provider), valueOrNone(dimensions.Runtime),
		valueOrNone(dimensions.ComputeMode), valueOrNone(dimensions.KubernetesMode),
		valueOrNone(dimensions.Release), valueOrNone(dimensions.Edition),
		valueOrNone(dimensions.Preset), valueOrNone(dimensions.Database),
		valueOrNone(dimensions.Cache), valueOrNone(dimensions.WebCache),
		valueOrNone(dimensions.Edge),
	}, "/")
}

func credentialRequirements(provider, edition string) []string {
	values := []string{provider + " provider credentials"}
	if edition == "commerce" {
		values = append(values, "Adobe Commerce Composer credentials through a secret reference")
	}
	sort.Strings(values)
	return values
}

func teardownScope(provider, runtime string) []string {
	return []string{
		provider + "/" + runtime + " stack resources identified by the run ownership marker",
		"provider-managed delayed deletions and protected metadata reported separately",
		"pre-existing external edge services, domains, zones, and credentials preserved",
	}
}
