package naming

import "testing"

func TestResourceTruncatesAndSanitizes(t *testing.T) {
	got := Resource("My Project", "Staging Env", "web")
	if got != "my-project-staging-env-web" {
		t.Fatalf("got %q", got)
	}
}
