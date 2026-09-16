package observability

import (
	"sync"
	"testing"

	"github.com/magelift/magelift/sdk"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type mocks struct {
	mu        sync.Mutex
	resources []pulumi.MockResourceArgs
}

func (m *mocks) NewResource(args pulumi.MockResourceArgs) (string, resource.PropertyMap, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resources = append(m.resources, args)
	return args.Name, args.Inputs.Copy(), nil
}

func (m *mocks) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func TestNewCreatesCockpitSourcesAndKeepsUnsupportedSignalsVisible(t *testing.T) {
	observabilityMocks := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{
			ProjectID: "project", Region: "fr-par",
			Intent: sdk.ObservabilityIntent{
				NativeProvider: "scaleway-cockpit", OwnershipMarker: "magelift/architecture/test",
				Signals: []string{"traces", "logs", "metrics", "audit-events"}, RetentionDays: 30,
			},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err != nil {
		t.Fatal(err)
	}
	sources := 0
	for _, resource := range observabilityMocks.resources {
		if resource.TypeToken == "scaleway:observability/source:Source" {
			sources++
		}
	}
	if sources != 3 {
		t.Fatalf("Cockpit source count = %d, resources = %#v", sources, observabilityMocks.resources)
	}
}

func TestUnavailableOperationsRemainExplicit(t *testing.T) {
	got := toUnavailableOperations(sdk.ObservabilityIntent{
		Alerts:     []sdk.AlertIntent{{ID: "queue-health"}},
		Dashboards: []sdk.DashboardIntent{{ID: "runtime"}},
		SLOs:       []sdk.SLOIntent{{ID: "availability"}},
	})
	want := []string{"alert:queue-health", "dashboard:runtime", "slo:availability"}
	if len(got) != len(want) {
		t.Fatalf("unavailable operation count = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unavailable operations = %v, want %v", got, want)
		}
	}
}
