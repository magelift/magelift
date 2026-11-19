package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"sync"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

// The host sends fresh stack outputs at each phase. Pulumi mutations go through
// the lifecycle RPC so the plugin cannot apply the stack twice on a greenfield deploy.
type phaseOutputs struct{ values map[string]any }

func (b *phaseOutputs) Outputs(context.Context) (map[string]any, error) { return b.values, nil }
func (*phaseOutputs) Preview(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return nil, errors.New("AWS deployment phases cannot preview infrastructure")
}
func (*phaseOutputs) Update(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return nil, errors.New("create AWS infrastructure before registering the deployment candidate")
}
func (*phaseOutputs) Destroy(context.Context, automation.Request, io.Writer) (map[string]int, error) {
	return nil, errors.New("AWS deployment phases cannot destroy infrastructure")
}

type deploymentCandidate struct {
	mu         sync.Mutex
	plan       [32]byte
	steps      deploy.Steps
	outputs    *phaseOutputs
	migrated   bool
	stabilized bool
	finished   bool
}

func (s *Server) DeployAppPhase(req *sdk.DeployAppPhaseCall, resp *sdk.DeployAppPhaseResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		if !req.Phase.Valid() || req.ImageDigest == "" || req.ImageDigest != p.ImageDigest() {
			return invalid(errors.New("AWS deploy phase or artifact digest does not match the plan"))
		}
		var plan stored
		if err := json.Unmarshal(req.Plan.Opaque, &plan); err != nil || !plan.Admitted || plan.DestroyOnly || plan.AccountOnly || plan.ObserveOnly {
			return invalid(errors.New("AWS deployment phases require an admitted deployment plan"))
		}
		outputs, err := decodeOutputs(req.OutputsJSON)
		if err != nil {
			return err
		}
		id := sha256.Sum256(req.Plan.Opaque)
		request := deploy.Request{Target: m.Descriptor(), ImageDigest: req.ImageDigest}
		if req.Phase == sdk.DeployPhaseRegister {
			token, err := newToken()
			if err != nil {
				return err
			}
			backend := &phaseOutputs{values: outputs}
			steps, err := platform.ModuleOps(m).NewDeploySteps(ctx, backend, p, io.Discard)
			if err != nil {
				return err
			}
			if err = steps.Validate(ctx, request); err != nil {
				return err
			}
			if err = steps.RegisterCandidate(ctx, request); err != nil {
				return err
			}
			s.mu.Lock()
			if s.candidates == nil {
				s.candidates = make(map[string]*deploymentCandidate)
			}
			s.candidates[token] = &deploymentCandidate{plan: id, steps: steps, outputs: backend}
			s.mu.Unlock()
			return encode(&resp.StateJSON, token)
		}
		token, err := decode[string](req.StateJSON)
		if err != nil {
			return err
		}
		s.mu.Lock()
		candidate := s.candidates[token]
		s.mu.Unlock()
		if candidate == nil || candidate.plan != id {
			return invalid(errors.New("AWS deployment candidate does not belong to this plan"))
		}
		candidate.mu.Lock()
		defer candidate.mu.Unlock()
		if candidate.finished {
			return invalid(errors.New("AWS deployment candidate is already closed"))
		}
		candidate.outputs.values = outputs
		switch req.Phase {
		case sdk.DeployPhaseMigrate:
			if candidate.migrated {
				return invalid(errors.New("AWS candidate has already run migrations"))
			}
			if err := candidate.steps.RunMigrations(ctx, request); err != nil {
				return err
			}
			candidate.migrated = true
			return encode(&resp.StateJSON, token)
		case sdk.DeployPhaseCleanup:
			if err := candidate.steps.CleanupCandidate(ctx, request); err != nil {
				return err
			}
		case sdk.DeployPhaseStabilize, sdk.DeployPhaseHealth:
			if !candidate.migrated {
				return invalid(errors.New("AWS candidate has not completed migrations"))
			}
			if req.Phase == sdk.DeployPhaseStabilize {
				if err := candidate.steps.Stabilize(ctx, request); err != nil {
					return err
				}
				candidate.stabilized = true
				return nil
			}
			if !candidate.stabilized {
				return invalid(errors.New("AWS candidate has not stabilized"))
			}
			if err := candidate.steps.Health(ctx, request); err != nil {
				return err
			}
		}
		s.mu.Lock()
		candidate.finished = true
		delete(s.candidates, token)
		s.mu.Unlock()
		return nil
	})
}
