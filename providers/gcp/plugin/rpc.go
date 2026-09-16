package plugin

import (
	"context"
	"net/rpc"

	"github.com/hashicorp/go-plugin"

	"github.com/magelift/magelift/sdk"
)

// HandshakeConfig gates transport compatibility before Describe negotiates
// operation semantics. The core client builds the identical config from the
// same SDK constants.
var HandshakeConfig = plugin.HandshakeConfig{
	ProtocolVersion:  sdk.HandshakeProtocolVersion,
	MagicCookieKey:   sdk.HandshakeCookieKey,
	MagicCookieValue: sdk.HandshakeCookieValue,
}

// PluginMap is the single plugin served by provider binaries.
func PluginMap(server *Server) map[string]plugin.Plugin {
	return map[string]plugin.Plugin{sdk.PluginName: &RPCPlugin{Impl: server}}
}

// RPCPlugin implements plugin.Plugin over net/rpc with gob encoding.
type RPCPlugin struct {
	Impl *Server
}

var _ plugin.Plugin = (*RPCPlugin)(nil)

// Server returns the net/rpc server. A nil Impl serves zero-value
// production wiring so the plugin never crashes on construction.
func (p *RPCPlugin) Server(*plugin.MuxBroker) (any, error) {
	server := p.Impl
	if server == nil {
		server = &Server{}
	}
	return &RPCServer{Server: server}, nil
}

// Client is unused: the core ships its own client (it cannot import the
// provider module). It exists only to satisfy the interface.
func (p *RPCPlugin) Client(*plugin.MuxBroker, *rpc.Client) (any, error) {
	return nil, nil
}

// RPCServer exposes the 29 operations over net/rpc. Every method validates
// the protocol version, enforces the per-operation timeout, and reports
// failures as typed in-band errors: a non-nil rpc return means transport
// breakage, never an operation failure.
type RPCServer struct {
	Server *Server
}

func (s *RPCServer) server() *Server {
	if s == nil || s.Server == nil {
		return &Server{}
	}
	return s.Server
}

// dispatch runs one operation with version validation and timeout. The
// handler runs in a goroutine so a stuck backend cannot hang the rpc call
// past the per-operation deadline.
func dispatch[Resp any](s *RPCServer, operation sdk.Operation, version string, resp *Resp, run func(context.Context) (*Resp, *sdk.OperationError)) error {
	if err := sdk.ValidateProtocolVersion(version); err != nil {
		return err
	}
	server := s.server()
	timeout := server.timeoutFor(operation)
	ctx, cancel := context.WithTimeout(context.Background(), timeout.Duration)
	defer cancel()
	type outcome struct {
		result *Resp
		operr  *sdk.OperationError
	}
	done := make(chan outcome, 1)
	go func() {
		result, operr := run(ctx)
		done <- outcome{result: result, operr: operr}
	}()
	select {
	case out := <-done:
		if out.operr != nil {
			setError(resp, out.operr)
			return nil
		}
		if out.result != nil {
			*resp = *out.result
		}
		return nil
	case <-ctx.Done():
		setError(resp, &sdk.OperationError{
			Code:      sdk.ErrCodeTimeout,
			Message:   string(operation) + " timed out; reconcile before retrying",
			Retryable: timeout.RetryableOnTimeout,
		})
		return nil
	}
}

// setError stores a typed error on a result value. Every result type carries
// an Error field; the type switch keeps the mapping compile-checked.
func setError[Resp any](resp *Resp, operr *sdk.OperationError) {
	switch result := any(resp).(type) {
	case *sdk.DescribeResponse:
		result.Error = operr
	case *sdk.ValidateConfigResult:
		result.Error = operr
	case *sdk.PlanResult:
		result.Error = operr
	case *sdk.LifecycleResult:
		result.Error = operr
	case *sdk.OutputsResult:
		result.Error = operr
	case *sdk.DestroyLeftoverBackupsResult:
		result.Error = operr
	case *sdk.BootstrapVerifyResult:
		result.Error = operr
	case *sdk.BootstrapEnsureResult:
		result.Error = operr
	case *sdk.StateStatusResult:
		result.Error = operr
	case *sdk.StateLockResult:
		result.Error = operr
	case *sdk.StateUnlockResult:
		result.Error = operr
	case *sdk.StateBackupResult:
		result.Error = operr
	case *sdk.StateRestoreResult:
		result.Error = operr
	case *sdk.SecretListResult:
		result.Error = operr
	case *sdk.SecretSetResult:
		result.Error = operr
	case *sdk.SecretRemoveResult:
		result.Error = operr
	case *sdk.SecretReadResult:
		result.Error = operr
	case *sdk.TailLogsResult:
		result.Error = operr
	case *sdk.CheckRuntimeResult:
		result.Error = operr
	case *sdk.PrepareExecResult:
		result.Error = operr
	case *sdk.PrepareTunnelResult:
		result.Error = operr
	case *sdk.CostInputsResult:
		result.Error = operr
	case *sdk.InventoryResult:
		result.Error = operr
	case *sdk.DeleteResult:
		result.Error = operr
	case *sdk.EdgePlanResult:
		result.Error = operr
	case *sdk.EdgeExecuteResult:
		result.Error = operr
	case *sdk.ResiliencePlanResult:
		result.Error = operr
	case *sdk.ResilienceExecuteResult:
		result.Error = operr
	}
}

