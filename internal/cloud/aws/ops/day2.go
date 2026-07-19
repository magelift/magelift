package ops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	awsbootstrap "github.com/acourtiol/magelift/internal/cloud/aws/bootstrap"
	awsoperations "github.com/acourtiol/magelift/internal/cloud/aws/operations"
	awssecrets "github.com/acourtiol/magelift/internal/cloud/aws/secrets"
	awsstack "github.com/acourtiol/magelift/internal/cloud/aws/stack"
	awsstate "github.com/acourtiol/magelift/internal/cloud/aws/state"
	"github.com/acourtiol/magelift/internal/platform"
)

func (Module) Bootstrap() platform.Bootstrap           { return Bootstrap{} }
func (Module) State() platform.State                   { return State{} }
func (Module) Secrets() platform.Secrets               { return Secrets{} }
func (Module) RuntimeObserve() platform.RuntimeObserve { return Observe{} }

// Bootstrap implements platform.Bootstrap for AWS.
type Bootstrap struct{}

func (Bootstrap) VerifyAccount(ctx context.Context, planned platform.PlannedStack) error {
	return awsbootstrap.VerifyAccount(ctx, planned.Region(), accountID(planned))
}

func (Bootstrap) Ensure(ctx context.Context, planned platform.PlannedStack, req platform.BootstrapRequest) (platform.BootstrapResult, error) {
	if strings.TrimSpace(req.AccessLogBucket) == "" {
		return platform.BootstrapResult{}, fmt.Errorf("--access-log-bucket is required")
	}
	if strings.TrimSpace(req.GitHubOwner) == "" || strings.TrimSpace(req.GitHubRepo) == "" {
		return platform.BootstrapResult{}, fmt.Errorf("--github-owner and --github-repo are required")
	}
	awsPlanned, ok := awsstack.AsAWSPlanned(planned)
	if !ok {
		return platform.BootstrapResult{}, fmt.Errorf("AWS bootstrap received unexpected planned type %T", planned)
	}
	spec := awsPlanned.AWSSpec()
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: req.AccessLogBucket,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	ensurer, err := awsbootstrap.NewAWS(ctx, spec.Identity.Region)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	result, err := ensurer.Ensure(ctx, plan, spec.Identity.Region)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	identityPlan, err := awsbootstrap.BuildIdentityPlan(awsbootstrap.IdentitySpec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		GitHubOwner: req.GitHubOwner, GitHubRepo: req.GitHubRepo,
		StateBucket: result.Plan.StateBucket, KMSKeyARN: result.KeyARN,
	})
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	identity, err := awsbootstrap.NewAWSIdentity(ctx, spec.Identity.Region)
	if err != nil {
		return platform.BootstrapResult{}, err
	}
	if err := identity.Ensure(ctx, identityPlan); err != nil {
		return platform.BootstrapResult{}, err
	}
	return platform.BootstrapResult{
		BackendURL: "s3://" + result.Plan.StateBucket,
		KeyRef:     result.KeyARN,
		Details: map[string]any{
			"state":    result,
			"identity": identityPlan,
		},
	}, nil
}

// State implements platform.State for AWS DIY locks.
type State struct{}

