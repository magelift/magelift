package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/magelift/magelift/internal/cloud/kube"
	"github.com/magelift/magelift/internal/platform"
	provcleanup "github.com/magelift/magelift/providers/gcp/cleanup"
	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	gcpops "github.com/magelift/magelift/providers/gcp/ops"
	providerschema "github.com/magelift/magelift/providers/gcp/schema"
	gcpstack "github.com/magelift/magelift/providers/gcp/stack"
	gcpstate "github.com/magelift/magelift/providers/gcp/state"
	"github.com/magelift/magelift/sdk"
)

// loadSpecCall unmarshals and cross-checks a plan-scoped call. Every
// plan-scoped handler starts here so the integrity check cannot be skipped.
func loadSpecCall(envelope sdk.Envelope, plan sdk.StoredPlan) (gcpstack.Spec, *sdk.OperationError) {
	spec, operr := loadSpec(plan.Opaque)
	if operr != nil {
		return gcpstack.Spec{}, operr
	}
	if operr := checkEnvelope(envelope, plan, spec); operr != nil {
		return gcpstack.Spec{}, operr
	}
	return spec, nil
}

// BootstrapVerify mirrors Bootstrap.VerifyAccount.
func (s *Server) BootstrapVerify(ctx context.Context, req *sdk.BootstrapVerifyCall) (*sdk.BootstrapVerifyResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("bootstrap verify call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	if err := s.bootstrap().VerifyAccount(ctx, spec); err != nil {
		return nil, mapError(err)
	}
	return &sdk.BootstrapVerifyResult{Verified: true}, nil
}

