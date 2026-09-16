package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	billingbudgets "google.golang.org/api/billingbudgets/v1"
	cloudbilling "google.golang.org/api/cloudbilling/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/magelift/magelift/internal/platform"
	gcpcost "github.com/magelift/magelift/providers/gcp/cost"
	gcpresilience "github.com/magelift/magelift/providers/gcp/resilience"
	"github.com/magelift/magelift/sdk"
)

func ownedCloudSQLInstance(project, name, marker string) gcpresilience.CloudSQLInstance {
	return gcpresilience.CloudSQLInstance{
		Project: project, Name: name, Region: "europe-west1",
		UserLabels: map[string]string{"magelift_ownership": marker, "magelift_data_class": "database"},
	}
}

func budgetTestReader() gcpcost.BudgetReader {
	return gcpcost.NewBudgetReaderFromFuncs(
		func(context.Context, string) (*cloudbilling.ProjectBillingInfo, error) {
			return &cloudbilling.ProjectBillingInfo{ProjectId: "example-gcp-project", BillingEnabled: true, BillingAccountName: "billingAccounts/123456"}, nil
		},
		func(context.Context, string) (int64, error) { return 123, nil },
		func(context.Context, string, string) ([]*billingbudgets.GoogleCloudBillingBudgetsV1Budget, error) {
			return []*billingbudgets.GoogleCloudBillingBudgetsV1Budget{{
				Name:        "billingAccounts/123456/budgets/budget-1",
				DisplayName: "shop staging",
				BudgetFilter: &billingbudgets.GoogleCloudBillingBudgetsV1Filter{
					Projects:       []string{"projects/example-gcp-project"},
					CalendarPeriod: "MONTH",
				},
				Amount: &billingbudgets.GoogleCloudBillingBudgetsV1BudgetAmount{
					SpecifiedAmount: &billingbudgets.GoogleTypeMoney{Units: 500, CurrencyCode: "USD"},
				},
				ThresholdRules: []*billingbudgets.GoogleCloudBillingBudgetsV1ThresholdRule{
					{ThresholdPercent: 0.9, SpendBasis: "CURRENT_SPEND"},
				},
			}}, nil
		},
	)
}

func planCall(envelope sdk.Envelope, plan sdk.StoredPlan) (sdk.Envelope, sdk.StoredPlan) {
	envelope.StackName = plan.StackName
	return envelope, plan
}

func TestBootstrapVerify(t *testing.T) {
	t.Parallel()
	server := bootstrapServer(t, newMemBuckets())
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	result, operr := server.BootstrapVerify(context.Background(), &sdk.BootstrapVerifyCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if !result.Verified {
		t.Fatal("verified is false")
	}
}

func TestBootstrapEnsure(t *testing.T) {
	t.Parallel()
	server := bootstrapServer(t, newMemBuckets())
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	request, err := json.Marshal(platform.BootstrapRequest{})
	if err != nil {
		t.Fatal(err)
	}
	result, operr := server.BootstrapEnsure(context.Background(), &sdk.BootstrapEnsureCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: request})
	if operr != nil {
		t.Fatal(operr)
	}
	var decoded platform.BootstrapResult
	if err := json.Unmarshal(result.ResultJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(decoded.BackendURL, "gs://") {
		t.Fatalf("backend URL = %q", decoded.BackendURL)
	}
	if _, operr := server.BootstrapEnsure(context.Background(), &sdk.BootstrapEnsureCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, RequestJSON: []byte("{nope")}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("malformed request error = %v", operr)
	}
}

