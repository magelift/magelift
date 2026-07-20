package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	gcpbootstrap "github.com/acourtiol/magelift/internal/cloud/gcp/bootstrap"
	gcpoperations "github.com/acourtiol/magelift/internal/cloud/gcp/operations"
	gcpsecrets "github.com/acourtiol/magelift/internal/cloud/gcp/secrets"
	gcpstack "github.com/acourtiol/magelift/internal/cloud/gcp/stack"
	gcpstate "github.com/acourtiol/magelift/internal/cloud/gcp/state"
	"github.com/acourtiol/magelift/internal/platform"
)

func (Module) Bootstrap() platform.Bootstrap           { return Bootstrap{} }
func (Module) State() platform.State                   { return State{} }
func (Module) Secrets() platform.Secrets               { return Secrets{} }
func (Module) RuntimeObserve() platform.RuntimeObserve { return Observe{} }

// Bootstrap implements platform.Bootstrap for GCS DIY (WIF deferred).
type Bootstrap struct{}

func (Bootstrap) VerifyAccount(ctx context.Context, planned platform.PlannedStack) error {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return fmt.Errorf("GCP bootstrap received unexpected planned type %T", planned)
	}
	return gcpbootstrap.VerifyAccount(ctx, gcpPlanned.GCPSpec().Identity.GCPProject)
}

func (Bootstrap) Ensure(ctx context.Context, planned platform.PlannedStack, _ platform.BootstrapRequest) (platform.BootstrapResult, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return platform.BootstrapResult{}, fmt.Errorf("GCP bootstrap received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	ensurer, err := gcpbootstrap.New(ctx)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	result, err := ensurer.Ensure(ctx, plan)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	return platform.BootstrapResult{
		BackendURL: gcpbootstrap.BackendURL(result.Plan),
		Details:    map[string]any{"state": result, "wif": "deferred"},
	}, nil
}

// State implements platform.State for GCS DIY locks.
type State struct{}

func (State) Status(ctx context.Context, planned platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := stateManager(ctx, planned)
	if err != nil {
		return false, nil, "", err
	}
	info, err := manager.Status(ctx)
	if errors.Is(err, gcpstate.ErrNotLocked) {
		return false, nil, bucket, nil
	}
	if err != nil {
		return false, nil, bucket, err
	}
	meta := toLockInfo(info)
	return true, &meta, bucket, nil
}

func (State) Lock(ctx context.Context, planned platform.PlannedStack, owner string) (func(context.Context) error, error) {
	manager, _, err := stateManager(ctx, planned)
	if err != nil {
		return nil, err
	}
	release, err := manager.Lock(ctx, planned.Project(), planned.Environment(), owner)
	if err != nil {
		return nil, err
	}
	return func(context.Context) error { return release() }, nil
}

func (State) Unlock(ctx context.Context, planned platform.PlannedStack) (*platform.LockInfo, error) {
	manager, _, err := stateManager(ctx, planned)
	if err != nil {
		return nil, err
	}
	info, err := manager.Unlock(ctx)
	if errors.Is(err, gcpstate.ErrNotLocked) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	meta := toLockInfo(info)
	return &meta, nil
}

func (State) Backup(ctx context.Context, planned platform.PlannedStack) (platform.BackupResult, error) {
	archive, err := stateArchive(ctx, planned)
	if err != nil {
		return platform.BackupResult{}, err
	}
	result, err := archive.Backup(ctx)
	if err != nil {
		return platform.BackupResult{}, err
	}
	return platform.BackupResult{ID: result.ID, Location: result.Prefix}, nil
}

func (State) Restore(ctx context.Context, planned platform.PlannedStack, location string) (platform.RestoreResult, error) {
	archive, err := stateArchive(ctx, planned)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	result, err := archive.Restore(ctx, location)
	if err != nil {
		return platform.RestoreResult{}, err
	}
	return platform.RestoreResult{ID: result.ID, Location: result.Prefix}, nil
}

// Secrets implements platform.Secrets for Secret Manager.
type Secrets struct{}

func (Secrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP secrets received unexpected planned type %T", planned)
	}
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return nil, err
	}
	listed, err := store.List(ctx, gcpPlanned.GCPSpec().Identity.GCPProject)
	if err != nil {
		return nil, err
	}
	out := make([]platform.SecretMeta, 0, len(listed))
	for _, item := range listed {
		out = append(out, platform.SecretMeta{Name: item.Name})
	}
	return out, nil
}

func (Secrets) Set(ctx context.Context, planned platform.PlannedStack, name string, value []byte) error {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return fmt.Errorf("GCP secrets received unexpected planned type %T", planned)
	}
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return err
	}
	return store.Set(ctx, gcpPlanned.GCPSpec().Identity.GCPProject, name, value)
}

