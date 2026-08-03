package platform_test

import (
	"slices"
	"testing"

	"github.com/acourtiol/magelift/internal/platform"
)

func TestOutputKubeconfigOptionalNotRequired(t *testing.T) {
	t.Parallel()
	if platform.OutputKubeconfig != "kubeconfig" {
		t.Fatalf("OutputKubeconfig = %q, want kubeconfig", platform.OutputKubeconfig)
	}
	if slices.Contains(platform.RequiredOutputKeys(), platform.OutputKubeconfig) {
		t.Fatal("OutputKubeconfig must not be in RequiredOutputKeys (ECS stays free of it)")
	}
}
