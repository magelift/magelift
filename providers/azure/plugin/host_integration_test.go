package plugin_test

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/magelift/magelift/internal/providerhost"
	"github.com/magelift/magelift/sdk"
)

func TestAzureProviderNegotiatesImplementedOperations(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "magelift-provider-azure")
	cmd := exec.Command("go", "build", "-o", binary, "../cmd/magelift-provider-azure")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build Azure provider: %v\n%s", err, output)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	client, err := providerhost.Dial(ctx, binary, providerhost.DialOptions{Required: []sdk.Operation{sdk.OpDescribe, sdk.OpValidateConfig, sdk.OpPlan, sdk.OpPreview}})
	if err != nil {
		t.Fatalf("dial Azure provider: %v", err)
	}
	defer client.Close()
	if client.ProviderID() != "azure" {
		t.Fatalf("provider ID = %q", client.ProviderID())
	}
	result, err := providerhost.Call[sdk.ValidateConfigRequest, sdk.ValidateConfigResult](ctx, client, sdk.OpValidateConfig, &sdk.ValidateConfigRequest{
		ProtocolVersion: sdk.ProtocolV1, Runtime: "aks", TargetBlock: []byte("subscriptionId: example\nresourceGroup: shop\n"),
	})
	if err != nil || !result.Valid || result.Error != nil {
		t.Fatalf("validate = %#v, %v", result, err)
	}
	request := &sdk.PlanRequest{ProtocolVersion: sdk.ProtocolV1, Runtime: "aks", Envelope: sdk.Envelope{Project: "shop", Environment: "preview", EnvironmentClass: "preview", Region: "francecentral", StackName: "shop-preview-azure-aks"},
		Application: sdk.Application{Edition: "open-source", Version: "2.4.9", Mode: "headless", WebRuntime: "nginx-fpm"},
		TargetBlock: []byte("subscriptionId: sub\nresourceGroup: shop\ntenantId: tenant\nimageDigest: registry.example/shop@sha256:" + strings.Repeat("a", 64) + "\n"),
		Services:    map[string]sdk.ServiceBinding{"database": {Family: "mysql", Ownership: "managed", Version: "8.0.21"}, "cache": {Family: "valkey", Ownership: "self-hosted", Version: "8.1"}, "search": {Family: "opensearch", Ownership: "self-hosted", Version: "2.19"}, "queue": {Family: "database"}}}
	projected, err := providerhost.Call[sdk.PlanRequest, sdk.PlanResult](ctx, client, sdk.OpPlan, request)
	if err != nil || projected.Error != nil {
		t.Fatalf("subprocess plan = %#v, %v", projected, err)
	}
	var opaque struct {
		Spec struct{ ApplicationMode, CacheImage, SearchImage, MySQLVersion string } `json:"spec"`
	}
	if err := json.Unmarshal(projected.Plan.Opaque, &opaque); err != nil {
		t.Fatal(err)
	}
	if opaque.Spec.ApplicationMode != "headless" || opaque.Spec.CacheImage != "valkey/valkey:8.1" || opaque.Spec.SearchImage != "opensearchproject/opensearch:2.19.0" || opaque.Spec.MySQLVersion != "8.0.21" {
		t.Fatalf("subprocess lost application or service selections: %#v", opaque.Spec)
	}
	_, err = providerhost.Call[sdk.DeployAppPhaseCall, sdk.DeployAppPhaseResult](ctx, client, sdk.OpCostInputs, &sdk.DeployAppPhaseCall{ProtocolVersion: sdk.ProtocolV1})
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("unsupported operation error = %v", err)
	}
}
