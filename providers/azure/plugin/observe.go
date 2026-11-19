package plugin

import (
	"context"
	"errors"
	"time"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func (s *Server) CheckRuntime(req *sdk.CheckRuntimeCall, resp *sdk.CheckRuntimeResult) error {
	if req == nil || resp == nil {
		return errors.New("runtime request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(&sdk.StackCall{Envelope: req.Envelope, Plan: req.Plan})
	if err != nil || plan.AccountOnly {
		resp.Error = InvalidError("invalid Azure runtime plan")
		return nil
	}
	outputs, opErr := decodeOutputs(req.OutputsJSON)
	if opErr != nil {
		resp.Error = opErr
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	health, err := kube.NewObserveWithFactory(s.deployFactory()).CheckRuntime(ctx, nil, outputs)
	if err != nil {
		resp.Error = mapError(err)
		return nil
	}
	resp.HealthJSON, resp.Error = marshalJSON(health)
	return nil
}

func (s *Server) TailLogs(req *sdk.TailLogsCall, resp *sdk.TailLogsResult) error {
	if req == nil || resp == nil {
		return errors.New("logs request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(&sdk.StackCall{Envelope: req.Envelope, Plan: req.Plan})
	if err != nil || plan.AccountOnly {
		resp.Error = InvalidError("invalid Azure runtime plan")
		return nil
	}
	outputs, opErr := decodeOutputs(req.OutputsJSON)
	if opErr != nil {
		resp.Error = opErr
		return nil
	}
	var query platform.LogQuery
	if opErr := unmarshalJSON(req.QueryJSON, &query, "log query"); opErr != nil {
		resp.Error = opErr
		return nil
	}
	observe := kube.NewObserveWithFactory(s.deployFactory())
	if err := observe.BindOutputs(outputs); err != nil {
		resp.Error = mapError(err)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	events, err := observe.TailLogs(ctx, nil, query)
	if err != nil {
		var partial *platform.PartialLogError
		if !errors.As(err, &partial) {
			resp.Error = mapError(err)
			return nil
		}
		var failures []string
		for _, failure := range partial.Failures {
			message := failure.Source
			if failure.Err != nil {
				message += ": " + platform.RedactLogMessage(failure.Err.Error())
			}
			failures = append(failures, message)
		}
		resp.FailuresJSON, resp.Error = marshalJSON(failures)
		if resp.Error != nil {
			return nil
		}
	}
	resp.EventsJSON, resp.Error = marshalJSON(events)
	return nil
}

func (s *Server) PrepareExec(req *sdk.PrepareExecCall, resp *sdk.PrepareExecResult) error {
	if req == nil || resp == nil {
		return errors.New("exec request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(&sdk.StackCall{Envelope: req.Envelope, Plan: req.Plan})
	if err != nil || plan.AccountOnly {
		resp.Error = InvalidError("invalid Azure runtime plan")
		return nil
	}
	outputs, opErr := decodeOutputs(req.OutputsJSON)
	if opErr != nil {
		resp.Error = opErr
		return nil
	}
	var query platform.ExecQuery
	if opErr = unmarshalJSON(req.QueryJSON, &query, "exec query"); opErr != nil {
		resp.Error = opErr
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	target, err := kube.NewObserveWithFactory(s.deployFactory()).PrepareExec(ctx, nil, outputs, query)
	if err != nil {
		resp.Error = mapError(err)
		return nil
	}
	resp.TargetJSON, resp.Error = marshalJSON(target)
	return nil
}

func (s *Server) PrepareTunnel(req *sdk.PrepareTunnelCall, resp *sdk.PrepareTunnelResult) error {
	if req == nil || resp == nil {
		return errors.New("tunnel request and response are required")
	}
	if err := sdk.ValidateProtocolVersion(req.ProtocolVersion); err != nil {
		return err
	}
	plan, err := checkedStoredPlan(&sdk.StackCall{Envelope: req.Envelope, Plan: req.Plan})
	if err != nil || plan.AccountOnly {
		resp.Error = InvalidError("invalid Azure runtime plan")
		return nil
	}
	outputs, opErr := decodeOutputs(req.OutputsJSON)
	if opErr != nil {
		resp.Error = opErr
		return nil
	}
	var query platform.TunnelQuery
	if opErr = unmarshalJSON(req.QueryJSON, &query, "tunnel query"); opErr != nil {
		resp.Error = opErr
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	target, err := kube.NewObserveWithFactory(s.deployFactory()).PrepareTunnel(ctx, nil, outputs, query)
	if err != nil {
		if errors.Is(err, platform.ErrNotSupported) {
			resp.Error = InvalidError(err.Error())
		} else {
			resp.Error = mapError(err)
		}
		return nil
	}
	resp.TargetJSON, resp.Error = marshalJSON(target)
	return nil
}