func (Secrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return fmt.Errorf("GCP secrets received unexpected planned type %T", planned)
	}
	store, err := gcpsecrets.NewStore(ctx)
	if err != nil {
		return err
	}
	return store.Remove(ctx, gcpPlanned.GCPSpec().Identity.GCPProject, name)
}

// Observe implements platform.RuntimeObserve for GKE.
type Observe struct{}

func (Observe) TailLogs(ctx context.Context, planned platform.PlannedStack, query platform.LogQuery) ([]platform.LogEvent, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP observe received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	store, err := gcpoperations.NewObserve(ctx, spec.Identity.GCPProject, spec.Identity.Region)
	if err != nil {
		return nil, err
	}
	cluster := gcpstack.ClusterHint(spec)
	workload := string(query.Workload)
	if workload == "" {
		workload = planned.Project() + "-" + planned.Environment() + "-app-web"
	}
	limit := query.Limit
	if limit <= 0 {
		limit = platform.DefaultLogLimit
	}
	events, err := store.TailLogs(ctx, cluster, workload, query.Since, limit)
	if err != nil {
		return nil, err
	}
	out := make([]platform.LogEvent, 0, len(events))
	for _, event := range events {
		out = append(out, platform.LogEvent{Timestamp: event.Timestamp, Message: event.Message})
	}
	return out, nil
}

func (Observe) CheckRuntime(ctx context.Context, planned platform.PlannedStack, outputs map[string]any) ([]platform.RuntimeHealth, error) {
	cluster, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil {
		return nil, err
	}
	service, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return nil, err
	}
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP observe received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	runtime, err := gcpoperations.NewRuntime(ctx, spec.Identity.GCPProject, spec.Identity.Region)
	if err != nil {
		return nil, err
	}
	health, err := runtime.Check(ctx, cluster, service)
	if err != nil {
		return nil, err
	}
	status, detail := "healthy", fmt.Sprintf("GKE deployment has %d ready of %d desired replicas", health.ReadyReplicas, health.DesiredReplicas)
	if !health.Available || health.DesiredReplicas <= 0 || health.ReadyReplicas < health.DesiredReplicas {
		status = "unhealthy"
	}
	return []platform.RuntimeHealth{{
		ID: "runtime.gke.deployment", Service: service, Status: status, Detail: detail,
	}}, nil
}

func (Observe) PrepareExec(_ context.Context, planned platform.PlannedStack, outputs map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
	cluster, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	service, err := platform.RequireStringOutput(outputs, platform.OutputServiceName)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return platform.ExecTarget{}, fmt.Errorf("GCP observe received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	command := query.Command
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}
	selector := "app=" + service
	if query.Workload != "" && string(query.Workload) != "web" {
		selector = "app=" + strings.TrimSuffix(service, "-web") + "-" + string(query.Workload)
	}
	args := []string{
		"--context", fmt.Sprintf("gke_%s_%s_%s", spec.Identity.GCPProject, spec.Identity.Region, cluster),
		"exec", "-it", "deploy/" + service, "--",
	}
	if query.Workload != "" && string(query.Workload) != "web" {
		args[len(args)-2] = "deploy/" + strings.TrimSuffix(service, "-web") + "-" + string(query.Workload)
	}
	_ = selector
	args = append(args, command...)
	return platform.ExecTarget{
		Launcher: "gke-job",
		Args:     args,
		Cluster:  cluster,
		Task:     service,
	}, nil
}

func stateManager(ctx context.Context, planned platform.PlannedStack) (*gcpstate.Manager, string, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, "", fmt.Errorf("GCP state received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, "", err
	}
	manager, err := gcpstate.NewGCS(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
	if err != nil {
		return nil, "", err
	}
	return manager, plan.StateBucket, nil
}

func stateArchive(ctx context.Context, planned platform.PlannedStack) (*gcpstate.Archive, error) {
	gcpPlanned, ok := gcpstack.AsGCPPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("GCP state received unexpected planned type %T", planned)
	}
	spec := gcpPlanned.GCPSpec()
	plan, err := gcpbootstrap.BuildPlan(gcpbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		GCPProject: spec.Identity.GCPProject, Region: spec.Identity.Region,
	})
	if err != nil {
		return nil, err
	}
	return gcpstate.NewArchiveGCS(ctx, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment)
}

func toLockInfo(info gcpstate.Info) platform.LockInfo {
	return platform.LockInfo{
		Project: info.Project, Environment: info.Environment,
		Owner: info.Owner, AcquiredAt: info.AcquiredAt,
	}
}
