package naming

import (
	"strings"
	"testing"
)

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

func TestNodeServiceAccountIDFitsGCPLimit(t *testing.T) {
	got := NodeServiceAccountID("shop", "preview")
	if got != "ml-shop-preview-nodes" {
		t.Fatalf("got %q", got)
	}
	if len(got) > 30 {
		t.Fatalf("id length %d exceeds 30: %q", len(got), got)
	}
	if got[0] < 'a' || got[0] > 'z' {
		t.Fatalf("id must start with a letter: %q", got)
	}
}

func TestNodeServiceAccountIDCompactsLongNames(t *testing.T) {
	longProject := "verylongmagento-project-name-here"
	longEnvironment := "high-availability-preview-env"
	got := NodeServiceAccountID(longProject, longEnvironment)
	if len(got) > 30 {
		t.Fatalf("id length %d: %q", len(got), got)
	}
	if !strings.HasPrefix(got, "ml-") || !strings.Contains(got, "-") {
		t.Fatalf("unexpected compact id %q", got)
	}
}

func TestNodeServiceAccountIDCollisionResistance(t *testing.T) {
	a := NodeServiceAccountID("aaaa-project-one", "staging-environment-x")
	b := NodeServiceAccountID("aaaa-project-two", "staging-environment-y")
	if a == b {
		t.Fatalf("distinct inputs produced the same id %q", a)
	}
}

func TestNodeServiceAccountEmailUsesProjectAndID(t *testing.T) {
	id := NodeServiceAccountID("shop", "preview")
	got := NodeServiceAccountEmail("example-gcp-project", "shop", "preview")
	want := id + "@example-gcp-project.iam.gserviceaccount.com"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNodeServiceAccountIDSanitizesNumericSegments(t *testing.T) {
	got := NodeServiceAccountID("Shop 9", "Staging 1")
	if got != "ml-shop-9-staging-1-nodes" {
		t.Fatalf("got %q", got)
	}
}

func TestNodePoolNameFitsGKEConstraint(t *testing.T) {
	got := NodePoolName("verylongmagento-project-name", "high-availability")
	if len(got) >= 40 {
		t.Fatalf("node pool name length = %d (%q), want less than 40", len(got), got)
	}
}
