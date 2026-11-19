package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/magelift/magelift/internal/deploy"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

type recordedCandidate struct {
	deploy.Steps
	outputs *phaseOutputs
	calls   []string
}

func (s *recordedCandidate) RunMigrations(context.Context, deploy.Request) error {
	s.calls = append(s.calls, "migrate")
	return nil
}
func (s *recordedCandidate) Stabilize(context.Context, deploy.Request) error {
	s.calls = append(s.calls, "stabilize")
	return nil
}
func (s *recordedCandidate) Health(context.Context, deploy.Request) error {
	if s.outputs.values["rollout"] != "ready" {
		return errors.New("missing final rollout outputs")
	}
	s.calls = append(s.calls, "health")
	return nil
}

func TestAWSDeploymentKeepsRegisteredCandidateThroughHealth(t *testing.T) {
	req := shopRequest(t, "ecs-fargate")
	req.Admit = true
	s := Server{admit: func(_ context.Context, _ platform.StackModule, p platform.PlannedStack) (platform.PlannedStack, error) {
		return p, nil
	}}
	var plan sdk.PlanResult
	if err := s.Plan(req, &plan); err != nil || plan.Error != nil {
		t.Fatalf("plan: %v %v", err, plan.Error)
	}
	fake := &recordedCandidate{outputs: &phaseOutputs{}}
	const token = "test-candidate"
	s.candidates = map[string]*deploymentCandidate{token: {plan: sha256.Sum256(plan.Plan.Opaque), steps: fake, outputs: fake.outputs}}
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	call := sdk.DeployAppPhaseCall{ProtocolVersion: sdk.ProtocolV1, Envelope: req.Envelope, Plan: plan.Plan, ImageDigest: plan.Plan.ImageDigest, StateJSON: tokenJSON}
	phase := func(name sdk.DeployAppPhase, valid bool) *sdk.DeployAppPhaseResult {
		t.Helper()
		call.Phase = name
		var reply sdk.DeployAppPhaseResult
		if err := s.DeployAppPhase(&call, &reply); err != nil || (reply.Error == nil) != valid {
			t.Fatalf("%s: %v %v", name, err, reply.Error)
		}
		return &reply
	}
	phase(sdk.DeployPhaseHealth, false)
	reply := phase(sdk.DeployPhaseMigrate, true)
	call.StateJSON = reply.StateJSON
	phase(sdk.DeployPhaseMigrate, false)
	phase(sdk.DeployPhaseHealth, false)
	phase(sdk.DeployPhaseStabilize, true)
	call.OutputsJSON = []byte(`{"rollout":"ready"}`)
	phase(sdk.DeployPhaseHealth, true)
	phase(sdk.DeployPhaseHealth, false)
	if !slices.Equal(fake.calls, []string{"migrate", "stabilize", "health"}) {
		t.Fatal(fake.calls)
	}
}
