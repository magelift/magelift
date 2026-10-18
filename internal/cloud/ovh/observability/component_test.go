package observability

import (
	"strings"
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
	state := args.Inputs.Copy()
	state["subscriptionId"] = resource.NewStringProperty("subscription-1")
	return args.Name, state, nil
}

func (m *mocks) Call(pulumi.MockCallArgs) (resource.PropertyMap, error) {
	return resource.PropertyMap{}, nil
}

func TestNewCreatesAuditSubscriptionFromOpaqueStreamReference(t *testing.T) {
	observabilityMocks := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{
			ServiceName: "project", ClusterID: pulumi.String("cluster-1"),
			Intent: sdk.ObservabilityIntent{
				NativeProvider: "ovh-logs-data-platform", NativeReference: "stream-1", OwnershipMarker: "magelift/architecture/test",
				Signals: []string{"audit-events"},
			},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, resource := range observabilityMocks.resources {
		if strings.Contains(resource.TypeToken, "KubeLogSubscription") {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("OVH audit subscription was not registered: %#v", observabilityMocks.resources)
	}
}

func TestNewRejectsAuditWithoutStreamReferenceBeforeRegisteringResources(t *testing.T) {
	observabilityMocks := &mocks{}
	err := pulumi.RunErr(func(ctx *pulumi.Context) error {
		_, err := New(ctx, "shop-observability", Args{
			ServiceName: "project", ClusterID: pulumi.String("cluster-1"),
			Intent: sdk.ObservabilityIntent{
				NativeProvider: "ovh-logs-data-platform", OwnershipMarker: "magelift/architecture/test", Signals: []string{"audit-events"},
			},
		})
		return err
	}, pulumi.WithMocks("magelift", "test", observabilityMocks))
	if err == nil || !strings.Contains(err.Error(), "nativeReference") {
		t.Fatalf("missing OVH stream reference error = %v", err)
	}
	if len(observabilityMocks.resources) != 0 {
		t.Fatalf("missing stream reference registered resources before failing: %#v", observabilityMocks.resources)
	}
}

func TestUnavailableSignalsAndOperationsRemainExplicit(t *testing.T) {
	intent := sdk.ObservabilityIntent{
		Signals:    []string{"metrics", "audit-events", "logs"},
		Alerts:     []sdk.AlertIntent{{ID: "queue-health"}},
		Dashboards: []sdk.DashboardIntent{{ID: "runtime"}},
		SLOs:       []sdk.SLOIntent{{ID: "availability"}},
	}
	if got := unavailableSignals(intent); len(got) != 2 || got[0] != "logs" || got[1] != "metrics" {
		t.Fatalf("unavailable OVH signals = %v", got)
	}
	want := []string{"alert:queue-health", "dashboard:runtime", "slo:availability"}
	got := unavailableOperations(intent)
	if len(got) != len(want) {
		t.Fatalf("unavailable OVH operations = %v", got)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("unavailable OVH operations = %v, want %v", got, want)
		}
	}
}
