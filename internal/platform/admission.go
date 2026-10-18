package platform

import (
	"context"
	"errors"
	"fmt"
)

// PlanAdmission is the provider-neutral pre-mutation capability boundary.
// Providers may query live read-only capabilities and materialize opaque
// provider details, while the core owns ordering and identity validation.
type PlanAdmission interface {
	Admit(context.Context, PlannedStack) (PlannedStack, error)
}

// HasPlanAdmission is implemented by modules with a static, already-created
// admission adapter.
type HasPlanAdmission interface {
	PlanAdmission() PlanAdmission
}

// PlanAdmissionFactory constructs an admission adapter for one planned stack.
// Construction and admission must remain read-only; resource mutation starts
// only after AdmitPlan returns successfully.
type PlanAdmissionFactory interface {
	NewPlanAdmission(context.Context, PlannedStack) (PlanAdmission, error)
}

// ModulePlanAdmission returns a static admission adapter when the module
// exposes one. A nil result is an explicit declaration that no live admission
// is required by that module.
func ModulePlanAdmission(module StackModule) PlanAdmission {
	if module == nil {
		return nil
	}
	provider, ok := module.(HasPlanAdmission)
	if !ok {
		return nil
	}
	return provider.PlanAdmission()
}

// ModulePlanAdmissionFor resolves a planned admission adapter through the
// provider factory/static fallback used by the other lifecycle ports.
func ModulePlanAdmissionFor(ctx context.Context, module StackModule, planned PlannedStack) (PlanAdmission, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	if factory, ok := module.(PlanAdmissionFactory); ok {
		admission, err := factory.NewPlanAdmission(ctx, planned)
		if err != nil {
			return nil, fmt.Errorf("create plan admission for %q: %w", module.Descriptor().ID, err)
		}
		if admission != nil {
			return admission, nil
		}
	}
	return ModulePlanAdmission(module), nil
}

// AdmitPlan resolves and executes provider admission before any infrastructure
// backend or Pulumi program can be created.
func AdmitPlan(ctx context.Context, module StackModule, planned PlannedStack) (PlannedStack, error) {
	if err := validatePlannedModule(ctx, module, planned); err != nil {
		return nil, err
	}
	admission, err := ModulePlanAdmissionFor(ctx, module, planned)
	if err != nil {
		return nil, err
	}
	if admission == nil {
		return planned, nil
	}
	admitted, err := admission.Admit(ctx, planned)
	if err != nil {
		return nil, fmt.Errorf("admit plan for %q: %w", module.Descriptor().ID, err)
	}
	if err := validatePlannedModule(ctx, module, admitted); err != nil {
		return nil, fmt.Errorf("validate admitted plan for %q: %w", module.Descriptor().ID, err)
	}
	if err := validateAdmittedPlanIdentity(planned, admitted); err != nil {
		return nil, fmt.Errorf("validate admitted plan identity for %q: %w", module.Descriptor().ID, err)
	}
	return admitted, nil
}

func validateAdmittedPlanIdentity(before, after PlannedStack) error {
	if after == nil {
		return errors.New("provider returned a nil admitted plan")
	}
	checks := []struct {
		name string
		from string
		to   string
	}{
		{name: "stack name", from: before.StackName(), to: after.StackName()},
		{name: "provider", from: string(before.Provider()), to: string(after.Provider())},
		{name: "runtime", from: string(before.Runtime()), to: string(after.Runtime())},
		{name: "project", from: before.Project(), to: after.Project()},
		{name: "environment", from: before.Environment(), to: after.Environment()},
		{name: "region", from: before.Region(), to: after.Region()},
		{name: "environment class", from: before.EnvironmentClass(), to: after.EnvironmentClass()},
		{name: "image digest", from: before.ImageDigest(), to: after.ImageDigest()},
	}
	var problems []error
	for _, check := range checks {
		if check.from != check.to {
			problems = append(problems, fmt.Errorf("%s changed from %q to %q", check.name, check.from, check.to))
		}
	}
	if before.CertificationTier() != after.CertificationTier() {
		problems = append(problems, fmt.Errorf("certification tier changed from %q to %q", before.CertificationTier(), after.CertificationTier()))
	}
	if before.Protected() != after.Protected() {
		problems = append(problems, fmt.Errorf("protection changed from %t to %t", before.Protected(), after.Protected()))
	}
	return errors.Join(problems...)
}
