package platform

import (
	"strings"
	"testing"

	sdk "github.com/magelift/magelift/sdk/v1"
)

func TestFormatStackNameIncludesProviderAndRuntime(t *testing.T) {
	t.Parallel()
	got := FormatStackName("shop", "staging", sdk.ProviderID("aws"), sdk.RuntimeID("ecs-fargate"))
	if got != "shop-staging-aws-ecs-fargate" {
		t.Fatalf("FormatStackName = %q", got)
	}
	got = FormatStackName("shop", "staging", sdk.ProviderID("gcp"), sdk.RuntimeID("gke-autopilot"))
	if got != "shop-staging-gcp-gke-autopilot" {
		t.Fatalf("FormatStackName = %q", got)
	}
}

func TestRequireOutputs(t *testing.T) {
	t.Parallel()
	err := RequireOutputs(map[string]any{"applicationURL": "https://x"}, []string{"applicationURL", "clusterName"})
	if err == nil || !strings.Contains(err.Error(), "clusterName") {
		t.Fatalf("error = %v", err)
	}
	if err := RequireOutputs(map[string]any{"a": 1, "b": 2}, []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
}
