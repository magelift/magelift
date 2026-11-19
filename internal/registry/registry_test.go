package registry

import (
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestNewDefaultRegistersAlphaSet(t *testing.T) {
	modules, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
		{"azure", "aks"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("alpha set lacks %s/%s", want[0], want[1])
		}
	}
	for _, absent := range [][2]string{
		{"aws", "eks"},
		{"ovh", "mks"},
		{"scaleway", "kapsule"},
	} {
		if _, found := modules.Module(sdk.ProviderID(absent[0]), sdk.RuntimeID(absent[1])); found {
			t.Errorf("alpha set registers deferred %s/%s", absent[0], absent[1])
		}
	}
}

func TestNewDefaultWithExperimentalAddsDeferred(t *testing.T) {
	modules, err := NewDefaultWithExperimental()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"aws", "eks"},
		{"ovh", "mks"},
		{"scaleway", "kapsule"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
		{"azure", "aks"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("experimental set lacks %s/%s", want[0], want[1])
		}
	}
}

func TestDefaultSetSharesHookAndModuleLifecycle(t *testing.T) {
	modules, hooks, err := NewDefaultSet()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(modules.Close)
	if hooks.Close != nil {
		t.Fatal("default hooks must share module-owned provider sessions")
	}
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
		{"azure", "aks"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("default set lacks %s/%s", want[0], want[1])
		}
	}
}
