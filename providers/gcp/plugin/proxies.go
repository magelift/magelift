package plugin

import (
	"context"
	"errors"
	"strings"

	gcpedge "github.com/magelift/magelift/providers/gcp/edge"
	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
	"github.com/magelift/magelift/sdk"
)

// EdgePlan forwards an edge adapter Plan call.
func (s *Server) EdgePlan(ctx context.Context, req *sdk.EdgePlanCall) (*sdk.EdgePlanResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("edge plan call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	var edgeReq sdk.EdgePlanRequest
	if operr := unmarshalJSON(req.RequestJSON, &edgeReq, "edge plan request"); operr != nil {
		return nil, operr
	}
	adapter, operr := s.edgeAdapter(ctx, spec.Identity.GCPProject)
	if operr != nil {
		return nil, operr
	}
	plan, err := adapter.PlanEdge(ctx, edgeReq)
	if err != nil {
		return nil, proxyError(err)
	}
	encoded, operr := marshalJSON(plan)
	if operr != nil {
		return nil, operr
	}
	return &sdk.EdgePlanResult{PlanJSON: encoded}, nil
}

// EdgeExecute forwards an edge adapter execution.
func (s *Server) EdgeExecute(ctx context.Context, req *sdk.EdgeExecuteCall) (*sdk.EdgeExecuteResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("edge execute call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	var execReq sdk.EdgeExecutionRequest
	if operr := unmarshalJSON(req.RequestJSON, &execReq, "edge execution request"); operr != nil {
		return nil, operr
	}
	adapter, operr := s.edgeAdapter(ctx, spec.Identity.GCPProject)
	if operr != nil {
		return nil, operr
	}
	result, err := adapter.ExecuteEdge(ctx, execReq)
	if err != nil {
		return nil, proxyError(err)
	}
	encoded, operr := marshalJSON(result)
	if operr != nil {
		return nil, operr
	}
	return &sdk.EdgeExecuteResult{ResultJSON: encoded}, nil
}

// ResiliencePlan forwards a resilience adapter Plan call.
func (s *Server) ResiliencePlan(ctx context.Context, req *sdk.ResiliencePlanCall) (*sdk.ResiliencePlanResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("resilience plan call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	var planReq sdk.ResiliencePlanRequest
	if operr := unmarshalJSON(req.RequestJSON, &planReq, "resilience plan request"); operr != nil {
		return nil, operr
	}
	adapter, operr := s.resilienceAdapter(ctx, spec.Identity.GCPProject)
	if operr != nil {
		return nil, operr
	}
	plan, err := adapter.PlanResilience(ctx, planReq)
	if err != nil {
		return nil, proxyError(err)
	}
	encoded, operr := marshalJSON(plan)
	if operr != nil {
		return nil, operr
	}
	return &sdk.ResiliencePlanResult{PlanJSON: encoded}, nil
}

// ResilienceExecute forwards a resilience stage execution.
func (s *Server) ResilienceExecute(ctx context.Context, req *sdk.ResilienceExecuteCall) (*sdk.ResilienceExecuteResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("resilience execute call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	var execReq sdk.ResilienceExecutionRequest
	if operr := unmarshalJSON(req.RequestJSON, &execReq, "resilience execution request"); operr != nil {
		return nil, operr
	}
	adapter, operr := s.resilienceAdapter(ctx, spec.Identity.GCPProject)
	if operr != nil {
		return nil, operr
	}
	result, err := adapter.ExecuteResilience(ctx, execReq)
	if err != nil {
		return nil, proxyError(err)
	}
	encoded, operr := marshalJSON(result)
	if operr != nil {
		return nil, operr
	}
	return &sdk.ResilienceExecuteResult{ResultJSON: encoded}, nil
}

func (s *Server) edgeAdapter(ctx context.Context, project string) (sdk.EdgeAdapter, *sdk.OperationError) {
	if strings.TrimSpace(project) == "" {
		return nil, InvalidError("stored plan lacks the GCP project")
	}
	if s != nil && s.NewEdgeAdapter != nil {
		adapter, err := s.NewEdgeAdapter(ctx, project)
		if err != nil {
			return nil, mapError(err)
		}
		return adapter, nil
	}
	adapter, err := gcpedge.NewNativeSDKLifecycleAdapter(ctx, project, gcpedge.AlwaysHealthy{}, sdk.DefaultEdgeOperationPolicy())
	if err != nil {
		return nil, mapError(err)
	}
	return adapter, nil
}

func (s *Server) resilienceAdapter(ctx context.Context, project string) (sdk.ResilienceAdapter, *sdk.OperationError) {
	if strings.TrimSpace(project) == "" {
		return nil, InvalidError("stored plan lacks the GCP project")
	}
	if s != nil && s.NewResilienceAdapter != nil {
		adapter, err := s.NewResilienceAdapter(ctx, project)
		if err != nil {
			return nil, mapError(err)
		}
		return adapter, nil
	}
	sqlClient, err := s.cleanupSQL(ctx)
	if err != nil {
		return nil, mapError(err)
	}
	api, err := gcpresilience.NewNativeAPIWithSQLAndProjectionAndPubSub(nil, nil, sqlClient, nil, gcpresilience.NativeAPIConfig{Project: project}, nil)
	if err != nil {
		return nil, mapError(err)
	}
	client, err := gcpresilience.NewNativeResilienceClient(api)
	if err != nil {
		return nil, mapError(err)
	}
	adapter, err := gcpresilience.New(client, sdk.DefaultResilienceOperationPolicy())
	if err != nil {
		return nil, mapError(err)
	}
	return adapter, nil
}

// proxyError maps adapter errors. Edge capability errors (unsupported action
// for this adapter) are invalid requests, not upstream failures: the
// adapter descriptor advertises support precisely so callers can avoid them.
func proxyError(err error) *sdk.OperationError {
	var capability sdk.EdgeCapabilityError
	if errors.As(err, &capability) {
		return &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: capability.Error()}
	}
	return mapError(err)
}
