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
