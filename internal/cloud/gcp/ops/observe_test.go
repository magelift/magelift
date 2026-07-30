package ops

import (
	"testing"

	"github.com/acourtiol/magelift/internal/cloud/kube"
	"github.com/acourtiol/magelift/internal/platform"
)

func TestRuntimeObserveTypeIdentity(t *testing.T) {
	obs := platform.ModuleRuntimeObserve(Module{})
	if obs == nil {
		t.Fatal("RuntimeObserve returned nil")
	}
	if _, ok := obs.(*kube.Observe); !ok {
		t.Fatalf("want *kube.Observe, got %T", obs)
	}
}