func (s *RPCServer) Describe(req *sdk.DescribeRequest, resp *sdk.DescribeResponse) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpDescribe, version, resp, func(ctx context.Context) (*sdk.DescribeResponse, *sdk.OperationError) {
		return s.server().Describe(ctx, req)
	})
}

func (s *RPCServer) ValidateConfig(req *sdk.ValidateConfigRequest, resp *sdk.ValidateConfigResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpValidateConfig, version, resp, func(ctx context.Context) (*sdk.ValidateConfigResult, *sdk.OperationError) {
		return s.server().ValidateConfig(ctx, req)
	})
}

func (s *RPCServer) Plan(req *sdk.PlanRequest, resp *sdk.PlanResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpPlan, version, resp, func(ctx context.Context) (*sdk.PlanResult, *sdk.OperationError) {
		return s.server().Plan(ctx, req)
	})
}

func (s *RPCServer) Preview(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpPreview, version, resp, func(ctx context.Context) (*sdk.LifecycleResult, *sdk.OperationError) {
		return s.server().Preview(ctx, req)
	})
}

func (s *RPCServer) Apply(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpApply, version, resp, func(ctx context.Context) (*sdk.LifecycleResult, *sdk.OperationError) {
		return s.server().Apply(ctx, req)
	})
}

func (s *RPCServer) Outputs(req *sdk.StackCall, resp *sdk.OutputsResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpOutputs, version, resp, func(ctx context.Context) (*sdk.OutputsResult, *sdk.OperationError) {
		return s.server().Outputs(ctx, req)
	})
}

func (s *RPCServer) Destroy(req *sdk.StackCall, resp *sdk.LifecycleResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpDestroy, version, resp, func(ctx context.Context) (*sdk.LifecycleResult, *sdk.OperationError) {
		return s.server().Destroy(ctx, req)
	})
}

func (s *RPCServer) DestroyLeftoverBackups(req *sdk.DestroyLeftoverBackupsCall, resp *sdk.DestroyLeftoverBackupsResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpDestroyLeftoverBackups, version, resp, func(ctx context.Context) (*sdk.DestroyLeftoverBackupsResult, *sdk.OperationError) {
		return s.server().DestroyLeftoverBackups(ctx, req)
	})
}

func (s *RPCServer) BootstrapVerify(req *sdk.BootstrapVerifyCall, resp *sdk.BootstrapVerifyResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpBootstrapVerify, version, resp, func(ctx context.Context) (*sdk.BootstrapVerifyResult, *sdk.OperationError) {
		return s.server().BootstrapVerify(ctx, req)
	})
}

func (s *RPCServer) BootstrapEnsure(req *sdk.BootstrapEnsureCall, resp *sdk.BootstrapEnsureResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpBootstrapEnsure, version, resp, func(ctx context.Context) (*sdk.BootstrapEnsureResult, *sdk.OperationError) {
		return s.server().BootstrapEnsure(ctx, req)
	})
}

func (s *RPCServer) StateStatus(req *sdk.StateStatusCall, resp *sdk.StateStatusResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpStateStatus, version, resp, func(ctx context.Context) (*sdk.StateStatusResult, *sdk.OperationError) {
		return s.server().StateStatus(ctx, req)
	})
}

func (s *RPCServer) StateLock(req *sdk.StateLockCall, resp *sdk.StateLockResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpStateLock, version, resp, func(ctx context.Context) (*sdk.StateLockResult, *sdk.OperationError) {
		return s.server().StateLock(ctx, req)
	})
}

func (s *RPCServer) StateUnlock(req *sdk.StateUnlockCall, resp *sdk.StateUnlockResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpStateUnlock, version, resp, func(ctx context.Context) (*sdk.StateUnlockResult, *sdk.OperationError) {
		return s.server().StateUnlock(ctx, req)
	})
}

func (s *RPCServer) StateBackup(req *sdk.StateBackupCall, resp *sdk.StateBackupResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpStateBackup, version, resp, func(ctx context.Context) (*sdk.StateBackupResult, *sdk.OperationError) {
		return s.server().StateBackup(ctx, req)
	})
}

func (s *RPCServer) StateRestore(req *sdk.StateRestoreCall, resp *sdk.StateRestoreResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpStateRestore, version, resp, func(ctx context.Context) (*sdk.StateRestoreResult, *sdk.OperationError) {
		return s.server().StateRestore(ctx, req)
	})
}

