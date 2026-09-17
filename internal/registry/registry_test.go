package registry

import (
	"testing"

	"github.com/magelift/magelift/sdk"
)

func TestNewDefaultRegistersAlphaSet(t *testing.T) {
	t.Parallel()
	modules, err := NewDefault()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
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
	t.Parallel()
	modules, err := NewDefaultWithExperimental()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][2]string{
		{"aws", "ecs-fargate"},
		{"aws", "eks"},
		{"ovh", "mks"},
		{"scaleway", "kapsule"},
		{"gcp", "gke-autopilot"},
		{"gcp", "gke-standard"},
	} {
		if _, found := modules.Module(sdk.ProviderID(want[0]), sdk.RuntimeID(want[1])); !found {
			t.Errorf("experimental set lacks %s/%s", want[0], want[1])
		}
	}
}