func TestStateLockCycle(t *testing.T) {
	t.Parallel()
	server := stateServer(t, newMemObjects())
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	status, operr := server.StateStatus(context.Background(), &sdk.StateStatusCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if status.Locked {
		t.Fatal("fresh state reports locked")
	}
	locked, operr := server.StateLock(context.Background(), &sdk.StateLockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, Owner: "tester"})
	if operr != nil {
		t.Fatal(operr)
	}
	if !locked.Locked {
		t.Fatal("lock acquisition not reported")
	}
	status, operr = server.StateStatus(context.Background(), &sdk.StateStatusCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if !status.Locked || len(status.InfoJSON) == 0 {
		t.Fatalf("locked status = %#v", status)
	}
	var info platform.LockInfo
	if err := json.Unmarshal(status.InfoJSON, &info); err != nil {
		t.Fatal(err)
	}
	if info.Owner != "tester" {
		t.Fatalf("lock owner = %q", info.Owner)
	}
	unlocked, operr := server.StateUnlock(context.Background(), &sdk.StateUnlockCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if len(unlocked.InfoJSON) == 0 {
		t.Fatal("unlock did not return the released lock")
	}
	status, operr = server.StateStatus(context.Background(), &sdk.StateStatusCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if status.Locked {
		t.Fatal("state reports locked after unlock")
	}
}

func TestStateBackupRestore(t *testing.T) {
	t.Parallel()
	objects := newMemObjects()
	if _, err := objects.Put(context.Background(), "shop-staging-state", "stacks/shop-staging.json", []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	server := stateServer(t, objects)
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	backup, operr := server.StateBackup(context.Background(), &sdk.StateBackupCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	var backupResult platform.BackupResult
	if err := json.Unmarshal(backup.ResultJSON, &backupResult); err != nil {
		t.Fatal(err)
	}
	if backupResult.Location == "" {
		t.Fatalf("backup result = %#v", backupResult)
	}
	restore, operr := server.StateRestore(context.Background(), &sdk.StateRestoreCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, Location: backupResult.ID})
	if operr != nil {
		t.Fatal(operr)
	}
	var restoreResult platform.RestoreResult
	if err := json.Unmarshal(restore.ResultJSON, &restoreResult); err != nil {
		t.Fatal(err)
	}
	if restoreResult.Location == "" {
		t.Fatalf("restore result = %#v", restoreResult)
	}
}

func TestSecretsRoundTrip(t *testing.T) {
	t.Parallel()
	server := secretsServer(t, newMemSecrets())
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	set, operr := server.SecretSet(context.Background(), &sdk.SecretSetCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, Name: "db-password", Value: []byte("s3cret")})
	if operr != nil {
		t.Fatal(operr)
	}
	if !set.Written {
		t.Fatal("write not reported")
	}
	listed, operr := server.SecretList(context.Background(), &sdk.SecretListCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	var metas []platform.SecretMeta
	if err := json.Unmarshal(listed.SecretsJSON, &metas); err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 {
		t.Fatalf("secrets = %#v", metas)
	}
	read, operr := server.SecretRead(context.Background(), &sdk.SecretReadRequest{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Name: "projects/example-gcp-project/secrets/db-password/versions/latest"})
	if operr != nil {
		t.Fatal(operr)
	}
	if string(read.Value) != "s3cret" {
		t.Fatalf("value = %q", read.Value)
	}
	removed, operr := server.SecretRemove(context.Background(), &sdk.SecretRemoveCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, Name: "db-password"})
	if operr != nil {
		t.Fatal(operr)
	}
	if !removed.Removed {
		t.Fatal("removal not reported")
	}
}

func TestCheckRuntime(t *testing.T) {
	t.Parallel()
	replicas := int32(2)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2, Conditions: []appsv1.DeploymentCondition{
			{Type: appsv1.DeploymentAvailable, Status: corev1.ConditionTrue},
		}},
	}
	server := &Server{KubeClients: fakeKubeFactory(k8sfake.NewClientset(deployment))}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	outputs, err := json.Marshal(map[string]any{"serviceName": "shop-web"})
	if err != nil {
		t.Fatal(err)
	}
	result, operr := server.CheckRuntime(context.Background(), &sdk.CheckRuntimeCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: outputs})
	if operr != nil {
		t.Fatal(operr)
	}
	var health []platform.RuntimeHealth
	if err := json.Unmarshal(result.HealthJSON, &health); err != nil {
		t.Fatal(err)
	}
	if len(health) != 1 || health[0].Status != "healthy" {
		t.Fatalf("health = %#v", health)
	}
}

func TestPrepareExec(t *testing.T) {
	t.Parallel()
	server := &Server{}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	outputs, err := json.Marshal(map[string]any{
		"clusterName": "shop-staging-gke", "serviceName": "shop-web", "kubeconfig": "fake-config",
	})
	if err != nil {
		t.Fatal(err)
	}
	query, err := json.Marshal(platform.ExecQuery{Command: []string{"bin/magento", "cache:flush"}})
	if err != nil {
		t.Fatal(err)
	}
	result, operr := server.PrepareExec(context.Background(), &sdk.PrepareExecCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: query})
	if operr != nil {
		t.Fatal(operr)
	}
	var target platform.ExecTarget
	if err := json.Unmarshal(result.TargetJSON, &target); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(target.Args, " ")
	if !strings.Contains(joined, "exec") || !strings.Contains(joined, "cache:flush") {
		t.Fatalf("target = %#v", target)
	}
}