func (s *RPCServer) SecretList(req *sdk.SecretListCall, resp *sdk.SecretListResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpSecretList, version, resp, func(ctx context.Context) (*sdk.SecretListResult, *sdk.OperationError) {
		return s.server().SecretList(ctx, req)
	})
}

func (s *RPCServer) SecretSet(req *sdk.SecretSetCall, resp *sdk.SecretSetResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpSecretSet, version, resp, func(ctx context.Context) (*sdk.SecretSetResult, *sdk.OperationError) {
		return s.server().SecretSet(ctx, req)
	})
}

func (s *RPCServer) SecretRemove(req *sdk.SecretRemoveCall, resp *sdk.SecretRemoveResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpSecretRemove, version, resp, func(ctx context.Context) (*sdk.SecretRemoveResult, *sdk.OperationError) {
		return s.server().SecretRemove(ctx, req)
	})
}

func (s *RPCServer) SecretRead(req *sdk.SecretReadRequest, resp *sdk.SecretReadResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpSecretRead, version, resp, func(ctx context.Context) (*sdk.SecretReadResult, *sdk.OperationError) {
		return s.server().SecretRead(ctx, req)
	})
}

func (s *RPCServer) TailLogs(req *sdk.TailLogsCall, resp *sdk.TailLogsResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpTailLogs, version, resp, func(ctx context.Context) (*sdk.TailLogsResult, *sdk.OperationError) {
		return s.server().TailLogs(ctx, req)
	})
}

func (s *RPCServer) CheckRuntime(req *sdk.CheckRuntimeCall, resp *sdk.CheckRuntimeResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpCheckRuntime, version, resp, func(ctx context.Context) (*sdk.CheckRuntimeResult, *sdk.OperationError) {
		return s.server().CheckRuntime(ctx, req)
	})
}

func (s *RPCServer) PrepareExec(req *sdk.PrepareExecCall, resp *sdk.PrepareExecResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpPrepareExec, version, resp, func(ctx context.Context) (*sdk.PrepareExecResult, *sdk.OperationError) {
		return s.server().PrepareExec(ctx, req)
	})
}

func (s *RPCServer) PrepareTunnel(req *sdk.PrepareTunnelCall, resp *sdk.PrepareTunnelResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpPrepareTunnel, version, resp, func(ctx context.Context) (*sdk.PrepareTunnelResult, *sdk.OperationError) {
		return s.server().PrepareTunnel(ctx, req)
	})
}

func (s *RPCServer) CostInputs(req *sdk.CostInputsRequest, resp *sdk.CostInputsResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpCostInputs, version, resp, func(ctx context.Context) (*sdk.CostInputsResult, *sdk.OperationError) {
		return s.server().CostInputs(ctx, req)
	})
}

func (s *RPCServer) Inventory(req *sdk.InventoryCall, resp *sdk.InventoryResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpInventory, version, resp, func(ctx context.Context) (*sdk.InventoryResult, *sdk.OperationError) {
		return s.server().Inventory(ctx, req)
	})
}

func (s *RPCServer) Delete(req *sdk.DeleteCall, resp *sdk.DeleteResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpDelete, version, resp, func(ctx context.Context) (*sdk.DeleteResult, *sdk.OperationError) {
		return s.server().Delete(ctx, req)
	})
}

func (s *RPCServer) EdgePlan(req *sdk.EdgePlanCall, resp *sdk.EdgePlanResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpEdgePlan, version, resp, func(ctx context.Context) (*sdk.EdgePlanResult, *sdk.OperationError) {
		return s.server().EdgePlan(ctx, req)
	})
}

func (s *RPCServer) EdgeExecute(req *sdk.EdgeExecuteCall, resp *sdk.EdgeExecuteResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpEdgeExecute, version, resp, func(ctx context.Context) (*sdk.EdgeExecuteResult, *sdk.OperationError) {
		return s.server().EdgeExecute(ctx, req)
	})
}

func (s *RPCServer) ResiliencePlan(req *sdk.ResiliencePlanCall, resp *sdk.ResiliencePlanResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpResiliencePlan, version, resp, func(ctx context.Context) (*sdk.ResiliencePlanResult, *sdk.OperationError) {
		return s.server().ResiliencePlan(ctx, req)
	})
}

func (s *RPCServer) ResilienceExecute(req *sdk.ResilienceExecuteCall, resp *sdk.ResilienceExecuteResult) error {
	version := ""
	if req != nil {
		version = req.ProtocolVersion
	}
	return dispatch(s, sdk.OpResilienceExecute, version, resp, func(ctx context.Context) (*sdk.ResilienceExecuteResult, *sdk.OperationError) {
		return s.server().ResilienceExecute(ctx, req)
	})
}
