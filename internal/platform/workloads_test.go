package platform_test

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
)

func TestMagentoMigrationShellIsStableContract(t *testing.T) {
	t.Parallel()
	cmd := platform.MagentoMigrationShell()
	joined := strings.Join(cmd, " ")
	for _, want := range []string{"app:config:import", "setup:upgrade", "cache:clean", "cache:flush"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("migration shell missing %q: %v", want, cmd)
		}
	}
}

func TestRequireStringOutput(t *testing.T) {
	t.Parallel()
	outputs := map[string]any{"clusterName": "shop-gke", "privateSubnetIds": []any{"a", "b"}}
	got, err := platform.RequireStringOutput(outputs, platform.OutputClusterName)
	if err != nil || got != "shop-gke" {
		t.Fatalf("RequireStringOutput = %q, %v", got, err)
	}
	list, err := platform.RequireStringListOutput(outputs, platform.OutputPrivateSubnetIDs)
	if err != nil || strings.Join(list, ",") != "a,b" {
		t.Fatalf("RequireStringListOutput = %v, %v", list, err)
	}
	if _, err := platform.RequireStringOutput(outputs, "missing"); err == nil {
		t.Fatal("missing output was accepted")
	}
}
