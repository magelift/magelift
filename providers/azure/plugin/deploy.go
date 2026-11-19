package plugin

import (
	"context"
	"fmt"
	"time"

	"encoding/json"
	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/internal/secretsafe"
	"github.com/magelift/magelift/sdk"
	"k8s.io/client-go/kubernetes"
	"reflect"
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
func (s *Server) deployAppPhase(ctx context.Context, req *sdk.DeployAppPhaseCall) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
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
	plan, err := checkedStoredPlan(&sdk.StackCall{Envelope: req.Envelope, Plan: req.Plan})
	if err != nil || plan.AccountOnly || plan.Spec.ObserveOnly || plan.DestroyPreview {
		return nil, InvalidError("invalid Azure application deployment plan")
	}
	expected := deployInputs(plan.Spec)
	if !reflect.DeepEqual(inputs, expected) {
		return nil, InvalidError("deploy inputs do not match the Azure plan")
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
		engine := ""
		if plan.Spec.SearchImage != "" {
			engine = "opensearch"
		}
		return s.deployHealth(ctx, candidate, runtime, outputs, inputs, req.ImageDigest, engine)
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
	client, err := s.deployClient(outputs)
	if err != nil {
		return nil, nil, mapError(err)
	}
	namespace, _ := outputs[platform.OutputNamespace].(string)
	runtime, err := kube.NewRuntimeFromClientWithNamespace(client, namespace)
	if err != nil {
		return nil, nil, mapError(err)
	}
	return kube.NewCandidateFromFactory(s.deployFactory()), runtime, nil
}

// deployFactory resolves the client factory. The default uses the secret
// kubeconfig output, whose Azure exec credential helper must be installed.
func (s *Server) deployFactory() kube.ClientFactory {
	if s != nil && s.KubeClients != nil {
		return s.KubeClients
	}
	return kube.ClientFromOutputs
}

func (s *Server) deployClient(outputs map[string]any) (kubernetes.Interface, error) {
	return s.deployFactory()(outputs)
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

func (s *Server) deployHealth(ctx context.Context, candidate *kube.CandidateStore, runtime *kube.DeploymentRuntime, outputs map[string]any, inputs sdk.DeployInputs, imageDigest, engine string) (*sdk.DeployAppPhaseResult, *sdk.OperationError) {
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
	// Bounded request through the serving path first: a broken PHP-FPM or
	// upstream route fails here even when jobs and scheduler counters look
	// healthy. The probe job then asserts database and search specifics.
	client, err := s.deployClient(outputs)
	if err != nil {
		return nil, mapError(err)
	}
	storefront, err := kube.ResolveStorefrontURL(ctx, client, request.Namespace, outputs, request.ServiceName)
	if err != nil {
		return nil, InvalidError(err.Error())
	}
	if storefront == "" {
		return nil, InvalidError("serving storefront URL cannot be determined; refusing to declare a healthy deploy")
	}
	var doer kube.HTTPDoer
	if s != nil {
		doer = s.HTTPDoer
	}
	if err := kube.ProbeStorefrontHTTP(ctx, doer, storefront); err != nil {
		return nil, mapError(fmt.Errorf("serving path unhealthy: %w", err))
	}
	command := platform.MagentoProbeShell(request.SearchEndpoint, request.SearchEndpoint != "", engine)
	if err := candidate.RunProbe(ctx, request, command); err != nil {
		return nil, mapError(err)
	}
	return &sdk.DeployAppPhaseResult{Message: "serving health verified"}, nil
}

func InvalidError(message string) *sdk.OperationError {
	return &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: message}
}
func mapError(err error) *sdk.OperationError {
	message, _ := secretsafe.RedactSensitiveText(err.Error())
	return &sdk.OperationError{Code: sdk.ErrCodeUpstream, Message: message}
}
func unmarshalJSON(raw []byte, out any, description string) *sdk.OperationError {
	if err := json.Unmarshal(raw, out); err != nil {
		return InvalidError("invalid " + description)
	}
	return nil
}
func marshalJSON(value any) ([]byte, *sdk.OperationError) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, &sdk.OperationError{Code: sdk.ErrCodeInternal, Message: "encode deploy state"}
	}
	return data, nil
}
func decodeOutputs(raw []byte) (map[string]any, *sdk.OperationError) {
	var outputs map[string]any
	if err := json.Unmarshal(raw, &outputs); err != nil || outputs == nil {
		return nil, InvalidError("invalid stack outputs")
	}
	return outputs, nil
}
func (s *Server) DeployAppPhase(req *sdk.DeployAppPhaseCall, resp *sdk.DeployAppPhaseResult) error {
	if req == nil || resp == nil {
		return fmt.Errorf("deploy request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Minute)
	defer cancel()
	result, opErr := s.deployAppPhase(ctx, req)
	if opErr != nil {
		resp.Error = opErr
	} else {
		*resp = *result
	}
	return nil
}
