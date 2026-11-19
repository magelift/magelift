package platform

import (
	"context"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/config"
	"github.com/magelift/magelift/sdk"
)

type serviceTestAdapter struct {
	target sdk.TargetDescriptor
	output sdk.ServiceBinding
	input  sdk.ServiceRequest
}

func (a *serviceTestAdapter) ServiceDescriptor() sdk.ServiceAdapterDescriptor {
	return sdk.ServiceAdapterDescriptor{APIVersion: sdk.ExtensionAPIVersion, ID: "example.search", Version: "1.0", Source: "example.org/search", Roles: []string{"search"}, Targets: []sdk.TargetDescriptor{a.target}}
}

func (a *serviceTestAdapter) ResolveService(_ context.Context, request sdk.ServiceRequest) (sdk.ServiceBinding, error) {
	a.input = request
	return a.output, nil
}

type serviceTestModule struct {
	fakeModule
	binding sdk.ServiceBinding
}

func (m *serviceTestModule) Plan(cfg config.Config, _ string, _ PlanOptions) (PlannedStack, error) {
	m.binding = cfg.Services["search"]
	return nil, nil
}

func TestPlanResolvesExplicitServiceAdapter(t *testing.T) {
	r := NewModuleRegistry()
	m := &serviceTestModule{fakeModule: fakeModule{id: "aws.ecs-fargate", provider: "aws", runtime: "ecs-fargate", tier: TierBuiltIn, keys: RequiredOutputKeys()}}
	if err := r.RegisterModule(m); err != nil {
		t.Fatal(err)
	}
	input := sdk.ServiceBinding{Family: "hosted-search", Adapter: "example.search", Version: "37.2"}
	cfg := config.Config{Project: config.Project{Name: "shop"}, Target: config.Target{Provider: "aws", Runtime: "ecs-fargate"}, Services: map[string]sdk.ServiceBinding{"search": input}, Extensions: map[string]any{"example.search": "eu", "unrelated.extension": "private"}}
	plan := func() error { _, _, err := r.Plan(cfg, "uat", PlanOptions{}); return err }
	if err := plan(); err == nil || !strings.Contains(err.Error(), "link the service adapter") {
		t.Fatalf("missing adapter: %v", err)
	}
	a := &serviceTestAdapter{target: m.Descriptor(), output: sdk.ServiceBinding{Family: "opensearch", Ownership: "existing", Endpoint: "https://search.example.org"}}
	if err := r.RegisterServiceAdapter(a); err != nil {
		t.Fatal(err)
	}
	if inventory := r.ServiceAdapters(); len(inventory) != 1 || inventory[0].ID != input.Adapter {
		t.Fatalf("missing linked service adapter in inventory: %v", inventory)
	}
	if err := r.RegisterServiceAdapter(a); err == nil {
		t.Fatal("duplicate adapter accepted")
	}
	if err := plan(); err != nil {
		t.Fatal(err)
	}
	if cfg.Services["search"] != input || a.input.Binding != input || a.input.Environment != "uat" || a.input.Project != "shop" || string(a.input.Options) != `"eu"` || m.binding.Version != input.Version || m.binding.Endpoint != a.output.Endpoint || !m.binding.Attached() {
		t.Fatalf("unexpected adapter input or planned binding: %#v %#v", a.input, m.binding)
	}
	a.output.Endpoint = "https://somebody:password@search.example.org"
	if err := plan(); err == nil || strings.Contains(err.Error(), "password") {
		t.Fatalf("credential-bearing endpoint: %v", err)
	}
	a.output.Endpoint = "search.example.org"
	a.output.Ownership = "managed"
	if err := plan(); err == nil {
		t.Fatal("accepted provider ownership from connection adapter")
	}
	a.output.Ownership, a.output.Version = "external", "37.3"
	if err := plan(); err == nil {
		t.Fatal("accepted an unrequested version change")
	}
}