func (State) Status(ctx context.Context, planned platform.PlannedStack) (bool, *platform.LockInfo, string, error) {
	manager, bucket, err := stateManager(ctx, planned)
	if err != nil {
		return false, nil, "", err
	}
	info, err := manager.Status(ctx)
	if errors.Is(err, awsstate.ErrNotLocked) {
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
	if errors.Is(err, awsstate.ErrNotLocked) {
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

// Secrets implements platform.Secrets for AWS Secrets Manager.
type Secrets struct{}

func (Secrets) List(ctx context.Context, planned platform.PlannedStack) ([]platform.SecretMeta, error) {
	store, err := awssecrets.NewStore(ctx, planned.Region())
	if err != nil {
		return nil, err
	}
	listed, err := store.List(ctx)
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
	store, err := awssecrets.NewStore(ctx, planned.Region())
	if err != nil {
		return err
	}
	return store.Set(ctx, name, value)
}

func (Secrets) Remove(ctx context.Context, planned platform.PlannedStack, name string) error {
	store, err := awssecrets.NewStore(ctx, planned.Region())
	if err != nil {
		return err
	}
	return store.Remove(ctx, name)
}

// Observe implements platform.RuntimeObserve for ECS.
type Observe struct{}

func (Observe) TailLogs(ctx context.Context, planned platform.PlannedStack, query platform.LogQuery) ([]platform.LogEvent, error) {
	store, err := awsoperations.New(ctx, planned.Region())
	if err != nil {
		return nil, err
	}
	workload := string(query.Workload)
	if workload == "" {
		workload = "web"
	}
	group := fmt.Sprintf("/magelift/%s/%s/%s", planned.Project(), planned.Environment(), workload)
	limit := query.Limit
	if limit <= 0 {
		limit = platform.DefaultLogLimit
	}
	events, err := store.Tail(ctx, group, query.Since, query.Until, query.Filter, limit)
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
	cluster, _ := outputs["clusterName"].(string)
	service, _ := outputs["serviceName"].(string)
	if cluster == "" || service == "" {
		return nil, fmt.Errorf("clusterName and serviceName outputs are required for runtime health")
	}
	runtime, err := awsoperations.NewRuntime(ctx, planned.Region())
	if err != nil {
		return nil, err
	}
	health, err := runtime.Check(ctx, cluster, service)
	if err != nil {
		return nil, err
	}
	out := make([]platform.RuntimeHealth, 0, 3+len(health.Tasks))
	serviceStatus := "healthy"
	serviceDetail := fmt.Sprintf("ECS service has %d running of %d desired tasks", health.RunningCount, health.DesiredCount)
	if health.DesiredCount <= 0 || health.RunningCount < health.DesiredCount {
		serviceStatus = "unhealthy"
	}
	out = append(out, platform.RuntimeHealth{ID: "runtime.ecs.service", Service: service, Status: serviceStatus, Detail: serviceDetail})
	rolloutStatus, rolloutDetail := "healthy", "the primary ECS deployment rollout completed"
	if health.PrimaryRollout != "COMPLETED" {
		rolloutStatus, rolloutDetail = "unhealthy", "the primary ECS deployment rollout has not completed"
	}
	out = append(out, platform.RuntimeHealth{ID: "runtime.ecs.rollout", Service: service, Status: rolloutStatus, Detail: rolloutDetail})
	for _, task := range health.Tasks {
		status, detail := "healthy", "ECS task is running and healthy"
		// ECS reports UNKNOWN when the task definition has no container health
		// check; that is not a failure. Only UNHEALTHY (or non-RUNNING) is.
		switch {
		case task.LastStatus != "RUNNING" || task.HealthStatus == "UNHEALTHY":
			status = "unhealthy"
			detail = fmt.Sprintf("ECS task status is %s/%s", task.LastStatus, task.HealthStatus)
		case task.HealthStatus == "" || task.HealthStatus == "UNKNOWN":
			detail = "ECS task is running (no container health check configured)"
		}
		out = append(out, platform.RuntimeHealth{ID: "runtime.ecs.task." + task.ARN, Service: service, Status: status, Detail: detail})
	}
	return out, nil
}

func (Observe) PrepareExec(ctx context.Context, planned platform.PlannedStack, outputs map[string]any, query platform.ExecQuery) (platform.ExecTarget, error) {
	cluster, _ := outputs["clusterName"].(string)
	workload := string(query.Workload)
	if workload == "" {
		workload = "web"
	}
	serviceKey := "serviceName"
	switch workload {
	case "deploy":
		serviceKey = "deployServiceName"
	case "cron":
		serviceKey = "cronServiceName"
	}
	service, _ := outputs[serviceKey].(string)
	if cluster == "" || service == "" {
		return platform.ExecTarget{}, fmt.Errorf("stack outputs do not contain ECS %s runtime identifiers", workload)
	}
	execStore, err := awsoperations.NewExec(ctx, planned.Region())
	if err != nil {
		return platform.ExecTarget{}, err
	}
	task, err := execStore.SelectTask(ctx, cluster, service)
	if err != nil {
		return platform.ExecTarget{}, err
	}
	container := strings.TrimSpace(query.Container)
	if container == "" {
		container = "web"
		if awsPlanned, ok := awsstack.AsAWSPlanned(planned); ok && workload == "web" && awsPlanned.AWSSpec().Application.WebRuntime == "nginx-fpm" {
			container = "php-fpm"
		}
	}
	command := query.Command
	if len(command) == 0 {
		command = []string{"/bin/sh"}
	}
	args := []string{
		"ecs", "execute-command",
		"--cluster", cluster,
		"--task", task.ARN,
		"--container", container,
		"--command", strings.Join(command, " "),
		"--interactive",
	}
	return platform.ExecTarget{
		Launcher: "aws", Args: args,
		Cluster: cluster, Task: task.ARN, Container: container,
	}, nil
}

func accountID(planned platform.PlannedStack) string {
	if awsPlanned, ok := awsstack.AsAWSPlanned(planned); ok {
		return awsPlanned.AWSSpec().Identity.AccountID
	}
	return ""
}

func stateManager(ctx context.Context, planned platform.PlannedStack) (*awsstate.Manager, string, error) {
	awsPlanned, ok := awsstack.AsAWSPlanned(planned)
	if !ok {
		return nil, "", fmt.Errorf("AWS state received unexpected planned type %T", planned)
	}
	spec := awsPlanned.AWSSpec()
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return nil, "", err
	}
	manager, err := awsstate.NewAWS(ctx, spec.Identity.Region, plan.StateBucket, spec.Identity.Project, spec.Identity.Environment, spec.Dependencies.KMSKeyARN)
	if err != nil {
		return nil, "", err
	}
	return manager, plan.StateBucket, nil
}

func stateArchive(ctx context.Context, planned platform.PlannedStack) (*awsstate.Archive, error) {
	awsPlanned, ok := awsstack.AsAWSPlanned(planned)
	if !ok {
		return nil, fmt.Errorf("AWS state archive received unexpected planned type %T", planned)
	}
	spec := awsPlanned.AWSSpec()
	plan, err := awsbootstrap.BuildPlan(awsbootstrap.Spec{
		Project: spec.Identity.Project, Environment: spec.Identity.Environment,
		AccountID: spec.Identity.AccountID, Region: spec.Identity.Region,
		AccessLogBucket: "magelift-access-logs",
	})
	if err != nil {
		return nil, err
	}
	return awsstate.NewAWSArchive(ctx, spec.Identity.Region, plan.StateBucket, spec.Dependencies.KMSKeyARN)
}

func toLockInfo(info awsstate.Info) platform.LockInfo {
	return platform.LockInfo{
		Project: info.Project, Environment: info.Environment,
		Owner: info.Owner, AcquiredAt: info.AcquiredAt,
	}
}