func TestTailLogs(t *testing.T) {
	t.Parallel()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "shop-web-abc", Namespace: "default", Labels: map[string]string{"app": "shop-web"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "web"}}},
	}
	client := k8sfake.NewClientset(pod)
	client.PrependReactor("get", "pods/log", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, &runtime.Unknown{Raw: []byte("2026-09-16T10:00:00.000000Z INFO cache flushed\n")}, nil
	})
	server := &Server{KubeClients: fakeKubeFactory(client)}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	outputs, err := json.Marshal(map[string]any{"serviceName": "shop-web"})
	if err != nil {
		t.Fatal(err)
	}
	query, err := json.Marshal(platform.LogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	result, operr := server.TailLogs(context.Background(), &sdk.TailLogsCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: query})
	if operr != nil {
		t.Fatal(operr)
	}
	var events []platform.LogEvent
	if err := json.Unmarshal(result.EventsJSON, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !strings.Contains(events[0].Message, "cache flushed") || events[0].Source != "shop-web-abc" {
		t.Fatalf("events = %#v", events)
	}
	if len(result.FailuresJSON) != 0 {
		t.Fatalf("failures = %s", result.FailuresJSON)
	}
}

func TestTailLogsPartial(t *testing.T) {
	t.Parallel()
	pods := []runtime.Object{
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "shop-web-good", Namespace: "default", Labels: map[string]string{"app": "shop-web"}}},
		&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "shop-web-bad", Namespace: "default", Labels: map[string]string{"app": "shop-web"}}},
	}
	client := k8sfake.NewClientset(pods...)
	call := 0
	client.PrependReactor("get", "pods/log", func(ktesting.Action) (bool, runtime.Object, error) {
		call++
		if call == 1 {
			return true, &runtime.Unknown{Raw: []byte("good log")}, nil
		}
		return true, nil, errors.New("log backend down")
	})
	server := &Server{KubeClients: fakeKubeFactory(client)}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	outputs, err := json.Marshal(map[string]any{"serviceName": "shop-web"})
	if err != nil {
		t.Fatal(err)
	}
	query, err := json.Marshal(platform.LogQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	result, operr := server.TailLogs(context.Background(), &sdk.TailLogsCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: query})
	if operr != nil {
		t.Fatal(operr)
	}
	var failures []string
	if err := json.Unmarshal(result.FailuresJSON, &failures); err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 || !strings.Contains(failures[0], "log backend down") {
		t.Fatalf("failures = %#v", failures)
	}
	var events []platform.LogEvent
	if err := json.Unmarshal(result.EventsJSON, &events); err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
}

func TestPrepareTunnelDatabase(t *testing.T) {
	t.Parallel()
	server := &Server{}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	outputs, err := json.Marshal(map[string]any{"databaseConnectionName": "example-project:europe-west1:shop-staging-sql"})
	if err != nil {
		t.Fatal(err)
	}
	query, err := json.Marshal(platform.TunnelQuery{Target: platform.TunnelTargetDatabase, LocalPort: 13306, RemotePort: 3306})
	if err != nil {
		t.Fatal(err)
	}
	result, operr := server.PrepareTunnel(context.Background(), &sdk.PrepareTunnelCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, OutputsJSON: outputs, QueryJSON: query})
	if operr != nil {
		t.Fatal(operr)
	}
	var target platform.ExecTarget
	if err := json.Unmarshal(result.TargetJSON, &target); err != nil {
		t.Fatal(err)
	}
	if target.Launcher != "cloud-sql-proxy" {
		t.Fatalf("launcher = %q", target.Launcher)
	}
	ui, err := json.Marshal(platform.TunnelQuery{Target: platform.TunnelTargetDatabaseUI, LocalPort: 18080, RemotePort: 8080})
	if err != nil {
		t.Fatal(err)
	}
	if _, operr := server.PrepareTunnel(context.Background(), &sdk.PrepareTunnelCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan, QueryJSON: ui}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("database UI error = %v", operr)
	}
}