// BootstrapEnsure mirrors Bootstrap.Ensure.
func (s *Server) BootstrapEnsure(ctx context.Context, req *sdk.BootstrapEnsureCall) (*sdk.BootstrapEnsureResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("bootstrap ensure call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	var bootstrapReq platform.BootstrapRequest
	if operr := unmarshalJSON(req.RequestJSON, &bootstrapReq, "bootstrap request"); operr != nil {
		return nil, operr
	}
	result, err := s.bootstrap().Ensure(ctx, spec, bootstrapReq)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(result)
	if operr != nil {
		return nil, operr
	}
	return &sdk.BootstrapEnsureResult{ResultJSON: encoded}, nil
}

// StateStatus mirrors State.Status.
func (s *Server) StateStatus(ctx context.Context, req *sdk.StateStatusCall) (*sdk.StateStatusResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("state status call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	locked, info, backend, err := s.state().Status(ctx, spec)
	if err != nil {
		return nil, stateLockConflict(err)
	}
	result := &sdk.StateStatusResult{Locked: locked, BackendURL: backend}
	if info != nil {
		encoded, operr := marshalJSON(info)
		if operr != nil {
			return nil, operr
		}
		result.InfoJSON = encoded
	}
	return result, nil
}

// StateLock mirrors State.Lock.
func (s *Server) StateLock(ctx context.Context, req *sdk.StateLockCall) (*sdk.StateLockResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("state lock call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	if err := s.state().Lock(ctx, spec, req.Owner); err != nil {
		return nil, stateLockConflict(err)
	}
	return &sdk.StateLockResult{Locked: true}, nil
}

// StateUnlock mirrors State.Unlock.
func (s *Server) StateUnlock(ctx context.Context, req *sdk.StateUnlockCall) (*sdk.StateUnlockResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("state unlock call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	info, err := s.state().Unlock(ctx, spec)
	if err != nil {
		return nil, stateLockConflict(err)
	}
	result := &sdk.StateUnlockResult{}
	if info != nil {
		encoded, operr := marshalJSON(info)
		if operr != nil {
			return nil, operr
		}
		result.InfoJSON = encoded
	}
	return result, nil
}

// StateBackup mirrors State.Backup (Pulumi state snapshot).
func (s *Server) StateBackup(ctx context.Context, req *sdk.StateBackupCall) (*sdk.StateBackupResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("state backup call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	result, err := s.state().Backup(ctx, spec)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(result)
	if operr != nil {
		return nil, operr
	}
	return &sdk.StateBackupResult{ResultJSON: encoded}, nil
}

// StateRestore mirrors State.Restore (Pulumi state restore).
func (s *Server) StateRestore(ctx context.Context, req *sdk.StateRestoreCall) (*sdk.StateRestoreResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("state restore call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	result, err := s.state().Restore(ctx, spec, req.Location)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(result)
	if operr != nil {
		return nil, operr
	}
	return &sdk.StateRestoreResult{ResultJSON: encoded}, nil
}

// SecretList mirrors Secrets.List.
func (s *Server) SecretList(ctx context.Context, req *sdk.SecretListCall) (*sdk.SecretListResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("secret list call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	listed, err := s.secrets().List(ctx, spec)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(listed)
	if operr != nil {
		return nil, operr
	}
	return &sdk.SecretListResult{SecretsJSON: encoded}, nil
}

// SecretSet mirrors Secrets.Set.
func (s *Server) SecretSet(ctx context.Context, req *sdk.SecretSetCall) (*sdk.SecretSetResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("secret set call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	if err := s.secrets().Set(ctx, spec, req.Name, req.Value); err != nil {
		return nil, mapError(err)
	}
	return &sdk.SecretSetResult{Written: true}, nil
}

// SecretRemove mirrors Secrets.Remove.
func (s *Server) SecretRemove(ctx context.Context, req *sdk.SecretRemoveCall) (*sdk.SecretRemoveResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("secret remove call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	if err := s.secrets().Remove(ctx, spec, req.Name); err != nil {
		return nil, mapError(err)
	}
	return &sdk.SecretRemoveResult{Removed: true}, nil
}

// SecretRead serves secretref and Composer credential resolution. The name
// is a full version resource name; no plan is needed.
func (s *Server) SecretRead(ctx context.Context, req *sdk.SecretReadRequest) (*sdk.SecretReadResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("secret read request is required")
	}
	value, err := s.secrets().ReadValue(ctx, req.Name)
	if err != nil {
		return nil, mapError(err)
	}
	return &sdk.SecretReadResult{Value: value}, nil
}

// TailLogs mirrors RuntimeObserve.TailLogs.
func (s *Server) TailLogs(ctx context.Context, req *sdk.TailLogsCall) (*sdk.TailLogsResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("tail logs call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	var query platform.LogQuery
	if operr := unmarshalJSON(req.QueryJSON, &query, "log query"); operr != nil {
		return nil, operr
	}
	outputs, operr := decodeOutputs(req.OutputsJSON)
	if operr != nil {
		return nil, operr
	}
	observe := s.observe()
	if err := observe.BindOutputs(outputs); err != nil {
		return nil, InvalidError("log outputs are incomplete: " + err.Error())
	}
	events, err := observe.TailLogs(ctx, gcpstack.SpecPlanned{Spec: spec}, query)
	if err != nil {
		var partial *platform.PartialLogError
		if errors.As(err, &partial) {
			return partialLogsResult(events, partial)
		}
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(events)
	if operr != nil {
		return nil, operr
	}
	return &sdk.TailLogsResult{EventsJSON: encoded}, nil
}

func partialLogsResult(events []platform.LogEvent, partial *platform.PartialLogError) (*sdk.TailLogsResult, *sdk.OperationError) {
	encoded, operr := marshalJSON(events)
	if operr != nil {
		return nil, operr
	}
	formatted := make([]string, 0, len(partial.Failures))
	for _, failure := range partial.Failures {
		text := failure.Source
		if failure.Err != nil {
			text += ": " + platform.RedactLogMessage(failure.Err.Error())
		}
		formatted = append(formatted, text)
	}
	failures, operr := marshalJSON(formatted)
	if operr != nil {
		return nil, operr
	}
	return &sdk.TailLogsResult{EventsJSON: encoded, FailuresJSON: failures}, nil
}

// CheckRuntime mirrors RuntimeObserve.CheckRuntime.
func (s *Server) CheckRuntime(ctx context.Context, req *sdk.CheckRuntimeCall) (*sdk.CheckRuntimeResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("check runtime call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	outputs, operr := decodeOutputs(req.OutputsJSON)
	if operr != nil {
		return nil, operr
	}
	health, err := s.observe().CheckRuntime(ctx, gcpstack.SpecPlanned{Spec: spec}, outputs)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(health)
	if operr != nil {
		return nil, operr
	}
	return &sdk.CheckRuntimeResult{HealthJSON: encoded}, nil
}

// PrepareExec mirrors RuntimeObserve.PrepareExec.
func (s *Server) PrepareExec(ctx context.Context, req *sdk.PrepareExecCall) (*sdk.PrepareExecResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("prepare exec call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	outputs, operr := decodeOutputs(req.OutputsJSON)
	if operr != nil {
		return nil, operr
	}
	var query platform.ExecQuery
	if operr := unmarshalJSON(req.QueryJSON, &query, "exec query"); operr != nil {
		return nil, operr
	}
	target, err := s.observe().PrepareExec(ctx, gcpstack.SpecPlanned{Spec: spec}, outputs, query)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(target)
	if operr != nil {
		return nil, operr
	}
	return &sdk.PrepareExecResult{TargetJSON: encoded}, nil
}

// PrepareTunnel mirrors RuntimeTunnel.PrepareTunnel.
func (s *Server) PrepareTunnel(ctx context.Context, req *sdk.PrepareTunnelCall) (*sdk.PrepareTunnelResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("prepare tunnel call is required")
	}
	spec, operr := loadSpecCall(req.Envelope, req.Plan)
	if operr != nil {
		return nil, operr
	}
	outputs, operr := decodeOutputs(req.OutputsJSON)
	if operr != nil {
		return nil, operr
	}
	var query platform.TunnelQuery
	if operr := unmarshalJSON(req.QueryJSON, &query, "tunnel query"); operr != nil {
		return nil, operr
	}
	target, err := s.tunnel().PrepareTunnel(ctx, spec, outputs, query)
	if err != nil {
		if errors.Is(err, platform.ErrNotSupported) {
			return nil, &sdk.OperationError{Code: sdk.ErrCodeInvalid, Message: err.Error()}
		}
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(target)
	if operr != nil {
		return nil, operr
	}
	return &sdk.PrepareTunnelResult{TargetJSON: encoded}, nil
}

// CostInputs mirrors CostEstimator.Estimate without requiring a stored plan.
func (s *Server) CostInputs(ctx context.Context, req *sdk.CostInputsRequest) (*sdk.CostInputsResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("cost inputs request is required")
	}
	var target *providerschema.GCPTarget
	if len(bytes.TrimSpace(req.TargetBlock)) > 0 {
		var parsed providerschema.GCPTarget
		if err := yaml.Unmarshal(req.TargetBlock, &parsed); err != nil {
			return nil, InvalidError("target block is not valid YAML: " + err.Error())
		}
		if problems := providerschema.Validate(&parsed, req.Runtime); len(problems) > 0 {
			return nil, InvalidError("target block is invalid: " + strings.Join(problems, "; "))
		}
		target = &parsed
	}
	report, err := s.estimator().Estimate(ctx, gcpcost.EstimateInputs{
		Envelope: req.Envelope,
		Target:   target,
		Runtime:  req.Runtime,
		Live:     req.Live,
		Budget:   req.Budget,
	})
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(report)
	if operr != nil {
		return nil, operr
	}
	return &sdk.CostInputsResult{ReportJSON: encoded}, nil
}

// Inventory mirrors CleanupProvider.Inventory.
func (s *Server) Inventory(ctx context.Context, req *sdk.InventoryCall) (*sdk.InventoryResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("inventory call is required")
	}
	var inventoryReq sdk.CleanupInventoryRequest
	if operr := unmarshalJSON(req.RequestJSON, &inventoryReq, "inventory request"); operr != nil {
		return nil, operr
	}
	provider, operr := s.cleanupProvider(ctx, inventoryReq.Project, inventoryReq.Marker)
	if operr != nil {
		return nil, operr
	}
	resources, err := provider.Inventory(ctx, inventoryReq)
	if err != nil {
		return nil, mapError(err)
	}
	encoded, operr := marshalJSON(resources)
	if operr != nil {
		return nil, operr
	}
	return &sdk.InventoryResult{ResourcesJSON: encoded}, nil
}

// Delete mirrors CleanupProvider.Delete.
func (s *Server) Delete(ctx context.Context, req *sdk.DeleteCall) (*sdk.DeleteResult, *sdk.OperationError) {
	if req == nil {
		return nil, InvalidError("delete call is required")
	}
	var resource sdk.CleanupResource
	if operr := unmarshalJSON(req.ResourceJSON, &resource, "cleanup resource"); operr != nil {
		return nil, operr
	}
	provider, operr := s.cleanupProvider(ctx, req.Project, req.Marker)
	if operr != nil {
		return nil, operr
	}
	if err := provider.Delete(ctx, resource); err != nil {
		return nil, mapError(err)
	}
	return &sdk.DeleteResult{Deleted: true}, nil
}

func (s *Server) cleanupProvider(ctx context.Context, project, marker string) (provcleanup.GCPCloudSQLProvider, *sdk.OperationError) {
	if strings.TrimSpace(project) == "" {
		return provcleanup.GCPCloudSQLProvider{}, InvalidError("cleanup requires a GCP project")
	}
	sqlClient, err := s.cleanupSQL(ctx)
	if err != nil {
		return provcleanup.GCPCloudSQLProvider{}, mapError(err)
	}
	return provcleanup.GCPCloudSQLProvider{SQL: sqlClient, Project: strings.TrimSpace(project), Marker: marker}, nil
}

func decodeOutputs(data []byte) (map[string]any, *sdk.OperationError) {
	if len(bytes.TrimSpace(data)) == 0 {
		return map[string]any{}, nil
	}
	var outputs map[string]any
	if err := json.Unmarshal(data, &outputs); err != nil {
		return nil, InvalidError("malformed outputs: " + err.Error())
	}
	if outputs == nil {
		outputs = map[string]any{}
	}
	return outputs, nil
}

// stateLockConflict maps lock contention to conflict. Lock absence maps to
// not-found so callers can distinguish "no lock" from failures.
func stateLockConflict(err error) *sdk.OperationError {
	if errors.Is(err, gcpstate.ErrNotLocked) {
		return &sdk.OperationError{Code: sdk.ErrCodeNotFound, Message: err.Error()}
	}
	if errors.Is(err, gcpops.ErrStateBucketMissing) {
		return &sdk.OperationError{Code: sdk.ErrCodeConflict, Message: err.Error()}
	}
	return mapError(err)
}

func (s *Server) bootstrap() gcpops.Bootstrap {
	if s == nil {
		return gcpops.Bootstrap{}
	}
	return s.Bootstrap
}

func (s *Server) state() gcpops.State {
	if s == nil {
		return gcpops.State{}
	}
	return s.State
}

func (s *Server) secrets() gcpops.Secrets {
	if s == nil {
		return gcpops.Secrets{}
	}
	return s.Secrets
}

func (s *Server) estimator() gcpcost.Estimator {
	if s == nil {
		return gcpcost.Estimator{}
	}
	return s.Estimator
}

func (s *Server) observe() *kube.Observe {
	if s == nil {
		return kube.NewObserveWithFactory(nil)
	}
	return kube.NewObserveWithFactory(s.KubeClients)
}

func (s *Server) tunnel() gcpops.Tunnel {
	return gcpops.NewTunnel(s.observe())
}
