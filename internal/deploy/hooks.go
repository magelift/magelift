package deploy

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	sdk "github.com/acourtiol/magelift/sdk/v1"
)

const (
	HookTargetPreview    sdk.HookID = "deploy.preview"
	HookTargetCandidate  sdk.HookID = "deploy.candidate"
	HookTargetMigrations sdk.HookID = "deploy.migrations"
	HookTargetUpdate     sdk.HookID = "deploy.update"
	HookTargetStabilize  sdk.HookID = "deploy.stabilize"
	HookTargetHealth     sdk.HookID = "post-deploy.health"
	HookTargetRecord     sdk.HookID = "post-deploy.record"
)

type hookPlan struct {
	ordered []sdk.LifecycleHook
}

func newHookPlan(hooks []sdk.LifecycleHook) (*hookPlan, error) {
	if len(hooks) == 0 {
		return &hookPlan{}, nil
	}
	descriptors := make([]sdk.LifecycleHookDescriptor, 0, len(hooks))
	byID := make(map[sdk.HookID]sdk.LifecycleHook, len(hooks))
	for _, hook := range hooks {
		if hook == nil {
			return nil, errors.New("lifecycle hook cannot be nil")
		}
		descriptor := hook.Descriptor()
		if descriptor.Phase != sdk.PhaseDeploy && descriptor.Phase != sdk.PhasePostDeploy {
			return nil, fmt.Errorf("deployment hook %q must use deploy or post-deploy phase", descriptor.ID)
		}
		if !validHookTarget(descriptor.RelativeTo) {
			return nil, fmt.Errorf("deployment hook %q targets unknown operation %q", descriptor.ID, descriptor.RelativeTo)
		}
		if descriptor.Phase != phaseForTarget(descriptor.RelativeTo) {
			return nil, fmt.Errorf("deployment hook %q uses phase %q for %q; expected %q", descriptor.ID, descriptor.Phase, descriptor.RelativeTo, phaseForTarget(descriptor.RelativeTo))
		}
		descriptors = append(descriptors, descriptor)
		byID[descriptor.ID] = hook
	}
	if err := sdk.ValidateLifecycleHooks(descriptors); err != nil {
		return nil, err
	}
	for _, descriptor := range descriptors {
		for _, dependency := range descriptor.DependsOn {
			if _, exists := byID[dependency]; !exists {
				return nil, fmt.Errorf("deployment hook %q depends on unregistered hook %q", descriptor.ID, dependency)
			}
		}
	}
	ordered, err := topologicalHooks(descriptors, byID)
	if err != nil {
		return nil, err
	}
	return &hookPlan{ordered: ordered}, nil
}

func (p *hookPlan) run(ctx context.Context, request sdk.LifecycleHookRequest, target sdk.HookID, relationship sdk.HookRelationship) (bool, error) {
	if p == nil {
		return false, nil
	}
	skipped := false
	for _, hook := range p.ordered {
		descriptor := hook.Descriptor()
		if descriptor.RelativeTo != target {
			continue
		}
		if descriptor.Relationship == sdk.HookDisable {
			skipped = true
			continue
		}
		if descriptor.Relationship != relationship && !(relationship == sdk.HookBefore && descriptor.Relationship == sdk.HookReplace) {
			continue
		}
		attempts := descriptor.MaxAttempts
		if attempts < 1 {
			attempts = 1
		}
		var err error
		for attempt := 0; attempt < attempts; attempt++ {
			hookContext, cancel := context.WithTimeout(ctx, time.Duration(descriptor.Timeout)*time.Second)
			err = hook.Run(hookContext, request)
			cancel()
			if err == nil {
				break
			}
		}
		if err != nil {
			return false, fmt.Errorf("run lifecycle hook %q: %w", descriptor.ID, err)
		}
		if descriptor.Relationship == sdk.HookReplace {
			skipped = true
		}
	}
	return skipped, nil
}

func topologicalHooks(descriptors []sdk.LifecycleHookDescriptor, hooks map[sdk.HookID]sdk.LifecycleHook) ([]sdk.LifecycleHook, error) {
	byID := make(map[sdk.HookID]sdk.LifecycleHookDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		byID[descriptor.ID] = descriptor
	}
	state := make(map[sdk.HookID]uint8, len(descriptors))
	ordered := make([]sdk.LifecycleHook, 0, len(descriptors))
	var visit func(sdk.HookID) error
	visit = func(id sdk.HookID) error {
		switch state[id] {
		case 1:
			return fmt.Errorf("deployment hook dependency cycle at %q", id)
		case 2:
			return nil
		}
		state[id] = 1
		for _, dependency := range byID[id].DependsOn {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, hooks[id])
		return nil
	}
	ids := make([]sdk.HookID, 0, len(descriptors))
	for _, descriptor := range descriptors {
		ids = append(ids, descriptor.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		if err := visit(id); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func validHookTarget(target sdk.HookID) bool {
	switch target {
	case HookTargetPreview, HookTargetCandidate, HookTargetMigrations, HookTargetUpdate, HookTargetStabilize, HookTargetHealth, HookTargetRecord:
		return true
	default:
		return false
	}
}

func phaseForTarget(target sdk.HookID) sdk.LifecyclePhase {
	if target == HookTargetHealth || target == HookTargetRecord {
		return sdk.PhasePostDeploy
	}
	return sdk.PhaseDeploy
}
