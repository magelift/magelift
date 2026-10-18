package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResolvedFingerprintIncludesSemanticExtensionChangesButNotRawSecretValues(t *testing.T) {
	first := Config{Extensions: map[string]any{
		"community.network": map[string]any{"mode": "private", "apiToken": "raw-token-a"},
	}}
	second := Config{Extensions: map[string]any{
		"community.network": map[string]any{"mode": "public", "apiToken": "raw-token-b"},
	}}
	firstFingerprint, err := ResolvedFingerprint(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFingerprint, err := ResolvedFingerprint(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint == secondFingerprint {
		t.Fatal("semantic extension change did not change the resolved fingerprint")
	}

	safe := (Effective{Config: first}).SafeEffectiveOutput()
	data, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "raw-token-a") || !strings.Contains(string(data), "redacted") {
		t.Fatalf("safe output leaked or failed to redact a raw token: %s", data)
	}
	if !strings.Contains(string(data), `"fingerprint":"`) {
		t.Fatalf("safe output omitted the resolved fingerprint: %s", data)
	}
}

func TestSafeEffectiveOutputPreservesSecretReferences(t *testing.T) {
	effective := Effective{Config: Config{Edge: EdgeConfig{TokenSecret: "aws-secrets-manager://magelift/fastly"}}}
	data, err := json.Marshal(effective.SafeEffectiveOutput())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "aws-secrets-manager://magelift/fastly") {
		t.Fatalf("secret reference was removed from safe output: %s", data)
	}
}
