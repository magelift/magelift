package naming_test

import (
	"testing"

	"github.com/magelift/magelift/internal/cloud/aws/naming"
)

func TestAWSNameRespectsLimit(t *testing.T) {
	exact := stringsOfLen(32)
	if got := naming.AWSName(exact, 32); got != exact {
		t.Fatalf("exact name changed: %q", got)
	}
	long := exact + "-x"
	got := naming.AWSName(long, 32)
	if len(got) > 32 || got == long {
		t.Fatalf("truncation failed: %q (%d)", got, len(got))
	}
}

func stringsOfLen(n int) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = 'a'
	}
	return string(b)
}
