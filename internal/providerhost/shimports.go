package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
	"gopkg.in/yaml.v3"
)

func shimPlanOf(planned platform.PlannedStack) (*ShimPlanned, error) {
	shim, ok := AsShimPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP shim received unexpected planned type %T", planned)
	}
	if shim.module == nil || shim.module.client == nil {
		return nil, errors.New("provider client is required")
	}
	return shim, nil
}

func shimClientOf(shim *ShimPlanned) *Client {
	if shim == nil || shim.module == nil {
		return nil
	}
	return shim.module.client
}

// ShimBootstrap forwards platform.Bootstrap over the v2 client.
type ShimBootstrap struct {
	client *Client
}

var _ platform.Bootstrap = ShimBootstrap{}

func (b ShimBootstrap) VerifyAccount(ctx context.Context, planned platform.PlannedStack) error {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return err
	}
	resp, err := Call[sdk.BootstrapVerifyCall, sdk.BootstrapVerifyResult](ctx, shimClientOf(shim), sdk.OpBootstrapVerify, &sdk.BootstrapVerifyCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return AsPluginError(sdk.OpBootstrapVerify, resp.Error)
	}
	return nil
}

func (b ShimBootstrap) Ensure(ctx context.Context, planned platform.PlannedStack, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	resp, err := Call[sdk.BootstrapEnsureCall, sdk.BootstrapEnsureResult](ctx, shimClientOf(shim), sdk.OpBootstrapEnsure, &sdk.BootstrapEnsureCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, RequestJSON: encoded,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	if resp.Error != nil {
		return platform.BootstrapResult{}, AsPluginError(sdk.OpBootstrapEnsure, resp.Error)
	}
	var result platform.BootstrapResult
	if err := json.Unmarshal(resp.ResultJSON, &result); err != nil {
		return platform.BootstrapResult{}, fmt.Errorf("decode bootstrap result: %w", err)
	}
	return result, nil
}

// ShimState forwards platform.State over the v2 client.
type ShimState struct {
	client *Client
}

var _ platform.State = ShimState{}

func (s ShimState) Status(ctx context.Context, planned platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return false, nil, "", err
	}
	resp, err := Call[sdk.StateStatusCall, sdk.StateStatusResult](ctx, shimClientOf(shim), sdk.OpStateStatus, &sdk.StateStatusCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
	})
	if err != nil {
		return false, nil, "", err
	}
	if resp.Error != nil {
		return false, nil, "", AsPluginError(sdk.OpStateStatus, resp.Error)
	}
	if len(resp.InfoJSON) == 0 {
		return resp.Locked, nil, resp.BackendURL, nil
	}
	var info platform.LockInfo
	if err := json.Unmarshal(resp.InfoJSON, &info); err != nil {
		return false, nil, "", fmt.Errorf("decode lock info: %w", err)
	}
	return resp.Locked, &info, resp.BackendURL, nil
}

func (s ShimState) Lock(ctx context.Context, planned platform.PlannedStack, owner string) (func(context.Context) error, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.StateLockCall, sdk.StateLockResult](ctx, shimClientOf(shim), sdk.OpStateLock, &sdk.StateLockCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, Owner: owner,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpStateLock, resp.Error)
	}
	return func(releaseCtx context.Context) error {
		_, err := s.Unlock(releaseCtx, planned)
		return err
	}, nil
}

func (s ShimState) Unlock(ctx context.Context, planned platform.PlannedStack) (*platform.LockInfo, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.StateUnlockCall, sdk.StateUnlockResult](ctx, shimClientOf(shim), sdk.OpStateUnlock, &sdk.StateUnlockCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpStateUnlock, resp.Error)
	}
	if len(resp.InfoJSON) == 0 {
		return nil, nil
	}
	var info platform.LockInfo
	if err := json.Unmarshal(resp.InfoJSON, &info); err != nil {
		return nil, fmt.Errorf("decode lock info: %w", err)
	}
	return &info, nil
}

func (s ShimState) Backup(ctx context.Context, planned platform.PlannedStack) (platform.BackupResult, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return platform.BackupResult{}, err
	}
	resp, err := Call[sdk.StateBackupCall, sdk.StateBackupResult](ctx, shimClientOf(shim), sdk.OpStateBackup, &sdk.StateBackupCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
	})
	if err != nil {
		return platform.BackupResult{}, err
	}
	if resp.Error != nil {
		return platform.BackupResult{}, AsPluginError(sdk.OpStateBackup, resp.Error)
	}
	var result platform.BackupResult
	if err := json.Unmarshal(resp.ResultJSON, &result); err != nil {
		return platform.BackupResult{}, fmt.Errorf("decode backup result: %w", err)
	}
	return result, nil
}