func TestCostInputs(t *testing.T) {
	t.Parallel()
	server := &Server{}
	result, operr := server.CostInputs(context.Background(), &sdk.CostInputsRequest{
		ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), TargetBlock: testTargetBlock(), Runtime: "gke-autopilot",
	})
	if operr != nil {
		t.Fatal(operr)
	}
	var report platform.CostReport
	if err := json.Unmarshal(result.ReportJSON, &report); err != nil {
		t.Fatal(err)
	}
	if report.Mode != "account-free" || len(report.Estimated) == 0 {
		t.Fatalf("report = %#v", report)
	}
	if _, operr := server.CostInputs(context.Background(), &sdk.CostInputsRequest{
		ProtocolVersion: sdk.ProtocolV1, Envelope: testEnvelope(), TargetBlock: []byte(":\t:"),
	}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("malformed block error = %v", operr)
	}
}

func TestCostInputsBudget(t *testing.T) {
	t.Parallel()
	reader := budgetTestReader()
	server := costServer(reader)
	envelope := testEnvelope()
	envelope.MonthlyBudgetCents = 50000
	result, operr := server.CostInputs(context.Background(), &sdk.CostInputsRequest{
		ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, TargetBlock: testTargetBlock(), Runtime: "gke-autopilot", Budget: true,
	})
	if operr != nil {
		t.Fatal(operr)
	}
	var report platform.CostReport
	if err := json.Unmarshal(result.ReportJSON, &report); err != nil {
		t.Fatal(err)
	}
	if report.Budget == nil || len(report.Budget.Budgets) != 1 {
		t.Fatalf("budget = %#v", report.Budget)
	}
}

func TestInventoryDelete(t *testing.T) {
	t.Parallel()
	sql := newStubSQL()
	sql.instances["shop-staging-sql"] = ownedCloudSQLInstance("example-gcp-project", "shop-staging-sql", "magelift/test")
	server := &Server{NewCleanupSQL: func(context.Context) (gcpresilience.CloudSQLAPI, error) { return sql, nil }}
	request, err := json.Marshal(sdk.CleanupInventoryRequest{Marker: "magelift/test", Provider: "gcp", Project: "example-gcp-project"})
	if err != nil {
		t.Fatal(err)
	}
	inventoried, operr := server.Inventory(context.Background(), &sdk.InventoryCall{ProtocolVersion: sdk.ProtocolV1, RequestJSON: request})
	if operr != nil {
		t.Fatal(operr)
	}
	var resources []sdk.CleanupInventoryResource
	if err := json.Unmarshal(inventoried.ResourcesJSON, &resources); err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].Identity == "" {
		t.Fatalf("resources = %#v", resources)
	}
	resource, err := json.Marshal(sdk.CleanupResource{Kind: resources[0].Kind, Role: resources[0].Role, Name: resources[0].Name, Identity: resources[0].Identity})
	if err != nil {
		t.Fatal(err)
	}
	deleted, operr := server.Delete(context.Background(), &sdk.DeleteCall{ProtocolVersion: sdk.ProtocolV1, Project: "example-gcp-project", Marker: "magelift/test", ResourceJSON: resource})
	if operr != nil {
		t.Fatal(operr)
	}
	if !deleted.Deleted {
		t.Fatal("delete not reported")
	}
	if len(sql.deleted) != 1 {
		t.Fatalf("deleted = %#v", sql.deleted)
	}
	if _, operr := server.Delete(context.Background(), &sdk.DeleteCall{ProtocolVersion: sdk.ProtocolV1, ResourceJSON: resource}); operr == nil || operr.Code != sdk.ErrCodeInvalid {
		t.Fatalf("missing project error = %v", operr)
	}
}

func TestDestroyLeftoverBackups(t *testing.T) {
	t.Parallel()
	sql := newStubSQL()
	sql.backups = []gcpresilience.CloudSQLBackup{
		{Name: "backup-1", Instance: "shop-staging-sql"},
		{Name: "backup-other", Instance: "other-instance"},
	}
	server := &Server{NewCleanupSQL: func(context.Context) (gcpresilience.CloudSQLAPI, error) { return sql, nil }}
	envelope, plan := planCall(testEnvelope(), storedTestPlan(t, testSpec()))
	result, operr := server.DestroyLeftoverBackups(context.Background(), &sdk.DestroyLeftoverBackupsCall{ProtocolVersion: sdk.ProtocolV1, Envelope: envelope, Plan: plan})
	if operr != nil {
		t.Fatal(operr)
	}
	if len(result.Destroyed) != 1 || result.Destroyed[0] != "backup-1" {
		t.Fatalf("destroyed = %#v", result.Destroyed)
	}
	if len(sql.deleted) != 1 || sql.deleted[0] != "backup-1" {
		t.Fatalf("sql deleted = %#v", sql.deleted)
	}
}
