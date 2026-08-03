package endpoint

import (
	"strings"
	"testing"
)

func TestParseAcceptsLoopbackEmulatorEndpoints(t *testing.T) {
	for _, raw := range []string{
		"http://localhost:4566",
		"http://127.0.0.1:4566/",
		"https://[::1]:4566",
	} {
		got, err := Parse(raw)
		if err != nil {
			t.Fatalf("Parse(%q): %v", raw, err)
		}
		if strings.HasSuffix(got, "/") {
			t.Fatalf("Parse(%q) retained a trailing slash: %q", raw, got)
		}
	}
}

func TestParseRejectsCredentialExfiltrationEndpoints(t *testing.T) {
	for _, raw := range []string{
		"https://example.test",
		"http://169.254.169.254/latest",
		"http://user:password@127.0.0.1:4566",
		"http://127.0.0.1:4566/path",
		"ftp://127.0.0.1:4566",
	} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("Parse(%q) accepted an unsafe endpoint", raw)
		}
	}
}

func TestFromEnvUsesTheValidatedOverride(t *testing.T) {
	t.Setenv(EnvironmentVariable, "http://127.0.0.1:4566/")
	got, err := FromEnv()
	if err != nil || got != "http://127.0.0.1:4566" {
		t.Fatalf("FromEnv() = %q, %v", got, err)
	}
}