func (s ShimState) Restore(ctx context.Context, planned platform.PlannedStack, location string) (platform.RestoreResult, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	resp, err := Call[sdk.StateRestoreCall, sdk.StateRestoreResult](ctx, shimClientOf(shim), sdk.OpStateRestore, &sdk.StateRestoreCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, Location: location,
	})
	if err != nil {
		return platform.RestoreResult{}, err
	}
	if resp.Error != nil {
		return platform.RestoreResult{}, AsPluginError(sdk.OpStateRestore, resp.Error)
	}
	var result platform.RestoreResult
	if err := json.Unmarshal(resp.ResultJSON, &result); err != nil {
		return platform.RestoreResult{}, fmt.Errorf("decode restore result: %w", err)
	}
	return result, nil
}

// ShimSecrets forwards platform.Secrets over the v2 client.
type ShimSecrets struct {
	client *Client
}

var _ platform.Secrets = ShimSecrets{}

func (s ShimSecrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.SecretListCall, sdk.SecretListResult](ctx, shimClientOf(shim), sdk.OpSecretList, &sdk.SecretListCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpSecretList, resp.Error)
	}
	var listed []platform.SecretMeta
	if err := json.Unmarshal(resp.SecretsJSON, &listed); err != nil {
		return nil, fmt.Errorf("decode secret list: %w", err)
	}
	return listed, nil
}

func (s ShimSecrets) Set(ctx context.Context, planned platform.PlannedStack, name string, value []byte) error {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return err
	}
	resp, err := Call[sdk.SecretSetCall, sdk.SecretSetResult](ctx, shimClientOf(shim), sdk.OpSecretSet, &sdk.SecretSetCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, Name: name, Value: value,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return AsPluginError(sdk.OpSecretSet, resp.Error)
	}
	return nil
}

func (s ShimSecrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return err
	}
	resp, err := Call[sdk.SecretRemoveCall, sdk.SecretRemoveResult](ctx, shimClientOf(shim), sdk.OpSecretRemove, &sdk.SecretRemoveCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, Name: name,
	})
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return AsPluginError(sdk.OpSecretRemove, resp.Error)
	}
	return nil
}

// ShimObserve forwards platform.RuntimeObserve over the v2 client. Outputs
// bind explicitly (the CLI logs flow binds before TailLogs, mirroring the
// kube observer contract).
type ShimObserve struct {
	client  *Client
	outputs map[string]any
}

var _ platform.RuntimeObserve = (*ShimObserve)(nil)

// BindOutputs stores stack outputs for TailLogs.
func (o *ShimObserve) BindOutputs(outputs map[string]any) error {
	if o == nil {
		return errors.New("shim observer is nil")
	}
	o.outputs = outputs
	return nil
}

func (o *ShimObserve) TailLogs(ctx context.Context, planned platform.PlannedStack, query platform.LogQuery) ([]platform.LogEvent, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	encodedQuery, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	encodedOutputs, err := json.Marshal(o.outputsOrEmpty())
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.TailLogsCall, sdk.TailLogsResult](ctx, shimClientOf(shim), sdk.OpTailLogs, &sdk.TailLogsCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, OutputsJSON: encodedOutputs, QueryJSON: encodedQuery,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpTailLogs, resp.Error)
	}
	var events []platform.LogEvent
	if err := json.Unmarshal(resp.EventsJSON, &events); err != nil {
		return nil, fmt.Errorf("decode log events: %w", err)
	}
	if len(resp.FailuresJSON) > 0 {
		var failures []string
		if err := json.Unmarshal(resp.FailuresJSON, &failures); err != nil {
			return nil, fmt.Errorf("decode log failures: %w", err)
		}
		if len(failures) > 0 {
			return events, &platform.PartialLogError{Failures: partialFailures(failures)}
		}
	}
	return events, nil
}

func partialFailures(formatted []string) []platform.LogReadFailure {
	failures := make([]platform.LogReadFailure, 0, len(formatted))
	for _, text := range formatted {
		source, reason, _ := strings.Cut(text, ": ")
		failure := platform.LogReadFailure{Source: source}
		if strings.TrimSpace(reason) != "" {
			failure.Err = errors.New(reason)
		}
		failures = append(failures, failure)
	}
	return failures
}

func (o *ShimObserve) outputsOrEmpty() map[string]any {
	if o == nil || o.outputs == nil {
		return map[string]any{}
	}
	return o.outputs
}

