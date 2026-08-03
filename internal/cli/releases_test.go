package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/magelift/magelift/internal/cosign"
	"github.com/magelift/magelift/internal/releasejournal"
)

const releaseOne = "registry.example.invalid/shop@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const releaseTwo = "registry.example.invalid/shop@sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

func TestPromoteRollbackAndHistoryUseForwardJournalEntries(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	verified := 0
	newOptions := func(stdout, stderr *bytes.Buffer) *options {
		o := testOptions(stdout, &fakeTerminal{interactive: false})
		o.stderr = stderr
		o.configPath = configPath
		o.environment = "staging"
		o.output = "json"
		o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
		o.verifyRelease = func(_ context.Context, reference string, options cosign.VerifyOptions) error {
			verified++
			if reference == "" || options.CertificateIdentity == "" || options.OIDCIssuer == "" {
				t.Fatal("signature policy was incomplete")
			}
			return nil
		}
		return o
	}
	for _, digest := range []string{releaseOne, releaseTwo} {
		var stdout, stderr bytes.Buffer
		command := newCommandWithOptions(newOptions(&stdout, &stderr))
		command.SetArgs([]string{"promote", "--digest", digest, "--certificate-identity", "release@example.invalid", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		if stderr.Len() != 0 || !strings.Contains(stdout.String(), `"forwardOnly": true`) {
			t.Fatalf("stdout=%s stderr=%s", stdout.String(), stderr.String())
		}
	}
	if verified != 2 {
		t.Fatalf("signature verifications = %d", verified)
	}

	var rollbackOut, rollbackErr bytes.Buffer
	rollback := newCommandWithOptions(newOptions(&rollbackOut, &rollbackErr))
	rollback.SetArgs([]string{"rollback", "--to-sequence", "1", "--ack-forward-only"})
	if err := rollback.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rollbackErr.String(), "Database migrations are not reversed") || !strings.Contains(rollbackErr.String(), "expand/contract") || !strings.Contains(rollbackOut.String(), releaseOne) || !strings.Contains(rollbackOut.String(), `"sourceSequence": 1`) || !strings.Contains(rollbackOut.String(), `"databaseMigrationsReversed": false`) {
		t.Fatalf("stdout=%s stderr=%s", rollbackOut.String(), rollbackErr.String())
	}

	var historyOut, historyErr bytes.Buffer
	history := newCommandWithOptions(newOptions(&historyOut, &historyErr))
	history.SetArgs([]string{"history"})
	if err := history.Execute(); err != nil {
		t.Fatal(err)
	}
	if historyErr.Len() != 0 || strings.Count(historyOut.String(), `"sequence"`) != 3 || !strings.Contains(historyOut.String(), `"action": "rollback"`) {
		t.Fatalf("stdout=%s stderr=%s", historyOut.String(), historyErr.String())
	}
}

func TestRollbackRequiresForwardOnlyAcknowledgement(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	o := testOptions(&stdout, &fakeTerminal{interactive: false})
	o.stderr = &stderr
	o.configPath = configPath
	o.environment = "staging"
	o.output = "json"
	o.newReleaseStore = func(root, environment string) (releaseStore, error) { return releasejournal.New(root, environment) }
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"rollback", "--to-sequence", "1"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--ack-forward-only") {
		t.Fatalf("error = %v", err)
	}
}

func TestProductionReleaseChangesRequireApproval(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	production := strings.Replace(starterConfig, "    account: \"123456789012\"", "    account: \"123456789012\"\n    class: production", 1)
	if err := os.WriteFile(configPath, []byte(production), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment = configPath, "staging"
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error {
		t.Fatal("verification ran before approval")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"promote", "--digest", releaseOne, "--certificate-identity", "identity", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
	err := command.Execute()
	if err == nil || ExitCode(err) != 2 || !strings.Contains(err.Error(), "requires --yes") {
		t.Fatalf("error = %v", err)
	}
}

func TestPromoteRejectsUnsignedOrMutableReferences(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "magelift.yaml")
	if err := os.WriteFile(configPath, []byte(starterConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	o := testOptions(&output, &fakeTerminal{interactive: false})
	o.configPath, o.environment = configPath, "staging"
	o.verifyRelease = func(context.Context, string, cosign.VerifyOptions) error {
		t.Fatal("mutable reference reached verification")
		return nil
	}
	command := newCommandWithOptions(o)
	command.SetArgs([]string{"promote", "--digest", "registry.example.invalid/shop:latest", "--certificate-identity", "identity", "--certificate-oidc-issuer", "https://issuer.example.invalid"})
	if err := command.Execute(); err == nil || ExitCode(err) != 2 {
		t.Fatalf("error = %v", err)
	}
}
