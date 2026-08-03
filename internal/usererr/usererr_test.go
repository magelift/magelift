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

func TestFormatAndAs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		err        error
		wantCause  string
		wantNext   string
		wantDoc    string
		asOK       bool
		omitNext   bool
		omitDocs   bool
	}{
		{
			name:      "format with docs",
			err:       Format("run doctor", "docs/getting-started.md", "missing %s", "digest"),
			wantCause: "missing digest",
			wantNext:  "run doctor",
			wantDoc:   "docs/getting-started.md",
			asOK:      true,
		},
		{
			name:     "plain error is not usererr",
			err:      errors.New("boom"),
			asOK:     false,
			omitNext: true,
			omitDocs: true,
		},
		{
			name:      "cause only",
			err:       New("stopped", "", ""),
			wantCause: "stopped",
			asOK:      true,
			omitNext:  true,
			omitDocs:  true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := As(tc.err)
			if ok != tc.asOK {
				t.Fatalf("As ok=%v want %v", ok, tc.asOK)
			}
			if !tc.asOK {
				return
			}
			text := got.Error()
			if !strings.Contains(text, tc.wantCause) {
				t.Fatalf("missing cause %q in %q", tc.wantCause, text)
			}
			if tc.omitNext && strings.Contains(text, "Next:") {
				t.Fatalf("unexpected Next in %q", text)
			}
			if !tc.omitNext && !strings.Contains(text, "Next: "+tc.wantNext) {
				t.Fatalf("missing next in %q", text)
			}
			if tc.omitDocs && strings.Contains(text, "Docs:") {
				t.Fatalf("unexpected Docs in %q", text)
			}
			if !tc.omitDocs && !strings.Contains(text, "Docs: "+tc.wantDoc) {
				t.Fatalf("missing docs in %q", text)
			}
		})
	}
}