func (o *ShimObserve) CheckRuntime(ctx context.Context, planned platform.PlannedStack, outputs map[string]any) ([]platform.RuntimeHealth, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(outputs)
	if err != nil {
		return nil, err
	}
	resp, err := Call[sdk.CheckRuntimeCall, sdk.CheckRuntimeResult](ctx, shimClientOf(shim), sdk.OpCheckRuntime, &sdk.CheckRuntimeCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, OutputsJSON: encoded,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, AsPluginError(sdk.OpCheckRuntime, resp.Error)
	}
	var health []platform.RuntimeHealth
	if err := json.Unmarshal(resp.HealthJSON, &health); err != nil {
		return nil, fmt.Errorf("decode runtime health: %w", err)
	}
	return health, nil
}

func (o *ShimObserve) PrepareExec(ctx context.Context, planned platform.PlannedStack, outputs map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	encodedOutputs, err := json.Marshal(outputs)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	encodedQuery, err := json.Marshal(query)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	resp, err := Call[sdk.PrepareExecCall, sdk.PrepareExecResult](ctx, shimClientOf(shim), sdk.OpPrepareExec, &sdk.PrepareExecCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, OutputsJSON: encodedOutputs, QueryJSON: encodedQuery,
	})
	if err != nil {
		return platform.ExecTarget{}, err
	}
	if resp.Error != nil {
		return platform.ExecTarget{}, AsPluginError(sdk.OpPrepareExec, resp.Error)
	}
	var target platform.ExecTarget
	if err := json.Unmarshal(resp.TargetJSON, &target); err != nil {
		return platform.ExecTarget{}, fmt.Errorf("decode exec target: %w", err)
	}
	return target, nil
}

// ShimTunnel forwards platform.RuntimeTunnel over the v2 client.
type ShimTunnel struct {
	client *Client
}

var _ platform.RuntimeTunnel = ShimTunnel{}

func (t ShimTunnel) PrepareTunnel(ctx context.Context, planned platform.PlannedStack, outputs map[string]any, query platform.TunnelQuery) (platform.ExecTarget, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	encodedOutputs, err := json.Marshal(outputs)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	encodedQuery, err := json.Marshal(query)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	resp, err := Call[sdk.PrepareTunnelCall, sdk.PrepareTunnelResult](ctx, shimClientOf(shim), sdk.OpPrepareTunnel, &sdk.PrepareTunnelCall{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, Plan: shim.stored, OutputsJSON: encodedOutputs, QueryJSON: encodedQuery,
	})
	if err != nil {
		return platform.ExecTarget{}, err
	}
	if resp.Error != nil {
		return platform.ExecTarget{}, AsPluginError(sdk.OpPrepareTunnel, resp.Error)
	}
	var target platform.ExecTarget
	if err := json.Unmarshal(resp.TargetJSON, &target); err != nil {
		return platform.ExecTarget{}, fmt.Errorf("decode tunnel target: %w", err)
	}
	return target, nil
}

// ShimCostEstimator forwards platform.CostEstimator over the v2 client.
type ShimCostEstimator struct {
	client *Client
}

var _ platform.CostEstimator = ShimCostEstimator{}

func (e ShimCostEstimator) Estimate(ctx context.Context, planned platform.PlannedStack, cfg config.Config, opts platform.CostOptions) (platform.CostReport, error) {
	shim, err := shimPlanOf(planned)
	if err != nil {
		return platform.CostReport{}, err
	}
	var targetBlock []byte
	if cfg.Target.GCP != nil {
		targetBlock, err = yaml.Marshal(cfg.Target.GCP)
		if err != nil {
			return platform.CostReport{}, fmt.Errorf("marshal GCP target block: %w", err)
		}
	}
	resp, err := Call[sdk.CostInputsRequest, sdk.CostInputsResult](ctx, shimClientOf(shim), sdk.OpCostInputs, &sdk.CostInputsRequest{
		ProtocolVersion: sdk.ProtocolV1, Envelope: shim.inputs.envelope, TargetBlock: targetBlock,
		Runtime: string(shim.Runtime()), Live: opts.Live, Budget: opts.Budget,
	})
	if err != nil {
		return platform.CostReport{}, err
	}
	if resp.Error != nil {
		return platform.CostReport{}, AsPluginError(sdk.OpCostInputs, resp.Error)
	}
	var report platform.CostReport
	if err := json.Unmarshal(resp.ReportJSON, &report); err != nil {
		return platform.CostReport{}, fmt.Errorf("decode cost report: %w", err)
	}
	return report, nil
}
