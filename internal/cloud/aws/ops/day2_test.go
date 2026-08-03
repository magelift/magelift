package ops

import (
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/platform"
	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestPrepareExecRejectsDeployWorkload(t *testing.T) {
	t.Parallel()
	_, err := (Observe{}).PrepareExec(t.Context(), nil, map[string]any{
		"clusterName": "shop-cluster",
		"serviceName": "shop-web",
	}, platform.ExecQuery{Workload: sdk.WorkloadID("deploy"), Command: []string{"/bin/sh"}})
	if err == nil || !strings.Contains(err.Error(), "does not support --service deploy") {
		t.Fatalf("expected deploy rejection, got %v", err)
	}
}
