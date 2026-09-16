package providerhost

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/internal/platform"
	"github.com/magelift/magelift/sdk"
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func scriptedModule(t *testing.T, respond func(string, any) error) (*ShimModule, platform.PlannedStack) {
	t.Helper()
	planCaller := &stubCaller{respond: planResponder(cannedStoredPlan())}
	module := shimTestModule(t, planCaller)
	planned, err := module.Plan(shimTestConfig(), "staging", platform.PlanOptions{})
	if err != nil {
		t.Fatal(err)
	}
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	shim.module.client = &Client{caller: &stubCaller{respond: respond}, describe: cannedDescribe()}
	return module, planned
}

func TestShimBootstrap(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		switch method {
		case "Plugin.BootstrapVerify":
			*(reply.(*sdk.BootstrapVerifyResult)) = sdk.BootstrapVerifyResult{Verified: true}
		case "Plugin.BootstrapEnsure":
			*(reply.(*sdk.BootstrapEnsureResult)) = sdk.BootstrapEnsureResult{ResultJSON: mustJSON(t, platform.BootstrapResult{BackendURL: "gs://bucket"})}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	})
	if err := module.Bootstrap().VerifyAccount(context.Background(), planned); err != nil {
		t.Fatal(err)
	}
	result, err := module.Bootstrap().Ensure(context.Background(), planned, platform.BootstrapRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.BackendURL != "gs://bucket" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShimState(t *testing.T) {
	t.Parallel()
	now := time.Now().UTC().Truncate(time.Second)
	module, planned := scriptedModule(t, func(method string, reply any) error {
		switch method {
		case "Plugin.StateStatus":
			*(reply.(*sdk.StateStatusResult)) = sdk.StateStatusResult{Locked: true, BackendURL: "gs://bucket", InfoJSON: mustJSON(t, &platform.LockInfo{Owner: "tester", AcquiredAt: now})}
		case "Plugin.StateUnlock":
			*(reply.(*sdk.StateUnlockResult)) = sdk.StateUnlockResult{InfoJSON: mustJSON(t, &platform.LockInfo{Owner: "tester"})}
		case "Plugin.StateBackup":
			*(reply.(*sdk.StateBackupResult)) = sdk.StateBackupResult{ResultJSON: mustJSON(t, platform.BackupResult{ID: "b-1", Location: "gs://bucket/backups/b-1"})}
		case "Plugin.StateRestore":
			*(reply.(*sdk.StateRestoreResult)) = sdk.StateRestoreResult{ResultJSON: mustJSON(t, platform.RestoreResult{ID: "b-1"})}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	})
	locked, info, backend, err := module.State().Status(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if !locked || info == nil || info.Owner != "tester" || backend != "gs://bucket" {
		t.Fatalf("status = %v %#v %q", locked, info, backend)
	}
	released, err := module.State().Unlock(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if released == nil || released.Owner != "tester" {
		t.Fatalf("unlock = %#v", released)
	}
	backup, err := module.State().Backup(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if backup.ID != "b-1" {
		t.Fatalf("backup = %#v", backup)
	}
	restore, err := module.State().Restore(context.Background(), planned, "b-1")
	if err != nil {
		t.Fatal(err)
	}
	if restore.ID != "b-1" {
		t.Fatalf("restore = %#v", restore)
	}
}

func TestShimSecrets(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		switch method {
		case "Plugin.SecretList":
			*(reply.(*sdk.SecretListResult)) = sdk.SecretListResult{SecretsJSON: mustJSON(t, []platform.SecretMeta{{Name: "db"}})}
		case "Plugin.SecretSet":
			*(reply.(*sdk.SecretSetResult)) = sdk.SecretSetResult{Written: true}
		case "Plugin.SecretRemove":
			*(reply.(*sdk.SecretRemoveResult)) = sdk.SecretRemoveResult{Removed: true}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	})
	listed, err := module.Secrets().List(context.Background(), planned)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Name != "db" {
		t.Fatalf("listed = %#v", listed)
	}
	if err := module.Secrets().Set(context.Background(), planned, "db", []byte("v")); err != nil {
		t.Fatal(err)
	}
	if err := module.Secrets().Remove(context.Background(), planned, "db"); err != nil {
		t.Fatal(err)
	}
}

func TestShimObserve(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		switch method {
		case "Plugin.CheckRuntime":
			*(reply.(*sdk.CheckRuntimeResult)) = sdk.CheckRuntimeResult{HealthJSON: mustJSON(t, []platform.RuntimeHealth{{ID: "runtime.kube.deployment", Status: "healthy"}})}
		case "Plugin.PrepareExec":
			*(reply.(*sdk.PrepareExecResult)) = sdk.PrepareExecResult{TargetJSON: mustJSON(t, platform.ExecTarget{Launcher: "kubectl"})}
		case "Plugin.TailLogs":
			*(reply.(*sdk.TailLogsResult)) = sdk.TailLogsResult{
				EventsJSON:   mustJSON(t, []platform.LogEvent{{Message: "hi"}}),
				FailuresJSON: mustJSON(t, []string{"pod-x: log backend down"}),
			}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	})
	health, err := module.RuntimeObserve().CheckRuntime(context.Background(), planned, map[string]any{"serviceName": "web"})
	if err != nil {
		t.Fatal(err)
	}
	if len(health) != 1 || health[0].Status != "healthy" {
		t.Fatalf("health = %#v", health)
	}
	target, err := module.RuntimeObserve().PrepareExec(context.Background(), planned, map[string]any{}, platform.ExecQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if target.Launcher != "kubectl" {
		t.Fatalf("target = %#v", target)
	}
	observe, ok := module.RuntimeObserve().(*ShimObserve)
	if !ok {
		t.Fatalf("observe = %T", module.RuntimeObserve())
	}
	if err := observe.BindOutputs(map[string]any{"serviceName": "web"}); err != nil {
		t.Fatal(err)
	}
	events, err := observe.TailLogs(context.Background(), planned, platform.LogQuery{})
	if err == nil {
		t.Fatal("partial logs were not reported")
	}
	var partial *platform.PartialLogError
	if !errors.As(err, &partial) || len(partial.Failures) != 1 || partial.Failures[0].Source != "pod-x" {
		t.Fatalf("partial = %#v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
}

func TestShimTunnel(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		if method != "Plugin.PrepareTunnel" {
			return errors.New("unexpected method " + method)
		}
		*(reply.(*sdk.PrepareTunnelResult)) = sdk.PrepareTunnelResult{TargetJSON: mustJSON(t, platform.ExecTarget{Launcher: "cloud-sql-proxy"})}
		return nil
	})
	target, err := module.RuntimeTunnel().PrepareTunnel(context.Background(), planned, map[string]any{}, platform.TunnelQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if target.Launcher != "cloud-sql-proxy" {
		t.Fatalf("target = %#v", target)
	}
}

func TestShimCostEstimator(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		if method != "Plugin.CostInputs" {
			return errors.New("unexpected method " + method)
		}
		*(reply.(*sdk.CostInputsResult)) = sdk.CostInputsResult{ReportJSON: mustJSON(t, platform.CostReport{Mode: "account-free"})}
		return nil
	})
	report, err := module.CostEstimator().Estimate(context.Background(), planned, shimTestConfig(), platform.CostOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != "account-free" {
		t.Fatalf("report = %#v", report)
	}
}

func TestShimPortsRejectForeignPlans(t *testing.T) {
	t.Parallel()
	module := shimTestModule(t, &stubCaller{})
	foreign := fakeForeignPlanned{}
	if err := module.Bootstrap().VerifyAccount(context.Background(), foreign); err == nil {
		t.Fatal("foreign plan was accepted")
	}
	if _, _, _, err := module.State().Status(context.Background(), foreign); err == nil {
		t.Fatal("foreign plan was accepted")
	}
	if _, err := module.Secrets().List(context.Background(), foreign); err == nil {
		t.Fatal("foreign plan was accepted")
	}
	if _, err := module.RuntimeObserve().CheckRuntime(context.Background(), foreign, nil); err == nil {
		t.Fatal("foreign plan was accepted")
	}
	if _, err := module.RuntimeTunnel().PrepareTunnel(context.Background(), foreign, nil, platform.TunnelQuery{}); err == nil {
		t.Fatal("foreign plan was accepted")
	}
	if _, err := module.CostEstimator().Estimate(context.Background(), foreign, config.Config{}, platform.CostOptions{}); err == nil {
		t.Fatal("foreign plan was accepted")
	}
	if _, err := module.Ops().AcquireLock(context.Background(), foreign); err == nil {
		t.Fatal("foreign plan was accepted")
	}
}
