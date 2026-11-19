package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/providers/aws/cost"
	"github.com/magelift/magelift/providers/aws/stack"
	"github.com/magelift/magelift/sdk"
)

func (s *Server) TailLogs(req *sdk.TailLogsCall, resp *sdk.TailLogsResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		query, err := decode[platform.LogQuery](req.QueryJSON)
		if err != nil {
			return err
		}
		outputs, err := decodeOutputs(req.OutputsJSON)
		if err != nil {
			return err
		}
		observe := platform.ModuleRuntimeObserve(m)
		if binder, ok := observe.(interface{ BindOutputs(map[string]any) error }); ok {
			if err := binder.BindOutputs(outputs); err != nil {
				return invalid(err)
			}
		}
		events, err := observe.TailLogs(ctx, p, query)
		var partial *platform.PartialLogError
		if err != nil && !errors.As(err, &partial) {
			return err
		}
		if partial != nil {
			var messages []string
			for _, failure := range partial.Failures {
				message := failure.Source
				if failure.Err != nil {
					message += ": " + platform.RedactLogMessage(failure.Err.Error())
				}
				messages = append(messages, message)
			}
			if err := encode(&resp.FailuresJSON, messages); err != nil {
				return err
			}
		}
		return encode(&resp.EventsJSON, events)
	})
}

func decodeOutputs(raw []byte) (map[string]any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	return decode[map[string]any](raw)
}

func (s *Server) CheckRuntime(req *sdk.CheckRuntimeCall, resp *sdk.CheckRuntimeResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		outputs, err := decodeOutputs(req.OutputsJSON)
		if err != nil {
			return err
		}
		health, err := platform.ModuleRuntimeObserve(m).CheckRuntime(ctx, p, outputs)
		if err != nil {
			return err
		}
		return encode(&resp.HealthJSON, health)
	})
}

func (s *Server) PrepareExec(req *sdk.PrepareExecCall, resp *sdk.PrepareExecResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		query, err := decode[platform.ExecQuery](req.QueryJSON)
		if err != nil {
			return err
		}
		outputs, err := decodeOutputs(req.OutputsJSON)
		if err != nil {
			return err
		}
		target, err := platform.ModuleRuntimeObserve(m).PrepareExec(ctx, p, outputs, query)
		if err != nil {
			return err
		}
		return encode(&resp.TargetJSON, target)
	})
}

func (s *Server) PrepareTunnel(req *sdk.PrepareTunnelCall, resp *sdk.PrepareTunnelResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		query, err := decode[platform.TunnelQuery](req.QueryJSON)
		if err != nil {
			return err
		}
		outputs, err := decodeOutputs(req.OutputsJSON)
		if err != nil {
			return err
		}
		tunnel := platform.ModuleRuntimeTunnel(m)
		if tunnel == nil {
			return invalid(platform.ErrNotSupported)
		}
		target, err := tunnel.PrepareTunnel(ctx, p, outputs, query)
		if err != nil {
			return err
		}
		return encode(&resp.TargetJSON, target)
	})
}

func (s *Server) CostInputs(req *sdk.CostInputsRequest, resp *sdk.CostInputsResult) error {
	return respond(req.ProtocolVersion, &resp.Error, 3*time.Minute, func(ctx context.Context) error {
		if req.Plan == nil {
			return invalid(errors.New("AWS cost requires the planned stack"))
		}
		_, _, p, err := checked(req.Envelope, *req.Plan)
		if err != nil {
			return err
		}
		target, err := parseTarget(req.TargetBlock)
		if err != nil {
			return invalid(err)
		}
		cfg := config.Config{Target: config.Target{Provider: "aws", Runtime: string(p.Runtime()), AWS: target}, Account: req.Envelope.Account, Class: req.Envelope.EnvironmentClass, MonthlyBudgetCents: req.Envelope.MonthlyBudgetCents, Defaults: config.Defaults{Region: req.Envelope.Region, Preset: req.Envelope.Preset}}
		report, err := (cost.Estimator{}).Estimate(ctx, p, cfg, platform.CostOptions{Live: req.Live, Budget: req.Budget})
		if err != nil {
			return err
		}
		return encode(&resp.ReportJSON, report)
	})
}

func (s *Server) EdgePlan(req *sdk.EdgePlanCall, resp *sdk.EdgePlanResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		request, err := decode[sdk.EdgePlanRequest](req.RequestJSON)
		if err != nil {
			return err
		}
		if string(request.TargetProvider) != req.Plan.Provider || string(request.TargetRuntime) != req.Plan.Runtime {
			return integrity("AWS edge request target differs from its stored plan")
		}
		if request.DeployedOutputs != nil && request.Intent.Mode == "none" {
			ecs, ok := stack.AsAWSPlanned(p)
			if !ok || ecs.Spec.Edge.Mode != "native" {
				return invalid(errors.New("this environment has no native AWS storefront CDN"))
			}
			request.Intent = ecs.Spec.Edge
		}
		adapter, err := s.edgeAdapter(ctx, m, p)
		if err != nil {
			return err
		}
		if adapter == nil {
			return invalid(platform.ErrNotSupported)
		}
		result, err := adapter.PlanEdge(ctx, request)
		if err != nil {
			return err
		}
		encoded, err := sdk.EncodeEdgePlanRPC(result, sdk.EdgeRPCBindingFor(req.Envelope, req.Plan))
		if err != nil {
			return err
		}
		resp.PlanJSON = encoded
		return nil
	})
}

func (s *Server) EdgeExecute(req *sdk.EdgeExecuteCall, resp *sdk.EdgeExecuteResult) error {
	return withPlan(req.ProtocolVersion, &resp.Error, req.Envelope, req.Plan, func(ctx context.Context, m platform.StackModule, p platform.PlannedStack) error {
		request, binding, version, err := sdk.DecodeEdgeExecutionRPC(req.RequestJSON)
		if err != nil {
			return invalid(err)
		}
		if version == 0 && request.Plan.AdapterID == "aws.edge.native" && request.Plan.Opaque == nil {
			return invalid(errors.New("AWS edge execution requires a host/plugin version that carries opaque plan state"))
		}
		if binding.Provider != "" {
			if err := binding.ValidateFor(req.Envelope, req.Plan); err != nil {
				return integrity(err.Error())
			}
		}
		if string(request.Plan.TargetProvider) != req.Plan.Provider || string(request.Plan.TargetRuntime) != req.Plan.Runtime {
			return integrity("AWS edge execution target differs from its stored plan")
		}
		adapter, err := s.edgeAdapter(ctx, m, p)
		if err != nil {
			return err
		}
		if adapter == nil {
			return invalid(platform.ErrNotSupported)
		}
		result, err := adapter.ExecuteEdge(ctx, request)
		if err != nil {
			return err
		}
		return encode(&resp.ResultJSON, result)
	})
}

func (s *Server) edgeAdapter(ctx context.Context, module platform.StackModule, planned platform.PlannedStack) (sdk.EdgeAdapter, error) {
	if s != nil && s.NewEdgeAdapter != nil {
		return s.NewEdgeAdapter(ctx, module, planned)
	}
	return platform.ModuleEdgeFor(ctx, module, planned)
}
