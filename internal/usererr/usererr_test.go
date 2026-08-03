package usererr

import (
	"errors"
	"strings"
	"testing"
)

func TestErrorIncludesNextAndDocs(t *testing.T) {
	err := New("config is invalid", "run magelift config validate --env staging", "docs/configuration.md")
	text := err.Error()
	for _, want := range []string{"config is invalid", "Next: run magelift", "Docs: docs/configuration.md"} {
		if !strings.Contains(text, want) {
			t.Fatalf("error text missing %q: %s", want, text)
		}
	}
}

func TestWrapPreservesCauseChain(t *testing.T) {
	inner := errors.New("no such host")
	err := Wrap(inner, "cannot reach AWS", "check AWS_PROFILE and network", "docs/bootstrap.md")
	if !errors.Is(err, inner) {
		t.Fatalf("errors.Is failed: %v", err)
	}
}
