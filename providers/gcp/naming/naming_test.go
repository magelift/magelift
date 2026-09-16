package naming

import "testing"

func TestResourceTruncatesAndSanitizes(t *testing.T) {
	got := Resource("My Project", "Staging Env", "web")
	if got != "my-project-staging-env-web" {
		t.Fatalf("got %q", got)
	}
}

func TestClusterNameFitsGKELimit(t *testing.T) {
	got := ClusterName("mlgcpwt", "preview")
	if got != "mlgcpwt-preview-gke" {
		t.Fatalf("got %q", got)
	}
	if len(got) > 40 {
		t.Fatalf("cluster name exceeds 40 characters: %q", got)
	}
	long := ClusterName("verylongprojectnamehere", "verylongenvironmentname")
	if len(long) > 40 {
		t.Fatalf("truncated cluster name exceeds 40 characters: %q (%d)", long, len(long))
	}
}

func TestClusterNameForRuntimeSeparatesGKEStandard(t *testing.T) {
	autopilot := ClusterNameForRuntime("shop", "production", "gke-autopilot")
	standard := ClusterNameForRuntime("shop", "production", "gke-standard")
	if autopilot == standard || standard != "shop-production-gke-standard" {
		t.Fatalf("runtime cluster names = %q, %q", autopilot, standard)
	}
}

func TestCloudSQLInstanceMatchesResourceSuffix(t *testing.T) {
	got := CloudSQLInstance("My Shop", "Staging")
	if got != "my-shop-staging-sql" {
		t.Fatalf("got %q", got)
	}
}

func TestNodePoolNameFitsGKEConstraint(t *testing.T) {
	got := NodePoolName("verylongmagento-project-name", "high-availability")
	if len(got) >= 40 {
		t.Fatalf("node pool name length = %d (%q), want less than 40", len(got), got)
	}
}
