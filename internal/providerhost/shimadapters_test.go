package providerhost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	"github.com/magelift/magelift/internal/automation"
	"github.com/magelift/magelift/sdk"
)

func TestShimEdgeAdapter(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		switch method {
		case "Plugin.EdgePlan":
			*(reply.(*sdk.EdgePlanResult)) = sdk.EdgePlanResult{PlanJSON: mustJSON(t, sdk.EdgePlan{AdapterID: "gcp.edge.native"})}
		case "Plugin.EdgeExecute":
			*(reply.(*sdk.EdgeExecuteResult)) = sdk.EdgeExecuteResult{ResultJSON: mustJSON(t, sdk.EdgeExecutionResult{OperationID: "op-1"})}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	})
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	adapter, err := NewShimEdgeAdapter(context.Background(), shim.module.client, planned)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.EdgeDescriptor().ID != "gcp.edge.native" {
		t.Fatalf("descriptor = %#v", adapter.EdgeDescriptor())
	}
	edge, err := module.LifecycleFactories.NewEdge(context.Background(), planned)
	if err != nil || edge == nil {
		t.Fatalf("factory edge = %v %v", edge, err)
	}
	plan, err := adapter.PlanEdge(context.Background(), sdk.EdgePlanRequest{TargetProvider: "gcp"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID != "gcp.edge.native" {
		t.Fatalf("plan = %#v", plan)
	}
	result, err := adapter.ExecuteEdge(context.Background(), sdk.EdgeExecutionRequest{Action: sdk.EdgePurge})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != "op-1" {
		t.Fatalf("result = %#v", result)
	}
	bare, err := NewShimEdgeAdapter(context.Background(), &Client{describe: &sdk.DescribeResponse{}}, planned)
	if err == nil || bare != nil {
		t.Fatalf("missing descriptor = %v %v", bare, err)
	}
}

func TestShimResilienceAdapter(t *testing.T) {
	t.Parallel()
	module, planned := scriptedModule(t, func(method string, reply any) error {
		switch method {
		case "Plugin.ResiliencePlan":
			*(reply.(*sdk.ResiliencePlanResult)) = sdk.ResiliencePlanResult{PlanJSON: mustJSON(t, sdk.ResiliencePlan{AdapterID: "gcp.resilience"})}
		case "Plugin.ResilienceExecute":
			*(reply.(*sdk.ResilienceExecuteResult)) = sdk.ResilienceExecuteResult{ResultJSON: mustJSON(t, sdk.ResilienceExecutionResult{OperationID: "op-2"})}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	})
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	adapter, err := NewShimResilienceAdapter(context.Background(), shim.module.client, planned)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.ResilienceDescriptor().ID != "gcp.resilience" {
		t.Fatalf("descriptor = %#v", adapter.ResilienceDescriptor())
	}
	resilience, err := module.LifecycleFactories.NewResilience(context.Background(), planned)
	if err != nil || resilience == nil {
		t.Fatalf("factory resilience = %v %v", resilience, err)
	}
	plan, err := adapter.PlanResilience(context.Background(), sdk.ResiliencePlanRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.AdapterID != "gcp.resilience" {
		t.Fatalf("plan = %#v", plan)
	}
	result, err := adapter.ExecuteResilience(context.Background(), sdk.ResilienceExecutionRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if result.OperationID != "op-2" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShimCleanupAndLeftovers(t *testing.T) {
	t.Parallel()
	client := &Client{caller: &stubCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Inventory":
			*(reply.(*sdk.InventoryResult)) = sdk.InventoryResult{ResourcesJSON: mustJSON(t, []sdk.CleanupInventoryResource{{Kind: "sql", Identity: "i-1"}})}
		case "Plugin.Delete":
			*(reply.(*sdk.DeleteResult)) = sdk.DeleteResult{Deleted: true}
		case "Plugin.DestroyLeftoverBackups":
			*(reply.(*sdk.DestroyLeftoverBackupsResult)) = sdk.DestroyLeftoverBackupsResult{Destroyed: []string{"b-1"}}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}}, describe: cannedDescribe()}
	resources, err := CleanupInventory(context.Background(), client, sdk.CleanupInventoryRequest{Marker: "m", Provider: "gcp", Project: "p"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].Identity != "i-1" {
		t.Fatalf("resources = %#v", resources)
	}
	if err := CleanupDelete(context.Background(), client, "p", "m", sdk.CleanupResource{Kind: "sql", Identity: "i-1"}); err != nil {
		t.Fatal(err)
	}
	_, planned := scriptedModule(t, func(string, any) error { return errors.New("unexpected") })
	shim, ok := AsShimPlanned(planned)
	if !ok {
		t.Fatalf("planned = %T", planned)
	}
	shim.module.client = client
	destroyed, err := DestroyLeftoverBackups(context.Background(), client, planned)
	if err != nil {
		t.Fatal(err)
	}
	if len(destroyed) != 1 || destroyed[0] != "b-1" {
		t.Fatalf("destroyed = %#v", destroyed)
	}
}

func TestShimSecretStore(t *testing.T) {
	t.Parallel()
	client := &Client{caller: &stubCaller{respond: func(method string, reply any) error {
		if method != "Plugin.SecretRead" {
			return errors.New("unexpected method " + method)
		}
		*(reply.(*sdk.SecretReadResult)) = sdk.SecretReadResult{Value: []byte("s3cret")}
		return nil
	}}, describe: cannedDescribe()}
	store, err := NewSecretStore(client, sdk.Envelope{Project: "shop"})
	if err != nil {
		t.Fatal(err)
	}
	value, err := store.GetSecretValue(context.Background(), "projects/p/secrets/s/versions/1")
	if err != nil {
		t.Fatal(err)
	}
	if string(value) != "s3cret" {
		t.Fatalf("value = %q", value)
	}
	if _, err := NewSecretStore(nil, sdk.Envelope{}); err == nil {
		t.Fatal("nil client was accepted")
	}
}

func TestPluginBackend(t *testing.T) {
	t.Parallel()
	client := &Client{caller: &stubCaller{respond: func(method string, reply any) error {
		switch method {
		case "Plugin.Preview":
			*(reply.(*sdk.LifecycleResult)) = sdk.LifecycleResult{Summary: sdk.ChangeSummary{Create: 2}, Diagnostics: []string{"previewing"}}
		case "Plugin.Apply":
			*(reply.(*sdk.LifecycleResult)) = sdk.LifecycleResult{Summary: sdk.ChangeSummary{Create: 2, Same: 5}}
		case "Plugin.Destroy":
			*(reply.(*sdk.LifecycleResult)) = sdk.LifecycleResult{Summary: sdk.ChangeSummary{Delete: 7}}
		case "Plugin.Outputs":
			*(reply.(*sdk.OutputsResult)) = sdk.OutputsResult{
				ValuesJSON: mustJSON(t, map[string]any{"clusterName": "c", "kubeconfig": "secret"}),
				SecretKeys: []string{"kubeconfig"},
			}
		default:
			return errors.New("unexpected method " + method)
		}
		return nil
	}}, describe: cannedDescribe()}
	backend, err := NewPluginBackend(client, sdk.Envelope{Project: "shop"}, cannedStoredPlan())
	if err != nil {
		t.Fatal(err)
	}
	var diagnostics bytes.Buffer
	preview, err := backend.Preview(context.Background(), automation.Request{}, &diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if preview["create"] != 2 || !bytes.Contains(diagnostics.Bytes(), []byte("previewing")) {
		t.Fatalf("preview = %#v %q", preview, diagnostics.String())
	}
	update, err := backend.Update(context.Background(), automation.Request{}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if update["create"] != 2 || update["same"] != 5 {
		t.Fatalf("update = %#v", update)
	}
	destroyed, err := backend.Destroy(context.Background(), automation.Request{Destroy: true}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if destroyed["delete"] != 7 {
		t.Fatalf("destroy = %#v", destroyed)
	}
	outputs, err := backend.Outputs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outputs["kubeconfig"] != "secret" {
		t.Fatalf("outputs = %#v", outputs)
	}
	redacted, err := backend.RedactedOutputs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	masked, ok := redacted["kubeconfig"].(map[string]any)
	if !ok || masked["secret"] != true || redacted["clusterName"] != "c" {
		t.Fatalf("redacted = %#v", redacted)
	}
	if _, err := NewPluginBackend(nil, sdk.Envelope{}, cannedStoredPlan()); err == nil {
		t.Fatal("nil client was accepted")
	}
	if _, err := NewPluginBackend(client, sdk.Envelope{}, sdk.StoredPlan{}); err == nil {
		t.Fatal("empty plan was accepted")
	}
}
