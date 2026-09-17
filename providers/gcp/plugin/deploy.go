package plugin

import (
	"context"
	"fmt"
	"time"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
	gcpauth "github.com/magelift/magelift/providers/gcp/auth"
	"github.com/magelift/magelift/sdk"
)

// deployPhaseTimeouts bound each stabilize/health wait inside a phase call.
// The operation timeout (45m, non-retryable) caps the whole call.
const (
	deployWaitInterval = 5 * time.Second
	deployWaitTimeout  = 30 * time.Minute
)

// DeployAppPhase executes one Magento deploy phase inside the provider:
// candidate registration, migration run, candidate cleanup, service
// stabilization, or serving health. The core drives phases in flow order
// and holds the opaque candidate handle between calls; all execution,
// clients, and waits live here, so deploy behavior changes ship with the
// provider, never the core.
func (s *Server) DeployAppPhase(ctx context.Context, req *sdk.DeployAppPhaseCall) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("deploy phase call is required")
	}
	if !req.Phase.Valid() {
		return nil, InvalidError(fmt.Sprintf("unknown deploy phase %q", string(req.Phase)))
	}
	var inputs sdk.DeployInputs
	if operr := unmarshalJSON(req.Plan.DeployInputsJSON, &inputs, "deploy inputs"); operr != nil {
		return nil, operr
	}
	if err := inputs.Validate(); err != nil {
		return nil, InvalidError("deploy inputs are invalid: " + err.Error())
	}
	if req.ImageDigest != inputs.ImageDigest {
		return nil, InvalidError("deployment digest does not match the planned artifact")
	}
	outputs, operr := decodeOutputs(req.OutputsJSON)
	if operr != nil {
		return nil, operr
	}
	candidate, runtime, operr := s.deployStores(outputs)
	if operr != nil {
		return nil, operr
	}
	switch req.Phase {
	case sdk.DeployPhaseRegister:
		return s.deployRegister(ctx, candidate, outputs, inputs, req.ImageDigest)
	case sdk.DeployPhaseMigrate:
		return s.deployMigrate(ctx, candidate, req.StateJSON)
	case sdk.DeployPhaseCleanup:
		return s.deployCleanup(ctx, candidate, req.StateJSON)
	case sdk.DeployPhaseStabilize:
		if err := kube.WaitForHealthyService(ctx, outputs, runtime, deployWaitInterval, deployWaitTimeout); err != nil {
			return nil, mapError(err)
		}
		return &sdk.DeployAppPhaseResult{Message: "service stabilized"}, nil
	case sdk.DeployPhaseHealth:
		return s.deployHealth(ctx, candidate, runtime, outputs, inputs, req.ImageDigest)
	default:
		return nil, InvalidError(fmt.Sprintf("unknown deploy phase %q", string(req.Phase)))
	}
}

func (s *Server) deployStores(outputs map[string]any) (*kube.CandidateStore, *kube.DeploymentRuntime, *sdk.OperationError) {
	if s != nil && s.NewDeployStores != nil {
		candidate, runtime, err := s.NewDeployStores(s.KubeClients, outputs)
		if err != nil {
			return nil, nil, mapError(err)
		}
		return candidate, runtime, nil
	}
	var factory kube.ClientFactory
	if s != nil {
		factory = s.KubeClients
	}
	if factory == nil {
		factory = gcpauth.NewClientFactory()
	}
	candidate := kube.NewCandidateFromFactory(factory)
	client, err := factory(outputs)
	if err != nil {
		return nil, nil, mapError(err)
	}
	runtime, err := kube.NewRuntimeFromClient(client)
	if err != nil {
		return nil, nil, mapError(err)
	}
	return candidate, runtime, nil
}

func (s *Server) deployRegister(ctx context.Context, candidate *kube.CandidateStore, outputs map[string]any, inputs sdk.DeployInputs, imageDigest string) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
	request, err := kube.CandidateRequestFromOutputs(outputs, inputs, imageDigest)
	if err != nil {
		return nil, mapError(err)
	}
	registered, err := candidate.RegisterCandidate(ctx, request)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(registered)
	if operr != nil {
		return nil, operr
	}
	return &sdk.DeployAppPhaseResult{StateJSON: encoded, Message: "candidate registered"}, nil
}

func (s *Server) deployMigrate(ctx context.Context, candidate *kube.CandidateStore, state []byte) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
	var registered kube.Candidate
	if operr := unmarshalJSON(state, &registered, "candidate handle"); operr != nil {
		return nil, operr
	}
	runErr := candidate.RunMigrations(ctx, registered)
	cleanupErr := candidate.Cleanup(ctx, registered)
	if runErr != nil {
		if cleanupErr != nil {
			return nil, mapError(fmt.Errorf("run Magento migrations: %w; clean up candidate: %v", runErr, cleanupErr))
		}
		return nil, mapError(runErr)
	}
	if cleanupErr != nil {
		return nil, mapError(cleanupErr)
	}
	return &sdk.DeployAppPhaseResult{Message: "migrations complete"}, nil
}

func (s *Server) deployCleanup(ctx context.Context, candidate *kube.CandidateStore, state []byte) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
	if len(state) == 0 {
		return &sdk.DeployAppPhaseResult{Message: "no candidate to clean"}, nil
	}
	var registered kube.Candidate
	if operr := unmarshalJSON(state, &registered, "candidate handle"); operr != nil {
		return nil, operr
	}
	if err := candidate.Cleanup(ctx, registered); err != nil {
		return nil, mapError(err)
	}
	return &sdk.DeployAppPhaseResult{Message: "candidate cleaned"}, nil
}

func (s *Server) deployHealth(ctx context.Context, candidate *kube.CandidateStore, runtime *kube.DeploymentRuntime, outputs map[string]any, inputs sdk.DeployInputs, imageDigest string) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
	health, err := kube.WaitForIntendedRollout(ctx, outputs, imageDigest, runtime, deployWaitInterval, deployWaitTimeout)
	if err != nil {
		return nil, mapError(err)
	}
	if health.ImageDigest != imageDigest {
		return nil, InvalidError(fmt.Sprintf("served image %q does not match the release", health.ImageDigest))
	}
	request, err := kube.CandidateRequestFromOutputs(outputs, inputs, imageDigest)
	if err != nil {
		return nil, mapError(err)
	}
	command := platform.MagentoProbeShell(request.SearchEndpoint, request.SearchEndpoint != "")
	if err := candidate.RunProbe(ctx, request, command); err != nil {
		return nil, mapError(err)
	}
	return &sdk.DeployAppPhaseResult{Message: "serving health verified"}, nil
}
